package handler

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestNewPendingHostEnablesAgentAutoUpdate(t *testing.T) {
	h := newPendingHost("web-1", "pending-x", "patchmon_abc", "hash")
	if !h.AutoUpdate {
		t.Fatal("new hosts must default to auto_update=true so the global setting alone decides")
	}
	if h.Status != "pending" || h.MachineID == nil || *h.MachineID != "pending-x" || h.FriendlyName != "web-1" || h.ApiID != "patchmon_abc" || h.ApiKey != "hash" {
		t.Fatalf("unexpected host: %+v", h)
	}
}

func TestCreateEnrolledHostEnablesAgentAutoUpdate(t *testing.T) {
	fake := &fakeEnrollHosts{}
	h := &AutoEnrollmentHandler{hosts: fake}
	host, _, ok := h.createEnrolledHost(context.Background(), httptest.NewRecorder(), "web-01", "lxc-1", nil, "")
	if !ok || host == nil || len(fake.created) != 1 {
		t.Fatalf("expected one created host, ok=%v", ok)
	}
	if !fake.created[0].AutoUpdate {
		t.Fatal("auto-enrolled hosts must default to auto_update=true")
	}
}
