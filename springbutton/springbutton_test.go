package springbutton_test

import (
	"image"
	"math"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/reactivego/rx"
	"github.com/vibrantgio/components/button"
	"github.com/vibrantgio/components/golden"
	"github.com/vibrantgio/components/icons"
	"github.com/vibrantgio/effects/spring"
	"github.com/vibrantgio/effects/springbutton"
	"github.com/vibrantgio/theme/theme"
)

// TestSpringButtonObservableEmits is the construction smoke test: the
// observable must emit a non-nil layout.Widget when subscribed against the
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
		t.Fatal("SpringButton emitted a nil layout.Widget")
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
// within 12 ticks at 60 Hz (200 ms), the button looks
// indistinguishable from a static one, defeating the purpose of the
// variant.
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

// TestTheChromeSpringButtonRecordsItsState is the state's own guard at the
// pixels: a spring button handed a symbol in a chrome region draws the
// platform's bordered toolbar control, and a control that records a yes draws
// the chosen segment's patch inside its own box.
//
// It is pinned against the static chrome button rather than against a stored
// image, because that is the claim: the spring variant is the same control
// with a scale on press, so at rest — the spring starts settled at 1.0 and the
// affine is the identity — it has to be the static drawing pixel for pixel, in
// both states. The two states are also required to differ, or the pin would
// pass on a control that drew the patch in neither.
func TestTheChromeSpringButtonRecordsItsState(t *testing.T) {
	th := theme.Default()
	colors, err := th.Platform.First()
	if err != nil {
		t.Fatalf("theme platform colours: %v", err)
	}
	density, err := th.Density.First()
	if err != nil {
		t.Fatalf("theme density: %v", err)
	}

	size := image.Pt(80, 60)
	mark := icons.Mark(icons.Sidebar)
	if mark == nil {
		t.Fatal("no painter for the sidebar")
	}

	shot := func(w layout.Widget) *image.RGBA {
		return golden.Capture(t, size, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, colors.SidebarMaterial, clip.Rect{Max: gtx.Constraints.Max}.Op())
			defer op.Offset(image.Pt(12, 12)).Push(gtx.Ops).Pop()
			return w(gtx)
		})
	}

	var states [2]*image.RGBA
	for i, checked := range [2]bool{false, true} {
		props := button.Props{
			Description: "Hide the conversations",
			Variant:     button.Chrome,
			Surface:     colors.SidebarMaterial,
			Icon:        mark,
			Checked:     checked,
		}
		spring, err := springbutton.SpringButton(rx.Of(th), props, springbutton.Options{}).First()
		if err != nil {
			t.Fatalf("checked=%v: First() = %v", checked, err)
		}
		static := button.RenderChrome(mark, colors, density, button.RenderState{
			Variant: button.Chrome,
			Surface: colors.SidebarMaterial,
			Checked: checked,
		})
		got, want := shot(spring), shot(static)
		if n := golden.PixelDiff(got, want); n != 0 {
			t.Errorf("checked=%v: the spring button differs from the static chrome button in %d pixels — "+
				"at rest the two are one control", checked, n)
		}
		states[i] = got
	}

	if n := golden.PixelDiff(states[0], states[1]); n == 0 {
		t.Error("the spring button drew the same control switched on and off, so it records no state")
	}
}
