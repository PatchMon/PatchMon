package pgtime

import (
	"testing"
	"time"
)

// TestFromNormalizesToUTC guards the core invariant of this package: any
// time.Time handed to pgtype.Timestamp must be re-labelled in UTC location
// so pgx's discardTimeZone does not silently drop the offset.
func TestFromNormalizesToUTC(t *testing.T) {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	// A BST instant: 2026-06-15 12:00:00 +0100 == 2026-06-15 11:00:00 UTC.
	in := time.Date(2026, 6, 15, 12, 0, 0, 0, loc)
	got := From(in)

	if !got.Valid {
		t.Fatalf("expected Valid timestamp")
	}
	if got.Time.Location() != time.UTC {
		t.Errorf("location = %v, want UTC", got.Time.Location())
	}
	if h := got.Time.Hour(); h != 11 {
		t.Errorf("hour = %d, want 11 (BST 12:00 => UTC 11:00)", h)
	}
	if !got.Time.Equal(in) {
		t.Errorf("instant changed: got %v, want %v", got.Time, in)
	}
}

func TestFromPtrNilYieldsInvalid(t *testing.T) {
	got := FromPtr(nil)
	if got.Valid {
		t.Errorf("FromPtr(nil) should be invalid, got %+v", got)
	}
}

func TestFromPtrNonNilNormalizesToUTC(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	in := time.Date(2026, 1, 2, 3, 4, 5, 0, loc)
	got := FromPtr(&in)

	if !got.Valid {
		t.Fatalf("expected Valid timestamp")
	}
	if got.Time.Location() != time.UTC {
		t.Errorf("location = %v, want UTC", got.Time.Location())
	}
	if !got.Time.Equal(in) {
		t.Errorf("instant changed: got %v, want %v", got.Time, in)
	}
}

func TestNowIsUTC(t *testing.T) {
	got := Now()
	if !got.Valid {
		t.Fatalf("expected Valid timestamp")
	}
	if got.Time.Location() != time.UTC {
		t.Errorf("location = %v, want UTC", got.Time.Location())
	}
	if d := time.Since(got.Time); d < 0 || d > 5*time.Second {
		t.Errorf("Now drift out of range: %v", d)
	}
}
