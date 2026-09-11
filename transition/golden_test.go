package transition_test

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/vibrantgio/components/golden"
	"github.com/vibrantgio/effects/transition"
	"github.com/vibrantgio/theme/tokens"
)

// TestThemeTransitionGolden is a golden test of a transitioning theme at
// frame 0/15/30, with the tween settling to the target colour set at
// frame 30.
//
// The swatch is painted directly with image/draw rather than through Gio.
// This package is testing colour-value interpolation, not component rendering;
// the GPU layer would only add headless-render flake without exercising
// anything new.
func TestThemeTransitionGolden(t *testing.T) {
	const frames = 30
	tw := transition.PlatformColorsTween(tokens.PlatformLight, tokens.PlatformDark, frames)

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

	// Verify settling at the value level, not just the pixel level: pixel
	// goldens alone could miss a settling bug if the final lerp happens to
	// round to visually identical bytes.
	if got := tw.At(frames); got != tokens.PlatformDark {
		t.Errorf("tween did not settle to target at frame %d: got %+v, want PlatformDark", frames, got)
	}
}

// paintSwatch fills img with the window's own plane, then paints five vertical
// bands: the chrome material, the accent, the emphasized selection, the
// push button's fill and the platform's grid line. Together they carry enough
// contrast to make light, dark and midpoint frames visually distinct in the
// golden PNGs.
func paintSwatch(img *image.NRGBA, colors tokens.PlatformColors) {
	bounds := img.Bounds()
	draw.Draw(img, bounds, &image.Uniform{C: colors.WindowBackground}, image.Point{}, draw.Src)

	bands := []color.NRGBA{
		colors.SidebarMaterial,
		colors.ControlAccent,
		colors.SelectedContentBackground,
		colors.PushButtonFill,
		colors.Grid,
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
