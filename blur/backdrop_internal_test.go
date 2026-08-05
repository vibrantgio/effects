package blur

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// requireHeadless skips the test when headless GPU rendering is not
// available on this platform, the same policy as the golden harness.
func requireHeadless(t testing.TB) {
	t.Helper()
	if !Available() {
		t.Skipf("blur: headless rendering not supported on this platform")
	}
}

// splitLayer returns a layer that fills the left half of fullSize with
// left and the right half with right, recorded at full logical size.
func splitLayer(fullSize image.Point, left, right color.NRGBA) func(ops *op.Ops) {
	return func(ops *op.Ops) {
		mid := fullSize.X / 2
		l := clip.Rect{Max: image.Pt(mid, fullSize.Y)}.Push(ops)
		paint.ColorOp{Color: left}.Add(ops)
		paint.PaintOp{}.Add(ops)
		l.Pop()
		r := clip.Rect{Min: image.Pt(mid, 0), Max: fullSize}.Push(ops)
		paint.ColorOp{Color: right}.Add(ops)
		paint.PaintOp{}.Add(ops)
		r.Pop()
	}
}

// rgbaAt returns the R and A bytes of the pixel at (x, y) in im.
func rgbaAt(im *image.RGBA, x, y int) (r, a uint8) {
	off := im.PixOffset(x, y)
	return im.Pix[off], im.Pix[off+3]
}

// TestBackdropPipeline runs the full pipeline on a two-colour split
// layer and asserts the readback really was rendered, reduced and
// blurred: solid colours far from the seam, a gradient across it.
func TestBackdropPipeline(t *testing.T) {
	requireHeadless(t)
	var b Backdrop
	defer b.Release()

	full := image.Pt(128, 96)
	red := color.NRGBA{R: 255, A: 255}
	blue := color.NRGBA{B: 255, A: 255}
	if err := b.Update(splitLayer(full, red, blue), full, 8); err != nil {
		t.Fatalf("Update: %v", err)
	}
	imgOp, ok := b.Op()
	if !ok {
		t.Fatalf("Op reports ok == false after a successful Update")
	}
	// sigma 8 → DefaultDivisor 4 → 32×24.
	if got, want := imgOp.Size(), image.Pt(32, 24); got != want {
		t.Fatalf("op size = %v, want %v", got, want)
	}

	y := 12 // middle row of the 32×24 readback; the seam is at x = 16
	rL, aL := rgbaAt(b.frame, 2, y)
	rM, aM := rgbaAt(b.frame, 16, y)
	rR, aR := rgbaAt(b.frame, 29, y)
	if aL != 255 || aM != 255 || aR != 255 {
		t.Fatalf("opaque layer read back translucent: alpha %d/%d/%d", aL, aM, aR)
	}
	if rL < 200 {
		t.Fatalf("left of seam R = %d, want near-red (>= 200)", rL)
	}
	if rR > 60 {
		t.Fatalf("right of seam R = %d, want near-blue (<= 60)", rR)
	}
	if rM <= rR+30 || rM >= rL-30 {
		t.Fatalf("seam R = %d not a blend of left %d and right %d", rM, rL, rR)
	}
	// The blur turns the hard seam into a monotone ramp.
	prev, _ := rgbaAt(b.frame, 10, y)
	for x := 11; x <= 22; x++ {
		cur, _ := rgbaAt(b.frame, x, y)
		if cur > prev {
			t.Fatalf("R not monotone across the seam: rises %d -> %d at x=%d", prev, cur, x)
		}
		prev = cur
	}
}

// TestBackdropWindowReuse pins the reuse contract: same reduced size
// reuses the headless window, a size or divisor change reallocates.
func TestBackdropWindowReuse(t *testing.T) {
	requireHeadless(t)
	var b Backdrop
	defer b.Release()

	full := image.Pt(128, 96)
	layer := splitLayer(full, color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255})
	mustUpdate := func(fullSize image.Point, sigma float64, opts ...Option) {
		t.Helper()
		if err := b.Update(layer, fullSize, sigma, opts...); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}

	mustUpdate(full, 8)
	mustUpdate(full, 8)
	if b.winAllocs != 1 {
		t.Fatalf("same size twice: %d window allocations, want 1", b.winAllocs)
	}
	mustUpdate(image.Pt(256, 96), 8) // resize → realloc
	if b.winAllocs != 2 {
		t.Fatalf("after resize: %d window allocations, want 2", b.winAllocs)
	}
	mustUpdate(image.Pt(256, 96), 16) // sigma change moves the divisor → realloc
	if b.winAllocs != 3 {
		t.Fatalf("after divisor change: %d window allocations, want 3", b.winAllocs)
	}
	mustUpdate(image.Pt(256, 96), 16)
	if b.winAllocs != 3 {
		t.Fatalf("steady state reallocated: %d window allocations, want 3", b.winAllocs)
	}
}

