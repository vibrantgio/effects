// Package depth renders Material-style cast shadows under rectangular
// regions by composing linear gradients.
//
// # An explicit effect, never a default
//
// A shadow is opt-in vibrancy, not something a component gets for
// being raised. Per ADR-005, a raised surface on desktop reads as
// raised by tint first and shadow second: it names its rung on
// [tokens.ElevationScale] and paints the neutral-ramp colour that
// (tokens.ColorTokens).SurfaceAt resolves — one fill, no shadow. A
// shadow is right only for what floats and can leave: transient,
// dismissible surfaces above the plane — a toast, a popover, a menu, a
// drag preview. What is raised in place — a card, a header, static
// hierarchy — reads as raised by its surface step alone, and no
// component should default into calling this package for it.
//
// The cost difference backs the rule. One [Shadow] call issues eight
// [paint.LinearGradientOp] fills — four edge bands and four corner
// tiles — plus one flat interior fill: nine paint operations per
// shadow, every frame it is drawn. A surface step is a single
// [paint.FillShape].
//
// # Caller audit (E2.2)
//
// The organization's three callers, judged against that criterion:
//
//   - cadence/toast keeps its shadow: a toast floats over the content
//     plane and leaves on its own, and on dark themes the shadow, not
//     the fill, is what separates it.
//   - workbench/mindchat's undo bar keeps its shadow: a transient bar
//     floating over the chat surfaces, a toast by another name. (App
//     code is not governed by this verdict; recorded as guidance.)
//   - cadence/card's Elevated variant loses its shadow: a card is
//     raised in place, so it becomes a level-2 surface fill. E2.3
//     executes the migration.
//
// # Geometry
//
// A shadow is a soft black fringe around a "shadow rectangle" — the
// caller's bounds shifted downward by half the elevation extent — to
// approximate light from above. The geometry mirrors
// [github.com/vibrantgio/pulse/glow.Halo] but with two differences:
//
//   - The shadow rectangle's interior is filled at the peak alpha too,
//     so the strip extending below bounds stays visible once the caller
//     paints their foreground on top.
//   - Nothing about the colour is a parameter. It is a fixed key-shadow
//     black at a fixed peak alpha, and neither varies with the elevation
//     level — only the geometry does.
//
// Extent (the gradient's falloff distance) and offset (the downward
// shift of the shadow rectangle) both follow the level: extent is the
// level's dp value from [tokens.Elevation] converted to pixels through
// gtx.Metric, and offset is half of that. [tokens.Level0] — and any
// level whose dp rounds to zero pixels at the current density — paints
// nothing at all.
//
// # Three things a caller trips on
//
// The interior fill is a hard rectangle. [Shadow] paints before the
// foreground and the caller draws over it, so that fill is normally
// hidden — but a foreground with rounded corners does not hide it, and
// the fill's four square corners show through the rounding as dark
// wedges. Every consumer of this package in the organization paints a
// rounded rectangle (cadence/card, cadence/toast, workbench/mindchat),
// so that is the default outcome rather than an edge case.
//
// There is no opacity parameter. A shadow that has to fade with the
// surface it sits under needs the caller to wrap the call in a
// [paint.PushOpacity] layer, which is what cadence/toast does to keep
// the shadow from outliving a toast's fade.
//
// The black is not a token role, so it does not follow the theme. The
// same call separates a surface strongly on a light background and
// barely at all on a dark one; a dark theme that wants visible
// elevation needs something other than this package.
package depth

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/vibrantgio/spectrum/tokens"
)

// keyShadowAlpha is Material 3's recommended key-shadow opacity (~30 %).
// All elevation levels share this peak alpha; only the geometry varies
// with the level.
const keyShadowAlpha uint8 = 76

