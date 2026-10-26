// Package springbutton ships the spring-physics variant of
// components/button: the same button, with a press that scales down and
// springs back.
//
// It is a sibling of [github.com/vibrantgio/components/button.Button], not a
// decorator over it. Effects never animates components globally — a caller
// picks the animated component by name, so the call site says what it
// does and the dependency runs effects → components and never back.
// SpringButton owns its own [widget.Clickable] and renders through
// [github.com/vibrantgio/components/button.Render], the pure renderer, then
// wraps the output in an [op.Affine] scale driven by a
// [github.com/vibrantgio/effects/spring.Spring]. The underlying button's
// visual contract — the emphasis variant and the platform colours it
// reads at rest, pressed, focused and disabled; the drawn control as its
// own pointer target; semantic ops — is preserved.
//
//	// Static components button.Button:
//	w, _ := button.Button(theme, button.Props{Title: "Save", OnClick: save}).First()
//
//	// Spring-physics variant:
//	w, _ := springbutton.SpringButton(theme,
//	    button.Props{Title: "Save", OnClick: save},
//	    springbutton.Options{}, // zero-valued: package defaults
//	).First()
//
// # Physics
//
// On press, the spring retargets to [Options.PressScale]; on release,
// back to 1.0. The free particle's position is read every frame as a
// scale factor applied around the button's centre. The spring is the
// theme's Motion.SpringSnappy preset. The theme's default preset
// (Stiffness 300, Damping 22, Mass 1) is underdamped, so the release
// overshoots slightly and comes back — the "pop". Measured against this
// package's own settle tolerance, the release settles about 415 ms
// after the pointer lifts.
//
// While the spring is in flight the component schedules its own redraws
// via [op.InvalidateCmd]; once it settles, no further frames are
// requested, so a SpringButton at rest costs the same per frame as a
// static components button.
//
// # Time
//
// Each frame steps the spring by the time since the previous frame,
// read from gtx.Now. The press takes the same wall-clock time on a
// 60 Hz and a 120 Hz display. The step is split into sub-steps of at
// most 1/60 s, which keeps the explicit integrator stable. One frame
// steps at most 1/30 s, so a stalled host slows the press and never
// makes it jump, and the first frame after a pause steps 1/30 s. A frame with no earlier frame, or with a clock that
// runs backwards, steps zero time.
//
// # Reduced motion
//
// While the OS preference is on, the theme's Motion emits every
// duration stop as zero. The button reads those zeros and moves the
// spring to its target in the frame that sets the target: press and
// release land in one frame, with no animation.
//
// # Fonts
//
// The title is shaped with the theme's cached shaper
// (Typography.Shaper()) in the SystemFont role; the theme owns the
// typeface. That shaper is built once for the process and shared by
// every component reading the same typography: the cache lives behind
// the Typography value, so it survives the copy the map function below
// makes of it. It is not safe to use from two goroutines — Gio lays the
// layout tree out on the one goroutine that runs the event loop,
// which is what makes sharing it correct. Props.Shaper is an explicit
// per-instance override only; leave it nil in normal use. The role's
// whole text style — typeface, weight, size and line height — reaches
// [github.com/vibrantgio/components/button.Render], as does the theme's
// Density, so a spring button is glyph- and pixel-identical to the
// static one at scale 1.
//
// # State scope
//
// The clickable and the spring are allocated inside the rx.Defer
// closure, so they persist across theme and
// disabled emissions and across frames for the lifetime of one
// subscription. A theme emission with a different spring preset
// replaces the spring at its current position, with zero velocity. A
// new subscription resets the physics — a button that is subscribed
// twice has two independent springs, and one that is resubscribed
// mid-press restarts at scale 1.
package springbutton

import (
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/widget"

	"github.com/reactivego/rx"
	"github.com/vibrantgio/components/button"
	"github.com/vibrantgio/effects/spring"
	"github.com/vibrantgio/mvu"
	"github.com/vibrantgio/theme/theme"
	"github.com/vibrantgio/theme/tokens"
)

// DefaultPressScale is the scale a pressed button shrinks to. The
// change is small (8 %), so the theme's snappy spring covers most of it
// within a ~150 ms press.
const (
	DefaultPressScale = 0.92
	settleTolerance   = 0.001
)

// maxSubStep is the longest single integration step. At 1/60 s a 60 Hz
// frame is one step. Velocity Verlet on the snappy preset
// (ω ≈ 17.3 rad/s) is stable below a step of about 115 ms, so 1/60 s
// is far inside that. maxFrameStep is the most time one frame steps.
// It limits how far the press moves in one frame and how many
// sub-steps one frame runs.
const (
	maxSubStep   = time.Second / 60
	maxFrameStep = time.Second / 30
)

