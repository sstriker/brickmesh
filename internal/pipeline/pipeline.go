// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package pipeline runs a mechanism from description to file.
//
// The stages are the layers the README describes, in order: check the
// mechanism functionally, place its shafts on the lattice, work out where the
// gears sit along them, find a structure that bears them, and write the result
// out as LDraw.
//
// Each stage can fail in a way worth reporting rather than aborting on, so the
// result carries findings from every stage that ran. A caller decides whether a
// FAIL is fatal; the pipeline's job is to say what it found.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/sstriker/brickmesh/internal/clutch"
	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/layout"
	"github.com/sstriker/brickmesh/internal/ldcad"
	"github.com/sstriker/brickmesh/internal/ldr"
	"github.com/sstriker/brickmesh/internal/mech"
	"github.com/sstriker/brickmesh/internal/part"
	"github.com/sstriker/brickmesh/internal/progress"
	"github.com/sstriker/brickmesh/internal/rigidity"
	"github.com/sstriker/brickmesh/internal/synth"
	"github.com/sstriker/brickmesh/internal/torque"
	"github.com/sstriker/brickmesh/internal/voxel"
)

// GearParts maps a tooth count to the part that has it.
//
// Taken from the libraries themselves rather than memory. Note 3647 and 32270
// have no shadow file, which is why gear orientation here follows the disc
// convention — a gear's rotation axis is its own Z — rather than asking the
// shadow library.
var GearParts = map[int]string{
	8:  "3647.dat",  // Technic Gear  8 Tooth
	12: "32270.dat", // Technic Gear 12 Tooth Double Bevel
	16: "4019.dat",  // Technic Gear 16 Tooth
	20: "32269.dat", // Technic Gear 20 Tooth Double Bevel
	24: "3648b.dat", // Technic Gear 24 Tooth with Single Axle Hole
	36: "32498.dat", // Technic Gear 36 Tooth Double Bevel
	40: "3649.dat",  // Technic Gear 40 Tooth
}

// DrivingRing is the part that locks a clutch gear to the shaft it rides on.
//
// 6539 is the ring of the first switching system, the one in 8466 4x4
// Off-Roader. Its hub is one stud wide with dog teeth reaching a stud either
// side, which is how it engages the gear beside it; measured from the part, not
// recalled. Its axle hole runs along its own Z, as the shadow library says, so
// it orients like a gear.
//
// Both generations are placed now. Which one a shift gets depends on the gear
// it has to lock to, and internal/clutch decides that.
func isRing(name string) bool {
	for _, s := range clutch.Systems {
		if s.Ring == name {
			return true
		}
	}
	return false
}

func isJoiner(name string) bool {
	for _, s := range clutch.Systems {
		if s.Joiner == name {
			return true
		}
	}
	return false
}

// SelectorParts are the shift linkage: what a builder puts between the catches
// and their hand. The catches themselves are placed — see clutch.System.Catch —
// but where the linkage runs follows from the housing rather than from the
// gears, so these are named rather than placed.
var SelectorParts = []string{
	"6631 Technic Plate 2 x 6 with 2 Position Gear Shift",
	"32068 Technic Plate 3 x 5 with Hole",
}

// isSelector reports whether a part is a catch that moves a driving ring.
//
// Which part that is depends on the generation, so it is asked of the systems
// rather than named here. See clutch.System.Catch.
func isSelector(name string) bool {
	for _, s := range clutch.Systems {
		if s.Catch != "" && s.Catch == name {
			return true
		}
	}
	return false
}

// SlipPart is the 24-tooth gear with a friction centre.
//
// A torque limiter, and the other kind of clutch entirely from the one a
// driving ring engages: no dogs anywhere on it, a centre that gives way above a
// force. internal/clutch excludes 24 from the shiftable counts for that reason
// and says so at length; this is the part that reason is about.
//
// 24 teeth is the only size it is made in, so a slip clutch has to sit on a
// 24-tooth station or nowhere.
const SlipPart = "76019.dat"

// DiffPart is the differential the engine places.
//
// 65414c01, which is the casing, its 28-tooth drive gear and the five bevel
// satellites inside it, as one LDraw shortcut. Three studs along its own Z with
// a port at each end where the output shafts enter, same as the bare housing it
// replaces.
//
// It replaces 62821, and the difference matters: 62821 is the housing alone, so
// a model built on it showed an empty shell. The gears that make a differential
// a differential were missing, and without them the case is a tube on the axles
// rather than an averaging device.
//
// The first answer to that was to name the gears in the report rather than place
// them, on the grounds that LDraw's housing does not model the chamber they sit
// in — its innermost surface is 10 LDU from the axis and a bevel is wider than
// that, so one put where it belongs reads as buried in the housing at every
// angle. That measurement is right and the conclusion drawn from it was wrong:
// the library has the assembled part, and the reason to look for one was that
// somebody said they were surprised the parts and measurements did not exist.
// They did. 65414's own title is "Differential Casing for 5 Internal Gears",
// and 65414c01 is that casing with the five in it.
//
// Five, not three. Two side gears on the outputs and three planets between them.
const DiffPart = "65414c01.dat"

// DiffHalf is how far the housing reaches either side of its centre, in LDU.
const DiffHalf = 30.0

// AxleParts maps a length in studs to the part that is that long. Verified
// against the library: every one runs along its own X and is 12 LDU across.
var AxleParts = map[int]string{
	2: "3704.dat", 3: "4519.dat", 4: "3705.dat", 5: "32073.dat",
	6: "3706.dat", 7: "44294.dat", 8: "3707.dat", 9: "60485.dat",
	10: "3737.dat", 12: "3708.dat", 16: "50451.dat",
}

// axleLengths are the lengths available, shortest first.
var axleLengths = []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 12, 16}

// Deps are the libraries the pipeline reads from.
type Deps struct {
	// Lib is where triangles come from. An interface rather than the parts
	// library, because in a browser they come out of a mesh blob instead.
	Lib part.Shapes
	// Shadow is where connection points come from: the shadow library on a
	// machine with one, the published catalogue in a browser.
	Shadow part.Holes
	Rast   *voxel.Rasterizer
}

// Options tunes the stages.
type Options struct {
	MaxLayouts int
	Span       int
	Restarts   int
	// Into places the mechanism inside a model that already exists, rather
	// than building it a frame of its own. See FitInto.
	Into *FitInto
	// HoldShift asks for a frame that bears the axle each catch turns on, as
	// well as the shafts.
	//
	// A choice rather than the default, because it is a real trade and not a
	// free improvement. Measured on the two-speed: off, the frame is two walls
	// and 10.4 cubic studs and the shift falls out; on, it is six parts and
	// about 103, and holds everything. A builder who wants a compact gearbox
	// and will hold the shift themselves should not be made to pay for that.
	//
	// It also costs search. Where such a frame exists it is found quickly; on a
	// compound gearbox, where two control axles on two lines make the cover
	// much harder, the full allowance took 51 seconds against 0.85 and still
	// came back in two pieces. The run says so rather than pretending.
	HoldShift bool
	// Budget is what a good structure is: what to charge for beam length, for
	// parts, for the envelope, and how big the envelope may be. Zero means
	// synth.DefaultBudget.
	Budget        synth.Budget
	Seed          int64
	Inventory     []part.Beam
	SkipStructure bool
	// Animate adds LDCad groups and an animation script turning every shaft at
	// the ratio the mechanism solved for.
	Animate    bool
	ScriptName string
	Seconds    float64
	InputTurns float64
	// Progress is told as each stage starts and as the structural search works
	// through its restarts. Optional.
	Progress progress.Func
}

// Result is what the run found, stage by stage.
type Result struct {
	Findings  []mech.Finding
	Layout    *layout.Layout
	Stations  []layout.Station
	Structure *synth.Solution
	Axles     []rigidity.Axle
	Model     *ldr.Model
	// Script is the animation, when one was asked for.
	Script *ldcad.Script

	axles []axlePlacement
	// ringSites is where each shift's driving ring sits and slides, kept so
	// the animation can move it rather than only place it.
	ringSites []ringSite
	// fitOffset is how far the mechanism was moved to sit in a given model, and
	// fitBearsEverything whether that model holds all of it.
	into               []ldr.Placed
	fitOffset          geom.Vec3
	fitBearsEverything bool
	// slip is the shafts a torque limiter is fitted to, so the gear chosen at a
	// 24-tooth station on one is the slipping variant.
	slip map[string]bool
	// intoOccupied and intoRast are the model this was fitted into, kept so
	// that what is placed after the fit — the catches above all — can be kept
	// out of it too.
	intoOccupied map[geom.Cell]bool
	intoRast     *voxel.Rasterizer
}

// axlePlacement is a shaft worked out but not yet written.
type axlePlacement struct {
	name   string
	studs  int
	rot    geom.Mat3
	center geom.Vec3
	shaft  string
	label  string
}

