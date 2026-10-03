package config

import "testing"

// clearEnv blanks every variable Load reads so each case starts from defaults.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"APP_ENV", "DATABASE_URL", "RABBITMQ_URL", "JWT_SECRET", "CORS_ORIGIN", "TZ",
		"MEDIA_STORAGE", "MEDIA_ROOT", "S3_BUCKET", "AWS_REGION", "PORT", "TOKEN_TTL_MINUTES"} {
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
	if cfg.MediaStorage != StorageLocal {
		t.Errorf("MediaStorage = %q, want local so Compose needs no AWS", cfg.MediaStorage)
	}
}

func TestLoadRejectsInvalid(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"production without secret", map[string]string{"APP_ENV": "production"}},
		{"non-numeric port", map[string]string{"PORT": "eighty"}},
		{"unknown timezone", map[string]string{"TZ": "Mars/Olympus"}},
		{"unknown media backend", map[string]string{"MEDIA_STORAGE": "ftp"}},
		{"s3 without bucket", map[string]string{"MEDIA_STORAGE": "s3", "AWS_REGION": "ap-south-1"}},
		{"s3 without region", map[string]string{"MEDIA_STORAGE": "s3", "S3_BUCKET": "b"}},
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

func TestLoadProductionWithSecret(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "x")
	if _, err := Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}
