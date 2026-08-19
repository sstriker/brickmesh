# Findings

Facts that are not documented anywhere and had to be measured or worked out one
by one. This is the real asset of the project — the code can be rewritten, this
cannot.

## Gears

**Pitch rule.** Two parallel gears mesh at a center distance of
`(t1 + t2) / 16` studs. All standard tooth counts are multiples of 4, so that
distance always lands on a whole half-stud (10 LDU).

**Effective radius** = `teeth / 16` studs. Checked against measured geometry:
the 28-tooth crown has a tip radius of 36 LDU against a computed pitch radius
of 35, and the 12-tooth 17 against 15. Consistent.

**Bevel engagement.** With perpendicular intersecting axes, *neither* gear sits
at the intersection point. Each sits at the other's effective radius from it.
Consequence: the two stations individually fall off the lattice (12t at 35 LDU,
28t at 15 LDU), but their sum is exactly 50 LDU = 2.5 studs. So they cannot be
positioned independently; one fixes the other.

**Tooth phase.** Every standard gear in LDraw has a tooth at exactly 0° modulo
its pitch. For a meshing pair, one needs a tooth on the line of centers and the
other a gap — half a pitch apart. For a 24t that works out to 7.5°, which
doubles as a self-check: any other value means the tooth extraction is wrong.

**Cross-axle symmetry.** All tooth counts are divisible by four and a cross
axle has four-fold symmetry, so seating a gear on its axle never shifts the
tooth phase. No counting needed during assembly.

**Shared gears.** Two gears with the same tooth count, on the same shaft, in
the same plane, are one physical gear driving two meshes. That saves parts and
search space.

**Loop closure.** Three shafts driving each other in a ring fix three center
distances, and that triangle has to close on the lattice. Of all 512
combinations from eight standard tooth counts, **seven** close. Three 24t gears
give an equilateral triangle with the third shaft at height 5.196 half-studs —
irrational, therefore unbuildable.

## Lattices

**Two incompatible lattices.** A Technic brick is 24 LDU tall, a liftarm 20.
Stack bricks and your holes sit 24 apart vertically and 20 horizontally. Stack
liftarms and it is 20 in every direction. A load path that crosses the boundary
ends up skewed.

Rule: the mechanism lives entirely in the studless Technic lattice. System
parts go on as cladding, never in the load path.

**Angles.** 45° cannot be braced rigidly: the diagonal of an isosceles right
angle is `a*sqrt(2)`, which never lands on a hole. What is buildable are
Pythagorean triples. Within a hull the workhorse is 3-4-5 (36.87 / 53.13
degrees). The closest approach to 45 degrees is 20-21-29 (43.60), which needs a
29-stud reach — useless.

Angle connectors do give exact angles, measured between hole directions:
32016 = 22.48, 32192 = 45.01, 32015 = 67.48, 32014 = 90.00 degrees. A rigid
joint, not a truss.

**Azimuth.** A bevel pinion may sit anywhere around the crown's circumference;
engagement stays perpendicular. The freedom is continuous for the gears but
quantised for the bearings: a bearing at distance d and angle t lands at
(d cos t, d sin t), and both must be on the lattice. Buildable azimuths are
therefore 0, 36.87, 53.13 and 90 degrees, plus 22.62 and 67.38 at a reach of
13 studs.

## Data

**Group snaps.** 108 snaps in the shadow library carry a `[group=...]`: crane
arm, door hinge, electrical connector, ball joint. These must not be treated as
generic pins or holes. Do so and every liftarm appears to have a male port
running its whole length — that is the slot for a crane-arm clamp.

**Subparts.** 206 of 3003 catalog entries have a `~` prefix in their title.
They exist only inside another file and cannot be ordered separately. Leave
them in and the solver will invent structures from parts that do not exist.

**Incomplete hole rows.** The shadow library often describes only one hole of a
liftarm and leaves the rest to the part's own geometry. An 11L beam then has
one known hole out of eleven. Fill in the row at the known 20 LDU pitch along
the length; leave parts that do carry a full grid alone.

**Grid notation.** `[grid=<nx> <ny> <dx> <dy>]`, where a count may be preceded
by `C` for centerd. The grid lies in the snap's own XZ plane and is then
rotated by the orientation matrix. Validated against a physical probe: both
methods give, for the 5x7 frame, transverse holes at Z in {-40, 0, +40} and
longitudinal holes at X in {-20, 0, +20}.

