package springbutton_test

import (
	"image"
	"math"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/reactivego/rx"
	"github.com/vibrantgio/components/button"
	"github.com/vibrantgio/effects/spring"
	"github.com/vibrantgio/effects/springbutton"
	"github.com/vibrantgio/theme/theme"
)

// TestSpringButtonObservableEmits is the construction smoke test: the
// observable must emit a non-nil widget when subscribed against the
// default theme. Catches wiring regressions in the rx pipeline.
func TestSpringButtonObservableEmits(t *testing.T) {
	w, err := springbutton.SpringButton(
		rx.Of(theme.Default()),
		button.Props{Label: "OK"},
		springbutton.Options{},
	).First()
	if err != nil {
		t.Fatalf("First() = %v", err)
	}
	if w == nil {
		t.Fatal("SpringButton emitted nil widget")
	}
}

// TestSpringButtonRendersWithoutPanic exercises the full render path
// once. A laid-out frame proves the gtx wiring (Clickable.Layout,
// op.Affine, op.Record/Stop) composes correctly under realistic
// constraints.
func TestSpringButtonRendersWithoutPanic(t *testing.T) {
	w, err := springbutton.SpringButton(
		rx.Of(theme.Default()),
		button.Props{Label: "Hello"},
		springbutton.Options{},
	).First()
	if err != nil {
		t.Fatalf("First() = %v", err)
	}

	var ops op.Ops
	gtx := layout.Context{
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(300, 60)),
		Ops:         &ops,
	}
	dims := w(gtx)
	if dims.Size.X == 0 || dims.Size.Y == 0 {
		t.Errorf("SpringButton dims = %v, want non-zero size", dims.Size)
	}
}

// TestDefaultsProduceVisibleMotionWithin200ms is the calibration
// guard: a typical button tap lasts ~150–200 ms. If the default spring
// has not moved a visible fraction of the way toward [DefaultPressScale]
// within 12 ticks at 60 Hz (200 ms), the side-by-side gallery demo
// will look indistinguishable from a static button, defeating the
// purpose of the variant.
//
// Visible fraction: at least 30 % of the 1.0 → 0.92 span must be
// covered in 200 ms. This is a calibration threshold, not a precision
// claim — the spring's exact trajectory is governed by the physics
// constants in [springbutton]'s defaults.
func TestDefaultsProduceVisibleMotionWithin200ms(t *testing.T) {
	sp := spring.New(1.0, springbutton.DefaultPressScale, spring.Options{
		Stiffness: springbutton.DefaultStiffness,
		Damping:   springbutton.DefaultDamping,
		Mass:      springbutton.DefaultMass,
	})
	for range 12 {
		sp.Tick(60) // 12 ticks @ 60 Hz = 200 ms
	}
	span := 1.0 - springbutton.DefaultPressScale
	progress := (1.0 - sp.Value()) / span
	const minVisible = 0.30
	if progress < minVisible {
		t.Errorf("after 200 ms the spring covered %.1f%% of the press span (value=%v); "+
			"need at least %.0f%% for the gallery side-by-side to be visibly different from static",
			progress*100, sp.Value(), minVisible*100)
	}
}

// TestSpringSettlesOnReleaseWithin500ms asserts the spring fully
// recovers to 1.0 (no leftover scale offset) within 500 ms after the
// target snaps from PressScale back to 1.0. Guards against an
// over-damped tuning that would leave the button visibly shrunken
// after the user lifts off.
func TestSpringSettlesOnReleaseWithin500ms(t *testing.T) {
	sp := spring.New(springbutton.DefaultPressScale, 1.0, spring.Options{
		Stiffness: springbutton.DefaultStiffness,
		Damping:   springbutton.DefaultDamping,
		Mass:      springbutton.DefaultMass,
	})
	for range 30 {
		sp.Tick(60) // 30 ticks @ 60 Hz = 500 ms
	}
	if got := math.Abs(sp.Value() - 1.0); got > 0.005 {
		t.Errorf("after 500 ms release, |scale - 1.0| = %v, want <= 0.005 (value=%v)",
			got, sp.Value())
	}
}
