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
|---|---|---|
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
