package tween_test

import (
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/vibrantgio/effects/tween"
)

// settlingFrameFloat returns the smallest frame index at which the tween's
// value is within tolerance of the target. The simulation runs
// deterministically, so the frame at which the value enters the target
// tolerance band is the measured settling time.
//
// max bounds the search; if the tween hasn't settled by frame max, we
// return max+1 so the assertion fails with a clear "did not settle" signal
// rather than looping forever.
func settlingFrameFloat(tw tween.Tween[float64], tolerance float64, max int) int {
	for n := 0; n <= max; n++ {
		if math.Abs(tw.At(n)-tw.To) <= tolerance {
			return n
		}
	}
	return max + 1
}

func settlingFrameNRGBA(tw tween.Tween[color.NRGBA], max int) int {
	for n := 0; n <= max; n++ {
		if tw.At(n) == tw.To {
			return n
		}
	}
	return max + 1
}

// TestSettlingFrameFloat asserts that a linear float64 tween settles
// exactly at frame Frames — not earlier (which would mean an easing curve
// finished early or t-clamping fired prematurely), not later (an off-by-one
// in the lerp boundary). The tolerance is 1e-9: tight enough that only the
// exact-To boundary frame qualifies as "settled".
func TestSettlingFrameFloat(t *testing.T) {
	for _, frames := range []int{1, 10, 30, 60, 120} {
		tw := tween.Tween[float64]{From: 0, To: 1, Frames: frames, Lerp: tween.LerpFloat64}
		got := settlingFrameFloat(tw, 1e-9, frames+5)
		if got != frames {
			t.Errorf("settling frame for Frames=%d: got %d, want %d", frames, got, frames)
		}
	}
}

// TestSettlingFrameNRGBA asserts a colour tween settles exactly at frame
// Frames, byte-exact. Colour values are integers, so the natural tolerance
// is zero: any partial channel mismatch counts as not-yet-settled.
func TestSettlingFrameNRGBA(t *testing.T) {
	from := color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	to := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	for _, frames := range []int{1, 15, 30, 60} {
		tw := tween.Tween[color.NRGBA]{From: from, To: to, Frames: frames, Lerp: tween.LerpNRGBA}
		got := settlingFrameNRGBA(tw, frames+5)
		if got != frames {
			t.Errorf("colour tween settling frame for Frames=%d: got %d, want %d", frames, got, frames)
		}
	}
}

// TestSettledStaysSettledFloat asserts that once a tween reaches the
// target frame, all subsequent frames remain at the target. Catches a
// regression where post-settle frames drift due to a missing high-end
// clamp.
func TestSettledStaysSettledFloat(t *testing.T) {
	tw := tween.Tween[float64]{From: 0, To: 1, Frames: 30, Lerp: tween.LerpFloat64}
	for n := 30; n <= 60; n++ {
		if got := tw.At(n); got != 1 {
			t.Errorf("post-settle drift at frame %d: got %v, want 1", n, got)
		}
	}
}

// TestMonotonicityFloat asserts |At(n+1) - To| <= |At(n) - To| across
// the tween's frames. A linear tween must approach the target
// monotonically; a non-monotonic interpolation would indicate overshoot
// (spring-like behaviour creeping into a tween) or a sign-flip bug.
func TestMonotonicityFloat(t *testing.T) {
	tw := tween.Tween[float64]{From: 0, To: 100, Frames: 60, Lerp: tween.LerpFloat64}
	for n := 0; n < 65; n++ {
		curr := math.Abs(tw.At(n) - tw.To)
		next := math.Abs(tw.At(n+1) - tw.To)
		if next > curr {
			t.Errorf("monotonicity broken at frame %d: dist %v -> %v", n, curr, next)
		}
	}
}

