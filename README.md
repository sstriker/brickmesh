# brickmesh

Tooling to analyze and construct LEGO Technic mechanisms *before* you build
them. Original creations rarely fail on the idea and almost always on the
execution: the gear train does not close on the lattice, a shaft has only one
bearing, the structure hinges. This catches that.

## Layers

Each layer adds constraints the previous one cannot express. That is not
tidiness but necessity — a functional graph happily allows two differentials to
occupy the same place.

| layer | question | catches |
| --- | --- | --- |
| **functional** | what drives what? | locked trains, motors fighting each other, loops that do not close |
| **geometric** | where do the shafts lie? | center distances off the lattice, parts overlapping |
| **stations** | where on the shaft do the gears sit? | gears overlapping, no room left for bearings |
| **structural** | what holds it together? | shafts with one bearing, loose pieces, hinging structures |

The functional layer is a linear system: every transmission is one equation
between shaft speeds, and the null space of the matrix is your degrees of
freedom. A subtractor should have two.

## Architecture

All Go.

```text
internal/    The engine. Reads the LDraw parts library and the LDCad shadow
             library, works out what meshes what, places it, frames it, and
             writes .ldr and an LDCad animation.

cmd/         The commands: brickmesh, brickmesh-extract, brickmesh-assets,
             brickmesh-torque, and the WebAssembly build behind the page.

web/         The functional layer as a static page.

tests/       A little Python, which runs the emitted Lua under a real Lua
             runtime. See tests/README.md.
```

It began in Python and was ported module by module; `docs/findings.md` records
what each port was checked against — the extractor byte-identical over 2,649
parts, the collision test matching FCL numerically, torque agreeing across
eighteen trains. The Python went when the port finished, because a second
implementation that has fallen behind answers confidently and wrongly.

## Status

Working and validated:

- functional layer: degrees of freedom, speeds, loop closure, torque
- geometric layer: shaft lines on the lattice, gear stations
- catalog: 2796 usable parts, 21617 ports from the shadow library
- collision detection: voxel occupancy (coarse) and FCL (exact)
- tooth phase: derived from geometry, self-checked against half a pitch
- rigidity: mobility formula, planar and spatial

Not finished:

- the covering search produces structures that bear the shafts but do not
  always hang together
- the connection search does not yet count loose pins in its cost
- bevel engagement: the position cannot be derived reliably, see
  `docs/findings.md`

## Plan

See [PLAN.md](PLAN.md) for the prioritized work queue and what is already done.

## Installing it

The engine is a Go program:

```console
go install github.com/sstriker/brickmesh/cmd/brickmesh@latest
```

Claude Code can drive it, through a skill that knows the description format and
how to read the findings:

```console
/plugin marketplace add sstriker/brickmesh
/plugin install brickmesh@brickmesh
```

