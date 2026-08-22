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

## M2 — finish structural synthesis — DONE

> Reopened and closed again 2026-08-19. It had been marked done on the strength
> of "every example comes out in one piece and rigid, with no warnings at all",
> and that measure was being defeated by a brace that braced nothing. Fixing
> that left two examples genuinely hinging. They are closed now, by the part
> that could not previously be expressed. See the two notes at the end.

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

- [x] the inventory needed a part with holes on more than one axis — an angle or
      perpendicular connector — which `part.WorldHoles` could not express, since
      it returned one axis for a whole part. That signature was the actual work.
- [x] `knownHinges` is empty, and kept rather than deleted: it was not empty, and
      the reason it is now is a part in the inventory rather than a fact about
      the world.

### How it was closed

Three things had to be true, and none of them was.

**A part had to be able to have holes facing different ways.** `WorldHoles`
returned one axis for a whole part, from the shadow library's `RotationAxis`,
and laid the holes out from a hole count assuming a straight beam. Both are
exactly true of a straight liftarm. `part.WorldPorts` replaces it: rigidity
matches hole against hole, `CandidatesFor` asks each hole whether it faces along
the shaft, and `beamsSpanning` becomes `partsSpanning`, which asks only whether
two of a part's holes can reach both points.

**The holes had to be readable.** A connector's own shadow file declares one of
its two holes; the rest come from the primitives the part places. See the
findings entry on following subfiles.

**A bearing had to be a bearing.** The first structure built with connectors in
the inventory laid one along a shaft, where two of its cross holes sat on the
shaft line — so the shaft keyed it and drove it round. `CandidatesFor` now
rejects a placement where any cross hole of the part lies on the shaft it is
being asked to bear: an axle seizes in a cross hole, and a bearing has to let
the shaft turn inside it.

What every example costs now, against what it cost when this was first called
done:

| example | parts then | now | cubic studs then | now |
| --- | --- | --- | --- | --- |
| reduction | 9 | 6 | 387.7 | 23.5 |
| gearbox-2-speed | 21 | 16 | 128.8 | 36.0 |
| gearbox-3-speed-compound | 38 | 29 | 273.9 | 100.8 |
| subtractor | 4 | 4 | 125.1 | 27.9 |

`internal/connect`'s A* search is still not wired in, and is still not needed.
The one thing it would add is a bridge of several parts at once, since `brace`
adds one part at a time and each has to bridge on its own. Nothing yet requires
one.

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
- [x] the ports are rich enough now. "32523 declares ONE hole and leaves the
      rest to the part's own geometry" was the blocker, and following the
      primitives a part places is the answer — a three-hole beam has three
      holes and `--infer-holes` has nothing left to infer. The pin extent this
      also asked for is carried as `rigidity.PinReach` rather than as a segment
      in the catalog, which is the same 40 LDU by a shorter road.
- [x] the pins are placed, not just counted. One per run of joints on a hole
      line, midway along it, skipping the lines a shaft already fills, and
      long-pin where a run is deeper than two parts.
- [x] feed rigidity back in as a hard constraint rather than a post-check. The
      search returns solutions smallest first and there are usually many, so one
      that folds is passed over for one that does not, instead of being taken
      and then told off in the report. It falls back rather than failing: if
      every candidate hinges the first that at least fits is taken and the
      rigidity check says so, since a model that hinges is still worth looking
      at and refusing to emit one would hide what the report is for.

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
- [x] the newer switching systems. Both generations are placed now, and which
      one a shift gets depends on the gear it has to lock to. They were absent
      from the parts mirror this used to read; fetching the official library
      whole made them measurable. See `internal/clutch`.
- [x] one ring per two gears. A ring engages a gear on either side, so a
      three-speed needs two rings and not four. The report said so every time it
      ran while the placement went ahead anyway.

      What made it more than bookkeeping is that the hardware generation was
      chosen per gear: the 20t exists only in the second system and the 16t in
      both, so taking the first that fits each landed a pair in two generations,
      and a ring of one does not grip the other's gears. A pair settles it
      between them now, through clutch.ForBoth.

      The two-speed goes from 17 parts to 14 and the three-speed from 31 to 25.
