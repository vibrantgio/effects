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
// visual contract — the emphasis register; hover, focus, press,
// disabled colours; 44 dp minimum hit target; semantic ops — is
// preserved.
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
// The label is shaped with the theme's cached shaper
// (Typography.Shaper(), ADR-003: the theme owns the typeface) in the
// LabelLarge role. That shaper is built once for the process and shared
// by every component reading the same typography: theme's cache
// lives behind the Typography value, so it survives the copy the map
// function below makes of it (spectrum F5.1). It is not safe to use
// from two goroutines — Gio lays the widget forest out on the one
// goroutine that runs the event loop, which is what makes sharing it
// correct. Props.Shaper is an explicit per-instance override
// only; leave it nil in normal use. Since prism v0.2.0 the role's whole
// text style — typeface, weight, size and line height — reaches
// [github.com/vibrantgio/prism/button.Render], as does the theme's
// Density, so a spring button is glyph- and pixel-identical to the
// static one at scale 1.
//
// # State scope
//
// The clickable and the spring are allocated inside the rx.Defer
// closure, so they persist across theme and
// disabled emissions and across frames for the lifetime of one
// subscription. A new subscription resets the physics — a button that
// is subscribed twice has two independent springs, and one that is
// resubscribed mid-press restarts at scale 1.
package springbutton

import (
	"gioui.org/f32"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/widget"

	"github.com/reactivego/rx"
	"github.com/vibrantgio/mvu"
	"github.com/vibrantgio/prism/button"
	"github.com/vibrantgio/pulse/spring"
	"github.com/vibrantgio/theme/theme"
	"github.com/vibrantgio/theme/tokens"
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

	// Flatten the nested theme observables into a concrete snapshot. The
	// typography emission supplies both the LabelLarge text style and the
	// theme's cached shaper (ADR-003: the theme owns the typeface),
	// replacing the former Theme.Type source.
	resolved := rx.SwitchMap(th, func(t theme.Theme) rx.Observable[resolvedTokens] {
		return rx.Map(
			rx.CombineLatest5(t.Color, t.Typography, t.Spacing, t.Radius, t.Density),
			func(n rx.Tuple5[tokens.ColorTokens, tokens.Typography, tokens.SpacingScale, tokens.RadiusScale, tokens.Density]) resolvedTokens {
				typ := n.Second
				return resolvedTokens{
					color:   n.First,
					label:   typ.LabelLarge,
					spacing: n.Third,
					radius:  n.Fourth,
					density: n.Fifth,
					shaper:  typ.Shaper(),
				}
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
		return rx.Map(inputs, func(next rx.Tuple2[resolvedTokens, bool]) layout.Widget {
			tok, dis := next.First, next.Second

			// Props.Shaper is an explicit override; the theme's shaper is
			// the default.
			shaper := props.Shaper
			if shaper == nil {
				shaper = tok.shaper
			}

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

					state := renderState(props, button.RenderState{
						Hovered:  hovered,
						Focused:  focused,
						Pressed:  pressed,
						Disabled: dis,
					})

					macro := op.Record(gtx.Ops)
					// Render takes the LabelLarge role's whole text style
					// and the theme's density, so the spring button shapes
					// and sizes exactly like the static one.
					innerDims := button.Render(
						shaper,
						props.Label,
						tok.color, tok.spacing, tok.radius,
						tok.label, tok.density,
						state,
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

// renderState completes the interaction half of a
// [github.com/vibrantgio/prism/button.RenderState] — which this package reads
// off its own clickable — with every field the button's Props carries.
//
// It exists because this package renders through button.Render, the PURE
// renderer, and so has to reassemble by hand what button.Button assembles for
// itself. That reassembly is where a field goes missing: Props.Emphasis was
// added to prism and this package went on drawing every spring button filled,
// because the literal here simply did not mention it. The fix is one line; the
// guard is that this function is the one place the copy happens, and
// springbutton_internal_test.go asserts by reflection that every field
// RenderState and Props share arrives here — so the NEXT field added to the
// pair fails a test instead of silently drawing wrong.
func renderState(props button.Props, interaction button.RenderState) button.RenderState {
	s := interaction
	s.Emphasis = props.Emphasis
	return s
}

// resolvedTokens mirrors the per-emission token snapshot that
// prism/button keeps unexported. Spring composition operates in the
// same token space as the static button so the visual contract matches
// at scale = 1.
type resolvedTokens struct {
	color   tokens.ColorTokens
	label   tokens.TextStyle // the LabelLarge role: typeface, weight, size, line height
	spacing tokens.SpacingScale
	radius  tokens.RadiusScale
	density tokens.Density // control height and inner padding
	shaper  *text.Shaper   // the theme's cached shaper
}
