package transition_test

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/vibrantgio/prism/golden"
	"github.com/vibrantgio/pulse/transition"
	"github.com/vibrantgio/spectrum/tokens"
)

// TestThemeTransitionGolden discharges G2.3 Measurable: a golden test of a
// transitioning theme at frame 0/15/30, with the tween settling to the
// target colour tokens at frame 30.
//
// The swatch is painted directly with image/draw rather than through Gio.
// This package is testing colour-value interpolation, not widget rendering;
// the GPU layer would only add headless-render flake without exercising
// anything new.
func TestThemeTransitionGolden(t *testing.T) {
	const frames = 30
	tw := transition.ColorTokensTween(tokens.DefaultLight, tokens.DefaultDark, frames)

	cases := []struct {
		name  string
		frame int
	}{
		{"theme-transition-frame00", 0},
		{"theme-transition-frame15", 15},
		{"theme-transition-frame30", frames},
	}

	size := image.Pt(300, 60)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img := image.NewNRGBA(image.Rectangle{Max: size})
			paintSwatch(img, tw.At(tc.frame))
			golden.CompareNRGBA(t, tc.name, img)
		})
	}

	// "tween settles to target" — verify at the value level, not just the
	// pixel level. Pixel goldens alone could miss a settling bug if the
	// final lerp happens to round to visually identical bytes.
	if got := tw.At(frames); got != tokens.DefaultDark {
		t.Errorf("tween did not settle to target at frame %d: got %+v, want DefaultDark", frames, got)
	}
}

// paintSwatch fills img with colors.Background, then paints five vertical
// bands showing Surface, Primary, Secondary, OnPrimary, and the Neutral 500
// strong-border step. These fields together carry enough contrast to make
// light/dark/midpoint frames visually distinct in the golden PNGs.
func paintSwatch(img *image.NRGBA, colors tokens.ColorTokens) {
	bounds := img.Bounds()
	draw.Draw(img, bounds, &image.Uniform{C: colors.Background}, image.Point{}, draw.Src)

	bands := []color.NRGBA{
		colors.Surface,
		colors.Primary,
		colors.Secondary,
		colors.OnPrimary,
		colors.Ramps.Neutral.Step(500),
	}
	bandW := bounds.Dx() / len(bands)
	const inset = 10
	for i, c := range bands {
		rect := image.Rect(
			bounds.Min.X+i*bandW, bounds.Min.Y+inset,
			bounds.Min.X+(i+1)*bandW, bounds.Max.Y-inset,
		)
		draw.Draw(img, rect, &image.Uniform{C: c}, image.Point{}, draw.Src)
	}
}
