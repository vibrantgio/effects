package springbutton_test

import (
	"image"
	"testing"
	"time"

	"github.com/reactivego/rx"
	"github.com/vibrantgio/components/button"
	"github.com/vibrantgio/components/golden"
	"github.com/vibrantgio/components/icons"
	"github.com/vibrantgio/effects/springbutton"
	"github.com/vibrantgio/theme/tokens"
)

// pressShot captures the chrome spring button after a press held for frames
// frames, one frame every step, under the motion scale m.
func pressShot(t *testing.T, m tokens.MotionScale, frames int, step time.Duration) *image.RGBA {
	t.Helper()
	mark := icons.Mark(icons.SidebarLeft)
	if mark == nil {
		t.Fatal("no painter for the sidebar")
	}
	c := tokens.PlatformLight
	th := staticTheme(c)
	th.Motion = rx.Of(m)
	w, err := springbutton.SpringButton(rx.Of(th), button.Props{
		Description: "Hide the conversations",
		Variant:     button.Chrome,
		Surface:     c.SidebarMaterial,
		Icon:        mark,
	}, springbutton.Options{}).First()
	if err != nil {
		t.Fatalf("First() = %v", err)
	}
	scene := onChrome(c, w)
	if frames >= 0 {
		scene = held(scene, chromeFrame, chromePress, frames, step)
	}
	return golden.Capture(t, chromeFrame, scene)
}

// TestPressFollowsTheFrameTime holds that the button reads the frame time from
// gtx.Now: the same frames drawn 1/60 s apart and 1/120 s apart leave the
// press at different depths.
func TestPressFollowsTheFrameTime(t *testing.T) {
	at60 := pressShot(t, tokens.Motion, 3, time.Second/60)
	at120 := pressShot(t, tokens.Motion, 3, time.Second/120)
	if n := golden.PixelDiff(at60, at120); n == 0 {
		t.Error("frames 1/60 s apart and 1/120 s apart drew the same press, so the spring does not read the frame time")
	}
}

// TestReducedMotionPressLandsInOneFrame holds the snap: with the theme's
// reduced motion scale, the first frame that sees the press draws the button
// at its pressed scale, the same drawing thirty frames later shows.
func TestReducedMotionPressLandsInOneFrame(t *testing.T) {
	reduced := tokens.Motion.Reduced()
	rest := pressShot(t, reduced, -1, frame60)
	first := pressShot(t, reduced, 0, frame60)
	settled := pressShot(t, reduced, 30, frame60)
	if n := golden.PixelDiff(first, settled); n != 0 {
		t.Errorf("under reduced motion the first pressed frame differs from the settled press in %d pixels", n)
	}
	if n := golden.PixelDiff(first, rest); n == 0 {
		t.Error("under reduced motion the first pressed frame draws the button at rest, so the press did not land")
	}

	// The same first frame with full motion is still in flight.
	moving := pressShot(t, tokens.Motion, 0, frame60)
	if n := golden.PixelDiff(moving, settled); n == 0 {
		t.Error("with full motion the first pressed frame already shows the settled press, so the test cannot tell a snap from a spring")
	}
}
