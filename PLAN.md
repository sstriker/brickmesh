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

Now exercised end to end, which immediately found the titles bug recorded under
Corrections.

## M1 — regression tests from the findings — DONE

- [x] two 24t gears mesh at 60 LDU; jam at 58
- [x] the `(t1+t2)/16` rule — tested, and corrected; see below
- [x] exactly 7 of 512 gear triples close on the lattice
- [x] a 24t needs 7.5 degrees of phase against its partner (half a pitch)
- [x] the 5x7 frame yields holes at Z in {-40,0,+40} and X in {-20,0,+20}
- [x] subtractor graph: 2 degrees of freedom, tracks equal when driving
      straight, opposite when pivoting
- [x] no catalog entry has a `~` title or a `[group=...]` snap

Every one of them holds. The last four need real part geometry, which synthetic
fixtures cannot supply, so they live in `tests/test_real_libraries.py` behind
the `libraries` marker: off by default, on with `BRICKMESH_LIBRARIES=1`, and run
by a CI job that caches both libraries between runs. A cold run takes about a
minute; a warm one, thirty seconds.

Two caveats came out of writing them:

- `mesh_lock` is sensitive to angular resolution. The free window either side of
  a meshed tooth is a couple of degrees wide, so sampling every 5 degrees steps
  over it and reports a confident "cannot be assembled" for a pair that meshes
  perfectly. The default of 144 steps is fine; coarser is not, and the failure
  does not look like a resolution problem.
- The hole probe reports corner artifacts. Just outside a corner the thin probe
  misses the part while the thick one still clips it, which has the same
  signature as a hole. Every such hit lies outside the part, so position alone
  separates them, but `find_holes` does not do it for you.

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

## M3 — port to Go

Profiling showed the bottleneck was never the language: it was a missing index
and needless object creation. The Python version is now 55x faster. Go is for
distribution and one language instead of two, not rescue.

The original plan said to keep extraction in Python because the parsing is full
of edge cases that each had to be found. That reasoning has weakened: those edge
cases are now executable tests rather than memory, and a port is checkable by
running both extractors over the whole library and diffing. Which is how the
three-axis grid bug surfaced within an hour of starting.

- [x] step 1: reading. `internal/ldraw` (fetch, cache, resolve, the
      repeated-subfile rule), `internal/shadow` (snap metadata, titles,
      download and extract) and `internal/extract` (grid expansion, tiers, the
      subpart filter, records). `cmd/brickmesh-extract` writes the same schema
      to the same cache directories as the Python one.
- [x] step 2: `internal/mech` (degrees of freedom, speeds, the five checks,
      backlash), `internal/layout` (shaft lines, the backtracking search, gear
      stations, free intervals) and `internal/rigidity` (joints, components,
      the mobility formula). `internal/synth` holds the vocabulary they share —
      a placed part, hole offsets, the beam inventory — with the search itself
      still to come.

      No catalog file passes between these, so there is nothing to diff. What
      holds them together is that the Python tests were ported case for case:
      the same seven closing triples of 512, the same two that are actually
      buildable, the same speeds through a subtractor, the same hinge and
      four-bar results. Both suites assert the same numbers independently.

      The linear algebra is hand-rolled: rank by elimination, least squares
      through the normal equations. numpy uses LAPACK, but these matrices are
      one row per transmission by one column per shaft with small integer
      coefficients, so it keeps the engine free of a dependency for no loss of
      accuracy at this size.
- [x] step 3: `internal/voxel` rasterizes to a bitset grid; `internal/synth`
      is the greedy set cover with restarts, parallel and reproducible per
      seed; `internal/connect` is the A* chain search. The top-k truncation is
      simply absent — `internal/catalog`'s index returns every placement, since
      truncating only ever dodged Python's allocation cost.

      `internal/part` was carved out on the way. The repair phase asks the
      rigidity check which pieces hang loose while the rigidity check needs the
      search's vocabulary; Python breaks that with an import inside a function,
      which Go cannot do, so the shared nouns live in a package neither owns.

      Two things are ported but not yet wired to each other: `synth` repairs
      connectivity with straight beams only, while `connect` can route a chain
      through arbitrary parts. Making the repair phase call the A* search is
      M2's "connection phase", and is what turns a structure that bears every
      shaft into one that also holds together.
- [ ] step 4: triangle-triangle intersection to replace FCL, gated on
      reproducing two 24t gears meshing at 60 LDU and jamming at 58

Both extractors agree exactly: 2,649 parts and 21,675 ports, every field
identical, enforced by `tests/test_go_parity.py` in the libraries job. The
Python extractor stays until step 4 lands, as the reference to diff against.

What is not being ported: `bevel.py` and `holes.py`. They are instruments whose
output is a number destined for `docs/findings.md`, and they lean on trimesh,
FCL and scipy, which Go has no equivalent of. Rewriting an uncalibrated
instrument in another language does not make it trustworthy.

## M4 — translate the Python source — DONE

All sixteen modules under `extract/brickmesh_extract/` are English, including
docstrings, comments and the strings that reach the user. The `Finding`
categories went with them, so anything matching on them by name needs updating:
`samenhang` is now `connectivity`, `starheid` `rigidity`, `vrijheidsgraden`
`dof`, `lagering` `bearings`, `lussluiting` `loop closure`, `botsing`
`collision`, `tandstand` `tooth phase`, `rooster` `grid`.

## Corrections

Three things did not survive being tested.

**The extractor read the wrong titles.** `build.py` passed the shadow parts
directory to `catalog.usable`, which expects an LDraw one, so every title came
out as `LDCad shadow info for "..."`. That matches no tier pattern and no
subpart prefix, so nothing was filtered and every part landed in tier 3 —
silently, since the output was otherwise well-formed. Titles now come from
`catalog.shadow_titles`, which reads the real title the shadow header quotes
back; no LDraw fetch needed, and the `~` prefix is carried through. Over the
first 400 parts that is 27 subparts dropped and a 12 / 17 / 300 tier spread
where before it was 0 / 0 / everything.

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

## Open question: the third grid axis

92 grid specs in the shadow library declare three axes rather than two. The
count is unambiguous — each axis contributes a count, a spacing, and a leading
C when centered, so `t` tokens of which `c` are C means `(t-c)/2` axes — but
which local direction the third one is has not been established. For two axes
it is local X and Z, the pair perpendicular to the cylinder; the third is
presumably the cylinder's own axis, but presumably is not good enough to place
a port on.

72 of the 92 have a degenerate first axis (count 1, spacing 0), so the ordering
only changes the answer for the other 20. Both extractors currently keep the
one position the file states outright and drop the repeats, which is incomplete
but never wrong.

Settle it from the LDCad documentation, or by taking a part with such a grid and
checking its holes against a physical one.

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
