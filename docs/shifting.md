# Shifting

A gearbox needs three things beyond its ratios: something that locks a gear to
the output shaft, something that moves it, and something that decides when.
brickmesh models the first fully, the second partly, and the third as a stated
rule.

## What locks the gear

A driving ring, splined to the output shaft, slid until its dogs sit in a clutch
gear's recesses. The geometry is measured rather than assumed and is written up
in [findings.md](findings.md): faces meet at three half studs, and at exactly
that distance a 16-tooth clutch gear reads as sixteen free windows a clutch
tooth apart. That is the engagement.

The engine places the ring, picks the clutch variant where the library has one,
and reserves the shaft length the ring needs to slide clear.

## What moves it

Placed, but measured from official models rather than searched for.

The shadow library says what mates with what. A driving ring's bore pairs with a
ridged axle joiner (6538a, 6538b for the first system; 18948 for the 3L ring of
the second), and a catch mounts on a control axle through one of its holes with
a fork reaching into the ring's groove. Those are fits: the parts are meant to
be inside one another.

The joiner *is* placed, because where it goes follows from the ring and not from
the fit: a shaft carrying a ring is cut there, and an axle butts into each end
against the stop in the middle.

The catch was the harder case, and the reason is worth stating. The interference
sweep, which settles whether two gears mesh and whether a ring clears its gear,
cannot settle a fit. In LDraw everything is modelled at nominal size, so a
spline that grips reads as a spline that collides, and a fork that straddles a
groove reads as a fork that touches it. Measured:

- 34,992 placements put the changeover catch around the ring's center. Every one
  intersects it. The catch is 20 LDU across and the ring's groove is 26 in
  diameter, so it could not encircle it in any case.
- The ring on its own axle joiner is blocked at every rotation and every slide
  position, which is exactly what a spline should be.

So a sweep can confirm a catch's placement and can never find one. The numbers
come from LDraw's official models instead — 8448 for the first system, 42110,
42083 and 42056 for the second — and the sweep is used the way it can be used,
to check that the placement read off a model holds up:

| system | ring  | catch | out from the shaft | tip reaches |
| ------ | ----- | ----- | ------------------ | ----------- |
| first  | 6539  | 6641  | 60 LDU             | r 17.2      |
| second | 18947 | 35188 | 40 LDU             | r 12.4      |

Both tips land in the channel: inside the flanges, which reach r 20 and r 18,
and clear of the groove floor. Move either 5 LDU in and it is buried; move it
15 out and it no longer reaches. `clutch_test.go` re-derives all of that.

The two systems need different frames — the first's catch is an arm reaching
back along its own z, the second's a collar whose face is its own z — so each
records which of its own axes points along the shaft and which points out.
Getting the third axis's sign wrong there puts the part in mirrored, which
LDraw renders without complaint and no builder can assemble.

A catch is not tied to its own generation. 42110, 42083 and 42056 all put a 6641
against an 18947 at the same 60 LDU, and the sweep agrees. An earlier note in
`clutch.go` claimed it did not fit, on the strength of a sweep reading "clear"
at that distance — but clear is exactly what a working fork reads as, since it
straddles the channel instead of bottoming in it. 35188 is preferred for the
second system because 40 LDU of room is easier to find than 60, not because it
is the only thing that fits.

The groove itself is 10 LDU wide on both rings, floor at r 9.8 (6539) and r 8.7
(18947). That is exactly the thickness of a thin Technic liftarm, which is not a
coincidence: LEGO's own newer shift forks are made to that thickness so their
tines slot into the groove. A thin liftarm is therefore a dimensionally exact
fork, and a zero-clearance fit is precisely what no sweep can place — so the
engine does not place one. None of the official models available here uses one
as a catch; they use 6641 and 35188.

There is a third generation, with a compact 2-module ring and a fork driven by a
shifter drum, and its parts are all in the library: 2473a (the ring, 8 teeth),
4158 (the drum) and 4159 (the fork). Its groove is 10 LDU wide too, floor at
r 11.7, flanges at r 17.5.

What is missing is a model, not a part. Nothing available here places a 4159
beside a 2473a, and OMR's Technic coverage stops well short of the sets that
use it. So the third generation is measurable but unanchored, and it is not
placed. One .ldr with the two parts correctly positioned would close it.

Also in the library and not yet used: 51149, which is the same fork as 6641 on a
different axle hole. It returns the same verdicts at the same distances against
all three rings.

