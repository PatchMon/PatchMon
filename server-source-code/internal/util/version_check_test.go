package util

import (
	"errors"
	"testing"
)

// With the switch on, no DNS query may be issued at all; the domain below is
// unresolvable, so reaching net.LookupTXT would surface a different error.
func TestLookupVersionFromDNS_Disabled(t *testing.T) {
	SetVersionCheckDisabled(true)
	t.Cleanup(func() { SetVersionCheckDisabled(false) })

	_, err := LookupVersionFromDNS("unresolvable.invalid")
	if !errors.Is(err, ErrVersionCheckDisabled) {
		t.Fatalf("err = %v, want ErrVersionCheckDisabled", err)
	}
}
