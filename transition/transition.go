// Package transition interpolates a whole set of the platform's colours, so a
// light-to-dark flip can cross-fade instead of snapping.
//
// It is one bridge and nothing else. [github.com/vibrantgio/effects/tween]
// owns the generic Tween[T] machinery and the per-channel LerpNRGBA
// primitive; this package supplies the two pieces that teach it the
// theme's colour contract — [LerpPlatformColors], which lerps every field of
// a [tokens.PlatformColors] at a parameter in [0,1], and [PlatformColorsTween],
// which packages that as a Tween you sample with At.
//
// Three things constrain what it can do. The unit is frames, not time:
// PlatformColorsTween(a, b, 30) is settled at At(30) whether those thirty
// frames took half a second or five, so a duration-based fade needs the
// caller to convert. Interpolation is a straight per-channel average of
// 8-bit sRGB values, coverage included, with no perceptual or gamma
// correction, so a sweep between two saturated names can pass through a
// duller midpoint than either endpoint — acceptable for the near-neutral
// planes and labels, more visible on the accent. And the tween only produces
// values: nothing here drives it. Emitting the intermediate set as a theme,
// frame by frame, is the caller's job.
//
// The platform's coverages are interpolated as coverages and NOT flattened
// onto anything: a set is what a consumer reads names out of, and where each
// name lands is that consumer's business, so a half-way Label is black at
// half-way to the other appearance's coverage and a consumer flattens it as
// it always does.
package transition

import (
	"github.com/vibrantgio/effects/tween"
	"github.com/vibrantgio/theme/tokens"
)

