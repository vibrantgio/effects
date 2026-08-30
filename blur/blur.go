// Package blur implements a fast CPU Gaussian-blur approximation over
// [image.NRGBA] for the backdrop-blur pipeline: Gio has no blur
// primitive, so the backdrop is rendered offscreen, read back, blurred
// here, and painted as an ImageOp.
//
// # Method
//
// Three successive box blurs approximate a true Gaussian to within a
// few percent, and a box blur is separable into a horizontal and a
// vertical pass, each a sliding-window running sum that costs O(1) per
// pixel regardless of radius. The three box sizes for a given sigma
// come from the canonical W3C/Kovesi boxesForGauss(sigma, 3) formula.
// Passes are parallelised across [runtime.NumCPU] goroutines: rows are
// chunked for the horizontal pass, columns for the vertical.
//
// # Edges
//
// The edge policy is clamp-extend: the sliding window reads the
// nearest border pixel where it overhangs the image. Nothing wraps and
// nothing averages in zeros, so a uniform input stays byte-exact
// uniform right up to the edge, and the mean of an arbitrary input is
// preserved to within rounding.
//
// # Alpha
//
// NRGBA is non-premultiplied; the alpha channel is blurred exactly
// like the colour channels. That is correct for opaque backdrop
// readbacks but not premultiplied-correct for translucent sources,
// where colour would bleed from fully transparent pixels.
// Premultiplied correctness for translucent sources is out of scope.
//
// # Allocation
//
// The per-frame cost that matters at 60 fps is image-sized
// allocation. [Gaussian] is the convenient form and allocates its
// result and scratch every call; a [Blurrer] holds the scratch image
// across calls and writes into a caller-provided destination, so a
// steady-state frame loop allocates only small per-call worker state
// (goroutine bookkeeping and per-chunk column sums, ~70 KB at
// 1440×900) and never an image-sized buffer.
//
// # Measured cost
//
// Blur-only cost of (*Blurrer).Gaussian and full [Backdrop] pipeline
// cost — record ops, headless render, readback, blur — for a 1440×900
// backdrop at each divisor, measured on a ten-core Apple Silicon
// machine (go test -bench, darwin/arm64). Sigma follows the backdrop
// model: the look is σ=8 in full-resolution pixels, so the blur runs
// at σ=8/divisor on the reduced pixels:
//
//	÷1  1440×900  σ=8   blur  5.8  ms   pipeline 29.0 ms
//	÷2   720×450  σ=4   blur  1.8  ms   pipeline  8.1 ms
//	÷4   360×225  σ=2   blur  0.74 ms   pipeline  2.9 ms
//	÷8   180×112  σ=1   blur  0.35 ms   pipeline  1.1 ms
//
// Parallelism buys ~5× over the serial path at 1440×900; the smaller
// sizes are increasingly barrier-bound and fall short of linear
// scaling. At the working configuration (÷4) the blur itself is ~4% of
// a 16.7 ms frame budget. A heavier layer barely moves the pipeline
// totals (a fresh full-resolution texture upload per update adds
// ~0.5 ms at ÷1); synchronous GPU readback dominates, which is why the
// resolution divisor is the lever that matters.
package blur

import (
	"image"
	"math"
	"runtime"
	"sync"
)

// Gaussian returns a new image containing src blurred by a 3-pass box
// approximation of a Gaussian with standard deviation sigma. src is
// not modified. A sigma <= 0 returns an unblurred copy.
//
// Gaussian allocates the result and a scratch buffer on every call;
// use a [Blurrer] to reuse buffers across frames.
func Gaussian(src *image.NRGBA, sigma float64) *image.NRGBA {
	dst := image.NewNRGBA(image.Rectangle{Max: src.Rect.Size()})
	var b Blurrer
	b.Gaussian(dst, src, sigma)
	return dst
}

// Box applies a single box blur of the given radius to src and writes
// the result to dst. The window spans 2*radius+1 pixels; radius 0
// copies. dst and src must have equal bounds sizes; dst == src is
// allowed. Box allocates a scratch buffer; use a [Blurrer] to reuse
// it.
func Box(dst, src *image.NRGBA, radius int) {
	var b Blurrer
	b.Box(dst, src, radius)
}

// A Blurrer blurs images while reusing its scratch buffer across
// calls, so repeated same-size blurs allocate nothing after the first.
// The zero value is ready to use. A Blurrer must not be used
// concurrently from multiple goroutines (each call already
// parallelises internally across all CPUs).
type Blurrer struct {
	scratch *image.NRGBA
}

