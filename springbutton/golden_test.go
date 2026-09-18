package springbutton_test

import (
	"image"
	"testing"

	"gioui.org/f32"
	gioinput "gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/reactivego/rx"
	"github.com/vibrantgio/components/button"
	"github.com/vibrantgio/components/golden"
	"github.com/vibrantgio/components/icons"
	"github.com/vibrantgio/effects/springbutton"
	"github.com/vibrantgio/theme/theme"
	"github.com/vibrantgio/theme/tokens"
)

// The frame the chrome control is recorded in, and where it stands in it: a
// square of chrome material with the control clear of every edge, so the drop
// shadow it casts on its band has room in the stored image and the scale it
// springs through has room to grow into.
var (
	chromeFrame  = image.Pt(120, 90)
	chromeOrigin = image.Pt(40, 27)
	// The point the press is queued at: the control's own middle, which is
	// its origin plus half the measured 38 by 36 box.
	chromePress = f32.Pt(59, 45)
)

// springFrames is how many frames the press is held for before the control is
// recorded. The spring is ticked once per frame at a fixed inverse step of 60,
// so the scale a stored frame carries is the same on every machine; six frames
// in, the press is part way down its travel rather than at either end, which
// is the state a resting render cannot show.
const springFrames = 6

// staticTheme freezes one colour scheme into a Theme whose every field emits
// once — the shape theme/window feeds a component, minus the live OS poll.
func staticTheme(c tokens.PlatformColors) theme.Theme {
	return theme.Theme{
		Platform:   rx.Of(c),
		Typography: rx.Of(tokens.DefaultTypography),
		Density:    rx.Of(tokens.Comfortable),
		Motion:     rx.Of(tokens.Motion),
		Spacing:    rx.Of(tokens.Spacing),
		Radius:     rx.Of(tokens.Radius),
		Elevation:  rx.Of(tokens.Elevation),
	}
}

// TestChromeSpringButtonGolden records the platform's bordered toolbar control
// as this package draws it, in both appearances and in the two states the
// variant exists for: at rest, where the spring is settled and the affine is
// the identity, and part way down a press, where it is not.
//
// It is a stored image rather than a pin against the static button because a
// spring in flight has no static counterpart to be held to — the whole of what
// this package adds is what the control looks like while it is moving, the
// shadow it casts on its band included, and only a picture carries that.
func TestChromeSpringButtonGolden(t *testing.T) {
	mark := icons.Mark(icons.Sidebar)
	if mark == nil {
		t.Fatal("no painter for the sidebar")
	}
	for _, sc := range []struct {
		name   string
		colors tokens.PlatformColors
	}{{"light", tokens.PlatformLight}, {"dark", tokens.PlatformDark}} {
		props := button.Props{
			Description: "Hide the conversations",
			Variant:     button.Chrome,
			Surface:     sc.colors.SidebarMaterial,
			Icon:        mark,
		}
		for _, st := range []struct {
			name   string
			frames int
		}{{"rest", 0}, {"spring", springFrames}} {
			name := "chrome-spring-" + sc.name + "-" + st.name
			t.Run(name, func(t *testing.T) {
				w, err := springbutton.SpringButton(rx.Of(staticTheme(sc.colors)), props, springbutton.Options{}).First()
				if err != nil {
					t.Fatalf("First() = %v", err)
				}
				scene := onChrome(sc.colors, w)
				if st.frames > 0 {
					scene = held(scene, chromeFrame, chromePress, st.frames)
				}
				golden.Render(t, name, chromeFrame, scene)
			})
		}
	}
}

// onChrome stands the control on the chrome material, which is the band a
// toolbar control stands in everywhere in this library, and clear of the
// frame's edges so its drop shadow is in the picture.
func onChrome(p tokens.PlatformColors, w layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, p.SidebarMaterial, clip.Rect{Max: gtx.Constraints.Max}.Op())
		defer op.Offset(chromeOrigin).Push(gtx.Ops).Pop()
		return w(gtx)
	}
}

// held drives w through frames headless frames with the pointer down at pos
// and returns a layout.Widget drawing from the state those frames left behind.
// It is how a spring is caught in flight: the physics live inside the
// component, so the only way to a frame part way down a press is to press and
// keep laying out.
func held(w layout.Widget, size image.Point, pos f32.Point, frames int) layout.Widget {
	r := new(gioinput.Router)
	drive := func() {
		var ops op.Ops
		w(layout.Context{
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(size),
			Ops:         &ops,
			Source:      r.Source(),
		})
		r.Frame(&ops)
	}
	drive()
	r.Queue(pointer.Event{Kind: pointer.Press, Position: pos, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	for range frames {
		drive()
	}
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Source = r.Source()
		return w(gtx)
	}
}
