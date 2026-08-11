package motion_test

import (
	"image"
	"image/color"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/vibrantgio/components/button"
	"github.com/vibrantgio/components/golden"
	motion "github.com/vibrantgio/effects/motion"
	"github.com/vibrantgio/theme/tokens"
)

// ---- fixture geometry & colours ----

const (
	canvasW, canvasH = 320, 80

	// invDt matches the spring-package recommended frame-loop
	// convention for a 60 Hz host.
	invDt = 60.0
)

var (
	bgColor    = color.NRGBA{R: 248, G: 248, B: 250, A: 255}
	canvasSize = image.Pt(canvasW, canvasH)
	buttonSize = image.Pt(canvasW, canvasH)
)

// ---- button rendering ----

// shaper is the canonical Roboto shaper, built once for the process and
// cached behind tokens.DefaultTypography, which every copy of that value
// shares; shaper construction dominates the per-test wall time otherwise.
var shaper = tokens.DefaultTypography.DeterministicShaper()

// renderBtn returns a layout.Widget for a button rendered with the
// given colour tokens and visual state. Sharp-cornered, empty-label —
// matches the determinism trick in components/button/button_test.go so the
// motion-applied output stays bit-stable across GPU contexts.
func renderBtn(colors tokens.ColorTokens, s button.RenderState) layout.Widget {
	sharp := tokens.RadiusScale{}
	return button.Render(shaper, "", colors, tokens.Spacing, sharp, tokens.DefaultTypography.LabelLarge, tokens.Comfortable, s)
}

// scene composes a light backdrop and a motion-transformed button on
// top. The light backdrop gives the dimming opacity and the scaled
// edges unambiguous contrast for golden diffing.
func scene(state motion.State, colors tokens.ColorTokens, btnState button.RenderState) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		motion.Apply(gtx, state, renderBtn(colors, btnState))
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
}

// swapScene composes a light backdrop, the outgoing button at out
// state, then the incoming button at in state. Both buttons render at
// the same position so straight-alpha composition produces the
// crossfade.
func swapScene(out motion.State, outColors tokens.ColorTokens, in motion.State, inColors tokens.ColorTokens) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		motion.Apply(gtx, out, renderBtn(outColors, button.RenderState{}))
		motion.Apply(gtx, in, renderBtn(inColors, button.RenderState{}))
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
}

// bgOnly is the no-button baseline: just the backdrop. Used as the
// expected pixels when motion state == Hidden (opacity 0).
func bgOnly(gtx layout.Context) layout.Dimensions {
	paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

// ---- frame schedules ----

// runEnter ticks an Enter primitive ticks times and returns its state.
func runEnter(opts motion.Options, ticks int) motion.State {
	e := motion.NewEnter(opts)
	for range ticks {
		e.Tick(invDt)
	}
	return e.State()
}

func runExit(opts motion.Options, ticks int) motion.State {
	e := motion.NewExit(opts)
	for range ticks {
		e.Tick(invDt)
	}
	return e.State()
}

// runTransition ticks a Transition ticks times and returns the (out, in)
// state pair.
func runTransition(opts motion.Options, ticks int) (motion.State, motion.State) {
	tr := motion.NewTransition(opts)
	for range ticks {
		tr.Tick(invDt)
	}
	return tr.Out(), tr.In()
}

// ---- golden tests (the G3.4 Measurable) ----

// TestEnterGoldens captures three milestones of the Enter animation:
// start (frame 0, fully Hidden), mid (half-way through the opacity
// tween), and end (well after the tween completes and the spring
// settles).
func TestEnterGoldens(t *testing.T) {
	opts := motion.Options{}
	cases := []struct {
		name  string
		ticks int
	}{
		{"enter-start", 0},
		{"enter-mid", motion.DefaultFrames / 2},
		{"enter-end", 2 * motion.DefaultFrames}, // generous settle margin
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := runEnter(opts, tc.ticks)
			golden.Render(t, tc.name, canvasSize, scene(s, tokens.DefaultLight, button.RenderState{}))
		})
	}
}

