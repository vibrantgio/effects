package transition_test

import (
	"testing"

	"github.com/vibrantgio/pulse/transition"
	"github.com/vibrantgio/spectrum/tokens"
)

func TestLerpColorTokensEndpoints(t *testing.T) {
	if got := transition.LerpColorTokens(tokens.DefaultLight, tokens.DefaultDark, 0); got != tokens.DefaultLight {
		t.Errorf("LerpColorTokens(_, _, 0) != DefaultLight")
	}
	if got := transition.LerpColorTokens(tokens.DefaultLight, tokens.DefaultDark, 1); got != tokens.DefaultDark {
		t.Errorf("LerpColorTokens(_, _, 1) != DefaultDark")
	}
}

func TestColorTokensTweenSettlesAtTarget(t *testing.T) {
	// The "tween settles to target" clause from G2.3 Measurable, asserted
	// at the value-equality level (not just pixel equality).
	tw := transition.ColorTokensTween(tokens.DefaultLight, tokens.DefaultDark, 30)
	if got := tw.At(30); got != tokens.DefaultDark {
		t.Errorf("Tween.At(Frames) did not settle to target: got %+v, want %+v", got, tokens.DefaultDark)
	}
}
