package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Check probes one dependency for /readyz. A nil error means reachable.
type Check struct {
	Name string
	Fn   func(ctx context.Context) error
}

// checkTimeout bounds each probe so a hung dependency fails readiness quickly
// instead of stalling the orchestrator's health request.
const checkTimeout = 3 * time.Second

// readiness runs every check and answers 200 when all pass, or 503 naming each
// failing dependency. Liveness (/healthz) deliberately never calls this.
func readiness(checks []Check) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusOK
		state := "ready"
		deps := make(map[string]string, len(checks))

		for _, c := range checks {
			ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
			err := c.Fn(ctx)
			cancel()
			if err != nil {
				slog.Warn("readiness check failed", "dependency", c.Name, "error", err)
				status, state = http.StatusServiceUnavailable, "unavailable"
				deps[c.Name] = "down"
				continue
			}
			deps[c.Name] = "up"
		}

		WriteJSON(w, status, map[string]any{"status": state, "dependencies": deps})
	}
}
