package spring_test

import (
	"math"
	"testing"

	"github.com/vibrantgio/pulse/spring"
)

// criticalDamping returns 2·√(k·m), the damping coefficient at which
// the spring settles fastest without overshooting. Used as the seed
// for the headline settling test.
func criticalDamping(k, m float64) float64 {
	return 2 * math.Sqrt(k*m)
}

// TestSpringSettlesWithinToleranceUnderFixedSeed is the milestone
// G3.2 Measurable criterion: the spring settles within tolerance when
// driven from deterministic initial conditions and parameters ("fixed
// seed" — there is no RNG; the seed here is the parameter set and
// frame schedule).
//
// The seed: critically-damped spring, k=20, m=1, c=2·√20; start=0,
// target=1; 200 ticks at invDt=60 (a 60 Hz frame loop with dt=1/60 s,
// totalling ~3.33 s of simulated time). Critical damping puts the
// settling time at ~2 s; 3.33 s leaves comfortable margin.
//
// Tolerance: 0.005 on |value − target| and 0.05 on |velocity|. These
// are tight enough to fail if the integrator silently regresses to a
// non-converging step, and loose enough that they don't depend on
// the exact bit pattern of velocity-Verlet rounding.
func TestSpringSettlesWithinToleranceUnderFixedSeed(t *testing.T) {
	const (
		start  = 0.0
		target = 1.0
		k      = 20.0
		m      = 1.0
		invDt  = 60.0
		ticks  = 200
		posTol = 0.005
		velTol = 0.05
	)
	sp := spring.New(start, target, spring.Options{
		Stiffness: k,
		Damping:   criticalDamping(k, m),
		Mass:      m,
	})
	for range ticks {
		sp.Tick(invDt)
	}
	if got := math.Abs(sp.Value() - target); got > posTol {
		t.Errorf("|value - target| = %v after %d ticks, want <= %v (value=%v)",
			got, ticks, posTol, sp.Value())
	}
	if got := math.Abs(sp.Velocity()); got > velTol {
		t.Errorf("|velocity| = %v after %d ticks, want <= %v", got, ticks, velTol)
	}
	if !sp.Settled(posTol) {
		t.Errorf("Settled(%v) = false after %d ticks; value=%v, velocity=%v",
			posTol, ticks, sp.Value(), sp.Velocity())
	}
}

// TestSpringDeterminism asserts bit-identical trajectories from two
// independently constructed springs that share parameters and tick
// schedule. Catches accidental nondeterminism creeping into the
// simulation (e.g., a map iteration in the integrator, a shared
// global state).
func TestSpringDeterminism(t *testing.T) {
	opts := spring.Options{Stiffness: 20, Damping: criticalDamping(20, 1), Mass: 1}
	a := spring.New(0, 1, opts)
	b := spring.New(0, 1, opts)
	for n := range 200 {
		a.Tick(60)
		b.Tick(60)
		if a.Value() != b.Value() {
			t.Fatalf("nondeterminism: frame %d, a.Value=%v b.Value=%v", n, a.Value(), b.Value())
		}
		if a.Velocity() != b.Velocity() {
			t.Fatalf("nondeterminism: frame %d, a.Vel=%v b.Vel=%v", n, a.Velocity(), b.Velocity())
		}
	}
}

// TestSpringStaysSettled asserts that once the spring meets the
// settle predicate, further ticks do not push it back out. Guards a
// regression where, for instance, a numerical drift in the integrator
// could re-energise a settled system.
func TestSpringStaysSettled(t *testing.T) {
	sp := spring.New(0, 1, spring.Options{
		Stiffness: 20, Damping: criticalDamping(20, 1), Mass: 1,
	})
	for range 200 {
		sp.Tick(60)
	}
	if !sp.Settled(0.005) {
		t.Fatalf("did not settle in setup; value=%v vel=%v", sp.Value(), sp.Velocity())
	}
	for n := range 600 {
		sp.Tick(60)
		if !sp.Settled(0.01) {
			t.Errorf("settled spring drifted out of tolerance at extra frame %d: value=%v vel=%v",
				n, sp.Value(), sp.Velocity())
		}
	}
}

// TestSpringSetTargetMidFlight asserts that retargeting an in-flight
// spring causes it to converge to the new target. The free particle's
// position and velocity carry over (no reset), modelling a smooth
// blend from the old trajectory into the new pull.
func TestSpringSetTargetMidFlight(t *testing.T) {
	sp := spring.New(0, 1, spring.Options{
		Stiffness: 20, Damping: criticalDamping(20, 1), Mass: 1,
	})
	for range 30 {
		sp.Tick(60)
	}
	mid := sp.Value()
	if mid >= 1 {
		t.Fatalf("setup expected free particle still en route to 1, got value=%v", mid)
	}

	sp.SetTarget(2)
	if got := sp.Target(); got != 2 {
		t.Errorf("Target() after SetTarget(2) = %v, want 2", got)
	}

	for range 200 {
		sp.Tick(60)
	}
	if got := math.Abs(sp.Value() - 2); got > 0.005 {
		t.Errorf("after retarget to 2: |value - 2| = %v, want <= 0.005 (value=%v)",
			got, sp.Value())
	}
	if got := math.Abs(sp.Velocity()); got > 0.05 {
		t.Errorf("after retarget to 2: |velocity| = %v, want <= 0.05", got)
	}
}

// TestUnderdampedOvershoots checks the wiring of the physics: a
// lightly damped spring released from rest must overshoot its target
// at least once. If this test fails, we are not running a real
// damped-spring simulation (e.g., we accidentally clamped the value,
// or used a tween instead of physics).
func TestUnderdampedOvershoots(t *testing.T) {
	sp := spring.New(0, 1, spring.Options{
		Stiffness: 20,
		Damping:   1, // zeta ≈ 0.11, deeply underdamped
		Mass:      1,
	})
	maxValue := math.Inf(-1)
	for range 200 {
		sp.Tick(60)
		if v := sp.Value(); v > maxValue {
			maxValue = v
		}
	}
	if maxValue <= 1 {
		t.Errorf("underdamped spring did not overshoot target=1; max value seen = %v", maxValue)
	}
}

// TestNewInitialState asserts the constructor stores the start
// position with zero velocity, before any tick has run.
func TestNewInitialState(t *testing.T) {
	sp := spring.New(7, 42, spring.Options{Stiffness: 1, Damping: 1, Mass: 1})
	if got := sp.Value(); got != 7 {
		t.Errorf("initial Value() = %v, want 7", got)
	}
	if got := sp.Velocity(); got != 0 {
		t.Errorf("initial Velocity() = %v, want 0", got)
	}
	if got := sp.Target(); got != 42 {
		t.Errorf("initial Target() = %v, want 42", got)
	}
}

// TestNewAppliesDefaults asserts zero-valued option fields are
// replaced with the package defaults rather than left at zero (which
// would produce a degenerate spring with no force or no mass).
func TestNewAppliesDefaults(t *testing.T) {
	sp := spring.New(0, 1, spring.Options{})
	for range 5000 {
		sp.Tick(60)
	}
	if !sp.Settled(0.01) {
		t.Errorf("default-options spring did not settle; value=%v vel=%v (defaults must produce a working spring)",
			sp.Value(), sp.Velocity())
	}
}