// Failed reports whether any finding is fatal.
func (r *Result) Failed() bool {
	for _, f := range r.Findings {
		if f.Level == "FAIL" {
			return true
		}
	}
	return false
}

// Run takes a mechanism as far as it will go.
// Run takes a context because the structural search is the slow part of this
// and a caller watching it needs to be able to stop it — a browser abandoning a
// run on a changed input, a CLI on a keystroke. Everything cancellable is
// downstream of here, so the context goes in at the top rather than being
// reached for later.
func Run(ctx context.Context, m *mech.Mechanism, deps Deps, opts Options) (*Result, error) {
	if opts.MaxLayouts <= 0 {
		opts.MaxLayouts = 20
	}
	if opts.Span <= 0 {
		opts.Span = 4
	}
	if opts.Inventory == nil {
		opts.Inventory = part.Beams
	}

	res := &Result{Findings: m.RunChecks()}
	if res.Failed() {
		// The functional layer has already said this cannot work. Placing it
		// would only produce a second opinion on the same problem.
		return res, nil
	}

	opts.Progress.Report(progress.Report{Stage: progress.StageLayout,
		Note: "arranging the shafts on the lattice"})
	layouts := layout.Realize(m, layout.Options{
		MaxSolutions: opts.MaxLayouts, Span: opts.Span,
	})
	if len(layouts) == 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "FAIL", Check: "layout",
			Detail: "no arrangement of these shafts lands on the lattice",
		})
		return res, nil
	}
	res.Layout = layouts[0]
	// Settled before the fit, not after it, because the fit has to name the
	// same parts the model will end up with: a station on a slipping shaft
	// takes the slip clutch, and one a ring engages takes the clutch gear. The
	// fitter guessing plain gears put a 20-tooth where a 35188 would go and
	// passed an offset the finished model failed on.
	res.slip = slipShafts(m)
	if opts.Into != nil {
		res.Layout = bestLayoutFor(layouts, opts.Into)
		if err := fitInto(ctx, deps, res, opts.Into); err != nil {
			return res, err
		}
		res.into = opts.Into.Parts
		res.intoOccupied, res.intoRast = opts.Into.Occupied, opts.Into.Rast
		opts.SkipStructure = opts.SkipStructure || res.fitBearsEverything
	}

	stations, stationFindings := layout.SolveStations(m, res.Layout)
	// The along-axis half of a fit offset lives here rather than in the layout:
	// a placement cannot carry it, since a line has no origin along itself.
	if opts.Into != nil {
		for i := range stations {
			if place, ok := res.Layout.Place[stations[i].Shaft]; ok {
				d := place.Direction.Unit()
				stations[i].Axial += res.fitOffset.Dot(d) / synth.HalfStud
			}
		}
	}
	res.Stations = stations
	res.Findings = append(res.Findings, stationFindings...)

	// Where the rings go decides where the shafts are cut, so it is settled
	// before the axles rather than when the model is drawn.
	checkFraming(res)
	checkSlipClutches(m, res)
	res.ringSites = ringSites(m, res)
	// Worked out before the structural search, because the rigidity check needs
	// to know the shafts are there: they are what ties the bearings together.
	res.axles = computeAxles(m, res)
	// And the catches, for the same reason: the axle each one turns on is a
	// line the frame has to bear, and the search cannot be told after it runs.
	settleCatches(ctx, deps, res)

	if !opts.SkipStructure {
		if err := runStructure(ctx, res, deps, opts); err != nil {
			return res, err
		}
	}
	if err := ctx.Err(); err != nil {
		return res, err
	}

	opts.Progress.Report(progress.Report{Stage: progress.StageModel,
		Note: "placing the parts"})
	model, err := buildModel(m, res, deps)
	if err != nil {
		return res, err
	}
	// The fasteners, after the frame is settled and before anything measures
	// the model: a pin occupies the holes it goes through, so a check that ran
	// first would be checking a model that is not the one written out.
	if err := placePins(res, deps, model); err != nil {
		return res, fmt.Errorf("placing the pins: %w", err)
	}
	res.Model = model

	opts.Progress.Report(progress.Report{Stage: progress.StagePhase})
	if err := applyPhase(ctx, m, res, deps); err != nil {
		return res, err
	}
	// Where the force between meshed teeth goes, now that there is a frame to
	// ask about. Before the clearance sweep, since it is cheap and a frame that
	// cannot take the load is worth knowing about either way.
	checkMeshing(res, m)
	checkLoadPaths(res, deps, m)

	opts.Progress.Report(progress.Report{Stage: progress.StageClearance})
	if err := checkClearance(ctx, res, deps); err != nil {
		return res, err
	}
	if opts.Animate {
		opts.Progress.Report(progress.Report{Stage: progress.StageAnimation})
		animate(m, res, opts)
	}
	return res, nil
}

// firstThatFits takes the best solution whose own parts do not overlap.
//
// Beams only: everything else is placed elsewhere and checked as a whole
// afterwards. This is about the one thing the covering search can get wrong on
// its own, which is putting two of its own parts in the same place. It works on
// a voxel lattice that has to tolerate parts touching, and that tolerance is
// enough to let two beams share a few LDU. The search proposes; the geometry
// disposes.
// firstThatHolds takes the first structure that fits together AND stays rigid
// once braced.
//
// Rigidity used to be a post-check: the first candidate that fitted was taken,
// braced, and then told off in the report if it still hinged. That is advice
// arriving after the decision. The search returns solutions smallest first and
// there are usually many, so a frame that folds can simply be passed over for
// one that does not.
//
// It falls back rather than failing. If every candidate hinges, the first that
// at least fits is taken and the rigidity check says so — a model that hinges is
// still worth looking at, and refusing to emit one would hide the thing the
// report is trying to show.
func firstThatHolds(ctx context.Context, solutions []synth.Solution, deps Deps,
	searcher *synth.Searcher, res *Result) (*synth.Solution, int, int) {

	var fallback *synth.Solution
	fallbackAt, hinged := 0, 0

	for i := range solutions {
		if err := ctx.Err(); err != nil {
			break
		}
		if overlapping(ctx, solutions[i].Parts, deps) {
			continue
		}
		if fallback == nil {
			fallback, fallbackAt = &solutions[i], i
		}
		braced, err := searcher.StiffenToRigid(solutions[i].Parts)
		if err != nil {
			continue
		}
		if holdsRigid(deps, braced, res) {
			return &solutions[i], i, hinged
		}
		hinged++
	}
	if fallback != nil {
		return fallback, fallbackAt, hinged - 1
	}
	return nil, len(solutions), hinged
}

// holdsRigid reports whether a braced structure is one piece and does not fold.
func holdsRigid(deps Deps, parts []synth.Placed, res *Result) bool {
	placed := make([]part.Placed, 0, len(parts))
	for _, p := range parts {
		placed = append(placed, part.Placed(p))
	}
	findings, err := rigidity.AnalyzeWith(deps.Shadow, placed, nil, res.Axles)
	if err != nil {
		return false
	}
	for _, f := range findings {
		if f.Level == "FAIL" {
			return false
		}
	}
	return true
}

func overlapping(ctx context.Context, parts []synth.Placed, deps Deps) bool {
	for a := 0; a < len(parts); a++ {
		for b := a + 1; b < len(parts); b++ {
			// Beams among themselves: none of them turns, so no axes are
			// needed here.
			inside, _, err := sharesSpace(ctx, deps,
				lattice(parts[a]), lattice(parts[b]), turning{}, -1, -1)
			if err == nil && inside {
				return true
			}
		}
	}
	return false
}

// lattice turns a placed beam into the shape the clearance check works on.
func lattice(p synth.Placed) ldr.Part {
	return ldr.Part{Name: p.Part, Rot: geom.Rotations[p.Rot], Pos: p.Origin}
}

