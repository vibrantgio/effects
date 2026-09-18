package depth_test

import (
	"image"
	"image/color"
	"math"
	"testing"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/vibrantgio/components/golden"
	depth "github.com/vibrantgio/effects/depth"
	"github.com/vibrantgio/theme/tokens"
)

// TestRampLandsWhereThePlatformComposites walks the rendered ramp outward
// from the surface's edge and holds every pixel of it against the value the
// platform's own blend puts there: the encoded byte kept at (1 − coverage)
// of the plane it falls on.
//
// The ramp is drawn over the window's own plane in each scheme and over the
// platform's accent, which is where a blend in linear light misses widest —
// the accent's middle channel is the one a gamma error moves most. One 255th
// is the whole tolerance, and it is the tolerance the ramp itself was fitted
// to off the sidebar-shadow captures.
func TestRampLandsWhereThePlatformComposites(t *testing.T) {
	for _, tc := range []struct {
		name   string
		plane  color.NRGBA
		shadow tokens.DropShadow
	}{
		{"over the light plane", tokens.PlatformLight.WindowBackground, tokens.PlatformLight.FloatingShadow},
		{"over the dark plane", tokens.PlatformDark.WindowBackground, tokens.PlatformDark.FloatingShadow},
		{"over the accent", tokens.PlatformLight.ControlAccent, tokens.PlatformLight.FloatingShadow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The reading's own reach, in px at the test's 1:1 metric.
			reach := int(tc.shadow.Reach)
			const pad = 40
			size := image.Pt(boundsX1+pad, frameH)
			img := golden.Capture(t, size, func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, tc.plane, clip.Rect{Max: gtx.Constraints.Max}.Op())
				depth.Shadow(gtx, boundsRect, 0, tc.shadow)
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
			y := (boundsY0 + boundsY1) / 2
			worst, where := 0, 0
			for k := 0; k < reach; k++ {
				// The gradient is sampled at the pixel's centre, half a pixel
				// out from the stop at the surface's edge.
				coverage := float64(tc.shadow.Peak.A) / 255 * (1 - (float64(k)+0.5)/float64(reach))
				i := img.PixOffset(boundsX1+k, y)
				for c, plane := range []uint8{tc.plane.R, tc.plane.G, tc.plane.B} {
					want := int(math.Round((1 - coverage) * float64(plane)))
					e := int(img.Pix[i+c]) - want
					if e < 0 {
						e = -e
					}
					if e > worst {
						worst, where = e, k
					}
				}
			}
			if worst > 1 {
				t.Errorf("the ramp misses the platform's composite by %d/255 at %d px out; one 255th is the tolerance", worst, where)
			}
		})
	}
}
