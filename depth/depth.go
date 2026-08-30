// Package depth renders Material-style cast shadows under rectangular
// regions by composing linear gradients.
//
// # An explicit effect, never a default
//
// A shadow is opt-in vibrancy, not something a component gets for
// being raised. On desktop a raised surface reads as raised by tint
// first and shadow second: it names its rung on
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
// tiles — plus one interior fill: nine paint operations per shadow,
// every frame it is drawn. A surface step is a single
// [paint.FillShape].
//
// # Geometry
//
// A shadow is a soft black fringe around a "shadow rectangle" — the
// caller's bounds shifted downward by half the elevation extent — to
// approximate light from above. The geometry mirrors
// [github.com/vibrantgio/effects/glow.Halo] but with two differences:
//
//   - The shadow rectangle's interior is filled at the peak alpha too,
//     so the strip extending below bounds stays visible once the caller
//     paints their foreground on top.
//   - Nothing about the colour is a parameter. It is a fixed key-shadow
//     black; the peak alpha scales only with the caller's opacity, and
//     neither varies with the elevation level — only the geometry does.
//
// Extent (the gradient's falloff distance) and offset (the downward
// shift of the shadow rectangle) both follow the level: extent is the
// level's dp value from [tokens.Elevation] converted to pixels through
// gtx.Metric, and offset is half of that. [tokens.Level0] — and any
// level whose dp rounds to zero pixels at the current density — paints
// nothing at all.
//
// # Rounding and opacity
//
// The interior fill is a [clip.RRect] at the caller's radius, so a
// caller passing the same radius it rounds its foreground to does not
// get square dark wedges showing through the rounded corners; the
// corner tiles of the penumbra grow inward to cover the notch between
// the rounded interior and the square corner, keeping the alpha ramp
// seam-free. Opacity scales the whole ramp, so a shadow that fades
// with its surface passes its fade alpha straight through instead of
// wrapping the call in a [paint.PushOpacity] layer. Radius 0 and
// opacity 1 give a square, full-strength shadow.
//
// # One thing a caller trips on
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

	"github.com/vibrantgio/theme/tokens"
)

// keyShadowAlpha is Material 3's recommended key-shadow opacity (~30 %).
// All elevation levels share this peak alpha; only the geometry varies
// with the level. The caller's opacity scales it.
const keyShadowAlpha uint8 = 76

// bezierCircle is the cubic-Bézier control-point ratio that best
// approximates a quarter circle: 4/3·(√2−1).
const bezierCircle = 0.55228475

// Shadow paints a Material-style cast shadow under bounds onto gtx.Ops
// at the given elevation level. The shadow is biased downward to
// approximate light from above.
//
// radius rounds the shadow rectangle's corners, in pixels. Callers
// pass the radius they round their foreground to, so the interior
// fill cannot show through the rounding as square wedges; 0 keeps the
// square geometry. opacity scales the shadow's alpha ramp and is
// clamped to [0, 1]; a shadow that fades with its surface passes the
// surface's fade alpha here.
//
// At [tokens.Level0] (and any level whose dp value rounds to zero pixels
// at the current metric), and at opacity 0, the function is a no-op.
func Shadow(gtx layout.Context, bounds image.Rectangle, level tokens.ElevationLevel, radius int, opacity float32) {
	dp := dpFor(level)
	if dp <= 0 {
		return
	}
	extent := gtx.Metric.Dp(unit.Dp(dp))
	if extent <= 0 {
		return
	}
	if opacity > 1 {
		opacity = 1
	}
	if opacity <= 0 {
		return
	}
	alpha := uint8(float32(keyShadowAlpha)*opacity + 0.5)
	if alpha == 0 {
		return
	}
	offset := extent / 2

	shadowBounds := bounds.Add(image.Pt(0, offset))
	if radius < 0 {
		radius = 0
	}
	if m := min(shadowBounds.Dx(), shadowBounds.Dy()) / 2; radius > m {
		radius = m
	}
	inner := color.NRGBA{A: alpha}
	outer := color.NRGBA{A: 0}

	// Interior of the shadow rectangle, rounded to the caller's radius.
	// Most of this is later covered by the caller's foreground; the
	// strip extending below bounds stays visible as the cast-shadow
	// band.
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
	}
	return 0
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