func runStructure(ctx context.Context, res *Result, deps Deps, opts Options) error {
	if deps.Rast == nil || deps.Shadow == nil {
		return fmt.Errorf("the structural search needs both libraries")
	}
	searcher := synth.NewSearcher(deps.Rast, deps.Shadow, opts.Inventory)
	searcher.Taken = ringSpans(res)
	searcher.Reserved = turningCells(res, deps)
	if opts.Into != nil {
		// The model it is being fitted into is in the way too, and the search
		// had no idea: it would happily run a beam straight through somebody's
		// chassis to reach a bearing.
		//
		// Eroded first, unlike the turning parts already in Reserved. Those
		// admit no contact at all, because a beam sharing a cell with something
		// that turns is inside it. A chassis is the opposite case: a new beam
		// bolted to it shares cells at the joint by design, which is what
		// bolting IS. A layer of erosion is the difference between touching it
		// and going through it.
		if searcher.Reserved == nil {
			searcher.Reserved = map[geom.Cell]bool{}
		}
		for c := range erode(opts.Into.Occupied) {
			searcher.Reserved[c] = true
		}
	}
	// The same joints the rigidity report counts. Without these the search
	// braces against a frame it believes to be in loose pieces.
	searcher.Shafts = res.Axles
	// A control axle is not a shaft, so the layout does not offer it, but it
	// still has to be held — if the caller wants to pay for that. See
	// Options.HoldShift.
	restarts := opts.Restarts
	if opts.HoldShift {
		searcher.Extra = controlRequirements(res)
	}
	solutions, err := searcher.Synthesize(ctx, res.Layout, res.Stations, synth.Options{
		Restarts: restarts, Seed: opts.Seed, Progress: opts.Progress,
		Budget: opts.Budget,
	})
	if errors.Is(err, context.Canceled) {
		return err // stopped is not a failure of the search, and says so itself
	}
	if err != nil {
		return fmt.Errorf("the structural search: %w", err)
	}
	if len(solutions) == 0 {
		why := "no structure found that bears every shaft; the gears are " +
			"placed but nothing holds them"
		if b := opts.Budget.MaxStuds; b.X > 0 || b.Y > 0 || b.Z > 0 {
			why += fmt.Sprintf(". The envelope was capped at %v studs, and a "+
				"bound is not a preference — a frame outside it is not a "+
				"candidate. Try widening it", b)
		}
		res.Findings = append(res.Findings, mech.Finding{
			Level: "WARN", Check: "structure", Detail: why})
		return nil
	}
	// The search works on a voxel lattice, which has to tolerate parts touching
	// — the rasteriser marks every cell a part so much as brushes — and that
	// tolerance lets two beams overlap by a little. So the candidates are put
	// to the exact question before one is taken. The search proposes; the
	// geometry disposes.
	chosen, rejected, hinged := firstThatHolds(ctx, solutions, deps, searcher, res)
	if chosen == nil {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "WARN", Check: "structure", Detail: fmt.Sprintf(
				"every one of the %d structures found has parts inside each other; "+
					"the gears are placed but nothing holds them", len(solutions))})
		return nil
	}
	if rejected > 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "OK", Check: "structure", Detail: fmt.Sprintf(
				"%d structure(s) rejected before this one for parts inside each "+
					"other or for hinging", rejected)})
	}
	if hinged > 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "OK", Check: "structure", Detail: fmt.Sprintf(
				"%d of those hinged even after bracing, and were passed over "+
					"rather than reported", hinged)})
	}
	// Stiffening happens here rather than inside the search: it is the one
	// solution that will be used, and doing it to every restart costs minutes
	// for an answer that is thrown away sixty times over.
	// Already stiffened by the search that chose it, so this is the same call
	// reaching the same answer; it stays because the count is worth reporting.
	stiffened, err := searcher.StiffenToRigid(chosen.Parts)
	if err != nil {
		return fmt.Errorf("stiffening the structure: %w", err)
	}
	if added := len(stiffened) - len(chosen.Parts); added > 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "OK", Check: "structure", Detail: fmt.Sprintf(
				"%d beam(s) added to stop it hinging", added)})
		chosen.Parts = stiffened
		chosen.Count = len(stiffened)
		chosen.BBoxStud3 = searcher.BoundingVolume(stiffened)
	}
	res.Structure = chosen
	res.Findings = append(res.Findings, mech.Finding{
		Level: "OK", Check: "structure",
		Detail: fmt.Sprintf("%d parts bear the shafts, %.1f cubic studs",
			res.Structure.Count, res.Structure.BBoxStud3),
	})

	// Bearing every shaft is not the same as holding together. Ask — but the
	// covering search is not finished, and its repair phase can only bridge
	// pieces whose holes already line up: joining a perpendicular pair needs
	// the A* connection search, which is not wired into it yet. So this is
	// reported loudly and does not condemn the model, which is still worth
	// looking at. PLAN.md M2 is where that gets fixed.
	rigid, err := rigidity.AnalyzeWith(deps.Shadow, res.Structure.Parts, opts.Inventory,
		res.Axles)
	if err != nil {
		return fmt.Errorf("the rigidity check: %w", err)
	}
	for _, f := range rigid {
		if f.Level == "FAIL" {
			f.Level = "WARN"
			f.Detail += " — the covering search does not join pieces yet, see PLAN.md M2"
		}
		res.Findings = append(res.Findings, f)
	}
	return nil
}

// buildModel places the gears along their shafts and adds the structure.
func buildModel(m *mech.Mechanism, res *Result, deps Deps) (*ldr.Model, error) {
	model := ldr.New(m.Name)

	// The model it is being fitted into comes first, so what is written is a
	// copy of somebody's build with a mechanism added rather than a mechanism
	// with their build alongside it. Their parts keep their own colours.
	for _, p := range res.into {
		model.Add(p.Name, p.Color, p.Rot, p.Pos, "")
	}

	stations := append([]layout.Station(nil), res.Stations...)
	sort.SliceStable(stations, func(i, j int) bool {
		if stations[i].Shaft != stations[j].Shaft {
			return stations[i].Shaft < stations[j].Shaft
		}
		return stations[i].Axial < stations[j].Axial
	})

	sites := res.ringSites
	for _, st := range stations {
		name, ok := gearAt(st, sites, res.slip)
		if !ok {
			res.Findings = append(res.Findings, mech.Finding{
				Level: "WARN", Check: "parts",
				Detail: fmt.Sprintf("no part known for a %dt gear; it is left out of the model",
					st.Teeth),
			})
			continue
		}
		place, ok := res.Layout.Place[st.Shaft]
		if !ok {
			continue
		}
		rot, ok := alignZTo(place.Direction)
		if !ok {
			res.Findings = append(res.Findings, mech.Finding{
				Level: "WARN", Check: "parts",
				Detail: fmt.Sprintf("shaft '%s' does not run along a lattice direction; "+
					"its gears are left out", st.Shaft),
			})
			continue
		}
		// Shaft points are in half studs; stations are along the shaft.
		pos := place.Point.Scale(synth.HalfStud).
			Add(place.Direction.Scale(st.Axial * synth.HalfStud))
		model.Add(name, ldr.ColorLightGray, rot, pos,
			fmt.Sprintf("%dt on shaft '%s'", st.Teeth, st.Shaft))
	}

	placeDrivingRings(res, model, sites)
	for _, a := range res.axles {
		model.Add(a.name, ldr.ColorBlack, a.rot, a.center,
			a.label)
	}
	if res.Structure != nil {
		for _, p := range res.Structure.Parts {
			if err := model.AddLattice(p.Part, ldr.ColorBlack, p.Rot, p.Origin, ""); err != nil {
				return nil, err
			}
		}
	}
	// After the frame, because whether the frame holds them is part of the
	// answer.
	placeControlAxles(res, deps, model)
	// And the markers last of all, because they are the one thing here that
	// may be left out: a flag on a shaft end is worth having and not worth
	// displacing a beam for, so it has to see what is already placed.
	addMarkers(res, deps, model, m)
	return model, nil
}

// placeDrivingRings puts a ring beside each gear a shift engages.
//
// A coupling forces its two shafts coaxial, so the ring goes on that shared
// line, next to the gear it locks. Which side is whichever is free.
// ringSite is a driving ring, the gear it engages, and where it slides.
type ringSite struct {
	coupling mech.Coupling
	station  layout.Station
	// system is which generation of hardware this shift uses. They do not mix:
	// a ring of one does not grip the other's gears.
	system clutch.System
	// rides is the shaft the ring is splined to, which is the side of the
	// coupling that is not the gear it engages. It turns with that shaft in
	// every state, engaged or not — the gear is the part that runs free.
	rides string
	// Engaged is where the ring sits when the coupling is engaged, along the
	// shaft in half studs; Disengaged is where it slides to when it is not.
	engaged, disengaged float64
	// joiner is where the ridged joiner under the ring is centered.
	joiner float64
	// catchAt is where the catch that moves this ring sits, relative to the
	// ring, once it has been placed. Zero when no catch was placed.
	catchAt geom.Vec3
	// mate is the second gear this same ring engages, when it sits between two
	// of them and reaches either by sliding.
	//
	// A driving ring has dogs on both faces. One between two clutch gears is
	// how a real two-speed is built, and placing one ring per shift put two
	// rings back to back where a builder would use a single part — the report
	// said as much and the placement did it anyway.
	//
	// catchRot is the catch's orientation in the model, kept because the
	// animation turns it about one of its own axes and needs to know where
	// that axis points once placed.
	catchRot geom.Mat3
	// When this is set, disengaged is not "clear of the gear": it is the
	// position that engages the mate, and the neutral where neither is engaged
	// is halfway between the two.
	mate *ringMate
}

