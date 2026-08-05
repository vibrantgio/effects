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
// foreground is squared off at the corners. A blur-based glow would
// fix both; it was prototyped, measured and rejected — see below.
//
// # Why not a real blur (E4.4 evaluation)
//
// A blur-based glow was prototyped against this package: rasterize
// the shape at peak alpha, Gaussian-blur it with pulse/blur at
// sigma = Radius/2 (so both falloffs reach zero at Radius), and paint
// the result as an image. The prototype, the falloff-comparison test
// and the benchmarks live in blurglow_test.go; it deliberately does
// not ship as API.
//
// Visually the blur is the better halo: a true radial falloff with
// smooth rounded corners and no tile seams. The gradient rim is
// visibly chamfered — the corner tiles' far stop sits at Radius/√2,
// so the diagonal falloff dies at ~0.71·Radius with a C0 kink, an
// octagonal rim that grows more obvious with Radius. Measured halo
// coverage over a dark background at Radius 16 (0 = background,
// 1 = opaque halo): at 0.75·Radius the gradient reads 0.43 along an
// edge normal but 0.00 along the 45° diagonal, where the blur reads a
// consistent 0.02/0.005. The blur is not a drop-in either: blurring a
// step edge halves it, so its inner rim renders at 0.39 coverage
// where the gradient renders 0.99 — at equal Intensity the blur halo
// looks roughly half as bright with a shorter perceived reach, and a
// replacement would need intensity compensation or pre-blur shape
// dilation.
//
// The animated cost decided it. Per-frame cost of one glow, measured
// on a ten-core Apple Silicon machine (darwin/arm64, go test -bench,
// 2026-08-05):
//
//	gradient path, op recording (whole per-frame cost)   0.5 µs    ~0 B/frame
//	blur, 132×72 canvas (100×40 button, R=16, σ=8)       0.23 ms    51 KB/frame
//	blur, 348×144 canvas (300×96 card, R=24, σ=12)       0.82 ms   227 KB/frame
//	blur via headless render (arbitrary shapes, ÷1)      0.70 ms   (132×72)
//	headless frame of the gradient scene, 160×100        0.27 ms
//	headless frame of the blur scene, 160×100            0.43 ms
//
// A static glow could pay the blur once and cache the image, but the
// glows worth having animate — Radius or Intensity driven by a spring
// — and a cache keyed on shape parameters (size, radius, colour,
// intensity) misses every frame by construction while the parameters
// move. (pulse/blur.Cache does not offer that keying anyway: it keys
// on source-image identity.) An animating blur-glow therefore costs
// 0.2–0.8 ms of events-thread CPU plus an image-sized allocation and
// a texture upload per glow per frame, against the gradient path's
// ~0.5 µs — and a screen of N glowing widgets multiplies it. Per the
// G-E4 rule that a correct approximation beats a slow exact answer,
// the gradient path stays.
//
// Two findings for whoever revisits this. pulse/blur blurs
// straight-alpha channels independently (its documented translucency
// caveat), so a blur-glow must keep the colour planes uniform across
// the whole canvas and let alpha alone carry the shape — blurring a
// coloured shape over transparent black bleeds black into the halo
// and dims it to roughly alpha². That trick only works for a
// single-colour glow; an arbitrary multi-colour source would need a
// premultiplied-correct blur first. And if a static blur-glow is ever
// wanted (it did look better), the missing piece is small: a cache
// keyed on (size, corner radius, sigma, colour, intensity) in front
// of the prototype in blurglow_test.go.
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
