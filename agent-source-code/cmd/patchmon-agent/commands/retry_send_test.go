package commands

import (
	"errors"
	"testing"
)

func TestRetrySend(t *testing.T) {
	calls := 0
	err := retrySend(5, 0, func() error {
		calls++
		if calls < 3 {
			return errors.New("connection reset")
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("expected success on the third try, got err=%v calls=%d", err, calls)
	}

	calls = 0
	err = retrySend(4, 0, func() error {
		calls++
		return errors.New("down")
	})
	if err == nil || calls != 4 {
		t.Fatalf("expected the last error after 4 tries, got err=%v calls=%d", err, calls)
	}
}
