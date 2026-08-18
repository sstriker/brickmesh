---
name: brickmesh
description: Design, validate and build LEGO Technic mechanisms — gear trains, reductions, differentials, subtractors — producing an .ldr file that opens in Stud.io. Use when asked to build, design, check or lay out a Technic mechanism, gearbox, gear train or drivetrain, or when asked whether a set of tooth counts or a gear arrangement will actually work.
---

# brickmesh

Work out a Technic mechanism before it gets built: whether it can turn at all,
whether the gears land on the lattice, where the shafts go, and what holds them.
The output is LDraw, which Stud.io opens directly.

The engine validates and realizes. It does not invent mechanisms — that part is
yours. Compose the gear train, then let the engine tell you it is wrong.

## Ask first, when it changes the answer

Ask these up front, in one go, before building anything. Skip any the request
already settles, and do not ask about things that do not change the outcome.

- **What drives it, and how hard.** A motor choice sets the input torque, which
  decides whether an 8t gear survives the first stage. `torque.py`'s limits are
  unverified community figures — say so if torque is load-bearing to the answer.
- **Studless or studded.** Liftarms sit on a 20 LDU lattice, Technic bricks at
  24 LDU vertically. A transmission spanning both cannot line up, and the
  engine reports it as a `grid` failure. Studless is the safe default.
- **Ratio, or ratios.** "A reduction" is not a specification. Ask for the ratio
  or the input and output speeds. For multi-speed, ask for each ratio.
- **Envelope.** Is there a size or shape it has to fit inside? The search
  prefers compact but does not honour a hard limit yet.
- **Inventory.** Only parts they own, or anything? Tier 1 is common Technic,
  tier 2 all Technic, tier 3 the whole library.

Do not ask about tooth counts. Choosing them is the work.

## What it cannot do yet

Say this plainly rather than producing something that looks close.

- **The shift linkage is not placed.** The driving rings are, and the clutch
  gear they lock to where the library has one — but nothing that *moves* a
  ring: no changeover catch, no shift gate, and not even the ridged axle joiner
  the ring is splined to. This is a limit of what can be checked rather than an
  oversight. Those are fits, parts meant to be inside one another, and the
  interference sweep that settles whether gears mesh cannot settle a fit: in
  LDraw everything is nominal size, so a spline that grips reads as a spline
  that collides. Measured both ways, in `docs/shifting.md`. Tell the builder
  which parts to add; the numbers are below.
- **A clutch gear only in 16t.** Careful with the word, it means two things.
  The engage/disengage kind is placed: a shifted 16t station comes out as 6542a
  rather than the plain 4019. There is no 20t or 24t equivalent in the library —
  a real one reaches its gear through a driving ring extension (32187, 35186),
  which is not modelled, and the run warns when a shift lands on one. The other
  meaning, the old white 24t torque limiter that slips above a force, is not
  modelled at all: there is no slip element anywhere.
- **The structure does not always hold together.** Most runs come out rigid now,
  but a `rigidity` warning still turns up — the subtractor example hinges. The
  gears are right; the frame may need a joint adding.
- **Only two ways to decide a shift.** By hand, or on the speed of a watched
  shaft. Both are described in `docs/shifting.md`, along with the many ways
  builders actually do it — governors, torque reaction, ratchets and sequential
  drums, pneumatics, gravity, motors — none of which are modelled. If someone
  asks for one of those, build the box and say the trigger is theirs to design.
- **Bevel engagement is unresolved.** See the open question in `PLAN.md`. Avoid
  bevel pairs where a spur pair will do, and flag it when one is unavoidable.

## Shifting on its own

A box can be told when to change up. `shift_points` watches a shaft and names
the speed at which each gear gives way to the next, the way an automatic shifts
on engine speed:

```json
"shift_points": {
  "watch": "input",
  "up_at":   [1.0, 1.6],
  "down_at": [0.45, 0.8]
}
```

The run reports the schedule and checks it holds together — most usefully,
whether the box *hunts*: changing up drops the watched shaft, and if it drops
past the speed the box changes back down at, it changes down at once and then
straight back up. Give `down_at` and the run judges it; leave it out and the run
says how low each one would have to be. `examples/gearbox-3-speed-auto.json` is
a worked one.