The two are separate on purpose — the skill is instructions and the engine is a
program. Without the engine the skill can still design and check a mechanism;
it needs the engine to place the gears and write the file. To install the skill
by hand instead, unpack
[brickmesh-skill.zip](https://sstriker.github.io/brickmesh/skill/brickmesh-skill.zip)
into `~/.claude/skills/`, or read
[SKILL.md](https://sstriker.github.io/brickmesh/skill/SKILL.md) first.

## Running it

A mechanism is described functionally — shafts and what connects them, no
positions and no part numbers, because that is the only level at which it can
be stated before it has been worked out:

```json
{
  "name": "reduction",
  "shafts": [{"id": "input", "bearings": 2}, {"id": "output", "bearings": 2}],
  "meshes": [{"a": "input", "b": "output", "teeth_a": 8, "teeth_b": 24}],
  "inputs": [{"shaft": "input", "speed": 1.0}],
  "outputs": ["output"]
}
```

```console
brickmesh --spec examples/reduction.json --out reduction.ldr
```

It runs the layers in order and reports what each one found:

```text
  OK    [dof         ] 1 degrees of freedom, 1 drives: determined
  OK    [bearings    ] every shaft borne at both ends
  OK    [center dist ] every spur pair lands on a whole half stud
  OK    [station     ] 2 gear stations determined, no conflicts
  OK    [structure   ] 2 parts bear the shafts, 23.5 cubic studs
```

The result is LDraw, which Stud.io opens directly. `--check` stops after the
checks and writes nothing, which is the quick way to ask whether an idea holds
up at all.

`--animate` writes an LDCad animation beside the model:

```console
brickmesh --spec examples/gearbox-2-speed.json --out gearbox.ldr --animate
```

Each shaft becomes an LDCad group and the script turns it at the ratio the
functional layer solved for, so what moves on screen is the mechanism the model
actually is rather than an impression of it. A gearbox gets one animation per
state — and its idle gears keep turning in every one, because they are always
meshed — plus one that walks through the states, sliding each driving ring
between engaged and clear so the shift is something you watch rather than
something that happens between two files.

A box can also be told when to shift for itself, on the speed of a shaft it
watches. The run reports the schedule and checks whether it hunts; see
[docs/shifting.md](docs/shifting.md).

The parts libraries are fetched on first use: the LDraw parts library as one
144 MB archive and the LDCad shadow library beside it, both cached under
`~/.cache`. So the first run is slow and the rest are not. See
[ATTRIBUTION.md](ATTRIBUTION.md) for what they are and what their licences ask
of anything you publish from them.

## Starting from a model

Both directions, for when there is already an `.ldr` or `.mpd`.

```console
brickmesh --read model.ldr --drive line1
```

says what mechanism is in it — gears and tooth counts, which run loose on their
shaft and which are keyed to it, the pairs that stand where they would drive
each other, the driving rings — and works out the ratios. It also says what it
did not recognise, which on a real set is most of the parts: a ratio worked out
from the third of a model that was understood is not a ratio.

```console
brickmesh --fit chassis.ldr --spec mechanism.json --out merged.ldr
```

places that mechanism inside that model and writes a copy of the chassis with it
built in, adding a frame only for the shafts the chassis does not already hold.
It turns the mechanism as well as moving it, so the result may be oriented
differently from what `--spec` alone would build. Add `--replace` where the
model has a drivetrain of its own and the new one should go where it was.

Reading LDraw's 42110 finds 3,012 parts, 65 gears on 125 axis lines and 24 pairs
meeting, and offers 767 lines two studs or more long that a shaft could turn in.

## Will it survive being turned?

`brickmesh` answers whether a train turns. `brickmesh-torque` answers whether it
holds together while it does:

```console
echo '{"input_ncm": 40, "stages": [
        {"name": "8t to 24t", "driver_teeth": 8, "driven_teeth": 24}]}' |
  brickmesh-torque
```

An XL motor at stall through an 8t puts 100 N on a tooth, and the 8t is the
classic first thing to strip. The propagation is exact; the limits it is judged
against are not, and every run prints which of them are estimates.

## In a browser

The functional layer — speeds, degrees of freedom, gear centres, loop closure,
shift schedules — needs no parts library at all, so it runs as a static page
with the engine compiled to WebAssembly:

```console
make serve   # then http://localhost:8080
```

Type a mechanism and the answer follows as you type. Nothing is uploaded; the
engine runs in the browser, and the page fetches nothing outside its own
directory. It ships no data derived from the LDraw or LDCad libraries, so
nothing on it carries their terms.

Build one and it draws, and it moves. **Play** turns the input at a quarter of a
turn a second, so every other shaft runs at its own ratio: two marker arms, one
going round three times while the other goes round once, is the same fact the
ratio table states and a good deal harder to misread. A gearbox offers its
states, and choosing one slides the driving ring to the gear that state selects.

Pick **shift** and it walks the states instead of holding one: each ring waits
out most of its segment and travels near the end of it, and every shaft fed
through a ring that is between gears holds still while it passes. That last part
is the one worth watching — a gearbox in neutral drives nothing, and a page that
kept the output turning through the change would be drawing a shift that works
better than the mechanism does.

The motion is the engine's own. It solves the animation once and renders it
twice — as Lua for LDCad, and as data the page turns into transforms — so the
page cannot drift from the file you open in LDCad.

## Building

Go 1.22 or newer. [uv](https://docs.astral.sh/uv/) as well, for the one test
that runs the emitted Lua.

```console
make build   # the Go binary, into bin/
make test    # the Go tests, and the Lua one
make lint    # vet, gofmt, staticcheck, complexity lens, ruff
```

Most tests run offline against synthetic parts under `tests/fixtures/`, and the
suite fails any test that reaches for the network. The handful that need real
part geometry — gear meshing, tooth phase, the hole probe against a known
part — are opt-in, because they download both libraries:

```console
BRICKMESH_LIBRARIES=1 uv run --extra geometry pytest -m libraries
```

CI runs those in a job that caches the libraries, so only a cold run fetches
anything. Expect about a minute cold and thirty seconds warm.

`prek run --all-files` runs the same hooks CI runs. It needs `uv` on PATH as
well — the ruff hook shells out to `uv run ruff`, and without it prek stops with
a bare "No such file or directory" that names neither. Every source file carries
an SPDX header, and a pre-commit hook adds one to files that do not.

## License

Apache 2.0, see `LICENSE`.

## Data provenance

This repository does not contain the libraries; they are fetched on first use.
See `ATTRIBUTION.md` — the LDCad shadow library is CC BY-SA 4.0 and that
share-alike condition carries through to derived data.
