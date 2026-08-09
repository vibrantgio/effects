package springbutton

import (
	"reflect"
	"testing"

	"github.com/vibrantgio/prism/button"
)

// TestRenderStateForwardsEveryFieldPropsCarries is the guard that would have
// caught the bug this file's renderState was extracted to fix.
//
// SpringButton renders through button.Render, the pure renderer, so it has to
// assemble the button.RenderState that button.Button assembles for itself.
// Anything prism adds to BOTH button.Props and button.RenderState is a field
// this package must copy across, and the failure mode is silence: prism gained
// Props.Emphasis, the literal here did not mention it, and every spring button
// went on drawing filled with nothing red anywhere.
//
// So the test does not name Emphasis. It asks reflection which fields the two
// structs share — by name AND type, which is what makes Props.Disabled
// (rx.Observable[bool]) correctly not one of them — sets a distinctive
// non-zero value on each, and requires all of them to arrive. Add a field to
// the pair and forget to forward it and this test fails, whatever the field is
// called.
func TestRenderStateForwardsEveryFieldPropsCarries(t *testing.T) {
	stateT := reflect.TypeOf(button.RenderState{})
	propsT := reflect.TypeOf(button.Props{})

	var props button.Props
	pv := reflect.ValueOf(&props).Elem()

	var shared []string
	for i := range stateT.NumField() {
		f := stateT.Field(i)
		pf, ok := propsT.FieldByName(f.Name)
		if !ok || pf.Type != f.Type {
			continue
		}
		dst := pv.FieldByName(f.Name)
		if !setDistinctive(dst) {
			t.Fatalf("button.Props.%s is a %s, which this test does not know how to "+
				"fill; teach setDistinctive that kind so the field stays guarded", f.Name, f.Type)
		}
		shared = append(shared, f.Name)
	}

	if len(shared) == 0 {
		t.Fatal("button.Props and button.RenderState share no field by name and type; " +
			"either prism's API changed shape or this test no longer guards anything")
	}

	got := reflect.ValueOf(renderState(props, button.RenderState{}))
	for _, name := range shared {
		want := pv.FieldByName(name).Interface()
		have := got.FieldByName(name).Interface()
		if want != have {
			t.Errorf("renderState dropped button.Props.%s: RenderState.%s = %v, want %v — "+
				"copy it in renderState, or a spring button will silently ignore it",
				name, name, have, want)
		}
	}
}

// TestRenderStateKeepsTheInteractionHalf pins the other direction: the fields
// that come off the live clickable rather than off Props must survive the copy
// untouched.
func TestRenderStateKeepsTheInteractionHalf(t *testing.T) {
	in := button.RenderState{Hovered: true, Focused: true, Pressed: true, Disabled: true}
	got := renderState(button.Props{}, in)

	iv, gv := reflect.ValueOf(in), reflect.ValueOf(got)
	st := iv.Type()
	for i := range st.NumField() {
		f := st.Field(i)
		if f.Type.Kind() != reflect.Bool {
			continue // the register comes from Props; the other test owns it
		}
		if !gv.Field(i).Bool() {
			t.Errorf("renderState dropped the interaction field %s", f.Name)
		}
	}
}

// setDistinctive writes a non-zero value of v's own type into v, and reports
// whether it knew how. Kept deliberately small: an unknown kind fails the test
// loudly rather than silently skipping a field.
func setDistinctive(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1)
	case reflect.String:
		v.SetString("x")
	default:
		return false
	}
	return true
}
