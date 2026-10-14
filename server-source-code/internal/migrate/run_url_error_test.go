package migrate

import (
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestRunDoesNotLeakDatabaseURL(t *testing.T) {
	err := Run("postgres://user:s3cr%zzet@db.invalid:5432/x", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("expected an error for a malformed URL")
	}
	if strings.Contains(err.Error(), "s3cr") || strings.Contains(err.Error(), "zzet") {
		t.Fatalf("error leaks the password: %v", err)
	}
}
