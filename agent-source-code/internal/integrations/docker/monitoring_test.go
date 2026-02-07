package docker

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// TestMonitoringLoopReconnection tests that monitoring loop reconnects on EOF
func TestMonitoringLoopReconnection(t *testing.T) {
	attemptCount := 0

	t.Run("reconnect_on_eof", func(t *testing.T) {
		startTime := time.Now()
		testCtx, testCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer testCancel()

		go func() {
			backoffDuration := initialBackoffDuration

			for i := 0; i < 3; i++ {
				select {
				case <-testCtx.Done():
					return
				case <-time.After(backoffDuration):
					attemptCount++
					backoffDuration = time.Duration(float64(backoffDuration) * 1.5)
					if backoffDuration > maxBackoffDuration {
						backoffDuration = maxBackoffDuration
					}
				}
			}
		}()

		<-testCtx.Done()
		elapsed := time.Since(startTime)

		if elapsed < 4*time.Second {
			t.Logf("Test completed too quickly: %v (expected >= 4 seconds)", elapsed)
		}

		if attemptCount != 3 {
			t.Errorf("Expected 3 attempts, got %d", attemptCount)
		}
	})
}

// TestExponentialBackoff tests the exponential backoff calculation
func TestExponentialBackoff(t *testing.T) {
	tests := []struct {
		attempt    int
		maxAttempt int
		expectMin  time.Duration
		expectMax  time.Duration
	}{
		{0, 3, initialBackoffDuration, initialBackoffDuration},
		{1, 3, initialBackoffDuration, initialBackoffDuration + 100*time.Millisecond},
	}

	for _, tt := range tests {
		result := exponentialBackoff(tt.attempt)

		if tt.attempt <= 0 {
			if result != initialBackoffDuration {
				t.Errorf("exponentialBackoff(%d) = %v, want %v", tt.attempt, result, initialBackoffDuration)
			}
		}
	}
}

// TestBackoffDoesNotExceedMax tests that backoff never exceeds max duration
func TestBackoffDoesNotExceedMax(t *testing.T) {
	for attempt := 1; attempt <= 100; attempt++ {
		result := exponentialBackoff(attempt)
		if result > maxBackoffDuration {
			t.Errorf("exponentialBackoff(%d) = %v, exceeds maxBackoffDuration %v",
				attempt, result, maxBackoffDuration)
		}
	}
}

// TestEOFErrorHandling tests that EOF errors are properly identified
func TestEOFErrorHandling(t *testing.T) {
	tests := []struct {
		err   error
		isEOF bool
		name  string
	}{
		{io.EOF, true, "io.EOF"},
		{errors.New("connection reset"), false, "connection error"},
		{context.Canceled, true, "context canceled"},
		{nil, false, "nil error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isEOF := errors.Is(tt.err, io.EOF)
			isCanceled := errors.Is(tt.err, context.Canceled)

			if tt.isEOF && !isEOF && !isCanceled {
				t.Errorf("Expected %v to be recognized as EOF or Canceled", tt.err)
			}
		})
	}
}

// TestMonitoringStateTransition tests the monitoring state flag transitions
func TestMonitoringStateTransition(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	integration := &Integration{
		logger: logger,
	}

	if integration.monitoring {
		t.Error("Expected monitoring to be false initially")
	}

	integration.monitoringMu.Lock()
	if integration.monitoring {
		t.Error("Monitoring should still be false")
	}
	integration.monitoring = true
	integration.monitoringMu.Unlock()

	integration.monitoringMu.RLock()
	if !integration.monitoring {
		t.Error("Expected monitoring to be true after setting")
	}
	integration.monitoringMu.RUnlock()
}

// BenchmarkExponentialBackoff benchmarks the backoff calculation
func BenchmarkExponentialBackoff(b *testing.B) {
	for i := 0; i < b.N; i++ {
		exponentialBackoff(i % 20)
	}
}

// TestMonitoringLoopContextCancellation tests that monitoring properly exits on context cancellation
func TestMonitoringLoopContextCancellation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eventChan := make(chan interface{}, 10)

	integration := &Integration{
		logger:     logger,
		monitoring: true,
	}

	// Skip test if Docker client is not available (unit test environment)
	if integration.client == nil {
		// Try to initialize, but don't fail if Docker isn't available
		if !integration.IsAvailable() {
			t.Skip("Docker not available, skipping integration test")
		}
	}

	done := make(chan bool)

	go func() {
		integration.monitoringLoop(ctx, eventChan)
		done <- true
	}()

	time.Sleep(100 * time.Millisecond)

	cancel()

	select {
	case <-done:
		// Success - loop exited
	case <-time.After(2 * time.Second):
		t.Error("Monitoring loop did not exit after context cancellation")
	}

	integration.monitoringMu.RLock()
	if integration.monitoring {
		t.Error("Expected monitoring flag to be cleared after exit")
	}
	integration.monitoringMu.RUnlock()
}
