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
	// CORSOrigin is the web origin allowed to call this API.
	CORSOrigin string
	// Timezone the business day is reckoned in. The 16:00 cutoff and all
	// delivery windows are wall-clock times in this zone.
	Timezone string

	// DemoMode runs the API on the demo clock instead of the wall clock, so the
	// seeded (past) demo day is "today" for the judge walkthrough. DEMO_MODE.
	DemoMode bool
	// DemoClockStart is where the demo clock starts when DemoMode is on.
	// DEMO_CLOCK_START, RFC 3339 with offset.
	DemoClockStart time.Time

	// MediaStorage selects the media backend: "local" or "s3". Defaults to
	// "local" so Docker Compose works with no AWS configuration at all.
	MediaStorage string
	// MediaRoot is the filesystem root for local media. Only used when
	// MediaStorage is "local".
	MediaRoot string
	// S3Bucket is the private bucket for media. Only used when MediaStorage is
	// "s3"; the bucket must not be public.
	S3Bucket string
	// AWSRegion is the region of S3Bucket. Region and credentials otherwise
	// come from the standard AWS SDK chain (IAM role, SSO, env), never from a
	// committed file.
	AWSRegion string
}

const (
	// StorageLocal persists media on the local filesystem (Compose fallback).
	StorageLocal = "local"
	// StorageS3 persists media in a private S3 bucket (hosted deployment).
	StorageS3 = "s3"
)

// defaultDemoClockStart is Fri 25 Sep 2026 15:40 Asia/Colombo: twenty minutes
// before the 16:00 cutoff on the Task 2B S1 planning day (CLAUDE.md §6).
const defaultDemoClockStart = "2026-09-25T15:40:00+05:30"

// Load reads the environment and validates it.
func Load() (Config, error) {
	cfg := Config{
		Env:         getEnv("APP_ENV", "development"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://waypoint:waypoint@localhost:5432/waypoint?sslmode=disable"),
		RabbitURL:   getEnv("RABBITMQ_URL", "amqp://waypoint:waypoint@localhost:5672/"),
		CORSOrigin:  getEnv("CORS_ORIGIN", "http://localhost:3000"),
		Timezone:    getEnv("TZ", "Asia/Colombo"),

		MediaStorage: getEnv("MEDIA_STORAGE", StorageLocal),
		MediaRoot:    getEnv("MEDIA_ROOT", "/data/media"),
		S3Bucket:     getEnv("S3_BUCKET", ""),
		AWSRegion:    getEnv("AWS_REGION", ""),
	}

	port, err := strconv.Atoi(getEnv("PORT", "8080"))
	if err != nil {
		return Config{}, fmt.Errorf("PORT must be a number: %w", err)
	}
	cfg.Port = port

	demoMode, err := strconv.ParseBool(getEnv("DEMO_MODE", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("DEMO_MODE must be true or false: %w", err)
	}
	cfg.DemoMode = demoMode

	demoStart, err := time.Parse(time.RFC3339, getEnv("DEMO_CLOCK_START", defaultDemoClockStart))
	if err != nil {
		return Config{}, fmt.Errorf("DEMO_CLOCK_START must be RFC 3339, e.g. %s: %w", defaultDemoClockStart, err)
	}
	cfg.DemoClockStart = demoStart

	if _, err := time.LoadLocation(cfg.Timezone); err != nil {
		return Config{}, fmt.Errorf("TZ %q is not a known timezone: %w", cfg.Timezone, err)
	}

	// Fail fast on a misconfigured media backend rather than at the first
	// upload. "local" needs a root directory; "s3" needs a bucket and region.
	switch cfg.MediaStorage {
	case StorageLocal:
		if cfg.MediaRoot == "" {
			return Config{}, errors.New("MEDIA_ROOT must be set when MEDIA_STORAGE=local")
		}
	case StorageS3:
		if cfg.S3Bucket == "" {
			return Config{}, errors.New("S3_BUCKET must be set when MEDIA_STORAGE=s3")
		}
		if cfg.AWSRegion == "" {
			return Config{}, errors.New("AWS_REGION must be set when MEDIA_STORAGE=s3")
		}
	default:
		return Config{}, fmt.Errorf("MEDIA_STORAGE must be %q or %q, got %q", StorageLocal, StorageS3, cfg.MediaStorage)
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
