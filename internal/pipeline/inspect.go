// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"math"
	"sort"

	"github.com/sstriker/brickmesh/internal/clutch"
	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/ldr"
	"github.com/sstriker/brickmesh/internal/mech"
	"github.com/sstriker/brickmesh/internal/part"
)

// readTolerance is how far out a placement may be and still read as meshing,
// in LDU.
//
// Not the exactness the engine's own checks use. A model this engine wrote is
// exact to a float, but one written by a person is rounded to a couple of
// decimals at every level and then composed through nested submodels, so the
// error accumulates. Matching to a millionth found nine pairs among the
// sixty-five gears of a real set; half an LDU is well under the two studs that
// separate any two lattice positions, so it cannot invent a pair that is not
// there.
const readTolerance = 0.5

// ToothSource answers how many teeth a part has, for parts this engine does
// not itself place.
type ToothSource interface {
	Teeth(part string) (int, bool)
}

// Found is one part recognised in a model somebody else built.
type Found struct {
	ldr.Placed
	// Teeth is the count when this is a gear, and zero otherwise.
	Teeth int
	// Axis is the direction it turns about, for a gear or a ring.
	Axis geom.Vec3
	// What it is: one of the class constants.
	Class int
}

// Reading is what a model turned out to contain.
type Reading struct {
	Parts []Found
	// Lines groups what shares an axis, keyed the same way the layout keys a
	// placement so the two can be compared.
	Lines map[[6]float64][]int
	// Meshes are pairs of gears standing where they would drive each other.
	Meshes []FoundMesh
	// Unknown counts parts nothing here recognises, so a reading says how much
	// of a model it did not understand rather than quietly ignoring it.
	Unknown  map[string]int
	Findings []mech.Finding
}

// FoundMesh is two gears meeting, with how they meet.
type FoundMesh struct {
	A, B int    // indexes into Parts
	Kind string // mech.Spur or mech.Bevel
}

// beamHoles is the structural inventory by name, so a beam in a model somebody
// else built is recognised as structure rather than as a mystery.
var beamHoles = func() map[string]int {
	out := map[string]int{}
	for _, b := range part.Beams {
		out[b.Part] = b.Holes
	}
	// Placed by the engine but not part of the search's inventory.
	out[MarkerPart] = 2
	return out
}()

// gearTeeth is every gear this engine knows by name, and how many teeth it has.
//
// From the tables the engine places by, so a model it wrote reads back exactly.
// A gear outside this list is reported as unknown rather than guessed at: a
// wrong tooth count is a wrong ratio, and a wrong ratio said confidently is
// worse than an admission.
func gearTeeth() map[string]int {
	out := map[string]int{}
	for teeth, name := range GearParts {
		out[name] = teeth
	}
	for _, s := range clutch.Systems {
		for teeth, name := range s.Gears {
			out[name] = teeth
		}
	}
	// Placed by the engine but not through GearParts.
	out[SlipPart] = 24
	return out
}

// Inspect works out what a model contains.
//
// The inverse of the pipeline, and it uses the same rules: two gears mesh here
// if they stand where checkMeshing would require them to stand. A criterion
// good enough to reject a placement is good enough to recognise one, and using
// one rule for both means they cannot drift apart.
func Inspect(parts []ldr.Placed) *Reading { return InspectWith(parts, nil) }

// InspectWith is Inspect with a way to ask a part how many teeth it has.
//
// Without one the vocabulary is whatever this engine places, which for a real
// set is most of the gears missed: 42110 has twenty-four and only the clutch
// gears were named. A gear's own title says its tooth count, so given the
// library it can be read there — see ldraw.Library.Title.
func InspectWith(parts []ldr.Placed, ask ToothSource) *Reading {
	teeth := gearTeeth()
	r := &Reading{Lines: map[[6]float64][]int{}, Unknown: map[string]int{}}

	for _, p := range parts {
		f := Found{Placed: p, Class: classOf(ldr.Part{Name: p.Name, Pos: p.Pos, Rot: p.Rot})}
		if n, ok := teeth[p.Name]; ok {
			f.Teeth = n
			f.Class = classGear
		} else if ask != nil {
			if n, ok := ask.Teeth(p.Name); ok {
				f.Teeth = n
				f.Class = classGear
			}
		}
		switch {
		case f.Teeth > 0, isRing(p.Name), isJoiner(p.Name):
			// A gear, a ring and a joiner all turn about their own z.
			f.Axis = p.Rot.Apply(geom.Vec3{Z: 1}).Unit()
		case isAxle(p.Name):
			f.Axis = p.Rot.Apply(geom.Vec3{X: 1}).Unit() // an axle runs along its x
			f.Class = classAxle
		case isPin(p.Name):
			f.Class = classPin
		case isSelector(p.Name):
			f.Class = classSelector
		case p.Name == DiffPart:
			// A differential is one part with its gears inside it, so it turns
			// about its own z like a gear and carries no tooth count of its own.
			f.Axis = p.Rot.Apply(geom.Vec3{Z: 1}).Unit()
			f.Class = classGear
		case beamHoles[p.Name] > 0:
			// Structure. Note a thin liftarm reads as structure here even when
			// the engine put it there as a marker: without the label that wrote
			// it, a liftarm on a shaft end IS a liftarm, and saying otherwise
			// would be reading intent out of geometry.
			f.Class = classStructure
		default:
			r.Unknown[p.Name]++
		}
		r.Parts = append(r.Parts, f)
	}

	for i, f := range r.Parts {
		if f.Axis == (geom.Vec3{}) {
			continue
		}
		k := lineKey(f.Pos, f.Axis)
		r.Lines[k] = append(r.Lines[k], i)
	}
	r.findMeshes()
	r.report()
	return r
}

