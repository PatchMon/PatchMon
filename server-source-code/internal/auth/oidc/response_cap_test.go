package oidc

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExchangeCapsProviderResponseBodies(t *testing.T) {
	oversized := maxProviderResponseBytes + 1
	for name, setup := range map[string]func(*fakeOP){
		"userinfo": func(op *fakeOP) {
			op.userInfo = func(w http.ResponseWriter) {
				_, _ = w.Write([]byte(strings.Repeat(" ", oversized)))
				_ = json.NewEncoder(w).Encode(map[string]any{"sub": op.sub, "email": "alice@example.com"})
			}
		},
		"gzipped userinfo": func(op *fakeOP) {
			op.userInfo = func(w http.ResponseWriter) {
				writeGzip(w, append([]byte(strings.Repeat(" ", oversized)), []byte(`{"sub":"user-1","email":"alice@example.com"}`)...))
			}
		},
		"jwks":      func(op *fakeOP) { op.jwksPadding = oversized },
		"discovery": func(op *fakeOP) { op.discoveryPad = oversized },
	} {
		t.Run(name, func(t *testing.T) {
			op := newFakeOP(t, "user-1")
			setup(op)
			if _, err := op.exchange(t); err == nil || !strings.Contains(err.Error(), errProviderResponseTooLarge.Error()) {
				t.Fatalf("expected the size limit error, got %v", err)
			}
		})
	}
}

// oauth2 reads at most 1 MiB of a token response itself, so an oversized one
// fails as unparseable before this package's cap is reached.
func TestExchangeRejectsOversizedTokenResponse(t *testing.T) {
	op := newFakeOP(t, "user-1")
	op.tokenPad = maxProviderResponseBytes + 1
	if _, err := op.exchange(t); err == nil || !strings.Contains(err.Error(), "token exchange") {
		t.Fatalf("expected the token exchange to fail, got %v", err)
	}
}

func TestExchangeAcceptsResponseAtTheCap(t *testing.T) {
	op := newFakeOP(t, "user-1")
	op.userInfo = func(w http.ResponseWriter) {
		body, _ := json.Marshal(map[string]any{"sub": op.sub, "email": "alice@example.com", "email_verified": true})
		_, _ = w.Write([]byte(strings.Repeat(" ", maxProviderResponseBytes-len(body))))
		_, _ = w.Write(body)
	}
	if _, err := op.exchange(t); err != nil {
		t.Fatalf("exchange: %v", err)
	}
}

func TestExchangeAcceptsGzippedResponseUnderTheCap(t *testing.T) {
	op := newFakeOP(t, "user-1")
	op.userInfo = func(w http.ResponseWriter) {
		writeGzip(w, []byte(`{"sub":"user-1","email":"alice@example.com","email_verified":true}`))
	}
	if _, err := op.exchange(t); err != nil {
		t.Fatalf("exchange: %v", err)
	}
}

func writeGzip(w http.ResponseWriter, body []byte) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(body)
	_ = zw.Close()
	w.Header().Set("Content-Encoding", "gzip")
	_, _ = w.Write(buf.Bytes())
}
