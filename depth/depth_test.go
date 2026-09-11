package depth_test

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/vibrantgio/components/golden"
	depth "github.com/vibrantgio/effects/depth"
	"github.com/vibrantgio/theme/tokens"
)

const (
	frameW, frameH                         = 160, 100
	boundsX0, boundsY0, boundsX1, boundsY1 = 50, 30, 110, 70
)

var (
	fgColor    = color.NRGBA{R: 60, G: 110, B: 200, A: 255}
	frameSize  = image.Pt(frameW, frameH)
	boundsRect = image.Rect(boundsX0, boundsY0, boundsX1, boundsY1)
)

// faded returns the platform's floating shadow at a share of its own
// coverage, which is how a surface that fades takes its shadow with it.
func faded(p tokens.PlatformColors, share float32) color.NRGBA {
	s := p.FloatingShadow
	s.A = uint8(float32(s.A)*share + 0.5)
	return s
}

// scene composes the window's own plane, a cast shadow, and a foreground
// rectangle drawn on top of it. The plane is the platform's, so the shadow is
// read where it is actually drawn; the foreground rect anchors bounds so a
// missing or mis-placed shadow is visually obvious.
func scene(p tokens.PlatformColors, shadow color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, p.WindowBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
		depth.Shadow(gtx, boundsRect, 0, shadow)
		paint.FillShape(gtx.Ops, fgColor, clip.Rect(boundsRect).Op())
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
}

// roundedScene mirrors scene but rounds both the shadow and the
// foreground to the same radius. A square interior fill showing
// through the foreground's rounded corners is a defect only a golden
// catches.
func roundedScene(p tokens.PlatformColors, radius int, shadow color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, p.WindowBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
		depth.Shadow(gtx, boundsRect, radius, shadow)
		paint.FillShape(gtx.Ops, fgColor, clip.RRect{
			Rect: boundsRect,
			SE:   radius, SW: radius, NE: radius, NW: radius,
		}.Op(gtx.Ops))
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
}

// ---- tests ----

// TestShadowGoldens pins the one shadow the platform draws, in both
// appearances, and the same shadow half faded out — which is the whole of
// what varies: a floating surface either casts the platform's shadow or is
// on its way in or out.
func TestShadowGoldens(t *testing.T) {
	for _, tc := range []struct {
		name   string
		colors tokens.PlatformColors
		share  float32
	}{
		{"floating-light", tokens.PlatformLight, 1},
		{"floating-dark", tokens.PlatformDark, 1},
		{"floating-light-half", tokens.PlatformLight, 0.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			golden.Render(t, tc.name, frameSize, scene(tc.colors, faded(tc.colors, tc.share)))
		})
	}
}

// TestShadowRoundedGolden pins a rounded surface over a shadow rounded
// to the same radius: the interior fill must not show square corners
// through the foreground's rounding as four dark wedges.
func TestShadowRoundedGolden(t *testing.T) {
	p := tokens.PlatformLight
	golden.Render(t, "floating-rounded", frameSize, roundedScene(p, 12, p.FloatingShadow))
}

// TestShadowFollowsItsCoverage asserts the one parameter that varies: a
// shadow with no coverage paints nothing, and a half-faded one is lighter
// than the platform's own without disappearing.
func TestShadowFollowsItsCoverage(t *testing.T) {
	p := tokens.PlatformLight
	plane := golden.Capture(t, frameSize, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, p.WindowBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	shadowAt := func(share float32) *image.RGBA {
		return golden.Capture(t, frameSize, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, p.WindowBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
			depth.Shadow(gtx, boundsRect, 12, faded(p, share))
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
	}
	zero, half, full := shadowAt(0), shadowAt(0.5), shadowAt(1)
	if n := golden.PixelDiff(plane, zero); n != 0 {
		t.Errorf("a shadow with no coverage painted %d pixel(s); want a no-op", n)
	}
	if n := golden.PixelDiff(half, full); n == 0 {
		t.Error("half the coverage renders identically to the platform's own; want a lighter shadow")
	}
	if n := golden.PixelDiff(plane, half); n == 0 {
		t.Error("half the coverage painted nothing; want a visible shadow")
	}
}

// TestShadowReachesTheMeasuredDistance reads the ramp off the render the way
// it was read off the captures: outward from the surface's edge along one
// row, the plane must be darkened at the edge, still darkened a pixel inside
// the reach, and back to its own value a pixel past it.
func TestShadowReachesTheMeasuredDistance(t *testing.T) {
	// The light plane reads the tail: at 4 px inside the reach the ramp is a
	// eightieth of the way to black, which moves a 255 by three and a 30 by
	// less than half a unit.
	p := tokens.PlatformLight
	const pad = 40
	size := image.Pt(boundsX1+pad, frameH)
	img := golden.Capture(t, size, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, p.WindowBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
		depth.Shadow(gtx, boundsRect, 0, p.FloatingShadow)
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	reach := 24 // Reach in px at the test's 1:1 metric
	y := (boundsY0 + boundsY1) / 2
	at := func(x int) color.NRGBA {
		i := img.PixOffset(x, y)
		return color.NRGBA{R: img.Pix[i], G: img.Pix[i+1], B: img.Pix[i+2], A: img.Pix[i+3]}
	}
	plane := at(boundsX1 + reach + 2)
	if plane.R != p.WindowBackground.R || plane.G != p.WindowBackground.G || plane.B != p.WindowBackground.B {
		t.Errorf("%d px past the surface the plane reads %v, want the window's own %v — the shadow carries further than it was measured to",
			reach+2, plane, p.WindowBackground)
	}
	if edge := at(boundsX1 + 1); edge.G >= plane.G {
		t.Errorf("at the surface's edge the plane reads %v against its own %v; the shadow is not at its peak there", edge, plane)
	}
	if inside := at(boundsX1 + reach - 4); inside.G >= plane.G {
		t.Errorf("%d px out the plane already reads its own value %v; the shadow stops short of the measured reach", reach-4, plane)
	}
}