// ringMate is the far side of a shared ring.
type ringMate struct {
	coupling mech.Coupling
	station  layout.Station
}

// ringSites works out where every shift's driving ring goes.
//
// Separated from placing them because the gears have to know too: a gear a ring
// engages is not the plain gear, it is the clutch variant, and buildModel picks
// the part before any ring is placed.
func ringSites(m *mech.Mechanism, res *Result) []ringSite {
	shifts := shiftsOf(m, res)
	paired := pairShifts(res, shifts)

	var out []ringSite
	for _, sh := range shifts {
		if sh.folded {
			continue // its ring is the one its partner carries
		}
		c, station, rides := sh.coupling, sh.station, sh.rides

		// Two gears sharing one ring settle the hardware between them: a ring
		// of one generation does not grip the other's gears, so a pair has to
		// be of one generation even where each gear alone could be either.
		if mate, ok := paired[sh.key()]; ok {
			site, ok := betweenSites(res, sh, mate)
			if ok {
				out = append(out, site)
				continue
			}
		}

		// Which hardware can shift this gear at all decides everything after
		// it: the ring, the joiner under it, the gear itself, and how much
		// shaft the three of them need.
		system, ok := clutch.For(station.Teeth)
		if !ok {
			res.Findings = append(res.Findings, mech.Finding{
				Level: "FAIL", Check: "parts", Detail: fmt.Sprintf(
					"nothing can shift a %dt gear: no driving ring has a clutch gear "+
						"that size. %v can be shifted; reach this ratio through one of "+
						"those instead", station.Teeth, clutch.ShiftableTeeth())})
			continue
		}

		axial, ok := freeSideOfIn(res.Stations, station, res.Layout, system)
		if !ok {
			res.Findings = append(res.Findings, mech.Finding{
				Level: "WARN", Check: "parts", Detail: fmt.Sprintf(
					"no room beside the %dt on '%s' for a driving ring; the shift for %v "+
						"has nowhere to go", station.Teeth, station.Shaft, c.States)})
			continue
		}
		// Away from the gear, which is the only direction there is room in.
		side := 1.0
		if axial < station.Axial {
			side = -1
		}
		// The joiner goes as close to the gear as it can without touching it.
		// It is a solid sleeve wider than an axle, so it cannot enter the
		// gear's bore, and pushing it out any further than it has to be only
		// takes ring off it — the second system's joiner is three studs long
		// and its ring nearly three, so there is not much to spare.
		joiner := station.Axial + side*(station.Thickness/2+system.JoinerHalf)
		out = append(out, ringSite{coupling: c, station: station, rides: rides,
			system: system, joiner: joiner,
			engaged: axial, disengaged: axial + side*system.Travel()})
	}
	return out
}

// shift is one coupling that a ring has to make, before any hardware is chosen.
type shift struct {
	coupling mech.Coupling
	station  layout.Station
	rides    string
	folded   bool // its ring is carried by the partner it shares with
}

func (s shift) key() string {
	return fmt.Sprintf("%s@%g/%s", s.station.Shaft, s.station.Axial, s.rides)
}

// shiftsOf lists the couplings that need a ring, and the gear each engages.
func shiftsOf(m *mech.Mechanism, res *Result) []*shift {
	var out []*shift
	for _, link := range m.Links {
		c, ok := link.(mech.Coupling)
		if !ok || len(c.States) == 0 {
			continue // a permanent coupling is a joiner, not a shift
		}
		station, found := stationOn(res.Stations, c.B)
		rides := c.A
		if !found {
			station, found = stationOn(res.Stations, c.A)
			rides = c.B
		}
		if !found {
			continue // nothing to engage: reported by CheckShiftable already
		}
		out = append(out, &shift{coupling: c, station: station, rides: rides})
	}
	return out
}

// pairShifts finds the shifts that one ring can serve two of.
//
// A driving ring has dogs on both faces, so one sitting between two clutch
// gears engages either by sliding. One per shift puts two of them back to back,
// which no builder would do — and the report said so every time it ran while
// the placement went ahead anyway.
func pairShifts(res *Result, shifts []*shift) map[string]*shift {
	out := map[string]*shift{}
	for i, a := range shifts {
		if a.folded {
			continue
		}
		for _, b := range shifts[i+1:] {
			if b.folded || !canPair(res, a, b) {
				continue
			}
			out[a.key()] = b
			b.folded = true
			break
		}
	}
	return out
}

// canPair reports whether one ring could serve both shifts.
func canPair(res *Result, a, b *shift) bool {
	if a.rides != b.rides || a.station.Axial == b.station.Axial {
		return false
	}
	if _, ok := clutch.ForBoth(a.station.Teeth, b.station.Teeth); !ok {
		return false
	}
	pa, oka := res.Layout.Place[a.station.Shaft]
	pb, okb := res.Layout.Place[b.station.Shaft]
	if !oka || !okb || pa.Key() != pb.Key() {
		return false
	}
	// Nothing may stand between them, or the ring cannot reach across.
	lo, hi := a.station.Axial, b.station.Axial
	if hi < lo {
		lo, hi = hi, lo
	}
	for _, st := range res.Stations {
		if p, ok := res.Layout.Place[st.Shaft]; !ok || p.Key() != pa.Key() {
			continue
		}
		if st.Axial > lo && st.Axial < hi {
			return false
		}
	}
	return true
}

// betweenSites builds the one ring that sits between two gears.
func betweenSites(res *Result, a, b *shift) (ringSite, bool) {
	system, ok := clutch.ForBoth(a.station.Teeth, b.station.Teeth)
	if !ok {
		return ringSite{}, false
	}
	lo, hi := a, b
	if hi.station.Axial < lo.station.Axial {
		lo, hi = hi, lo
	}
	// Each engaged position is on the face looking at the other gear.
	near := lo.station.Axial + system.Engaged
	far := hi.station.Axial - system.Engaged
	if far <= near {
		return ringSite{}, false // no room to sit between and reach either
	}
	return ringSite{
		coupling: lo.coupling, station: lo.station, rides: lo.rides,
		system:  system,
		engaged: near, disengaged: far,
		// One joiner under the whole travel, centred between the two gears.
		joiner: (lo.station.Axial + hi.station.Axial) / 2,
		mate:   &ringMate{coupling: hi.coupling, station: hi.station},
	}, true
}

// gearAt names the part for a station, using the clutch variant where a ring
// engages it and the slipping variant where a torque limiter is fitted.
func gearAt(st layout.Station, sites []ringSite, slip map[string]bool) (string, bool) {
	if st.Teeth == 24 && slip[st.Shaft] {
		return SlipPart, true
	}
	for _, site := range sites {
		on := site.station.Shaft == st.Shaft && site.station.Axial == st.Axial
		if site.mate != nil && site.mate.station.Shaft == st.Shaft &&
			site.mate.station.Axial == st.Axial {
			on = true
		}
		if !on {
			continue
		}
		if name, ok := site.system.Gears[st.Teeth]; ok {
			return name, true
		}
		break
	}
	name, ok := GearParts[st.Teeth]
	return name, ok
}

// placeDrivingRings puts a ring beside each shifted gear, at the distance the
// sweep says its dogs sit in the gear's recesses.
func placeDrivingRings(res *Result, model *ldr.Model, sites []ringSite) {
	// Written back, because where the catch ended up decides where it has to
	// slide to and the animation asks afterwards.
	back := func(i int, at geom.Vec3, rot geom.Mat3) {
		if i < len(res.ringSites) {
			res.ringSites[i].catchAt = at
			res.ringSites[i].catchRot = rot
		}
	}
	var nominal []string
	catches := 0
	for i, site := range sites {
		place, ok := res.Layout.Place[site.station.Shaft]
		if !ok {
			continue
		}
		rot, ok := alignZTo(place.Direction)
		if !ok {
			continue
		}
		pos := place.Point.Scale(synth.HalfStud).
			Add(place.Direction.Scale(site.engaged * synth.HalfStud))
		label := site.coupling.Name
		if label == "" {
			label = fmt.Sprintf("driving ring for %v", site.coupling.States)
		}
		model.Add(site.system.Ring, ldr.ColorRed, rot, pos, label)
		if at, rot, ok := placeSelector(res, model, site, place, pos, label); ok {
			back(i, at, rot)
			catches++
		}

		if _, ok := site.system.Gears[site.station.Teeth]; !ok {
			nominal = append(nominal, fmt.Sprintf("%dt on '%s'",
				site.station.Teeth, site.station.Shaft))
		}
	}

	if len(sites) == 0 {
		return
	}
	shared := 0
	for _, site := range sites {
		if site.mate != nil {
			shared++
		}
	}
	sharing := ""
	if shared > 0 {
		sits, engages := "sits", "engages"
		if shared > 1 {
			sits, engages = "sit", "engage"
		}
		sharing = fmt.Sprintf(" %d of them %s between two clutch gears and "+
			"%s either by sliding, which is one part where a ring per shift "+
			"would have used two.", shared, sits, engages)
	}
	res.Findings = append(res.Findings, mech.Finding{
		Level: "OK", Check: "parts", Detail: fmt.Sprintf(
			"%d driving ring(s) placed, each at its system's engaged distance "+
				"from the gear's center so its dogs sit in the recesses, and %d "+
				"catch(es) with them.%s What is still not placed is the linkage "+
				"back from the catches to a lever (%s): where that runs follows "+
				"from the housing, not from the gears. The catch's own hold on a "+
				"ring could not be searched for either — in LDraw a fork that "+
				"straddles a groove reads as a fork that collides — so it is "+
				"measured from official models. See docs/shifting.md.",
			len(sites), catches, sharing, strings.Join(SelectorParts, ", "))})

	if len(nominal) > 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "WARN", Check: "parts", Detail: fmt.Sprintf(
				"%v get a plain gear, not a clutch one, so those rings turn beside "+
					"their gear without gripping it. %v are the counts that can be "+
					"shifted", nominal, clutch.ShiftableTeeth())})
	}
}

