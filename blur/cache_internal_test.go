package blur

import (
	"image"
	"math/rand"
	"testing"
)

// testImage returns a w×h seeded-noise NRGBA image. (The external
// test package has its own noise helper; this one is in-package so
// cache tests can read the unexported blur counter.)
func testImage(w, h int, seed int64) *image.NRGBA {
	rng := rand.New(rand.NewSource(seed))
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range im.Pix {
		im.Pix[i] = uint8(rng.Intn(256))
	}
	return im
}

// TestCacheRepeatDoesNoWork asserts the core contract: a repeated
// call with unchanged inputs performs no blur and returns the
// identical ImageOp value.
func TestCacheRepeatDoesNoWork(t *testing.T) {
	var c Cache
	src := testImage(64, 48, 1)
	op1 := c.Image(src, 4)
	if c.blurs != 1 {
		t.Fatalf("first call: %d blurs, want 1", c.blurs)
	}
	op2 := c.Image(src, 4)
	if c.blurs != 1 {
		t.Fatalf("repeated call re-blurred: %d blurs, want 1", c.blurs)
	}
	if op1 != op2 {
		t.Fatalf("repeated call returned a different ImageOp value")
	}
}

// TestCacheInvalidation asserts that changing sigma, the divisor (and
// with it the target size), or the source identity each triggers a
// fresh blur.
func TestCacheInvalidation(t *testing.T) {
	var c Cache
	src := testImage(64, 48, 1)
	base := c.Image(src, 4)

	sigma := c.Image(src, 6)
	if c.blurs != 2 {
		t.Fatalf("sigma change: %d blurs, want 2", c.blurs)
	}
	if sigma == base {
		t.Fatalf("sigma change returned the cached ImageOp")
	}

	div := c.Image(src, 4, WithDivisor(4))
	if c.blurs != 3 {
		t.Fatalf("divisor/size change: %d blurs, want 3", c.blurs)
	}
	if got, want := div.Size(), image.Pt(16, 12); got != want {
		t.Fatalf("divisor 4 op size = %v, want %v", got, want)
	}

	other := testImage(64, 48, 1) // identical pixels, different identity
	c.Image(other, 4)
	if c.blurs != 4 {
		t.Fatalf("new source identity: %d blurs, want 4", c.blurs)
	}

	// And all of the above are now cached: replaying every call
	// does no further work.
	c.Image(src, 4)
	c.Image(src, 6)
	c.Image(src, 4, WithDivisor(4))
	c.Image(other, 4)
	if c.blurs != 4 {
		t.Fatalf("replay re-blurred: %d blurs, want 4", c.blurs)
	}
}

// TestCacheDefaultDivisor pins the divisor rule — the largest power
// of two ≤ sigma/2, clamped to [1, 8] — and that Image applies it to
// the returned op's size.
func TestCacheDefaultDivisor(t *testing.T) {
	cases := []struct {
		sigma float64
		want  int
	}{
		{0, 1}, {1, 1}, {2, 1}, {3.9, 1},
		{4, 2}, {7.9, 2},
		{8, 4}, {15.9, 4},
		{16, 8}, {100, 8},
	}
	for _, tc := range cases {
		if got := DefaultDivisor(tc.sigma); got != tc.want {
			t.Errorf("DefaultDivisor(%v) = %d, want %d", tc.sigma, got, tc.want)
		}
	}

	var c Cache
	src := testImage(100, 80, 1)
	op := c.Image(src, 8) // divisor 4
	if got, want := op.Size(), image.Pt(25, 20); got != want {
		t.Fatalf("sigma 8 default op size = %v, want %v", got, want)
	}
}

// TestCacheEviction asserts the LRU bound: the cache holds at most
// maxEntries results, the least recently used entry is the one to
// go, and recently used entries survive.
func TestCacheEviction(t *testing.T) {
	var c Cache
	srcs := make([]*image.NRGBA, maxEntries+1)
	for i := range srcs {
		srcs[i] = testImage(16, 16, int64(i))
	}
	for _, s := range srcs[:maxEntries] {
		c.Image(s, 4)
	}
	if c.blurs != maxEntries {
		t.Fatalf("fill: %d blurs, want %d", c.blurs, maxEntries)
	}
	c.Image(srcs[0], 4) // refresh entry 0 so entry 1 is now LRU
	c.Image(srcs[maxEntries], 4)
	if len(c.entries) != maxEntries {
		t.Fatalf("cache holds %d entries, want %d", len(c.entries), maxEntries)
	}
	c.Image(srcs[1], 4) // evicted above: must re-blur
	if c.blurs != maxEntries+2 {
		t.Fatalf("evicted entry re-request: %d blurs, want %d", c.blurs, maxEntries+2)
	}
	c.Image(srcs[0], 4) // refreshed earlier: must still be cached
	c.Image(srcs[maxEntries], 4)
	if c.blurs != maxEntries+2 {
		t.Fatalf("recent entries re-blurred: %d blurs, want %d", c.blurs, maxEntries+2)
	}
}

// TestCacheDownscaledBlurMatches asserts the downscale path really
// blurs: the ÷2 result approximates a full-resolution blur — compare
// against Gaussian at full size, tolerating the resampling error.
func TestCacheDownscaledBlurMatches(t *testing.T) {
	var c Cache
	src := testImage(128, 96, 7)
	op := c.Image(src, 8, WithDivisor(2))
	if got, want := op.Size(), image.Pt(64, 48); got != want {
		t.Fatalf("op size = %v, want %v", got, want)
	}
	// The reduced-size blur with reduced sigma must be close in the
	// mean to the full-resolution blur (means are scale-invariant).
	full := Gaussian(src, 8)
	var wantMean, n float64
	for i := 0; i < len(full.Pix); i += 4 {
		wantMean += float64(full.Pix[i])
		n++
	}
	wantMean /= n
	// The op wraps an RGBA copy that cannot be read back, so inspect
	// the same pixels through render's own pixel path.
	tmp := c.renderNRGBA(src, 8, 2, image.Pt(64, 48))
	var gotMean, m float64
	for i := 0; i < len(tmp.Pix); i += 4 {
		gotMean += float64(tmp.Pix[i])
		m++
	}
	gotMean /= m
	if diff := gotMean - wantMean; diff > 2 || diff < -2 {
		t.Fatalf("downscaled blur mean %.2f, full-res blur mean %.2f", gotMean, wantMean)
	}
}

// TestCacheConcurrent exercises the mutex under the race detector.
func TestCacheConcurrent(t *testing.T) {
	var c Cache
	srcs := []*image.NRGBA{testImage(32, 32, 1), testImage(32, 32, 2)}
	done := make(chan struct{})
	for g := 0; g < 4; g++ {
		go func(g int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < 20; i++ {
				c.Image(srcs[g%2], 4)
			}
		}(g)
	}
	for g := 0; g < 4; g++ {
		<-done
	}
	if c.blurs != 2 {
		t.Fatalf("%d blurs across goroutines, want 2", c.blurs)
	}
}
