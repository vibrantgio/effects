// Package tween provides a frame-indexed interpolator for non-physical
// motion: fades, slides, and colour interpolations.
//
// Tween is intentionally minimal: a value type with no internal state, no
// easing, and no clock. Callers drive it by passing a frame index to
// [Tween.At]. The frame index is whatever the caller chooses to use as
// "time" — render-loop frames, real-time samples, or test fixtures.
//
// Pulse uses Tween for motion that is cheap, predictable, and where
// physical realism is not the goal. Reach for pulse/spring when the
// motion needs to feel physical.
package tween

import "image/color"

// Tween is a frame-indexed interpolator. Construct one with From, To,
// Frames, and a Lerp function; call [Tween.At] to read the value at a
// given frame.
//
// Frame semantics: At(0) returns From, At(n >= Frames) returns To, and
// frames in between are interpolated by Lerp at parameter n/Frames.
type Tween[T any] struct {
	From, To T
	Frames   int
	Lerp     func(from, to T, t float64) T
}

// At returns the tweened value at frame n. n is clamped to [0, Frames]:
// n <= 0 returns From; n >= Frames returns To.
func (tw Tween[T]) At(n int) T {
	if n <= 0 || tw.Frames <= 0 {
		return tw.From
	}
	if n >= tw.Frames {
		return tw.To
	}
	return tw.Lerp(tw.From, tw.To, float64(n)/float64(tw.Frames))
}

// LerpFloat64 linearly interpolates between two float64 values at
// parameter t. t is clamped to [0, 1]. Use for fades (opacity), 1D
// slides (position), and other scalar motion.
func LerpFloat64(from, to, t float64) float64 {
	if t <= 0 {
		return from
	}
	if t >= 1 {
		return to
	}
	return from + (to-from)*t
}

// LerpNRGBA linearly interpolates each channel of two NRGBA colours at
// parameter t. t is clamped to [0, 1]. Inputs are treated as straight
// alpha (no premultiplication, no gamma correction); perceptual blending
// is a separate concern handled higher up the stack.
func LerpNRGBA(from, to color.NRGBA, t float64) color.NRGBA {
	if t <= 0 {
		return from
	}
	if t >= 1 {
		return to
	}
	return color.NRGBA{
		R: lerpByte(from.R, to.R, t),
		G: lerpByte(from.G, to.G, t),
		B: lerpByte(from.B, to.B, t),
		A: lerpByte(from.A, to.A, t),
	}
}

// lerpByte interpolates a single byte channel and rounds to nearest.
// The result is in [min(from,to), max(from,to)] for t in [0,1], so the
// "+0.5 then truncate" round-half-up trick is safe.
func lerpByte(from, to uint8, t float64) uint8 {
	return uint8(float64(from) + (float64(to)-float64(from))*t + 0.5)
}