// stationOn finds a gear on a shaft.
func stationOn(stations []layout.Station, shaft string) (layout.Station, bool) {
	for _, st := range stations {
		if st.Shaft == shaft {
			return st, true
		}
	}
	return layout.Station{}, false
}

// freeSideOf picks a spot beside a gear that nothing else occupies.
//
// Judged along the line rather than the named shaft: the whole point of a
// coupling is that two shafts share an axis, so the gear of another ratio is
// right there even though it belongs to a differently named shaft.
func freeSideOfIn(stations []layout.Station, gear layout.Station, l *layout.Layout,
	system clutch.System) (float64, bool) {

	line := layout.LineOf(l, gear.Shaft)
	for _, side := range []float64{1, -1} {
		axial := gear.Axial + side*system.Engaged
		// The ring has to stay clear of everything else on the line once it has
		// slid the whole way out.
		lo := math.Min(axial, axial+side*system.Travel()) - system.RingHalf
		hi := math.Max(axial, axial+side*system.Travel()) + system.RingHalf
		clear := true
		for _, st := range stations {
			if layout.LineOf(l, st.Shaft) != line || st == gear {
				continue
			}
			slo, shi := st.Span()
			if math.Min(shi, hi)-math.Max(slo, lo) > 1e-6 {
				clear = false
				break
			}
		}
		if clear {
			return axial, true
		}
	}
	return 0, false
}

// placeAxles puts a shaft through each line that carries anything.
//
// They are real parts the build needs, and they are also what ties the bearings
// of one shaft together: a bearing's hole faces along its shaft, so no beam
// laid between two of them can be pinned to either, and the shaft itself is
// what actually connects them.
func computeAxles(m *mech.Mechanism, res *Result) []axlePlacement {
	// An axle belongs to the shaft the gears are locked to, not to whichever
	// happened to be seen first: on a gearbox's layshaft that is the output,
	// and calling it "first gear's shaft" would animate the wrong thing.
	owner := map[string]bool{}
	for _, l := range m.Links {
		if c, ok := l.(mech.Coupling); ok && len(c.States) > 0 {
			owner[c.A] = true
		}
	}

	type line struct {
		place  layout.Placement
		lo, hi float64 // extent along the shaft, in half studs
		shaft  string
	}
	lines := map[[6]float64]*line{}

	note := func(shaft string, axial float64) {
		place, ok := res.Layout.Place[shaft]
		if !ok {
			return
		}
		key := place.Key()
		l, seen := lines[key]
		if !seen {
			lines[key] = &line{place: place, lo: axial, hi: axial, shaft: shaft}
			return
		}
		l.lo = math.Min(l.lo, axial)
		l.hi = math.Max(l.hi, axial)
		if owner[shaft] && !owner[l.shaft] {
			l.shaft = shaft
		}
	}

	for _, st := range res.Stations {
		note(st.Shaft, st.Axial)
	}
	// A shaft that gears are locked to may carry none of its own, so it would
	// otherwise never be seen — and it is the one that turns at the selected
	// ratio.
	for id := range owner {
		if place, ok := res.Layout.Place[id]; ok {
			_ = place
			note(id, 0)
		}
	}
	// A ring and the joiner under it stand past the last gear, and the shaft
	// has to run under them: without this the line stops at the gears and the
	// length left for the axle beyond the last joiner comes out negative.
	for shaft, spans := range ringSpans(res) {
		for _, sp := range spans {
			note(shaft, sp[0])
			note(shaft, sp[1])
		}
	}
	// The bearings are the far ends of what the shaft has to reach.
	for _, r := range synth.BearingRequirements(res.Layout, res.Stations, 2, 8) {
		place, ok := res.Layout.Place[r.Shaft]
		if !ok {
			continue
		}
		note(r.Shaft, r.Point.Sub(place.Point.Scale(synth.HalfStud)).Dot(place.Direction)/synth.HalfStud)
	}

	keys := make([][6]float64, 0, len(lines))
	for k := range lines {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return lessKey(keys[i], keys[j]) })

	var axles []axlePlacement
	for _, k := range keys {
		l := lines[k]
		// Reach a stud past each end so it runs right through the bearings.
		lo := l.lo*synth.HalfStud - geom.Stud
		hi := l.hi*synth.HalfStud + geom.Stud
		rot, ok := alignXTo(l.place.Direction)
		if !ok {
			continue
		}
		origin := l.place.Point.Scale(synth.HalfStud)
		at := func(d float64) geom.Vec3 { return origin.Add(l.place.Direction.Scale(d)) }

		// A differential cuts its line in two and owns the middle. The shafts
		// either side of it turn independently — that is what a differential
		// is — so they are separate axles, not one running through.
		if d, ok := differentialOn(m, res, l.place); ok {
			axles = append(axles, differentialAxles(res, l.place, d, lo, hi)...)
			mid := (lo + hi) / 2
			res.Axles = append(res.Axles, rigidity.Axle{
				Point: at(mid), Dir: l.place.Direction,
				From: -(hi - lo) / 2, To: (hi - lo) / 2,
			})
			continue
		}

		// A shaft carrying a driving ring is cut where the ring rides: the ring
		// is splined to a joiner, not to the axle, and two axles butt inside it.
		joiners := joinersOn(res, l.place)
		for _, j := range joiners {
			jrot, ok := alignZTo(l.place.Direction)
			if !ok {
				continue
			}
			axles = append(axles, axlePlacement{
				name: j.system.Joiner, studs: int(j.system.JoinerHalf * 2 / 2),
				rot: jrot, center: at(j.center),
				shaft: l.shaft,
				label: fmt.Sprintf("joiner for shaft '%s', the ring rides on this",
					l.shaft),
			})
		}

		for _, seg := range shaftSegments(lo, hi, joiners) {
			studs, name, ok := seg.axle()
			if !ok {
				res.Findings = append(res.Findings, mech.Finding{
					Level: "WARN", Check: "parts", Detail: fmt.Sprintf(
						"shaft '%s' needs a length between %.0f and %.0f LDU, which no "+
							"single axle gives; join two", l.shaft, seg.min, seg.max)})
				continue
			}
			length := float64(studs) * geom.Stud
			center := seg.centerFor(length)
			axles = append(axles, axlePlacement{
				name: name, studs: studs, rot: rot, center: at(center),
				shaft: l.shaft,
				label: fmt.Sprintf("axle %d for shaft '%s'", studs, l.shaft),
			})
		}

		// One shaft as far as the structure is concerned, however many parts
		// draw it: a joiner joins, so the bearings at either end are still tied
		// together by it.
		mid := (lo + hi) / 2
		res.Axles = append(res.Axles, rigidity.Axle{
			Point: at(mid), Dir: l.place.Direction,
			From: -(hi - lo) / 2, To: (hi - lo) / 2,
		})
	}
	return axles
}

