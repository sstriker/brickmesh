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

## The joints were counted and never built

Every check the engine ran counted joints. None of them placed one. So a model
came out as beams and connectors lying against each other with nothing through
them — the frame of a two-speed gearbox was four parts and no fasteners — and it
read as correct in every report, because every report was about the joints
rather than about the pins.

Pins are real parts. They cost something, they fill the holes they pass
through, and leaving them out understates the part count and overstates what is
left free for anything else to use.

One pin per run of joints along a hole line: three parts stacked at one hole are
two joints and one pin through all three, while two pairs on the same line a
long way apart are two pins. Merging by line alone would have dropped the second
of those, which is a joint the rigidity count is leaning on with nothing in it —
so runs are merged only where their spans touch. The pin sits midway along the
run, and a run deeper than two parts gets the three-stud pin.

Lines a shaft already runs down are skipped. The axle is the fastener there, and
a pin in the same hole would be two things in one place.

### Two things it turned up

**A connection was being counted twice.** Two parts with coincident holes on a
shaft are found by both halves of the joint finder: once as a pin through those
holes, once as two parts threaded on the same axle. It is one connection. The
test on mobility is `M <= 0`, so over-counting is the direction that calls a
hinge rigid — the three-speed compound box read `M = -15` and is really `M = 0`.
Still rigid, but exactly, not comfortably.

**The two port sources order a part's holes differently**, and the search breaks
ties on that order, so the browser and the command line can pick different — and
equally valid — frames. It surfaced because pins made the difference visible in
the comparison. The model comparison already tolerated frame ties flipping and
now counts pins as frame; the mechanism still has to match numerically, and the
frame still has to pass its own checks.

### And the pin was lying across the holes

The first pins went in at exactly the right points and fastened nothing.

A pin's direction was read off its ports, on the reasoning that a pin is one
cylinder and every port on it points along it. That is true of 3673, which says
X twice. It is not true of 2780, which declares its pin by including 3673 whole
and then, once its own subfiles are followed, turns up a third port facing Y
from a piece of the friction slot. Which of the two came first was an accident
of sorting.

It looked right in the file. The joint it was first tested on happened to run
along Y, so the wrong axis and the right one agreed, and the emitted line was an
identity rotation that read as obviously fine.

The shape settles it: a pin is long in one direction and short in the other two.
The test now asks that a pin's longest dimension lie along the joint it fills,
not merely that a pin sits at the right point, and it fails on the old code —
four joints in the subtractor, each with a pin across it.

## Counting the work, because timing it does not survive CI

A change that made the whole suite three to four times slower was committed and
noticed only because a test timed out. Nothing failed. The work it added was
real and repeated — a part's subfile tree walked again on every lookup, inside
the search's inner loop — and it would have been every bit as invisible had it
been half as bad.

The first instinct was to count file reads, since the regression was I/O. That
would not have caught it. **The read count did not move at all**: the parts
library caches the files it has already read, so re-walking the tree re-did the
traversal and the allocation and touched no disk. Only the time changed.

So the counter has to sit on the operation that regressed, not on the resource
it looks like it consumes. `Ports.Walks` counts answers actually worked out
rather than remembered, and one per distinct part is the entire budget.

| example | walks | with the cache removed |
| --- | --- | --- |
| reduction | 13 | 735 |
| subtractor | 13 | 3,964 |
| gearbox-2-speed | 23 | 5,064 |
| gearbox-3-speed-compound | 27 | 6,969 |

Counted rather than timed on purpose. A shared CI runner varies by more than a
real regression often does, so a wall-clock threshold loose enough not to flake
is too loose to catch anything — the 3.5× slowdown above would sail through any
honest timing gate. Counts are the same on every machine, so the budget can sit
close to the truth: these are set at two or three times the real figure, and the
regression they were written for exceeds them by a hundredfold.

