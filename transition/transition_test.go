package transition_test

import (
	"fmt"
	"image/color"
	"reflect"
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

// TestLerpColorTokensCoversEveryField proves LerpColorTokens interpolates
// literally every colour leaf of tokens.ColorTokens — all forty-five ramp
// steps, every pin and on-colour, the semantic layer, and the deprecated
// aliases. It builds two token sets whose every leaf holds a distinct
// non-zero colour via reflection, then requires lerp at t=0 and t=1 to
// reproduce each endpoint exactly: a field LerpColorTokens misses stays at
// its zero value and is reported by path. Because the walk enumerates the
// struct via reflection, adding a field to ColorTokens without teaching
// LerpColorTokens about it fails this test rather than silently snapping.
func TestLerpColorTokensCoversEveryField(t *testing.T) {
	var from, to tokens.ColorTokens
	n := uint32(1)
	fillDistinct(t, reflect.ValueOf(&from).Elem(), &n)
	fillDistinct(t, reflect.ValueOf(&to).Elem(), &n)

	if got := transition.LerpColorTokens(from, to, 0); got != from {
		t.Errorf("LerpColorTokens(from, to, 0) != from; unlerped fields: %v",
			diffLeaves(reflect.ValueOf(got), reflect.ValueOf(from)))
	}
	if got := transition.LerpColorTokens(from, to, 1); got != to {
		t.Errorf("LerpColorTokens(from, to, 1) != to; unlerped fields: %v",
			diffLeaves(reflect.ValueOf(got), reflect.ValueOf(to)))
	}
}

var nrgbaType = reflect.TypeOf(color.NRGBA{})

// fillDistinct assigns a unique, fully opaque, non-zero NRGBA to every
// colour leaf reachable from v, recursing through nested structs and
// arrays (the RampSet and its Ramp arrays). Any leaf that is not a
// color.NRGBA fails the test: a future non-colour field in ColorTokens
// needs an explicit decision here and in LerpColorTokens.
func fillDistinct(t *testing.T, v reflect.Value, n *uint32) {
	t.Helper()
	switch {
	case v.Type() == nrgbaType:
		v.Set(reflect.ValueOf(color.NRGBA{
			R: uint8(*n), G: uint8(*n >> 8), B: uint8(*n >> 16), A: 0xFF,
		}))
		*n++
	case v.Kind() == reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			fillDistinct(t, v.Field(i), n)
		}
	case v.Kind() == reflect.Array:
		for i := 0; i < v.Len(); i++ {
			fillDistinct(t, v.Index(i), n)
		}
	default:
		t.Fatalf("ColorTokens carries a non-colour leaf of type %v; extend LerpColorTokens and this test", v.Type())
	}
}

// diffLeaves reports the paths of colour leaves where got differs from want.
func diffLeaves(got, want reflect.Value) []string {
	var diffs []string
	var walk func(path string, g, w reflect.Value)
	walk = func(path string, g, w reflect.Value) {
		switch {
		case g.Type() == nrgbaType:
			if !g.Equal(w) {
				diffs = append(diffs, path)
			}
		case g.Kind() == reflect.Struct:
			for i := 0; i < g.NumField(); i++ {
				walk(path+"."+g.Type().Field(i).Name, g.Field(i), w.Field(i))
			}
		case g.Kind() == reflect.Array:
			for i := 0; i < g.Len(); i++ {
				walk(fmt.Sprintf("%s[%d]", path, i), g.Index(i), w.Index(i))
			}
		}
	}
	walk("ColorTokens", got, want)
	return diffs
}

func TestColorTokensTweenSettlesAtTarget(t *testing.T) {
	// The "tween settles to target" clause from G2.3 Measurable, asserted
	// at the value-equality level (not just pixel equality).
	tw := transition.ColorTokensTween(tokens.DefaultLight, tokens.DefaultDark, 30)
	if got := tw.At(30); got != tokens.DefaultDark {
		t.Errorf("Tween.At(Frames) did not settle to target: got %+v, want %+v", got, tokens.DefaultDark)
	}
}
