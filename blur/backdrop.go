package blur

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"sync"

	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/op"
	"gioui.org/op/paint"
)

// A Backdrop is the headless backdrop-blur pipeline: it renders
// a caller-supplied layer into an offscreen GPU window at a reduced
// resolution, reads the pixels back, blurs them on the CPU, and serves
// the result as a [paint.ImageOp] for the caller to paint stretched to
// full size. It is how a blurred "behind the dialog" surface is built
// from Gio's own primitives, which include no blur.
//
// The zero value is ready to use. A Backdrop owns a [headless.Window]
// allocated on first [Backdrop.Update] and reused across calls —
// window creation costs ~1.1 ms, so it is reallocated only when the
// reduced resolution changes. Call [Backdrop.Release] when the
// Backdrop is retired. A Backdrop must not be used concurrently.
//
// # Rendering at reduced resolution
//
// Callers ask for a look, not a resolution: Update takes the blur
// sigma in full-resolution pixels and derives the render divisor from
// it with [DefaultDivisor] (overridable with [WithDivisor]). The blur
// destroys any detail finer than roughly sigma pixels, so rendering at
// 1/divisor resolution loses nothing visible while cutting render,
// readback and blur work by divisor².
//
// The layer function records its ops at full logical size — exactly
// what it would draw onscreen — and Update replays them under an
// affine scale into the reduced-size window. That keeps callers
// honest: the layer never needs to know the divisor exists.
//
// # Painting the result
//
// [Backdrop.Op] returns the current op at the reduced size (its Size
// is ceil(fullSize/divisor)); paint it through an affine scale into
// the full-size rect and Gio's default FilterLinear upscales it on the
// GPU at draw time, the same convention as [Cache]. The op's backing
// pixels are reused by the next Update, so always paint the op most
// recently returned; a stale op held across an Update may show the new
// content.
//
// # Refresh policy
//
// Update runs the full render–readback–blur pipeline synchronously and
// stalls the calling goroutine — the Gio events thread, for a caller
// updating from a frame handler — for the full-pipeline cost in the
// package doc's measured table (29 ms at ÷1 down to 1.1 ms at ÷8 for a
// 1440×900 backdrop; a 60 fps frame budget is 16.7 ms). It must
// therefore be driven by content change, never by every frame: call
// Update when what is behind the blur has actually changed — a scroll
// settled, a dialog opened over new content, the wallpaper changed —
// and paint the cached op on every frame in between. Op is free.
//
// # Fallback when headless rendering is unavailable
//
// Headless GPU rendering is not available on every platform. Update
// never panics for that: it returns the error, the Backdrop stays
// invalid, and Op reports ok == false — including on a nil *Backdrop.
// On ok == false the caller paints a flat tinted surface instead, its
// scrim colour, for which [FallbackOp] is a ready-made op. Callers
// that want to decide up front can consult [Available].
//
// # Alpha
//
// The window is cleared to transparent and [headless.Window.Screenshot]
// fills the readback *image.RGBA with straight-alpha values, so viewing
// the buffer as NRGBA for the blur is exact, not an approximation. The returned
// ImageOp, however, wraps the buffer as *image.RGBA, which Gio treats
// as premultiplied. For an opaque backdrop — a layer that paints its
// background edge to edge, the intended use — the two conventions
// coincide and everything is exact; a layer that leaves uncovered or
// translucent pixels will see them composited slightly darker.
type Backdrop struct {
	win     *headless.Window
	winSize image.Point // reduced size the window was allocated at
	ops     op.Ops
	frame   *image.RGBA // reused readback buffer, blurred in place
	blurrer Blurrer
	op      paint.ImageOp
	valid   bool

	// winAllocs counts headless-window allocations; test
	// instrumentation for the reuse contract.
	winAllocs int
}