// Gaussian writes into dst the 3-pass box approximation of a Gaussian
// blur of src with standard deviation sigma. dst and src must have
// equal bounds sizes; dst == src is allowed, and src is not otherwise
// modified. A sigma <= 0 copies src into dst unblurred.
func (b *Blurrer) Gaussian(dst, src *image.NRGBA, sigma float64) {
	checkSizes(dst, src)
	if sigma <= 0 {
		copyImage(dst, src)
		return
	}
	cur := src
	for _, w := range boxesForGauss(sigma, 3) {
		b.box(dst, cur, (w-1)/2)
		cur = dst
	}
}

// Box writes into dst a single box blur of src with the given radius.
// The window spans 2*radius+1 pixels; radius 0 copies. dst and src
// must have equal bounds sizes; dst == src is allowed.
func (b *Blurrer) Box(dst, src *image.NRGBA, radius int) {
	checkSizes(dst, src)
	if radius < 0 {
		radius = 0
	}
	b.box(dst, src, radius)
}

// box runs one horizontal-then-vertical box pass: src → scratch → dst.
// Each direction reads one buffer and writes the other, which is what
// makes dst == src safe.
func (b *Blurrer) box(dst, src *image.NRGBA, radius int) {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	if w == 0 || h == 0 {
		return
	}
	if radius > maxRadius {
		radius = maxRadius
	}
	if b.scratch == nil || b.scratch.Rect.Dx() != w || b.scratch.Rect.Dy() != h {
		b.scratch = image.NewNRGBA(image.Rect(0, 0, w, h))
	}
	parallel(h, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			blurRow(row(b.scratch, y), row(src, y), w, radius)
		}
	})
	parallel(w, func(x0, x1 int) {
		blurCols(dst, b.scratch, radius, x0, x1)
	})
}

func checkSizes(dst, src *image.NRGBA) {
	if dst.Rect.Dx() != src.Rect.Dx() || dst.Rect.Dy() != src.Rect.Dy() {
		panic("blur: dst and src bounds sizes differ")
	}
}

func copyImage(dst, src *image.NRGBA) {
	if dst == src {
		return
	}
	for y := 0; y < src.Rect.Dy(); y++ {
		copy(row(dst, y), row(src, y))
	}
}

// row returns the pixel bytes of logical row y (0-based from the top
// of im's bounds), respecting origin and stride so sub-images work.
func row(im *image.NRGBA, y int) []uint8 {
	off := im.PixOffset(im.Rect.Min.X, im.Rect.Min.Y+y)
	return im.Pix[off : off+im.Rect.Dx()*4 : off+im.Rect.Dx()*4]
}

func parallel(n int, fn func(lo, hi int)) {
	workers := runtime.NumCPU()
	if workers > n {
		workers = n
	}
	if workers <= 1 {
		fn(0, n)
		return
	}
	var wg sync.WaitGroup
	chunk := (n + workers - 1) / workers
	for lo := 0; lo < n; lo += chunk {
		hi := lo + chunk
		if hi > n {
			hi = n
		}
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			fn(lo, hi)
		}(lo, hi)
	}
	wg.Wait()
}

// invShift and invWindow implement exact division by the window size
// as a multiply-shift: with m = floor(2^40/d)+1, (n*m)>>40 ==
// floor(n/d) for every n <= 256*d as long as d < 2^16 (the
// Granlund-Montgomery bound: the error n*(m*d-2^40)/(d*2^40) stays
// below 1/d). Box-pass sums never exceed 255*d + d/2 < 256*d, and box
// radii are capped at maxRadius to keep d = 2r+1 under 2^16, so the
// fast path is always exact. This replaces four uint32 divisions per
// pixel, which otherwise dominate the pass.
const invShift = 40

// maxRadius caps the box radius so the window 2r+1 stays below 2^16,
// the validity bound of the multiply-shift division. A radius of
// 32767 exceeds any real image; the cap is a safety net, not a
// usable limit.
const maxRadius = 32767

func invWindow(window uint32) uint64 {
	return (1<<invShift)/uint64(window) + 1
}

