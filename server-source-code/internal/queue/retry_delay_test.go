package queue

import (
	"testing"

	"github.com/hibiken/asynq"
)

func TestRetryDelay(t *testing.T) {
	if got := retryDelay(5, ErrRetryLater, asynq.NewTask(TypeRunScan, nil)); got != complianceScanRetryDelay {
		t.Fatalf("run_scan should retry after %v, got %v", complianceScanRetryDelay, got)
	}
	if got := retryDelay(1, ErrRetryLater, asynq.NewTask(TypeInstallComplianceTools, nil)); got != complianceInstallRetryDelay {
		t.Fatalf("install should retry after %v, got %v", complianceInstallRetryDelay, got)
	}
	if got := retryDelay(0, nil, asynq.NewTask(TypeReportNow, nil)); got <= 0 {
		t.Fatalf("other tasks should keep asynq's default backoff, got %v", got)
	}
}
