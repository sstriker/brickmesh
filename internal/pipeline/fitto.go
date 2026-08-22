// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"math"
	"sort"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/layout"
	"github.com/sstriker/brickmesh/internal/ldr"
	"github.com/sstriker/brickmesh/internal/mech"
	"github.com/sstriker/brickmesh/internal/synth"
	"github.com/sstriker/brickmesh/internal/voxel"
)

// Fit is a place a mechanism could go in a model that already exists.
//
// A layout is worked out around the origin and a chassis is wherever somebody
// built it, so fitting the two together is a question of what to move the
// layout by. Only a translation: turning the mechanism would be a different
// layout, and the layout search already offers those.
type Fit struct {
	Offset geom.Vec3
	// Borne is how many of the mechanism's shafts land on a line the model
	// already bears, out of Total.
	Borne, Total int
	// On names them, for saying which.
	On []string
	// Clashes is how many of the mechanism's gears would be inside the model
	// at this placement. A line the model bears is only half the question: the
	// other half is whether there is room along it.
	Clashes int
}

// fitTolerance is how close a line has to come to a bearing to count, in LDU.
//
// The same reasoning as readTolerance: a model somebody built is rounded and
// composed, and 42110's own chassis lines come out along {0 0.003 1} rather
// than {0 0 1}. Half an LDU is far under the two studs between lattice
// positions, so it cannot claim a fit that is not there.
const fitTolerance = 0.5

// slideStuds is how far along its own line a mechanism is slid looking for room,
// in studs either way.
//
// Ten is two shafts' worth of gearbox in each direction, which is enough to
// clear a wall and not so much that the search stops being quick.
const slideStuds = 10

// FitTo finds where a layout could sit in a model, best first.
//
// Every bearing the model offers is tried as the home of every shaft, which
// fixes a translation; the rest of the shafts are then counted. That is
// quadratic in a number that is small for a mechanism and large for a chassis —
// 42110 offers 767 usable lines — so the bearings are the outer loop and the
// shafts the inner one.
func FitTo(l *layout.Layout, bearings []Bearing, want int) []Fit {
	return FitToIn(l, bearings, nil, nil, want)
}