// Update re-renders the backdrop: it replays layer — recorded at
// fullSize logical coordinates — into the reduced-resolution offscreen
// window, reads the pixels back, blurs them by a Gaussian of standard
// deviation sigma (in full-resolution pixels), and makes the result
// available from [Backdrop.Op].
//
// The divisor defaults from sigma via [DefaultDivisor]; [WithDivisor]
// overrides it. The offscreen window is reused across calls and
// reallocated only when the reduced size changes.
//
// Update is synchronous and costs milliseconds (see the refresh policy
// on [Backdrop]): call it when the content behind the blur changes,
// never per frame. Any error — including headless rendering being
// unavailable on the platform — leaves the Backdrop invalid until a
// later Update succeeds; it never panics.
func (b *Backdrop) Update(layer func(ops *op.Ops), fullSize image.Point, sigma float64, opts ...Option) error {
	if b == nil {
		return errors.New("blur: Update on a nil Backdrop")
	}
	b.valid = false
	if layer == nil {
		return errors.New("blur: Update with a nil layer")
	}
	if fullSize.X <= 0 || fullSize.Y <= 0 {
		return fmt.Errorf("blur: Update with empty size %v", fullSize)
	}

	var o options
	for _, opt := range opts {
		opt(&o)
	}
	d := o.divisor
	if d < 1 {
		d = DefaultDivisor(sigma)
	}
	reduced := image.Point{X: (fullSize.X + d - 1) / d, Y: (fullSize.Y + d - 1) / d}

	if b.win == nil || b.winSize != reduced {
		if b.win != nil {
			b.win.Release()
			b.win = nil
		}
		w, err := headless.NewWindow(reduced.X, reduced.Y)
		if err != nil {
			return fmt.Errorf("blur: headless rendering unavailable: %w", err)
		}
		b.win = w
		b.winSize = reduced
		b.winAllocs++
	}

	b.ops.Reset()
	scale := f32.Affine2D{}.Scale(f32.Point{}, f32.Point{
		X: float32(reduced.X) / float32(fullSize.X),
		Y: float32(reduced.Y) / float32(fullSize.Y),
	})
	tr := op.Affine(scale).Push(&b.ops)
	layer(&b.ops)
	tr.Pop()

	if err := b.win.Frame(&b.ops); err != nil {
		return fmt.Errorf("blur: headless frame: %w", err)
	}
	if b.frame == nil || b.frame.Rect.Size() != reduced {
		b.frame = image.NewRGBA(image.Rectangle{Max: reduced})
	}
	if err := b.win.Screenshot(b.frame); err != nil {
		return fmt.Errorf("blur: readback: %w", err)
	}

	// Screenshot yields straight-alpha bytes in an RGBA struct; the
	// NRGBA view shares the buffer, so the blur runs in place with no
	// copy. Sigma scales with the resolution.
	view := &image.NRGBA{Pix: b.frame.Pix, Stride: b.frame.Stride, Rect: b.frame.Rect}
	b.blurrer.Gaussian(view, view, sigma/float64(d))

	b.op = paint.NewImageOp(b.frame)
	b.valid = true
	return nil
}

// Op returns the current backdrop op and whether it is valid. The op
// covers the reduced size; paint it scaled up into the full-size rect
// (see [Backdrop]). ok is false — on a nil Backdrop, before the first
// successful [Backdrop.Update], or after a failed one — and then the
// caller paints its fallback tint instead, e.g. [FallbackOp].
func (b *Backdrop) Op() (_ paint.ImageOp, ok bool) {
	if b == nil || !b.valid {
		return paint.ImageOp{}, false
	}
	return b.op, true
}

// Release frees the offscreen GPU window and invalidates the Backdrop.
// The Backdrop remains usable: the next [Backdrop.Update] allocates a
// fresh window. Release on a nil Backdrop is a no-op.
func (b *Backdrop) Release() {
	if b == nil {
		return
	}
	if b.win != nil {
		b.win.Release()
		b.win = nil
	}
	b.valid = false
}

// FallbackOp returns a uniform [paint.ImageOp] of the given tint: the
// documented fallback surface for when [Backdrop.Op] reports ok ==
// false. A uniform op has no backing texture and paints the current
// clip like a colour fill, so it stands in for the blurred backdrop at
// any size.
func FallbackOp(tint color.NRGBA) paint.ImageOp {
	return paint.NewImageOp(image.NewUniform(tint))
}

var (
	availableOnce sync.Once
	available     bool
)

// Available reports whether headless GPU rendering — and with it the
// [Backdrop] pipeline — works on this platform. The first call probes
// by allocating and releasing a 1×1 offscreen window (~1 ms); the
// result is cached. A Backdrop remains safe to use either way: when
// Available is false, [Backdrop.Update] returns errors and
// [Backdrop.Op] keeps reporting ok == false.
func Available() bool {
	availableOnce.Do(func() {
		w, err := headless.NewWindow(1, 1)
		if err != nil {
			return
		}
		w.Release()
		available = true
	})
	return available
}
