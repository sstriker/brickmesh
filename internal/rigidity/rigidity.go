// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package rigidity checks whether a structure holds together.
//
// A structure that bears every axle is not yet a structure. Two beams that
// touch nowhere hang loose in the air, and two beams with a single pin between
// them are a hinge. That is exactly the mistake you only discover once the
// thing goes limp in your hands.
//
// The test is the mobility formula. For pin joints with parallel axes the
// planar form applies:
//
//	M = 3(n-1) - 2j
//
// with n parts and j pin joints. M > 0 means it hinges. If the pin axes are not
// all parallel, the spatial form M = 6(n-1) - 5j applies instead.
//
// Note why the planar form is needed: computed spatially, a square of four
// beams would look rigid, while it is really a four-bar linkage with one degree
// of freedom. That is the classic Gruebler paradox, and exactly the case you
// run into constantly in LEGO.
package rigidity

import (
	"fmt"
	"math"
	"sort"

	"brickmesh/internal/geom"
	"brickmesh/internal/mech"
	"brickmesh/internal/part"
)

const tol = 1e-6

// PinReach is how far apart two holes can be along their shared axis and still
// take one pin.
//
// A standard Technic pin is 40 LDU end to end — its shadow entry is a centered
// cylinder of sections 2+16+4+16+2 — so it spans 20 either side of its middle
// and reaches the hole planes of two parts lying against each other, which are
// 20 apart. Holes at the very same point are the degenerate case of the same
// thing.
const PinReach = 40.0

// Joint is a pin through two parts' coincident holes.
type Joint struct {
	A, B  int
	Point geom.Vec3
	Axis  geom.Vec3 // sign-free
}

func round3(v geom.Vec3) geom.Vec3 {
	r := func(f float64) float64 { return math.Round(f*1e3) / 1e3 }
	return geom.Vec3{X: r(v.X), Y: r(v.Y), Z: r(v.Z)}
}

func abs3(v geom.Vec3) geom.Vec3 {
	return geom.Vec3{X: math.Abs(v.X), Y: math.Abs(v.Y), Z: math.Abs(v.Z)}
}

// Axle is a shaft passing through the parts that bear it.
//
// It is what actually ties two bearings together, and leaving it out is why a
// structure can look like loose pieces when the build would hold: the bearings
// are joined through the very shaft they carry.
type Axle struct {
	Point geom.Vec3 // a point on its line
	Dir   geom.Vec3 // unit, along the axle
	From  float64   // extent along Dir from Point, in LDU
	To    float64
}

// Covers reports whether a hole facing along the axle sits on it, within reach.
func (a Axle) Covers(hole, axis geom.Vec3) bool {
	if math.Abs(math.Abs(axis.Unit().Dot(a.Dir))-1) > 1e-6 {
		return false
	}
	d := hole.Sub(a.Point)
	along := d.Dot(a.Dir)
	if d.Sub(a.Dir.Scale(along)).Len() > tol {
		return false // on a parallel line, not this one
	}
	return along >= a.From-tol && along <= a.To+tol
}

// FindJoints finds coincident holes with parallel axes: a pin can go through
// there.
func FindJoints(src part.Holes, parts []part.Placed, inventory []part.Beam) ([]Joint, error) {
	return FindJointsWith(src, parts, inventory, nil)
}

// FindJointsWith also counts the parts an axle threads together.
func FindJointsWith(src part.Holes, parts []part.Placed, inventory []part.Beam,
	axles []Axle) ([]Joint, error) {

	joints, err := findPinJoints(src, parts, inventory)
	if err != nil {
		return nil, err
	}
	threaded, err := findAxleJoints(src, parts, inventory, axles)
	if err != nil {
		return nil, err
	}
	return append(joints, threaded...), nil
}

// findAxleJoints threads the parts on each axle together in order.
//
// In a chain rather than every pair: five parts on one axle are four
// constraints, not ten, and the mobility formula counts what it is given.
func findAxleJoints(src part.Holes, parts []part.Placed, _ []part.Beam,
	axles []Axle) ([]Joint, error) {

	if len(axles) == 0 {
		return nil, nil
	}
	var out []Joint
	for _, axle := range axles {
		type onAxle struct {
			idx   int
			along float64
			at    geom.Vec3
		}
		var found []onAxle
		for i, p := range parts {
			ports, err := part.WorldPorts(src, p)
			if err != nil {
				continue // nothing to thread
			}
			for _, h := range ports {
				// Each hole on its own axis: a part can present one hole to the
				// shaft and face its others elsewhere.
				if !axle.Covers(h.Pos, h.Axis) {
					continue
				}
				found = append(found, onAxle{i, h.Pos.Sub(axle.Point).Dot(axle.Dir), h.Pos})
				break // one hole is enough to be on it
			}
		}
		sort.SliceStable(found, func(i, j int) bool { return found[i].along < found[j].along })
		for k := 1; k < len(found); k++ {
			out = append(out, Joint{
				A: found[k-1].idx, B: found[k].idx,
				Point: round3(found[k].at), Axis: round3(abs3(axle.Dir)),
			})
		}
	}
	return out, nil
}

