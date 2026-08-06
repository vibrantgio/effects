package depth_test

import (
	"flag"
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
	"gioui.org/unit"

	depth "github.com/vibrantgio/pulse/depth"
	"github.com/vibrantgio/spectrum/tokens"
)

// goldenUpdate, when set, overwrites stored goldens with the live
// render output. Mirrors pulse/glow's harness; inlined here because
// prism/internal/golden lives in a separate module tree and is not
// importable from pulse.
var goldenUpdate = flag.Bool("golden.update", false, "overwrite golden images with current output")

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
	gtx := layout.Context{
		Constraints: layout.Exact(size),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Ops:         &ops,
	}
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
	if n := pixelDiff(stored, img); n > 0 {
		actualPath := strings.TrimSuffix(path, ".png") + ".actual.png"
		_ = saveImage(actualPath, img)
		t.Fatalf("%q: %d pixel(s) differ (actual saved to %s)", name, n, actualPath)
	}
}

func pixelDiff(a, b *image.RGBA) int {
	if a.Bounds() != b.Bounds() {
		return -1
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
	// headless.Screenshot writes straight-alpha into *image.RGBA; reinterpret
	// as NRGBA so png.Encode stores the bytes verbatim instead of
	// premultiplying edge alpha.
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

// TestShadowGoldens exercises the G3.3b Measurable: golden-image tests
// at all six elevation levels (level-0 through level-5).
func TestShadowGoldens(t *testing.T) {
	cases := []struct {
		name  string
		level tokens.ElevationLevel
	}{
		{"level-0", tokens.Level0},
		{"level-1", tokens.Level1},
		{"level-2", tokens.Level2},
		{"level-3", tokens.Level3},
		{"level-4", tokens.Level4},
		{"level-5", tokens.Level5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			renderGolden(t, tc.name, canvasSize, scene(tc.level))
		})
	}
}

// TestShadowRoundedGolden exercises the FX.3 fix: a rounded surface
// over a shadow rounded to the same radius. Before the fix the
// interior fill was a hard clip.Rect, and this scene showed its
// square corners through the foreground's rounding as four dark
// wedges — the exact pixels this golden pins.
func TestShadowRoundedGolden(t *testing.T) {
	renderGolden(t, "level-3-rounded", canvasSize, roundedScene(tokens.Level3, 12, 1))
}

// TestShadowOpacity asserts the opacity parameter scales the ramp:
// 0 paints nothing at all, and a half-strength shadow differs from a
// full-strength one.
func TestShadowOpacity(t *testing.T) {
	bg := capture(t, canvasSize, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	if bg == nil {
		return
	}
	shadowAt := func(opacity float32) *image.RGBA {
		return capture(t, canvasSize, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
			depth.Shadow(gtx, boundsRect, tokens.Level3, 12, opacity)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
	}
	zero, half, full := shadowAt(0), shadowAt(0.5), shadowAt(1)
	if zero == nil || half == nil || full == nil {
		return
	}
	if n := pixelDiff(bg, zero); n != 0 {
		t.Errorf("opacity 0 painted %d pixel(s); want a no-op", n)
	}
	if n := pixelDiff(half, full); n == 0 {
		t.Errorf("opacity 0.5 renders identically to opacity 1; want a lighter shadow")
	}
	if n := pixelDiff(bg, half); n == 0 {
		t.Errorf("opacity 0.5 painted nothing; want a visible shadow")
	}
}

// TestShadowAdjacentLevelsDiffer asserts that each adjacent pair of
// elevation levels produces a visibly different render. Catches
// regressions where the level-to-geometry mapping silently rounds
// adjacent levels into the same offset/extent — which would let "six
// different" goldens drift toward the same byte sequence over time.
func TestShadowAdjacentLevelsDiffer(t *testing.T) {
	levels := []tokens.ElevationLevel{
		tokens.Level0, tokens.Level1, tokens.Level2,
		tokens.Level3, tokens.Level4, tokens.Level5,
	}
	imgs := make([]*image.RGBA, len(levels))
	for i, l := range levels {
		imgs[i] = capture(t, canvasSize, scene(l))
		if imgs[i] == nil {
			return
		}
	}
	for i := 0; i < len(imgs)-1; i++ {
		if n := pixelDiff(imgs[i], imgs[i+1]); n == 0 {
			t.Errorf("level-%d and level-%d render identically; expected adjacent levels to differ",
				i, i+1)
		}
	}
}
