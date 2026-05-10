// Package depth renders Material-style cast shadows under rectangular
// regions by composing linear gradients.
//
// A shadow is a soft black fringe around a "shadow rectangle" — the
// caller's bounds shifted downward by half the elevation extent — to
// approximate light from above. The geometry mirrors [pulse/glow.Halo]
// but with two differences:
//
//   - The colour is a fixed Material-style key-shadow black; intensity
//     is a function of elevation, not a caller knob.
//   - The shadow rectangle's interior is filled with the inner alpha so
//     the strip extending below bounds remains visible once the caller
//     paints their foreground rectangle on top.
//
// Both extent (gradient falloff distance) and offset (downward shift of
// the shadow rectangle) are derived from [tokens.ElevationLevel] via
// [tokens.Elevation]: extent equals the level's dp value; offset is
// half the extent.
//
// The shadow is painted before the foreground; the caller draws their
// rectangle on top of [Shadow]'s output.
package depth

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/vibrantgio/prism/tokens"
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
