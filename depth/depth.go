// Package depth renders the shadow a floating surface casts, by composing
// linear gradients.
//
// # An explicit effect, never a default
//
// A shadow is what tells a floating thing from a raised one. A raised
// surface — a card, a grouped box, a filled inset — is told by its own small
// step of fill and draws no shadow at all; a floating one — a dialog, a
// toast, a menu, a popover, a tooltip — stands on the window's own plane and
// is told by this. No component should default into calling this package for
// something that stays where it is.
//
// The cost difference backs the rule. One [Shadow] call issues eight
// [paint.LinearGradientOp] fills — four edge bands and four corner
// tiles — plus one interior fill: nine paint operations per shadow,
// every frame it is drawn. A step of fill is a single [paint.FillShape].
//
// # The measurement
//
// The colour, the coverage and the reach are the platform's, not a
// recommendation: outward from a floating pane's 1 px edge stroke the
// window's #232a2e plane reads #20272b and recovers to #232a2e over 24 px,
// identically in finder-sidebar-shadow.png and reminders-sidebar-shadow.png
// in the organization's macOS reference. Black at 0.075 reproduces the
// darkest pixel on every channel, which is what tokens.PlatformColors'
// FloatingShadow carries, and the falloff from there is LINEAR in the encoded
// pixel: with the peak at the pane's edge and zero 24 px out, a linear ramp
// reproduces the captured byte to within one 255th at every distance on every
// channel, and no other shape tried does better — a square falloff misses 24
// of the 29 sampled distances, a square-root one 16 and a smoothstep 11,
// against the linear ramp's 10, and every miss in every shape is a single
// 255th, the plane being dark enough that one unit is a twentieth of the
// whole shadow.
//
// The ramp is symmetric around the surface, with no downward bias. The
// captures are of a vertical edge and show the peak at the pane's own edge;
// nothing measured supports lighting the shadow from above, so the shadow
// rectangle is the caller's bounds and not a shifted copy of them.
//
// # Geometry
//
// A shadow is a soft fringe around the caller's bounds: the interior filled
// at the peak coverage — so the ring stays continuous once the caller paints
// their foreground on top — and eight gradient tiles carrying the ramp out to
// the reach.
//
// # Rounding
//
// The interior fill is a [clip.RRect] at the caller's radius, so a
// caller passing the same radius it rounds its foreground to does not
// get square dark wedges showing through the rounded corners; the
// corner tiles of the penumbra grow inward to cover the notch between
// the rounded interior and the square corner, keeping the ramp
// seam-free. Radius 0 gives a square shadow.
//
// # Fading one
//
// A surface that fades passes a shadow whose own coverage is scaled, rather
// than wrapping the call in a [paint.PushOpacity] layer: the parameter is the
// colour to draw the peak in, so a toast at a third of its way in hands over
// FloatingShadow with a third of its coverage.
package depth

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// Reach is how far the shadow carries past the surface it is cast by, and it
// is a measurement: the window plane recovers to its own value exactly 24 px
// out from a floating pane's edge in both stored sidebar-shadow captures. It
// does not vary with what is floating — the platform draws one shadow.
const Reach = unit.Dp(24)

// bezierCircle is the cubic-Bézier control-point ratio that best
// approximates a quarter circle: 4/3·(√2−1).
const bezierCircle = 0.55228475

