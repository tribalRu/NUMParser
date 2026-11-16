package utils

import "testing"

func TestI2BAndB2I(t *testing.T) {
	nums := []int64{0, 1, 100, 123456789, -1, -999999}
	for _, n := range nums {
		b := I2B(n)
		if len(b) != 8 {
			t.Fatalf("expected 8 bytes, got %d", len(b))
		}
		got := B2I(b)
		if got != n {
			t.Errorf("expected %d, got %d", n, got)
		}
	}

	// Test short slice safety
	if B2I(nil) != 0 {
		t.Errorf("expected 0 for nil slice")
	}
	if B2I([]byte{1, 2, 3}) != 0 {
		t.Errorf("expected 0 for short slice")
	}
}
