package conductor_test

import (
	"testing"

	"github.com/vibrantgio/effects/conductor"
	"github.com/vibrantgio/effects/tween"
)

// TestStaggeredListRevealPhaseLocked is the G3.5 Measurable criterion:
// "test demonstrates staggered list reveal across N rows phase-locked."
//
// Fixture: N rows, each running an identical opacity tween from 0 to 1
// over D frames; row i has stagger offset i*S, so it begins emerging
// at conductor frame i*S+1. All rows read from a single Conductor.
//
// Three properties together demonstrate "phase-locked staggered list
// reveal":
//
//  1. Stagger — row i remains at the From value (0) until conductor
//     frame > i*S, i.e., the row only begins moving once its slot opens.
//  2. Phase-lock — row j at conductor frame f equals row i at conductor
//     frame f − (j−i)*S for every valid (f, i, j). Two participants
//     share an identical motion trajectory time-shifted by the
//     difference of their offsets.
//  3. Settling — at conductor frame (N−1)*S + D + buffer, every row
//     has reached the To value (1).
func TestStaggeredListRevealPhaseLocked(t *testing.T) {
	const (
		N = 5  // rows
		D = 30 // tween duration in frames per row
		S = 8  // stagger delay (frames) between consecutive rows
	)
	cond := conductor.New()
	rows := make([]tween.Tween[float64], N)
	for i := range rows {
		rows[i] = tween.Tween[float64]{
			From: 0, To: 1, Frames: D, Lerp: tween.LerpFloat64,
		}
	}

	total := (N-1)*S + D + 10 // last row needs (N-1)*S + D frames; +10 settle margin

	snap := func() []float64 {
		row := make([]float64, N)
		for i := range rows {
			row[i] = rows[i].At(cond.Local(i * S))
		}
		return row
	}

	snapshots := make([][]float64, total+1)
	snapshots[0] = snap()
	for k := 1; k <= total; k++ {
		cond.Tick()
		snapshots[k] = snap()
	}

	// Property 1 — stagger: row i is still at From for every conductor
	// frame f in [0, i*S]. It begins emerging at f = i*S + 1.
	for i := range rows {
		for f := 0; f <= i*S; f++ {
			if got := snapshots[f][i]; got != 0 {
				t.Errorf("stagger: row[%d] at frame %d = %v, want 0 (still pre-stagger; offset=%d)",
					i, f, got, i*S)
			}
		}
	}

	// Property 2 — phase-lock: row j at frame f equals row i at frame
	// f − (j−i)*S. Asserted across every (f, i, j) where both indices
	// are in range.
	for f := 0; f <= total; f++ {
		for i := 0; i < N; i++ {
			for j := i + 1; j < N; j++ {
				shift := (j - i) * S
				if f < shift {
					continue
				}
				ref := snapshots[f-shift][i]
				got := snapshots[f][j]
				if got != ref {
					t.Errorf("phase-lock: row[%d] at frame %d = %v, want row[%d] at frame %d = %v (shift=%d)",
						j, f, got, i, f-shift, ref, shift)
				}
			}
		}
	}

	// Property 3 — settling: every row has reached To at the final
	// frame. (N−1)*S + D is the latest row's settling frame; the +10
	// buffer ensures we're past it.
	for i := range rows {
		if got := snapshots[total][i]; got != 1 {
			t.Errorf("settling: row[%d] at final frame %d = %v, want 1 (To)", i, total, got)
		}
	}

	// Sanity: at the start, no row has moved yet.
	for i := range rows {
		if got := snapshots[0][i]; got != 0 {
			t.Errorf("initial: row[%d] at frame 0 = %v, want 0 (no Tick yet)", i, got)
		}
	}
}

// TestNewIsAtFrameZero asserts the constructor returns a Conductor
// at frame 0, before any Tick has been applied.
func TestNewIsAtFrameZero(t *testing.T) {
	c := conductor.New()
	if got := c.Frame(); got != 0 {
		t.Errorf("New().Frame() = %d, want 0", got)
	}
}

// TestTickAdvancesFrame asserts each Tick increments Frame by exactly
// one. The conductor is a pure counter with no rate-shaping.
func TestTickAdvancesFrame(t *testing.T) {
	c := conductor.New()
	for want := 1; want <= 10; want++ {
		c.Tick()
		if got := c.Frame(); got != want {
			t.Errorf("after %d Tick calls, Frame() = %d, want %d", want, got, want)
		}
	}
}

// TestLocalBeforeOffset asserts Local returns 0 while the conductor
// frame is below the participant's offset — the participant has not
// yet started.
func TestLocalBeforeOffset(t *testing.T) {
	c := conductor.New()
	for range 4 {
		c.Tick()
	}
	if got := c.Local(5); got != 0 {
		t.Errorf("Local(5) at frame 4 = %d, want 0 (still pre-stagger)", got)
	}
	c.Tick() // frame=5, == offset
	if got := c.Local(5); got != 0 {
		t.Errorf("Local(5) at frame 5 = %d, want 0 (slot opens at frame > offset)", got)
	}
}

// TestLocalAtAndAfterOffset asserts Local returns frame − offset once
// the conductor passes the offset, including the boundary frame.
func TestLocalAtAndAfterOffset(t *testing.T) {
	c := conductor.New()
	for range 8 {
		c.Tick()
	}
	if got := c.Local(5); got != 3 {
		t.Errorf("Local(5) at frame 8 = %d, want 3", got)
	}
	if got := c.Local(0); got != 8 {
		t.Errorf("Local(0) at frame 8 = %d, want 8 (offset 0 means no stagger)", got)
	}
}

// TestLocalZeroOffsetTracksFrame asserts Local(0) is a thin wrapper
// over Frame — a participant with no stagger reads its local frame
// straight from the conductor.
func TestLocalZeroOffsetTracksFrame(t *testing.T) {
	c := conductor.New()
	for k := range 30 {
		if got, want := c.Local(0), k; got != want {
			t.Errorf("Local(0) at frame %d = %d, want %d", k, got, want)
		}
		c.Tick()
	}
}
