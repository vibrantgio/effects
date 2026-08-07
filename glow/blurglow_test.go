package glow_test

// E4.4 evaluation artifact: the blur-based glow prototype, its falloff
// comparison against the shipped eight-gradient halo, and the cost
// benchmarks behind the decision recorded in the package doc. The
// prototype is test-only on purpose — the evaluation kept the gradient
// path, so nothing here ships as API.

import (
	"flag"
	"image"
	"math"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/vibrantgio/prism/golden"
	"github.com/vibrantgio/pulse/blur"
	glow "github.com/vibrantgio/pulse/glow"
)

// blurglowDump, when set to a directory, makes the falloff-comparison
// test write side-by-side PNGs of the gradient and blur halos there.
var blurglowDump = flag.String("blurglow.dump", "", "directory to write blur-vs-gradient comparison PNGs")

// blurSigma maps the gradient path's Radius to the prototype's blur
// sigma. The gradient fades to zero at exactly Radius; a Gaussian-
// blurred step edge decays to ~2% at 2σ, so σ = Radius/2 matches the
// visible extents.
func blurSigma(radius int) float64 { return float64(radius) / 2 }

// rasterHaloShape fills the halo shape — the bounds rectangle at peak
// alpha — into dst. The colour planes are filled with the halo colour
// over the WHOLE canvas, not just the shape: pulse/blur blurs straight-
// alpha channels independently (its documented translucency caveat),
// so blurring a coloured shape over transparent black would bleed
// black into the halo and dim it to roughly alpha². Keeping the colour
// planes uniform makes the straight-alpha blur premultiplied-correct
// for this single-colour source; only alpha carries the shape.
func rasterHaloShape(dst *image.NRGBA, bounds image.Rectangle, opts glow.Options) {
	inner := opts.Color
	intensity := opts.Intensity
	if intensity > 1 {
		intensity = 1
	}
	inner.A = uint8(float64(opts.Color.A)*intensity + 0.5)
	w := dst.Rect.Dx()
	for y := 0; y < dst.Rect.Dy(); y++ {
		row := dst.Pix[y*dst.Stride : y*dst.Stride+w*4]
		inShape := y >= bounds.Min.Y && y < bounds.Max.Y
		for x := 0; x < w; x++ {
			row[x*4+0] = inner.R
			row[x*4+1] = inner.G
			row[x*4+2] = inner.B
			if inShape && x >= bounds.Min.X && x < bounds.Max.X {
				row[x*4+3] = inner.A
			} else {
				row[x*4+3] = 0
			}
		}
	}
}

// blurHalo is the E4.4 prototype pipeline for one frame: rasterize the
// shape, blur it in place, wrap it as a paint op. The NRGBA buffer is
// caller-owned and reused across frames; the ImageOp conversion
// (NewImageOp copies NRGBA into a fresh premultiplied RGBA) is part of
// the honest per-frame cost, as is the GPU texture upload it implies.
func blurHalo(buf *image.NRGBA, bounds image.Rectangle, opts glow.Options, blurrer *blur.Blurrer) paint.ImageOp {
	rasterHaloShape(buf, bounds, opts)
	blurrer.Gaussian(buf, buf, blurSigma(opts.Radius))
	return paint.NewImageOp(buf)
}

// blurScene composes the same fixture as scene() but with the blur
// prototype in place of glow.Halo: dark backdrop, blurred halo image,
// black foreground rect on top (covering the blur's inward bleed).
func blurScene(imgOp paint.ImageOp, bounds image.Rectangle) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		imgOp.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		paint.FillShape(gtx.Ops, fgColor, clip.Rect(bounds).Op())
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
}

// frac normalizes a rendered red-channel byte to halo coverage in
// [0, 1]: 0 at the dark background, 1 at a fully opaque white halo.
func frac(r uint8) float64 {
	return (float64(r) - float64(bgColor.R)) / (255 - float64(bgColor.R))
}

// edgeProfile samples halo coverage along the outward normal of the
// right bounds edge at mid-height: index d is d pixels outside bounds.
func edgeProfile(img *image.RGBA, bounds image.Rectangle, n int) []float64 {
	y := (bounds.Min.Y + bounds.Max.Y) / 2
	p := make([]float64, n)
	for d := 0; d < n; d++ {
		off := img.PixOffset(bounds.Max.X+d, y)
		p[d] = frac(img.Pix[off])
	}
	return p
}

// cornerProfile samples halo coverage along the 45° diagonal from the
// bottom-right corner: index d is Euclidean distance d from the corner.
func cornerProfile(img *image.RGBA, bounds image.Rectangle, n int) []float64 {
	p := make([]float64, n)
	for d := 0; d < n; d++ {
		step := int(math.Round(float64(d) / math.Sqrt2))
		off := img.PixOffset(bounds.Max.X+step, bounds.Max.Y+step)
		p[d] = frac(img.Pix[off])
	}
	return p
}

