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

// FindJoints finds coincident holes with parallel axes: a pin can go through
// there.
func FindJoints(src part.AxisSource, parts []part.Placed, inventory []part.Beam) ([]Joint, error) {
	counts := part.HoleCounts(inventory)
	type holes struct {
		pts  []geom.Vec3
		axis geom.Vec3
	}
	data := make([]holes, 0, len(parts))
	for _, p := range parts {
		n, ok := counts[p.Part]
		if !ok {
			return nil, fmt.Errorf("%s is not in the inventory", p.Part)
		}
		pts, axis, err := part.WorldHoles(src, p, n)
		if err != nil {
			return nil, err
		}
		data = append(data, holes{pts, axis})
	}

	var out []Joint
	for i := 0; i < len(parts); i++ {
		for j := i + 1; j < len(parts); j++ {
			if math.Abs(math.Abs(data[i].axis.Dot(data[j].axis))-1) > 1e-6 {
				continue // holes do not line up
			}
			for _, a := range data[i].pts {
				for _, b := range data[j].pts {
					if a.Sub(b).Len() < tol {
						out = append(out, Joint{A: i, B: j,
							Point: round3(a), Axis: round3(abs3(data[i].axis))})
					}
				}
			}
		}
	}
	return out, nil
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
func Analyze(src part.AxisSource, parts []part.Placed, inventory []part.Beam) ([]mech.Finding, error) {
	joints, err := FindJoints(src, parts, inventory)
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
func Summarize(src part.AxisSource, parts []part.Placed, inventory []part.Beam) (Summary, error) {
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