// FitToIn is FitTo with the model's own space taken into account.
//
// A placement that puts a gear where the bodywork is bears the shafts and
// cannot be built, so what it costs is counted rather than left to be
// discovered afterwards. Needs the stations, since a mechanism's gears are what
// take up room; the shafts themselves fit anywhere their line is clear.
func FitToIn(l *layout.Layout, bearings []Bearing, occupied map[geom.Cell]bool,
	solids []Solid, want int) []Fit {
	if l == nil || len(bearings) == 0 {
		return nil
	}
	shafts := l.Mech.Order()
	lines := map[[6]float64][]string{}
	for _, id := range shafts {
		p, ok := l.Place[id]
		if !ok {
			continue
		}
		lines[lineKey(p.Point.Scale(synth.HalfStud), p.Direction.Unit())] =
			append(lines[lineKey(p.Point.Scale(synth.HalfStud), p.Direction.Unit())], id)
	}

	seen := map[[3]float64]bool{}
	var out []Fit
	for _, b := range bearings {
		for k := range lines {
			at := geom.Vec3{X: k[0], Y: k[1], Z: k[2]}
			dir := geom.Vec3{X: k[3], Y: k[4], Z: k[5]}
			if math.Abs(math.Abs(dir.Dot(b.Axis.Unit()))-1) > 1e-3 {
				continue // a line can only sit on a bearing it is parallel to
			}
			// Move this line onto that bearing, across the axis only: sliding
			// along it changes nothing about which line it is.
			d := b.At.Sub(at)
			off := d.Sub(dir.Scale(d.Dot(dir)))
			// Exact, not rounded to the stud lattice. A mechanism's own gears
			// sit on the lattice with respect to EACH OTHER, and that is what
			// the layout settles; where the whole assembly then goes is
			// wherever the holes it bolts to are. 42110's chassis is turned
			// about three thousandths of a radian and its lines run through
			// points like {-200 -184.139 0.552}, so insisting on a lattice
			// offset put every one of its 767 bearing lines out of reach and
			// reported that a gearbox could not go anywhere in a Land Rover.
			// And slid along the line as well as across it. Which LINE a shaft
			// runs on does not depend on sliding along it, so the first version
			// dropped that component — but where the gears sit on the line does,
			// and every placement it offered put a gear inside the wall that
			// bears the shaft. The line is chosen across, the gears are cleared
			// along.
			for slide := -slideStuds; slide <= slideStuds; slide++ {
				at := off.Add(dir.Scale(float64(slide) * geom.Stud))
				key := [3]float64{
					math.Round(at.X*10) / 10,
					math.Round(at.Y*10) / 10,
					math.Round(at.Z*10) / 10,
				}
				if seen[key] {
					continue
				}
				seen[key] = true
				f := scoreFit(lines, bearings, at)
				f.Clashes = clashesAt(solids, occupied, at)
				out = append(out, f)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		// Room first, then support: a placement that cannot be built is not
		// better than one that needs a bearing adding, however well it lines up.
		if (out[i].Clashes == 0) != (out[j].Clashes == 0) {
			return out[i].Clashes == 0
		}
		if out[i].Borne != out[j].Borne {
			return out[i].Borne > out[j].Borne
		}
		if out[i].Clashes != out[j].Clashes {
			return out[i].Clashes < out[j].Clashes
		}
		return out[i].Offset.Len() < out[j].Offset.Len()
	})
	if want > 0 && len(out) > want {
		out = out[:want]
	}
	return out
}

// Solid is one of the mechanism's parts as cells, ready to be moved about.
//
// Rasterised once and then shifted, because the fitter tries a few thousand
// placements and rasterising a gear at each of them would be the whole cost of
// the exercise.
type Solid struct {
	Cells []geom.Cell
}

// SolidsOf rasterises a laid-out mechanism's gears where they stand.
func SolidsOf(l *layout.Layout, stations []layout.Station, rast *voxel.Rasterizer,
	sites []ringSite, slip map[string]bool) []Solid {

	if rast == nil || l == nil {
		return nil
	}
	var out []Solid
	for _, st := range stations {
		place, ok := l.Place[st.Shaft]
		if !ok {
			continue
		}
		name, ok := gearAt(st, sites, slip)
		if !ok {
			continue
		}
		rot, ok := rotationIndex(alignZTo(place.Direction))
		if !ok {
			continue
		}
		cells, err := rast.Voxels(name, rot)
		if err != nil {
			continue
		}
		at := place.Point.Scale(synth.HalfStud).
			Add(place.Direction.Unit().Scale(st.Axial * synth.HalfStud))
		shift := cellOf(at)
		moved := make([]geom.Cell, 0, len(cells))
		for _, c := range cells {
			moved = append(moved, c.Add(shift))
		}
		out = append(out, Solid{Cells: moved})
	}
	return out
}

func cellOf(at geom.Vec3) geom.Cell {
	return geom.Cell{
		X: int32(math.Round(at.X / geom.VoxelPitch)),
		Y: int32(math.Round(at.Y / geom.VoxelPitch)),
		Z: int32(math.Round(at.Z / geom.VoxelPitch)),
	}
}

// clashesAt counts the mechanism's parts that would be inside the model.
//
// The parts themselves, cell for cell. A ball of the gear's pitch radius was
// the first try and it counts a wall standing a stud away as a wall through the
// gear: the two-speed reported two of its own gears clashing with the frame
// built to clear them.
func clashesAt(solids []Solid, occupied map[geom.Cell]bool, off geom.Vec3) int {
	if len(occupied) == 0 || len(solids) == 0 {
		return 0
	}
	shift := cellOf(off)
	n := 0
	for _, s := range solids {
		for _, c := range s.Cells {
			if occupied[c.Add(shift)] {
				n++
				break
			}
		}
	}
	return n
}

// scoreFit counts how many of a layout's lines land on a bearing once moved.
func scoreFit(lines map[[6]float64][]string, bearings []Bearing, off geom.Vec3) Fit {
	f := Fit{Offset: off}
	for k, ids := range lines {
		f.Total += len(ids)
		at := geom.Vec3{X: k[0], Y: k[1], Z: k[2]}.Add(off)
		dir := geom.Vec3{X: k[3], Y: k[4], Z: k[5]}
		for _, b := range bearings {
			if math.Abs(math.Abs(dir.Dot(b.Axis.Unit()))-1) > 1e-3 {
				continue
			}
			d := b.At.Sub(at)
			if d.Sub(dir.Scale(d.Dot(dir))).Len() > fitTolerance {
				continue
			}
			f.Borne += len(ids)
			f.On = append(f.On, ids...)
			break
		}
	}
	sort.Strings(f.On)
	return f
}

// show rounds an offset for printing. The exact value is what gets used; a
// reader does not need to see that a chassis sits 0.11999999999999744 off.
func show(v geom.Vec3) string {
	return fmt.Sprintf("{%.2f %.2f %.2f}", v.X, v.Y, v.Z)
}

// ReportFit says where a mechanism could go in a model.
func ReportFit(l *layout.Layout, bearings []Bearing) []mech.Finding {
	return ReportFitIn(l, bearings, nil, nil)
}

// ReportFitIn is ReportFit with the model's own space taken into account.
func ReportFitIn(l *layout.Layout, bearings []Bearing,
	occupied map[geom.Cell]bool, solids []Solid) []mech.Finding {

	fits := FitToIn(l, bearings, occupied, solids, 3)
	if len(fits) == 0 {
		return []mech.Finding{{Level: "WARN", Check: "fit", Detail: "nothing to " +
			"fit to: the model offers no bearing running the way any of these " +
			"shafts do"}}
	}
	best := fits[0]
	if best.Borne == 0 {
		return []mech.Finding{{Level: "WARN", Check: "fit", Detail: fmt.Sprintf(
			"no placement puts any of the %d shaft(s) on a line this model "+
				"already bears. It would have to be built a frame of its own",
			best.Total)}}
	}
	out := []mech.Finding{{Level: "OK", Check: "fit", Detail: fmt.Sprintf(
		"best placement moves it by %v and puts %d of %d shaft(s) on lines the "+
			"model already bears: %v", show(best.Offset), best.Borne, best.Total, best.On)}}
	if best.Clashes > 0 {
		out = append(out, mech.Finding{Level: "WARN", Check: "fit", Detail: fmt.Sprintf(
			"%d of its gears would be inside the model there. Every placement "+
				"tried has that problem, so this one is the least bad rather "+
				"than a place it goes", best.Clashes)})
	}
	for _, f := range fits[1:] {
		if f.Borne == 0 {
			break
		}
		out = append(out, mech.Finding{Level: "OK", Check: "fit", Detail: fmt.Sprintf(
			"  or by %s for %d of %d", show(f.Offset), f.Borne, f.Total)})
	}
	if best.Borne < best.Total {
		out = append(out, mech.Finding{Level: "WARN", Check: "fit", Detail: fmt.Sprintf(
			"%d shaft(s) land where the model bears nothing, so those still "+
				"need holding", best.Total-best.Borne)})
	}
	return out
}

// SolidsOfLayout is SolidsOf for a mechanism that has not been through the
// pipeline, so has no ring sites or slip clutches to choose gear variants by.
func SolidsOfLayout(l *layout.Layout, stations []layout.Station,
	rast *voxel.Rasterizer) []Solid {

	return SolidsOf(l, stations, rast, nil, nil)
}

// FitInto is a model to place a mechanism inside.
type FitInto struct {
	// Parts is the model itself, so the result can be written as a copy of it
	// with the mechanism added.
	Parts []ldr.Placed
	// Bearings is what it already offers to hold a shaft on, and Occupied the
	// space nothing new may enter.
	Bearings []Bearing
	Occupied map[geom.Cell]bool
	// Rast turns the mechanism's own gears into cells, so a placement can be
	// asked whether there is room and not only whether the lines line up.
	// Without it every placement reads as clear, and the first one that lines
	// up wins however solidly it is buried.
	Rast *voxel.Rasterizer
}

// fitInto moves a laid-out mechanism to where it goes in an existing model.
//
// Only the layout moves. A mechanism's gears sit on the lattice with respect to
// each other and that is settled by the time this runs; what a chassis decides
// is where the whole assembly goes.
func fitInto(res *Result, into *FitInto) error {
	stations, _ := layout.SolveStations(res.Layout.Mech, res.Layout)
	solids := SolidsOf(res.Layout, stations, into.Rast, nil, nil)
	fits := FitToIn(res.Layout, into.Bearings, into.Occupied, solids, 1)
	if len(fits) == 0 || fits[0].Borne == 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "WARN", Check: "fit", Detail: "this model bears none of these " +
				"shafts anywhere, so the mechanism is placed at the origin and " +
				"given a frame of its own"})
		return nil
	}
	best := fits[0]
	// The offset splits in two, and only half of it can live in the layout.
	//
	// NewPlacement pulls a point back to the foot of the perpendicular, because
	// a line has no origin along itself — which is what stopped two shafts
	// disagreeing about where axial zero was, and is why the three-speed's
	// gears mesh. The consequence here is that the along-axis part of a fit
	// offset cannot be stored in a Placement at all: it vanishes. It has to
	// move the STATIONS instead. Put in the Point, a slide of one stud along
	// the shafts was silently dropped and the model came out where it had
	// already been rejected for.
	for id, p := range res.Layout.Place {
		d := p.Direction.Unit()
		across := best.Offset.Sub(d.Scale(best.Offset.Dot(d)))
		res.Layout.Place[id] = layout.NewPlacement(
			p.Point.Add(across.Scale(1/synth.HalfStud)), p.Direction)
	}
	res.fitOffset = best.Offset
	res.fitBearsEverything = best.Borne == best.Total && best.Clashes == 0

	level, detail := "OK", fmt.Sprintf(
		"placed %s into the model: %d of %d shaft(s) on lines it already bears",
		show(best.Offset), best.Borne, best.Total)
	if !res.fitBearsEverything {
		level = "WARN"
		detail += fmt.Sprintf(", and %d not — those get a frame of their own, "+
			"which may duplicate what is already there", best.Total-best.Borne)
	} else {
		detail += ", so it needs no frame of its own"
	}
	res.Findings = append(res.Findings, mech.Finding{
		Level: level, Check: "fit", Detail: detail})
	return nil
}

