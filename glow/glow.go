// Package glow renders luminance halos around rectangular regions by
// composing linear gradients.
//
// A halo is a soft luminous fringe extending Radius pixels outside a
// bounds rectangle, fading from the halo colour at the inner edge to
// fully transparent at the outer edge. Gio's render pipeline only
// exposes [paint.LinearGradientOp] (no native radial gradient), so
// glow approximates a radial falloff by composing eight linear
// gradients — one per edge band, one per corner — each clipped to its
// own region so corners do not double-paint.
//
// # Geometry
//
// The halo region is the strip between bounds and bounds expanded by
// Radius. It splits into eight axis-aligned tiles:
//
//	+-------+-------------+-------+
//	| TL    |    Top      |    TR |
//	+-------+-------------+-------+
//	|       |             |       |
//	| Left  |  (bounds)   | Right |
//	|       |             |       |
//	+-------+-------------+-------+
//	| BL    |   Bottom    |    BR |
//	+-------+-------------+-------+
//
// Edge tiles (Top/Right/Bottom/Left) carry a linear gradient
// perpendicular to the bounds edge: opaque at the inner side, zero
// alpha at the outer side. Corner tiles carry a 45°-diagonal gradient
// whose far stop sits at half the corner-square's diagonal, so that
// the gradient hits zero alpha exactly along the corner-tile edges
// shared with the adjacent edge tiles. This produces a seamless
// alpha boundary between corner and edge tiles without double-paint.
//
// The interior of bounds is never touched: callers that want the
// halo to sit beneath a foreground shape should call [Halo] first
// and then draw the shape on top.
//
// # What the caller has to handle
//
// [Halo] draws entirely outside bounds and returns nothing. It is not a
// [layout.Widget] and it reserves no space, so in a [layout.Flex] the
// halo spills over whatever sits next to it unless the caller either
// insets by Radius to make room or pushes a [clip.Rect] to cut it off.
// Nothing in this package can do that for you — it does not know what
// bounds means in the surrounding layout.
//
// [Options.Radius] is in pixels, not dp. The same Radius is a visibly
// wider halo on a 1x display than on a 2x one, which is backwards from
// every other size in the design system; convert with gtx.Dp first if
// the halo should look the same at both densities.
//
// The falloff is an approximation twice over. Eight linear gradients
// are not a radial gradient — the alpha along a diagonal differs from
// the alpha at the same distance along an axis — and the shape is
// always the bounds rectangle, so a halo around a rounded or circular
// foreground is squared off at the corners. A blur-based glow that
// would fix both is scheduled for a later release, and this package
// keeps the gradient path until that comparison is actually made.
package glow

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// Options configures a single [Halo] call.
type Options struct {
	// Color is the halo's luminance colour. The alpha channel scales
	// the inner-edge alpha; pass alpha 255 for a fully opaque inner
	// edge and use [Options.Intensity] to dim it.
	Color color.NRGBA

	// Radius is the halo extent in pixels — how far the luminance
	// bleeds beyond bounds. Zero or negative produces no halo.
	Radius int

	// Intensity is the peak alpha multiplier in [0, 1] applied at the
	// inner edge. Values outside the range are clamped: <= 0 produces
	// no halo, > 1 is treated as 1.
	Intensity float64
}

// Halo paints a luminance halo around bounds onto gtx.Ops, composing
// eight linear gradients (four edges, four corners). The halo is
// painted in the region outside bounds and never overlaps the
// interior; bounds itself is left untouched.
//
// Halo is a no-op when Radius <= 0 or Intensity <= 0.
func Halo(gtx layout.Context, bounds image.Rectangle, opts Options) {
	if opts.Radius <= 0 || opts.Intensity <= 0 {
		return
	}
	intensity := opts.Intensity
	if intensity > 1 {
		intensity = 1
	}

	inner := opts.Color
	inner.A = uint8(float64(opts.Color.A)*intensity + 0.5)
	outer := opts.Color
	outer.A = 0

	r := opts.Radius
	bMin, bMax := bounds.Min, bounds.Max

	// Edge bands. Inner stop sits flush with the bounds edge so the
	// row immediately outside bounds renders at near-peak alpha; the
	// outer stop sits at distance Radius for zero alpha.
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

	// Corner tiles: gradient runs along the diagonal from the inner
	// corner outward. Placing the outer stop at half the radius along
	// the diagonal makes the gradient hit zero alpha exactly along
	// the two corner-tile edges that meet the adjacent edge tiles —
	// matching the edge tiles' alpha at those shared boundaries and
	// avoiding any seam.
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
