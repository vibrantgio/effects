// Package conductor provides a shared clock for coordinated animation
// across widgets. Multiple participants — staggered list rows, paged
// transitions, choreographed reveals — read from a single Conductor so
// their relative phases remain deterministic.
//
// # Why a shared clock
//
// DESIGN §"5. Frame-Driven Physics — Caveats" notes that two widgets
// ticking their own ParticleSystem (or each maintaining their own
// frame counter) will not produce a coordinated wave: even
// bit-identical primitives diverge if they receive a different number
// of ticks. For coordinated effects — a wave of staggered rows
// revealing in lockstep, a synchronised cascade — participants must
// share a clock. The Conductor *is* that clock.
//
// # Stagger pattern
//
// Each participant has an offset (in frames): how many conductor
// frames after the start the participant should begin animating.
// [Conductor.Local] returns max(0, frame − offset) — the participant's
// local frame for indexing a [github.com/vibrantgio/pulse/tween.Tween]
// or any other frame-indexed value.
//
//	cond := conductor.New()
//	rows := make([]tween.Tween[float64], N)
//	for i := range rows {
//	    rows[i] = tween.Tween[float64]{From: 0, To: 1, Frames: D, Lerp: tween.LerpFloat64}
//	}
//	// per frame:
//	cond.Tick()
//	for i := range rows {
//	    opacity := rows[i].At(cond.Local(i * delay))
//	    // ... render row i with opacity ...
//	}
//
// Participants i and j with offsets oᵢ and oⱼ render bit-identical
// states at conductor frames oᵢ+k and oⱼ+k respectively — that is what
// "phase-locked" means here.
//
// # Threading
//
// A Conductor is not safe for concurrent use. Per DESIGN §"Threading
// rules", animation state lives on the Gio frame thread; one
// Conductor per coordinated group, ticked once per frame from layout.
package conductor

// Conductor is a shared frame counter. The zero value is a valid
// Conductor at frame 0; [Conductor.Tick] advances the counter and
// [Conductor.Frame] reports the current value.
type Conductor struct {
	frame int
}

// New constructs a Conductor at frame 0.
func New() *Conductor { return &Conductor{} }

// Tick advances the conductor by one frame. Call once per render-loop
// frame, before the participants read their local frame.
func (c *Conductor) Tick() { c.frame++ }

// Frame returns the current frame index. Starts at 0 (no [Conductor.Tick]
// applied) and is incremented by each call to Tick.
func (c *Conductor) Frame() int { return c.frame }

// Local returns the participant's local frame given its stagger
// offset. Conductor frames before offset return 0 — the participant
// has not yet started; conductor frames at or beyond offset return
// frame − offset, the participant's local frame for indexing a
// frame-indexed primitive such as [github.com/vibrantgio/pulse/tween.Tween.At].
func (c *Conductor) Local(offset int) int {
	if c.frame < offset {
		return 0
	}
	return c.frame - offset
}