With shift points the animation gives each gear as much of its length as the
schedule says it is held for, instead of dividing the time equally.

## The rules worth knowing before choosing tooth counts

These were expensive to establish. Using them saves a round trip through a
failed check.

- **Center distance is `(t1+t2)/16` studs.** 8t+24t is 2 studs, 40 LDU.
- **The two counts must SUM to a multiple of 8**, or the pair lands off the
  lattice and cannot be built. Each being a multiple of 4 is *not* enough:
  8t+12t needs 2.5 half studs and 36t+40t needs 9.5. This is the single most
  common way a plausible-looking gear train fails.
- **Standard counts**: 8, 12, 16, 20, 24, 36, 40. Mixing the {8,16,24,40} group
  with the {12,20,36} group breaks the rule above.
- **Of 512 three-gear loops, 7 close on the lattice and only 2 are buildable**:
  8-16-24 and 16-24-24. A third gear meshing with two others is nearly always
  the thing that will not fit.
- **A differential's case runs at the average of its two outputs.** Drive both
  outputs together and the case follows; drive them opposite and it stands
  still. That is what makes a subtractor work, and it is the only element with
  more than two ports.
- **Every shaft needs two bearings, as far apart as they will go.** One bearing
  means it whips under load, and the engine fails the mechanism for it.

## Writing the spec

Functional only — no positions, no part numbers:

```json
{
  "name": "reduction",
  "shafts": [
    {"id": "input", "bearings": 2},
    {"id": "output", "bearings": 2}
  ],
  "meshes": [
    {"a": "input", "b": "output", "teeth_a": 8, "teeth_b": 24}
  ],
  "differentials": [{"case": "case", "out_a": "left", "out_b": "right"}],
  "inputs": [{"shaft": "input", "speed": 1.0}],
  "outputs": ["output"]
}
```

### Gearboxes

A shifting gearbox is a set of **states** and a **coupling** per ratio. The
gears are always meshed and always turning; a shift changes which of them is
locked to the output. So a gear that freewheels is its own shaft, coupled to
the output only in the state where its ratio is selected:

```json
{
  "name": "3-speed",
  "states": ["1st", "2nd", "3rd"],
  "shafts": [
    {"id": "input", "bearings": 2}, {"id": "output", "bearings": 2},
    {"id": "g1", "bearings": 2}, {"id": "g2", "bearings": 2}, {"id": "g3", "bearings": 2}
  ],
  "meshes": [
    {"a": "input", "b": "g1", "teeth_a": 8, "teeth_b": 24},
    {"a": "input", "b": "g2", "teeth_a": 16, "teeth_b": 16},
    {"a": "input", "b": "g3", "teeth_a": 24, "teeth_b": 8}
  ],
  "couplings": [
    {"a": "output", "b": "g1", "name": "dog low", "states": ["1st"]},
    {"a": "output", "b": "g2", "name": "dog mid", "states": ["2nd"]},
    {"a": "output", "b": "g3", "name": "dog high", "states": ["3rd"]}
  ],
  "inputs": [{"shaft": "input", "speed": 1.0}],
  "outputs": ["output"]
}
```

That reports a ratio per state, and fails any state that locks up — engaging
two dog rings at once gives zero degrees of freedom, which is what a real
gearbox destroys itself doing.

Three things follow, and all of them matter when choosing counts:

- **Every pair on a shared pair of shafts needs the same center distance**, so
  the tooth counts of each ratio must sum to the *same* total. 16+24, 20+20 and
  24+16 all sum to 40, which is why those three make a clean three-speed box at
  2.5 studs between the shafts.
- **Only 16t, 20t and 24t can be shifted by a driving ring.** Those are the
  gears made with the ridged bore a ring engages; an 8t or 40t on a shifted
  shaft has to be moved another way. The engine warns about this — `shiftable`
  in the table below — but it cannot know your intent, so it is a warning and
  not a refusal.
- A coupling implies the two shafts are **coaxial** — a gear rides on the shaft
  it can be locked to — and the geometric layer places them on one line.

### The shifting parts, for when you tell the builder what to use

