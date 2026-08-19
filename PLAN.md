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

## M2 — finish structural synthesis — REOPENED

> Reopened 2026-08-19. It was marked done on the strength of "every example
> comes out in one piece and rigid, with no warnings at all", and that measure
> was wrong. Two examples do hinge, and the parts to stop them are not in the
> inventory. See the note at the end of this section.

Three separate things have to be true, and they were separate warnings for a
long time: bearing every shaft, holding together, and not folding up.

The last of those is the one that was missing. `StiffenToRigid` keeps adding
beams while Grübler says the frame still has a degree of freedom. It runs on the
chosen solution rather than inside the search: sixty restarts each stiffening an
answer that is then thrown away costs minutes for nothing.

The connectivity half turned out to be mostly a layout question rather than a
search one. Two parallel shafts an odd half stud apart cannot share a beam, and
no chain of beams reaches between them either — their holes fall on sublattices
half a stud out of step. `checkFraming` now says so before the search runs,
because the answer is a different spacing and not more searching. This is the
rule in docs/findings.md from the other end: a gear pair whose teeth sum to a
multiple of 8 lands on a valid centre distance, but only a multiple of 16 lands
on one you can frame.

`internal/connect`'s A* search is still not wired in. It was the plan for this,
and it turned out not to be what was wrong — but see below, because it is not
what is wrong now either.

### Why this is open again

Mobility is a count: 3(n-1) - 2j for a planar frame. It does not know which
parts a joint is between. So a beam bolted twice to one bearing lowers the
number by exactly as much as one spanning the frame, and `brace` — which asked
only for "two places at once", and took two holes of the same part as two
places — preferred the longest beam available and took it.

The result satisfied every check and was visible only once models were drawn: a
reduction braced with three 13-hole beams marching 35 studs off the end of a
10-stud mechanism, each pinned to the last, none reaching the far bearing, which
the axle had been holding all along. The rigidity report counted the shafts as
joints; the search did not, so it believed the bearings were loose and braced
them. Both halves are fixed — the search is given the shafts, and a brace must
now bridge two bodies that are not already rigid with respect to each other.

What that uncovered is a real gap. Two examples cannot be made rigid at all:

- **subtractor** — two bearing walls on one shaft line, which can counter-rotate
  about the shaft.
- **gearbox-3-speed-compound** — the same, on one of its lines.

No straight liftarm can close either. A shaft passes through a bearing, so the
bearing's holes face along the shaft; every hole of a straight liftarm faces the
same way, so it lies across the shaft with all its holes at one point along it;
and a pin joins two holes only if they are on one line within two studs. So a
liftarm reaches one wall or the other and never both, whatever its length. That
is proved with a control in `TestNoStraightBeamTiesTwoWallsOnAShaftLine`.

- [ ] the inventory needs a part with holes on more than one axis — an angle or
      perpendicular connector — which `part.WorldHoles` cannot express, since it
      returns one axis for a whole part. That signature is the actual work.
- [ ] until then the two hinges are reported honestly and listed in
      `knownHinges`, which fails if one of them stops hinging.

- [x] a joint is now what a joint is: two holes facing the same way, on one
      axis line, within a pin's reach of each other. Holes at the very same
      point were only the degenerate case, and treating them as the whole of it
      meant two parts lying against each other — the normal way to build —
      counted as unconnected. The repair also refuses a bridge that reaches
      without being pinnable to both ends, which stopped it piling on parts
      that changed nothing.
- [x] the shafts are modelled. An axle goes through each line that carries
      anything, long enough to reach past both bearings, and the rigidity check
      counts the parts it threads — in a chain rather than every pair, since
      five parts on one shaft are four constraints and not ten. Both examples
      now report `M <= 0`, which was this milestone's acceptance.

      This was the answer to why the bearings of one shaft could not be joined
      by a beam, which is geometry and not a gap in the search: a bearing's hole
      faces along its shaft and the two sit apart along that same direction, so
      a pin between them would have to run down the shaft's own line. The shaft
      IS what joins them.
- [ ] wiring `internal/connect` in needs richer ports first. The A* search
      matches ports by position, but a port is a point in the catalog while the
      shadow library describes a segment: a pin's entry is a centered cylinder
      of sections 2+16+4+16+2, 40 LDU end to end. Without that extent nothing
      can express a pin reaching between two parts lying against each other.
      Beams are worse: 32523 declares ONE hole and leaves the rest to the
      part's own geometry, so the default catalog has a single port for a
      three-hole beam and `--infer-holes` is opt-in.
- [ ] count the pins that hole-to-hole joins require; they are real parts and
      currently invisible in both the cost and the output
- [ ] feed rigidity back in as a hard constraint rather than a post-check

Acceptance: synthesized structures report `M <= 0` from `rigidity.analyze`.
Both examples do.