// TestBlurGlowFalloffComparison renders the golden fixture through
// both paths and quantifies the falloff differences the E4.4 decision
// rests on (numbers recorded in the package doc):
//
//   - inner edge: the gradient starts at peak alpha flush with bounds;
//     the blur of a step edge starts at ~half, so the blur halo's
//     visible inner rim is roughly half as bright at equal Intensity.
//   - corners: the gradient's diagonal falloff hits zero at Radius/√2
//     (~0.71R) while its edge falloff runs to R — an anisotropic rim
//     with a C0 kink at the tile seams. The blur is smooth and still
//     above background beyond 0.71R on the diagonal.
//   - smoothness: both blur profiles are monotone non-increasing.
func TestBlurGlowFalloffComparison(t *testing.T) {
	opts := glow.Options{Color: haloColor, Radius: haloRadius, Intensity: 1}

	gradImg := golden.Capture(t, canvasSize, scene(haloBounds, opts))
	buf := image.NewNRGBA(image.Rectangle{Max: canvasSize})
	var blurrer blur.Blurrer
	imgOp := blurHalo(buf, haloBounds, opts, &blurrer)
	blurImg := golden.Capture(t, canvasSize, blurScene(imgOp, haloBounds))

	const n = haloRadius + 5
	gradEdge := edgeProfile(gradImg, haloBounds, n)
	gradCorner := cornerProfile(gradImg, haloBounds, n)
	blurEdge := edgeProfile(blurImg, haloBounds, n)
	blurCorner := cornerProfile(blurImg, haloBounds, n)

	t.Logf("halo coverage by distance outside bounds (R=%d, sigma=%.1f):", haloRadius, blurSigma(haloRadius))
	t.Logf("%4s  %-10s %-10s  %-10s %-10s", "d", "grad-edge", "grad-45deg", "blur-edge", "blur-45deg")
	for d := 0; d < n; d += 2 {
		t.Logf("%4d  %-10.3f %-10.3f  %-10.3f %-10.3f", d, gradEdge[d], gradCorner[d], blurEdge[d], blurCorner[d])
	}

	// Inner edge: gradient near peak, blur near half.
	if gradEdge[0] < 0.9 {
		t.Errorf("gradient inner edge coverage = %.3f, want >= 0.9", gradEdge[0])
	}
	if blurEdge[0] > 0.6 || blurEdge[0] < 0.3 {
		t.Errorf("blur inner edge coverage = %.3f, want ~0.5 (step edge halves under blur)", blurEdge[0])
	}

	// Corner anisotropy of the gradient path: at 0.75R along the
	// diagonal the corner tile has already faded out (its far stop is
	// at R/√2 ≈ 0.71R) while the edge at the same distance is still
	// clearly lit — a corner-vs-edge gap the blur does not have.
	d75 := haloRadius * 3 / 4
	if gradCorner[d75] > 0.03 {
		t.Errorf("gradient 45° coverage at 0.75R = %.3f, want ~0 (corner stop at R/√2)", gradCorner[d75])
	}
	if gap := gradEdge[d75] - gradCorner[d75]; gap < 0.3 {
		t.Errorf("gradient corner-vs-edge gap at 0.75R = %.3f, want >= 0.3 (the corner cliff)", gap)
	}
	if gap := math.Abs(blurEdge[d75] - blurCorner[d75]); gap > 0.1 {
		t.Errorf("blur corner-vs-edge gap at 0.75R = %.3f, want < 0.1 (smooth falloff)", gap)
	}

	// Smoothness: blur profiles decay monotonically (1-byte AA slack).
	const slack = 1.5 / 255
	for d := 1; d < n; d++ {
		if blurEdge[d] > blurEdge[d-1]+slack {
			t.Errorf("blur edge profile rises at d=%d: %.3f -> %.3f", d, blurEdge[d-1], blurEdge[d])
		}
		if blurCorner[d] > blurCorner[d-1]+slack {
			t.Errorf("blur 45° profile rises at d=%d: %.3f -> %.3f", d, blurCorner[d-1], blurCorner[d])
		}
	}

	if *blurglowDump != "" {
		dumpComparison(t, gradImg, blurImg)
	}
}

// dumpComparison writes the two fixture renders plus a 3×-parameter
// pair (R=48) where the corner difference is easy to eyeball.
func dumpComparison(t *testing.T, gradImg, blurImg *image.RGBA) {
	t.Helper()
	if err := os.MkdirAll(*blurglowDump, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", *blurglowDump, err)
	}
	save := func(name string, img *image.RGBA) {
		path := filepath.Join(*blurglowDump, name)
		if err := golden.Save(path, img); err != nil {
			t.Fatalf("save %s: %v", path, err)
		}
	}
	save("gradient-halo.png", gradImg)
	save("blur-halo.png", blurImg)

	big := image.Pt(3*canvasW, 3*canvasH)
	bigBounds := image.Rect(3*boundsX0, 3*boundsY0, 3*boundsX1, 3*boundsY1)
	bigOpts := glow.Options{Color: haloColor, Radius: 3 * haloRadius, Intensity: 1}
	save("gradient-halo-3x.png", golden.Capture(t, big, scene(bigBounds, bigOpts)))
	buf := image.NewNRGBA(image.Rectangle{Max: big})
	var blurrer blur.Blurrer
	save("blur-halo-3x.png", golden.Capture(t, big, blurScene(blurHalo(buf, bigBounds, bigOpts, &blurrer), bigBounds)))
}

