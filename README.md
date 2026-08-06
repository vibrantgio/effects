# pulse

The effects layer of [Vibrant Gio](https://github.com/vibrantgio), a design
system for native desktop applications on macOS, Windows and Linux, written in
pure Go on [Gio](https://gioui.org). pulse is where a component stops being
correct and starts being alive: the shadow under a toast, the halo around a
focused thing, the press that gives way under the pointer and springs back, the
fade that carries a notification out, the frosted glass behind a dialog.

Gio hands you `op/paint` and a frame callback. It has no shadow, no radial
gradient, no blur, no animation clock and no notion of a spring — a widget that
moves is a widget whose author wrote the interpolation, decided what "one
frame" means, and remembered to ask for the next frame. pulse writes that once.
The animation core is two flavours that answer to different problems. `tween`
is a value type: from, to, frames, a lerp function, no clock and no state, for
motion that just has to land where you said. `spring` is a real damped-spring
simulation over one [traer](https://github.com/vibrantgio/traer) particle, for
motion that has to feel like it has mass. `motion` composes both,
`springbutton` is prism's button wired to a spring, and `transition` teaches
`tween` the colour-token contract. Alongside the animation core sit the static
effects — `depth` and `glow`, gradient compositions — and `blur`, the module's
own Gaussian blur pipeline for the primitive Gio does not have.

**pulse never decorates prism globally.** There is no theme flag and no wrapper
that quietly animates every button in an application. An animated component is
an explicit variant, exported alongside its prism counterpart and named at the
call site — `springbutton.SpringButton` where you would have written
`button.Button` — so reading the call tells you the thing moves, the
unanimated component keeps costing nothing, and the dependency runs pulse →
prism and never the other way. `motion.Apply` is the mechanism for building
your own: wrap any prism render function in it.

## Where it sits

Tier 3 of the stack — `mvu → spectrum → prism → pulse → cadence → markdown`.
pulse imports [mvu](https://github.com/vibrantgio/mvu), `theme` and `tokens`
from [spectrum](https://github.com/vibrantgio/spectrum), `button` from
[prism](https://github.com/vibrantgio/prism), and the
[traer](https://github.com/vibrantgio/traer) particle system.
[cadence](https://github.com/vibrantgio/cadence) is built on it — its toast
takes its cast shadow from `depth` and its fade from `tween` — and one
[workbench](https://github.com/vibrantgio/workbench) application uses `depth`
directly. The [organization page](https://github.com/vibrantgio) has the full
tier table.

The layering inversions that used to run through this module are cut. The
prism↔pulse cycle is gone: prism's root module does not require pulse, and
only its demo `gallery` — a nested module, exempt by ADR-001 — imports
`springbutton`. And `transition`, which once lived in spectrum and dragged
tier 1 up to tier 3, now lives here. The deprecated `spectrum/transition`
alias shim that forwarded to it is gone as of spectrum v0.2.0, so this is
the only path.

```sh
go get github.com/vibrantgio/pulse
```

Every module in the organization is on gioui.org v0.10.1,
github.com/reactivego/rx v0.3.0 and Go 1.25.1.

## Packages

| Package | |
| --- | --- |
| `tween` | `Tween[T]{From, To, Frames, Lerp}` and `At(n)`. A value type with no clock, no easing and no state — the caller decides what a frame is. `LerpFloat64` and `LerpNRGBA` cover opacity, position and colour. |
| `spring` | One scalar pulled toward a target by a damped spring, simulated as a two-particle `traer.ParticleSystem`. `SetTarget` retargets mid-flight without losing velocity; `Settled` says when to stop asking for frames. |
| `motion` | `Enter`, `Exit` and `Transition`: a tween for opacity and a spring for scale, ticked together. `Apply` is the bridge to Gio — it lays a widget out once and replays it inside a scale-around-centre transform and an opacity layer, so the footprint never jitters. Defaults resolve from the token motion scale: `FramesAt` converts a duration stop at a frame rate, `SpringOptions` converts a `tokens.Spring` preset. |
| `springbutton` | `prism/button` with a press that scales down and springs back, on the same props and the same visual contract. The one shipped variant. |
| `transition` | Interpolates a whole `tokens.ColorTokens` set between two values, so a light-to-dark flip can cross-fade rather than snap. Moved here from spectrum in the G-B3 inversion; the `spectrum/transition` alias that forwarded here is deleted as of spectrum v0.2.0. |
| `blur` | The blur Gio does not have: a parallel three-pass box approximation of a Gaussian (`Gaussian`, `Blurrer`), `Cache` for static imagery blurred once and reused, and `Backdrop` — the "blurred behind the dialog" pipeline on `gioui.org/gpu/headless`, with `FallbackOp` and `Available` for the platforms where headless rendering is not. |
| `depth` | A Material-style cast shadow under a rectangle, composed from eight linear gradients, with extent and offset read from a `tokens.ElevationLevel`. Opt-in vibrancy per ADR-005: a shadow marks what floats and can leave — a toast, a popover, a menu — never what is raised in place, which reads as raised by its surface step alone. The E2.2 caller audit is recorded in the package doc. |
| `glow` | A luminance halo around a rectangle, composed from eight linear gradients standing in for the radial gradient Gio does not expose. The E4.4 verdict — why an animated glow is gradients, not blur — is recorded in the package doc, with the measurements. |
| `conductor` | A shared frame counter, so a staggered wave stays phase-locked. Independent per-widget simulations drift; participants reading `Local(offset)` off one clock do not. |

## Usage

The two packages the design system leans on hardest are `depth` and `tween`,
and both are one line inside a render function. `cadence/toast` uses both:
the cast shadow goes down first — a toast floats and can leave, exactly what
ADR-005 reserves shadows for — and the surface is painted over it:

```go
depth.Shadow(gtx, image.Rectangle{Max: image.Pt(w, h)}, tokens.Level3)
```

The same toast fades out over the last stretch of its lifetime — the theme's
`DurSlow` stop. Note what `Frames` is counting: the unit is never interpreted
inside `tween`, so this one counts milliseconds and indexes by the toast's
age:

```go
tw := tween.Tween[float64]{
	From:   1,
	To:     0,
	Frames: int(fade / time.Millisecond),
	Lerp:   tween.LerpFloat64,
}
frame := int((age - (lifetime - fade)) / time.Millisecond)
return tw.At(frame)
```

A variant is chosen by name and takes the same props as the component it
varies. From `prism/gallery`, which renders the static and the spring button
side by side:

```go
g.btnCompare, err = button.Button(th, button.Props{
	Label:   "Click me",
	OnClick: func(_ layout.Context) { g.btnCompareClicks++; w.Invalidate() },
}).First()

g.springBtnLive, err = springbutton.SpringButton(th, button.Props{
	Label:   "Click me",
	OnClick: func(_ layout.Context) { g.springBtnClicks++; w.Invalidate() },
}, springbutton.Options{}).First()
```

`springbutton.Options{}` is a zero value you can rely on — its fallbacks were
tuned for this component. `spring.Options{}` is not, and `motion.Options{}` is
tuned but does not add up; both are in the status section below.

Driving a `spring` or a `motion` primitive yourself is the other half. The
object is stateful, so it has to be allocated once per subscription — inside
the `rx.Defer` closure, the same place a prism component keeps its
`widget.Clickable` — and then ticked once per frame, asking for the next frame
only while it is still moving:

```go
rx.Defer(func() rx.Observable[layout.Widget] {
	sp := spring.New(1.0, 1.0, spring.Options{Stiffness: 300, Damping: 22, Mass: 1})

	return rx.Map(inputs, func(next input) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			sp.SetTarget(target)
			sp.Tick(60)
			// ... render at scale sp.Value() ...
			if !sp.Settled(0.001) {
				gtx.Execute(op.InvalidateCmd{})
			}
			return dims
		}
	})
})
```

Reconstructing the spring inside the map function instead would restart the
simulation on every emission, and reconstructing it inside the layout function
would restart it on every frame — the animation would look frozen at its first
value.

### Blur

Two supported ways to ship a blur, with different lifecycles:

**Static imagery: `blur.Cache`.** A known source image blurred once and
reused — a hero image behind text, a frosted thumbnail:

```go
var cache blur.Cache          // long-lived, e.g. per layer
op := cache.Image(src, sigma) // paint.ImageOp, cached on source identity+sigma+size
```

Repeat calls with unchanged inputs do no work. Large radii render at a reduced
size and upscale on the GPU (the divisor defaults from sigma).

**Backdrops: `blur.Backdrop`** — the "blurred behind the dialog" pipeline. It
renders a caller-supplied layer into an offscreen headless GPU window at
reduced resolution, reads back, blurs, and serves a `paint.ImageOp`. Two
contracts to honour:

1. **Refresh policy.** `Update` runs the whole pipeline synchronously on the
   calling goroutine — measured on a 1440×900 backdrop, 2.9 ms at the working
   ÷4 divisor and 29 ms at full resolution, against a 16.7 ms frame budget.
   Call `Update` only when the content behind the blur actually changed — a
   scroll settled, a dialog opened — and paint the cached `Op()` every frame
   in between; `Op` is free.
2. **Fallback.** Headless GPU rendering is not available everywhere. `Update`
   returns the error and `Op` reports `ok == false`; paint a flat tinted
   scrim instead — `blur.FallbackOp(tint)` is ready-made, and
   `blur.Available()` answers up front. Never assume the blur.

**What not to blur: animated glows.** This was prototyped, measured and
rejected (E4.4; the evidence is in `glow`'s package doc): an animating
blur-glow costs 0.2–0.8 ms of events-thread CPU plus an allocation and a
texture upload per glow per frame, against ~0.5 µs for `glow`'s
eight-gradient halo, and no cache holds while the radius or intensity
animates. Use `glow` for halos — a correct approximation beats a slow exact
answer.

## For coding assistants

Read the canonical guide before writing code against this module — the module
inventory with current tags, the application skeleton, MVU and rx semantics,
typography, and the pitfalls that are not guessable:

<https://raw.githubusercontent.com/vibrantgio/.github/master/llms.txt>

[`AGENTS.md`](./AGENTS.md) in this repository has the build, test and
golden-image commands. The golden line there is exact and both halves of it
matter — `-golden.update` must follow the package list, and the list cannot be
replaced by `./...`.

## Status

Honest about what does not work yet. Every number below is measured, not
estimated.

- **Three of the nine packages have no consumer anywhere in the
  organization.** `conductor`, `glow` and `motion` are imported by nothing
  outside their own tests — tested, golden-tested, working, and never wired
  to a component. `transition` now has no importer at all: its one consumer
  was the `spectrum/transition` shim, deleted in spectrum v0.2.0, and
  nothing actually drives a theme cross-fade (see spectrum's status).
  `depth`, `tween`, `blur` and `springbutton` are the
  ones really in use, and `spring` is used through `motion` and
  `springbutton`.
- **`springbutton` is the only variant that exists.** The design calls for a
  spring variant of each interactive prism component; one was built.
  Everything needed for the rest is here — `motion.Apply` takes any prism
  render function — but the wrapper packages are not written, and no phase of
  the current plan claims them.
- **`motion`'s defaults do not finish together.** `DefaultFrames` and
  `DefaultSpring` now resolve from the token motion scale, and they still do
  not land in step: a `NewEnter(Options{})` ticked at invDt=60 has its
  opacity complete at frame 30 and its scale still at 0.991, and does not
  report `Settled(0.005)` until frame 52. A loop that runs to `Settled`
  overshoots the visible animation by 22 frames.
- **`motion`'s partial spring override silently misfires.** `Options.Spring`
  falls back to `DefaultSpring` only when the whole struct is zero. Set
  `Stiffness` alone and the other two fields come from `spring`'s own
  defaults instead, giving a damping ratio near 0.02 — a spring that rings
  for thousands of frames. Set all three fields or none, or use
  `motion.SpringOptions` on one of the token presets (`SpringDefault`,
  `SpringSnappy`, `SpringGentle`), which sets all three together.
- **`spring.Options{}` is not a usable default.** Its zero-value fallbacks
  (k=0.4, c=0.7, m=1) take 873 frames — roughly fifteen seconds of continuous
  redraw at 60 Hz — to settle to 0.005. Neither in-module consumer goes near
  them.
- **`depth` paints a hard rectangle in a fixed black.** The interior fill has
  square corners, so a foreground with rounded corners shows dark wedges at
  all four. There is no opacity parameter, so a shadow that has to fade with
  its surface must be wrapped in a `paint.PushOpacity` layer, as
  `cadence/toast` does. And the black is not a token role, so the same shadow
  that separates a toast on a light background barely registers on a dark
  one. What changed around it is the role: elevation is now a tonal surface
  ladder (`SurfaceAt`, ADR-005) and this package is the explicit opt-in for
  the surfaces that float, not the default way to raise anything.
- **`glow` reserves no space and measures in pixels.** `Halo` draws outside
  the bounds it is given and returns nothing, so in a flex it spills over its
  neighbours unless the caller insets or clips; and `Options.Radius` is raw
  pixels rather than dp, which is backwards from every other size in the
  system. The blur-based alternative was measured and rejected (E4.4, in the
  package doc); the pixel unit and the missing inset remain.
- **Frame counting is still frame counting.** `springbutton` ticks its spring
  at a hard-coded inverse step of 60 whatever the display is doing, so a
  press that takes 415 ms on a 60 Hz screen takes half that on a 120 Hz one;
  `tween` and `conductor` count frames by construction. The token motion
  scale now carries duration stops and spring presets, and `motion.FramesAt`
  converts a stop at a stated frame rate — but nothing here reads the actual
  refresh rate.
- **Reduced motion is honoured by the theme, not yet by the springs.** The
  theme's `Motion` observable emits `MotionScale.Reduced()` — every duration
  zero — while the OS preference is on, and duration-driven consumers
  (cadence's toast fade and tooltip delay resolve from the theme) snap for
  free. But `springbutton` does not subscribe `Motion`, so its press spring
  still runs at full amplitude under reduced motion; a spring consumer must
  read the zero durations as the snap signal itself.

## License

MIT — see [LICENSE](./LICENSE).