There is still a wall-clock ceiling, at ninety seconds against a real cost near
one, for a runaway that no counter here is watching. And the published files
have a size limit in the Pages workflow, since the download is the one cost a
visitor cannot opt out of: 6.1 MB today against a 12 MB limit, 1.9 MB gzipped,
which is what is actually served.

A new example with no budget fails the gate rather than being skipped. That is
deliberate — the alternative is a gate that quietly stops covering the thing it
was added for.

## A group's placement is its centre, not the model's origin

The first animation LDCad ever ran came out with the parts scattered — every
shaft orbiting instead of spinning, including the one on the origin.

The scripting reference settles it, and says the opposite of what this engine
assumed. `setPos` "applies to the groups center position not the main item's
true position"; `getPos` "returns the position of the linked LDCad group current
center point". Contents are held relative to the centre, and the placement says
where that centre goes and how it is turned. So `setOri` turns a group about its
own centre already, and a spinning shaft needs nothing but a rotation.

The engine believed placement was applied about the model origin, and so added
`t = q − R·q` to drag the pivot back onto each shaft. A correct correction to a
problem that was not there. It displaced every group by up to twice its distance
from the origin, and by a different amount per group, since each turns at its
own rate — which is why the parts did not merely shift but came apart.

Where the wrong belief came from is worth writing down, because it was a careful
misreading rather than a guess. LDCad's own examples say:

> This group has a main item with identity placement so we can apply the
> rotation around y absolutely.

"Absolutely" there means *not incrementally* — set the orientation outright
rather than adding to it. It was read as *about the model origin*. Both readings
explain the sentence; only one is true, and the example that would have
distinguished them is the one where the group is not at the origin, which the
examples do not show for rotation.

### The part that should have caught it

`tests/test_animation_lua.py` executes the emitted Lua and asserts that a point
on a shaft's own axis does not move. It passed throughout.

It passed because the stub modelled LDCad the same way the generator did. Two
wrongs that agree are invisible to a test that only compares them to each other.
The stub now models the documented behaviour, and there is a control —
`test_the_pivot_correction_would_now_be_caught` — that hands a group the old
offset and requires the axis to move. If that ever stops failing, the stub has
drifted back and the rest of the file means nothing.

The general shape of this has now happened three times: a fixture that
reimplements the thing it checks agrees with itself. The gears missing from the
browser, the frame that braced nothing, and this. What broke the tie each time
was output from something outside the loop — a renderer, a picture, LDCad.

### And setOri replaces, it does not add

The pivot was only half of it. With the offset removed the parts still came
apart, differently.

`setOri` is absolute: it *replaces* a group's orientation rather than turning it
by that much. Every group here holds a gear on a shaft, so every group starts
already turned — and handing one a bare rotation snapped it to that orientation
instead of rotating it from where it was. A group whose parts happened to sit
square to the model would have looked fine, which is why every example in
LDCad's own set does exactly that.

The fix is the idiom those examples use: buffer what each group starts with, and
multiply the turn onto it.

```lua
ori = grp:getOri()             -- once, in onStart
local m = ori:clone()          -- every frame
m:mulRotateBA(angle, x, y, z)  -- self = rotation * self
grp:setOri(m)
```

And the comment that had already been misread once turns out to be a warning
about precisely this:

> This group has a main item with identity placement so we can apply the
> rotation around y absolutely.

Identity *placement*. It is saying: this works only because the group starts
square. Read once as "about the model origin", it is really "and yours will not
start square, so buffer it".

The stub could not have caught this either, for the same reason as before: it
gave every group an identity starting orientation, which is the one case where
replacing and turning-by agree. It now starts them turned, has real 3×3
matrices rather than an angle and an axis, and carries two controls — one that
hands a group the old pivot offset, one that hands it a bare rotation — each
required to move a point that should be still. If either stops failing, the
file has drifted back to agreeing with whatever the generator does.

### And it was the centre being relative, which one run settled

Neither of the two readings above was the cause. Both were changes to how the
turn was expressed; the fault was in the model file, one line up.

