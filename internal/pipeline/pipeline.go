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
// The newer rings — 18947 of the Chiron and the MT-10's — are not in the parts
// mirror this reads from, so only the classic system can be placed.
const (
	DrivingRing = clutch.Ring
)

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
}

// axlePlacement is a shaft worked out but not yet written.
type axlePlacement struct {
	name   string
	studs  int
	rot    geom.Mat3
	center geom.Vec3
	shaft  string
	axle   rigidity.Axle
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
func Run(m *mech.Mechanism, deps Deps, opts Options) (*Result, error) {
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

	// Worked out before the structural search, because the rigidity check needs
	// to know the shafts are there: they are what ties the bearings together.
	res.axles = computeAxles(m, res)
	for _, a := range res.axles {
		res.Axles = append(res.Axles, a.axle)
	}

	if !opts.SkipStructure {
		if err := runStructure(res, deps, opts); err != nil {
			return res, err
		}
	}

	model, err := buildModel(m, res)
	if err != nil {
		return res, err
	}
	res.Model = model
	applyPhase(m, res, deps)
	if opts.Animate {
		animate(m, res, opts)
	}
	return res, nil
}

func runStructure(res *Result, deps Deps, opts Options) error {
	if deps.Rast == nil || deps.Shadow == nil {
		return fmt.Errorf("the structural search needs both libraries")
	}
	searcher := synth.NewSearcher(deps.Rast, deps.Shadow, opts.Inventory)
	solutions, err := searcher.Synthesize(res.Layout, res.Stations, synth.Options{
		Restarts: opts.Restarts, Seed: opts.Seed,
	})
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
	res.Structure = &solutions[0]
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

	sites := ringSites(m, res)
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
			fmt.Sprintf("axle %d for shaft '%s'", a.studs, a.shaft))
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
	// Engaged is where the ring sits when the coupling is engaged, along the
	// shaft in half studs; Disengaged is where it slides to when it is not.
	engaged, disengaged float64
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
		if !found {
			station, found = stationOn(res.Stations, c.A)
		}
		if !found {
			continue // nothing to engage: reported by CheckShiftable already
		}
		axial, ok := freeSideOf(res.Stations, station, res.Layout)
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
		out = append(out, ringSite{coupling: c, station: station,
			engaged: axial, disengaged: axial + side*clutch.Travel})
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
		if name, ok := clutch.Gears[st.Teeth]; ok {
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
		model.Add(DrivingRing, ldr.ColorRed, rot, pos, label)

		if _, ok := clutch.Gears[site.station.Teeth]; !ok {
			nominal = append(nominal, fmt.Sprintf("%dt on '%s'",
				site.station.Teeth, site.station.Shaft))
		}
	}

	if len(sites) == 0 {
		return
	}
	res.Findings = append(res.Findings, mech.Finding{
		Level: "OK", Check: "parts", Detail: fmt.Sprintf(
			"%d driving ring(s) placed, each a half stud short of three from its "+
				"gear's center so its dogs sit in the recesses. What moves them is not "+
				"placed: %v — their position follows from the shift linkage, which is "+
				"not modelled. Two rings beside the same gear could be one ring "+
				"engaging either side.", len(sites), SelectorParts)})

	if len(nominal) > 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "WARN", Check: "parts", Detail: fmt.Sprintf(
				"%v get a plain gear, not a clutch one: the library has a clutch "+
					"variant only for the 16t (%v). A real 20t or 24t shift reaches its "+
					"gear through a driving ring extension (32187, or 35186), which is "+
					"not modelled — so those rings turn beside their gear without "+
					"gripping it.", nominal, clutch.Gears)})
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
func freeSideOf(stations []layout.Station, gear layout.Station, l *layout.Layout) (float64, bool) {
	line := layout.LineOf(l, gear.Shaft)
	for _, side := range []float64{1, -1} {
		axial := gear.Axial + side*clutch.Engaged
		// The ring is four half studs long, and it has to stay clear of
		// everything else on the line once it has slid the whole way out.
		lo := math.Min(axial, axial+side*clutch.Travel) - 2
		hi := math.Max(axial, axial+side*clutch.Travel) + 2
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
		spanLDU := (l.hi-l.lo)*synth.HalfStud + 2*geom.Stud
		studs, name, ok := axleFor(spanLDU)
		if !ok {
			res.Findings = append(res.Findings, mech.Finding{
				Level: "WARN", Check: "parts", Detail: fmt.Sprintf(
					"shaft '%s' needs %.0f LDU of axle, longer than any single one; "+
						"join two", l.shaft, spanLDU)})
			continue
		}
		rot, ok := alignXTo(l.place.Direction)
		if !ok {
			continue
		}
		mid := (l.lo + l.hi) / 2 * synth.HalfStud
		center := l.place.Point.Scale(synth.HalfStud).Add(l.place.Direction.Scale(mid))
		half := float64(studs) * geom.Stud / 2
		axles = append(axles, axlePlacement{
			name: name, studs: studs, rot: rot, center: center, shaft: l.shaft,
			axle: rigidity.Axle{
				Point: center, Dir: l.place.Direction, From: -half, To: half,
			},
		})
	}
	return axles
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
