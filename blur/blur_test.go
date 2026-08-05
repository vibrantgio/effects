package blur_test

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"testing"

	"github.com/vibrantgio/pulse/blur"
)

// noiseImage returns a w×h image of seeded uniform random noise in all
// four channels.
func noiseImage(w, h int, seed int64) *image.NRGBA {
	rng := rand.New(rand.NewSource(seed))
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range im.Pix {
		im.Pix[i] = uint8(rng.Intn(256))
	}
	return im
}

// referenceGaussian is the ground truth: a separable direct
// convolution with the true Gaussian kernel in float64, clamp-extend
// at the edges (the same edge policy as the package under test, so any
// difference is purely kernel shape).
func referenceGaussian(src *image.NRGBA, sigma float64) *image.NRGBA {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	radius := int(math.Ceil(3 * sigma))
	kernel := make([]float64, 2*radius+1)
	var norm float64
	for i := range kernel {
		x := float64(i - radius)
		kernel[i] = math.Exp(-x * x / (2 * sigma * sigma))
		norm += kernel[i]
	}
	for i := range kernel {
		kernel[i] /= norm
	}
	clamp := func(i, n int) int {
		if i < 0 {
			return 0
		}
		if i >= n {
			return n - 1
		}
		return i
	}
	tmp := make([]float64, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			for c := 0; c < 4; c++ {
				var acc float64
				for k, kv := range kernel {
					sx := clamp(x+k-radius, w)
					acc += kv * float64(src.Pix[y*src.Stride+sx*4+c])
				}
				tmp[(y*w+x)*4+c] = acc
			}
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			for c := 0; c < 4; c++ {
				var acc float64
				for k, kv := range kernel {
					sy := clamp(y+k-radius, h)
					acc += kv * tmp[(sy*w+x)*4+c]
				}
				dst.Pix[y*dst.Stride+x*4+c] = uint8(math.Round(acc))
			}
		}
	}
	return dst
}

// channelStats returns the mean and variance of channel c across the
// sub-rectangle r of im (r in im's coordinate space).
func channelStats(im *image.NRGBA, c int, r image.Rectangle) (mean, variance float64) {
	n := float64(r.Dx() * r.Dy())
	var sum, sumSq float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		off := im.PixOffset(r.Min.X, y)
		for x := 0; x < r.Dx(); x++ {
			v := float64(im.Pix[off+x*4+c])
			sum += v
			sumSq += v * v
		}
	}
	mean = sum / n
	return mean, sumSq/n - mean*mean
}

// TestGaussianConvergence compares the 3-pass box approximation
// against a direct true-Gaussian convolution on random noise: the
// per-channel variance reduction var(blurred)/var(src) must agree
// within a few percent, and no pixel may stray far.
//
// The comparison runs over the interior where both kernels have full
// support (a margin of 3σ is excluded): inside the border band the two
// edge-clamp policies compose differently — clamping each box pass is
// not the same operator as clamping one composed kernel — so border
// pixels measure edge policy, not kernel convergence. The edge policy
// has its own tests below.
func TestGaussianConvergence(t *testing.T) {
	for _, sigma := range []float64{4, 8} {
		src := noiseImage(192, 192, 1)
		got := blur.Gaussian(src, sigma)
		want := referenceGaussian(src, sigma)

		margin := int(math.Ceil(3 * sigma))
		interior := src.Rect.Inset(margin)

		for c := 0; c < 4; c++ {
			_, varSrc := channelStats(src, c, interior)
			_, varGot := channelStats(got, c, interior)
			_, varWant := channelStats(want, c, interior)
			gotRatio := varGot / varSrc
			wantRatio := varWant / varSrc
			relDiff := math.Abs(gotRatio-wantRatio) / wantRatio
			t.Logf("sigma=%v channel=%d: variance reduction box=%.5f gauss=%.5f rel diff=%.2f%%",
				sigma, c, gotRatio, wantRatio, 100*relDiff)
			if relDiff > 0.03 {
				t.Errorf("sigma=%v channel=%d: variance reduction differs by %.2f%%, want within 3%%",
					sigma, c, 100*relDiff)
			}
		}

		maxDelta := 0
		for y := interior.Min.Y; y < interior.Max.Y; y++ {
			for x := interior.Min.X; x < interior.Max.X; x++ {
				for c := 0; c < 4; c++ {
					off := y*got.Stride + x*4 + c
					d := int(got.Pix[off]) - int(want.Pix[off])
					if d < 0 {
						d = -d
					}
					if d > maxDelta {
						maxDelta = d
					}
				}
			}
		}
		t.Logf("sigma=%v: max interior pixel delta vs true Gaussian = %d", sigma, maxDelta)
		if maxDelta > 8 {
			t.Errorf("sigma=%v: max interior pixel delta %d vs true Gaussian, want <= 8", sigma, maxDelta)
		}
	}
}