// TestExitGoldens captures three milestones of the Exit animation:
// start (frame 0, fully Visible), mid, and end (after the opacity tween
// completes and the spring settles back to FromScale).
func TestExitGoldens(t *testing.T) {
	opts := motion.Options{}
	cases := []struct {
		name  string
		ticks int
	}{
		{"exit-start", 0},
		{"exit-mid", motion.DefaultFrames / 2},
		{"exit-end", 2 * motion.DefaultFrames},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := runExit(opts, tc.ticks)
			golden.Render(t, tc.name, canvasSize, scene(s, tokens.DefaultLight, button.RenderState{}))
		})
	}
}

// TestSwapGoldens captures three milestones of a Transition (swap)
// from a light-theme button to a dark-theme one. The colour change
// makes the crossfade unambiguous in the goldens: the outgoing
// (light) button fades out while the incoming (dark) one fades in,
// and the mid-frame shows both contributing.
func TestSwapGoldens(t *testing.T) {
	opts := motion.Options{}
	cases := []struct {
		name  string
		ticks int
	}{
		{"swap-start", 0},
		{"swap-mid", motion.DefaultFrames / 2},
		{"swap-end", 2 * motion.DefaultFrames},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, in := runTransition(opts, tc.ticks)
			golden.Render(t, tc.name, canvasSize, swapScene(
				out, tokens.DefaultLight,
				in, tokens.DefaultDark,
			))
		})
	}
}

// ---- cross-frame structural assertions ----

// TestEnterStartIsBackground asserts the enter animation at frame 0 is
// indistinguishable from the no-button background: opacity 0 means the
// button cannot bleed through. Catches a regression where Apply's
// opacity layer leaks paint past Opacity=0 (e.g., if a future
// short-circuit replaces the layer push).
func TestEnterStartIsBackground(t *testing.T) {
	bg := golden.Capture(t, canvasSize, bgOnly)
	startFrame := golden.Capture(t, canvasSize, scene(motion.Hidden, tokens.DefaultLight, button.RenderState{}))
	if n := golden.PixelDiff(bg, startFrame); n != 0 {
		t.Errorf("Enter start (Hidden state) should match background; %d pixels differ", n)
	}
}

// TestExitEndIsBackground asserts the exit animation, well after its
// opacity tween completes, has faded the button to invisibility.
func TestExitEndIsBackground(t *testing.T) {
	bg := golden.Capture(t, canvasSize, bgOnly)
	end := runExit(motion.Options{}, 2*motion.DefaultFrames)
	endFrame := golden.Capture(t, canvasSize, scene(end, tokens.DefaultLight, button.RenderState{}))
	if n := golden.PixelDiff(bg, endFrame); n != 0 {
		t.Errorf("Exit end should match background; %d pixels differ (opacity=%v)", n, end.Opacity)
	}
}

// TestEnterEndDistinctFromStart asserts the enter animation moves the
// pixels: the end frame must visibly differ from the start frame.
// Guards against a regression where the opacity tween silently
// degenerates (e.g., Frames=0 falls through and the tween returns
// From forever).
func TestEnterEndDistinctFromStart(t *testing.T) {
	startState := runEnter(motion.Options{}, 0)
	endState := runEnter(motion.Options{}, 2*motion.DefaultFrames)
	startImg := golden.Capture(t, canvasSize, scene(startState, tokens.DefaultLight, button.RenderState{}))
	endImg := golden.Capture(t, canvasSize, scene(endState, tokens.DefaultLight, button.RenderState{}))
	if n := golden.PixelDiff(startImg, endImg); n == 0 {
		t.Error("Enter start and end render identically; expected the animation to change pixels")
	}
}

// TestExitEndDistinctFromStart mirrors TestEnterEndDistinctFromStart
// for the Exit primitive.
func TestExitEndDistinctFromStart(t *testing.T) {
	startImg := golden.Capture(t, canvasSize, scene(runExit(motion.Options{}, 0), tokens.DefaultLight, button.RenderState{}))
	endImg := golden.Capture(t, canvasSize, scene(runExit(motion.Options{}, 2*motion.DefaultFrames), tokens.DefaultLight, button.RenderState{}))
	if n := golden.PixelDiff(startImg, endImg); n == 0 {
		t.Error("Exit start and end render identically; expected the animation to change pixels")
	}
}

