// Package sessionrecording stores encrypted, compressed terminal session events.
package sessionrecording

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"
)

const (
	DefaultBlockSize = 256 * 1024
	formatVersion    = byte(1)
)

var safeID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

type Event struct {
	OffsetMicros int64  `json:"t"`
	Type         string `json:"type"`
	Data         string `json:"data,omitempty"`
	Cols         int    `json:"cols,omitempty"`
	Rows         int    `json:"rows,omitempty"`
}

type Store struct {
	root      string
	aead      cipher.AEAD
	blockSize int
}

func NewStore(root, encodedKey string, blockSize int) (*Store, error) {
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, fmt.Errorf("decode recording key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("recording key must decode to 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create recording cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create recording AEAD: %w", err)
	}
	if root == "" {
		return nil, errors.New("recording directory is required")
	}
	if blockSize <= 0 {
		blockSize = DefaultBlockSize
	}
	return &Store{root: root, aead: aead, blockSize: blockSize}, nil
}

type Writer struct {
	store     *Store
	tenantID  string
	sessionID string
	started   time.Time
	block     int
	plain     bytes.Buffer
	mu        sync.Mutex
	closed    bool
}

func (s *Store) NewWriter(ctx context.Context, tenantID, sessionID string, started time.Time) (*Writer, error) {
	if err := validateID("tenant", tenantID); err != nil {
		return nil, err
	}
	if err := validateID("session", sessionID); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir := s.sessionDir(tenantID, sessionID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create recording directory: %w", err)
	}
	return &Writer{store: s, tenantID: tenantID, sessionID: sessionID, started: started.UTC()}, nil
}

func (w *Writer) Append(ctx context.Context, event Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("recording writer is closed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if event.OffsetMicros < 0 {
		return errors.New("event offset cannot be negative")
	}
	if !validEventType(event.Type) {
		return fmt.Errorf("invalid event type %q", event.Type)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	if len(encoded) > w.store.blockSize {
		return fmt.Errorf("event exceeds recording block size: %d", len(encoded))
	}
	if w.plain.Len() > 0 && w.plain.Len()+len(encoded)+1 > w.store.blockSize {
		if err := w.flushLocked(); err != nil {
			return err
		}
	}
	w.plain.Write(encoded)
	w.plain.WriteByte('\n')
	return nil
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	return w.flushLocked()
}

func (w *Writer) flushLocked() error {
	if w.plain.Len() == 0 {
		return nil
	}
	var compressed bytes.Buffer
	gz, err := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
	if err != nil {
		return fmt.Errorf("create compressor: %w", err)
	}
	if _, err := gz.Write(w.plain.Bytes()); err != nil {
		return fmt.Errorf("compress recording: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("finish recording compression: %w", err)
	}

	nonce := make([]byte, w.store.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("generate recording nonce: %w", err)
	}
	aad := []byte(fmt.Sprintf("%s/%s/%06d", w.tenantID, w.sessionID, w.block))
	ciphertext := w.store.aead.Seal(nil, nonce, compressed.Bytes(), aad)
	payload := make([]byte, 1+2+len(nonce)+len(ciphertext))
	payload[0] = formatVersion
	binary.BigEndian.PutUint16(payload[1:3], uint16(len(nonce)))
	copy(payload[3:], nonce)
	copy(payload[3+len(nonce):], ciphertext)

	path := filepath.Join(w.store.sessionDir(w.tenantID, w.sessionID), fmt.Sprintf("%06d.pmr", w.block))
	tmp, err := os.CreateTemp(filepath.Dir(path), ".recording-*")
	if err != nil {
		return fmt.Errorf("create temporary recording block: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("protect recording block: %w", err)
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write recording block: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync recording block: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close recording block: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("commit recording block: %w", err)
	}
	w.block++
	w.plain.Reset()
	return nil
}

func (s *Store) Read(ctx context.Context, tenantID, sessionID string) ([]Event, error) {
	if err := validateID("tenant", tenantID); err != nil {
		return nil, err
	}
	if err := validateID("session", sessionID); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(s.sessionDir(tenantID, sessionID), "*.pmr"))
	if err != nil {
		return nil, fmt.Errorf("list recording blocks: %w", err)
	}
	sort.Strings(files)
	var events []Event
	for i, path := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		blockEvents, err := s.readBlock(path, []byte(fmt.Sprintf("%s/%s/%06d", tenantID, sessionID, i)))
		if err != nil {
			return nil, err
		}
		events = append(events, blockEvents...)
	}
	return events, nil
}

func (s *Store) readBlock(path string, aad []byte) ([]Event, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read recording block: %w", err)
	}
	if len(payload) < 3 || payload[0] != formatVersion {
		return nil, errors.New("unsupported recording block format")
	}
	nonceLen := int(binary.BigEndian.Uint16(payload[1:3]))
	if nonceLen != s.aead.NonceSize() || len(payload) < 3+nonceLen+s.aead.Overhead() {
		return nil, errors.New("invalid recording block")
	}
	plain, err := s.aead.Open(nil, payload[3:3+nonceLen], payload[3+nonceLen:], aad)
	if err != nil {
		return nil, fmt.Errorf("decrypt recording block: %w", err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(plain))
	if err != nil {
		return nil, fmt.Errorf("open recording block: %w", err)
	}
	defer gz.Close()
	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, 4096), s.blockSize+1)
	var events []Event
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("decode recording event: %w", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read recording events: %w", err)
	}
	return events, nil
}

func (s *Store) Delete(tenantID, sessionID string) error {
	if err := validateID("tenant", tenantID); err != nil {
		return err
	}
	if err := validateID("session", sessionID); err != nil {
		return err
	}
	return os.RemoveAll(s.sessionDir(tenantID, sessionID))
}

func (s *Store) sessionDir(tenantID, sessionID string) string {
	return filepath.Join(s.root, tenantID, sessionID)
}

func validateID(kind, value string) error {
	if !safeID.MatchString(value) {
		return fmt.Errorf("invalid %s id", kind)
	}
	return nil
}

func validEventType(value string) bool {
	switch value {
	case "output", "input", "resize", "marker":
		return true
	default:
		return false
	}
}
