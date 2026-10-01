package glow_test

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/vibrantgio/components/golden"
	glow "github.com/vibrantgio/effects/glow"
)

// ---- test fixture geometry ----

const (
	frameW, frameH                         = 160, 100
	boundsX0, boundsY0, boundsX1, boundsY1 = 50, 30, 110, 70
	spreadRadius                           = 16
)

var (
	bgColor      = color.NRGBA{R: 40, G: 40, B: 48, A: 255}
	fgColor      = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	spreadColor  = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	frameSize    = image.Pt(frameW, frameH)
	spreadBounds = image.Rect(boundsX0, boundsY0, boundsX1, boundsY1)
)

// scene composes a dark backdrop, an optional spread around bounds, and a
// black foreground rect on top. The dark backdrop gives the additive
// luminance of the spread unambiguous contrast for golden diffing; the
// black foreground rect anchors the inner edge so a missing or
// double-painted spread is visually obvious in the diff.
func scene(bounds image.Rectangle, opts glow.Options) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		glow.Spread(gtx, bounds, opts)
		paint.FillShape(gtx.Ops, fgColor, clip.Rect(bounds).Op())
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
}

func bgOnly(gtx layout.Context) layout.Dimensions {
	paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

// ---- tests ----

// TestSpreadGoldens pins golden images at four intensities.
// intensity-zero is the no-spread baseline, proving the spread path is
// opt-in; the remaining three cover the spectrum.
func TestSpreadGoldens(t *testing.T) {
	cases := []struct {
		name      string
		intensity float64
	}{
		{"intensity-zero", 0},
		{"intensity-low", 0.25},
		{"intensity-mid", 0.5},
		{"intensity-high", 1.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := glow.Options{Color: spreadColor, Radius: spreadRadius, Intensity: tc.intensity}
			golden.Render(t, tc.name, frameSize, scene(spreadBounds, opts))
		})
	}
}

// TestSpreadIntensitySteppingDiffers asserts that successive intensity
// stops produce visibly different renders. Catches regressions where
// the intensity multiplier silently saturates or rounds to a single
// alpha bucket — which would let four "different" goldens drift to
// the same byte sequence over time.
func TestSpreadIntensitySteppingDiffers(t *testing.T) {
	cap := func(intensity float64) *image.RGBA {
		return golden.Capture(t, frameSize, scene(spreadBounds, glow.Options{
			Color: spreadColor, Radius: spreadRadius, Intensity: intensity,
		}))
	}
	zero, low, mid, high := cap(0), cap(0.25), cap(0.5), cap(1.0)
	pairs := []struct {
		a, b         *image.RGBA
		nameA, nameB string
	}{
		{zero, low, "zero", "low"},
		{low, mid, "low", "mid"},
		{mid, high, "mid", "high"},
	}
	for _, p := range pairs {
		if n := golden.PixelDiff(p.a, p.b); n == 0 {
			t.Errorf("intensity %s and %s render identically; expected spread intensity to affect pixels",
				p.nameA, p.nameB)
		}
	}
}

// TestSpreadDoesNotPaintInsideBounds asserts the spread paints only outside
// bounds: pixels strictly inside bounds must match the no-spread baseline
// even at maximum intensity. Guards against a regression where an edge
// tile's clip rect grows by one pixel and bleeds into the foreground.
func TestSpreadDoesNotPaintInsideBounds(t *testing.T) {
	withSpread := golden.Capture(t, frameSize, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		glow.Spread(gtx, spreadBounds, glow.Options{
			Color: spreadColor, Radius: spreadRadius, Intensity: 1,
		})
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	noSpread := golden.Capture(t, frameSize, bgOnly)
	for y := spreadBounds.Min.Y; y < spreadBounds.Max.Y; y++ {
		for x := spreadBounds.Min.X; x < spreadBounds.Max.X; x++ {
			off := y*withSpread.Stride + x*4
			for ch := 0; ch < 4; ch++ {
				if withSpread.Pix[off+ch] != noSpread.Pix[off+ch] {
					t.Fatalf("interior pixel (%d, %d) channel %d differs: with-spread=%d, no-spread=%d",
						x, y, ch, withSpread.Pix[off+ch], noSpread.Pix[off+ch])
				}
			}
		}
	}
}

// TestSpreadNoOpAtZeroRadius asserts Spread with Radius=0 is a no-op.
func TestSpreadNoOpAtZeroRadius(t *testing.T) {
	bg := golden.Capture(t, frameSize, bgOnly)
	zeroR := golden.Capture(t, frameSize, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		glow.Spread(gtx, spreadBounds, glow.Options{
			Color: spreadColor, Radius: 0, Intensity: 1,
		})
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	if n := golden.PixelDiff(bg, zeroR); n != 0 {
		t.Errorf("Spread with Radius=0 should be a no-op; %d pixels differ", n)
	}
}

// TestSpreadNoOpAtZeroIntensity asserts Spread with Intensity=0 is a no-op.
func TestSpreadNoOpAtZeroIntensity(t *testing.T) {
	bg := golden.Capture(t, frameSize, bgOnly)
	zeroI := golden.Capture(t, frameSize, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		glow.Spread(gtx, spreadBounds, glow.Options{
			Color: spreadColor, Radius: spreadRadius, Intensity: 0,
		})
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	if n := golden.PixelDiff(bg, zeroI); n != 0 {
		t.Errorf("Spread with Intensity=0 should be a no-op; %d pixels differ", n)
	}
}

// TestSpreadIntensityClamps asserts Intensity > 1 is clamped, not
// extrapolated: Intensity=2 must render byte-identical to Intensity=1.
func TestSpreadIntensityClamps(t *testing.T) {
	one := golden.Capture(t, frameSize, scene(spreadBounds, glow.Options{
		Color: spreadColor, Radius: spreadRadius, Intensity: 1,
	}))
	two := golden.Capture(t, frameSize, scene(spreadBounds, glow.Options{
		Color: spreadColor, Radius: spreadRadius, Intensity: 2,
	}))
	if n := golden.PixelDiff(one, two); n != 0 {
		t.Errorf("Intensity > 1 should clamp to 1; %d pixels differ between Intensity=1 and Intensity=2", n)
	}
}
