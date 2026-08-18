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

- **Nothing decides when to shift.** Gearboxes are expressible — states and dog
  rings, see below — and each state is checked and its ratio reported. What is
  missing is anything that *chooses* a state: no centrifugal governor, no
  torque-reactive mechanism, no sequential selector. "Auto shifting" therefore
  means: build the box, and the shift mechanism is yours to design.
- **The structure does not hold together.** Expect a `connectivity` warning on
  nearly every run; see the table below. The gears are right, the frame is not
  finished.
- **No tooth phase in the output.** Gears land at the right centers but are not
  turned to interleave, so a rendered model shows teeth overlapping. The
  geometry of the centers is right; the visual is not.
- **No animation export.** Stud.io opens the file; nothing animates it yet.
- **Bevel engagement is unresolved.** See the open question in `PLAN.md`. Avoid
  bevel pairs where a spur pair will do, and flag it when one is unavoidable.

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

Two things follow from the model, and both are useful when choosing counts:

- **Every pair on a shared pair of shafts needs the same center distance**, so
  the tooth counts of each ratio must sum to the *same* total. 8+24, 16+16 and
  24+8 all sum to 32, which is why those three make a clean three-speed box.
- A coupling implies the two shafts are **coaxial** — a gear rides on the shaft
  it can be locked to — and the geometric layer places them on one line.

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
