// Package motion provides enter/exit/transition primitives for animating
// widgets into, out of, and between visual states.
//
// Each primitive composes [github.com/vibrantgio/pulse/tween]
// (deterministic, frame-indexed opacity) and
// [github.com/vibrantgio/pulse/spring] (physics-driven scale): opacity
// provides predictable timing, spring provides physical character. The
// two run on independent timescales — opacity finishes at frame Frames,
// scale settles when the spring's restoring force balances out.
//
// # Composition with widgets
//
// [Apply] is the bridge from a [State] to a Gio [layout.Widget]. It
// records the widget once (to obtain its layout footprint), then replays
// the recorded ops inside an [op.Affine] scale-around-centre transform
// and a [paint.PushOpacity] layer. The widget is laid out exactly once;
// the visual transformation is purely a paint-time effect.
//
//	enter := motion.NewEnter(motion.Options{})
//	for !enter.Settled(0.005) {
//	    enter.Tick(60)            // 60 Hz frame loop
//	    motion.Apply(gtx, enter.State(), buttonWidget)
//	}
//
// # Variant pattern
//
// Pulse never decorates prism globally. A motion-aware component is an
// explicit variant, exported alongside its prism counterpart and chosen
// by name at the call site, so that reading the call tells you the
// component animates and the dependency runs pulse → prism and never
// back. [Apply] is the mechanism: wrap a prism render function with it.
//
//	bw := button.Render(shaper, label, colors, sp, rad, ts, btnState)
//	motion.Apply(gtx, primitive.State(), bw)
//
// This package ships only the primitives. The one variant that exists
// today is [github.com/vibrantgio/pulse/springbutton], and it is built
// on spring directly rather than on Apply.
//
// # The defaults do not line up
//
// [DefaultFrames] and [DefaultSpring] were meant to finish together and
// do not. Measured with invDt=60: [NewEnter] on a zero [Options] has an
// opacity tween complete at frame 30, and its scale spring does not
// reach Settled(0.005) until frame 52 — scale is still 0.991 when the
// fade ends. A caller looping until [Enter.Settled] therefore runs 22
// frames past the visible end of the animation, and a caller that stops
// at Frames leaves the widget fractionally undersized.
//
// [Options.Spring] has a sharper edge. It falls back to [DefaultSpring]
// only when the whole struct is zero. Set one field — Spring:
// spring.Options{Stiffness: 200} — and the other two do not come from
// [DefaultSpring] at all; they come from
// [github.com/vibrantgio/pulse/spring]'s own per-field defaults, which
// are a much softer spring. The result is a damping ratio near 0.02
// that rings for thousands of frames. Override all three fields or
// none.
package motion

import (
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"

	"github.com/vibrantgio/pulse/spring"
	"github.com/vibrantgio/pulse/tween"
	"github.com/vibrantgio/spectrum/tokens"
)

// State captures the visual transformation applied to a widget at one
// instant of an animation. Both fields are normalised: Opacity in [0,1],
// Scale as a multiplier (1.0 = identity).
type State struct {
	// Opacity in [0,1]: 0 = fully transparent, 1 = fully opaque. Drives
	// a [paint.PushOpacity] layer in [Apply].
	Opacity float64

	// Scale factor: 1.0 = identity. Drives an [op.Affine] scale around
	// the widget's centre in [Apply], so 0.85 shrinks the widget toward
	// its midpoint without changing its layout footprint.
	Scale float64
}

// Visible is the canonical end-state of an Enter and the starting state
// of an Exit: fully opaque, identity scale.
var Visible = State{Opacity: 1, Scale: 1}

// Hidden is the canonical end-state of an Exit and the starting state of
// an Enter: fully transparent, scaled down to [DefaultFromScale].
var Hidden = State{Opacity: 0, Scale: DefaultFromScale}

// Defaults applied when a corresponding [Options] field is zero-valued.
// Since E3.1 the timing defaults resolve from the spectrum motion tokens
// ([tokens.Motion], the value every theme.Theme.Motion emits by default)
// rather than from local constants.
const (
	// DefaultFromScale is the scale a widget starts at on Enter (and
	// returns to on Exit).
	DefaultFromScale = 0.85

	// defaultFPS is the 60 Hz reference frame rate the token durations
	// convert to frame counts at.
	defaultFPS = 60
)