The diagnostic asked LDCad what a group's placement actually is, and got:

```
rest   pos 140.00  0.00 -80.00   ori 1 0 0 | 0 1 0 | 0 0 1
```

That group's parts sit on a shaft at z=-40, and it was declared
`[center=0 0 -40]` — the point on the shaft. LDCad reported its centre at
z=-80, because a group's centre is read **relative to the group's main item**.
The main item is an axle at (140, 0, -40); add the declared -40 and you get -80.

So every group had been turning about a point 40 LDU off its own shaft. The
files now write `[center=0 0 0]`, which puts the centre on the main item's own
origin — and every part in one of these groups sits on the shaft it turns about,
so that origin is a point on the axis. That is not merely safe, it is the right
answer.

LDCad's meta reference does say so, in four words: "Relative center to use for
this group". It was read as a model coordinate.

Two other things the same run settled, both of which had been guessed at:
a group's orientation starts as the identity, and `mulRotateBA`, `mulRotateAB`
and a bare `setRotate` therefore all agreed. So the orientation change was
harmless and was not the cause.

### The lesson, which cost three rounds

Every one of these was inferred from prose that admitted two readings, and each
wrong reading is invisible for a group sitting square at the origin — which is
what every example in LDCad's own set happens to be. The engine's own tests
could not break the tie either, because the stub was built from the same reading
as the generator.

What settled it in two minutes was asking the software what it thought, in
numbers. That option existed from the first round. Reach for it sooner: when a
contract is ambiguous and the other side is running on the same machine, measure
it rather than read harder.

### mulRotateAB and mulRotateBA are the other way round

The centre fix left the axles right and every gear tumbling. The report that
came back — "the axles are now correct, but some parts are spinning on the wrong
axis" — is what made this quick, because the model says which groups are which:
`shaft_input` and `shaft_output` end in an axle placed square to the model, and
those two behaved. `shaft_low`, `shaft_high`, `ring_1` and `ring_2` hold only a
gear or a ring, placed turned, and those tumbled.

So the run measured what a quarter turn about x does to the 20t gear, five ways.
Its axis is its own local z, which its placement sends to world x, so a correct
turn leaves the third column of its orientation at (1, 0, 0):

| call | where the gear's axis ends up | |
| --- | --- | --- |
| `mulRotateBA`, world x | (0.00, 0.16, −0.99) | wrong — what was shipped |
| `mulRotateAB`, world x | (1.00, 0.00, 0.00) | **right** |
| bare `setRotate` | (0.00, −1.00, 0.00) | wrong |
| `mulRotateAB`, local z | (0.00, 1.00, 0.00) | wrong |
| `mulRotateBA`, local z | (1.00, 0.00, 0.00) | right, same matrix as AB |

The reference says `mulRotateAB` is `self=self*rotate` and `mulRotateBA` is
`self=rotate*self`, which makes BA the one that applies a rotation in the
model's frame. In effect it is the other way round. There is no attempt here to
explain the naming; the measurement decides.

And it is invisible unless a group is placed turned, because for a square one
the two orders coincide. Every group in LDCad's own examples is square, and so
was every group in this engine's tests until the stub was given a starting
orientation.

That was the last of it. Four wrong readings of the same page — the centre, the
frame the placement is in, whether setOri adds or replaces, and now the
multiplication order — and every one of them is invisible for a group sitting
square at the origin.

## A differential that was only ever arithmetic

Opening the subtractor in LDCad showed one moving axle. That was not an
animation fault: **the differential was never placed**.

The functional layer has modelled differentials from the start — the case runs
at the average of the outputs, the degrees of freedom come out right, the
subtractor's whole point is checked. `internal/bevel` even measures the 28-tooth
ring inside 62821 to find where a 12t double bevel engages it. And nothing ever
put the part into a model. A subtractor came out as a single axle inside a
frame, and every check passed, because every check was about the kinematics.