The engine does not place these. There are three driving-ring systems, all
using the same 4-ridge interlock (the newest ring has 8 ridges and is still
compatible, engaging sooner):

| system | driving ring | moved by |
| --- | --- | --- |
| 8466 4x4 Off-Roader | 6539 | 6641 / 51149 on a lever, guided by 6631 changeover plate |
| Bugatti Chiron | 18947 | 35188 selector |
| Yamaha MT-10 SP | 2473 | 4158 gear shifter with groove, 4159 shifter fork |

Two extension pieces exist, one 4-ridge and one 8-ridge, to lengthen any of
them. The MT-10 system also uses 2474, an 8-tooth gear stepper — despite the
name it is not for meshing, it indexes eighths of a turn so the drum lands where
it engages the ring.

Leave `states` out for a mechanism with only one. A coupling with no `states`
is permanently locked, which is a shaft joiner rather than a shift.

### Other fields

`kind` on a mesh is `spur` by default; `bevel`, `worm` and `chain` are the
others. `domain` on a shaft is `technic-studless` by default; use
`technic-brick` for a studded subassembly. Unknown keys are rejected, so a typo
is reported rather than silently building something else.

## Running it

Check first — it is fast and answers the only question that matters early:

```console
go run ./cmd/brickmesh --spec mechanism.json --check
```

Then build:

```console
go run ./cmd/brickmesh --spec mechanism.json --out mechanism.ldr --seed 1
```

For something to watch rather than only measure, add `--animate`: it writes a
`.lua` beside the model and references it, so opening the model in LDCad offers
the animations by name. A gearbox gets one per state plus a `shift` that walks
through them, sliding each ring between engaged and clear. `--seconds` and
`--turns` set the length and how far the input turns over it.

`--seed` makes the structural search reproducible. `--restarts` trades time for
a smaller structure. `--force` writes a model despite a failed check, which is
for looking at a problem, not for building.

The first run fetches the LDraw and LDCad libraries, so it is slow once.

## Reading what it says

Findings come worst first, one line per check.

| check | what a FAIL means |
| --- | --- |
| `dof` | 0 degrees of freedom: the train is locked. More drives than freedoms: the motors fight each other. |
| `bearings` | A shaft has fewer than two bearing points. |
| `grid` | The transmission spans two lattices; the holes cannot line up. |
| `center dist` | Tooth counts do not sum to a multiple of 8. Change a gear. |
| `loop closure` | Three shafts in a ring whose triangle does not close on the lattice. |
| `station` | Two gears want the same stretch of one shaft, or a bevel pair's shafts do not intersect. |
| `shift` | A state whose speeds do not resolve: it selects nothing definite. |
| `shiftable` | A gear on a shifted shaft with no driving-ring variant. Warning: the engine does not know how you mean to shift it. |
| `layout` | No arrangement of these shafts lands on the lattice at all. |
| `structure` | Nothing was found that bears every shaft. Widen the inventory or the span. |
| `parts` | A gear or shaft was left out of the file — no part number for that tooth count, or a shaft not along a lattice direction. |

Two warnings mean something specific:

- **`connectivity`** almost always appears. The covering search finds parts that
  bear every shaft but does not yet join them to each other, so the pieces
  float apart. The gears and their centers are still right; the thing will not
  hold together as written. This is PLAN.md M2 and it is expected, not a
  surprise — say so rather than presenting the file as buildable.
- **`station`** about the lattice usually means a bevel pair; see the open
  question before trusting the number.

## After building

Report honestly:

- the ratio achieved, and whether it is what was asked for
- the part count and the bounding volume the run reported
- anything left out — a gear with no known part number, a shaft the structural
  search could not bear, tooth phase
- that the gears are not phase-corrected, if the file is going to be looked at
  rather than only measured

The engine's job is to say no. If everything comes back OK on the first attempt
with an unusual mechanism, be suspicious and check the findings actually ran
rather than the spec being emptier than intended.

## Where the rest is written down

- `docs/findings.md` — the non-obvious facts, and why they cost something
- `PLAN.md` — what is done, what is not, and the two open questions
- `README.md` — the layer model the checks follow