// Shadow paints the shadow a floating surface casts around bounds onto
// gtx.Ops, in shadow at its own coverage at the surface's edge, falling
// linearly to nothing [Reach] away.
//
// shadow is the platform's FloatingShadow, or a copy of it whose coverage the
// caller has scaled to fade with the surface. radius rounds the shadow's
// corners, in pixels: callers pass the radius they round their foreground to,
// so the interior fill cannot show through the rounding as square wedges, and
// 0 keeps the square geometry.
//
// A zero coverage, or a reach that rounds to zero pixels at the current
// metric, paints nothing.
func Shadow(gtx layout.Context, bounds image.Rectangle, radius int, shadow color.NRGBA) {
	extent := gtx.Metric.Dp(Reach)
	if extent <= 0 || shadow.A == 0 {
		return
	}

	shadowBounds := bounds
	if radius < 0 {
		radius = 0
	}
	if m := min(shadowBounds.Dx(), shadowBounds.Dy()) / 2; radius > m {
		radius = m
	}
	inner := shadow
	outer := color.NRGBA{R: shadow.R, G: shadow.G, B: shadow.B}

	// The interior, rounded to the caller's radius. Most of it is covered by
	// the caller's foreground; filling it keeps the ring of tiles around it
	// continuous at their inner seam.
	if radius == 0 {
		paint.FillShape(gtx.Ops, inner, clip.Rect(shadowBounds).Op())
	} else {
		paint.FillShape(gtx.Ops, inner, clip.RRect{
			Rect: shadowBounds,
			SE:   radius, SW: radius, NE: radius, NW: radius,
		}.Op(gtx.Ops))
	}

	r := extent
	rho := radius
	bMin, bMax := shadowBounds.Min, shadowBounds.Max

	// Edge bands, shortened by the corner radius so the corner tiles
	// own the rounded ends. Inner stop is flush with the shadow
	// rectangle's edge (matching the interior fill at the seam); outer
	// stop sits at distance extent for zero alpha.
	fillTile(gtx,
		image.Rect(bMin.X+rho, bMin.Y-r, bMax.X-rho, bMin.Y),
		f32.Pt(0, float32(bMin.Y-r)), outer,
		f32.Pt(0, float32(bMin.Y)), inner,
	)
	fillTile(gtx,
		image.Rect(bMin.X+rho, bMax.Y, bMax.X-rho, bMax.Y+r),
		f32.Pt(0, float32(bMax.Y)), inner,
		f32.Pt(0, float32(bMax.Y+r)), outer,
	)
	fillTile(gtx,
		image.Rect(bMin.X-r, bMin.Y+rho, bMin.X, bMax.Y-rho),
		f32.Pt(float32(bMin.X-r), 0), outer,
		f32.Pt(float32(bMin.X), 0), inner,
	)
	fillTile(gtx,
		image.Rect(bMax.X, bMin.Y+rho, bMax.X+r, bMax.Y-rho),
		f32.Pt(float32(bMax.X), 0), inner,
		f32.Pt(float32(bMax.X+r), 0), outer,
	)

	// Corner tiles: a 45°-diagonal gradient whose outer stop sits half
	// the extent past the inner one, so each corner meets the adjacent
	// edge bands at matching alpha along both seams. The rounding
	// shifts the inner stop radius/2 inside the square corner, which is
	// what keeps the seams continuous once the bands are shortened by
	// radius.
	cornerTile(gtx, image.Pt(bMin.X, bMin.Y), -1, -1, rho, r, inner, outer)
	cornerTile(gtx, image.Pt(bMax.X, bMin.Y), +1, -1, rho, r, inner, outer)
	cornerTile(gtx, image.Pt(bMin.X, bMax.Y), -1, +1, rho, r, inner, outer)
	cornerTile(gtx, image.Pt(bMax.X, bMax.Y), +1, +1, rho, r, inner, outer)
}

// cornerTile fills one corner of the penumbra. corner is the shadow
// rectangle's square corner point and sx, sy its outward direction
// (±1 each). With rho == 0 the tile is the r×r square outside the
// corner. With rho > 0 it grows inward to the (rho+r)-sided square
// anchored at the corner circle's centre, minus the quarter disc the
// rounded interior already painted — covering the notch between the
// arc and the square corner so the shadow has no transparent bite at
// each rounded corner.
//
// The diagonal gradient's inner stop sits rho/2 inside the square
// corner and the outer stop half the extent outside it. Solving the
// seam constraint against both adjacent (shortened) edge bands gives
// exactly that inner-stop shift, so the ramp is continuous across all
// four seams; the only deviation from an ideal rounded penumbra is a
// slight lightening at the middle of each arc.
func cornerTile(gtx layout.Context, corner image.Point, sx, sy, rho, r int, inner, outer color.NRGBA) {
	h := float32(r) / 2
	fx, fy := float32(sx), float32(sy)
	kx, ky := float32(corner.X), float32(corner.Y)
	s1 := f32.Pt(kx-fx*float32(rho)/2, ky-fy*float32(rho)/2)
	s2 := f32.Pt(s1.X+fx*h, s1.Y+fy*h)

	if rho == 0 {
		fillTile(gtx,
			image.Rectangle{
				Min: corner,
				Max: corner.Add(image.Pt(sx*r, sy*r)),
			}.Canon(),
			s1, inner, s2, outer,
		)
		return
	}

	rf := float32(rho)
	ext := rf + float32(r)
	c := f32.Pt(kx-fx*rf, ky-fy*rf) // corner circle centre
	k := bezierCircle * rf

	var p clip.Path
	p.Begin(gtx.Ops)
	p1 := f32.Pt(c.X+fx*rf, c.Y) // arc end on the horizontal axis through the centre
	p5 := f32.Pt(c.X, c.Y+fy*rf) // arc end on the vertical axis through the centre
	p.MoveTo(p1)
	p.LineTo(f32.Pt(c.X+fx*ext, c.Y))
	p.LineTo(f32.Pt(c.X+fx*ext, c.Y+fy*ext))
	p.LineTo(f32.Pt(c.X, c.Y+fy*ext))
	p.LineTo(p5)
	p.CubeTo(
		f32.Pt(p5.X+fx*k, p5.Y),
		f32.Pt(p1.X, p1.Y+fy*k),
		p1,
	)
	p.Close()
	defer clip.Outline{Path: p.End()}.Op().Push(gtx.Ops).Pop()
	paint.LinearGradientOp{
		Stop1:  s1,
		Color1: inner,
		Stop2:  s2,
		Color2: outer,
	}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
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
