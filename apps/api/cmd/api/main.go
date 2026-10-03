// Command api is the Waypoint REST API server.
//
//	go run ./cmd/api          # from apps/api
//	nx serve api              # from the repo root
//
// Start-up order: configuration, PostgreSQL, migrations, reference seed, media
// storage, then the HTTP server with graceful shutdown. The role handlers are
// not wired yet — see apps/api/README.md for the order to build them in.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
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
	"waypoint.lk/api/internal/media"
	"waypoint.lk/api/internal/seed"
	"waypoint.lk/api/internal/store"
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

	// Connect, apply migrations, then seed reference data — in that order. A
	// failure here is fatal: serving requests against an unmigrated or
	// unseeded database would produce wrong answers rather than errors.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		return err
	}
	slog.Info("migrations applied")

	if res, err := seed.Run(ctx, db.Pool()); err != nil {
		return err
	} else {
		slog.Info("reference data seeded",
			"depots", res.Depots,
			"outlets", res.Outlets,
			"vehicles", res.Vehicles,
			"districtTravel", res.DistrictTravel,
			"serviceAllowance", res.ServiceAllowance,
			"calendarDays", res.CalendarDays,
			"demoOrders", res.DemoOrders,
			"demoVehicleDays", res.DemoAvailability,
		)
	}

	// Media backend: local filesystem in Compose, private S3 when configured.
	// The auth resolver/authorizer are placeholders until Better Auth lands, so
	// media endpoints reject unauthenticated calls rather than allow them.
	storage, err := media.NewStorage(ctx, cfg)
	if err != nil {
		return err
	}
	mediaHandler := media.NewHandler(storage, media.UnimplementedResolver{}, media.DenyAuthorizer{})
	slog.Info("media storage ready", "backend", cfg.MediaStorage)

	checks := []httpx.Check{
		{Name: "database", Fn: db.Pool().Ping},
		{Name: "queue", Fn: queueCheck(cfg.RabbitURL)},
	}

	started := time.Now()
	server := &http.Server{
		Addr:    cfg.Addr(),
		Handler: httpx.Router(cfg, started, checks, mediaHandler.RegisterRoutes),
		// A slow or malicious client must not be able to hold a connection open
		// indefinitely. Write timeout is generous because a planning board
		// response can be large.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", server.Addr, err)
	}
	slog.Info("waypoint api listening",
		"addr", server.Addr,
		"env", cfg.Env,
		"timezone", cfg.Timezone,
	)

	// Shutdown is triggered by the same signal context created above, so an
	// in-flight allocation confirmation can finish instead of being cut off.
	return serve(ctx, server, ln, shutdownTimeout)
}

// shutdownTimeout bounds how long in-flight requests may take to drain after a
// shutdown signal before the remaining connections are closed.
const shutdownTimeout = 20 * time.Second

// serve runs server on ln until it fails or ctx is cancelled. On cancellation
// it stops accepting connections and waits up to drain for in-flight requests
// to finish; past that it closes what remains rather than hang the container.
func serve(ctx context.Context, server *http.Server, ln net.Listener, drain time.Duration) error {
	serverError := make(chan error, 1)
	go func() {
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverError <- err
		}
	}()

	select {
	case err := <-serverError:
		return err
	case <-ctx.Done():
		slog.Info("shutdown signal received, draining connections")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), drain)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		// Force the remaining connections closed rather than hang the container.
		_ = server.Close()
		return fmt.Errorf("drain connections: %w", err)
	}

	slog.Info("shutdown complete")
	return nil
}

// queueCheck verifies the RabbitMQ broker accepts TCP connections. The AMQP
// consumer is not wired yet, so reachability is all readiness can honestly say.
func queueCheck(rawURL string) func(context.Context) error {
	return func(ctx context.Context) error {
		u, err := url.Parse(rawURL)
		if err != nil {
			return fmt.Errorf("parse RABBITMQ_URL: %w", err)
		}
		host := u.Host
		if u.Port() == "" {
			host = net.JoinHostPort(u.Hostname(), "5672")
		}
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", host)
		if err != nil {
			return err
		}
		return conn.Close()
	}
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
