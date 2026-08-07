package config

import (
	"testing"
)

func TestLoad_ValidEnv(t *testing.T) {
	t.Setenv("ENV_FILE", "/nonexistent")
	t.Setenv("DATABASE_URL", "postgresql://localhost/test")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("PORT", "3001")
	t.Setenv("ENABLE_LOGGING", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DatabaseURL != "postgresql://localhost/test" {
		t.Errorf("DatabaseURL = %q, want postgresql://localhost/test", cfg.DatabaseURL)
	}
	if cfg.Port != 3001 {
		t.Errorf("Port = %d, want 3001", cfg.Port)
	}
	if cfg.Version != DefaultVersion {
		t.Errorf("Version = %q, want %s", cfg.Version, DefaultVersion)
	}
	if !cfg.EnableLogging {
		t.Error("EnableLogging = false, want true")
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	t.Setenv("ENV_FILE", "/nonexistent")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "test-secret")

	_, err := Load()
	if err == nil {
		t.Error("Load() expected error, got nil")
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := &Config{DatabaseURL: "postgres://x", Port: 0, LogLevel: "info"}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() expected error for port 0")
	}
}
