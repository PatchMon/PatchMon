package queue

import (
	"encoding/json"
	"testing"
	"time"
)

func decodeReportNowPayload(t *testing.T, raw []byte) ReportNowPayload {
	t.Helper()
	var p ReportNowPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return p
}

func TestCatchUpReportTaskIsGatedOnOverdue(t *testing.T) {
	task, err := NewCatchUpReportTask("patchmon_abc123", "tenant.example.com", 12*time.Second)
	if err != nil {
		t.Fatalf("NewCatchUpReportTask: %v", err)
	}
	if task.Type() != TypeReportNow {
		t.Errorf("type = %q, want %q", task.Type(), TypeReportNow)
	}

	p := decodeReportNowPayload(t, task.Payload())
	if !p.OnlyIfOverdue {
		t.Error("OnlyIfOverdue = false, want true; the worker would send unconditionally")
	}
	if p.ApiID != "patchmon_abc123" || p.Host != "tenant.example.com" {
		t.Errorf("payload = %+v, want the api id and host key preserved", p)
	}
}

func TestOperatorFetchIsNotGatedOnOverdue(t *testing.T) {
	task, err := NewReportNowTask("patchmon_abc123", "tenant.example.com")
	if err != nil {
		t.Fatalf("NewReportNowTask: %v", err)
	}
	if p := decodeReportNowPayload(t, task.Payload()); p.OnlyIfOverdue {
		t.Error("OnlyIfOverdue = true on an operator-triggered fetch; Fetch Report would silently do nothing on a host that is reporting normally")
	}
}

// The unique lock is keyed on the payload bytes, so two hosts must not collide
// and one host must produce a stable key across reconnects.
func TestCatchUpPayloadDistinguishesHosts(t *testing.T) {
	a, err := NewCatchUpReportTask("patchmon_aaa", "tenant.example.com", 0)
	if err != nil {
		t.Fatalf("NewCatchUpReportTask: %v", err)
	}
	b, err := NewCatchUpReportTask("patchmon_bbb", "tenant.example.com", 0)
	if err != nil {
		t.Fatalf("NewCatchUpReportTask: %v", err)
	}
	if string(a.Payload()) == string(b.Payload()) {
		t.Error("two hosts produced identical payloads, so one would suppress the other's catch-up")
	}

	again, err := NewCatchUpReportTask("patchmon_aaa", "tenant.example.com", 45*time.Second)
	if err != nil {
		t.Fatalf("NewCatchUpReportTask: %v", err)
	}
	if string(a.Payload()) != string(again.Payload()) {
		t.Error("payload varies for the same host, so repeat reconnects would not deduplicate")
	}
}
