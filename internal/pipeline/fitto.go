// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"math"
	"sort"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/layout"
	"github.com/sstriker/brickmesh/internal/mech"
	"github.com/sstriker/brickmesh/internal/synth"
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
}

// fitTolerance is how close a line has to come to a bearing to count, in LDU.
//
// The same reasoning as readTolerance: a model somebody built is rounded and
// composed, and 42110's own chassis lines come out along {0 0.003 1} rather
// than {0 0 1}. Half an LDU is far under the two studs between lattice
// positions, so it cannot claim a fit that is not there.
const fitTolerance = 0.5

// FitTo finds where a layout could sit in a model, best first.
//
// Every bearing the model offers is tried as the home of every shaft, which
// fixes a translation; the rest of the shafts are then counted. That is
// quadratic in a number that is small for a mechanism and large for a chassis —
// 42110 offers 767 usable lines — so the bearings are the outer loop and the
// shafts the inner one.
func FitTo(l *layout.Layout, bearings []Bearing, want int) []Fit {
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
			key := [3]float64{
				math.Round(off.X*10) / 10,
				math.Round(off.Y*10) / 10,
				math.Round(off.Z*10) / 10,
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, scoreFit(lines, bearings, off))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Borne != out[j].Borne {
			return out[i].Borne > out[j].Borne
		}
		return out[i].Offset.Len() < out[j].Offset.Len()
	})
	if want > 0 && len(out) > want {
		out = out[:want]
	}
	return out
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
	fits := FitTo(l, bearings, 3)
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