It is placed now, and the two outputs get an axle each, butting against the
housing rather than running through it — which is the whole of what the part
does. Three groups where there was one.

Two things this turned up:

**The sign of the face an axle butts against.** Getting it backwards ran a
four-stud axle straight through the middle of the housing, and the clearance
sweep let it past — because an axle may be inside anything, which is true and is
exactly the exception that hides this. There is a test on the spans now rather
than a reliance on the sweep.

**The browser gate caught its own author.** Adding 62821 to the model without
adding it to `pipeline.Placeable` failed `TestNoExampleUsesAPartTheBrowserCannotSee`
immediately: "no geometry for 62821.dat, so nothing was checked against it".
That gate was written three days of work ago for exactly this and it worked on
the first thing it was pointed at.

### And group ids have to differ between models

The 3-speed and the reduction both died with "Active group link needed" on their
first `getOri`, while the 2-speed ran. LDCad's GID is a globally unique group id
and it holds it to that; the id was derived from the group's name alone, so
every model this engine wrote called its input shaft `shaft_input` and handed
LDCad the same id. Two models open at once, and the second one's groups link to
nothing.

The subtractor escaped only because its one group is called `shaft_case`, and
the 2-speed worked because it got there first. Opening a reduction beside a
gearbox is not a corner case; it is what looking at a set of examples means.

## A ring in neutral drives nothing

Reported by someone watching the model: during a shift the axle carrying the
driving ring kept turning while the ring was between gears, engaged with
neither. The gears on that axle should turn — the input still reaches them
through their mesh — and the axle itself should not, because nothing is driving
it.

Every named state does engage something, so this was only ever visible during
the `shift` animation, in the quarter of each segment where the ring is sliding.
The walk accrued angle at the outgoing ratio right through it, which draws a
drive that is not there.

Shafts driven only through a ring are now marked, and hold still while their
ring is in neither gear. It changes the arithmetic: over a three-state walk the
output turns three quarters of what the ratios alone would give, because for the
other quarter it is not connected to anything.

### And it is a question about the whole graph

Holding the shaft a ring rides was not enough, because the rule does not stop
there. Stated properly, and it has to hold both ways round:

> A gear that turns turns whatever it meshes with. A shaft keyed to a turning
> gear turns, whether it is keyed through an axle hole or through an engaged
> driving ring. And anything nothing reaches does not turn at all.

The first two hold by construction — the solver's mesh equations make it
impossible for two meshed gears to disagree, and a gear placed on a shaft is
keyed to it. The third is the one that was wrong, and marking the ring's own
shaft only covered the first hop of it.

In a compound gearbox the second stage's gears are driven by the first stage's
output, so when that output stops they stop too. They were still turning.
`alwaysDriven` walks the graph from the inputs using only the links a shift
cannot interrupt, and everything it does not reach holds:

| | keeps turning | holds while a ring slides |
| --- | --- | --- |
| gearbox-2-speed | input, low, high | output |
| gearbox-3-speed-compound | input, s1low, s1high | mid, output, s2low, s2high |

A differential is the exception worth spelling out: two of its three shafts
determine the third, and one determines nothing, since the other two are free to
turn against each other. Driving the case alone leaves both outputs
undetermined, which is the whole of what the part is for — so the walk requires
two before it propagates.

## Two shifts, one ring

A driving ring has dogs on both faces, so one sitting between two clutch gears
engages either by sliding. The engine placed one per shift — two rings back to
back, which no builder would do — and said so in its own report every time it
ran, while going ahead and doing it anyway.

What made this more than bookkeeping is that the hardware generation was chosen
per gear. The 20t exists only in the second system; the 16t exists in both, and
taking the first that fits gave it the first system's part. Two gears that could
have shared a ring were therefore in two generations, and a ring of one does not
grip the other's gears — so the merge could never fire until the choice moved up
to the pair. `clutch.ForBoth` picks a generation that serves both.

