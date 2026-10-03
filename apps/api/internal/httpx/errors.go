package httpx

import (
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
)

// Error codes shared by every handler. CONSTRAINT_VIOLATION lives with
// WriteConstraintViolation; domain packages add their own for their own rules.
const (
	CodeBadRequest       = "BAD_REQUEST"
	CodeValidationFailed = "VALIDATION_FAILED"
	CodeNotFound         = "NOT_FOUND"
	CodeInternal         = "INTERNAL_ERROR"
	CodeUnavailable      = "SERVICE_UNAVAILABLE"
)

// FieldError names one invalid request field and why it was rejected.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// WriteBadRequest reports a malformed request, such as an undecodable body.
func WriteBadRequest(w http.ResponseWriter, message string) {
	WriteErrorCode(w, http.StatusBadRequest, CodeBadRequest, message)
}

// Validation collects field errors so a request reports every problem at once
// instead of one per round trip. It checks request shape only; business rules
// belong in services and the constraint validator.
type Validation struct {
	errs []FieldError
}

// Add records a failure for field.
func (v *Validation) Add(field, message string) {
	v.errs = append(v.errs, FieldError{Field: field, Message: message})
}

// Required fails when value is empty or whitespace.
func (v *Validation) Required(field, value string) {
	if strings.TrimSpace(value) == "" {
		v.Add(field, field+" is required")
	}
}

// OneOf fails when value is not one of allowed. An empty value is left to Required.
func (v *Validation) OneOf(field, value string, allowed ...string) {
	if value != "" && !slices.Contains(allowed, value) {
		v.Add(field, fmt.Sprintf("%s must be one of %s", field, strings.Join(allowed, ", ")))
	}
}

// Positive fails when n is zero or negative.
func (v *Validation) Positive(field string, n float64) {
	if n <= 0 {
		v.Add(field, field+" must be greater than zero")
	}
}

// Errors returns the collected failures, or nil when the request is valid.
func (v *Validation) Errors() []FieldError { return v.errs }

// WriteValidation answers 400 VALIDATION_FAILED with every field error.
func WriteValidation(w http.ResponseWriter, errs []FieldError) {
	WriteJSON(w, http.StatusBadRequest, ErrorBody{
		Message:     "The request has invalid fields",
		Code:        CodeValidationFailed,
		FieldErrors: errs,
	})
}

// withRecover turns a handler panic into the standard 500 body instead of a
// dropped connection. The panic value is logged, never sent to the client.
func withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				slog.Error("handler panicked", "method", r.Method, "path", r.URL.Path, "panic", rec)
				WriteErrorCode(w, http.StatusInternalServerError, CodeInternal, "Something went wrong on our side")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