**Orientations.** LDCad snap cylinders point along +Y before the orientation
matrix is applied. Gears have their axis along Z, axles along X, liftarms their
length along Z with holes along Y.

## Collisions

**An axle through a hole cannot be detected as a collision.** Hole and axle are
both 12 LDU; there is no clearance to rasterise. At any resolution the hole wall
and the axle surface land in the same cell. Consequence: the occupancy grid
carries structure only. Axles and gears are handled analytically.

**Contact versus interpenetration.** Two pin-connected parts sit flush; their
surfaces coincide. At a 5 LDU pitch that is indistinguishable from overlap.
Hence a threshold on the fraction of shared cells rather than an absolute
disjointness requirement.

**LDraw parts are open shells**, not closed volumes — the differential has 835
boundary loops. FCL returns answers on those that shift with orientation. Run
measurements you know in advance must come out symmetric, and take the
majority.

**Bevel engagement resists measurement.** Nine different tests gave nine
different answers. What does discriminate is the co-rotation test: turn both
gears together at their true ratio and require that they neither jam nor
separate. Even that is not conclusive. For this one dimension, measuring a
physical part is faster and more reliable.

**Calibration.** Two 24t gears mesh in parallel at exactly 60 LDU. At 58 they
jam; at 60 they turn with 0.19 LDU clearance. Use this as the reference point
for any new collision test. Note that "turns freely" only pins the lower bound —
62, 64 and 66 also turn. The criterion is the smallest distance that still
turns freely.

## Performance

The first version of the connection search took 180 seconds for a single link.
That was not Python but two design mistakes: scanning the entire inventory on
every query instead of indexing by hole direction, and materialising 2600
candidate objects that were then almost all discarded.

Indexing alone changed nothing. Filtering and scoring vectorised, and
materialising only the best candidates, gave 55x.

In Go that last trick becomes unnecessary: structs in a slice are cheap enough
to keep every candidate. That is not only faster but more correct, since
truncating on a heuristic can cut away the right answer.

## The driving ring, measured

The ring is 40 LDU along its shaft against a gear's 20, so their faces meet at
30 LDU between centers. That is also where they engage: at exactly 30 the sweep
finds a 16-tooth clutch gear (6542a, 6542b) blocked for most of a revolution
with sixteen free windows 22.5 degrees apart, one per clutch tooth, which is the
ring's dogs sitting in the gear's recesses. At 31 LDU every gear is free at
every angle.

The plain gears show no such signature at any distance. 4019 gives four windows
90 degrees apart, which is its axle hole and not a clutch; 3648b and 32269 give
none. A driving ring beside a plain gear is scenery, and the only gear in the
library with clutch teeth is the 16t. Real 20t and 24t shifts reach their gear
through a driving ring extension (32187, or 35186 with its eight clutch teeth).

Slicing the ring across its axis, rather than reading off where its vertices
happen to sit, gives the profile the shift linkage has to work with:

| z, LDU | radius | what it is |
| --- | --- | --- |
| ±10 to ±20 | 15 | the hub |
| ±6 to ±8 | 20 | the rims |
| −4 to +4 | 13 | **the groove**, 8 LDU wide |

## The catch does not fit, and cannot be made to

The changeover catch (6641) is a bar 52 LDU long and 20 by 18 across, with two
axle holes at right angles: one at [0 0 -20] along its own X, one at [0 0 -2]
along its own Z. It is 20 across and the ring's groove is 26 in diameter, so it
cannot encircle the groove however it is turned.

Searching every lattice rotation and every position on a 2 LDU grid gives 34,992
placements in which the catch surrounds the ring's center. Every one of them
intersects the ring. That is not a bug in the search — the same sweep reproduces
the 24t reference exactly — it is what an interference fit looks like in
idealised geometry. Real LEGO parts flex and are moulded with clearance; LDraw
models them at nominal size.

So the sweep, which settles whether gears mesh and whether a ring clears its
gear, cannot settle whether a catch has hold of a ring. Anything built on that
joint has to be placed from the shadow library's axle-hole data and said to be
placed rather than verified.

## The parts mirror is missing the newer parts