| | parts before | after |
| --- | --- | --- |
| gearbox-2-speed | 17 | 14 |
| gearbox-3-speed-compound | 31 | 25 |

A shared ring has three positions rather than two: engaged with one gear,
neutral, engaged with the other. That needed no change to the animation format —
the neutral is simply the midpoint of a travel whose two ends are both
engagements, so the same interpolation carries it.

## A differential housing with nothing in it

Three — no, five — bevel gears go inside a modern Technic differential, and the
model showed the bare housing. It was placing 62821, which is the housing alone.

The first answer was to name the missing gears in the report rather than place
them. The reasoning was measured: LDraw's 62821 models its outside and its axle
bore and not the chamber the gears occupy, its innermost surface at mid-length
is 10 LDU from the axis, and a bevel is wider than that. Sweeping one along the
inside reads TOO DEEP at every position in both facings. A gear put where it
belongs would fail the clearance check for being correct — the same shape as a
driving ring's splines reading as a collision rather than a grip.

Every one of those measurements is right and the conclusion was wrong.

What prompted a second look was someone saying they were surprised the correct
parts and measurements did not exist. They do. The library has
**65414c01**, a shortcut: `65414`, whose own title is "Differential Casing for 5
Internal Gears", with `65413` for the drive gear and five `6589` bevels placed
inside it — two side gears on the outputs at z = ±17, and three planets at 120°
around the middle. Three studs long on its own Z, ports at either end, the same
convention as the bare housing. A drop-in.

So the differential is one part with its gears in it, there is nothing to add by
hand, and no clearance exception is needed at all — which is the outcome the
exception would have been a workaround for.

Two lessons, and the second is the one worth keeping.

**The count was wrong too.** A web search said three bevels, and the part itself
says five. The library was the better source and it was on disk the whole time.

**A careful measurement can make a wrong conclusion look settled.** Sweeping the
gear and finding it fits nowhere was real evidence, and it answered "does this
gear fit inside this part" when the question was "how is a differential built".
Asking the narrower question well is not the same as asking the right one, and
the tell was that the answer implied something absurd — that a standard
mechanism could not be represented in a library that represents everything else.

### All four differentials, and why interpenetration is not the test

The library has four usable differentials: 6573 and 73071 at four studs long,
62821 and 65414 at three, and only 65414 comes assembled. Every one of them
reports the same thing when a bevel is placed at the satellite positions — no
room anywhere.

Including 65414 itself, at the positions its own official shortcut uses.

So LDraw's assembled differential has its five satellites intersecting the
casing, and ships that way. A triangle-intersection test is simply the wrong
instrument for parts designed to nest: the same reason a driving ring's splines
read as a collision rather than a grip, and an axle in a bore, and a pin in a
hole. Those already have exceptions; a differential's internals are the same
shape of thing.

Which means the earlier conclusion was wrong twice over. Not only does the
assembled part exist, but the measurement that seemed to rule out placing gears
by hand rules out the official assembly too — and that should have been the tell.

## Rigidity was advice arriving after the decision

The structural search returns solutions smallest first, and there are usually
many of them. The pipeline took the first that fitted together, braced it, and
then — if it still folded — said so in the report.

That is a check standing where a constraint belongs. Nothing acted on it; the
reader was handed a frame and told it hinges.

It is now part of choosing: a candidate has to fit together *and* stay rigid
once braced, and one that folds is passed over for one that does not. The
fallback matters as much as the constraint — if every candidate hinges, the
first that at least fits is still emitted and the rigidity check still reports
it, because a model that hinges is worth looking at and refusing to produce one
would hide the very thing the report exists to show.

## Brackets, not walls, and the metric that kept them that way

Asked whether the frames distribute load, the answer was no, and the reason was
one line in the bearing requirements: every shaft was asked for a bearing at
either end of *its own* free stretch. Those points almost never line up between
shafts, so nothing could bear two shafts at once, and the search returned the
least that holds — five parts for a two-speed gearbox, each holding one thing.