// TestUniformStaysUniform is the edge test: a blur that darkens or
// wraps at the borders breaks uniformity there first. A uniform input
// must come back byte-exact uniform right up to the edge.
func TestUniformStaysUniform(t *testing.T) {
	c := color.NRGBA{R: 201, G: 117, B: 63, A: 255}
	src := image.NewNRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			src.SetNRGBA(x, y, c)
		}
	}
	check := func(name string, got *image.NRGBA) {
		t.Helper()
		for y := 0; y < 48; y++ {
			for x := 0; x < 64; x++ {
				if p := got.NRGBAAt(x, y); p != c {
					t.Fatalf("%s: pixel (%d,%d) = %v, want %v", name, x, y, p, c)
				}
			}
		}
	}
	check("Gaussian sigma=6", blur.Gaussian(src, 6))

	dst := image.NewNRGBA(src.Rect)
	blur.Box(dst, src, 7)
	check("Box radius=7", dst)
}

// TestMeanPreserved catches wrap and darken on non-uniform input:
// blurring redistributes energy but must not create or destroy it, so
// the per-channel mean survives to within rounding.
func TestMeanPreserved(t *testing.T) {
	src := noiseImage(160, 90, 2)
	got := blur.Gaussian(src, 6)
	for c := 0; c < 4; c++ {
		srcMean, _ := channelStats(src, c, src.Rect)
		gotMean, _ := channelStats(got, c, got.Rect)
		if diff := math.Abs(gotMean - srcMean); diff > 0.5 {
			t.Errorf("channel %d: mean drifted from %.3f to %.3f (|Δ|=%.3f), want within 0.5",
				c, srcMean, gotMean, diff)
		}
	}
}

// TestIdentity: radius-0 box and sigma<=0 Gaussian must be exact
// copies.
func TestIdentity(t *testing.T) {
	src := noiseImage(37, 23, 3)

	dst := image.NewNRGBA(src.Rect)
	blur.Box(dst, src, 0)
	for i := range src.Pix {
		if dst.Pix[i] != src.Pix[i] {
			t.Fatalf("Box radius=0: pixel byte %d changed", i)
		}
	}

	got := blur.Gaussian(src, 0)
	for i := range src.Pix {
		if got.Pix[i] != src.Pix[i] {
			t.Fatalf("Gaussian sigma=0: pixel byte %d changed", i)
		}
	}
}

// TestSubImage locks the stride and origin handling: blurring a
// sub-image view must equal blurring the same pixels as a standalone
// image.
func TestSubImage(t *testing.T) {
	base := noiseImage(96, 96, 4)
	rect := image.Rect(16, 8, 80, 72)
	sub := base.SubImage(rect).(*image.NRGBA)

	standalone := image.NewNRGBA(image.Rectangle{Max: rect.Size()})
	for y := 0; y < rect.Dy(); y++ {
		for x := 0; x < rect.Dx(); x++ {
			standalone.SetNRGBA(x, y, base.NRGBAAt(rect.Min.X+x, rect.Min.Y+y))
		}
	}

	fromSub := blur.Gaussian(sub, 4)
	fromStandalone := blur.Gaussian(standalone, 4)
	for i := range fromStandalone.Pix {
		if fromSub.Pix[i] != fromStandalone.Pix[i] {
			t.Fatalf("sub-image blur diverges from standalone blur at pixel byte %d", i)
		}
	}
}

// TestBlurrerInPlace: dst == src must give the same result as
// blurring into a separate destination.
func TestBlurrerInPlace(t *testing.T) {
	src := noiseImage(64, 64, 5)
	want := blur.Gaussian(src, 5)

	var b blur.Blurrer
	inPlace := noiseImage(64, 64, 5)
	b.Gaussian(inPlace, inPlace, 5)
	for i := range want.Pix {
		if inPlace.Pix[i] != want.Pix[i] {
			t.Fatalf("in-place blur diverges at pixel byte %d", i)
		}
	}
}

// Benchmarks at the G-E4 pipeline table's resolutions. Sigma follows
// the backdrop model — the radius lives in source pixels, so halving
// the resolution halves sigma (σ=8 at ÷1). These time the blur alone;
// the plan's table times render+readback+blur.
func benchmarkGaussian(b *testing.B, w, h int, sigma float64) {
	src := noiseImage(w, h, 6)
	dst := image.NewNRGBA(src.Rect)
	var bl blur.Blurrer
	bl.Gaussian(dst, src, sigma) // warm the scratch buffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bl.Gaussian(dst, src, sigma)
	}
}

func BenchmarkGaussian1440x900(b *testing.B) { benchmarkGaussian(b, 1440, 900, 8) }
func BenchmarkGaussian720x450(b *testing.B)  { benchmarkGaussian(b, 720, 450, 4) }
func BenchmarkGaussian360x225(b *testing.B)  { benchmarkGaussian(b, 360, 225, 2) }
func BenchmarkGaussian180x112(b *testing.B)  { benchmarkGaussian(b, 180, 112, 1) }
