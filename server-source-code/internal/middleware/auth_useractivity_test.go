package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestUserActivity_OnlyExactFlagCounts guards the predicate that decides whether
// a request slides the inactivity window. Anything looser would let the UI's
// background polling keep an unattended session alive, which is the bug this
// header was introduced to fix.
func TestUserActivity_OnlyExactFlagCounts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		header string
		set    bool
		want   bool
	}{
		{name: "absent", set: false, want: false},
		{name: "empty", header: "", set: true, want: false},
		{name: "one", header: "1", set: true, want: true},
		{name: "zero", header: "0", set: true, want: false},
		{name: "true", header: "true", set: true, want: false},
		{name: "padded", header: " 1", set: true, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
			if tc.set {
				req.Header.Set(UserActivityHeader, tc.header)
			}
			if got := userActivity(req); got != tc.want {
				t.Errorf("userActivity(%q set=%v) = %v, want %v", tc.header, tc.set, got, tc.want)
			}
		})
	}
}
