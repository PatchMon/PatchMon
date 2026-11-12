package store

import (
	"reflect"
	"testing"
)

// Bulk "Assign to Group" must only touch the groups the user changed.
// A host's other memberships have to survive (issue #1245).
func TestApplyGroupChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		current, add, remove []string
		want                 []string
	}{
		{"add keeps existing", []string{"prod"}, []string{"linux"}, nil, []string{"prod", "linux"}},
		{"add already present", []string{"prod", "linux"}, []string{"linux"}, nil, []string{"prod", "linux"}},
		{"remove one", []string{"prod", "web"}, nil, []string{"web"}, []string{"prod"}},
		{"remove not held", []string{"prod"}, nil, []string{"web"}, []string{"prod"}},
		{"add and remove", []string{"prod", "web"}, []string{"linux"}, []string{"prod"}, []string{"web", "linux"}},
		{"in both lists is removed", []string{"prod"}, []string{"web"}, []string{"web"}, []string{"prod"}},
		{"no groups yet", nil, []string{"linux", "linux"}, nil, []string{"linux"}},
		{"nothing changes", []string{"prod"}, nil, nil, []string{"prod"}},
		{"remove all", []string{"prod"}, nil, []string{"prod"}, []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := applyGroupChanges(tc.current, tc.add, tc.remove)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("applyGroupChanges(%v, %v, %v) = %v, want %v", tc.current, tc.add, tc.remove, got, tc.want)
			}
		})
	}
}
