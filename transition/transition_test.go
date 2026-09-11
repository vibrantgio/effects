package transition_test

import (
	"fmt"
	"image/color"
	"reflect"
	"testing"

	"github.com/vibrantgio/effects/transition"
	"github.com/vibrantgio/theme/tokens"
)

func TestLerpPlatformColorsEndpoints(t *testing.T) {
	if got := transition.LerpPlatformColors(tokens.PlatformLight, tokens.PlatformDark, 0); got != tokens.PlatformLight {
		t.Errorf("LerpPlatformColors(_, _, 0) != PlatformLight")
	}
	if got := transition.LerpPlatformColors(tokens.PlatformLight, tokens.PlatformDark, 1); got != tokens.PlatformDark {
		t.Errorf("LerpPlatformColors(_, _, 1) != PlatformDark")
	}
}

// TestLerpPlatformColorsCoversEveryField proves LerpPlatformColors
// interpolates literally every field of tokens.PlatformColors — every plane,
// selection, label, control, system colour and measured material. It builds
// two sets whose every field holds a distinct non-zero colour via reflection,
// then requires the lerp at t=0 and t=1 to reproduce each endpoint exactly: a
// field LerpPlatformColors misses stays at its zero value and is reported by
// name. Because the walk enumerates the struct via reflection, adding a name
// to the set without teaching this package about it fails here rather than
// silently snapping one element while the rest of the window cross-fades.
func TestLerpPlatformColorsCoversEveryField(t *testing.T) {
	var from, to tokens.PlatformColors
	n := uint32(1)
	fillDistinct(t, reflect.ValueOf(&from).Elem(), &n)
	fillDistinct(t, reflect.ValueOf(&to).Elem(), &n)

	if got := transition.LerpPlatformColors(from, to, 0); got != from {
		t.Errorf("LerpPlatformColors(from, to, 0) != from; unlerped fields: %v",
			diffLeaves(reflect.ValueOf(got), reflect.ValueOf(from)))
	}
	if got := transition.LerpPlatformColors(from, to, 1); got != to {
		t.Errorf("LerpPlatformColors(from, to, 1) != to; unlerped fields: %v",
			diffLeaves(reflect.ValueOf(got), reflect.ValueOf(to)))
	}
}

var nrgbaType = reflect.TypeOf(color.NRGBA{})

// fillDistinct assigns a unique, fully opaque, non-zero NRGBA to every colour
// leaf reachable from v, recursing through nested structs and arrays. Any leaf
// that is not a color.NRGBA fails the test: a future non-colour field in
// PlatformColors needs an explicit decision here and in LerpPlatformColors.
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
		t.Fatalf("PlatformColors carries a non-colour leaf of type %v; extend LerpPlatformColors and this test", v.Type())
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
	walk("PlatformColors", got, want)
	return diffs
}

func TestPlatformColorsTweenSettlesAtTarget(t *testing.T) {
	// Settling to the target asserted at the value-equality level, not
	// just pixel equality.
	tw := transition.PlatformColorsTween(tokens.PlatformLight, tokens.PlatformDark, 30)
	if got := tw.At(30); got != tokens.PlatformDark {
		t.Errorf("Tween.At(Frames) did not settle to target: got %+v, want %+v", got, tokens.PlatformDark)
	}
}