Generating `meshes.bin` turns this up because it is the first thing that asks
for every part's geometry rather than a handful:

| tier | parts | with geometry | missing |
| --- | --- | --- | --- |
| 1 | 135 | 109 | 26 (19%) |
| 2 | 412 | 340 | 72 (17%) |

Every missing one is high-numbered — 18651, 18948, 21755, 22961, 23948, 24122,
24316, 27940 — and they are absent from the mirror rather than mislooked-up:

```console
$ curl -o /dev/null -w '%{http_code}\n' .../mpetrov/ldraw-parts/master/parts/3648b.dat
200
$ curl -o /dev/null -w '%{http_code}\n' .../mpetrov/ldraw-parts/master/parts/18948.dat
404
```

So `internal/ldraw`'s mirror is a snapshot from before those parts were
released. The shadow library, which comes from LDCad and is fetched whole and
current, knows about them — which is why they are in the catalogue at all.

This is not only a browser-assets problem. Anything that reads geometry hits it:
the voxel rasteriser, the interference sweep, the tooth phase. 18948 is the
axle joiner for the 3L driving ring, so the newer shifting system cannot be
measured at all while this stands — which is why `docs/shifting.md` only has
figures for the first one.

**Fixed.** `internal/ldraw` now fetches the official library whole —
`library.ldraw.org/library/updates/complete.zip`, 144 MB, extracted to about
600 MB of which only `parts/` and `p/` are kept, plus the two licence files. All
135 tier 1 parts have geometry where 109 did before, and 18947 and 18948 are
there, so the 3L driving ring system can be measured after all.

The lesson is worth keeping separately from the fix: the gap was invisible for
as long as nothing asked for every part. Everything that reads geometry read a
handful, and a handful of old, common parts is exactly the set a stale mirror
still has.

## Two shifted ratios per pair of shafts, and no more

Not a limit of this engine. It follows from three measured facts and a bit of
arithmetic.

Only 16 and 20 tooth gears come with dog clutches — the first driving-ring
system has a 16t (6542a, 6542b), the second has a 16t and a 20t (18946, 81346,
35185). The parts named "Technic Gear 24 Tooth Clutch" are the other kind of
clutch, a torque limiter with a slipping centre; swept against both rings they
read exactly like a plain gear at every distance. Neither driving ring extension
(32187, 35186) changes that.

Every gear pair on one pair of parallel shafts has to sum to the same tooth
count, because the shafts are a fixed distance apart. So with the driven gear
restricted to 16 or 20:

| centres | pairs that fit | driven is a clutch gear? |
| --- | --- | --- |
| 2 studs (sum 32) | 16+16, 12+20, 20+12, 24+8 | only 16+16 and 12+20 |
| 2.5 studs (sum 40) | 24+16, 20+20, 16+24 | only 24+16 and 20+20 |
| 1.5 studs (sum 24) | 8+16, 16+8 | only 8+16 |

Two ratios at two studs, two at two and a half, one at one and a half. Never
three.

Which is why real gearboxes with more speeds compound: two stages of two in
series give four, three give eight. `examples/gearbox-2-speed.json` is what a
single pair of shafts can do, and it uses both driving-ring systems at once —
the first for its 16t, the second for its 20t — because that is what the gears
demand.

Compounding has a trap of its own. Two identical stages give
0.6, 0.6, 1.0, 0.36: four combinations, three speeds, because the middle two
coincide. The stages have to differ, and the way to make them differ is to set
them different distances apart:

| stage | centres | ratios |
| --- | --- | --- |
| one | 2 studs | 12+20 = 0.60, 16+16 = 1.00 |
| two | 2.5 studs | 24+16 = 1.50, 20+20 = 1.00 |

which multiply out to 0.60, 0.90, 1.00 and 1.50 — four distinct speeds, and
each gear a 16t or 20t so every one of them can be gripped.
`examples/gearbox-4-speed-compound.json`.

## The meshing sweep does not settle a bevel pair

Spur meshing is settled by turning one gear a full revolution against the other:
engaged, it is blocked for most of the turn and free in as many narrow windows
as it has teeth. Two 24t gears show that exactly at 60 LDU.

A 12t double bevel against the 28t ring inside a differential shows it nowhere.
Sweeping the driver across 1152 positions and keeping the 444 where the two
surfaces come within 1.2 LDU, not one of them produces the signature. Requiring
it rejects every candidate.