// DefaultFrames is the opacity-tween duration in frames: the motion
// scale's slowest stop (DurXSlow, MD3 long2 = 500 ms) at the 60 Hz
// reference rate — 30 frames, the same count as the constant it
// replaced, because E3.1 pinned DurXSlow to the 500 ms this package
// already animated over.
var DefaultFrames = FramesAt(tokens.Motion.DurXSlow, defaultFPS)

// FramesAt converts a [tokens.MotionScale] duration stop into a whole
// frame count at the given frame rate, rounding to nearest. It is how a
// theme-driven caller derives [Options.Frames] from its Theme.Motion
// snapshot:
//
//	opts := motion.Options{Frames: motion.FramesAt(m.DurNormal, 60)}
func FramesAt(d time.Duration, fps float64) int {
	return int(math.Round(d.Seconds() * fps))
}

// SpringOptions converts a [tokens.Spring] preset from the theme's motion
// scale into [spring.Options] for this package and its siblings.
func SpringOptions(s tokens.Spring) spring.Options {
	return spring.Options{
		Stiffness: float64(s.Stiffness),
		Damping:   float64(s.Damping),
		Mass:      float64(s.Mass),
	}
}

// DefaultSpring is the [spring.Options] used when [Options.Spring] is
// zero-valued — and only when it is entirely zero-valued. It is the
// motion scale's SpringDefault preset: critically damped (c = 2·√(k·m))
// at k=80, m=1 — a brisk, no-overshoot curve that reaches
// Settled(0.005) at frame 52 with invDt=60, which is 22 frames past
// [DefaultFrames] rather than level with it.
var DefaultSpring = SpringOptions(tokens.Motion.SpringDefault)

// Options configures a single motion primitive. Zero-valued fields are
// replaced with package defaults at construction time, so the zero
// [Options] value produces a working primitive with the canonical feel.
type Options struct {
	// Frames is the opacity-tween duration in frames. Zero defaults to
	// [DefaultFrames]. The scale spring runs alongside on its own
	// physical timescale.
	Frames int

	// Spring is the [spring.Options] for the scale animation. Only the
	// wholly zero value is replaced with [DefaultSpring]. Setting a
	// single field opts the other two out of [DefaultSpring] as well and
	// into [spring.New]'s much softer per-field defaults, so partial
	// overrides do not do what they look like they do — set all three
	// fields or none.
	Spring spring.Options

	// FromScale is the scale at the Hidden end of the animation. Zero
	// defaults to [DefaultFromScale].
	FromScale float64
}

func (o Options) frames() int {
	if o.Frames <= 0 {
		return DefaultFrames
	}
	return o.Frames
}

func (o Options) fromScale() float64 {
	if o.FromScale <= 0 {
		return DefaultFromScale
	}
	return o.FromScale
}

func (o Options) springOpts() spring.Options {
	if o.Spring == (spring.Options{}) {
		return DefaultSpring
	}
	return o.Spring
}

// Enter animates a widget from [Hidden] to [Visible]. Construct with
// [NewEnter], advance with [Enter.Tick], query with [Enter.State] or
// [Enter.Settled].
type Enter struct {
	opacity tween.Tween[float64]
	scale   *spring.Spring
	frame   int
}

// NewEnter constructs an Enter primitive with opts.
func NewEnter(opts Options) *Enter {
	return &Enter{
		opacity: tween.Tween[float64]{From: 0, To: 1, Frames: opts.frames(), Lerp: tween.LerpFloat64},
		scale:   spring.New(opts.fromScale(), 1, opts.springOpts()),
	}
}

// Tick advances the animation by one simulation step. invDt mirrors
// [spring.Spring.Tick]: the DESIGN-recommended frame loop convention is
// max(1, fps/30).
func (e *Enter) Tick(invDt float64) {
	e.frame++
	e.scale.Tick(invDt)
}

// State returns the current visual transformation.
func (e *Enter) State() State {
	return State{
		Opacity: e.opacity.At(e.frame),
		Scale:   e.scale.Value(),
	}
}

