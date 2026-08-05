package blur

import (
	"image"
	"sync"

	"gioui.org/op/paint"
	xdraw "golang.org/x/image/draw"
)

// maxEntries bounds a [Cache]. Static imagery is small-N — a hero
// image, a dialog backdrop, a handful of cards — so a Cache holds at
// most this many blurred results and evicts the least recently used
// one beyond that. An unbounded cache of image-sized entries would be
// a leak.
const maxEntries = 8

// DefaultDivisor returns the downscale divisor [Cache.Image] uses
// when no [WithDivisor] option is given: the largest power of two no
// greater than sigma/2, clamped to [1, 8]. A Gaussian blur of
// standard deviation sigma destroys detail finer than roughly sigma
// source pixels, so blurring at 1/divisor resolution loses nothing
// visible while cutting the work by divisor². At sigma 8 the rule
// picks ÷4, the working configuration of the G-E4 pipeline table.
func DefaultDivisor(sigma float64) int {
	d := 1
	for d < 8 && sigma/2 >= float64(2*d) {
		d *= 2
	}
	return d
}

// An Option adjusts a single [Cache.Image] call.
type Option func(*options)

type options struct {
	divisor int
}

// WithDivisor overrides the downscale divisor for one [Cache.Image]
// call. The source is scaled to 1/n of its size before blurring and
// the returned op stays at that reduced size. Values below 1 fall
// back to [DefaultDivisor]. The divisor is part of the cache key.
func WithDivisor(n int) Option {
	return func(o *options) {
		o.divisor = n
	}
}

// A Cache memoizes blurred still imagery as ready-to-paint
// [paint.ImageOp]s: a known source image is blurred once and the
// uploaded op is reused on every subsequent frame.
//
// # Cache key
//
// Results are keyed on source identity, sigma, divisor and target
// size. Source identity is the image.Image interface value itself —
// for the usual pointer image types (*image.NRGBA, *image.RGBA, …)
// that is pointer equality. Passing a different image with identical
// pixels re-blurs; mutating the pixel buffer behind a previously
// passed pointer is caller misuse and is not detected — treat cached
// sources as immutable. The source's dynamic type must be comparable
// (every standard library image type is).
//
// # Downscale, blur, upscale
//
// For large radii the source is first downscaled by the divisor
// (defaulted by [DefaultDivisor], overridable with [WithDivisor])
// using x/image/draw's ApproxBiLinear — cheap and plenty, since the
// blur immediately destroys any scaling artifacts — then blurred at
// the reduced size with a proportionally reduced sigma. The returned
// op stays at the reduced size: paint it through an affine scale into
// the full-size destination rect, and Gio's default FilterLinear
// upscales it on the GPU at draw time. A CPU upscale would spend
// memory and bandwidth reconstructing detail the blur has already
// destroyed, so there is none.
//
// # Bounds and concurrency
//
// A Cache holds at most 8 entries and evicts least recently used
// beyond that. The zero value is ready to use, and a Cache is safe
// for concurrent use — though components normally call it from the
// single frame goroutine.
type Cache struct {
	mu      sync.Mutex
	entries map[cacheKey]*cacheEntry
	tick    uint64
	blurrer Blurrer

	// blurs counts blur computations (cache misses); test
	// instrumentation for the "repeated call does no work" contract.
	blurs int
}

type cacheKey struct {
	src     image.Image
	sigma   float64
	divisor int
	size    image.Point // target (reduced) size of the returned op
}

type cacheEntry struct {
	op   paint.ImageOp
	tick uint64
}

// Image returns a [paint.ImageOp] holding src blurred by a Gaussian
// of standard deviation sigma, computing it on the first call and
// serving the cached op — the identical value, no work — on repeated
// calls with unchanged inputs. See [Cache] for the key contract and
// the reduced-size paint contract: the op covers the source size
// divided by the divisor, and the caller paints it scaled up into the
// full-size rect.
func (c *Cache) Image(src image.Image, sigma float64, opts ...Option) paint.ImageOp {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	d := o.divisor
	if d < 1 {
		d = DefaultDivisor(sigma)
	}
	sz := src.Bounds().Size()
	small := image.Point{X: (sz.X + d - 1) / d, Y: (sz.Y + d - 1) / d}
	k := cacheKey{src: src, sigma: sigma, divisor: d, size: small}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.tick++
	if e, ok := c.entries[k]; ok {
		e.tick = c.tick
		return e.op
	}
	op := c.render(src, sigma, d, small)
	if c.entries == nil {
		c.entries = make(map[cacheKey]*cacheEntry, maxEntries)
	}
	if len(c.entries) >= maxEntries {
		c.evict()
	}
	c.entries[k] = &cacheEntry{op: op, tick: c.tick}
	return op
}

// render downscales src by d to size small, blurs it at reduced
// sigma, and wraps the result in an ImageOp. Called with c.mu held.
func (c *Cache) render(src image.Image, sigma float64, d int, small image.Point) paint.ImageOp {
	c.blurs++
	return paint.NewImageOp(c.renderNRGBA(src, sigma, d, small))
}

// renderNRGBA is render's pixel work, split out so tests can inspect
// the blurred image an ImageOp is built from.
func (c *Cache) renderNRGBA(src image.Image, sigma float64, d int, small image.Point) *image.NRGBA {
	tmp := image.NewNRGBA(image.Rectangle{Max: small})
	if n, ok := src.(*image.NRGBA); ok && d == 1 {
		c.blurrer.Gaussian(tmp, n, sigma)
		return tmp
	}
	xdraw.ApproxBiLinear.Scale(tmp, tmp.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	c.blurrer.Gaussian(tmp, tmp, sigma/float64(d))
	return tmp
}

// evict removes the least recently used entry. Called with c.mu held
// and len(c.entries) > 0.
func (c *Cache) evict() {
	var (
		oldest cacheKey
		found  bool
		best   uint64
	)
	for k, e := range c.entries {
		if !found || e.tick < best {
			oldest, best, found = k, e.tick, true
		}
	}
	delete(c.entries, oldest)
}