That is a property of the pair and not of the test. The bevel's teeth meet the
ring at an angle, so they neither block evenly through the turn nor clear
evenly, and a criterion built on evenly spaced windows has nothing to find.

So bevel engagement is still decided by the older and weaker rule: among the
positions where the surfaces touch, the one with the most points in contact.
That count depends on which vertices were sampled — the Python samples at random
and `internal/bevel` by stride, and from the same 444 positions they choose
different winners for that reason alone. The two agree exactly on the 444, which
is the gap arithmetic; they do not agree on the answer, and neither has been
checked against a mechanism that was built.

## What the Python was, and what it verified before it went

The engine began in Python and was ported to Go module by module. The Python is
gone now; this is what it proved on the way out, because the proof is the
durable part and the code was not.

**The extractor was byte-identical.** Both implementations were run over the
whole shadow library and compared part by part: 2,649 parts and 21,675 ports,
the same in both. That is what settled the port of the parsing, which is the
half full of edge cases.

**The collision test matched a third implementation.** The Python used FCL; the
Go is Möller's triangle test under a bounding-volume hierarchy. They agree on
the reference numerically — same meshing windows, same spacing, same backlash,
same free fraction at 60 and 62 LDU — and the Go is about forty-five times
faster.

**Torque agreed exactly** across eighteen trains, ratios and efficiencies alike.

**The tooth phase agreed** gear by gear, including the 40t's poor sharpness
reading of 0.2630, which both implementations produce and neither invented.

**The bevel gap arithmetic agreed** on all 444 touching positions, though not on
which of them to pick — that choice depends on how the surfaces are sampled.

One capability went with it and is worth naming so it is not lost silently:
`holes.find_holes` inferred a part's holes from its geometry, by pushing a
cylinder along an axis and clustering where it passed through. That is for parts
the shadow library says nothing about. It was wired into nothing, and it is the
`--infer-holes` item in PLAN.md. Reimplementing it in Go means a ray-mesh
intersection, or a run of empty cells through the voxel grid that
internal/voxel already builds.

## What turns, and how far it spreads

An axle in a round hole spins inside it: that is a bearing, the part stays put,
and nothing is carried. An axle in a cross hole cannot spin, so the part is
keyed to it and goes round with it. 61408 — a thin liftarm with an axle hole
through the middle — is such a part, and the shadow library describes 161 more
with an axle-shaped section.

It does not stop at the keyed part. Whatever is pinned to it goes round too, and
about the same axis rather than about its own pin: a liftarm keyed to a shaft
sweeps its own length, and a beam pinned to the end of that liftarm sweeps a
wider circle about the same shaft. So the axis is inherited along the joints.

Two things bound that, and both matter more than they look.

**One pin is a hinge.** A part held by a single pin is carried round the shaft
*and* free to swing about the pin while it goes. No one axis says where it can
be, so it is not swept about one — it is reported as free to swing, which is a
fact about the mechanism rather than about clearance. Two pins settle it only if
they are in different places: pins strung along one line leave the part free to
spin about that line, which is how a long hinge is built.

**Two axes mean nothing turns.** A part reached from two different shafts, or
keyed to one and pinned to something that is not going anywhere, cannot turn at
all. That is reported rather than resolved by choosing an axis.

None of this fires on what the engine currently places: the inventory beams have
round holes only, and a bearing is required to be round because an axle in a
cross hole seizes. A test asserts that rather than trusting it. It matters for a
larger inventory, and for a model read back in from somewhere else.

It matters more for rigidity than for clearance. A part keyed to a turning shaft
is not a structural member, and counting its pin joints as if it were would call
a frame rigid when it is free to rotate — wrong in the direction that says yes.

## The published site had no gear geometry, and said everything was fine

Found by building the preview renderer, which is the first thing that made the
browser's idea of a model visible rather than merely reported.

The site ships tier 1. Tier is graded from a part's title: tier 1 is beams,
pins, axles, bushes and axle joiners; tier 2 is everything else titled
`Technic `. A gear is titled `Technic Gear 24 Tooth`, so **every gear graded
tier 2 and none of them shipped**. Two more never shipped at any tier: 3647 and
32270 have no shadow file, so they carry no ports, and the extractor drops
anything portless.