// turningCells is the space every part that turns occupies, as voxels: the
// gears, the driving rings, and the joiners under them.
//
// A beam may not enter any of it. Keeping bearings off the length of shaft a
// part occupies is not enough on its own, and that trap was fallen into twice:
// a beam bridging two lines crosses a shaft wherever it likes, bearing nothing,
// and a beam bearing one shaft reaches a gear on another.
//
// The cells come from the same rasteriser the candidates do, and are shifted
// the same way. That matters more than it sounds: a cylinder worked out by hand
// rounds differently at its edges, and a bearing whose face meets the end of
// the ring's travel — which is exactly right — comes out sharing a cell with it
// and is thrown away.
func turningCells(res *Result, deps Deps) map[geom.Cell]bool {
	if deps.Rast == nil {
		return nil
	}
	out := map[geom.Cell]bool{}

	// Every gear, at its station. Leaving these out let beams be placed through
	// them: keeping bearings off the length of shaft a gear occupies is not the
	// same as keeping a beam's body out of the gear, and a beam bearing one
	// shaft reaches far enough to strike a gear on another.
	for _, st := range res.Stations {
		place, ok := res.Layout.Place[st.Shaft]
		if !ok {
			continue
		}
		name, ok := gearAt(st, res.ringSites, res.slip)
		if !ok {
			continue
		}
		rot, ok := rotationIndex(alignZTo(place.Direction))
		if !ok {
			continue
		}
		at := place.Point.Scale(synth.HalfStud).
			Add(place.Direction.Scale(st.Axial * synth.HalfStud))
		one := map[geom.Cell]bool{}
		markCells(one, deps, name, rot, at)
		for c := range erode(fill(one, across(place.Direction))) {
			out[c] = true
		}
	}

	for _, site := range res.ringSites {
		place, ok := res.Layout.Place[site.station.Shaft]
		if !ok {
			continue
		}
		rot, ok := rotationIndex(alignZTo(place.Direction))
		if !ok {
			continue
		}
		origin := place.Point.Scale(synth.HalfStud)
		at := func(halfStuds float64) geom.Vec3 {
			return origin.Add(place.Direction.Scale(halfStuds * synth.HalfStud))
		}
		one := map[geom.Cell]bool{}
		// Both ends of the travel and the middle, which is the whole of the
		// space the ring passes through on a shift.
		for _, p := range []geom.Vec3{at(site.engaged),
			at((site.engaged + site.disengaged) / 2), at(site.disengaged)} {
			markCells(one, deps, site.system.Ring, rot, p)
		}
		markCells(one, deps, site.system.Joiner, rot, at(site.joiner))

		for c := range erode(fill(one, across(place.Direction))) {
			out[c] = true
		}
	}
	return out
}

// markCells adds a part's voxels, placed, to a set.
//
// Shifted by rounding rather than flooring: this is how the searcher shifts a
// candidate's cells, and the two have to agree or the reservation lands a cell
// away from the part it is meant to protect.
func markCells(into map[geom.Cell]bool, deps Deps, part string, rot int,
	at geom.Vec3) {

	cells, err := deps.Rast.Voxels(part, rot)
	if err != nil {
		return
	}
	shift := geom.Cell{
		X: int32(math.Round(at.X / geom.VoxelPitch)),
		Y: int32(math.Round(at.Y / geom.VoxelPitch)),
		Z: int32(math.Round(at.Z / geom.VoxelPitch)),
	}
	for _, c := range cells {
		into[c.Add(shift)] = true
	}
}

// across picks an axis to fill along: any one but the shaft's own, since
// filling along the shaft would close the gaps between the rings rather than
// the bore through them.
func across(dir geom.Vec3) int {
	if math.Abs(dir.X) < 0.5 {
		return 0
	}
	return 2
}

// fill closes the bore.
//
// The rasteriser marks material, and a driving ring is a tube: what comes back
// is a shell with a hole down the middle. Eroding that leaves almost nothing.
// Nothing may pass down the bore either — that is where the shaft is — so the
// volume to reserve is the solid rod and not the plastic.
//
// Filled a row at a time rather than out to the bounding box, so a round part
// stays round and a beam may still pass the corner it does not occupy.
func fill(cells map[geom.Cell]bool, axis int) map[geom.Cell]bool {
	type row struct{ a, b int32 }
	along := func(c geom.Cell) (row, int32) {
		switch axis {
		case 0:
			return row{c.Y, c.Z}, c.X
		case 1:
			return row{c.X, c.Z}, c.Y
		default:
			return row{c.X, c.Y}, c.Z
		}
	}
	rebuild := func(k row, v int32) geom.Cell {
		switch axis {
		case 0:
			return geom.Cell{X: v, Y: k.a, Z: k.b}
		case 1:
			return geom.Cell{X: k.a, Y: v, Z: k.b}
		default:
			return geom.Cell{X: k.a, Y: k.b, Z: v}
		}
	}

	lo, hi := map[row]int32{}, map[row]int32{}
	for c := range cells {
		k, v := along(c)
		if got, ok := lo[k]; !ok || v < got {
			lo[k] = v
		}
		if got, ok := hi[k]; !ok || v > got {
			hi[k] = v
		}
	}
	out := make(map[geom.Cell]bool, len(cells))
	for k, from := range lo {
		for v := from; v <= hi[k]; v++ {
			out[rebuild(k, v)] = true
		}
	}
	return out
}

// erode drops the outermost cell of a reserved volume.
//
// The rasteriser marks every cell a part so much as touches, so two parts laid
// face to face share a cell-thick skin without overlapping at all. That is why
// the ordinary overlap rule tolerates contact. A reservation admits none, so it
// has to be the interior: a beam resting against the end of a ring's travel is
// right, and only one that reaches past the skin is inside it.
func erode(cells map[geom.Cell]bool) map[geom.Cell]bool {
	out := make(map[geom.Cell]bool, len(cells))
	for c := range cells {
		inside := true
		for _, n := range [6]geom.Cell{
			{X: c.X + 1, Y: c.Y, Z: c.Z}, {X: c.X - 1, Y: c.Y, Z: c.Z},
			{X: c.X, Y: c.Y + 1, Z: c.Z}, {X: c.X, Y: c.Y - 1, Z: c.Z},
			{X: c.X, Y: c.Y, Z: c.Z + 1}, {X: c.X, Y: c.Y, Z: c.Z - 1},
		} {
			if !cells[n] {
				inside = false
				break
			}
		}
		if inside {
			out[c] = true
		}
	}
	return out
}

// rotationIndex finds a lattice rotation by its matrix, which is what the
// rasteriser is keyed on.
func rotationIndex(m geom.Mat3, ok bool) (int, bool) {
	if !ok {
		return 0, false
	}
	for i, r := range geom.Rotations {
		if r == m {
			return i, true
		}
	}
	return 0, false
}

// bearingHalf is half a beam's thickness, in half studs.
const bearingHalf = 1.0

// ringSpans is the shaft each driving ring and its joiner take up, in half
// studs, for every shaft on the same line.
//
// By line and not by shaft: two shafts coupled together are the same piece of
// axle, so a ring on one of them blocks the other just as surely.
func ringSpans(res *Result) map[string][][2]float64 {
	if len(res.ringSites) == 0 {
		return nil
	}
	out := map[string][][2]float64{}
	for _, site := range res.ringSites {
		place, ok := res.Layout.Place[site.station.Shaft]
		if !ok {
			continue
		}
		// Widened by a beam's own half thickness. A bearing is a point, but the
		// beam that provides it is a stud thick, so one placed just outside the
		// joiner still has material inside it.
		lo := math.Min(site.engaged-site.system.RingHalf,
			site.joiner-site.system.JoinerHalf) - bearingHalf
		hi := math.Max(site.disengaged+site.system.RingHalf,
			site.joiner+site.system.JoinerHalf) + bearingHalf
		for id, p := range res.Layout.Place {
			if p.Key() == place.Key() {
				out[id] = append(out[id], [2]float64{lo, hi})
			}
		}
	}
	return out
}

// joinersOn is where a line is cut, in LDU along it, in order.
//
// The joiner is centered on the middle of the ring's travel rather than on
// either end of it, so the ring stays as far onto it as it can in both
// positions.
func joinersOn(res *Result, place layout.Placement) []joinerAt {
	var out []joinerAt
	for _, site := range res.ringSites {
		p, ok := res.Layout.Place[site.station.Shaft]
		if !ok || p.Key() != place.Key() {
			continue
		}
		out = append(out, joinerAt{
			center: site.joiner * synth.HalfStud,
			system: site.system,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].center < out[j].center })
	return out
}

// joinerAt is a cut in a shaft: where it is, and which generation of hardware
// made it. The two joiners are different lengths, so an axle butting into one
// has a different length to reach for.
type joinerAt struct {
	center float64
	system clutch.System
}

// segment is a length of shaft between two fixed points, and what length of
// axle can take it up.
//
// The bounds are what make a cut shaft work. An axle butting into a joiner has
// to reach its middle stop without trying to pass it, so its end has to land
// inside the joiner's near half.
//
// The two kinds of segment want opposite things of that. A stretch between two
// joiners is bounded at both ends and takes the longest axle that fits between
// them. A stretch running out to the end of the shaft is bounded only at the
// joiner: it takes the shortest axle that reaches, seated hard against the
// stop, and if that leaves it standing a little proud of the last bearing then
// it stands proud, which is ordinary. Requiring it to end exactly at the far
// end was what made a shaft needing "between 270 and 290 LDU" impossible —
// axles come in 240 and 320 and nothing between.
type segment struct {
	min, max float64
	// outer, when set, pins the far end: an axle that has to reach a bearing
	// cannot be centered on its gap.
	outer   float64
	pinned  bool
	towards float64 // +1 if the axle runs up from outer, -1 if down
	// inner is where the axle's far end seats, when pinned: the stop in the
	// joiner it butts into.
	inner float64
}

