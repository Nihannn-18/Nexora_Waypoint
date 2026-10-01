// Package config reads runtime settings from the environment.
//
// Everything has a working default for local development, so `go run` and
// `docker compose up` both start without a hand-written .env. Secrets have no
// safe default: JWTSecret must be set explicitly outside development, and Load
// refuses to start rather than fall back to a known value.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Env is "development", "test" or "production".
	Env string
	// Port the HTTP server listens on.
	Port int
	// DatabaseURL is a libpq connection string for PostgreSQL.
	DatabaseURL string
	// RabbitURL is the AMQP URL for the planning job queue.
	RabbitURL string
	// JWTSecret signs access tokens.
	JWTSecret string
	// TokenTTL is how long an access token stays valid.
	TokenTTL time.Duration
	// CORSOrigin is the web origin allowed to call this API.
	CORSOrigin string
	// Timezone the business day is reckoned in. The 16:00 cutoff and all
	// delivery windows are wall-clock times in this zone.
	Timezone string
}

const devJWTSecret = "dev-only-insecure-secret-change-me"

// Load reads the environment and validates it.
func Load() (Config, error) {
	cfg := Config{
		Env:         getEnv("APP_ENV", "development"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://waypoint:waypoint@localhost:5432/waypoint?sslmode=disable"),
		RabbitURL:   getEnv("RABBITMQ_URL", "amqp://waypoint:waypoint@localhost:5672/"),
		JWTSecret:   getEnv("JWT_SECRET", ""),
		CORSOrigin:  getEnv("CORS_ORIGIN", "http://localhost:3000"),
		Timezone:    getEnv("TZ", "Asia/Colombo"),
	}

	port, err := strconv.Atoi(getEnv("PORT", "8080"))
	if err != nil {
		return Config{}, fmt.Errorf("PORT must be a number: %w", err)
	}
	cfg.Port = port

	ttlMinutes, err := strconv.Atoi(getEnv("TOKEN_TTL_MINUTES", "720"))
	if err != nil {
		return Config{}, fmt.Errorf("TOKEN_TTL_MINUTES must be a number: %w", err)
	}
	cfg.TokenTTL = time.Duration(ttlMinutes) * time.Minute

	if cfg.JWTSecret == "" {
		if cfg.Env == "production" {
			return Config{}, errors.New("JWT_SECRET must be set when APP_ENV=production")
		}
		cfg.JWTSecret = devJWTSecret
	}

	if _, err := time.LoadLocation(cfg.Timezone); err != nil {
		return Config{}, fmt.Errorf("TZ %q is not a known timezone: %w", cfg.Timezone, err)
	}

	return cfg, nil
}

// IsDevelopment reports whether verbose errors and permissive CORS are acceptable.
func (c Config) IsDevelopment() bool { return c.Env == "development" }

// Addr is the listen address for the HTTP server.
func (c Config) Addr() string { return fmt.Sprintf(":%d", c.Port) }

// Location is the business timezone, already validated by Load.
func (c Config) Location() *time.Location {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
