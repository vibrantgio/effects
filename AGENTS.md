# AGENTS.md — pulse

The effects layer of the Vibrant Gio design system: the `tween` and
`spring` interpolators, `conductor`'s shared animation clock, `glow` and
`depth` for luminance halos and Material-style cast shadows, `motion`'s
enter and exit primitives, and `springbutton`, the spring-physics variant
of prism's button.

**Layer.** Tier 3 of ADR-001's stack, `mvu → spectrum → prism → pulse →
cadence → markdown`. It imports mvu, `prism/button`, `prism/theme` and
`prism/tokens`, plus the support library traer; cadence imports it, and
spectrum imports `pulse/tween` today — an edge goal G-B3 removes. The
prism-pulse cycle that once pinned half the organization to `prism v0.0.3`
is cut: prism's root module no longer requires pulse, only its exempt
`gallery` demo module does.

**Read the canonical guide before you write code against this module.** It is
the organization's one agent guide — the module inventory with current tags,
the application skeleton, the MVU loop and rx semantics, typography, and the
pitfalls that are not guessable. It lives exactly once, in `vibrantgio/.github`,
and this file links it rather than copying it:

    https://raw.githubusercontent.com/vibrantgio/.github/master/llms.txt

**Module.** `github.com/vibrantgio/pulse`, one module at the repository
root.

**Build and test.** From the repository root:

    go build ./... && go test ./...

**Golden images.** Tests in three packages compare rendered output against
PNGs committed under `testdata/golden/`. When a change legitimately moves
pixels, regenerate them within the same change, look at what came out, and
say so in the commit. From the repository root:

    go test ./depth ./glow ./motion -golden.update

Both halves of that line matter. `go test` cannot tell that an unfamiliar
flag is boolean, so a flag placed before the packages swallows them: `go
test -golden.update ./...` tests whatever package the repository root
holds, not `./...`. And `./...` cannot stand in for the list — this module
has test packages that store no goldens, and a test binary rejects a flag
it never declared.

**A golden test pins its faces; application code does not.** Every golden and
pixel test here builds its shaper with
`tokens.DefaultTypography.DeterministicShaper()` — the default typography's
faces and nothing else, system fonts off, so the stored PNGs are the same on
every machine. Applications call `Shaper()` instead, which falls back to the
platform's own fonts so that text outside Roboto and Roboto Mono still
resolves. The two are not interchangeable: a golden written against
`Shaper()` passes on the machine that wrote it and fails on one with a
different font set, which is the failure the split constructor exists to make
impossible. When a test genuinely needs a glyph the default faces lack, widen
the collection rather than reach for the system —
`tokens.DefaultTypography.WithFaces(notosansmono.FontFace()).DeterministicShaper()`
— and assert the shaper resolved the rune rather than storing it as pixels.
