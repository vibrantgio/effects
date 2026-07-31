# pulse

The effects layer of [VibrantGio](https://github.com/vibrantgio), a design
system for native desktop applications on macOS, Windows and Linux, written in
pure Go on [Gio](https://gioui.org). pulse is where a component stops being
correct and starts being alive: the shadow under a card, the halo around a
focused thing, the press that gives way under the pointer and springs back, the
fade that carries a toast out.

Gio hands you `op/paint` and a frame callback. It has no shadow, no radial
gradient, no blur, no animation clock and no notion of a spring — a widget that
moves is a widget whose author wrote the interpolation, decided what "one
frame" means, and remembered to ask for the next frame. pulse writes that once,
in two flavours that answer to different problems. `tween` is a value type: from,
to, frames, a lerp function, no clock and no state, for motion that just has to
land where you said. `spring` is a real damped-spring simulation over one
[traer](https://github.com/vibrantgio/traer) particle, for motion that has to
feel like it has mass. Everything else here is built out of those two —
`motion` composes both, `springbutton` is prism's button wired to a spring, and
`depth` and `glow` are static effects that use neither.

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
pulse imports [mvu](https://github.com/vibrantgio/mvu), `button`, `theme` and
`tokens` from [prism](https://github.com/vibrantgio/prism), and the
[traer](https://github.com/vibrantgio/traer) particle system.
[cadence](https://github.com/vibrantgio/cadence) is built on it — its card and
toast take their shadows from `depth` and the toast fade from `tween` — and one
[workbench](https://github.com/vibrantgio/workbench) application uses `depth`
directly. The [organization page](https://github.com/vibrantgio) has the full
tier table.

Two edges are worth being honest about. The cycle that used to run between this
module and prism is cut: prism's root module no longer requires pulse, and only
its demo `gallery` — a nested module, exempt by ADR-001 — still imports
`springbutton`. The remaining inversion runs the other way: `spectrum/transition`
imports `pulse/tween`, so tier 1 depends on tier 3. Phase B of the
[org plan](https://github.com/vibrantgio/.github) fixes it by moving that
package up here as `pulse/transition`, leaving an alias behind so no downstream
repository has to change a line.

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
| `motion` | `Enter`, `Exit` and `Transition`: a tween for opacity and a spring for scale, ticked together. `Apply` is the bridge to Gio — it lays a widget out once and replays it inside a scale-around-centre transform and an opacity layer, so the footprint never jitters. |
| `springbutton` | `prism/button` with a press that scales down and springs back, on the same props and the same visual contract. The one shipped variant. |
| `depth` | A Material-style cast shadow under a rectangle, composed from eight linear gradients, with extent and offset read from a `tokens.ElevationLevel`. |
| `glow` | A luminance halo around a rectangle, composed from eight linear gradients standing in for the radial gradient Gio does not expose. |
| `conductor` | A shared frame counter, so a staggered wave stays phase-locked. Independent per-widget simulations drift; participants reading `Local(offset)` off one clock do not. |

## Usage

The two packages the design system actually leans on are `depth` and `tween`,
and both are one line inside a render function. This is `cadence/card`, drawing
an elevated card — the shadow goes down first, and the surface is painted over
it:

```go
if props.Elevated {
	depth.Shadow(gtx, bounds, tokens.Level2)
}

rrect := clip.RRect{Rect: bounds, SE: r, SW: r, NE: r, NW: r}
paint.FillShape(gtx.Ops, colors.Surface, rrect.Op(gtx.Ops))
```

`cadence/toast` fades a notification out over the last stretch of its lifetime.
Note what `Frames` is counting — the unit is never interpreted inside `tween`,
so this one counts milliseconds and indexes by the toast's age:

```go
tw := tween.Tween[float64]{
	From:   1,
	To:     0,
	Frames: int(fadeWindow / time.Millisecond),
	Lerp:   tween.LerpFloat64,
}
frame := int((age - (lifetime - fadeWindow)) / time.Millisecond)
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

- **Three of the seven packages have no consumer anywhere in the
  organization.** `conductor` and `glow` are imported by nothing at all — no
  module, no application, not even another package in this repository — and
  `motion` is imported by nothing outside its own tests. They are tested and
  golden-tested and they work; they have simply never been wired to a
  component. `depth`, `tween` and `springbutton` are the three that are really
  in use, and `spring` is used through `motion` and `springbutton`.
- **`springbutton` is the only variant that exists.** The design calls for a
  spring variant of each interactive prism component; one was built. Everything
  needed for the rest is here — `motion.Apply` takes any prism render function
  — but the wrapper packages are not written, and no phase of the current plan
  claims them.
- **`motion`'s defaults do not finish together.** `DefaultFrames` is 30 and
  `DefaultSpring` was tuned to settle alongside it. It does not: a
  `NewEnter(Options{})` ticked at invDt=60 has its opacity complete at frame 30
  and its scale still at 0.991, and does not report `Settled(0.005)` until
  frame 52. A loop that runs to `Settled` overshoots the visible animation by
  22 frames.
- **`motion`'s partial spring override silently misfires.** `Options.Spring`
  falls back to `DefaultSpring` only when the whole struct is zero. Set
  `Stiffness` alone and the other two fields come from `spring`'s own defaults
  instead, giving a damping ratio near 0.02 — a spring that rings for thousands
  of frames. Set all three fields or none.
- **`spring.Options{}` is not a usable default.** Its zero-value fallbacks
  (k=0.4, c=0.7, m=1) take 873 frames — roughly fifteen seconds of continuous
  redraw at 60 Hz — to settle to 0.005. Neither in-module consumer goes near
  them.
- **`depth` paints a hard rectangle in a fixed black.** The interior fill has
  square corners, so a foreground with rounded corners shows dark wedges at all
  four — which is every caller, since all three paint rounded rectangles. There
  is no opacity parameter, so a shadow that has to fade with its surface must
  be wrapped in a `paint.PushOpacity` layer, as `cadence/toast` does. And the
  black is not a token role, so the same shadow that separates a card on a
  light background barely registers on a dark one. Phase E turns elevation into
  a tonal surface role and keeps this package as an explicit opt-in effect
  rather than the default way to raise a surface.
- **`glow` reserves no space and measures in pixels.** `Halo` draws outside the
  bounds it is given and returns nothing, so in a flex it spills over its
  neighbours unless the caller insets or clips; and `Options.Radius` is raw
  pixels rather than dp, which is backwards from every other size in the
  system. Phase E prototypes a blur-based glow and decides between the two on
  measured cost.
- **There is no blur.** Gio exposes no blur primitive and no custom shaders.
  Phase E builds `pulse/blur` on `gioui.org/gpu/headless` — a separable
  three-pass box blur, a cache for static imagery, and a backdrop pipeline —
  with a defined fallback for the platforms where headless rendering is not
  available.
- **Everything is counted in frames, and nothing reads the refresh rate.**
  `springbutton` ticks its spring at a hard-coded inverse step of 60 whatever
  the display is doing, so a press that takes 415 ms on a 60 Hz screen takes
  half that on a 120 Hz one. `tween` and `conductor` count frames by
  construction. A frame-rate-aware step is unwritten; Phase E adds MD3 duration
  stops and spring specifications to the token set for this layer to consume.
- **`springbutton` falls back to gofont.** With no `Shaper` in its props it
  builds one from `gioui.org/font/gofont`, inherited from `prism/button`, so a
  single button comes out in Go Regular inside an application that otherwise
  renders through [font](https://github.com/vibrantgio/font). Phase C removes
  the fallback across the stack; until then, pass `Props.Shaper`.
- **Reduced motion is not honoured.** `prism/a11y` publishes the OS
  reduce-motion preference and nothing in this module reads it, so an animation
  here runs at full amplitude for a user who has asked the system for less.
  Phase E routes the accessibility preferences into the theme and requires
  animated components to snap to their target under reduced motion.

## License

MIT — see [LICENSE](./LICENSE).
