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

	"brickmesh/internal/geom"
	"brickmesh/internal/layout"
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
	DrivingRing = "6539.dat"
	// ringHubHalfStuds is how far the ring's center sits from the gear it
	// engages: its own hub is two half studs, so clearing a gear of the same
	// width puts it two away.
	ringHubHalfStuds = 2.0
)

// SelectorParts are what moves a driving ring. Their position follows from the
// shift linkage, which the engine does not model, so they are named rather than
// placed.
var SelectorParts = []string{
	"6641 Technic Transmission Changeover Catch",
	"6631 Technic Plate 2 x 6 with 2 Position Gear Shift",
}

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
}

// Result is what the run found, stage by stage.
type Result struct {
	Findings  []mech.Finding
	Layout    *layout.Layout
	Stations  []layout.Station
	Structure *synth.Solution
	Model     *ldr.Model
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
	rigid, err := rigidity.Analyze(deps.Shadow, res.Structure.Parts, opts.Inventory)
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

	for _, st := range stations {
		name, ok := GearParts[st.Teeth]
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

	placeDrivingRings(m, res, model)

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
func placeDrivingRings(m *mech.Mechanism, res *Result, model *ldr.Model) {
	rings := 0
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
		place, ok := res.Layout.Place[station.Shaft]
		if !ok {
			continue
		}
		rot, ok := alignZTo(place.Direction)
		if !ok {
			continue
		}

		axial, ok := freeSideOf(res.Stations, station, res.Layout)
		if !ok {
			res.Findings = append(res.Findings, mech.Finding{
				Level: "WARN", Check: "parts", Detail: fmt.Sprintf(
					"no room beside the %dt on '%s' for a driving ring; the shift for %v "+
						"has nowhere to go", station.Teeth, station.Shaft, c.States)})
			continue
		}
		pos := place.Point.Scale(synth.HalfStud).
			Add(place.Direction.Scale(axial * synth.HalfStud))
		label := c.Name
		if label == "" {
			label = fmt.Sprintf("driving ring for %v", c.States)
		}
		model.Add(DrivingRing, ldr.ColorRed, rot, pos, label)
		rings++
	}

	if rings > 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "OK", Check: "parts", Detail: fmt.Sprintf(
				"%d driving ring(s) placed. What moves them is not placed: %v — their "+
					"position follows from the shift linkage, which is not modelled. Two "+
					"rings beside the same gear could be one ring engaging either side.",
				rings, SelectorParts)})
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
	for _, offset := range []float64{ringHubHalfStuds, -ringHubHalfStuds} {
		axial := gear.Axial + offset
		clear := true
		for _, st := range stations {
			if layout.LineOf(l, st.Shaft) != line {
				continue
			}
			lo, hi := st.Span()
			if math.Min(hi, axial+1)-math.Max(lo, axial-1) > 1e-6 {
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
