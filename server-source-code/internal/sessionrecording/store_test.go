package sessionrecording

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestEncryptedCompressedRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(t.TempDir(), base64.StdEncoding.EncodeToString(key), 80)
	if err != nil {
		t.Fatal(err)
	}
	w, err := store.NewWriter(context.Background(), "tenant-1", "session-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	want := []Event{
		{OffsetMicros: 0, Type: "marker", Data: "connected"},
		{OffsetMicros: 10, Type: "output", Data: "h?llo \\x1b[31mred\\x1b[0m"},
		{OffsetMicros: 20, Type: "resize", Cols: 140, Rows: 40},
		{OffsetMicros: 30, Type: "input", Data: "ls\\r"},
	}
	for _, event := range want {
		if err := w.Append(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read(context.Background(), "tenant-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events mismatch\n got: %#v\nwant: %#v", got, want)
	}
	files, err := filepath.Glob(filepath.Join(store.root, "tenant-1", "session-1", "*.pmr"))
	if err != nil || len(files) < 2 {
		t.Fatalf("expected multiple blocks, got %v (%v)", files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == string(mustJSON(t, want[0])) {
		t.Fatal("recording block contains plaintext event")
	}
}

func TestWrongKeyCannotReadRecording(t *testing.T) {
	root := t.TempDir()
	store := testStore(t, root, 1)
	w, err := store.NewWriter(context.Background(), "tenant", "session", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(context.Background(), Event{Type: "output", Data: "secret"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := testStore(t, root, 2).Read(context.Background(), "tenant", "session"); err == nil {
		t.Fatal("read with a different key should fail")
	}
}

func TestRejectsUnsafeIdentifiers(t *testing.T) {
	store := testStore(t, t.TempDir(), 1)
	if _, err := store.NewWriter(context.Background(), "../tenant", "session", time.Now()); err == nil {
		t.Fatal("unsafe tenant id should be rejected")
	}
}

func testStore(t *testing.T, root string, fill byte) *Store {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = fill
	}
	store, err := NewStore(root, base64.StdEncoding.EncodeToString(key), 0)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