func clamp(i, n int) int {
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

// blurRow box-blurs one row of w NRGBA pixels from src into dst using
// a sliding-window running sum with clamp-extend edges and rounding
// division.
func blurRow(dst, src []uint8, w, radius int) {
	window := uint32(2*radius + 1)
	half := uint64(window / 2)
	inv := invWindow(window)
	var sum [4]uint32
	for j := -radius; j <= radius; j++ {
		p := clamp(j, w) * 4
		sum[0] += uint32(src[p])
		sum[1] += uint32(src[p+1])
		sum[2] += uint32(src[p+2])
		sum[3] += uint32(src[p+3])
	}
	// The window slide needs index clamping only near the borders:
	// moving the centre from i to i+1 adds pixel i+1+radius and
	// removes pixel i-radius, both unclamped for radius <= i <=
	// w-2-radius. Split the loop so the interior runs clamp-free.
	step := func(i, lim int) {
		for ; i < lim; i++ {
			d := i * 4
			dst[d] = uint8(((uint64(sum[0]) + half) * inv) >> invShift)
			dst[d+1] = uint8(((uint64(sum[1]) + half) * inv) >> invShift)
			dst[d+2] = uint8(((uint64(sum[2]) + half) * inv) >> invShift)
			dst[d+3] = uint8(((uint64(sum[3]) + half) * inv) >> invShift)
			add := clamp(i+1+radius, w) * 4
			sub := clamp(i-radius, w) * 4
			sum[0] += uint32(src[add]) - uint32(src[sub])
			sum[1] += uint32(src[add+1]) - uint32(src[sub+1])
			sum[2] += uint32(src[add+2]) - uint32(src[sub+2])
			sum[3] += uint32(src[add+3]) - uint32(src[sub+3])
		}
	}
	lo, hi := radius, w-1-radius
	if lo > w {
		lo = w
	}
	if hi < lo {
		hi = lo
	}
	step(0, lo)
	for i := lo; i < hi; i++ {
		d := i * 4
		dst[d] = uint8(((uint64(sum[0]) + half) * inv) >> invShift)
		dst[d+1] = uint8(((uint64(sum[1]) + half) * inv) >> invShift)
		dst[d+2] = uint8(((uint64(sum[2]) + half) * inv) >> invShift)
		dst[d+3] = uint8(((uint64(sum[3]) + half) * inv) >> invShift)
		add := (i + 1 + radius) * 4
		sub := (i - radius) * 4
		sum[0] += uint32(src[add]) - uint32(src[sub])
		sum[1] += uint32(src[add+1]) - uint32(src[sub+1])
		sum[2] += uint32(src[add+2]) - uint32(src[sub+2])
		sum[3] += uint32(src[add+3]) - uint32(src[sub+3])
	}
	step(hi, w)
}

// blurCols box-blurs the column band [x0, x1) from src into dst,
// sliding one running sum per column down the rows so every read and
// write stays row-contiguous.
func blurCols(dst, src *image.NRGBA, radius, x0, x1 int) {
	h := src.Rect.Dy()
	n := (x1 - x0) * 4
	window := uint32(2*radius + 1)
	half := uint64(window / 2)
	inv := invWindow(window)
	sums := make([]uint32, n)
	for j := -radius; j <= radius; j++ {
		r := row(src, clamp(j, h))[x0*4:]
		for k := 0; k < n; k++ {
			sums[k] += uint32(r[k])
		}
	}
	for y := 0; y < h; y++ {
		d := row(dst, y)[x0*4:]
		addRow := row(src, clamp(y+1+radius, h))[x0*4:]
		subRow := row(src, clamp(y-radius, h))[x0*4:]
		for k := 0; k < n; k++ {
			s := sums[k]
			d[k] = uint8(((uint64(s) + half) * inv) >> invShift)
			sums[k] = s + uint32(addRow[k]) - uint32(subRow[k])
		}
	}
}

// boxesForGauss computes n box-filter window sizes (odd widths, in
// pixels) whose successive application approximates a Gaussian of
// standard deviation sigma — the canonical W3C SVG / Kovesi formula:
// the ideal width is sqrt(12σ²/n + 1); m of the boxes take the odd
// width just below it and the rest the odd width just above, with m
// chosen so the combined variance matches σ².
func boxesForGauss(sigma float64, n int) []int {
	wIdeal := math.Sqrt(12*sigma*sigma/float64(n) + 1)
	wl := int(math.Floor(wIdeal))
	if wl%2 == 0 {
		wl--
	}
	wu := wl + 2
	mIdeal := (12*sigma*sigma - float64(n*wl*wl) - float64(4*n*wl) - float64(3*n)) /
		(-4*float64(wl) - 4)
	m := int(math.Round(mIdeal))
	sizes := make([]int, n)
	for i := range sizes {
		if i < m {
			sizes[i] = wl
		} else {
			sizes[i] = wu
		}
	}
	return sizes
}