- [x] a selector element, so the shift itself is part of the mechanism rather
      than an instruction to the builder.

      It could not be found by measuring, and that part of the earlier note
      stands: 6641 swept against 6539 through all 24 lattice orientations, four
      directions and every distance up to 40 gives nothing both clear and
      holding, because LDraw models nominal surfaces and a fork that straddles a
      groove touches it.

      It came from LDraw's official 8448, which has a 6641 and three 6539s in
      it. The catch sits 60 LDU out from the shaft on a perpendicular, level
      with the ring — further than the search had gone. Once told where to look
      the sweep confirms it: at 60 the catch reaches the groove and the ring
      still turns, at 55 it is buried, at 65 it has let go.

      Only the first system. 6641 reads as clear of an 18947 at 60, so it would
      sit there holding nothing; the parts that move those rings are in the
      library but nothing here has a model with one beside a ring. Named in the
      report rather than guessed at.

      The same model confirms the engaged distance from a second direction: its
      rings sit exactly 30 LDU from the clutch gears either side of them, which
      is the 3.0 half studs the interference sweep had measured.
- [x] a slip element for the other kind of clutch — the 24t whose centre gives
      way above a force. `"slip_clutches": [{"shaft": "output"}]` fits one; the
      24-tooth station on that shaft becomes 76019 rather than the plain gear,
      and the report says what it protects and at what torque.

      It is refused where it has nowhere to sit. 24 teeth is the only size the
      part is made in, so a slip clutch on a shaft with no 24t is not something
      that can be built — better said than quietly placing a plain gear and
      leaving the reader to notice that nothing slips.

      The figure is an estimate and comes with that attached, the way every
      other number in internal/torque does. Nobody here has measured one.
- [x] gear thickness comes from a table keyed by tooth count, and anything not
      in it gets two half studs. Measured: every gear this engine can place is
      20 LDU along its axis, the clutch variants included, and only the 24t
      differs at 19.25 for its chamfer. The table cannot see the part, only the
      count, so a test now checks every placeable gear against what the layout
      reserves — a gear given too little room sits close enough to its neighbour
      to be called clear when it is not, and nothing else would say so, since
      the clearance sweep allows gear against gear.

## M7 — start from somebody else's model — DONE

Both directions. `--read` takes an `.ldr` or `.mpd` and says what mechanism is
in it: the gears and their tooth counts, which run loose on their shaft and
which are keyed to it, the pairs standing where they would drive each other,
the driving rings, and with `--drive` the ratios. `--fit` goes the other way
and places a mechanism from `--spec` into that model, writing a copy of it with
the mechanism built in and a frame only for the shafts the model does not
already hold. `--replace` takes the model's own drivetrain out first.

The cross hole decides most of the reading. A gear on a cross hole is keyed to
its axle and a gear on a round one turns free, so the same two parts side by
side are a pair that drives or a pair that idles depending on one bit in the
part's own definition.

What the fit cost was four wrong answers, each of them confident:

- The clash test rasterised only gears. A driving ring is 36 LDU across, fatter
  than most gears, and one sat 18 LDU inside a chassis beam while every gear
  cleared it. Rings and joiners are on the shaft too.
- A bearing was a line, and a line runs for ever. Fitted to two walls a hundred
  LDU apart, the two-speed settled at y = -120 — outside both — and reported
  four shafts of four borne. A bearing has to reach the stretch of line the
  gears occupy.
- The verdict came from the coarse filter. Parts are rasterised as surfaces so
  a Technic hole stays a hole; two shells that interpenetrate share only the
  thin ring where the surfaces cross, so no fraction of shared cells separates
  a gear driven 18 LDU into a beam from one resting against it. Voxels rank now
  and the tri-tri test decides — the same predicate clearance uses, since a
  stricter one rejects an axle standing in a hole, which is what holes are for.
- The catch was placed by a routine that knew about other shafts and nothing
  else. Into 42110 it went straight through a beam and a panel.

Three smaller things had to be true before any of that could be: slip shafts
are settled before the fit, so the fitter names the parts the model ends up
with; the shortlist is walked two hundred deep, where eight ran out with a
clean offset still waiting; and parts with no geometry are reported rather than
skipped, because space nothing could be measured in is not empty space.

