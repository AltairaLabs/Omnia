// Package intconv provides bounds-checked narrowing conversions between
// integer types. CodeQL's go/incorrect-integer-conversion rule flags
// bare int/int64 → int32 casts because a hostile caller can wrap the
// value around zero on 32-bit platforms, or overflow a signed field.
// Clamping instead of wrapping is almost always the safer choice.
package intconv

import "math"

// ClampInt32 converts x to int32, saturating at math.MaxInt32 and
// math.MinInt32 instead of wrapping. Use when the destination is an
// int32 field (e.g. a Kubernetes API field) and the value came from
// an unbounded source like strconv.Atoi or a uint64 counter.
func ClampInt32(x int64) int32 {
	if x > math.MaxInt32 {
		return math.MaxInt32
	}
	if x < math.MinInt32 {
		return math.MinInt32
	}
	return int32(x)
}

// ClampInt converts x to int, saturating at math.MaxInt / math.MinInt
// on 32-bit platforms. On 64-bit it's a no-op (int is already 64 bits).
// Useful for consuming int64 API totals from typed counters where the
// destination is a plain int.
func ClampInt(x int64) int {
	if x > math.MaxInt {
		return math.MaxInt
	}
	if x < math.MinInt {
		return math.MinInt
	}
	return int(x)
}
