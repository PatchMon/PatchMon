package main

import "testing"

func TestRecommendedDBPoolSize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		hosts int
		want  int
	}{
		{0, 0},
		{1, 10},
		{50, 80},
		{51, 80},
		{66, 100},
		{67, 110},
		{95, 150},
		{100, 150},
		{101, 160},
		{200, 300},
		{355, 540},
		{1000, 1500},
	}
	for _, tc := range cases {
		if got := recommendedDBPoolSize(tc.hosts); got != tc.want {
			t.Errorf("recommendedDBPoolSize(%d) = %d; want %d", tc.hosts, got, tc.want)
		}
	}
}
