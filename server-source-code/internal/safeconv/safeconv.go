package safeconv

import (
	"fmt"
	"math"
	"strconv"
)

// Atoi32 parses s as an int and returns int32 if within range.
// Returns an error if parsing fails or the value is outside [math.MinInt32, math.MaxInt32].
func Atoi32(s string) (int32, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if n < math.MinInt32 || n > math.MaxInt32 {
		return 0, fmt.Errorf("value %d out of int32 range", n)
	}
	return int32(n), nil
}

// ToInt32 converts n to int32 if within range.
// Returns an error if n is outside [math.MinInt32, math.MaxInt32].
func ToInt32(n int) (int32, error) {
	if n < math.MinInt32 || n > math.MaxInt32 {
		return 0, fmt.Errorf("value %d out of int32 range", n)
	}
	return int32(n), nil
}

// ClampToInt32 converts n to int32, clamping to [math.MinInt32, math.MaxInt32].
// Use when the value is validated or a clamp is acceptable (e.g. pagination limits).
func ClampToInt32(n int) int32 {
	if n < math.MinInt32 {
		return math.MinInt32
	}
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(n)
}
