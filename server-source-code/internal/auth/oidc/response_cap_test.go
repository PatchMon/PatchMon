package oidc

import (
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