// TestMonotonicityNRGBA asserts each channel approaches the target
// monotonically. Per-channel byte rounding could in principle introduce
// jitter; this guards against any such regression.
func TestMonotonicityNRGBA(t *testing.T) {
	from := color.NRGBA{R: 10, G: 20, B: 30, A: 255}
	to := color.NRGBA{R: 200, G: 100, B: 250, A: 255}
	tw := tween.Tween[color.NRGBA]{From: from, To: to, Frames: 30, Lerp: tween.LerpNRGBA}
	for n := 0; n < 35; n++ {
		c, c1 := tw.At(n), tw.At(n+1)
		check := func(name string, ch, ch1, target uint8) {
			d, d1 := absDiff(ch, target), absDiff(ch1, target)
			if d1 > d {
				t.Errorf("monotonicity broken on %s at frame %d: %d -> %d (target %d)", name, n, ch, ch1, target)
			}
		}
		check("R", c.R, c1.R, to.R)
		check("G", c.G, c1.G, to.G)
		check("B", c.B, c1.B, to.B)
		check("A", c.A, c1.A, to.A)
	}
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func TestTweenAtZeroReturnsFrom(t *testing.T) {
	tw := tween.Tween[int]{From: 100, To: 200, Frames: 10, Lerp: lerpInt}
	if got := tw.At(0); got != 100 {
		t.Errorf("At(0) = %d, want 100", got)
	}
}

func TestTweenAtFramesReturnsTo(t *testing.T) {
	tw := tween.Tween[int]{From: 100, To: 200, Frames: 10, Lerp: lerpInt}
	if got := tw.At(10); got != 200 {
		t.Errorf("At(Frames) = %d, want 200", got)
	}
}

func TestTweenAtBeyondFramesReturnsTo(t *testing.T) {
	tw := tween.Tween[int]{From: 100, To: 200, Frames: 10, Lerp: lerpInt}
	if got := tw.At(99); got != 200 {
		t.Errorf("At(>Frames) = %d, want 200", got)
	}
}

func TestTweenAtNegativeReturnsFrom(t *testing.T) {
	tw := tween.Tween[int]{From: 100, To: 200, Frames: 10, Lerp: lerpInt}
	if got := tw.At(-1); got != 100 {
		t.Errorf("At(-1) = %d, want 100", got)
	}
}

func TestTweenAtMidpoint(t *testing.T) {
	tw := tween.Tween[int]{From: 0, To: 100, Frames: 10, Lerp: lerpInt}
	if got := tw.At(5); got != 50 {
		t.Errorf("At(5) = %d, want 50", got)
	}
}

// TestTweenZeroFramesIsImmediate guards the divide-by-zero boundary:
// callers should not normally construct a zero-frame tween, but the
// degenerate case must not panic.
func TestTweenZeroFramesIsImmediate(t *testing.T) {
	tw := tween.Tween[int]{From: 100, To: 200, Frames: 0, Lerp: lerpInt}
	if got := tw.At(5); got != 100 {
		t.Errorf("At(5) on zero-frame tween = %d, want From=100", got)
	}
}

// TestTweenNilLerpEndpointsSurvive pins the endpoint contract: At never
// touches Lerp for n <= 0 or n >= Frames, so a Lerp-less tween returns
// the exact endpoints there — which is why endpoint-only tests cannot
// detect a missing Lerp.
func TestTweenNilLerpEndpointsSurvive(t *testing.T) {
	tw := tween.Tween[int]{From: 100, To: 200, Frames: 10}
	for _, tc := range []struct {
		n    int
		want int
	}{
		{-1, 100}, {0, 100}, {10, 200}, {99, 200},
	} {
		if got := tw.At(tc.n); got != tc.want {
			t.Errorf("nil-Lerp At(%d) = %d, want %d", tc.n, got, tc.want)
		}
	}
}

// TestTweenNilLerpInteriorPanics asserts the contract for a missing
// Lerp: the first interior frame panics with a message naming the field,
// instead of an anonymous nil function call.
func TestTweenNilLerpInteriorPanics(t *testing.T) {
	for _, n := range []int{1, 5, 9} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Errorf("nil-Lerp At(%d) did not panic", n)
					return
				}
				msg, ok := r.(string)
				if !ok || !strings.Contains(msg, "Lerp") {
					t.Errorf("nil-Lerp At(%d) panic = %v, want a message naming Lerp", n, r)
				}
			}()
			tw := tween.Tween[int]{From: 100, To: 200, Frames: 10}
			tw.At(n)
		}()
	}
}

func TestLerpFloat64Endpoints(t *testing.T) {
	if got := tween.LerpFloat64(10, 20, 0); got != 10 {
		t.Errorf("LerpFloat64(_, _, 0) = %v, want 10", got)
	}
	if got := tween.LerpFloat64(10, 20, 1); got != 20 {
		t.Errorf("LerpFloat64(_, _, 1) = %v, want 20", got)
	}
}

func TestLerpFloat64Clamp(t *testing.T) {
	if got := tween.LerpFloat64(10, 20, -1); got != 10 {
		t.Errorf("LerpFloat64(_, _, -1) = %v, want 10 (clamped)", got)
	}
	if got := tween.LerpFloat64(10, 20, 2); got != 20 {
		t.Errorf("LerpFloat64(_, _, 2) = %v, want 20 (clamped)", got)
	}
}

func TestLerpFloat64Midpoint(t *testing.T) {
	if got := tween.LerpFloat64(0, 100, 0.5); got != 50 {
		t.Errorf("LerpFloat64 midpoint = %v, want 50", got)
	}
}

func TestLerpNRGBAEndpoints(t *testing.T) {
	from := color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	to := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	if got := tween.LerpNRGBA(from, to, 0); got != from {
		t.Errorf("LerpNRGBA(_, _, 0) = %+v, want %+v", got, from)
	}
	if got := tween.LerpNRGBA(from, to, 1); got != to {
		t.Errorf("LerpNRGBA(_, _, 1) = %+v, want %+v", got, to)
	}
}

func TestLerpNRGBAClamp(t *testing.T) {
	from := color.NRGBA{R: 10, G: 20, B: 30, A: 40}
	to := color.NRGBA{R: 100, G: 110, B: 120, A: 130}
	if got := tween.LerpNRGBA(from, to, -1); got != from {
		t.Errorf("LerpNRGBA(_, _, -1) = %+v, want %+v (clamped)", got, from)
	}
	if got := tween.LerpNRGBA(from, to, 2); got != to {
		t.Errorf("LerpNRGBA(_, _, 2) = %+v, want %+v (clamped)", got, to)
	}
}

func TestLerpNRGBAMidpoint(t *testing.T) {
	from := color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	to := color.NRGBA{R: 100, G: 200, B: 50, A: 255}
	got := tween.LerpNRGBA(from, to, 0.5)
	want := color.NRGBA{R: 50, G: 100, B: 25, A: 255}
	if got != want {
		t.Errorf("LerpNRGBA midpoint = %+v, want %+v", got, want)
	}
}

func TestLerpNRGBAReverseDirection(t *testing.T) {
	from := color.NRGBA{R: 200, G: 200, B: 200, A: 255}
	to := color.NRGBA{R: 100, G: 100, B: 100, A: 255}
	got := tween.LerpNRGBA(from, to, 0.5)
	want := color.NRGBA{R: 150, G: 150, B: 150, A: 255}
	if got != want {
		t.Errorf("LerpNRGBA reverse midpoint = %+v, want %+v", got, want)
	}
}

func lerpInt(from, to int, t float64) int {
	return from + int(float64(to-from)*t+0.5)
}
