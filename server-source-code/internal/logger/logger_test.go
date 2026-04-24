package logger

import (
	"testing"
)

func TestNew_Disabled(t *testing.T) {
	log := New(Config{Enabled: false})
	if log == nil {
		t.Fatal("New() returned nil")
	}
	log.Info("test")
}

func TestNew_Enabled(t *testing.T) {
	log := New(Config{Enabled: true, Level: "debug"})
	if log == nil {
		t.Fatal("New() returned nil")
	}
	log.Info("test")
}

func TestNew_Levels(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error"} {
		log := New(Config{Enabled: true, Level: level})
		if log == nil {
			t.Errorf("New(level=%q) returned nil", level)
		}
	}
}

func TestNew_InvalidLevel(t *testing.T) {
	log := New(Config{Enabled: true, Level: "invalid"})
	if log == nil {
		t.Fatal("New() returned nil for invalid level")
	}
}