// Shadow paints a Material-style cast shadow under bounds onto gtx.Ops
// at the given elevation level. The shadow is biased downward to
// approximate light from above.
//
// At [tokens.Level0] (and any level whose dp value rounds to zero pixels
// at the current metric) the function is a no-op.
func Shadow(gtx layout.Context, bounds image.Rectangle, level tokens.ElevationLevel) {
	dp := dpFor(level)
	if dp <= 0 {
		return
	}
	extent := gtx.Metric.Dp(unit.Dp(dp))
	if extent <= 0 {
		return
	}
	offset := extent / 2

	shadowBounds := bounds.Add(image.Pt(0, offset))
	inner := color.NRGBA{A: keyShadowAlpha}
	outer := color.NRGBA{A: 0}

	// Interior of the shadow rectangle. Most of this is later covered
	// by the caller's foreground; the strip extending below bounds
	// stays visible as the cast-shadow band.
	paint.FillShape(gtx.Ops, inner, clip.Rect(shadowBounds).Op())

	r := extent
	bMin, bMax := shadowBounds.Min, shadowBounds.Max

	// Edge bands. Inner stop is flush with the shadow rectangle's edge
	// (matching the interior fill at the seam); outer stop sits at
	// distance extent for zero alpha.
	fillTile(gtx,
		image.Rect(bMin.X, bMin.Y-r, bMax.X, bMin.Y),
		f32.Pt(0, float32(bMin.Y-r)), outer,
		f32.Pt(0, float32(bMin.Y)), inner,
	)
	fillTile(gtx,
		image.Rect(bMin.X, bMax.Y, bMax.X, bMax.Y+r),
		f32.Pt(0, float32(bMax.Y)), inner,
		f32.Pt(0, float32(bMax.Y+r)), outer,
	)
	fillTile(gtx,
		image.Rect(bMin.X-r, bMin.Y, bMin.X, bMax.Y),
		f32.Pt(float32(bMin.X-r), 0), outer,
		f32.Pt(float32(bMin.X), 0), inner,
	)
	fillTile(gtx,
		image.Rect(bMax.X, bMin.Y, bMax.X+r, bMax.Y),
		f32.Pt(float32(bMax.X), 0), inner,
		f32.Pt(float32(bMax.X+r), 0), outer,
	)

	// Corner tiles: a 45°-diagonal gradient with the outer stop placed
	// at half the extent along the diagonal so each corner's outer
	// boundary meets the adjacent edge tiles at zero alpha — the same
	// seam-free trick used by [pulse/glow.Halo].
	h := float32(r) / 2
	fillTile(gtx,
		image.Rect(bMin.X-r, bMin.Y-r, bMin.X, bMin.Y),
		f32.Pt(float32(bMin.X)-h, float32(bMin.Y)-h), outer,
		f32.Pt(float32(bMin.X), float32(bMin.Y)), inner,
	)
	fillTile(gtx,
		image.Rect(bMax.X, bMin.Y-r, bMax.X+r, bMin.Y),
		f32.Pt(float32(bMax.X), float32(bMin.Y)), inner,
		f32.Pt(float32(bMax.X)+h, float32(bMin.Y)-h), outer,
	)
	fillTile(gtx,
		image.Rect(bMin.X-r, bMax.Y, bMin.X, bMax.Y+r),
		f32.Pt(float32(bMin.X)-h, float32(bMax.Y)+h), outer,
		f32.Pt(float32(bMin.X), float32(bMax.Y)), inner,
	)
	fillTile(gtx,
		image.Rect(bMax.X, bMax.Y, bMax.X+r, bMax.Y+r),
		f32.Pt(float32(bMax.X), float32(bMax.Y)), inner,
		f32.Pt(float32(bMax.X)+h, float32(bMax.Y)+h), outer,
	)
}

func dpFor(level tokens.ElevationLevel) float32 {
	switch level {
	case tokens.Level0:
		return tokens.Elevation.Level0
	case tokens.Level1:
		return tokens.Elevation.Level1
	case tokens.Level2:
		return tokens.Elevation.Level2
	case tokens.Level3:
		return tokens.Elevation.Level3
	case tokens.Level4:
		return tokens.Elevation.Level4
	case tokens.Level5:
		return tokens.Elevation.Level5
	}
	return 0
}

func fillTile(gtx layout.Context, rect image.Rectangle, stop1 f32.Point, c1 color.NRGBA, stop2 f32.Point, c2 color.NRGBA) {
	if rect.Dx() <= 0 || rect.Dy() <= 0 {
		return
	}
	defer clip.Rect(rect).Push(gtx.Ops).Pop()
	paint.LinearGradientOp{
		Stop1:  stop1,
		Color1: c1,
		Stop2:  stop2,
		Color2: c2,
	}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}
