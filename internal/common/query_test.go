package common

import (
	"strings"
	"testing"
)

func TestApproxRowSize(t *testing.T) {
	// A small row should be smaller than a row with a large text value, and
	// both should be positive. We don't assert exact byte counts (they track
	// JSON encoding), only the ordering and positivity the byte budget relies on.
	small := approxRowSize([]*string{new("1"), new("a")})
	large := approxRowSize([]*string{new("1"), new(strings.Repeat("x", 1000))})

	if small <= 0 {
		t.Errorf("approxRowSize(small) = %d, want > 0", small)
	}
	if large <= small {
		t.Errorf("approxRowSize(large)=%d should exceed approxRowSize(small)=%d", large, small)
	}
}