### A catch turns; it never travels with its ring

It was animated sliding along beside the ring, which is wrong, and the parts say
so. Every axle hole in both catches runs *across* the shaft, never along it:

| catch | hole | at | so it is |
| --- | --- | --- | --- |
| 6641, 51149 | across the shaft and the way out | its own z = -20, tip at -46 | a lever, arm 26 LDU |
| 35188 | along the shaft | its own z = -10 | a cam on a shaft-parallel axle |

Neither can be threaded onto a shaft-parallel axle and pushed. 35188's name says
as much on its own — "Changeover **Rotary** Catch".

The lever's angle follows rather than being chosen: its arm has to carry its tip
the ring's whole travel along the shaft, so it swings asin(travel/2*arm) either
side of square. The cam's does not — turning about an axis parallel to the shaft
cannot swing anything along it, so it must be a face cam, and one placement per
model fixes where a catch sits and not how far it goes. A quarter turn is a
guess, and every report that mentions it says "assumed".

The official models do at least bound it: the same catch appears with its ring
10 LDU either side of centre as well as level with it, which is a cam doing
20 LDU of work.

A group turns about its own centre, so a catch turning about an axle that is off
to one side of it needs its position to follow its orientation — the centre
orbits the pivot. That is the one other thing besides a sliding ring that may be
given a position at all.

What the sweep *can* settle is everything around the fit. A joiner is 20 LDU
across and a beam's hole is 12, so a joiner where a bearing is cannot be built
at all. Every ring and every joiner is turned a full revolution against the
structure before the model is written, and the search is kept out of the space
they sweep. Two provisos, both learned the hard way: the voxel grid marks every
cell a part so much as touches, so the reservation is eroded by a cell or a
bearing resting against the ring is thrown away; and the triangle test counts
two coplanar faces as intersecting, so parts whose boxes share less than an LDU
are touching rather than overlapping and are left alone.

### The axle the catch turns on

Placed, because it is determinate: the catch's own hole says where it is and
which way it runs, and the catch's placement is already measured. A cam's axle
comes out parallel to the shafts, so it runs the length of the gearbox and could
be borne by the same walls; a lever's crosses them and has nothing to run
alongside, so it gets a stud past the catch either way.

**Nothing holds it yet, and the run says so.** A frame today is two walls in the
plane of the shafts, and a catch sits a couple of studs off that plane, so the
walls do not reach its axle:

```text
WARN [bearings] 1 control axle(s) placed and 1 of them borne by nothing.
     The gears are held; the shift is not
```

That is worth knowing before building, and it is the requirement the structural
search does not yet take. Feeding it in — a control axle as another line the
frame has to bear — is what would close it, and would be the first thing to make
a frame more than two walls.

What is still not placed is the lever a hand turns and whatever joins two
control axles together. Those follow from the housing rather than from the
gears, so the run names them and says so.

## What decides

Two ways, and neither is the whole story.

**By hand.** The states are simply declared, and the animation walks through
them in order so the shift can be watched.

**By a rule.** `shift_points` names a shaft to watch and the speed at which each
gear gives way to the next, the way an automatic shifts on engine speed:

```json
"shift_points": {
  "watch": "input",
  "up_at":   [1.0, 1.6],
  "down_at": [0.45, 0.8]
}
```

The run reports the schedule and checks that it holds together. The check worth
having is whether the box hunts. Changing up drops the watched shaft by the step
between the two ratios; if it drops past the speed the box changes back down at,
it changes down at once and then straight back up, and sits banging between two
gears. That needs both sets of points — upshift points alone cannot describe it,
because changing up always leaves the shaft below the speed it changed up at.

Given only upshift points, the run says how low each downshift point would have
to be, which is the number a builder actually needs.

## And the rest

These two are where brickmesh starts, not the extent of what people do. Builders
have shifted gearboxes with centrifugal governors that throw weights out as the
engine picks up, with torque reaction against a sprung housing, with ratchets and
sequential drums that advance one gear per pull, with pneumatic cylinders and
valves, with gravity and tilt, and with motors under electronic control. Some of
those are decisions a mechanism makes; some are decisions made elsewhere and
merely carried out.

What they share is the part brickmesh does model: whatever decides, the shift
itself is a ring sliding into a clutch gear, and the ratios and shift points
either settle or they hunt.
