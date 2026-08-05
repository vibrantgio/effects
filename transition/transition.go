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
// not depend on the effects layer. A deprecated alias at the old path
// keeps existing imports working for one release cycle.
package transition

import (
	"github.com/vibrantgio/pulse/tween"
	"github.com/vibrantgio/spectrum/tokens"
)

// LerpColorTokens interpolates each colour field of two
// [tokens.ColorTokens] using [tween.LerpNRGBA].
func LerpColorTokens(from, to tokens.ColorTokens, t float64) tokens.ColorTokens {
	return tokens.ColorTokens{
		Background:       tween.LerpNRGBA(from.Background, to.Background, t),
		OnBackground:     tween.LerpNRGBA(from.OnBackground, to.OnBackground, t),
		Surface:          tween.LerpNRGBA(from.Surface, to.Surface, t),
		OnSurface:        tween.LerpNRGBA(from.OnSurface, to.OnSurface, t),
		SurfaceVariant:   tween.LerpNRGBA(from.SurfaceVariant, to.SurfaceVariant, t),
		OnSurfaceVariant: tween.LerpNRGBA(from.OnSurfaceVariant, to.OnSurfaceVariant, t),
		Primary:          tween.LerpNRGBA(from.Primary, to.Primary, t),
		OnPrimary:        tween.LerpNRGBA(from.OnPrimary, to.OnPrimary, t),
		Secondary:        tween.LerpNRGBA(from.Secondary, to.Secondary, t),
		OnSecondary:      tween.LerpNRGBA(from.OnSecondary, to.OnSecondary, t),
		Error:            tween.LerpNRGBA(from.Error, to.Error, t),
		OnError:          tween.LerpNRGBA(from.OnError, to.OnError, t),
		Outline:          tween.LerpNRGBA(from.Outline, to.Outline, t),
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