// Options configures the press layered on top of components/button.
// The spring itself is the theme's Motion.SpringSnappy preset. An empty
// Options{} produces the canonical SpringButton feel.
type Options struct {
	// PressScale is the scale the button shrinks to while pressed.
	// 1.0 means no shrink (the spring becomes a no-op visual);
	// values below 1.0 produce a press-down effect. Zero is replaced
	// with [DefaultPressScale].
	PressScale float64
}

// SpringButton returns an rx.Observable[layout.Widget] that renders
// the components button.Button visual with a spring-driven scale on press/release.
// All [button.Props] fields are honoured (title, description, disabled
// observable, OnClick, Message, custom Shaper) — the FRP and MVU
// integration paths from components/button continue to work unchanged.
func SpringButton(
	th rx.Observable[theme.Theme],
	props button.Props,
	opts Options,
) rx.Observable[layout.Widget] {
	if opts.PressScale == 0 {
		opts.PressScale = DefaultPressScale
	}

	disabled := props.Disabled
	if disabled == nil {
		disabled = rx.Of(false)
	}

	// Flatten the nested theme observables into a concrete snapshot. The
	// typography emission supplies both the SystemFont text style and the
	// theme's cached shaper; the theme owns the typeface. The motion
	// emission supplies the spring preset and the reduced-motion signal.
	resolved := rx.SwitchMap(th, func(t theme.Theme) rx.Observable[resolvedTokens] {
		return rx.Map(
			rx.CombineLatest2(
				rx.CombineLatest5(t.Platform, t.Typography, t.Spacing, t.Radius, t.Density),
				t.Motion,
			),
			func(m rx.Tuple2[rx.Tuple5[tokens.PlatformColors, tokens.Typography, tokens.SpacingScale, tokens.RadiusScale, tokens.Density], tokens.MotionScale]) resolvedTokens {
				n, motion := m.First, m.Second
				typ := n.Second
				return resolvedTokens{
					platform: n.First,
					title:    typ.SystemFont,
					spacing:  n.Third,
					radius:   n.Fourth,
					density:  n.Fifth,
					shaper:   typ.Shaper(),
					spring:   motion.SpringSnappy,
					reduced:  reducedMotion(motion),
				}
			},
		)
	})

	inputs := rx.CombineLatest2(resolved, disabled)

	return rx.Defer(func() rx.Observable[layout.Widget] {
		var click widget.Clickable
		var sp *spring.Spring
		var preset tokens.Spring
		var last time.Time
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
				// Only the frame touches the spring: the emissions arrive
				// on another goroutine.
				if sp == nil || tok.spring != preset {
					at := 1.0
					if sp != nil {
						at = sp.Value()
					}
					preset = tok.spring
					sp = newSpring(at, preset)
				}
				if tok.reduced {
					// Reduced motion: the spring lands on its target in
					// the frame that sets the target.
					if sp.Value() != target || sp.Velocity() != 0 {
						sp = newSpring(target, preset)
					}
				} else {
					sp.SetTarget(target)
					advance(sp, frameStep(last, gtx.Now))
				}
				last = gtx.Now
				scale := float32(sp.Value())

				state := renderState(props, button.RenderState{
					Hovered:  hovered,
					Focused:  focused,
					Pressed:  pressed,
					Disabled: dis,
				})

				desc := props.Description
				if desc == "" {
					desc = props.Title
				}
				// A button whose title is a symbol is a different drawing
				// from one carrying text, and in a chrome region it is the
				// platform's bordered toolbar control. The static button
				// picks between the three by the same two facts, and this
				// one picks with it: a spring button handed
				// Props.Variant Chrome and a symbol draws the toolbar
				// control, never the form button under another name.
				iconOnly := props.Icon != nil && props.Title == ""
				chromeSymbol := iconOnly && state.Variant == button.Chrome

				face := func(gtx layout.Context) layout.Dimensions {
					semantic.ClassOp(semantic.Button).Add(gtx.Ops)
					semantic.LabelOp(props.Title).Add(gtx.Ops)
					semantic.DescriptionOp(desc).Add(gtx.Ops)
					semantic.EnabledOp(!dis).Add(gtx.Ops)
					switch {
					case chromeSymbol:
						return button.BorderedFace(props.Icon, tok.platform, tok.radius, tok.density, state)(gtx)
					case iconOnly:
						return button.RenderIcon(props.Icon,
							tok.platform, tok.spacing, tok.radius, tok.density, state)(gtx)
					}
					// Render takes the SystemFont role's whole text style
					// and the theme's density, so the spring button shapes
					// and sizes exactly like the static one.
					return button.Render(
						shaper,
						props.Title,
						tok.platform, tok.spacing, tok.radius,
						tok.title, tok.density,
						state,
					)(gtx)
				}

				// The scale is applied around whatever was drawn, and for
				// the chrome control that includes the drop shadow it casts
				// on its band: the shadow belongs to the control and travels
				// with it under the press. It is cast AROUND the clickable
				// because it falls outside the control's own box and a
				// clickable clips what it wraps to that box.
				macro := op.Record(gtx.Ops)
				var innerDims layout.Dimensions
				if chromeSymbol {
					innerDims = button.BorderedShadow(gtx, tok.platform, state, func(gtx layout.Context) layout.Dimensions {
						return click.Layout(gtx, face)
					})
				} else {
					innerDims = click.Layout(gtx, face)
				}
				call := macro.Stop()

				origin := f32.Pt(float32(innerDims.Size.X)/2, float32(innerDims.Size.Y)/2)
				tr := f32.Affine2D{}.Scale(origin, f32.Pt(scale, scale))
				stack := op.Affine(tr).Push(gtx.Ops)
				call.Add(gtx.Ops)
				stack.Pop()
				dims := innerDims

				if !sp.Settled(settleTolerance) {
					gtx.Execute(op.InvalidateCmd{})
				}
				return dims
			}
		})
	})
}