// LerpPlatformColors interpolates every field of two [tokens.PlatformColors]
// using [tween.LerpNRGBA], so the result at t=0 and t=1 equals the endpoints
// exactly.
//
// Every field is named here rather than walked by reflection, and the package
// test walks the struct to prove none was missed: a name added to the set and
// forgotten here would cross-fade the window around one element that snaps.
func LerpPlatformColors(from, to tokens.PlatformColors, t float64) tokens.PlatformColors {
	return tokens.PlatformColors{
		WindowBackground:                      tween.LerpNRGBA(from.WindowBackground, to.WindowBackground, t),
		UnderPageBackground:                   tween.LerpNRGBA(from.UnderPageBackground, to.UnderPageBackground, t),
		ControlBackground:                     tween.LerpNRGBA(from.ControlBackground, to.ControlBackground, t),
		TextBackground:                        tween.LerpNRGBA(from.TextBackground, to.TextBackground, t),
		SelectedContentBackground:             tween.LerpNRGBA(from.SelectedContentBackground, to.SelectedContentBackground, t),
		UnemphasizedSelectedContentBackground: tween.LerpNRGBA(from.UnemphasizedSelectedContentBackground, to.UnemphasizedSelectedContentBackground, t),
		SelectedTextBackground:                tween.LerpNRGBA(from.SelectedTextBackground, to.SelectedTextBackground, t),
		UnemphasizedSelectedTextBackground:    tween.LerpNRGBA(from.UnemphasizedSelectedTextBackground, to.UnemphasizedSelectedTextBackground, t),
		FindHighlight:                         tween.LerpNRGBA(from.FindHighlight, to.FindHighlight, t),
		Separator:                             tween.LerpNRGBA(from.Separator, to.Separator, t),
		Grid:                                  tween.LerpNRGBA(from.Grid, to.Grid, t),
		Label:                                 tween.LerpNRGBA(from.Label, to.Label, t),
		SecondaryLabel:                        tween.LerpNRGBA(from.SecondaryLabel, to.SecondaryLabel, t),
		TertiaryLabel:                         tween.LerpNRGBA(from.TertiaryLabel, to.TertiaryLabel, t),
		QuaternaryLabel:                       tween.LerpNRGBA(from.QuaternaryLabel, to.QuaternaryLabel, t),
		Text:                                  tween.LerpNRGBA(from.Text, to.Text, t),
		PlaceholderText:                       tween.LerpNRGBA(from.PlaceholderText, to.PlaceholderText, t),
		SelectedText:                          tween.LerpNRGBA(from.SelectedText, to.SelectedText, t),
		Link:                                  tween.LerpNRGBA(from.Link, to.Link, t),
		HeaderText:                            tween.LerpNRGBA(from.HeaderText, to.HeaderText, t),
		Control:                               tween.LerpNRGBA(from.Control, to.Control, t),
		ControlText:                           tween.LerpNRGBA(from.ControlText, to.ControlText, t),
		DisabledControlText:                   tween.LerpNRGBA(from.DisabledControlText, to.DisabledControlText, t),
		SelectedControl:                       tween.LerpNRGBA(from.SelectedControl, to.SelectedControl, t),
		SelectedControlText:                   tween.LerpNRGBA(from.SelectedControlText, to.SelectedControlText, t),
		AlternateSelectedControlText:          tween.LerpNRGBA(from.AlternateSelectedControlText, to.AlternateSelectedControlText, t),
		ControlAccent:                         tween.LerpNRGBA(from.ControlAccent, to.ControlAccent, t),
		KeyboardFocusIndicator:                tween.LerpNRGBA(from.KeyboardFocusIndicator, to.KeyboardFocusIndicator, t),
		SystemRed:                             tween.LerpNRGBA(from.SystemRed, to.SystemRed, t),
		SystemOrange:                          tween.LerpNRGBA(from.SystemOrange, to.SystemOrange, t),
		SystemYellow:                          tween.LerpNRGBA(from.SystemYellow, to.SystemYellow, t),
		SystemGreen:                           tween.LerpNRGBA(from.SystemGreen, to.SystemGreen, t),
		SystemMint:                            tween.LerpNRGBA(from.SystemMint, to.SystemMint, t),
		SystemTeal:                            tween.LerpNRGBA(from.SystemTeal, to.SystemTeal, t),
		SystemCyan:                            tween.LerpNRGBA(from.SystemCyan, to.SystemCyan, t),
		SystemBlue:                            tween.LerpNRGBA(from.SystemBlue, to.SystemBlue, t),
		SystemIndigo:                          tween.LerpNRGBA(from.SystemIndigo, to.SystemIndigo, t),
		SystemPurple:                          tween.LerpNRGBA(from.SystemPurple, to.SystemPurple, t),
		SystemPink:                            tween.LerpNRGBA(from.SystemPink, to.SystemPink, t),
		SystemBrown:                           tween.LerpNRGBA(from.SystemBrown, to.SystemBrown, t),
		SystemGray:                            tween.LerpNRGBA(from.SystemGray, to.SystemGray, t),
		Shadow:                                tween.LerpNRGBA(from.Shadow, to.Shadow, t),
		Highlight:                             tween.LerpNRGBA(from.Highlight, to.Highlight, t),
		SidebarMaterial:                       tween.LerpNRGBA(from.SidebarMaterial, to.SidebarMaterial, t),
		CardFill:                              tween.LerpNRGBA(from.CardFill, to.CardFill, t),
		PushButtonFill:                        tween.LerpNRGBA(from.PushButtonFill, to.PushButtonFill, t),
		HoverOverlay:                          tween.LerpNRGBA(from.HoverOverlay, to.HoverOverlay, t),
		PressOverlay:                          tween.LerpNRGBA(from.PressOverlay, to.PressOverlay, t),
		FloatingShadow:                        tween.LerpNRGBA(from.FloatingShadow, to.FloatingShadow, t),
		FieldEdge:                             tween.LerpNRGBA(from.FieldEdge, to.FieldEdge, t),
		ScrollbarThumb:                        tween.LerpNRGBA(from.ScrollbarThumb, to.ScrollbarThumb, t),
		AlternatingContentBackground:          tween.LerpNRGBA(from.AlternatingContentBackground, to.AlternatingContentBackground, t),
		Scrim:                                 tween.LerpNRGBA(from.Scrim, to.Scrim, t),
	}
}

// PlatformColorsTween constructs a [tween.Tween] interpolating from a to b
// over frames frames, using [LerpPlatformColors].
func PlatformColorsTween(a, b tokens.PlatformColors, frames int) tween.Tween[tokens.PlatformColors] {
	return tween.Tween[tokens.PlatformColors]{
		From:   a,
		To:     b,
		Frames: frames,
		Lerp:   LerpPlatformColors,
	}
}