Bearing planes are chosen across shafts now: the cross sections where the most
shafts are simultaneously free, the two furthest apart of those. A shaft that
cannot reach either falls back to its own extremes.

| | frame parts before | after | cubic studs before | after |
| --- | --- | --- | --- | --- |
| gearbox-2-speed | 5 | 2 | 35.6 | 10.4 |
| gearbox-3-speed-compound | 6 | 2 | 85.7 | 35.3 |

Two walls with every shaft through both, which is what a gearbox is.

### Counting parts was the wrong measure

A pin counted the same as a thirteen-hole beam, though one is a fastener and the
other is most of the frame. And fewest-parts pushes against compactness — a
smaller structure often takes more parts, so the two goals were fighting and
the count was winning.

Cost is now a weighted sum with the terms named: per stud of beam, per part, per
cubic stud of envelope. The defaults charge a stud of beam and a cubic stud of
envelope alike and a bare part a fifth as much, so a fastener cannot outvote
structure. A test pins that those weights stay in the same range, since one term
an order of magnitude larger than the others decides every ranking on its own.

Separately from the cost there is a bound. `--max-x/-y/-z` cap the envelope in
studs, and a bound is not a preference: a frame outside it is not a candidate at
all. Asking for a two-speed gearbox inside two studs of depth reports that none
was found and names the cap, rather than quietly returning the best violation.

## Where the force between two gears goes

Meshing gears push their shafts apart, along the line of centres, in proportion
to the torque. It is the load that decides whether a gearbox holds its mesh or
spreads and skips, and nothing here had ever asked about it. A frame can hold
together and refuse to fold while still letting two shafts drift apart — both of
the structural checks pass on brackets that do exactly that.

What is checked is the path, not the magnitude. Whether a frame survives a given
torque needs numbers this engine does not have; `internal/torque` carries
failure limits and says outright that they are unverified estimates. Whether the
force has somewhere to go is geometry.

The measure is how many joints it crosses. Both shafts of a pair borne by one
part is the whole point of a wall: the load is taken inside the beam, between
two holes, and no pin sees it. Every joint after that is a pin in shear and a
little more give. A pair whose shafts share no frame at all is a failure, not a
degree — the force has nowhere to go and the gears will skip.

Every pair in every example is borne by one part, which is the wall change made
concrete rather than asserted. It is also the regression guard on it: go back to
a bearing per shaft end and the pairs stop sharing a part, and this says so.

## The page, in a browser

Everything about the page was checked without one: the module's answers against
the engine's, the worker's messages, the camera's arithmetic, the triangle
buffer's shape. None of that can link a shader. A vertex attribute that does not
exist, a varying spelled two ways, a stride off by one — all of them compile in
Go, pass every buffer check, and hand a person a blank canvas.

So a real headless browser serves the page over http, builds a model, reads the
pixels back and clicks a finding to watch them change.

It found two things on its first run, and only one of them was mine to expect.

**The shaders do link**, which was the open question — `vFlagged` is fine.

**Two of the four example buttons pointed at files that do not exist.** The page
offered "3-speed gearbox" and "auto-shifting" as `gearbox-3-speed.json` and
`gearbox-3-speed-auto.json`; the repository has `gearbox-3-speed-compound.json`
and `gearbox-2-speed-auto.json`. Clicking either fetched a 404 and put the
message in the status line. Nothing here had ever clicked one. That is now two
tests: the browser catches it, and a cheap one greps the page for the paths it
offers and checks each exists, so it fails in a second rather than in a minute.

A note on the harness, since it made the same class of mistake it exists to
catch. The first version read the canvas back after the frame had been
composited, without `preserveDrawingBuffer`, and got a blank image — reporting
that the page drew nothing when the page was fine. Pixels have to be read in the
same task that draws them. Its first version also built a second WebGL context
on the page's own canvas to test the shaders, which would have answered a
question about the harness rather than about the page.

## The selector cannot be placed by measuring, and now that is known rather than assumed

