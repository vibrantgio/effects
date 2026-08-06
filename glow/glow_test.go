package glow_test

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	glow "github.com/vibrantgio/pulse/glow"
)

// goldenUpdate, when set, overwrites stored goldens with the live
// render output. Mirrors the convention used by prism/internal/golden;
// inlined here because that package is internal to the prism module
// tree and not importable from pulse.
var goldenUpdate = flag.Bool("golden.update", false, "overwrite golden images with current output")

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

// ---- golden harness (inlined) ----

func capture(t *testing.T, size image.Point, draw layout.Widget) *image.RGBA {
	t.Helper()
	w, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		t.Skipf("headless rendering not supported: %v", err)
		return nil
	}
	defer w.Release()

	var ops op.Ops
	gtx := layout.Context{Constraints: layout.Exact(size), Ops: &ops}
	draw(gtx)
	if err := w.Frame(&ops); err != nil {
		t.Fatalf("Frame: %v", err)
	}
	img := image.NewRGBA(image.Rectangle{Max: size})
	if err := w.Screenshot(img); err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	return img
}

func renderGolden(t *testing.T, name string, size image.Point, draw layout.Widget) {
	t.Helper()
	img := capture(t, size, draw)
	if img == nil {
		return
	}
	path := filepath.Join("testdata", "golden", name+".png")

	if *goldenUpdate {
		if err := saveImage(path, img); err != nil {
			t.Fatalf("save %s: %v", path, err)
		}
		return
	}

	stored, err := loadImage(path)
	if os.IsNotExist(err) {
		t.Fatalf("%s not found; run go test -golden.update to create", path)
		return
	}
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
		return
	}
	// A size change is a failure in its own right, and it has to be caught
	// here: once the bounds differ there is no pixel count to compare, and
	// pixelDiff refuses to invent one.
	if sb, ib := stored.Bounds(), img.Bounds(); sb != ib {
		actualPath := strings.TrimSuffix(path, ".png") + ".actual.png"
		_ = saveImage(actualPath, img)
		t.Fatalf("%q: size changed: golden is %dx%d, render is %dx%d (actual saved to %s)",
			name, sb.Dx(), sb.Dy(), ib.Dx(), ib.Dy(), actualPath)
	}
	if n := pixelDiff(stored, img); n > 0 {
		actualPath := strings.TrimSuffix(path, ".png") + ".actual.png"
		_ = saveImage(actualPath, img)
		t.Fatalf("%q: %d pixel(s) differ (actual saved to %s)", name, n, actualPath)
	}
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
func pixelDiff(a, b *image.RGBA) int {
	if a.Bounds() != b.Bounds() {
		panic(fmt.Sprintf("pixelDiff: images must have equal bounds, got %v and %v",
			a.Bounds(), b.Bounds()))
	}
	bounds := a.Bounds()
	n := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			off := (y-bounds.Min.Y)*a.Stride + (x-bounds.Min.X)*4
			if a.Pix[off] != b.Pix[off] ||
				a.Pix[off+1] != b.Pix[off+1] ||
				a.Pix[off+2] != b.Pix[off+2] ||
				a.Pix[off+3] != b.Pix[off+3] {
				n++
			}
		}
	}
	return n
}

func saveImage(path string, img *image.RGBA) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	// headless.Screenshot writes straight-alpha into *image.RGBA; reinterpret as
	// NRGBA so png.Encode stores the bytes verbatim instead of premultiplying
	// edge alpha.
	nrgba := &image.NRGBA{Pix: img.Pix, Stride: img.Stride, Rect: img.Rect}
	return png.Encode(f, nrgba)
}

func loadImage(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	decoded, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	switch v := decoded.(type) {
	case *image.RGBA:
		return v, nil
	case *image.NRGBA:
		// NRGBA Pix bytes share their layout with RGBA; reinterpret in place
		// so the saved straight-alpha values round-trip without conversion.
		return &image.RGBA{Pix: v.Pix, Stride: v.Stride, Rect: v.Rect}, nil
	default:
		bounds := decoded.Bounds()
		rgba := image.NewRGBA(bounds)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				rgba.Set(x, y, decoded.At(x, y))
			}
		}
		return rgba, nil
	}
}

// ---- tests ----

// TestHaloGoldens exercises the G3.3a Measurable: golden-image tests at
// four intensities. intensity-zero is the no-halo baseline (proves the
// halo path is opt-in); the remaining three cover the spectrum.
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
			renderGolden(t, tc.name, canvasSize, scene(haloBounds, opts))
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
		return capture(t, canvasSize, scene(haloBounds, glow.Options{
			Color: haloColor, Radius: haloRadius, Intensity: intensity,
		}))
	}
	zero, low, mid, high := cap(0), cap(0.25), cap(0.5), cap(1.0)
	if zero == nil || low == nil || mid == nil || high == nil {
		return
	}
	pairs := []struct {
		a, b         *image.RGBA
		nameA, nameB string
	}{
		{zero, low, "zero", "low"},
		{low, mid, "low", "mid"},
		{mid, high, "mid", "high"},
	}
	for _, p := range pairs {
		if n := pixelDiff(p.a, p.b); n == 0 {
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
	withHalo := capture(t, canvasSize, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		glow.Halo(gtx, haloBounds, glow.Options{
			Color: haloColor, Radius: haloRadius, Intensity: 1,
		})
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	noHalo := capture(t, canvasSize, bgOnly)
	if withHalo == nil || noHalo == nil {
		return
	}
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
	bg := capture(t, canvasSize, bgOnly)
	zeroR := capture(t, canvasSize, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		glow.Halo(gtx, haloBounds, glow.Options{
			Color: haloColor, Radius: 0, Intensity: 1,
		})
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	if bg == nil || zeroR == nil {
		return
	}
	if n := pixelDiff(bg, zeroR); n != 0 {
		t.Errorf("Halo with Radius=0 should be a no-op; %d pixels differ", n)
	}
}

// TestHaloNoOpAtZeroIntensity asserts Halo with Intensity=0 is a no-op.
func TestHaloNoOpAtZeroIntensity(t *testing.T) {
	bg := capture(t, canvasSize, bgOnly)
	zeroI := capture(t, canvasSize, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		glow.Halo(gtx, haloBounds, glow.Options{
			Color: haloColor, Radius: haloRadius, Intensity: 0,
		})
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	if bg == nil || zeroI == nil {
		return
	}
	if n := pixelDiff(bg, zeroI); n != 0 {
		t.Errorf("Halo with Intensity=0 should be a no-op; %d pixels differ", n)
	}
}

// TestHaloIntensityClamps asserts Intensity > 1 is clamped, not
// extrapolated: Intensity=2 must render byte-identical to Intensity=1.
func TestHaloIntensityClamps(t *testing.T) {
	one := capture(t, canvasSize, scene(haloBounds, glow.Options{
		Color: haloColor, Radius: haloRadius, Intensity: 1,
	}))
	two := capture(t, canvasSize, scene(haloBounds, glow.Options{
		Color: haloColor, Radius: haloRadius, Intensity: 2,
	}))
	if one == nil || two == nil {
		return
	}
	if n := pixelDiff(one, two); n != 0 {
		t.Errorf("Intensity > 1 should clamp to 1; %d pixels differ between Intensity=1 and Intensity=2", n)
	}
}