// renderState completes the interaction half of a
// [github.com/vibrantgio/components/button.RenderState] — which this package reads
// off its own clickable — with every field the button's Props carries.
//
// It exists because this package renders through button.Render, the PURE
// renderer, and so has to reassemble by hand what button.Button assembles for
// itself. A field the literal below forgets is a silent mis-render, so this
// function must stay the one place the copy happens:
// springbutton_internal_test.go asserts by reflection that every field
// RenderState and Props share arrives here.
func renderState(props button.Props, interaction button.RenderState) button.RenderState {
	s := interaction
	s.Emphasis = props.Emphasis
	s.Variant = props.Variant
	s.Surface = props.Surface
	s.Fill = props.Fill
	s.Foreground = props.Foreground
	s.Checked = props.Checked
	return s
}

// newSpring returns a spring at rest at position at, with the physics
// of the theme preset p.
func newSpring(at float64, p tokens.Spring) *spring.Spring {
	return spring.New(at, at, spring.Options{
		Stiffness: float64(p.Stiffness),
		Damping:   float64(p.Damping),
		Mass:      float64(p.Mass),
	})
}

// frameStep returns the time one frame steps the spring: the time from
// the previous frame at last to this frame at now. The step is zero
// when no previous frame exists or when the clock runs backwards. It is
// at most [maxFrameStep].
func frameStep(last, now time.Time) time.Duration {
	if last.IsZero() || now.IsZero() {
		return 0
	}
	return min(max(now.Sub(last), 0), maxFrameStep)
}

// advance steps sp by d, in equal sub-steps of at most [maxSubStep].
func advance(sp *spring.Spring, d time.Duration) {
	if d <= 0 {
		return
	}
	n := int(math.Ceil(float64(d) / float64(maxSubStep)))
	invDt := float64(n) / d.Seconds()
	for range n {
		sp.Tick(invDt)
	}
}

// reducedMotion reports whether m is the reduced-motion scale: every
// duration stop is zero.
func reducedMotion(m tokens.MotionScale) bool {
	return m.DurXFast == 0 && m.DurFast == 0 && m.DurNormal == 0 &&
		m.DurSlow == 0 && m.DurXSlow == 0
}

// resolvedTokens mirrors the per-emission token snapshot that
// components/button keeps unexported. Spring composition operates in the
// same token space as the static button so the visual contract matches
// at scale = 1.
type resolvedTokens struct {
	platform tokens.PlatformColors
	title    tokens.TextStyle // the SystemFont role: typeface, weight, size, line height
	spacing  tokens.SpacingScale
	radius   tokens.RadiusScale
	density  tokens.Density // control height and inner padding
	shaper   *text.Shaper   // the theme's cached shaper
	spring   tokens.Spring  // the theme's Motion.SpringSnappy preset
	reduced  bool           // every Motion duration stop is zero
}
