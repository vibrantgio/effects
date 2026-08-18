# AGENTS.md — effects

The effects layer of the Vibrant Gio design system: the `tween` and
`spring` interpolators, `conductor`'s shared animation clock, `glow` and
`depth` for luminance halos and Material-style cast shadows, `motion`'s
enter and exit primitives, `springbutton`, the spring-physics variant of
components' button, `transition`, which interpolates a whole set of colour
tokens so a light-to-dark flip cross-fades instead of snapping, and `blur`,
a CPU Gaussian approximation over `image.NRGBA` for the backdrop-blur
pipeline Gio itself has no primitive for.

**Layer.** Tier 3 of ADR-001's stack, `mvu → theme → components → effects →
patterns → markdown`. The prism-pulse cycle that once pinned half the
organization to `prism v0.0.3` is cut, in both directions: ADR-001's tier
rule forbids the edge that closed it from below — `check-layers.sh` fails
the build on one — and the `spectrum/transition` alias that carried the
other half went with spectrum v0.2.0. Its root module imports `components`,
`mvu`, `theme` and `traer`, and reaches `font` through them. That direction
is measured rather than typed — `scripts/check-layers.sh --edges` reports
the graph and `scripts/sync-agents.sh` renders these sentences from it — so
correcting them here changes nothing. The other direction is measured too
and deliberately not written down: the gate checks the graph both ways, but
a public API's consumers are unknowable, so this file says what its module
needs and never who needs it.

**Read the canonical guide before you write code against this module.** It is
the organization's one agent guide — the module inventory with current tags,
the application skeleton, the MVU loop and rx semantics, typography, and the
pitfalls that are not guessable. It lives exactly once, in `vibrantgio/.github`,
and this file links it rather than copying it:

    https://raw.githubusercontent.com/vibrantgio/.github/master/llms.txt

**Module.** `github.com/vibrantgio/effects`, one module at the repository
root.

**Build and test.** From the repository root:

    go build ./... && go test ./...

**Golden images.** Tests in four packages compare rendered output against
PNGs committed under `testdata/golden/`. They render through
`github.com/vibrantgio/components/golden`, which declares `-golden.update`
and is the organization's only golden harness. Do not inline a copy of it,
and do not declare a second `-golden.update`: two registrations of one flag
name in a single test binary panic in `flag.Bool` at init, before any test
runs. When a change legitimately moves pixels, regenerate them within the
same change, look at what came out, and say so in the commit. From the
repository root:

    go test ./depth ./glow ./motion ./transition -golden.update

Both halves of that line matter. `go test` cannot tell that an unfamiliar
flag is boolean, so a flag placed before the packages swallows them: `go
test -golden.update ./...` tests whatever package the repository root
holds, not `./...`. And `./...` cannot stand in for the list — this module
has test packages that store no goldens, and a test binary rejects a flag
it never declared.

**A green CI run does not say these images matched. They are compared only
on a developer's machine, and that is deliberate.** The harness answers a
failed `headless.NewWindow` with `t.Skipf`, a skipped test passes, and the
runner has no GL driver for it to open — so the pixels and the build status
are independent facts. The `build` job's *Were the golden images compared,
or skipped?* step, added by F5.4, publishes which of the two happened as a
workflow annotation, readable without a token at `GET
/repos/vibrantgio/effects/commits/<sha>/check-runs`; it has answered
SKIPPED on every run. F5.7 then measured the alternative rather than
leaving it as an open question. Adding the drivers gio's own Linux CI
installs — `libegl1`, `libegl-mesa0`, `libglx-mesa0`, `libgl1-mesa-dri`,
`mesa-libgallium`, `libgbm1`, `mesa-vulkan-drivers` — does work: on pulse
the verdict flipped to COMPARED on the next run. Nine of that repository's
twenty-one images then failed, 12782 pixels apart, while the three drawn on
the CPU still matched exactly. Every golden in the organization was
recorded on macOS, so the gate would not be asserting that the images are
right, only that Linux mesa and Metal rasterise identically — which they do
not, and need not. **So CI gates the build and the tests, never the
pixels**, and moving an image is checked where it is regenerated.

**A golden test pins its faces; application code does not.** Every golden
and pixel test here builds its shaper with
`tokens.DefaultTypography.DeterministicShaper()` — the default typography's
faces and nothing else, system fonts off, so the stored PNGs are the same
on every machine. Applications call `Shaper()` instead, which falls back to
the platform's own fonts so that text outside Roboto and Roboto Mono still
resolves. The two are not interchangeable: a golden written against
`Shaper()` passes on the machine that wrote it and fails on one with a
different font set, which is the failure the split constructor exists to
make impossible.

When a test genuinely needs a glyph the default faces lack, widen the
collection rather than reach for the system:

    tokens.DefaultTypography.WithFaces(notosansmono.FontFace()).DeterministicShaper()

Then assert that the shaper resolved the rune, rather than storing the
result as pixels. A stored image proves the glyph came out somewhere; only
the assertion says which face drew it.

**`transition` is the odd package out, and it is the control.** Its swatches
are drawn on the CPU with `image/draw` — colour interpolation, no widget
rendering — so it calls `golden.CompareNRGBA` on an image it built itself
rather than `golden.Render`, and needs no headless window at all. That is
what made this repository the one F5.7 ran its experiment on: with the GL
drivers installed on the runner, nine of the twenty-one stored images differed
under Linux mesa while these three still matched exactly, which is how the
harness, the PNG encoding and the comparison were cleared and the GPU
rasteriser was not.

F5.5 deleted the four inlined harnesses that used to live here, one per
package.
