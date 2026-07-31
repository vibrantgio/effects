// Package springbutton ships the spring-physics variant of
// prism/button: the same button, with a press that scales down and
// springs back.
//
// It is a sibling of [github.com/vibrantgio/prism/button.Button], not a
// decorator over it. Pulse never animates prism globally — a caller
// picks the animated component by name, so the call site says what it
// does and the dependency runs pulse → prism and never back.
// SpringButton owns its own [widget.Clickable] and renders through
// [github.com/vibrantgio/prism/button.Render], the pure renderer, then
// wraps the output in an [op.Affine] scale driven by a
// [github.com/vibrantgio/pulse/spring.Spring]. The underlying button's
// visual contract — hover, focus, press, disabled colours; 44 dp
// minimum hit target; semantic ops — is preserved.
//
//	// Static prism.Button:
//	w, _ := button.Button(theme, button.Props{Label: "Save", OnClick: save}).First()
//
//	// Spring-physics variant:
//	w, _ := springbutton.SpringButton(theme,
//	    button.Props{Label: "Save", OnClick: save},
//	    springbutton.Options{}, // zero-valued: package defaults
//	).First()
//
// # Physics
//
// On press, the spring retargets to [Options.PressScale]; on release,
// back to 1.0. The free particle's position is read every frame as a
// scale factor applied around the button's centre. The default
// parameters (Stiffness 300, Damping 22, Mass 1) are underdamped, so
// the release overshoots slightly and comes back — the "pop". Measured
// against this package's own settle tolerance, a press-and-release
// settles 25 frames after the release.
//
// While the spring is in flight the widget schedules its own redraws
// via [op.InvalidateCmd]; once it settles, no further frames are
// requested, so a SpringButton at rest costs the same per frame as a
// static prism button.
//
// # Frames, not seconds
//
// The spring is ticked at a hard-coded inverse step of 60 rather than
// at the window's real frame rate, so those 25 frames are 25 frames
// whatever the display does. On a 120 Hz screen the press animation
// takes half the wall-clock time it takes on a 60 Hz one, and under a
// stuttering host it stretches out to match. Nothing here reads the
// frame rate, and there is no option to supply it; a frame-rate-aware
// step is a later refinement, shared with
// [github.com/vibrantgio/pulse/conductor].
//
// # Fonts
//
// A SpringButton with no Shaper in its props builds one from
// gioui.org/font/gofont, inherited from prism/button, which does the
// same. In an application that otherwise renders through
// github.com/vibrantgio/font, that means this one button silently comes
// out in Go Regular. Pass Props.Shaper until the organization's font
// migration removes the fallback.
//
// # State scope
//
// The clickable, the spring, and the fallback shaper are allocated
// inside the rx.Defer closure, so they persist across theme and
// disabled emissions and across frames for the lifetime of one
// subscription. A new subscription resets the physics — a button that
// is subscribed twice has two independent springs, and one that is
// resubscribed mid-press restarts at scale 1.
package springbutton

import (
	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/widget"

	"github.com/reactivego/rx"
	"github.com/vibrantgio/mvu"
	"github.com/vibrantgio/prism/button"
	"github.com/vibrantgio/prism/theme"
	"github.com/vibrantgio/prism/tokens"
	"github.com/vibrantgio/pulse/spring"
)

// Defaults tuned for a visible-but-snappy button press. The amplitude
// of the scale change is small (8 %), so the spring must be stiff
// enough to traverse most of it within a ~150 ms press window at a
// 60 Hz frame rate. Stiffness 300 with Damping 22 (zeta ≈ 0.635) gives
// a small overshoot for "pop" feel. Measured against settleTolerance,
// each leg settles 25 frames after the pointer event — ~415 ms at
// 60 Hz, not the ~250 ms this was tuned for.
const (
	DefaultStiffness  = 300.0
	DefaultDamping    = 22.0
	DefaultMass       = 1.0
	DefaultPressScale = 0.92
	settleTolerance   = 0.001
)

// The spring is ticked at this fixed inverse step. A constant matches
// the DESIGN-recommended convention max(1, fps/30) at 60 Hz; under
// frame-rate drift the animation drifts too. A frame-rate-aware
// variant is a future refinement (cross-cutting with pulse/conductor).
const invDt = 60.0

// Options configures the spring physics layered on top of
// prism/button. Zero-valued fields use package defaults so an empty
// Options{} produces the canonical SpringButton feel.
type Options struct {
	// Stiffness, Damping, Mass are the same spring parameters
	// documented in [pulse/spring.Options]. Zero values are replaced
	// with [DefaultStiffness], [DefaultDamping], [DefaultMass].
	Stiffness float64
	Damping   float64
	Mass      float64

	// PressScale is the scale the button shrinks to while pressed.
	// 1.0 means no shrink (the spring becomes a no-op visual);
	// values below 1.0 produce a press-down effect. Zero is replaced
	// with [DefaultPressScale].
	PressScale float64
}

