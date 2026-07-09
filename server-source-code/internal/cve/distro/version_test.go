package distro

import "testing"

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

func TestCompareDpkg(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0", "1.0", 0},
		{"1.0", "1.1", -1},
		{"6.8.0-51.52", "6.8.0-51.53", -1},
		{"6.8.0-52.53", "6.8.0-51.99", 1},
		{"6.1.55-1", "6.1.90-1", -1},
		{"1.0~rc1", "1.0", -1}, // tilde sorts before release
		{"1.0~rc1", "1.0~rc2", -1},
		{"1:1.0", "2.0", 1}, // epoch wins
		{"5.15.0-89.99", "5.15.0-89.99", 0},
	}
	for _, c := range cases {
		if got := sign(CompareDpkg(c.a, c.b)); got != c.want {
			t.Errorf("CompareDpkg(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
		if got := sign(CompareDpkg(c.b, c.a)); got != -c.want {
			t.Errorf("CompareDpkg(%q,%q) not antisymmetric: %d", c.b, c.a, got)
		}
	}
}

func TestCompareRPM(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"5.14.0-427.el9", "5.14.0-427.el9", 0},
		{"5.14.0-427.el9", "5.14.0-427.13.1.el9_4", -1}, // 427 == 427 then digit "13" > alpha "el"
		{"5.14.0-284.el9", "5.14.0-427.el9", -1},
		{"1.0", "1.0", 0},
		{"2~beta", "2", -1}, // tilde pre-release
		{"1.0", "1.0.1", -1},
		{"4.18.0-513.5.1.el8_9", "4.18.0-513.el8", 1},
		{"1.2.3-1.el9", "1.2.3-1.el9", 0},
	}
	for _, c := range cases {
		if got := sign(CompareRPM(c.a, c.b)); got != c.want {
			t.Errorf("CompareRPM(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
		if got := sign(CompareRPM(c.b, c.a)); got != -c.want {
			t.Errorf("CompareRPM(%q,%q) not antisymmetric: %d", c.b, c.a, got)
		}
	}
}