// ---- cost benchmarks ----
//
// The animated glow is the deciding case: Radius or Intensity change
// every frame, so any (shape params → blurred image) cache misses by
// construction and the full prototype pipeline runs per frame. The
// gradient path's per-frame cost is op construction alone.

// BenchmarkGradientHaloOps is the shipped path's entire per-frame CPU
// cost: recording eight gradient tiles into an op list.
func BenchmarkGradientHaloOps(b *testing.B) {
	var ops op.Ops
	gtx := layout.Context{Constraints: layout.Exact(canvasSize), Ops: &ops}
	opts := glow.Options{Color: haloColor, Radius: haloRadius, Intensity: 1}
	b.ReportAllocs()
	for b.Loop() {
		ops.Reset()
		glow.Halo(gtx, haloBounds, opts)
	}
}

// benchmarkBlurHaloRaster measures the prototype's per-frame CPU cost
// for a bw×bh px glow shape with the given radius: rasterize + blur +
// NewImageOp (which copies NRGBA into a fresh premultiplied RGBA). The
// GPU texture upload each fresh ImageOp implies at draw time is on top
// of this and not measured here.
func benchmarkBlurHaloRaster(b *testing.B, bw, bh, radius int) {
	canvas := image.Pt(bw+2*radius, bh+2*radius)
	bounds := image.Rect(radius, radius, radius+bw, radius+bh)
	opts := glow.Options{Color: haloColor, Radius: radius, Intensity: 1}
	buf := image.NewNRGBA(image.Rectangle{Max: canvas})
	var blurrer blur.Blurrer
	b.ReportAllocs()
	for b.Loop() {
		blurHalo(buf, bounds, opts, &blurrer)
	}
}

// Button-sized glow: 100×40 shape, radius 16 (canvas 132×72).
func BenchmarkBlurHaloRaster132x72(b *testing.B) { benchmarkBlurHaloRaster(b, 100, 40, 16) }

// Card-sized glow: 300×96 shape, radius 24 (canvas 348×144).
func BenchmarkBlurHaloRaster348x144(b *testing.B) { benchmarkBlurHaloRaster(b, 300, 96, 24) }

// BenchmarkBlurHaloBackdrop132x72 is the general-shape variant of the
// prototype: instead of a CPU raster of a rectangle, the shape is
// rendered by the GPU through blur.Backdrop (headless render +
// readback + blur), which is what an arbitrary clip-path glow would
// need. Divisor 1: at button sizes there is nothing to downscale.
func BenchmarkBlurHaloBackdrop132x72(b *testing.B) {
	if !blur.Available() {
		b.Skip("headless rendering not supported")
	}
	canvas := image.Pt(132, 72)
	bounds := image.Rect(16, 16, 116, 56)
	inner := haloColor
	layer := func(ops *op.Ops) {
		paint.FillShape(ops, inner, clip.Rect(bounds).Op())
	}
	var bd blur.Backdrop
	defer bd.Release()
	b.ReportAllocs()
	for b.Loop() {
		if err := bd.Update(layer, canvas, blurSigma(16), blur.WithDivisor(1)); err != nil {
			b.Fatal(err)
		}
	}
}

// benchmarkAnimatedFrame is the end-to-end per-frame comparison: build
// the scene's ops the way an animating widget would every frame and
// render them through a headless window. For the blur path that
// includes raster + blur + NewImageOp + the texture upload of the
// fresh image; for the gradient path just op recording. Headless
// Frame() timing stands in for the compositor's — same GPU work,
// minus presentation.
func benchmarkAnimatedFrame(b *testing.B, build func(ops *op.Ops)) {
	w, err := headless.NewWindow(canvasW, canvasH)
	if err != nil {
		b.Skipf("headless rendering not supported: %v", err)
	}
	defer w.Release()
	var ops op.Ops
	b.ReportAllocs()
	for b.Loop() {
		ops.Reset()
		build(&ops)
		if err := w.Frame(&ops); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGradientHaloAnimatedFrame(b *testing.B) {
	opts := glow.Options{Color: haloColor, Radius: haloRadius, Intensity: 1}
	benchmarkAnimatedFrame(b, func(ops *op.Ops) {
		gtx := layout.Context{Constraints: layout.Exact(canvasSize), Ops: ops}
		scene(haloBounds, opts)(gtx)
	})
}

func BenchmarkBlurHaloAnimatedFrame(b *testing.B) {
	opts := glow.Options{Color: haloColor, Radius: haloRadius, Intensity: 1}
	buf := image.NewNRGBA(image.Rectangle{Max: canvasSize})
	var blurrer blur.Blurrer
	benchmarkAnimatedFrame(b, func(ops *op.Ops) {
		gtx := layout.Context{Constraints: layout.Exact(canvasSize), Ops: ops}
		blurScene(blurHalo(buf, haloBounds, opts, &blurrer), haloBounds)(gtx)
	})
}
