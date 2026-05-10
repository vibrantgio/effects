// Package motion provides enter/exit/transition primitives for animating
// widgets into, out of, and between visual states.
//
// Each primitive composes [pulse/tween] (deterministic, frame-indexed
// opacity) and [pulse/spring] (physics-driven scale): opacity provides
// predictable timing, spring provides physical character. The two run on
// independent timescales — opacity finishes at frame Frames, scale settles
// when the spring's restoring force balances out.
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
// Per DESIGN §"Phase 3 — Pulse" composition mechanism, Pulse exposes
// motion-aware *variants* of Prism components by wrapping their render
// output with [Apply]. A motion-animated button is just:
//
//	bw := button.Render(shaper, label, colors, sp, rad, ts, btnState)
//	motion.Apply(gtx, primitive.State(), bw)
//
// Exporting wrapper functions per Prism component is G3.6's job; this
// package ships only the primitives.
package motion

import (
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"

	"github.com/vibrantgio/pulse/spring"
	"github.com/vibrantgio/pulse/tween"
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
const (
	// DefaultFrames is the opacity-tween duration, in frames.
	DefaultFrames = 30

	// DefaultFromScale is the scale a widget starts at on Enter (and
	// returns to on Exit).
	DefaultFromScale = 0.85

	// defaultSpringStiffness and defaultSpringMass parameterise the
	// motion-package default spring. Pairing critical damping
	// (c = 2·√(k·m)) with k=80, m=1 settles the spring in ~30 frames at
	// invDt=60 — coordinated with [DefaultFrames] so opacity and scale
	// finish together under default options.
	defaultSpringStiffness = 80.0
	defaultSpringMass      = 1.0
)

// DefaultSpring is the [spring.Options] used when [Options.Spring] is
// zero-valued. Critically damped at k=80, m=1 — a brisk, no-overshoot
// curve that settles in ~30 frames at invDt=60 (matching
// [DefaultFrames]).
var DefaultSpring = spring.Options{
	Stiffness: defaultSpringStiffness,
	Damping:   2 * math.Sqrt(defaultSpringStiffness*defaultSpringMass),
	Mass:      defaultSpringMass,
}

// Options configures a single motion primitive. Zero-valued fields are
// replaced with package defaults at construction time, so the zero
// [Options] value produces a working primitive with the canonical feel.
type Options struct {
	// Frames is the opacity-tween duration in frames. Zero defaults to
	// [DefaultFrames]. The scale spring runs alongside on its own
	// physical timescale.
	Frames int

	// Spring is the [spring.Options] for the scale animation. The zero
	// value (all fields zero) is replaced with [DefaultSpring]; pass any
	// non-zero field to override individually via [spring.New]'s own
	// per-field defaulting.
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
