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

Python does the data extraction, Go does the computation.

```text
extract/     Python. Reads the LDraw parts library and the LDCad shadow
             library, expands the grids, writes a single catalog file.
             Run once.

internal/    Go. Index, occupancy grid, search algorithms, .ldr output.
             Runs on every query.
```

The split is deliberate. The parsing is full of edge cases that each had to be
discovered (see `docs/findings.md`); you do not want to write that twice. The
computation is a tight problem where Go's structs, absent GC pressure and real
parallelism pay off directly.

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

## Building

Go 1.22 or newer for the engine, [uv](https://docs.astral.sh/uv/) for the
extractor. The two halves build and test independently; the Makefile runs both.

```console
make build   # the Go binary, into bin/
make test    # Go and Python tests
make lint    # vet, gofmt, staticcheck, complexity lens, ruff
```

Building the catalog needs numpy and nothing else. The mesh-level modules
(`holes`, `voxel`, `interfere`, `bevel`) additionally need a collision library,
which is the `geometry` extra:

```console
uv sync --extra geometry
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

`prek run --all-files` runs the same hooks CI runs. Every source file carries an
SPDX header, and a pre-commit hook adds one to files that do not.

## License

Apache 2.0, see `LICENSE`.

## Data provenance

This repository does not contain the libraries; they are fetched on first use.
See `ATTRIBUTION.md` — the LDCad shadow library is CC BY-SA 4.0 and that
share-alike condition carries through to derived data.