func shaftSegments(lo, hi float64, joiners []joinerAt) []segment {
	if len(joiners) == 0 {
		span := hi - lo
		return []segment{{min: span, max: math.Inf(1), outer: (lo + hi) / 2}}
	}
	reach := func(j joinerAt) float64 { return j.system.JoinerReach * synth.HalfStud }

	var out []segment
	first := joiners[0]
	out = append(out, segment{
		min: first.center - lo, max: math.Inf(1),
		outer: lo, pinned: true, towards: 1, inner: first.center,
	})
	for i := 1; i < len(joiners); i++ {
		a, b := joiners[i-1], joiners[i]
		gap := b.center - a.center
		out = append(out, segment{
			min: gap - reach(a) - reach(b), max: gap,
			outer: (a.center + b.center) / 2,
		})
	}
	last := joiners[len(joiners)-1]
	out = append(out, segment{
		min: hi - last.center, max: math.Inf(1),
		outer: hi, pinned: true, towards: -1, inner: last.center,
	})
	_ = reach
	return out
}

// axle picks the length for a segment.
//
// Bounded at both ends: the longest that fits, so it reaches as far into each
// joiner as it can. Bounded at one: the shortest that reaches.
func (s segment) axle() (int, string, bool) {
	if math.IsInf(s.max, 1) {
		return axleFor(s.min)
	}
	for i := len(axleLengths) - 1; i >= 0; i-- {
		length := float64(axleLengths[i]) * geom.Stud
		if length <= s.max+1e-6 && length >= s.min-1e-6 {
			return axleLengths[i], AxleParts[axleLengths[i]], true
		}
	}
	return 0, "", false
}

// centerFor places an axle of a given length within its segment.
//
// Seated against the stop in its joiner, when there is one, rather than aligned
// with the far end: the end that has to be right is the one inside the joiner.
func (s segment) centerFor(length float64) float64 {
	if !s.pinned {
		return s.outer
	}
	return s.inner - s.towards*length/2
}

// axleFor is the shortest axle that spans a length in LDU.
func axleFor(spanLDU float64) (int, string, bool) {
	for _, studs := range axleLengths {
		if float64(studs)*geom.Stud >= spanLDU-1e-6 {
			return studs, AxleParts[studs], true
		}
	}
	return 0, "", false
}

func lessKey(a, b [6]float64) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// alignXTo finds a lattice rotation taking an axle's own axis, its X, onto a
// shaft direction.
func alignXTo(dir geom.Vec3) (geom.Mat3, bool) {
	d := dir.Unit()
	for _, m := range geom.Rotations {
		if m.Apply(geom.Vec3{X: 1}).Sub(d).Len() < 1e-9 {
			return m, true
		}
	}
	return geom.Mat3{}, false
}

// alignZTo finds a lattice rotation taking a gear's own axis, its local Z, onto
// the shaft direction.
func alignZTo(dir geom.Vec3) (geom.Mat3, bool) {
	d := dir.Unit()
	for _, m := range geom.Rotations {
		if m.Apply(geom.Vec3{Z: 1}).Sub(d).Len() < 1e-9 {
			return m, true
		}
	}
	return geom.Mat3{}, false
}

// Placeable is every part the engine can put into a model.
//
// It exists because "which parts does the browser need" was being answered by
// tier, and tier is a judgement about how common a part is, not about what this
// engine places. They disagreed: gears are titled "Technic Gear ..." and so
// grade as tier 2, the site shipped tier 1, and every model it built came out
// with no gear geometry at all. Nothing said so — a part with no triangles is
// skipped by the clearance sweep and skipped by the renderer, so the page drew
// gearboxes without gears and reported that nothing collided, having never
// looked. See docs/findings.md.
//
// 3647 and 32270 have no shadow file, so they carry no ports and the extractor
// drops them however high the tier goes. That is the second half of the same
// problem: what the engine places and what the shadow library describes are
// different sets, and the assets have to cover the union.
func Placeable() []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	for _, n := range GearParts {
		add(n)
	}
	for _, n := range AxleParts {
		add(n)
	}
	for _, b := range part.Beams {
		add(b.Part)
	}
	for _, s := range clutch.Systems {
		add(s.Ring)
		add(s.Joiner)
		for _, n := range s.Gears {
			add(n)
		}
	}
	// The fasteners. Easy to forget, and forgetting them is the same bug as
	// before: no geometry in the browser, so the clearance sweep skips them and
	// the renderer leaves them out, and nothing says so.
	add(PinPart)
	add(AxlePinPart)
	add(LongPinPart)
	add(MarkerPart)
	add(DiffPart)
	add(SlipPart)
	for _, sys := range clutch.Systems {
		add(sys.Catch)
	}
	sort.Strings(out)
	return out
}

// differentialOn reports the differential whose shafts run down this line.
func differentialOn(m *mech.Mechanism, res *Result, place layout.Placement) (mech.Differential, bool) {
	for _, link := range m.Links {
		d, ok := link.(mech.Differential)
		if !ok {
			continue
		}
		p, ok := res.Layout.Place[d.Case]
		if ok && p.Key() == place.Key() {
			return d, true
		}
	}
	return mech.Differential{}, false
}

// differentialAxles places the housing and the two shafts that enter it.
//
// The housing sits at the line's origin and belongs to the case. Either side of
// it an axle runs out to its bearing, one for each output, butting against the
// housing's face rather than passing through — which is the whole point of the
// part.
func differentialAxles(res *Result, place layout.Placement, d mech.Differential,
	lo, hi float64) []axlePlacement {

	origin := place.Point.Scale(synth.HalfStud)
	at := func(v float64) geom.Vec3 { return origin.Add(place.Direction.Scale(v)) }

	zrot, ok := alignZTo(place.Direction)
	if !ok {
		return nil
	}
	xrot, ok := alignXTo(place.Direction)
	if !ok {
		return nil
	}

	out := []axlePlacement{{
		name: DiffPart, studs: int(DiffHalf * 2 / geom.Stud), rot: zrot, center: at(0),
		shaft: d.Case,
		label: fmt.Sprintf("differential for shaft '%s'", d.Case),
	}}
	res.Findings = append(res.Findings, mech.Finding{
		Level: "OK", Check: "parts", Detail: fmt.Sprintf(
			"the differential on '%s' is %s: the casing with its drive gear and "+
				"all five bevel satellites in it, two side gears on the outputs "+
				"and three planets between them. One part, so there is nothing to "+
				"add by hand",
			d.Case, DiffPart)})

	// One each side, outward from the housing's face to a stud past the end.
	for _, side := range []struct {
		shaft   string
		outer   float64
		towards float64
	}{
		{d.OutA, lo, 1},
		{d.OutB, hi, -1},
	} {
		// The face this axle butts against is the one on its own side, so the
		// sign is opposite to the direction it runs in: an axle coming from the
		// low end runs towards +1 and stops at -DiffHalf.
		inner := -side.towards * DiffHalf
		seg := segment{
			min: math.Abs(inner - side.outer), max: math.Inf(1),
			outer: side.outer, pinned: true, towards: side.towards, inner: inner,
		}
		studs, name, ok := seg.axle()
		if !ok {
			res.Findings = append(res.Findings, mech.Finding{
				Level: "WARN", Check: "parts", Detail: fmt.Sprintf(
					"shaft '%s' needs a length of about %.0f LDU out of the "+
						"differential, which no single axle gives",
					side.shaft, math.Abs(inner-side.outer))})
			continue
		}
		out = append(out, axlePlacement{
			name: name, studs: studs, rot: xrot,
			center: at(seg.centerFor(float64(studs) * geom.Stud)),
			shaft:  side.shaft,
			label:  fmt.Sprintf("axle %d for shaft '%s'", studs, side.shaft),
		})
	}
	return out
}

// slipShafts indexes the shafts a torque limiter is fitted to.
func slipShafts(m *mech.Mechanism) map[string]bool {
	out := map[string]bool{}
	for _, c := range m.SlipClutches() {
		out[c.Shaft] = true
	}
	return out
}