Twelve parts in all: every gear, both driving rings, both clutch gears.

What made it survive is that nothing failed. Three separate stages read
geometry, and all three treat its absence as nothing to do:

- the **clearance sweep** skipped every pair it could not measure and then
  reported "no two of the 9 parts share space" — a clear verdict on the parts
  that were left, phrased as a verdict on the model;
- the **tooth phase** stage had nothing to read the teeth from, so gears came
  out unphased and the emitted `.ldr` had teeth passing through teeth;
- the **renderer** drew the model without them.

The tests did not catch it because the fixture that publishes the two files for
the tests listed the parts again, by hand, and listed them correctly. So the
browser-versus-native comparison compared two things that were both right, while
the generator that builds the real files was wrong. A fixture that reimplements
the thing it is checking will agree with itself.

Three changes, and the third is the one that generalises:

1. `pipeline.Placeable()` is now the single list of what the engine can put in a
   model, and `assets.WithPlaceable` adds all of it to the catalogue whatever the
   tier says. Tier decides how common a part is; it never knew what this engine
   places, and the two were allowed to disagree.
2. The test fixture reads that same list instead of repeating it.
3. **Missing geometry is a finding.** The sweep names the parts it could not
   measure and withholds the clear verdict, because "checked and clear" and
   "not checked" had been coming out as the same sentence.

The measurement, retaken: tier 1 goes from 135 parts to 147, and `meshes.bin`
from 5.2 MB to 5.7 MB. Half a megabyte was the whole cost of shipping the parts
the engine actually uses.

## A brace that braces nothing, and the count that accepted it

The same render that found the missing gears found this. It is worth separating
from that one, because the missing gears were an absence and this is an answer
that was confidently wrong.

A reduction came out with three 13-hole beams marching 35 studs off the end of a
10-stud mechanism, each pinned to the last, none of them reaching the far
bearing. Every check said OK: connected, rigid, nothing sharing space.

Two mistakes, stacked.

**The search and the report disagreed about what was holding the frame.**
`rigidity.AnalyzeWith` counts the shafts — an axle through two bearings ties
them together, and leaving it out is why a structure can look like loose pieces
when the build would hold. `StiffenToRigid` called `FindJoints` without them. So
the search believed the two bearings were unconnected and braced until Grübler
was satisfied, while the axle they both carried had been holding them all along.

**Mobility is a count, and a count does not know which parts a joint is
between.** M = 3(n-1) - 2j falls by one for any beam pinned in two places. Two
holes of the same bearing are two places. So a beam bolted twice to one wall
scored exactly as well as one spanning the frame, and since `brace` preferred
the longest beam available, it took the longest beam available.

Fixed by giving the search the shafts, and by requiring a brace to reach two
parts that are not already rigid with respect to each other — parts joined by
two or more pins being one body for this purpose. Candidates are then scored by
overhang, the beam length left over past the gap it closes, rather than ordered
by length; length-first was standing in for "across the frame", which the
bridging test now says outright.

| example | parts before | after | structure volume before | after |
| --- | --- | --- | --- | --- |
| reduction | 9 | 6 | 387.7 | 23.5 |
| gearbox-2-speed | 21 | 16 | 128.8 | 42.5 |
| gearbox-3-speed-compound | 38 | 28 | 273.9 | 128.2 |
| subtractor | 4 | 3 | 125.1 | 23.5 |

Cubic studs.

### What it was hiding

Two examples now report that they hinge, and they do. A shaft passes through a
bearing, so the bearing's holes face along the shaft; every hole of a straight
liftarm faces the same way, so the liftarm lies across the shaft with all its
holes at one point along it; and a pin joins two holes only if they lie on one
line within two studs of each other. A liftarm therefore reaches one bearing
wall or the other and never both, whatever its length and wherever it is put.

So two walls on a shaft line cannot be tied together by anything in the
inventory, and they can counter-rotate about the shaft between them. Closing
that needs a part with holes on more than one axis, which `part.WorldHoles`
cannot express — it returns one axis for a whole part.

The warnings are the honest version of a verdict that was previously OK. M2 was
marked done on "every example comes out rigid, with no warnings at all", and
that is the measure this defect was defeating. PLAN.md M2 is reopened.

