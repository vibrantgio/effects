package springbutton

import (
	"math"
	"testing"
	"time"

	"github.com/vibrantgio/theme/tokens"
)

// TestSpringPositionDependsOnFrameTime holds the time-stepping contract: the
// spring's position after a frame depends on the frame's elapsed time, and
// the same wall-clock time gives about the same position at any frame rate.
func TestSpringPositionDependsOnFrameTime(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	after := func(frames int, every time.Duration) float64 {
		sp := newSpring(1, tokens.Motion.SpringSnappy)
		sp.SetTarget(DefaultPressScale)
		last := t0
		for i := 1; i <= frames; i++ {
			now := t0.Add(time.Duration(i) * every)
			advance(sp, frameStep(last, now))
			last = now
		}
		return sp.Value()
	}

	at60 := after(1, time.Second/60)
	at120 := after(1, time.Second/120)
	if at60 == at120 {
		t.Errorf("one frame of 1/60 s and one of 1/120 s left the spring at the same position %v", at60)
	}
	if at60 >= at120 {
		t.Errorf("the longer frame moved the spring less: 1/60 s at %v, 1/120 s at %v", at60, at120)
	}

	// 100 ms of wall-clock time at 60 Hz and at 120 Hz. The integrator's
	// step error makes the two differ by 0.0033, about 4 % of the press
	// span.
	six60 := after(6, time.Second/60)
	twelve120 := after(12, time.Second/120)
	if d := math.Abs(six60 - twelve120); d > 0.005 {
		t.Errorf("100 ms at 60 Hz left the spring at %v and at 120 Hz at %v: differ by %v, want <= 0.005",
			six60, twelve120, d)
	}
}

// TestFrameStepIsBounded holds the edges of frameStep: no previous frame and a
// clock that runs backwards step zero time, and a stalled frame steps at most
// maxFrameStep.
func TestFrameStepIsBounded(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name      string
		last, now time.Time
		want      time.Duration
	}{
		{"no previous frame", time.Time{}, t0, 0},
		{"no frame time", t0, time.Time{}, 0},
		{"clock runs backwards", t0, t0.Add(-time.Millisecond), 0},
		{"one 60 Hz frame", t0, t0.Add(time.Second / 60), time.Second / 60},
		{"stalled host", t0, t0.Add(10 * time.Second), maxFrameStep},
	} {
		if got := frameStep(c.last, c.now); got != c.want {
			t.Errorf("%s: frameStep = %v, want %v", c.name, got, c.want)
		}
	}
}
