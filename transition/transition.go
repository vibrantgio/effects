// Package transition interpolates a whole set of colour tokens, so a
// light-to-dark flip can cross-fade instead of snapping.
//
// It is one bridge and nothing else. [github.com/vibrantgio/pulse/tween]
// owns the generic Tween[T] machinery and the per-channel LerpNRGBA
// primitive; this package supplies the two pieces that teach it the
// spectrum theme contract — [LerpColorTokens], which lerps every field of
// a [tokens.ColorTokens] at a parameter in [0,1], and [ColorTokensTween],
// which packages that as a Tween you sample with At.
//
// Three things constrain what it can do. The unit is frames, not time:
// ColorTokensTween(a, b, 30) is settled at At(30) whether those thirty
// frames took half a second or five, so a duration-based fade needs the
// caller to convert. Interpolation is a straight per-channel average of
// 8-bit sRGB values with no perceptual or gamma correction, so a sweep
// between two saturated tokens can pass through a duller midpoint than
// either endpoint — acceptable for the near-greyscale background and
// surface roles, more visible on Primary. And the tween only produces
// values: nothing here drives it. Emitting the intermediate ColorTokens as
// a theme, frame by frame, is the caller's job, which is why an
// OS-driven appearance change through spectrum/system still snaps today.
//
// This package lived at github.com/vibrantgio/spectrum/transition; it
// moved here because it is animation code — a foundation module should
// not depend on the effects layer. The deprecated alias left behind at
// the old path is gone as of spectrum v0.2.0; this is the only path.
package transition

import (
	"github.com/vibrantgio/pulse/tween"
	"github.com/vibrantgio/spectrum/tokens"
)

// lerpRamp interpolates each step of two [tokens.Ramp] values using
// [tween.LerpNRGBA].
func lerpRamp(from, to tokens.Ramp, t float64) tokens.Ramp {
	var r tokens.Ramp
	for i := range r {
		r[i] = tween.LerpNRGBA(from[i], to[i], t)
	}
	return r
}

// LerpColorTokens interpolates each colour field of two
// [tokens.ColorTokens] using [tween.LerpNRGBA] — every ramp and every
// pinned base, so the result at t=0 and t=1 equals the endpoints exactly.
// The five MD3-named alias fields it also carried are gone with spectrum
// v0.2.0.
func LerpColorTokens(from, to tokens.ColorTokens, t float64) tokens.ColorTokens {
	return tokens.ColorTokens{
		Ramps: tokens.RampSet{
			Neutral:   lerpRamp(from.Ramps.Neutral, to.Ramps.Neutral, t),
			Primary:   lerpRamp(from.Ramps.Primary, to.Ramps.Primary, t),
			Secondary: lerpRamp(from.Ramps.Secondary, to.Ramps.Secondary, t),
			Tertiary:  lerpRamp(from.Ramps.Tertiary, to.Ramps.Tertiary, t),
			Error:     lerpRamp(from.Ramps.Error, to.Ramps.Error, t),
		},
		Tertiary:    tween.LerpNRGBA(from.Tertiary, to.Tertiary, t),
		OnTertiary:  tween.LerpNRGBA(from.OnTertiary, to.OnTertiary, t),
		Text:        tween.LerpNRGBA(from.Text, to.Text, t),
		Divider:     tween.LerpNRGBA(from.Divider, to.Divider, t),
		Background:  tween.LerpNRGBA(from.Background, to.Background, t),
		Surface:     tween.LerpNRGBA(from.Surface, to.Surface, t),
		Primary:     tween.LerpNRGBA(from.Primary, to.Primary, t),
		OnPrimary:   tween.LerpNRGBA(from.OnPrimary, to.OnPrimary, t),
		Secondary:   tween.LerpNRGBA(from.Secondary, to.Secondary, t),
		OnSecondary: tween.LerpNRGBA(from.OnSecondary, to.OnSecondary, t),
		Error:       tween.LerpNRGBA(from.Error, to.Error, t),
		OnError:     tween.LerpNRGBA(from.OnError, to.OnError, t),
	}
}

// ColorTokensTween constructs a [tween.Tween] interpolating from a to b
// over frames frames, using [LerpColorTokens]. This is the integration
// bridge between the generic Tween and the spectrum theme contract.
func ColorTokensTween(a, b tokens.ColorTokens, frames int) tween.Tween[tokens.ColorTokens] {
	return tween.Tween[tokens.ColorTokens]{
		From:   a,
		To:     b,
		Frames: frames,
		Lerp:   LerpColorTokens,
	}
}