// checkSlipClutches says what each torque limiter protects, and refuses one
// that has nowhere to sit.
//
// The part is made in 24 teeth and no other size, so a slip clutch on a shaft
// with no 24-tooth gear is not a thing that can be built. Saying so beats
// placing a plain gear and leaving the reader to notice that nothing slips.
func checkSlipClutches(m *mech.Mechanism, res *Result) {
	for _, c := range m.SlipClutches() {
		found := false
		for _, st := range res.Stations {
			if st.Shaft == c.Shaft && st.Teeth == 24 {
				found = true
				break
			}
		}
		if !found {
			res.Findings = append(res.Findings, mech.Finding{
				Level: "FAIL", Check: "slip clutch", Detail: fmt.Sprintf(
					"a slip clutch is fitted to '%s' and there is no 24-tooth gear "+
						"on it. %s is made in that size and no other, so it has "+
						"nowhere to sit: put a 24t on that shaft, or move the "+
						"clutch to a shaft that has one", c.Shaft, SlipPart)})
			continue
		}
		at, source := c.AtNcm, "given"
		if at == 0 {
			at, source = torque.SlipLimitNcm()
		}
		res.Findings = append(res.Findings, mech.Finding{
			Level: "OK", Check: "slip clutch", Detail: fmt.Sprintf(
				"%s on '%s' gives way at %.0f Ncm (%s), so nothing downstream of "+
					"it is loaded harder than that however hard the input is "+
					"driven. It is the other kind of clutch: a friction centre "+
					"that slips, not dogs that grip",
				SlipPart, c.Shaft, at, source)})
	}
}

// placeSelector puts the catch that moves a ring beside it.
//
// The shift stops being an instruction to the builder and becomes part of the
// model. How far out and which way round are read from official models rather
// than searched for — 8448 for the first generation, 42110 and 42083 for the
// second — because a sweep can confirm a catch's placement but not find one:
// in LDraw a fork that straddles a groove touches it at some angle whatever
// you do.
//
// Which perpendicular is a choice, and the only one the engine can make on its
// own: whichever direction is clear of the other shafts. A catch pointing into
// the neighbouring gear train is worse than none.
func placeSelector(res *Result, model *ldr.Model, site ringSite,
	place layout.Placement, at geom.Vec3, label string) (geom.Vec3, geom.Mat3, bool) {

	if site.catchRot == (geom.Mat3{}) {
		return geom.Vec3{}, geom.Mat3{}, false // settleCatches found nowhere for it
	}
	model.Add(site.system.Catch, ldr.ColorBlack, site.catchRot,
		at.Add(site.catchAt), "catch for "+label)
	return site.catchAt, site.catchRot, true
}

// settleCatches works out where every catch goes, before anything is drawn.
//
// Early because the structural search has to know: the axle a catch turns on is
// a line the frame must bear, and the search runs long before the model does.
// Nothing here depends on the structure — a catch's place follows from the ring
// and from which way out is clear of the other shafts, both of which the layout
// settles — so there is nothing to wait for.
func settleCatches(ctx context.Context, deps Deps, res *Result) {
	for i := range res.ringSites {
		site := &res.ringSites[i]
		if site.system.Catch == "" {
			continue // nothing knows what moves this generation
		}
		place, ok := res.Layout.Place[site.station.Shaft]
		if !ok {
			continue
		}
		at := place.Point.Scale(synth.HalfStud).
			Add(place.Direction.Unit().Scale(site.engaged * synth.HalfStud))
		d := place.Direction.Unit()
		out, ok, why := clearOfOtherShafts(ctx, deps, res, place, at, d, site.system)
		if !ok {
			res.Findings = append(res.Findings, mech.Finding{
				Level: "WARN", Check: "parts", Detail: fmt.Sprintf(
					"no room beside the ring for %s on '%s': %s. The shift is named "+
						"rather than placed for this one",
					site.system.Catch, site.rides, why)})
			continue
		}
		site.catchRot = catchFrame(site.system, d, out)
		site.catchAt = out.Scale(site.system.CatchReach)
	}
}

// catchFrame turns the catch to face the shaft.
//
// Two of its axes are pinned by the system: one along the shaft, one out from
// it. The third is whatever makes the result a rotation rather than a
// reflection — get its sign wrong and the part goes in mirrored, which LDraw
// will render without complaint and no builder can assemble.
func catchFrame(s clutch.System, d, out geom.Vec3) geom.Mat3 {
	along, away := int(s.CatchAlong-'x'), int(s.CatchOut-'x')
	third := 3 - along - away
	var col [3]geom.Vec3
	col[along], col[away] = d, out
	// col[along] x col[away] is +col[third] when (along, away, third) is an
	// even permutation and -col[third] when it is odd.
	col[third] = d.Cross(out)
	if (along+1)%3 != away {
		col[third] = col[third].Scale(-1)
	}
	return geom.Mat3{
		{col[0].X, col[1].X, col[2].X},
		{col[0].Y, col[1].Y, col[2].Y},
		{col[0].Z, col[1].Z, col[2].Z},
	}
}

// clearOfOtherShafts picks a way out of a shaft that nothing is already in.
//
// Other shafts were all it looked at, which is right for a mechanism built in
// open air and wrong for one fitted into somebody's model: a two-speed put into
// 42110 sent its catch straight through a beam and a panel, on the side the
// Land Rover was densest, because no other shaft happened to be that way.
func clearOfOtherShafts(ctx context.Context, deps Deps, res *Result,
	place layout.Placement, at, d geom.Vec3, sys clutch.System) (geom.Vec3, bool, string) {

	reach := sys.CatchReach
	var across []geom.Vec3
	for _, c := range []geom.Vec3{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}, {Z: 1}, {Z: -1}} {
		if math.Abs(c.Dot(d)) < 1e-6 {
			across = append(across, c)
		}
	}
	shafts, model := 0, 0
	for _, out := range across {
		blocked := false
		for _, other := range res.Layout.Place {
			if other.Key() == place.Key() {
				continue
			}
			// Is the other shaft's line within reach, that way?
			v := other.Point.Scale(synth.HalfStud).Sub(at)
			side := v.Sub(d.Scale(v.Dot(d)))
			if side.Dot(out) > 0 && side.Len() < reach+geom.Stud {
				blocked = true
				break
			}
		}
		if blocked {
			shafts++
			continue
		}
		if catchIsInTheModel(ctx, deps, res, sys, at, d, out) {
			model++
			continue
		}
		return out, true, ""
	}
	// Which of the two closed it off is worth saying: another shaft is the
	// mechanism's own doing and can be laid out around, while the model is
	// somebody else's and cannot.
	switch {
	case model > 0 && shafts > 0:
		return geom.Vec3{}, false, fmt.Sprintf(
			"%d way(s) out run into another shaft and %d into the model "+
				"it was fitted to", shafts, model)
	case model > 0:
		return geom.Vec3{}, false, "every way out of the shaft is inside the " +
			"model it was fitted to"
	default:
		return geom.Vec3{}, false, "every way out of the shaft runs into another one"
	}
}

// catchIsInTheModel reports whether a catch this way out would be inside the
// model the mechanism was fitted into.
//
// Only meaningful after a fit; without one there is no model to be inside and
// the answer is no.
//
// The cells narrow it down and the exact test answers it. Asking the cells
// alone — is any one of them somebody else's — makes resting against a beam
// indistinguishable from being buried in it: an 18947 shares 127 of its 466
// cells with a 40490 it is merely touching, and 129 with one it is 18 LDU
// inside. See docs/findings.md.
func catchIsInTheModel(ctx context.Context, deps Deps, res *Result,
	sys clutch.System, at, d, out geom.Vec3) bool {

	if res.intoRast == nil || len(res.intoOccupied) == 0 {
		return false
	}
	rot := catchFrame(sys, d, out)
	cells, err := res.intoRast.VoxelsAt(sys.Catch, rot)
	if err != nil {
		return false // nothing to say, rather than a wrong no
	}
	pos := at.Add(out.Scale(sys.CatchReach))
	shift := geom.Cell{
		X: int32(math.Round(pos.X / geom.VoxelPitch)),
		Y: int32(math.Round(pos.Y / geom.VoxelPitch)),
		Z: int32(math.Round(pos.Z / geom.VoxelPitch)),
	}
	touches := false
	for _, c := range cells {
		if res.intoOccupied[c.Add(shift)] {
			touches = true
			break
		}
	}
	if !touches {
		return false // nowhere near anything, and no mesh check needed
	}
	return !clearOf(ctx, deps, ldr.Part{Name: sys.Catch, Rot: rot, Pos: pos},
		nearbyParts(deps, res.into, pos, sys.CatchReach))
}

// nearbyParts is the model's parts whose boxes come within reach of a point.
func nearbyParts(deps Deps, into []ldr.Placed, at geom.Vec3, reach float64) []ldr.Part {
	var near []ldr.Part
	for _, p := range into {
		q := ldr.Part{Name: p.Name, Color: p.Color, Rot: p.Rot, Pos: p.Pos}
		lo, hi, err := placedBox(deps, q)
		if err != nil {
			continue
		}
		if overlapOf(lo, hi, at.Sub(geom.Vec3{X: reach, Y: reach, Z: reach}),
			at.Add(geom.Vec3{X: reach, Y: reach, Z: reach})) > 0 {
			near = append(near, q)
		}
	}
	return near
}