// TestBackdropDivisor pins that sigma chooses the render resolution
// via DefaultDivisor and that WithDivisor overrides it.
func TestBackdropDivisor(t *testing.T) {
	requireHeadless(t)
	var b Backdrop
	defer b.Release()

	full := image.Pt(100, 80)
	layer := splitLayer(full, color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255})

	if err := b.Update(layer, full, 8); err != nil { // DefaultDivisor(8) = 4
		t.Fatalf("Update: %v", err)
	}
	op, _ := b.Op()
	if got, want := op.Size(), image.Pt(25, 20); got != want {
		t.Fatalf("sigma 8 op size = %v, want ceil(100×80 / 4) = %v", got, want)
	}

	if err := b.Update(layer, full, 8, WithDivisor(2)); err != nil {
		t.Fatalf("Update: %v", err)
	}
	op, _ = b.Op()
	if got, want := op.Size(), image.Pt(50, 40); got != want {
		t.Fatalf("WithDivisor(2) op size = %v, want %v", got, want)
	}
}

// TestBackdropInvalid exercises the fallback contract without forcing
// platform unavailability: a nil Backdrop, a zero Backdrop before any
// Update, and a Backdrop after a failed Update all report ok == false
// and never panic.
func TestBackdropInvalid(t *testing.T) {
	var nilB *Backdrop
	if _, ok := nilB.Op(); ok {
		t.Fatalf("nil Backdrop reports a valid op")
	}
	if err := nilB.Update(func(*op.Ops) {}, image.Pt(10, 10), 4); err == nil {
		t.Fatalf("Update on a nil Backdrop returned no error")
	}
	nilB.Release() // must not panic

	var b Backdrop
	if _, ok := b.Op(); ok {
		t.Fatalf("zero Backdrop reports a valid op before any Update")
	}
	if err := b.Update(nil, image.Pt(10, 10), 4); err == nil {
		t.Fatalf("Update with a nil layer returned no error")
	}
	if err := b.Update(func(*op.Ops) {}, image.Pt(0, 0), 4); err == nil {
		t.Fatalf("Update with an empty size returned no error")
	}
	if _, ok := b.Op(); ok {
		t.Fatalf("failed Update left the Backdrop valid")
	}

	requireHeadless(t)
	layer := splitLayer(image.Pt(32, 32), color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255})
	if err := b.Update(layer, image.Pt(32, 32), 4); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, ok := b.Op(); !ok {
		t.Fatalf("successful Update did not validate the Backdrop")
	}
	if err := b.Update(nil, image.Pt(32, 32), 4); err == nil {
		t.Fatalf("Update with a nil layer returned no error")
	}
	if _, ok := b.Op(); ok {
		t.Fatalf("failed Update did not invalidate the previous op")
	}
	b.Release()
	if _, ok := b.Op(); ok {
		t.Fatalf("Release left the Backdrop valid")
	}
}

// TestFallbackOp renders the fallback op headlessly and asserts it
// paints the flat tint edge to edge at an arbitrary size — the
// documented stand-in when Op reports ok == false.
func TestFallbackOp(t *testing.T) {
	requireHeadless(t)
	tint := color.NRGBA{R: 32, G: 32, B: 48, A: 255}
	w, err := headless.NewWindow(16, 12)
	if err != nil {
		t.Fatalf("headless window: %v", err)
	}
	defer w.Release()

	var ops op.Ops
	c := clip.Rect{Max: image.Pt(16, 12)}.Push(&ops)
	FallbackOp(tint).Add(&ops)
	paint.PaintOp{}.Add(&ops)
	c.Pop()
	if err := w.Frame(&ops); err != nil {
		t.Fatalf("Frame: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 16, 12))
	if err := w.Screenshot(img); err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	for i := 0; i < len(img.Pix); i += 4 {
		got := color.NRGBA{R: img.Pix[i], G: img.Pix[i+1], B: img.Pix[i+2], A: img.Pix[i+3]}
		if got != tint {
			t.Fatalf("fallback pixel %d = %v, want %v", i/4, got, tint)
		}
	}
}

// Benchmarks of the assembled pipeline — record ops, headless render,
// readback, blur — for a 1440×900 backdrop at each divisor of the
// G-E4 table. The look is constant at sigma 8 (full-resolution
// pixels); the divisor sets the render resolution, so the blur runs at
// sigma 8/d exactly as in E4.1's blur-only benchmarks. ÷4 is what
// DefaultDivisor(8) picks on its own.
func benchmarkBackdrop(b *testing.B, div int) {
	requireHeadless(b)
	var bd Backdrop
	defer bd.Release()
	full := image.Pt(1440, 900)
	layer := splitLayer(full, color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255})
	if err := bd.Update(layer, full, 8, WithDivisor(div)); err != nil {
		b.Fatalf("Update: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := bd.Update(layer, full, 8, WithDivisor(div)); err != nil {
			b.Fatalf("Update: %v", err)
		}
	}
}

func BenchmarkBackdropDiv1(b *testing.B) { benchmarkBackdrop(b, 1) }
func BenchmarkBackdropDiv2(b *testing.B) { benchmarkBackdrop(b, 2) }
func BenchmarkBackdropDiv4(b *testing.B) { benchmarkBackdrop(b, 4) }
func BenchmarkBackdropDiv8(b *testing.B) { benchmarkBackdrop(b, 8) }
