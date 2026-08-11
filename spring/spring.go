// Package spring provides physics-based motion for animating a scalar
// value toward a target. It is the Pulse layer's bridge to the traer
// particle system: every Spring owns a tiny [traer.ParticleSystem]
// containing one fixed anchor (at the target) and one free particle
// (whose 1D position is the animated value), connected by a damped
// linear spring.
//
// Reach for spring when motion needs to feel physical: button presses,
// list reveals, focus rings, drag inertia. For non-physical motion —
// fades, slides, simple colour interpolation —
// [github.com/vibrantgio/pulse/tween] is cheaper and more predictable.
//
// # The zero Options is a usable default
//
// [Options] fills a zero Stiffness from [DefaultStiffness] (80) and a
// zero Mass from [DefaultMass] (1) — the values published as theme's
// tokens.Motion.SpringDefault preset — and then, last, fills a zero
// Damping with critical damping 2·√(k·m) computed from whatever
// Stiffness and Mass resolved to. The result is a brisk, no-overshoot
// curve: measured at invDt=60, spring.New(0, 1, spring.Options{})
// reaches Settled(0.005) at frame 68, about 1.1 s of continuous
// redraw. Because the damping derivation runs after the other fields
// are defaulted, a one-field override behaves: Options{Stiffness: 80}
// is bit-identical to the zero Options, and Options{Stiffness: 300} is
// a critically damped k=300 spring rather than one that rings. Pass an
// explicit Damping below 2·√(k·m) when overshoot is wanted —
// springbutton's press "pop" is k=300, c=22.
//
// # Defer-scoped allocation
//
// A Spring is stateful. It must be allocated once per subscription and
// kept alive across emissions and frames; reconstructing it inside a
// per-emission map function or a per-frame layout function would reset
// the simulation every render. The canonical pattern — the same one
// every components component uses for its interaction state — is to allocate
// inside an rx.Defer closure:
//
//	rx.Defer(func() rx.Observable[layout.Widget] {
//	    sp := spring.New(0, 0, spring.Options{Stiffness: 20}) // damping derives critical: 2·√20
//	    return rx.Map(targets, func(target float64) layout.Widget {
//	        sp.SetTarget(target)
//	        return func(gtx layout.Context) layout.Dimensions {
//	            sp.Tick(math.Max(1, fps.Value/30))
//	            // ... render using sp.Value() ...
//	            if !sp.Settled(0.005) {
//	                gtx.Execute(op.InvalidateCmd{})
//	            }
//	            return layout.Dimensions{}
//	        }
//	    })
//	})
//
// # Time step
//
// [Spring.Tick] takes invDt — the inverse of the simulation step,
// matching [traer.ParticleSystem.Tick]. The convention across this
// module is max(1, fps/30): floor the step at 1 (ensuring a dt no
// larger than 1 s under starvation) and otherwise pass the instantaneous
// frame rate divided by 30, the original Traer Physics reference rate.
// This trades real-time accuracy for Verlet stability when the host
// stutters, and it is why a spring's duration is measured in frames
// here rather than in seconds.
package spring

import (
	"math"

	"github.com/vibrantgio/traer"
)

// Default parameters are the values of theme's
// tokens.Motion.SpringDefault preset (FX.2). They are hardcoded rather
// than imported: spring is a pure-physics package over traer, and the
// design-token surface stays out of it. There is no DefaultDamping —
// the default damping is a rule, not a number: a zero [Options.Damping]
// derives critical damping 2·√(k·m) from the resolved Stiffness and
// Mass, so partial overrides stay critically damped.
const (
	DefaultStiffness = 80.0
	DefaultMass      = 1.0
)

// Options configures a Spring at construction. A zero Stiffness or
// Mass is replaced with the package default; a zero Damping is derived
// as critical damping from the resolved Stiffness and Mass. Pass
// explicit values for any field whose default does not match the
// desired feel.
type Options struct {
	// Stiffness is the spring constant k in the textbook damped-spring
	// model m·ẍ = −k·x − c·ẋ. Higher values produce faster oscillation
	// and a stiffer pull toward the target.
	Stiffness float64

	// Damping is the linear damping coefficient c. Critical damping
	// (no overshoot, fastest settle without ringing) is c = 2·√(k·m).
	// Below critical, the spring oscillates; above, it creeps. Zero
	// derives critical damping from the resolved Stiffness and Mass,
	// so leaving it unset never rings.
	Damping float64

	// Mass of the free particle (m). Higher mass adds inertia,
	// slowing the response without changing the underlying frequency
	// ratio.
	Mass float64
}

// Spring animates a scalar value toward a target via a 1-D damped
// spring. The simulation is fully deterministic: identical construction
// arguments and identical [Spring.Tick] calls produce bit-identical
// trajectories.
type Spring struct {
	ps     *traer.ParticleSystem
	anchor *traer.Particle
	free   *traer.Particle
}

// New constructs a Spring whose value starts at start and is heading
// toward target. A zero Stiffness or Mass is replaced with the package
// default; then a zero Damping is derived — from the resolved values,
// so the derivation holds under partial overrides — as critical
// damping 2·√(Stiffness·Mass).
func New(start, target float64, opts Options) *Spring {
	if opts.Stiffness == 0 {
		opts.Stiffness = DefaultStiffness
	}
	if opts.Mass == 0 {
		opts.Mass = DefaultMass
	}
	if opts.Damping == 0 {
		opts.Damping = 2 * math.Sqrt(opts.Stiffness*opts.Mass)
	}
	ps := traer.NewParticleSystem(0, 0)
	anchor := ps.NewParticle(1, target, 0, 0)
	anchor.Fixed = true
	free := ps.NewParticle(opts.Mass, start, 0, 0)
	ps.NewSpring(anchor, free, opts.Stiffness, opts.Damping, 0)
	return &Spring{ps: ps, anchor: anchor, free: free}
}

// Value returns the current scalar value (the free particle's position
// along the spring axis).
func (s *Spring) Value() float64 { return s.free.Position.X }

// Velocity returns the current rate of change of the value.
func (s *Spring) Velocity() float64 { return s.free.Velocity.X }

// Target returns the value the spring is currently pulling toward.
func (s *Spring) Target() float64 { return s.anchor.Position.X }

// SetTarget changes the target without resetting the free particle's
// position or velocity, so the in-flight motion blends smoothly into
// the new trajectory.
func (s *Spring) SetTarget(target float64) {
	s.anchor.Position.X = target
}

// Tick advances the simulation by 1/invDt seconds. It returns the
// activity metric reported by the underlying particle system —
// √(Σ |v|²) across all particles — which callers can use to decide
// whether to schedule another frame.
func (s *Spring) Tick(invDt float64) float64 {
	return s.ps.Tick(invDt)
}

// Settled reports whether the spring is at rest within tolerance: the
// value is within tolerance of the target AND the velocity magnitude
// is within tolerance of zero. Both checks are required because a
// damped oscillator passes through the target at peak velocity on its
// way to settling.
func (s *Spring) Settled(tolerance float64) bool {
	return math.Abs(s.Value()-s.Target()) <= tolerance &&
		math.Abs(s.Velocity()) <= tolerance
}
