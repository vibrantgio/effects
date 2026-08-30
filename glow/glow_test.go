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
	canvasW, canvasH                       = 160, 100
	boundsX0, boundsY0, boundsX1, boundsY1 = 50, 30, 110, 70
	haloRadius                             = 16
)

var (
	bgColor    = color.NRGBA{R: 40, G: 40, B: 48, A: 255}
	fgColor    = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	haloColor  = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	canvasSize = image.Pt(canvasW, canvasH)
	haloBounds = image.Rect(boundsX0, boundsY0, boundsX1, boundsY1)
)

// scene composes a dark backdrop, an optional halo around bounds, and a
// black foreground rect on top. The dark backdrop gives the additive
// luminance of the halo unambiguous contrast for golden diffing; the
// black foreground rect anchors the inner edge so a missing or
// double-painted halo is visually obvious in the diff.
func scene(bounds image.Rectangle, opts glow.Options) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		glow.Halo(gtx, bounds, opts)
		paint.FillShape(gtx.Ops, fgColor, clip.Rect(bounds).Op())
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
}

func bgOnly(gtx layout.Context) layout.Dimensions {
	paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

// ---- tests ----

// TestHaloGoldens pins golden images at four intensities.
// intensity-zero is the no-halo baseline, proving the halo path is
// opt-in; the remaining three cover the spectrum.
func TestHaloGoldens(t *testing.T) {
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
			opts := glow.Options{Color: haloColor, Radius: haloRadius, Intensity: tc.intensity}
			golden.Render(t, tc.name, canvasSize, scene(haloBounds, opts))
		})
	}
}

// TestHaloIntensitySteppingDiffers asserts that successive intensity
// stops produce visibly different renders. Catches regressions where
// the intensity multiplier silently saturates or rounds to a single
// alpha bucket — which would let four "different" goldens drift to
// the same byte sequence over time.
func TestHaloIntensitySteppingDiffers(t *testing.T) {
	cap := func(intensity float64) *image.RGBA {
		return golden.Capture(t, canvasSize, scene(haloBounds, glow.Options{
			Color: haloColor, Radius: haloRadius, Intensity: intensity,
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
			t.Errorf("intensity %s and %s render identically; expected halo intensity to affect pixels",
				p.nameA, p.nameB)
		}
	}
}

// TestHaloDoesNotPaintInsideBounds asserts the halo paints only outside
// bounds: pixels strictly inside bounds must match the no-halo baseline
// even at maximum intensity. Guards against a regression where an edge
// tile's clip rect grows by one pixel and bleeds into the foreground.
func TestHaloDoesNotPaintInsideBounds(t *testing.T) {
	withHalo := golden.Capture(t, canvasSize, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		glow.Halo(gtx, haloBounds, glow.Options{
			Color: haloColor, Radius: haloRadius, Intensity: 1,
		})
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	noHalo := golden.Capture(t, canvasSize, bgOnly)
	for y := haloBounds.Min.Y; y < haloBounds.Max.Y; y++ {
		for x := haloBounds.Min.X; x < haloBounds.Max.X; x++ {
			off := y*withHalo.Stride + x*4
			for ch := 0; ch < 4; ch++ {
				if withHalo.Pix[off+ch] != noHalo.Pix[off+ch] {
					t.Fatalf("interior pixel (%d, %d) channel %d differs: with-halo=%d, no-halo=%d",
						x, y, ch, withHalo.Pix[off+ch], noHalo.Pix[off+ch])
				}
			}
		}
	}
}

// TestHaloNoOpAtZeroRadius asserts Halo with Radius=0 is a no-op.
func TestHaloNoOpAtZeroRadius(t *testing.T) {
	bg := golden.Capture(t, canvasSize, bgOnly)
	zeroR := golden.Capture(t, canvasSize, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		glow.Halo(gtx, haloBounds, glow.Options{
			Color: haloColor, Radius: 0, Intensity: 1,
		})
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	if n := golden.PixelDiff(bg, zeroR); n != 0 {
		t.Errorf("Halo with Radius=0 should be a no-op; %d pixels differ", n)
	}
}

// TestHaloNoOpAtZeroIntensity asserts Halo with Intensity=0 is a no-op.
func TestHaloNoOpAtZeroIntensity(t *testing.T) {
	bg := golden.Capture(t, canvasSize, bgOnly)
	zeroI := golden.Capture(t, canvasSize, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		glow.Halo(gtx, haloBounds, glow.Options{
			Color: haloColor, Radius: haloRadius, Intensity: 0,
		})
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	if n := golden.PixelDiff(bg, zeroI); n != 0 {
		t.Errorf("Halo with Intensity=0 should be a no-op; %d pixels differ", n)
	}
}

// TestHaloIntensityClamps asserts Intensity > 1 is clamped, not
// extrapolated: Intensity=2 must render byte-identical to Intensity=1.
func TestHaloIntensityClamps(t *testing.T) {
	one := golden.Capture(t, canvasSize, scene(haloBounds, glow.Options{
		Color: haloColor, Radius: haloRadius, Intensity: 1,
	}))
	two := golden.Capture(t, canvasSize, scene(haloBounds, glow.Options{
		Color: haloColor, Radius: haloRadius, Intensity: 2,
	}))
	if n := golden.PixelDiff(one, two); n != 0 {
		t.Errorf("Intensity > 1 should clamp to 1; %d pixels differ between Intensity=1 and Intensity=2", n)
	}
}
