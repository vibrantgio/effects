package depth_test

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/vibrantgio/prism/golden"
	depth "github.com/vibrantgio/pulse/depth"
	"github.com/vibrantgio/spectrum/tokens"
)

const (
	canvasW, canvasH                       = 160, 100
	boundsX0, boundsY0, boundsX1, boundsY1 = 50, 30, 110, 70
)

var (
	bgColor    = color.NRGBA{R: 248, G: 248, B: 250, A: 255}
	fgColor    = color.NRGBA{R: 60, G: 110, B: 200, A: 255}
	canvasSize = image.Pt(canvasW, canvasH)
	boundsRect = image.Rect(boundsX0, boundsY0, boundsX1, boundsY1)
)

// scene composes a light backdrop, a cast shadow at the given level,
// and a foreground rectangle drawn on top of the shadow. The light
// backdrop gives the dark shadow unambiguous contrast for golden
// diffing; the foreground rect anchors bounds so a missing or
// mis-offset shadow is visually obvious.
func scene(level tokens.ElevationLevel) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		depth.Shadow(gtx, boundsRect, level, 0, 1)
		paint.FillShape(gtx.Ops, fgColor, clip.Rect(boundsRect).Op())
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
}

// roundedScene mirrors scene but rounds both the shadow and the
// foreground to the same radius — the shape every organizational
// caller draws. The wedge defect FX.3 fixed was the square interior
// fill showing through the foreground's rounded corners; only a
// golden catches it.
func roundedScene(level tokens.ElevationLevel, radius int, opacity float32) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		depth.Shadow(gtx, boundsRect, level, radius, opacity)
		paint.FillShape(gtx.Ops, fgColor, clip.RRect{
			Rect: boundsRect,
			SE:   radius, SW: radius, NE: radius, NW: radius,
		}.Op(gtx.Ops))
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
}

// ---- tests ----

// TestShadowGoldens exercises the G3.3b Measurable: golden-image tests
// at every elevation level (level-0 through level-3 — the desktop ladder
// tops out at 3 since spectrum v0.2.0 dropped levels 4 and 5).
func TestShadowGoldens(t *testing.T) {
	cases := []struct {
		name  string
		level tokens.ElevationLevel
	}{
		{"level-0", tokens.Level0},
		{"level-1", tokens.Level1},
		{"level-2", tokens.Level2},
		{"level-3", tokens.Level3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			golden.Render(t, tc.name, canvasSize, scene(tc.level))
		})
	}
}

// TestShadowRoundedGolden exercises the FX.3 fix: a rounded surface
// over a shadow rounded to the same radius. Before the fix the
// interior fill was a hard clip.Rect, and this scene showed its
// square corners through the foreground's rounding as four dark
// wedges — the exact pixels this golden pins.
func TestShadowRoundedGolden(t *testing.T) {
	golden.Render(t, "level-3-rounded", canvasSize, roundedScene(tokens.Level3, 12, 1))
}

// TestShadowOpacity asserts the opacity parameter scales the ramp:
// 0 paints nothing at all, and a half-strength shadow differs from a
// full-strength one.
func TestShadowOpacity(t *testing.T) {
	bg := golden.Capture(t, canvasSize, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	shadowAt := func(opacity float32) *image.RGBA {
		return golden.Capture(t, canvasSize, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
			depth.Shadow(gtx, boundsRect, tokens.Level3, 12, opacity)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
	}
	zero, half, full := shadowAt(0), shadowAt(0.5), shadowAt(1)
	if n := golden.PixelDiff(bg, zero); n != 0 {
		t.Errorf("opacity 0 painted %d pixel(s); want a no-op", n)
	}
	if n := golden.PixelDiff(half, full); n == 0 {
		t.Errorf("opacity 0.5 renders identically to opacity 1; want a lighter shadow")
	}
	if n := golden.PixelDiff(bg, half); n == 0 {
		t.Errorf("opacity 0.5 painted nothing; want a visible shadow")
	}
}

// TestShadowAdjacentLevelsDiffer asserts that each adjacent pair of
// elevation levels produces a visibly different render. Catches
// regressions where the level-to-geometry mapping silently rounds
// adjacent levels into the same offset/extent — which would let "four
// different" goldens drift toward the same byte sequence over time.
func TestShadowAdjacentLevelsDiffer(t *testing.T) {
	levels := []tokens.ElevationLevel{
		tokens.Level0, tokens.Level1, tokens.Level2, tokens.Level3,
	}
	imgs := make([]*image.RGBA, len(levels))
	for i, l := range levels {
		imgs[i] = golden.Capture(t, canvasSize, scene(l))
	}
	for i := 0; i < len(imgs)-1; i++ {
		if n := golden.PixelDiff(imgs[i], imgs[i+1]); n == 0 {
			t.Errorf("level-%d and level-%d render identically; expected adjacent levels to differ",
				i, i+1)
		}
	}
}
