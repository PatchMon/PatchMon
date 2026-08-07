package store

import "testing"

// The update-status chart navigates by the filter its segment carries. A
// segment without one is not a cosmetic problem: the click falls through and
// lands on an unfiltered host list, which reads as having worked. These tests
// pin that contract so a fourth segment cannot be added without a filter, and
// so the three filters stay the ones the Hosts page actually understands.
func TestUpdateStatusSegments_EverySegmentCarriesAFilter(t *testing.T) {
	t.Parallel()

	for _, seg := range updateStatusSegments(1, 2, 3) {
		name, ok := seg["name"].(string)
		if !ok || name == "" {
			t.Fatalf("segment %v has no display name", seg)
		}
		filter, ok := seg["filter"].(string)
		if !ok || filter == "" {
			t.Fatalf("segment %q carries no Hosts-page filter", name)
		}
	}
}

func TestUpdateStatusSegments_FiltersMatchTheHostsPage(t *testing.T) {
	t.Parallel()

	// "inactive" is the filter the not-reporting card already links to, and the
	// one whose server-side effective_status predicate the chart's own count is
	// computed from. The two must stay in step or the count and the list it
	// links to disagree.
	want := []struct{ name, filter string }{
		{"Up to date", "upToDate"},
		{"Needs updates", "needsUpdates"},
		{"Not reporting", "inactive"},
	}

	got := updateStatusSegments(0, 0, 0)
	if len(got) != len(want) {
		t.Fatalf("expected %d segments, got %d", len(want), len(got))
	}
	for i, w := range want {
		if got[i]["name"] != w.name {
			t.Errorf("segment %d: expected name %q, got %v", i, w.name, got[i]["name"])
		}
		if got[i]["filter"] != w.filter {
			t.Errorf("segment %d (%s): expected filter %q, got %v", i, w.name, w.filter, got[i]["filter"])
		}
	}
}

func TestUpdateStatusSegments_CountsLandOnTheRightSegment(t *testing.T) {
	t.Parallel()

	got := updateStatusSegments(7, 11, 13)
	for i, want := range []int{7, 11, 13} {
		if got[i]["count"] != want {
			t.Errorf("segment %d (%v): expected count %d, got %v", i, got[i]["name"], want, got[i]["count"])
		}
	}
}
