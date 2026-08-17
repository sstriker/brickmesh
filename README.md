# brickmesh

Tooling to analyse and construct LEGO Technic mechanisms *before* you build
them. Original creations rarely fail on the idea and almost always on the
execution: the gear train does not close on the lattice, a shaft has only one
bearing, the structure hinges. This catches that.

## Layers

Each layer adds constraints the previous one cannot express. That is not
tidiness but necessity — a functional graph happily allows two differentials to
occupy the same place.

| layer | question | catches |
|---|---|---|
| **functional** | what drives what? | locked trains, motors fighting each other, loops that do not close |
| **geometric** | where do the shafts lie? | centre distances off the lattice, parts overlapping |
| **stations** | where on the shaft do the gears sit? | gears overlapping, no room left for bearings |
| **structural** | what holds it together? | shafts with one bearing, loose pieces, hinging structures |

The functional layer is a linear system: every transmission is one equation
between shaft speeds, and the null space of the matrix is your degrees of
freedom. A subtractor should have two.

## Architecture

Python does the data extraction, Go does the computation.

```
extract/     Python. Reads the LDraw parts library and the LDCad shadow
             library, expands the grids, writes a single catalogue file.
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
- catalogue: 2796 usable parts, 21617 ports from the shadow library
- collision detection: voxel occupancy (coarse) and FCL (exact)
- tooth phase: derived from geometry, self-checked against half a pitch
- rigidity: mobility formula, planar and spatial

Not finished:

- the covering search produces structures that bear the shafts but do not
  always hang together
- the connection search does not yet count loose pins in its cost
- bevel engagement: the position cannot be derived reliably, see
  `docs/findings.md`

Note: the Python modules under `extract/` still carry Dutch docstrings and
comments. Translation is pending.

## Licence

Apache 2.0, see `LICENSE`.

## Data provenance

This repository does not contain the libraries; they are fetched on first use.
See `ATTRIBUTION.md` — the LDCad shadow library is CC BY-SA 4.0 and that
share-alike condition carries through to derived data.