// SpringButton returns an rx.Observable[layout.Widget] that renders
// the prism.Button visual with a spring-driven scale on press/release.
// All [button.Props] fields are honoured (label, description, disabled
// observable, OnClick, Message, custom Shaper) — the FRP and MVU
// integration paths from prism/button continue to work unchanged.
func SpringButton(
	th rx.Observable[theme.Theme],
	props button.Props,
	opts Options,
) rx.Observable[layout.Widget] {
	if opts.Stiffness == 0 {
		opts.Stiffness = DefaultStiffness
	}
	if opts.Damping == 0 {
		opts.Damping = DefaultDamping
	}
	if opts.Mass == 0 {
		opts.Mass = DefaultMass
	}
	if opts.PressScale == 0 {
		opts.PressScale = DefaultPressScale
	}

	disabled := props.Disabled
	if disabled == nil {
		disabled = rx.Of(false)
	}

	resolved := rx.SwitchMap(th, func(t theme.Theme) rx.Observable[resolvedTokens] {
		return rx.Map(
			rx.CombineLatest4(t.Color, t.Type, t.Spacing, t.Radius),
			func(n rx.Tuple4[tokens.ColorTokens, tokens.TypeScale, tokens.SpacingScale, tokens.RadiusScale]) resolvedTokens {
				return resolvedTokens{n.First, n.Second, n.Third, n.Fourth}
			},
		)
	})

	inputs := rx.CombineLatest2(resolved, disabled)

	return rx.Defer(func() rx.Observable[layout.Widget] {
		var click widget.Clickable
		sp := spring.New(1.0, 1.0, spring.Options{
			Stiffness: opts.Stiffness,
			Damping:   opts.Damping,
			Mass:      opts.Mass,
		})
		shaper := props.Shaper
		if shaper == nil {
			shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
		}

		return rx.Map(inputs, func(next rx.Tuple2[resolvedTokens, bool]) layout.Widget {
			tok, dis := next.First, next.Second

			return func(gtx layout.Context) layout.Dimensions {
				if dis {
					gtx = gtx.Disabled()
				}

				if click.Clicked(gtx) {
					if props.OnClick != nil {
						props.OnClick(gtx)
					}
					if props.Message != nil {
						mvu.MessageOp{Message: props.Message}.Add(gtx.Ops)
					}
				}

				pressed := click.Pressed()
				hovered := click.Hovered()
				focused := !dis && gtx.Focused(&click)

				target := 1.0
				if pressed {
					target = opts.PressScale
				}
				sp.SetTarget(target)
				sp.Tick(invDt)
				scale := float32(sp.Value())

				dims := click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					semantic.ClassOp(semantic.Button).Add(gtx.Ops)
					semantic.LabelOp(props.Label).Add(gtx.Ops)
					desc := props.Description
					if desc == "" {
						desc = props.Label
					}
					semantic.DescriptionOp(desc).Add(gtx.Ops)
					semantic.EnabledOp(!dis).Add(gtx.Ops)

					renderState := button.RenderState{
						Hovered:  hovered,
						Focused:  focused,
						Pressed:  pressed,
						Disabled: dis,
					}

					macro := op.Record(gtx.Ops)
					innerDims := button.Render(
						shaper,
						props.Label,
						tok.color, tok.spacing, tok.radius, tok.typ,
						renderState,
					)(gtx)
					call := macro.Stop()

					origin := f32.Pt(float32(innerDims.Size.X)/2, float32(innerDims.Size.Y)/2)
					tr := f32.Affine2D{}.Scale(origin, f32.Pt(scale, scale))
					stack := op.Affine(tr).Push(gtx.Ops)
					call.Add(gtx.Ops)
					stack.Pop()

					return innerDims
				})

				if !sp.Settled(settleTolerance) {
					gtx.Execute(op.InvalidateCmd{})
				}
				return dims
			}
		})
	})
}

// resolvedTokens mirrors the per-emission token snapshot that
// prism/button keeps unexported. Spring composition operates in the
// same token space as the static button to ensure the visual contract
// is identical pixel-for-pixel when scale = 1.
type resolvedTokens struct {
	color   tokens.ColorTokens
	typ     tokens.TypeScale
	spacing tokens.SpacingScale
	radius  tokens.RadiusScale
}
