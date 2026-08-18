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

	"brickmesh/internal/clutch"
	"brickmesh/internal/geom"
	"brickmesh/internal/layout"
	"brickmesh/internal/ldcad"
	"brickmesh/internal/ldr"
	"brickmesh/internal/ldraw"
	"brickmesh/internal/mech"
	"brickmesh/internal/part"
	"brickmesh/internal/progress"
	"brickmesh/internal/rigidity"
	"brickmesh/internal/shadow"
	"brickmesh/internal/synth"
	"brickmesh/internal/voxel"
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

// SelectorParts are what moves a driving ring. Their position follows from the
// shift linkage, which the engine does not model, so they are named rather than
// placed.
var SelectorParts = []string{
	"6641 Technic Transmission Changeover Catch",
	"6631 Technic Plate 2 x 6 with 2 Position Gear Shift",
}

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
	Lib    *ldraw.Library
	Shadow *shadow.Library
	Rast   *voxel.Rasterizer
}

// Options tunes the stages.
type Options struct {
	MaxLayouts    int
	Span          int
	Restarts      int
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

	stations, stationFindings := layout.SolveStations(m, res.Layout)
	res.Stations = stations
	res.Findings = append(res.Findings, stationFindings...)

	// Where the rings go decides where the shafts are cut, so it is settled
	// before the axles rather than when the model is drawn.
	checkFraming(res)
	res.ringSites = ringSites(m, res)
	// Worked out before the structural search, because the rigidity check needs
	// to know the shafts are there: they are what ties the bearings together.
	res.axles = computeAxles(m, res)

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
	model, err := buildModel(m, res)
	if err != nil {
		return res, err
	}
	res.Model = model

	opts.Progress.Report(progress.Report{Stage: progress.StagePhase})
	if err := applyPhase(ctx, m, res, deps); err != nil {
		return res, err
	}
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
func firstThatFits(ctx context.Context, solutions []synth.Solution,
	deps Deps) (*synth.Solution, int) {

	for i := range solutions {
		if !overlapping(ctx, solutions[i].Parts, deps) {
			return &solutions[i], i
		}
	}
	return nil, len(solutions)
}

func overlapping(ctx context.Context, parts []synth.Placed, deps Deps) bool {
	for a := 0; a < len(parts); a++ {
		for b := a + 1; b < len(parts); b++ {
			inside, _, err := sharesSpace(ctx, deps, lattice(parts[a]), lattice(parts[b]))
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
	solutions, err := searcher.Synthesize(ctx, res.Layout, res.Stations, synth.Options{
		Restarts: opts.Restarts, Seed: opts.Seed, Progress: opts.Progress,
	})
	if errors.Is(err, context.Canceled) {
		return err // stopped is not a failure of the search, and says so itself
	}
	if err != nil {
		return fmt.Errorf("the structural search: %w", err)
	}
	if len(solutions) == 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "WARN", Check: "structure",
			Detail: "no structure found that bears every shaft; the gears are placed but nothing holds them",
		})
		return nil
	}
	// The search works on a voxel lattice, which has to tolerate parts touching
	// — the rasteriser marks every cell a part so much as brushes — and that
	// tolerance lets two beams overlap by a little. So the candidates are put
	// to the exact question before one is taken. The search proposes; the
	// geometry disposes.
	chosen, rejected := firstThatFits(ctx, solutions, deps)
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
				"%d structure(s) rejected for parts overlapping before this one",
				rejected)})
	}
	// Stiffening happens here rather than inside the search: it is the one
	// solution that will be used, and doing it to every restart costs minutes
	// for an answer that is thrown away sixty times over.
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
func buildModel(m *mech.Mechanism, res *Result) (*ldr.Model, error) {
	model := ldr.New(m.Name)

	stations := append([]layout.Station(nil), res.Stations...)
	sort.SliceStable(stations, func(i, j int) bool {
		if stations[i].Shaft != stations[j].Shaft {
			return stations[i].Shaft < stations[j].Shaft
		}
		return stations[i].Axial < stations[j].Axial
	})

	sites := res.ringSites
	for _, st := range stations {
		name, ok := gearAt(st, sites)
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
}

// ringSites works out where every shift's driving ring goes.
//
// Separated from placing them because the gears have to know too: a gear a ring
// engages is not the plain gear, it is the clutch variant, and buildModel picks
// the part before any ring is placed.
func ringSites(m *mech.Mechanism, res *Result) []ringSite {
	var out []ringSite
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

// gearAt names the part for a station, using the clutch variant where a ring
// engages it.
func gearAt(st layout.Station, sites []ringSite) (string, bool) {
	for _, site := range sites {
		if site.station.Shaft != st.Shaft || site.station.Axial != st.Axial {
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
	var nominal []string
	for _, site := range sites {
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

		if _, ok := site.system.Gears[site.station.Teeth]; !ok {
			nominal = append(nominal, fmt.Sprintf("%dt on '%s'",
				site.station.Teeth, site.station.Shaft))
		}
	}

	if len(sites) == 0 {
		return
	}
	res.Findings = append(res.Findings, mech.Finding{
		Level: "OK", Check: "parts", Detail: fmt.Sprintf(
			"%d driving ring(s) placed, each three half studs from its gear's "+
				"center so its dogs sit in the recesses. What moves them is not "+
				"placed: %v. The catch's hold on a ring is a fit, and the sweep that "+
				"settles whether gears mesh cannot settle a fit: in LDraw a spline "+
				"that grips reads as a spline that collides. See docs/shifting.md. "+
				"Two rings beside the same gear could be one ring engaging either "+
				"side.",
			len(sites), SelectorParts)})

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
		name, ok := gearAt(st, res.ringSites)
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
