# effects

The effects layer of [Vibrant Gio](https://github.com/vibrantgio), a design
system for native desktop applications on macOS, Windows and Linux, written in
pure Go on [Gio](https://gioui.org). effects is where a component stops being
correct and starts being alive: the shadow under a toast, the press that gives way under the pointer and springs back, the
fade that carries a notification out, the frosted glass behind a dialog.

Gio hands you `op/paint` and a frame callback. It has no shadow, no radial
gradient, no blur, no animation clock and no notion of a spring — a component
that moves is one whose developer wrote the interpolation, decided what "one
frame" means, and remembered to ask for the next frame. effects writes that once.
The animation core is two flavours that answer to different problems. `tween`
is a value type: from, to, frames, a lerp function, no clock and no state, for
motion that just has to land where you said. `spring` is a real damped-spring
simulation over one [traer](https://github.com/vibrantgio/traer) particle, for
motion that has to feel like it has mass. `motion` composes both,
`springbutton` is components' button wired to a spring, and `transition` teaches
`tween` the platform's colour set and its drop shadows. Alongside the animation core sit the static
effects — `depth` and `glow`, gradient compositions — and `blur`, the module's
own Gaussian blur pipeline for the primitive Gio does not have.

**effects never decorates components globally.** There is no theme flag and no wrapper
that quietly animates every button in an application. An animated component is
a component of its own, exported beside the components one it animates and named at the
call site — `springbutton.SpringButton` where you would have written
`button.Button` — so reading the call tells you the thing moves, the
unanimated component keeps costing nothing, and the dependency runs effects →
components and never the other way. `motion.Apply` is the mechanism for building
your own: wrap any components render function in it.

## Where it sits

The tier above components in the stack. effects imports
[mvu](https://github.com/vibrantgio/mvu), `theme`, `tokens` and `color` from
[theme](https://github.com/vibrantgio/theme), `button` from
[components](https://github.com/vibrantgio/components), and the
[traer](https://github.com/vibrantgio/traer) particle system. The
[organization page](https://github.com/vibrantgio) has the full tier table.

```sh
go get github.com/vibrantgio/effects
```

## Packages

| Package | |
| --- | --- |
| `tween` | `Tween[T]{From, To, Frames, Lerp}` and `At(n)`. A value type with no clock, no easing and no state — the caller decides what a frame is. `LerpFloat64` and `LerpNRGBA` cover opacity, position and colour. |
| `spring` | One scalar pulled toward a target by a damped spring, simulated as a two-particle `traer.ParticleSystem`. `SetTarget` retargets mid-flight without losing velocity; `Value`, `Velocity` and `Target` read it; `Settled` says when to stop asking for frames. |
| `motion` | `Enter`, `Exit` and `Transition`: a tween for opacity and a spring for scale, ticked together, between the `State`s `Hidden` and `Visible`. `Apply` is the bridge to Gio — it lays a `layout.Widget` out once and replays it inside a scale-around-centre transform and an opacity layer, so the footprint never jitters. Defaults resolve from the token motion scale: `FramesAt` converts a duration stop at a frame rate, `SpringOptions` converts a `tokens.Spring` preset. |
| `springbutton` | `components/button` with a press that scales down and springs back, on the same props and the same visual contract — including `Props.Emphasis`, since it renders through the pure `button.Render` and has to copy across every field the two structs share. The spring is the theme's `Motion.SpringSnappy` preset, each frame steps it by the frame's elapsed time, and under reduced motion the press lands in one frame. |
| `transition` | Interpolates the platform's colour set, so a light-to-dark flip can cross-fade rather than snap: `LerpPlatformColors` lerps every field of two `tokens.PlatformColors` at a parameter in [0,1], `PlatformColorsTween` packages that as a `tween.Tween`, and `LerpDropShadow` lerps a `tokens.DropShadow`'s coverage, reach and offset together. It only produces values; driving it is the caller's. |
| `blur` | The blur Gio does not have: a parallel three-pass box approximation of a Gaussian (`Gaussian`, `Box`, `Blurrer`), `Cache` for static imagery blurred once and reused (`DefaultDivisor`, `WithDivisor` for the reduced size it renders at), and `Backdrop` — the "blurred behind the dialog" pipeline on `gioui.org/gpu/headless`, with `FallbackOp` and `Available` for the platforms where headless rendering is not. |
| `depth` | The shadow a floating surface casts: `Shadow(gtx, bounds, radius, sh)` paints the `tokens.DropShadow` it is handed — the platform's `FloatingShadow` or `PaneShadow` — as an interior filled at the shadow's peak coverage, rounded at the caller's radius, and eight linear gradients falling from the peak to nothing over the shadow's reach. A surface that fades passes the shadow with a scaled coverage, `sh.WithPeak(…)`. Opt-in; a shadow marks what floats and can leave — a toast, a popover, a menu — never what is raised in place, which is told by its step of fill alone. |
| `glow` | A luminance spread around a rectangle, composed from eight linear gradients standing in for the radial gradient Gio does not expose. The verdict — why an animated glow is gradients, not blur — is recorded in the package doc, with the measurements. |
| `conductor` | A shared frame counter, so a staggered wave stays phase-locked. Independent per-component simulations drift; participants reading `Local(offset)` off one clock do not. `New` makes one, `Tick` advances it, `Frame` reads it. |

## Usage

`depth` and `tween` are each one line inside a render function. A placed
toast uses both: the shadow goes down first — a toast floats and can leave,
which is what a shadow is for — and the surface is painted over it, at the
same radius:

```go
depth.Shadow(gtx, image.Rectangle{Max: image.Pt(w, h)}, radius, colors.FloatingShadow)
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

An animated component is chosen by name and takes the same props as the
component it animates. The static and the spring button, side by side:

```go
g.btnCompare, err = button.Button(th, button.Props{
	Title:   "Click me",
	OnClick: func(_ layout.Context) { g.btnCompareClicks++; w.Invalidate() },
}).First()

g.springBtnLive, err = springbutton.SpringButton(th, button.Props{
	Title:   "Click me",
	OnClick: func(_ layout.Context) { g.springBtnClicks++; w.Invalidate() },
}, springbutton.Options{}).First()
```

`springbutton.Options{}` is a zero value you can rely on — the spring comes
from the theme's `Motion`, and the press scale falls back to a tuned default.
`spring.Options{}` is not, and `motion.Options{}` is tuned but does not add
up; both are in the status section below.

Driving a `spring` or a `motion` primitive yourself is the other half. The
object is stateful, so it has to be allocated once per subscription — inside
the `rx.Defer` closure, the same place a components control keeps its
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

**What not to blur: animated glows.** The measurements are in `glow`'s
package doc: an animating
blur-glow costs 0.2–0.8 ms of events-thread CPU plus an allocation and a
texture upload per glow per frame, against ~0.5 µs for `glow`'s
eight-gradient spread, and no cache holds while the radius or intensity
animates. Use `glow` for spreads — a correct approximation beats a slow exact
answer.

## For coding assistants

Read the org guide before you write code against this module: the plan
root's [`AGENTS.md`](https://github.com/vibrantgio/organisation/blob/master/AGENTS.md).

The build, test and golden-image commands are reached from the same
[`organisation/AGENTS.md`](https://github.com/vibrantgio/organisation/blob/master/AGENTS.md).
`-golden.update` follows the package list (`go test ./depth -golden.update`);
placed before it, `go test` tests the current directory only.

## Status

Honest about what does not work yet. Every number below is measured, not
estimated.

- **`springbutton` is the only animated component.** `motion.Apply` takes
  any components render function, but no other animated component is
  written.
- **`motion`'s defaults do not finish together.** `DefaultFrames` and
  `DefaultSpring` resolve from the token motion scale, and they do not land
  in step: a `NewEnter(Options{})` ticked at invDt=60 has its
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
  redraw at 60 Hz — to settle to 0.005. `motion` sets its own, and
  `springbutton` reads the theme's `Motion.SpringSnappy` preset.
- **`glow` reserves no space and measures in pixels.** `Spread` draws outside
  the bounds it is given and returns nothing, so in a flex it spills over its
  neighbours unless the caller insets or clips; and `Options.Radius` is raw
  pixels rather than dp, which is backwards from every other size in the
  system. The blur-based alternative was measured and rejected, as the
  package doc records.
- **The primitives count frames, not time.** `tween` and `conductor` count
  frames by construction, and `spring` and `motion` take their step from the
  caller. The token motion scale carries duration stops and spring presets,
  and `motion.FramesAt` converts a stop at a stated frame rate. Only
  `springbutton` steps by the frame's elapsed time, read from `gtx.Now`.
- **Reduced motion reaches a spring only through its caller.** The theme's
  `Motion` observable emits `MotionScale.Reduced()` — every duration zero —
  while the OS preference is on, and anything timed off a duration stop,
  `components/tooltip`'s delay among them, snaps for free. `springbutton`
  reads the zero durations and lands its press in one frame. Code that drives
  a `spring` or a `motion` primitive itself must read the zeros as the snap
  signal.

## License

MIT — see [LICENSE](./LICENSE).
