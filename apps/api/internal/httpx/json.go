// Package httpx holds the HTTP plumbing: JSON helpers, middleware and the router.
//
// Handlers stay thin. No planning formula, capacity check or feasibility rule
// belongs in this package — those live in internal/planning and the constraint
// validator, which are unit-tested without a server.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"waypoint.lk/api/internal/domain"
)

// ErrorBody is the single error shape the web client parses. Its field names
// match ApiErrorBody in libs/api-client/src/lib/http.ts.
type ErrorBody struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	// ConstraintResults is populated on 422 from the allocation endpoints so the
	// dispatcher's rule panel can highlight exactly which rule blocked the plan.
	ConstraintResults []domain.ConstraintResult `json:"constraintResults,omitempty"`
}

// WriteJSON sends v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status line is already sent, so this can only be logged.
		slog.Error("failed to encode response", "error", err)
	}
}

// WriteError sends a problem the client can act on.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, ErrorBody{Message: message})
}

// WriteConstraintViolation reports an infeasible allocation as 422 together with
// every rule result, passed or failed, so the response is explainable.
func WriteConstraintViolation(w http.ResponseWriter, results []domain.ConstraintResult) {
	WriteJSON(w, http.StatusUnprocessableEntity, ErrorBody{
		Message:           "Allocation is not feasible",
		Code:              "CONSTRAINT_VIOLATION",
		ConstraintResults: results,
	})
}

// maxBodyBytes caps a request body. Proof-of-delivery images are uploaded
// separately, so no legitimate JSON body approaches this.
const maxBodyBytes = 1 << 20 // 1 MiB

// DecodeJSON reads exactly one JSON object into dst, rejecting unknown fields.
// Strictness is deliberate: a typo'd field name in a client payload should fail
// loudly rather than be silently dropped and produce a wrong allocation.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var syntax *json.SyntaxError
		var unmarshal *json.UnmarshalTypeError

		switch {
		case errors.As(err, &syntax):
			return fmt.Errorf("malformed JSON at byte %d", syntax.Offset)
		case errors.As(err, &unmarshal):
			return fmt.Errorf("field %q has the wrong type", unmarshal.Field)
		case errors.Is(err, io.EOF):
			return errors.New("request body is empty")
		default:
			return err
		}
	}

	// A second value in the body usually means a client bug; refuse it.
	if dec.More() {
		return errors.New("request body must contain a single JSON object")
	}

	return nil
}