## A thirteen-hole beam declares one hole

The shadow library entry for 41239, Technic Beam 13, is two lines: one
`SNAP_INCL` at one end, and a crane-arm slot. It says nothing about the other
twelve holes.

That is not an omission. LDCad resolves snaps through the part's own subfile
tree, and 41239.dat places thirteen `beamhole.dat` primitives; `p/beamhole.dat`
in the shadow library is what says a beamhole is a hole. The part's shadow file
only adds the one hole that no primitive covers.

Reading a part's own shadow file alone therefore gives a thirteen-hole beam with
one hole in it, and that is what the extractor did. The structural search worked
around it without anyone deciding to: hole positions were synthesised from a
hole count on the assumption of a straight beam, and every part was given a
single hole axis by `RotationAxis`. Both assumptions hold exactly for straight
liftarms and for nothing else, which is why the inventory is straight liftarms.

`EntryForWith` walks the tree. At each subfile the shadow library describes, it
takes those snaps and stops — a shadow file for a primitive describes it
completely, and descending further counts the same hole again from its rims.
Ports are then deduplicated sign-free, because a hole has no direction and the
same hole is often reached twice.

The check that it is right is the one thing already known to be right: for every
beam in the inventory, the walked holes are exactly the positions the search has
been assuming all along, on exactly one axis. Not close — the same set.

| | before | after |
| --- | --- | --- |
| parts with port data | 2,649 | 2,810 |
| ports, whole library | 24,005 | 61,978 |
| ports, usable parts | 21,675 | 56,219 |
| `catalog.bin`, tier 1 | 15 KB | 23 KB |

It found the missing kind of part while it was at it. 6536, the axle-and-pin
connector perpendicular, comes out with a cross hole on one axis and a round
hole on another — the part that can tie two bearing walls together, which no
straight liftarm can. Its own shadow file declares one of the two.

The bug that hid inside this one is worth its own line, because it is the third
time: the shadow library enumerates bare ids, the parts library is a directory
of `.dat` files, and passing one where the other was wanted produces no error
and no ports — indistinguishable from a part with nothing to say. The fix is a
`filename` call, and the guard against a fourth time is that the extractor now
reports both totals, declared and walked, so a walk that quietly does nothing
shows up as two equal numbers.

## The part that turns a corner

Two bearing walls on a shaft line could not be tied together, and the reason was
not the search. It was that no part in the inventory could turn a corner, and no
part that could could be described.

Three things had to change.

**A part had to be able to have holes facing different ways.** `WorldHoles`
returned one axis for a whole part and laid the holes out from a hole count.
Both are exactly true of a straight liftarm and of nothing else. `WorldPorts`
returns the holes themselves, each with its own axis, and the joint finder
matches hole against hole rather than part against part.

**The holes had to be readable**, which is the subfile walk above: 6536's own
shadow file declares one of its two holes.

**A bearing had to be a bearing.** The first structure built with connectors in
the inventory laid one along a shaft, with two of its cross holes on the shaft
line. An axle seizes in a cross hole, so the shaft drove the connector round
rather than turning inside it — and the turning propagation said so, which is
how it was caught. `CandidatesFor` now rejects a placement where any cross hole
of the part lies on the shaft it is being asked to bear.

That last one is worth dwelling on, because it is the second time a rule that
read as being about a part turned out to be about a placement. The tripwire that
fired was `TestNoStructuralPartCanBeKeyedToATurningShaft`, which asserted that no
part in the inventory has a cross hole, with a note saying that adding one would
fail this and point at what else had to change. It did, and the answer was
almost nothing: the clearance sweep had already stopped deciding what turns from
what a part is called, so a keyed frame member is swept correctly rather than
tested standing still. The rule it was standing in for is that no part in the
frame may turn, and that is now asserted of the models the search produces.

| example | parts before | after | cubic studs before | after |
| --- | --- | --- | --- | --- |
| reduction | 9 | 6 | 387.7 | 23.5 |
| gearbox-2-speed | 21 | 16 | 128.8 | 36.0 |
| gearbox-3-speed-compound | 38 | 29 | 273.9 | 100.8 |
| subtractor | 4 | 4 | 125.1 | 27.9 |

Every example is rigid with no warnings, which is what M2 claimed when it was
first called done. The difference is that the claim now survives asking why.
