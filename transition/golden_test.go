package transition_test

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vibrantgio/pulse/transition"
	"github.com/vibrantgio/spectrum/tokens"
)

var goldenUpdate = flag.Bool("golden.update", false, "overwrite golden images with current output")

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
			compareGolden(t, tc.name, img)
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

func compareGolden(t *testing.T, name string, got *image.NRGBA) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".png")

	if *goldenUpdate {
		if err := saveNRGBA(path, got); err != nil {
			t.Fatalf("golden: save %s: %v", path, err)
		}
		return
	}

	want, err := loadNRGBA(path)
	if os.IsNotExist(err) {
		t.Fatalf("golden: %s not found; run go test -golden.update to create", path)
	}
	if err != nil {
		t.Fatalf("golden: load %s: %v", path, err)
	}
	// A size change is a failure in its own right, and it has to be caught
	// here: once the bounds differ there is no pixel count to compare, and
	// pixelDiff refuses to invent one.
	if wb, gb := want.Bounds(), got.Bounds(); wb != gb {
		actualPath := strings.TrimSuffix(path, ".png") + ".actual.png"
		_ = saveNRGBA(actualPath, got)
		t.Fatalf("golden: %q: size changed: golden is %dx%d, render is %dx%d (actual saved to %s)",
			name, wb.Dx(), wb.Dy(), gb.Dx(), gb.Dy(), actualPath)
	}
	if n := pixelDiff(want, got); n != 0 {
		actualPath := strings.TrimSuffix(path, ".png") + ".actual.png"
		_ = saveNRGBA(actualPath, got)
		t.Fatalf("golden: %q: %d pixel(s) differ (actual saved to %s)", name, n, actualPath)
	}
}

func saveNRGBA(path string, img *image.NRGBA) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func loadNRGBA(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	decoded, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	if v, ok := decoded.(*image.NRGBA); ok {
		return v, nil
	}
	bounds := decoded.Bounds()
	out := image.NewNRGBA(bounds)
	draw.Draw(out, bounds, decoded, bounds.Min, draw.Src)
	return out, nil
}

// pixelDiff counts the pixels that differ between a and b, which must have equal
// bounds. It panics if they do not.
//
// The panic replaces a returned -1. There is no pixel count to report for two
// images of different shapes, and -1 read as "no difference" to every `n > 0`
// test — which is how a golden whose size had moved compared as a pass, here
// and across the whole organization. A caller for which a size change is a
// real outcome rather than a defect — the stored-golden comparison, and only
// it — must compare Bounds itself before calling.
func pixelDiff(a, b *image.NRGBA) int {
	if a.Bounds() != b.Bounds() {
		panic(fmt.Sprintf("pixelDiff: images must have equal bounds, got %v and %v",
			a.Bounds(), b.Bounds()))
	}
	n := 0
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			n++
		}
	}
	return n
}