Where it stands, a two-speed fitted with `--replace`: 42110, 42083 and 8880
take one with nothing sharing space; 42099 is refused, and the clearance check
agrees with the refusal. That agreement is the property worth keeping — the fit
and the check that judges it now answer the same question.

- [ ] the fit cannot say which side a catch will end up on, because the
      structural search settles that afterwards. It asks the weaker question
      instead — whether any of the four sides is free — and an offset where
      none is gets refused. Answering the stronger one means moving the catch
      into the search, which is a larger change than it has earned.

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

- [x] move the driving rings, so a shift can be watched rather than switched
      between
- [x] the shift itself as an animation: a `shift` animation walks the states,
      sliding each ring between engaged and clear over the last quarter of each
      segment, so the shift is a thing that happens rather than a thing between
      frames

## Bevel engagement — the rule now rests on a measurement, not a document

Was open on the grounds that nine measurement approaches disagreed. They did,
and all nine were asking the same wrong question: **where do the surfaces touch**
rather than where the pitch circles do. See docs/findings.md.

The rule the layout applies — each gear at the OTHER's pitch radius from where
the axes cross — has two halves, and both are now accounted for.

**The module is measured.** A gear's outermost material is its pitch circle plus
the tooth's addendum, and across nine gears that overhang is 1.0 to 2.6 LDU
against a pitch radius of teeth x 1.25. The three double bevels agree to a
hundredth of an LDU — 2.01 each — which is what says it is a designed addendum
rather than three shapes coinciding. The same module is what makes two 24-tooth
spur gears mesh at 60 LDU, which M1 already tested.

**The placement is geometry.** Two pitch circles have to touch. With the
crossing at the origin, A's axis along z and B's along x, a common point forces
db = Ra and da = Rb. There is nothing to measure there.

`internal/bevel/pitch_test.go` holds both.

### What a physical measurement would still add

Whether LEGO's bevel teeth engage at the theoretical pitch circle or somewhere
slightly off it — real bevels are sometimes cut with a profile shift, and no
amount of reasoning about nominal geometry can see that. So a differential and a
12-tooth gear under calipers would still be worth having, and would confirm or
correct a constant rather than choose one.

## The third grid axis — SETTLED

92 grid specs in the shadow library declare three axes rather than two. LDCad's
own documentation says that cannot happen: a grid is `Xcnt Zcnt Xstep Zstep`,
two axes, "as all snap info is Y-axis orientated only the X and Z grid stepping
values need to be given". The library disagrees with the documentation, so the
question was put to the parts.

**The order is X, then Y, then Z.** The guess recorded here was that the third
axis is the cylinder's own — the right axis, in the wrong place: Y comes second,
not last.

Measured. A pin hole is a bore, so its wall puts mesh vertices all the way round
a circle of the radius the snap declares, and solid material puts them nowhere.
Scoring all six orderings by whether the positions each produces are real holes:

| ordering | every position a real hole |
| --- | --- |
| **X Y Z** | **10** |
| Y X Z | 1 |
| X Z Y, Y Z X, Z X Y, Z Y X | 0 |

Of the eleven female snaps — bores, which is what the measure can see — eight
come out right under X Y Z. The male ones are pegs and the measure does not
apply to them, so their failures are not evidence either way.

`Expand` now lays three-axis grids out along X, Y and Z rather than keeping only
the position the file states. `TestNoOtherAxisOrderingFitsThePartsBetter` runs
the comparison against the real library on every test run, so the reading is
rechecked rather than remembered.

## What code cannot settle

Bevel engagement was here, on the grounds that the documented rule and the
collision pipeline disagreed and the pipeline is built on non-watertight meshes.
The disagreement turned out not to be about the answer: the pipeline was asked
where surfaces touch, which in LDraw's nominal geometry is not where gears mesh.
See above and docs/findings.md.

What is left for a physical measurement is confirmation rather than choice — see
"What a physical measurement would still add".

Note in passing: the fixture meshes are watertight and consistently wound, so
they are a usable calibration target for the *mechanics* of a collision test,
though not for tooth geometry.