The report has always said the shift linkage is named rather than placed, and
given a reason: the catch's hold on a ring is a fit, and in LDraw a spline that
grips reads as a spline that collides. That was an assertion. It is now a
measurement.

The rings' grooves are real and findable: 6539 is 20 LDU in radius except a band
at z ∈ [-4, +4] where it drops to 13, and 18947 has the same 8 LDU band at 12.
So there is a place for a catch to sit and it can be found from the part.

Where the catch goes cannot. Sweeping 6641 against 6539 through every one of the
24 lattice orientations, four radial directions, eight distances and three
positions along the shaft gives **no placement that is both clear and holding** —
every position that lets the ring turn a full revolution has the catch outside
the ring altogether, and every position that reaches the groove collides at some
angle.

That is not a fault in the sweep. It is the same thing as the driving ring's
dogs in a clutch gear, the axle in a bore, the pin in a hole, and the
differential's five satellites in their casing: LDraw models nominal surfaces
with no clearance, so two parts designed to nest intersect.

The difference from those, and the reason this one stays unplaced, is that they
had another source. The ring's engaged distance was found because the sweep
could see *windows* — free angular positions that a clutch gear has and a plain
gear does not — and the differential had an assembled part in the library. A
catch on a ring offers neither: no angular signature to read, and no shortcut
that puts the two together.

So it needs a reference build, not a cleverer sweep. The report says what to add
and why it is not there, which is the same answer as before with evidence under
it instead of a claim.

## The other kind of clutch

Two different parts share the name and nothing else. A driving ring's clutch
gear has dogs: it either grips or it does not, and which of the two is a
question about position. The 24-tooth clutch gear has a friction centre that
gives way above a force, so nothing downstream of it can be loaded harder than
that however hard the input is driven.

`internal/clutch` has excluded 24 from the shiftable counts since the driving
rings were measured, with a paragraph explaining why: swept against both rings
the 24t clutch gears stay solid at every distance and every angle, exactly as a
plain gear does, because there are no dogs anywhere on them. That paragraph was
the only thing in the engine that knew this part existed.

Now it is placed. `"slip_clutches": [{"shaft": "output"}]` puts 76019 at the
24-tooth station on that shaft in place of the plain gear, and the report says
what it protects and at what torque.

Two details worth keeping.

**It is refused where it cannot go.** 24 teeth is the only size the part is made
in, so a slip clutch on a shaft carrying an 8t is not a thing that can be built.
Placing a plain gear there and saying nothing would leave a model that looks
protected and is not.

**The torque comes with its provenance.** 20 Ncm, community figure, unverified,
and the report says so in the same sentence as the number —
`internal/torque` has carried its limits that way from the start on the grounds
that a limit without provenance is a number someone will eventually believe.

## The selector, from an official model

The previous entry said the catch could not be placed by measuring and needed a
reference build. Both halves turned out to be right, and the reference was
findable.

LDraw's Official Model Repository has 8448 Super Street Sensation, and its file
contains one 6641 and three 6539s. Walking the model tree and composing the
transforms puts them, in world coordinates:

| | |
| --- | --- |
| shafts | along Z at x = 0, ±40 |
| clutch gears | z = ±30 on each shaft |
| driving rings | z = 0, midway between gears 60 LDU apart |
| changeover catch | 60 LDU out from the shaft, level with the ring |

Three things fall out of that.

**The catch sits 60 LDU out.** My own search had stopped at 40, which is why it
found nothing. Told where to look, the sweep confirms it: at 60 the catch
reaches the groove and the ring turns a full revolution; at 55 it is buried and
at 65 it has let go. So the sweep could confirm a placement it could not find —
worth knowing about that instrument.

**The engaged distance is confirmed from a second direction.** The rings sit
exactly 30 LDU from the clutch gears either side of them, and 30 LDU is the 3.0
half studs measured from interference windows. Two independent methods, same
number.