// TestSwapMidShowsBoth asserts the mid-swap frame differs from both
// the swap-start frame (only outgoing) and the swap-end frame (only
// incoming). Catches a regression where Transition silently runs only
// one half (e.g., Tick forgets to advance one side).
func TestSwapMidShowsBoth(t *testing.T) {
	startOut, startIn := runTransition(motion.Options{}, 0)
	midOut, midIn := runTransition(motion.Options{}, motion.DefaultFrames/2)
	endOut, endIn := runTransition(motion.Options{}, 2*motion.DefaultFrames)

	startImg := golden.Capture(t, canvasSize, swapScene(startOut, tokens.DefaultLight, startIn, tokens.DefaultDark))
	midImg := golden.Capture(t, canvasSize, swapScene(midOut, tokens.DefaultLight, midIn, tokens.DefaultDark))
	endImg := golden.Capture(t, canvasSize, swapScene(endOut, tokens.DefaultLight, endIn, tokens.DefaultDark))

	if n := golden.PixelDiff(startImg, midImg); n == 0 {
		t.Error("swap-mid renders identically to swap-start; expected the incoming side to begin contributing")
	}
	if n := golden.PixelDiff(midImg, endImg); n == 0 {
		t.Error("swap-mid renders identically to swap-end; expected the outgoing side to still be visible")
	}
}

// ---- determinism ----

// TestEnterDeterminism asserts two independently-constructed Enter
// primitives advanced through the same tick schedule produce
// bit-identical State sequences. Catches accidental nondeterminism in
// the underlying spring or tween (e.g., a global RNG creeping in).
func TestEnterDeterminism(t *testing.T) {
	a := motion.NewEnter(motion.Options{})
	b := motion.NewEnter(motion.Options{})
	for n := range 90 {
		a.Tick(invDt)
		b.Tick(invDt)
		if a.State() != b.State() {
			t.Fatalf("nondeterminism at frame %d: a=%+v b=%+v", n, a.State(), b.State())
		}
	}
}

// TestEnterEndIsSettled asserts the Enter primitive reports Settled
// once both its opacity tween and scale spring have reached their
// targets. The tolerance is generous (0.01) because [DefaultSpring]
// is critically damped — it asymptotes rather than ringing in, so the
// final approach is exponential.
func TestEnterEndIsSettled(t *testing.T) {
	e := motion.NewEnter(motion.Options{})
	for range 4 * motion.DefaultFrames { // 4× margin
		e.Tick(invDt)
	}
	if !e.Settled(0.01) {
		t.Errorf("Enter not settled after 4×DefaultFrames ticks; State=%+v", e.State())
	}
}

// TestExitEndIsSettled mirrors TestEnterEndIsSettled for Exit.
func TestExitEndIsSettled(t *testing.T) {
	e := motion.NewExit(motion.Options{})
	for range 4 * motion.DefaultFrames {
		e.Tick(invDt)
	}
	if !e.Settled(0.01) {
		t.Errorf("Exit not settled after 4×DefaultFrames ticks; State=%+v", e.State())
	}
}

// TestApplyDimensionsStableAcrossOpacity asserts the dimensions
// returned by Apply do not depend on the opacity value — even at
// Opacity=0, the underlying widget is still laid out so the parent
// layout does not jitter mid-animation.
func TestApplyDimensionsStableAcrossOpacity(t *testing.T) {
	w := renderBtn(tokens.DefaultLight, button.RenderState{})

	measure := func(s motion.State) layout.Dimensions {
		var ops op.Ops
		gtx := layout.Context{
			Constraints: layout.Exact(buttonSize),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Ops:         &ops,
		}
		return motion.Apply(gtx, s, w)
	}

	at0 := measure(motion.State{Opacity: 0, Scale: 1})
	at05 := measure(motion.State{Opacity: 0.5, Scale: 1})
	at1 := measure(motion.State{Opacity: 1, Scale: 1})
	if at0 != at05 || at05 != at1 {
		t.Errorf("Apply dims vary with opacity: 0=%+v 0.5=%+v 1=%+v", at0, at05, at1)
	}
}
