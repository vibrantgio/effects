package blur

import "testing"

// TestInvWindowExact verifies the multiply-shift division against
// plain integer division for every sum a box pass can produce
// (n <= 255*window plus the rounding half), across all small odd
// windows and the capped maximum.
func TestInvWindowExact(t *testing.T) {
	windows := []uint32{1, 3, 5, 7, 9, 15, 17, 31, 33, 101, 1001, 2*maxRadius + 1}
	for _, w := range windows {
		inv := invWindow(w)
		half := uint64(w / 2)
		limit := uint64(255)*uint64(w) + half
		step := uint64(1)
		if limit > 1<<22 {
			step = 977 // sample large windows; primes avoid aliasing
		}
		for n := uint64(0); n <= limit; n += step {
			got := ((n + half) * inv) >> invShift
			want := (n + half) / uint64(w)
			if got != want {
				t.Fatalf("window %d: (%d+half)*inv>>%d = %d, want %d", w, n, invShift, got, want)
			}
		}
		// the extremes always exactly
		for _, n := range []uint64{limit - half, limit} {
			got := ((n) * inv) >> invShift
			if want := n / uint64(w); got != want {
				t.Fatalf("window %d: %d*inv>>%d = %d, want %d", w, n, invShift, got, want)
			}
		}
	}
}