**The rings sit midway between two clutch gears 60 LDU apart**, which is the
shared-ring arrangement — one ring, two gears, engaging either by sliding. That
was implemented from the shape of the part rather than from a model, and here is
a set built the same way.

Only the first system was settled at this point. 6641 read as clear of an 18947
at 60, which was taken to mean it would sit there holding nothing; the parts
that move the newer rings were in the library but no model here had one beside a
ring. `clutch.System.Catch` was empty for that generation and the report named
the hardware instead of placing it. The next section is how that was closed, and
how the reading above turned out to be backwards.

The model was read, not redistributed: what is kept is a measurement, and 8448's
LDraw file is by its OMR authors under CCAL 2.0.

## The second generation, and a null reading mistaken for a negative

The same method, on the same repository. 42110, 42083 and 42056 all have 18947
driving rings; 42110 and 42083 have 35188 rotary catches beside them.

One wrinkle in the walk: OMR embeds some parts as renamed subfiles, so the
catch appears as `42110 - 35188.dat` and an exact-name match misses it entirely.
Matching on the suffix finds it.

| | |
| --- | --- |
| ring | 18947, axis along the shaft |
| catch | 35188, 40 LDU out on a perpendicular, level with the ring |
| frame | the catch's own z along the shaft, its own x pointing out |

Confirmed by sweeping: at 40 the catch reaches into the groove and the ring
turns a full revolution, at 35 it is buried, and by 55 it no longer reaches.
42083 has two catches whose nearest ring is ambiguous and it does not matter —
35188 measures ±27.60 in both x and y, so it is symmetric across the axis the
ambiguity is about and both readings are the same placement.

The two generations need different frames. 6641 is an arm, z from −46 to +6,
reaching back along its own z; 35188 is a collar whose face is its own z. So
each system records which of its own axes points along the shaft and which
points out, and the third is whatever makes the result a rotation rather than a
reflection. Getting that sign wrong puts the part in mirrored, which LDraw
renders without complaint and no builder can assemble.

### The claim that 6641 does not fit an 18947 was wrong

It fits. 42110, 42083 and 42056 all put one against an 18947, at the same 60 LDU
and in the same frame it uses on a 6539.

The error was reading the instrument backwards. The sweep said "clear" at 60,
and clear was taken as evidence of nothing being held. But clear is exactly what
a working fork reads as — it straddles the channel instead of bottoming in it.
The tip comes to r 17.2, inside the flanges at r 18 and well outside the groove
floor at r 8.7. A verdict of NO ENGAGEMENT was the right answer to "can the ring
still turn", and it was read as an answer to "is the catch holding anything",
which the sweep was never asked.

**A null reading is not a negative result.** This is the differential lesson
again in a different costume: there the tell was a conclusion that implied
something absurd, here it is an instrument reporting the absence of the thing it
was told to look for and that absence being promoted to a finding. Both times
the measurement was correct and the question behind it was not.

35188 is still what gets placed for the second system, because 40 LDU of room is
easier to find beside a shaft than 60 — not because it is the only thing that
fits.

### A thin liftarm is an exact fit, which is why it cannot be placed

The groove measures 10 LDU wide on both rings, floor at r 9.8 on a 6539 and r
8.7 on an 18947. Ten LDU is exactly the thickness of a thin Technic liftarm, and
that is not a coincidence: LEGO's own newer shift forks are built to that
thickness so their tines slot into the groove.

Which makes a thin liftarm a dimensionally perfect fork and an unplaceable one.
Ten into ten is zero clearance, so the sweep reads TOO DEEP at every distance
where it reaches — the nominal-surface problem at its purest. None of the four
official models here uses one as a catch either; they use 6641 and 35188. So it
is not placed, on the same standard that got the other two placed.

There is a third generation — a compact two-module ring with a fork driven by a
shifter drum. Not anchored: the design IDs quoted for those parts in the
literature land on unrelated elements in LDraw's own numbering, and no model
here has them. Named rather than guessed at.