// bestLayoutFor picks the arrangement and orientation that suit a model best.
//
// Turned as well as chosen. The layout search always puts its first shaft along
// one fixed direction — see layout.candidates — so every arrangement it returns
// runs the same way, and a model's bearings run whichever way somebody built
// them. Without turning, a chassis whose shafts run along y was told that a
// reduction could not go in it anywhere.
//
// All 24 lattice rotations, which is cheap: a handful of layouts by 24 by a few
// thousand candidate offsets is still arithmetic on small vectors.
func bestLayoutFor(layouts []*layout.Layout, into *FitInto) *layout.Layout {
	best, bestBorne, bestClash := layouts[0], -1, 0
	for _, l := range layouts {
		for _, rot := range geom.Rotations {
			turned := turnLayout(l, rot)
			stations, _ := layout.SolveStations(turned.Mech, turned)
			fits := FitToIn(turned, into.Bearings, into.Occupied,
				SolidsOf(turned, stations, into.Rast, nil, nil), 1)
			if len(fits) == 0 {
				continue
			}
			f := fits[0]
			if bestBorne < 0 || f.Borne > bestBorne ||
				(f.Borne == bestBorne && f.Clashes < bestClash) {
				best, bestBorne, bestClash = turned, f.Borne, f.Clashes
			}
		}
	}
	return best
}

// turnLayout is a layout with every shaft turned by one of the 24.
//
// The stations do not move: a station is an axial position along its own shaft,
// so turning the shaft carries it round with no arithmetic of its own.
func turnLayout(l *layout.Layout, rot geom.Mat3) *layout.Layout {
	out := &layout.Layout{Mech: l.Mech, Place: make(map[string]layout.Placement, len(l.Place))}
	for id, p := range l.Place {
		out.Place[id] = layout.NewPlacement(rot.Apply(p.Point), rot.Apply(p.Direction))
	}
	return out
}