// Frame returns the current frame index (0 before any [Enter.Tick]).
func (e *Enter) Frame() int { return e.frame }

// Settled reports whether the animation has reached [Visible] within
// tolerance: the opacity tween has finished AND the scale spring is at
// rest at 1.0.
func (e *Enter) Settled(tol float64) bool {
	return e.frame >= e.opacity.Frames && e.scale.Settled(tol)
}

// Exit animates a widget from [Visible] to [Hidden]. Mirrors [Enter].
type Exit struct {
	opacity tween.Tween[float64]
	scale   *spring.Spring
	frame   int
}

// NewExit constructs an Exit primitive with opts.
func NewExit(opts Options) *Exit {
	return &Exit{
		opacity: tween.Tween[float64]{From: 1, To: 0, Frames: opts.frames(), Lerp: tween.LerpFloat64},
		scale:   spring.New(1, opts.fromScale(), opts.springOpts()),
	}
}

// Tick advances the animation by one simulation step.
func (e *Exit) Tick(invDt float64) {
	e.frame++
	e.scale.Tick(invDt)
}

// State returns the current visual transformation.
func (e *Exit) State() State {
	return State{
		Opacity: e.opacity.At(e.frame),
		Scale:   e.scale.Value(),
	}
}

// Frame returns the current frame index.
func (e *Exit) Frame() int { return e.frame }

// Settled reports whether the animation has reached [Hidden] within
// tolerance: opacity finished AND scale spring at rest at FromScale.
func (e *Exit) Settled(tol float64) bool {
	return e.frame >= e.opacity.Frames && e.scale.Settled(tol)
}

// Transition cross-fades between two widgets — an outgoing widget that
// runs an [Exit] and an incoming widget that runs an [Enter] in parallel,
// both ticked together.
type Transition struct {
	out *Exit
	in  *Enter
}

// NewTransition constructs a Transition with opts. Both Exit and Enter
// halves use the same opts.
func NewTransition(opts Options) *Transition {
	return &Transition{
		out: NewExit(opts),
		in:  NewEnter(opts),
	}
}

// Tick advances both halves by one simulation step.
func (t *Transition) Tick(invDt float64) {
	t.out.Tick(invDt)
	t.in.Tick(invDt)
}

// Out returns the outgoing widget's current state.
func (t *Transition) Out() State { return t.out.State() }

// In returns the incoming widget's current state.
func (t *Transition) In() State { return t.in.State() }

// Frame returns the current frame index (incremented once per
// [Transition.Tick]).
func (t *Transition) Frame() int { return t.in.Frame() }

// Settled reports whether both halves have reached their respective
// resting states within tolerance.
func (t *Transition) Settled(tol float64) bool {
	return t.out.Settled(tol) && t.in.Settled(tol)
}

// Apply renders w with the visual transformation s applied: an
// [op.Affine] scale around the widget's centre and a [paint.PushOpacity]
// layer. Returns w's natural layout dimensions (the visual scale does
// not change the widget's footprint).
//
// w is laid out exactly once: Apply records w into a macro to obtain
// its dimensions, then replays the macro inside the transform/opacity
// stack. Dimensions therefore stay stable across an animation, so
// surrounding layout does not jitter.
//
// Apply does not short-circuit on Opacity == 0 — the widget is still
// laid out and its ops are still recorded so the returned dimensions
// remain the true widget footprint at every frame.
func Apply(gtx layout.Context, s State, w layout.Widget) layout.Dimensions {
	rec := op.Record(gtx.Ops)
	dims := w(gtx)
	call := rec.Stop()

	centre := f32.Pt(float32(dims.Size.X)/2, float32(dims.Size.Y)/2)
	factor := f32.Pt(float32(s.Scale), float32(s.Scale))

	trans := op.Affine(f32.Affine2D{}.Scale(centre, factor)).Push(gtx.Ops)
	opStack := paint.PushOpacity(gtx.Ops, clampUnit(s.Opacity))
	call.Add(gtx.Ops)
	opStack.Pop()
	trans.Pop()

	return dims
}

// clampUnit clamps x to [0, 1] and converts to float32 for paint APIs.
func clampUnit(x float64) float32 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	return float32(x)
}
