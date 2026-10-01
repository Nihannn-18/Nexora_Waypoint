// Command api is the Waypoint REST API server.
//
//	go run ./cmd/api          # from apps/api
//	nx serve api              # from the repo root
//
// Scaffold state: configuration, logging, routing, graceful shutdown and the
// health endpoints are in place. Database, queue and the role handlers are not
// yet wired — see apps/api/README.md for the order to build them in.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Embeds the IANA timezone database in the binary. Without it, a static
	// build in a minimal container cannot resolve "Asia/Colombo", and every
	// delivery window and the 16:00 cutoff would silently fall back to UTC —
	// a five-and-a-half-hour error in the one calculation that matters most.
	_ "time/tzdata"

	"waypoint.lk/api/internal/config"
	"waypoint.lk/api/internal/httpx"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped with an error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	setupLogging(cfg)

	started := time.Now()
	server := &http.Server{
		Addr:    cfg.Addr(),
		Handler: httpx.Router(cfg, started),
		// A slow or malicious client must not be able to hold a connection open
		// indefinitely. Write timeout is generous because a planning board
		// response can be large.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Shut down on SIGINT/SIGTERM so an in-flight allocation confirmation is
	// allowed to finish its transaction instead of being cut mid-write.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverError := make(chan error, 1)
	go func() {
		slog.Info("waypoint api listening",
			"addr", server.Addr,
			"env", cfg.Env,
			"timezone", cfg.Timezone,
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverError <- err
		}
	}()

	select {
	case err := <-serverError:
		return err
	case <-ctx.Done():
		slog.Info("shutdown signal received, draining connections")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		// Force the remaining connections closed rather than hang the container.
		_ = server.Close()
		return err
	}

	slog.Info("shutdown complete")
	return nil
}

func setupLogging(cfg config.Config) {
	level := slog.LevelInfo
	if cfg.IsDevelopment() {
		level = slog.LevelDebug
	}

	var handler slog.Handler
	if cfg.IsDevelopment() {
		// Readable in a terminal during development.
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		// Structured JSON for the deployed environment's log collector.
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}

	slog.SetDefault(slog.New(handler).With("service", "waypoint-api"))
}
