package httpx

import (
	"log/slog"
	"net/http"
	"time"

	"waypoint.lk/api/internal/config"
)

// Router builds the HTTP handler for the whole API.
//
// Only the operational endpoints (/healthz, /readyz) and a meta endpoint are
// wired. Register the real route groups here as they land —
// docs/api.md lists every endpoint with its role and purpose.
//
// Go 1.22+ ServeMux patterns ("POST /api/v1/orders", "GET /api/v1/orders/{id}")
// cover this API's routing needs, so there is no third-party router dependency
// yet. Swap in chi or gin if you want middleware groups; nothing here depends
// on the choice.
// RouteRegistrar lets feature packages register their own routes without the
// router importing them (which would create an import cycle, since those
// packages use the JSON helpers in this one).
type RouteRegistrar func(mux *http.ServeMux)

// Router builds the HTTP handler for the whole API. checks feed /readyz; feature
// packages register their routes through the extra registrars.
func Router(cfg config.Config, started time.Time, checks []Check, registrars ...RouteRegistrar) http.Handler {
	mux := http.NewServeMux()

	// --- Operational -------------------------------------------------------
	// Liveness: the process is up. Must not touch the database, or a database
	// blip would make the orchestrator kill a healthy API.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"uptimeSec": int(time.Since(started).Seconds()),
		})
	})

	// Readiness: dependencies are reachable and this instance can serve traffic.
	// A failing check returns 503 naming the dependency; details go to the log,
	// not the response.
	mux.HandleFunc("GET /readyz", readiness(checks))

	// --- Meta --------------------------------------------------------------
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]any{
			"service":  "waypoint-api",
			"env":      cfg.Env,
			"timezone": cfg.Timezone,
			"apiBase":  "/api/v1",
		})
	})

	// --- Feature routes ----------------------------------------------------
	// Each feature package registers its own routes, so this package never
	// imports them.
	for _, register := range registrars {
		register(mux)
	}

	// --- API v1 ------------------------------------------------------------
	// Register handlers as they are implemented, e.g.
	//
	//   mux.Handle("POST /api/v1/auth/login",        authHandler.Login())
	//   mux.Handle("GET  /api/v1/orders/queue",      requireRole(domain.RoleDispatcher, orders.Queue()))
	//   mux.Handle("POST /api/v1/allocations/suggest", requireRole(domain.RoleDispatcher, planning.Suggest()))
	//
	// Anything not registered falls through to this 404, which returns the same
	// JSON error shape as every other failure so the client never has to
	// special-case an HTML error page.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, http.StatusNotFound, "No handler for "+r.Method+" "+r.URL.Path)
	})

	return withRecover(withRequestLogging(withCORS(cfg, mux)))
}

// withCORS allows the Next.js origin to call the API with credentials.
func withCORS(cfg config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", cfg.CORSOrigin)
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		w.Header().Set("Vary", "Origin")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the status code so it can be logged.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health checks run every few seconds; logging them buries real traffic.
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"durationMs", time.Since(start).Milliseconds(),
		)
	})
}