func findPinJoints(src part.Holes, parts []part.Placed, _ []part.Beam) ([]Joint, error) {
	data := make([][]part.Hole, 0, len(parts))
	for _, p := range parts {
		ports, err := part.WorldPorts(src, p)
		if err != nil {
			return nil, fmt.Errorf("%s has no connection points: %w", p.Part, err)
		}
		data = append(data, ports)
	}

	var out []Joint
	for i := 0; i < len(parts); i++ {
		for j := i + 1; j < len(parts); j++ {
			// Hole by hole rather than part by part: two parts may line up on
			// one pair of holes and face different ways on the rest, which is
			// exactly what a perpendicular connector is for.
			for _, a := range data[i] {
				for _, b := range data[j] {
					if math.Abs(math.Abs(a.Axis.Dot(b.Axis))-1) > 1e-6 {
						continue // these two holes do not line up
					}
					if !withinPinReach(a.Pos, b.Pos, a.Axis) {
						continue
					}
					out = append(out, Joint{A: i, B: j,
						Point: round3(a.Pos), Axis: round3(abs3(a.Axis))})
				}
			}
		}
	}
	return out, nil
}

// withinPinReach reports whether two holes line up well enough for one pin.
//
// They have to be on the SAME axis line, not merely parallel ones: the offset
// across the axis must be nothing, or the pin would have to bend. Along the
// axis they may be up to a pin's reach apart, which is what lets two beams lie
// against each other and still be joined.
func withinPinReach(a, b, axis geom.Vec3) bool {
	d := b.Sub(a)
	along := d.Dot(axis)
	across := d.Sub(axis.Scale(along))
	if across.Len() > tol {
		return false
	}
	return math.Abs(along) <= PinReach+tol
}

// Components groups parts that are joined, directly or through others.
func Components(n int, joints []Joint) [][]int {
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for _, j := range joints {
		parent[find(j.A)] = find(j.B)
	}
	groups := map[int][]int{}
	var order []int
	for i := 0; i < n; i++ {
		r := find(i)
		if _, seen := groups[r]; !seen {
			order = append(order, r)
		}
		groups[r] = append(groups[r], i)
	}
	out := make([][]int, 0, len(order))
	for _, r := range order {
		out = append(out, groups[r])
	}
	return out
}

// Mobility is the degrees of freedom left in the structure, and which form of
// the formula was used.
func Mobility(nParts int, joints []Joint) (int, string) {
	if nParts <= 1 {
		return 0, "single part"
	}
	axes := map[geom.Vec3]bool{}
	for _, j := range joints {
		axes[j.Axis] = true
	}
	j := len(joints)
	if len(axes) <= 1 {
		return 3*(nParts-1) - 2*j, "planar"
	}
	return 6*(nParts-1) - 5*j, "spatial"
}

// Analyze reports whether the structure hangs together and whether it is rigid.
func Analyze(src part.Holes, parts []part.Placed, inventory []part.Beam) ([]mech.Finding, error) {
	return AnalyzeWith(src, parts, inventory, nil)
}

// AnalyzeWith counts the shafts as what holds the bearings together.
func AnalyzeWith(src part.Holes, parts []part.Placed, inventory []part.Beam,
	axles []Axle) ([]mech.Finding, error) {

	joints, err := FindJointsWith(src, parts, inventory, axles)
	if err != nil {
		return nil, err
	}
	comps := Components(len(parts), joints)

	if len(comps) > 1 {
		sizes := make([]int, len(comps))
		for i, c := range comps {
			sizes[i] = len(c)
		}
		out := []mech.Finding{{Level: "FAIL", Check: "connectivity", Detail: fmt.Sprintf(
			"the structure falls apart into %d separate pieces (sizes %v). "+
				"Parts attached to nothing carry nothing.", len(comps), sizes)}}
		for _, c := range comps {
			if len(c) == 1 {
				p := parts[c[0]]
				out = append(out, mech.Finding{Level: "FAIL", Check: "connectivity",
					Detail: fmt.Sprintf("  %s at %v floats free", p.Part, p.Origin)})
			}
		}
		return out, nil
	}

	m, kind := Mobility(len(parts), joints)
	if m > 0 {
		return []mech.Finding{{Level: "FAIL", Check: "rigidity", Detail: fmt.Sprintf(
			"%d parts, %d pin joints, mobility M = %d (%s). The structure hinges. "+
				"Add %d joint(s), or triangulate it with a 3-4-5.",
			len(parts), len(joints), m, kind, m)}}, nil
	}
	over := ""
	if m < 0 {
		over = " (overconstrained, normal in LEGO)"
	}
	return []mech.Finding{{Level: "OK", Check: "rigidity", Detail: fmt.Sprintf(
		"%d parts, %d pin joints, M = %d (%s): rigid%s",
		len(parts), len(joints), m, kind, over)}}, nil
}

// Summary counts joints per pair, separating the hinges (one pin) from the
// rigid pairs (two or more).
type Summary struct {
	Joints     int
	Pairs      int
	Hinges     [][2]int
	RigidPairs [][2]int
}

// Summarize reports the joint structure of a set of placed parts.
func Summarize(src part.Holes, parts []part.Placed, inventory []part.Beam) (Summary, error) {
	joints, err := FindJoints(src, parts, inventory)
	if err != nil {
		return Summary{}, err
	}
	perPair := map[[2]int]int{}
	var order [][2]int
	for _, j := range joints {
		k := [2]int{j.A, j.B}
		if _, seen := perPair[k]; !seen {
			order = append(order, k)
		}
		perPair[k]++
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i][0] != order[j][0] {
			return order[i][0] < order[j][0]
		}
		return order[i][1] < order[j][1]
	})

	s := Summary{Joints: len(joints), Pairs: len(perPair)}
	for _, k := range order {
		if perPair[k] == 1 {
			s.Hinges = append(s.Hinges, k)
		} else {
			s.RigidPairs = append(s.RigidPairs, k)
		}
	}
	return s, nil
}
