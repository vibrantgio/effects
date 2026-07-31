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
// # The zero Options is not a usable default
//
// [Options] fills its zero fields from [DefaultStiffness],
// [DefaultDamping] and [DefaultMass], and that combination is far
// softer than any UI motion wants: measured at invDt=60, a
// spring.New(0, 1, spring.Options{}) does not reach Settled(0.005)
// until frame 873 — about fifteen seconds of continuous redraw. Neither
// consumer in this module goes near it; motion uses k=80 and
// springbutton k=300, and both set all three fields. Treat Options as
// required rather than optional, and pick the three values together —
// [Options.Damping] is only meaningful relative to
// [Options.Stiffness] and [Options.Mass], so overriding one field and
// inheriting the others is how a spring ends up ringing.
//
// # Defer-scoped allocation
//
// A Spring is stateful. It must be allocated once per subscription and
// kept alive across emissions and frames; reconstructing it inside a
// per-emission map function or a per-frame layout function would reset
// the simulation every render. The canonical pattern — the same one
// every prism component uses for its interaction state — is to allocate
// inside an rx.Defer closure:
//
//	rx.Defer(func() rx.Observable[layout.Widget] {
//	    sp := spring.New(0, 0, spring.Options{Stiffness: 20, Damping: 8.94})
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

// Default parameters describe a very soft, lightly underdamped spring:
// ζ ≈ 0.55, and slow enough at invDt=60 that [Spring.Settled] at
// tolerance 0.005 is not reached until frame 873. They are a fallback
// for a partially filled [Options], not a recommendation. Override all
// three.
const (
	DefaultStiffness = 0.4
	DefaultDamping   = 0.7
	DefaultMass      = 1.0
)

// Options configures a Spring at construction. Zero-valued fields are
// replaced with package defaults; pass explicit values for any field
// whose default does not match the desired feel.
type Options struct {
	// Stiffness is the spring constant k in the textbook damped-spring
	// model m·ẍ = −k·x − c·ẋ. Higher values produce faster oscillation
	// and a stiffer pull toward the target.
	Stiffness float64

	// Damping is the linear damping coefficient c. Critical damping
	// (no overshoot, fastest settle without ringing) is c = 2·√(k·m).
	// Below critical, the spring oscillates; above, it creeps.
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
// toward target. Zero-valued option fields are replaced with package
// defaults.
func New(start, target float64, opts Options) *Spring {
	if opts.Stiffness == 0 {
		opts.Stiffness = DefaultStiffness
	}
	if opts.Damping == 0 {
		opts.Damping = DefaultDamping
	}
	if opts.Mass == 0 {
		opts.Mass = DefaultMass
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
