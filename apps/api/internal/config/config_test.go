package config

import (
	"testing"
	"time"
)

// clearEnv blanks every variable Load reads so each case starts from defaults.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"APP_ENV", "DATABASE_URL", "RABBITMQ_URL", "SESSION_TTL", "CORS_ORIGIN", "TZ",
		"MEDIA_STORAGE", "MEDIA_ROOT", "S3_BUCKET", "AWS_REGION", "PORT",
		"DEMO_MODE", "DEMO_CLOCK_START"} {
		t.Setenv(k, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Env != "development" || cfg.Port != 8080 || cfg.Timezone != "Asia/Colombo" {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if cfg.SessionTTL != 12*time.Hour {
		t.Errorf("SessionTTL = %v, want 12h default", cfg.SessionTTL)
	}
	if cfg.MediaStorage != StorageLocal {
		t.Errorf("MediaStorage = %q, want local so Compose needs no AWS", cfg.MediaStorage)
	}
	if cfg.DemoMode {
		t.Error("DemoMode = true, want it off unless DEMO_MODE is set")
	}
	wantStart := time.Date(2026, 9, 25, 15, 40, 0, 0, time.FixedZone("", 5*3600+30*60))
	if !cfg.DemoClockStart.Equal(wantStart) {
		t.Errorf("DemoClockStart = %v, want %v", cfg.DemoClockStart, wantStart)
	}
}

func TestLoadSessionTTL(t *testing.T) {
	clearEnv(t)
	t.Setenv("SESSION_TTL", "30m")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SessionTTL != 30*time.Minute {
		t.Errorf("SessionTTL = %v, want 30m", cfg.SessionTTL)
	}
}

func TestLoadDemoSettings(t *testing.T) {
	clearEnv(t)
	t.Setenv("DEMO_MODE", "true")
	t.Setenv("DEMO_CLOCK_START", "2026-09-26T03:30:00+05:30")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.DemoMode {
		t.Error("DemoMode = false, want true from DEMO_MODE")
	}
	want := time.Date(2026, 9, 25, 22, 0, 0, 0, time.UTC)
	if !cfg.DemoClockStart.Equal(want) {
		t.Errorf("DemoClockStart = %v, want %v", cfg.DemoClockStart, want)
	}
}

func TestLoadRejectsInvalid(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"non-numeric port", map[string]string{"PORT": "eighty"}},
		{"unknown timezone", map[string]string{"TZ": "Mars/Olympus"}},
		{"invalid session ttl", map[string]string{"SESSION_TTL": "forever"}},
		{"non-positive session ttl", map[string]string{"SESSION_TTL": "0s"}},
		{"unknown media backend", map[string]string{"MEDIA_STORAGE": "ftp"}},
		{"s3 without bucket", map[string]string{"MEDIA_STORAGE": "s3", "AWS_REGION": "ap-south-1"}},
		{"s3 without region", map[string]string{"MEDIA_STORAGE": "s3", "S3_BUCKET": "b"}},
		{"non-boolean demo mode", map[string]string{"DEMO_MODE": "maybe"}},
		{"demo start without offset", map[string]string{"DEMO_CLOCK_START": "2026-09-25 15:40"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Fatal("Load() error = nil, want a validation error")
			}
		})
	}
}

// TestLoadProductionWithoutSecret proves authentication needs no Go signing
// secret: sessions are opaque and stored server-side, so production starts
// without one.
func TestLoadProductionWithoutSecret(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", "production")
	if _, err := Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}
