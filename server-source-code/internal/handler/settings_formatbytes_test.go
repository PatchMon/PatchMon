package handler

import "testing"

func TestFormatBytesEnv_ExactInLargestEvenUnit(t *testing.T) {
	t.Parallel()
	cases := map[int64]string{
		0:                 "0b",
		512:               "512b",
		1000:              "1000b",
		1025:              "1025b",
		64 << 10:          "64kb",
		1500 << 10:        "1500kb",
		1536 << 10:        "1536kb",
		5 << 20:           "5mb",
		32 << 20:          "32mb",
		(5 << 20) + 1:     "5242881b",
		2 << 30:           "2gb",
		(1 << 30) + 1<<20: "1025mb",
	}
	for n, want := range cases {
		if got := formatBytesEnv(n); got != want {
			t.Errorf("formatBytesEnv(%d) = %q, want %q", n, got, want)
		}
	}
}
