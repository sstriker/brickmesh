# Work plan

Read `docs/findings.md` first — it holds the non-obvious facts that were
expensive to discover, and several of them will otherwise be rediscovered the
hard way.

Status is kept current as milestones land. See "Corrections" at the bottom for
two facts that turned out differently once they were tested.

## Ground truth before anything else

Two rules that came out of a long and error-prone development:

1. **Do not trust a geometric measurement that has not been calibrated.**
   Two 24-tooth gears mesh at exactly 60 LDU. Any new collision or engagement
   test must reproduce that before its other answers mean anything. During
   development, nine different tests gave nine different answers to the same
   bevel question because none of them was calibrated.

2. **When a test disagrees with a documented rule, check the test first.**
   The pitch rule and effective radius are arithmetic and reliable. The
   collision pipeline is built on open-shell meshes and is not.

## M0 — make the repository run — DONE

- [x] `extract/brickmesh_extract/build.py`: fetches the libraries, builds the
      catalog, filters subparts and grouped snaps, optionally infers missing
      hole rows (`--infer-holes`), assigns tiers, and writes `data/catalog.json`
      in the schema `internal/catalog` expects (`id`, `title`, `tier`, `holes`,
      `pins`, each port `[x,y,z,ax,ay,az,cross]`).
- [x] `go build ./...` — compiles, vets and passes staticcheck. The corrections
      expected in `geom.go` and `catalog.go` turned out to be a documentation
      error rather than a code one; see Corrections.
- [x] Round-trip test: `to_records` writes the schema,
      `internal/catalog/testdata/catalog.json` holds its output, and both
      suites read that same file, so a change on either side fails a test.

Not yet exercised end to end: the full `build.py` run against the real
libraries. Only its pure half is under test.

## M1 — regression tests from the findings

- [ ] two 24t gears mesh at 60 LDU; jam at 58
- [x] the `(t1+t2)/16` rule — tested, and corrected; see below
- [x] exactly 7 of 512 gear triples close on the lattice
- [ ] a 24t needs 7.5 degrees of phase against its partner (half a pitch)
- [ ] the 5x7 frame yields holes at Z in {-40,0,+40} and X in {-20,0,+20}
- [x] subtractor graph: 2 degrees of freedom, tracks equal when driving
      straight, opposite when pivoting
- [x] no catalog entry has a `~` title or a `[group=...]` snap

The three unchecked items all need real part meshes, and the repository ships
none: the fixtures under `tests/fixtures/` are synthetic boxes and a square
tube, which cover parsing, grid expansion, the voxel grid and the hole probe,
but cannot stand in for tooth geometry. Closing them means either committing
measurements as constants or running against a fetched library in a job that is
allowed to download.

## M2 — finish structural synthesis

The covering search finds structures that bear every shaft but do not hang
together; the rigidity check correctly reports them as loose pieces.

- [ ] connection phase: a straight beam only joins parts whose holes are
      already collinear. Perpendicular components need intermediate parts.
      This is a Steiner tree problem — connect terminals via as few
      intermediate nodes as possible.
- [ ] count the pins that hole-to-hole joins require; they are real parts and
      currently invisible in both the cost and the output
- [ ] feed rigidity back in as a hard constraint rather than a post-check

Acceptance: synthesized structures report `M <= 0` from `rigidity.analyze`.

## M3 — port the hot path to Go

Profiling showed the bottleneck was never the language: it was a missing index
and needless object creation. The Python version is now 55x faster. Go is for
distribution and parallelism, not rescue.

- [ ] `internal/voxel`: occupancy as bitsets
- [ ] `internal/synth`: greedy set cover with restarts, parallel over restarts
- [ ] A* connection search
- [ ] drop the top-k truncation the Python version needs — it is a correctness
      compromise forced by allocation cost, and Go does not need it

Keep extraction in Python. The parsing is full of edge cases that each had to
be found.

## M4 — translate the Python source — DONE

All sixteen modules under `extract/brickmesh_extract/` are English, including
docstrings, comments and the strings that reach the user. The `Finding`
categories went with them, so anything matching on them by name needs updating:
`samenhang` is now `connectivity`, `starheid` `rigidity`, `vrijheidsgraden`
`dof`, `lagering` `bearings`, `lussluiting` `loop closure`, `botsing`
`collision`, `tandstand` `tooth phase`, `rooster` `grid`.

## Corrections

Two documented facts did not survive being tested.

**The pitch rule does not always land on a half stud.** `(t1+t2)/16` studs is
correct, but the claim that every standard pair therefore falls on the
half-stud lattice is not: that holds only when the two tooth counts SUM to a
multiple of 8. Each being a multiple of 4 is not enough. 8t+12t needs 25 LDU
and 36t+40t needs 95 LDU, and neither is buildable without an offset trick.
`Mechanism.check_center_distances` reports such a pair, and `layout.realize` no
longer rounds the squared distance to an integer — which used to place a
36t+40t pair at sqrt(90) = 9.487 half studs instead of 9.5.

**`check_closure` only validates the third shaft.** It puts the first at the
origin and the second at (d, 0), then tests whether the third lands on the
lattice — without checking that the second one did. So of the seven triples it
passes, `(8,12,40)` and `(12,24,36)` each contain an off-lattice pair and
cannot actually be built. Both checks now run in `run_checks`, so the FAIL
shows up alongside, but the wording of the closure finding is optimistic. Worth
deciding whether closure should require every pairwise distance to be on the
lattice, which would reduce the documented seven.

## Open question that code cannot settle

Bevel engagement position. Nine measurement approaches disagreed. The
documented rule says each gear sits at the other's effective radius from the
axis intersection; the collision pipeline disagrees, and the pipeline is built
on non-watertight meshes. Resolve by measuring a physical differential and a
12-tooth gear, then record the number in `docs/findings.md` and use it as a
fixed constant.

Note in passing: the fixture meshes are watertight and consistently wound, so
they are a usable calibration target for the *mechanics* of a collision test,
though not for tooth geometry.
