package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServePatchItemsRequiresAgentCredentials(t *testing.T) {
	h := &PatchingHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/patching/runs/x/items", strings.NewReader(`{"items":[]}`))
	rec := httptest.NewRecorder()
	h.ServePatchItems(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