// lineKey identifies an axis the way layout.Placement does: a direction, and
// the point on the line closest to the origin, so any point on it agrees.
func lineKey(at, dir geom.Vec3) [6]float64 {
	d := dir.Unit()
	// One of the two directions, so a line and its reverse are one line.
	if d.X < -1e-9 || (math.Abs(d.X) < 1e-9 && (d.Y < -1e-9 ||
		(math.Abs(d.Y) < 1e-9 && d.Z < -1e-9))) {
		d = d.Scale(-1)
	}
	foot := at.Sub(d.Scale(at.Dot(d)))
	r := func(v float64) float64 { return math.Round(v*1e3) / 1e3 }
	return [6]float64{r(foot.X), r(foot.Y), r(foot.Z), r(d.X), r(d.Y), r(d.Z)}
}

// findMeshes pairs up gears standing where they would drive each other.
func (r *Reading) findMeshes() {
	for i := range r.Parts {
		if r.Parts[i].Teeth == 0 {
			continue
		}
		for j := i + 1; j < len(r.Parts); j++ {
			if r.Parts[j].Teeth == 0 {
				continue
			}
			a, b := r.Parts[i], r.Parts[j]
			d := b.Pos.Sub(a.Pos)
			parallel := math.Abs(math.Abs(a.Axis.Dot(b.Axis))-1) < 1e-6
			switch {
			case parallel:
				// One plane, at the pitch distance: the spur rule.
				if math.Abs(d.Dot(a.Axis)) > readTolerance {
					continue
				}
				if math.Abs(d.Len()-geom.PitchDistance(a.Teeth, b.Teeth)) > readTolerance {
					continue
				}
				r.Meshes = append(r.Meshes, FoundMesh{A: i, B: j, Kind: mech.Spur})
			case math.Abs(a.Axis.Dot(b.Axis)) < 1e-6:
				// Square shafts: each at the other's pitch radius from where
				// the axes meet. See internal/bevel.
				if !bevelMeets(a, b) {
					continue
				}
				r.Meshes = append(r.Meshes, FoundMesh{A: i, B: j, Kind: mech.Bevel})
			}
		}
	}
}

// bevelMeets is the bevel rule, read rather than applied.
func bevelMeets(a, b Found) bool {
	// Where the two axes cross, if they do.
	n := a.Axis.Cross(b.Axis)
	denom := b.Axis.Cross(n).Dot(a.Axis)
	if math.Abs(denom) < 1e-9 {
		return false
	}
	t := -b.Axis.Cross(n).Dot(a.Pos.Sub(b.Pos)) / denom
	at := a.Pos.Add(a.Axis.Scale(t))
	// Each gear the other's pitch radius from it.
	da := math.Abs(a.Pos.Sub(at).Dot(a.Axis))
	db := math.Abs(b.Pos.Sub(at).Dot(b.Axis))
	return math.Abs(da-float64(b.Teeth)*1.25) < readTolerance &&
		math.Abs(db-float64(a.Teeth)*1.25) < readTolerance
}

func (r *Reading) report() {
	gears := 0
	for _, f := range r.Parts {
		if f.Teeth > 0 {
			gears++
		}
	}
	r.Findings = append(r.Findings, mech.Finding{
		Level: "OK", Check: "read", Detail: fmt.Sprintf(
			"%d part(s): %d gear(s) on %d axis line(s), %d gear pair(s) meeting",
			len(r.Parts), gears, len(r.Lines), len(r.Meshes))})
	if len(r.Unknown) == 0 {
		return
	}
	names := make([]string, 0, len(r.Unknown))
	for n := range r.Unknown {
		names = append(names, fmt.Sprintf("%s x%d", n, r.Unknown[n]))
	}
	sort.Strings(names)
	if len(names) > 8 {
		names = append(names[:8], fmt.Sprintf("and %d more", len(names)-8))
	}
	r.Findings = append(r.Findings, mech.Finding{
		Level: "WARN", Check: "read", Detail: fmt.Sprintf(
			"%d part(s) of %d kind(s) are not anything this knows: %v. Whatever "+
				"they do is not in what follows", countOf(r.Unknown), len(r.Unknown), names)})
}

func countOf(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}