One thing the shafts brought to light. A gear pair summing to 40 teeth sits 2.5
studs apart, which is on the half-stud lattice and passes the center-distance
check, but a beam's holes are a whole stud apart — so no beam can reach both
shafts and the two halves of the box cannot be framed together without a
half-stud offset. Choosing counts that sum to a multiple of 16 avoids it: 8+24,
12+20 and 16+16 all make 32, two studs, and their driven gears are 24t, 20t and
16t, every one of which has a driving-ring variant. That is what the example
uses now, and it is a better gearbox than the one it replaced.

## M5 — shifting mechanisms

Done: a mechanism can have **states**, and a **coupling** locks two coaxial
shafts together in the states it names. The gears are always meshed and always
turning; a shift changes which of them is locked to the output. So a gear that
freewheels is its own shaft, coupled only where its ratio is selected.

The kinematic checks run per state — a three-speed box reports three ratios,
and engaging two dog rings at once reads as zero degrees of freedom, which is
what a real gearbox destroys itself doing. The geometric checks run once, since
the parts are physically present whatever state it is in.

Writing it exposed a bug in the station solver. It propagated a plane from any
station already on a shaft, so a second gear pair sharing a shaft stacked into
the first one's plane. That equality only ever held for the two gears *of a
pair*; only a bevel anchor is absolute. Unanchored pairs now take successive
slots along the shaft, which is what lets a gearbox lay out at all.

What is still missing is anything that decides *when* to shift: no centrifugal
governor, no torque-reactive mechanism, no sequential selector. "Auto shifting"
therefore means the box is buildable and the shift mechanism is not.

- [x] place the driving ring. A shift now puts a 6539 beside the gear it
      engages, and the station allocator reserves the room for it. What moves
      the ring is named in the report rather than placed, since its position
      follows from the shift linkage and not from the mechanism.
- [ ] the newer switching systems. Only the classic one can be placed: 18947 of
      the Chiron and the MT-10's ring, selector 35188, shifter 4158, fork 4159
      and the 2474 stepper are all absent from the parts mirror this reads, and
      Rebrickable's numbers are not always LDraw's — 2473 and 2474 resolve to
      "moved to" stubs there. Worth pinning down before promising them.
- [ ] one ring per two gears. A ring engages a gear on either side, so a
      three-speed needs two rings and not three; the report says so but the
      placement does not do it
- [ ] a selector element, so the shift itself is part of the mechanism rather
      than an instruction to the builder
- [ ] a slip element for the other kind of clutch — the old white 24t whose
      inner axle gives way above a force. Different thing entirely from the
      engage/disengage gears, and it shares only the name
- [ ] gear thickness comes from a table of seven counts; a part not in it gets
      a default of 2 half studs

## M3 — port to Go — DONE

Every module has a Go counterpart and the Python has been removed. What each
port was verified against is recorded in docs/findings.md; the one capability
that went with it, hole inference from geometry, is named there too.

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
- [x] step 4: `internal/collide` is Möller's triangle-triangle test under a
      bounding-volume hierarchy, and `internal/interfere` is the meshing sweep
      on top of it. The gate held: two 24t gears mesh at 60 LDU with 24 windows
      one per tooth, jam at 58, and the numbers come out identical to FCL's —
      same window count, spacing, backlash and free fraction at 60 and at 62.

      It is also about 45x faster: a 360-step sweep takes 110 ms against roughly
      five seconds through FCL, because the hierarchy is built once per part and
      only the relative transform moves. The resolution caveat reproduces
      exactly too, which is the more reassuring result — 72 steps still reports
      a confident jam for a pair that meshes.

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

## M6 — the animated export — DONE

`--animate` writes an LDCad animation beside the model. Each shaft becomes a
group, and the script turns it at the ratio the functional layer solved for, so
what moves is the mechanism rather than an impression of it. A gearbox gets one
animation per state, and its idle gears keep turning in every one because they
are always meshed.

The syntax was taken from LDCad itself rather than guessed: its examples under
`AppData/Roaming/LDCad/examples` are the reference. Animation is not a meta
command — the model declares `GROUP_DEF` and tags part lines with `GROUP_NXT`,
names a script with `!LDCAD SCRIPT [source=...]`, and the Lua reaches the groups
by name. The calls used here are the ones in the 5510 example: `ldc.animation`,
`setLength`, `setEvent`, `ldc.subfile`, `getGroup`, `ldc.matrix`, `setRotate`
in degrees about an axis, and `setOri`, which that example is explicit about
being absolute rather than incremental.

Not verified by running it: there is no Lua interpreter on this machine, so the
tests check the file's structure and that the solved ratios reach it, not that
LDCad accepts it. Worth opening once.

- [ ] move the driving rings, so a shift can be watched rather than switched
      between
- [ ] the shift itself as an animation: ring slides, dog engages, ratio changes

## Open question: bevel engagement

Still open, and now with a measured negative to go with it. The sweep that
settles spur meshing finds nothing at any of the 444 candidate positions for a
12t against a differential's ring, because a bevel's teeth do not block and
clear evenly through a revolution. See docs/findings.md. What is left is a
heuristic — most points in contact — whose answer depends on how the surfaces
were sampled.

Settling it needs either a criterion that suits an angled mesh, or one built
mechanism to check an answer against.

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
