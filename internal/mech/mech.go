// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package mech is the functional layer: shafts and transmissions, independent
// of any part.
//
// This is where a mechanism is worked out before anything is placed. Ratios,
// directions of rotation, degrees of freedom, torque, and the geometric
// requirements that follow. It is the layer where an idea dies or survives, and
// you want to know which before you start building.
//
// The core: every transmission is one linear equation between shaft speeds. The
// whole mechanism is therefore a matrix, and its null space is the degrees of
// freedom. A subtractor should have two, drive and steer; if only one comes
// out, the train is locked.
package mech

import (
	"fmt"
	"math"
	"sort"
)

// HalfStud in LDU. A center distance lands on a whole half stud when the two
// tooth counts SUM to a multiple of 8; 8t+12t and 36t+40t are the pairs that
// do not.
const HalfStud = 10.0

// Shaft is one axis of rotation.
type Shaft struct {
	ID       string
	Bearings int    // number of bearing points
	Domain   string // parts on incompatible lattices cannot mesh
	Note     string
}

// Kind of transmission.
const (
	Spur  = "spur"
	Bevel = "bevel"
	Worm  = "worm"
	Chain = "chain"
)

// Link is one equation between shaft speeds.
type Link interface {
	// Equation writes a row: the coefficients of each shaft's speed.
	Equation(index map[string]int, n int) []float64
	// Shafts lists the shafts the link touches.
	Shafts() []string
}

// Mesh is a gear pair. Externally meshing, so the direction of rotation
// reverses.
type Mesh struct {
	A, B           string
	TeethA, TeethB int
	Kind           string
	BacklashDeg    float64
}

func (m Mesh) Reverses() bool { return m.Kind == Spur || m.Kind == Bevel }

func (m Mesh) Shafts() []string { return []string{m.A, m.B} }

// CenterDistanceHalfStuds is only meaningful for parallel shafts.
func (m Mesh) CenterDistanceHalfStuds() (float64, bool) {
	if m.Kind != Spur {
		return 0, false
	}
	return float64(m.TeethA+m.TeethB) / 8.0, true
}

// Equation is t_a * w_a (+/-) t_b * w_b = 0.
func (m Mesh) Equation(index map[string]int, n int) []float64 {
	row := make([]float64, n)
	sign := -1.0
	if m.Reverses() {
		sign = 1.0
	}
	row[index[m.A]] = float64(m.TeethA)
	row[index[m.B]] = sign * float64(m.TeethB)
	return row
}

// Differential has three ports. The case speed is the average of both outputs:
//
//	2*w_case - w_1 - w_2 = 0
//
// This is the only transmission with more than two ports, and that is exactly
// why a subtractor can exist.
type Differential struct {
	Case, OutA, OutB string
}

func (d Differential) Shafts() []string { return []string{d.Case, d.OutA, d.OutB} }

func (d Differential) Equation(index map[string]int, n int) []float64 {
	row := make([]float64, n)
	row[index[d.Case]] = 2
	row[index[d.OutA]] = -1
	row[index[d.OutB]] = -1
	return row
}

// Finding is one line of a report.
type Finding struct {
	Level  string // OK / WARN / FAIL
	Check  string
	Detail string
}

// Mechanism is the graph of shafts and the transmissions between them.
type Mechanism struct {
	Name     string
	order    []string // insertion order, which fixes the matrix columns
	shafts   map[string]*Shaft
	Links    []Link
	Inputs   map[string]float64
	inOrder  []string
	Outputs  []string
	inputSet map[string]bool
}

func New(name string) *Mechanism {
	return &Mechanism{
		Name:     name,
		shafts:   map[string]*Shaft{},
		Inputs:   map[string]float64{},
		inputSet: map[string]bool{},
	}
}

// Shaft adds a shaft and returns its id.
func (m *Mechanism) Shaft(id string, bearings int) string {
	return m.ShaftIn(id, bearings, "technic-studless")
}

// ShaftIn adds a shaft on a named lattice domain.
func (m *Mechanism) ShaftIn(id string, bearings int, domain string) string {
	if _, seen := m.shafts[id]; !seen {
		m.order = append(m.order, id)
	}
	m.shafts[id] = &Shaft{ID: id, Bearings: bearings, Domain: domain}
	return id
}

// Get returns a shaft by id.
func (m *Mechanism) Get(id string) (*Shaft, bool) {
	s, ok := m.shafts[id]
	return s, ok
}

// Order lists the shafts in the order they were added.
func (m *Mechanism) Order() []string { return append([]string(nil), m.order...) }

func (m *Mechanism) Mesh(a, b string, ta, tb int) {
	m.MeshOf(a, b, ta, tb, Spur, 5.0)
}

func (m *Mechanism) MeshOf(a, b string, ta, tb int, kind string, backlash float64) {
	m.Links = append(m.Links, Mesh{A: a, B: b, TeethA: ta, TeethB: tb,
		Kind: kind, BacklashDeg: backlash})
}

func (m *Mechanism) Differential(caseID, outA, outB string) {
	m.Links = append(m.Links, Differential{Case: caseID, OutA: outA, OutB: outB})
}

// Drive fixes a shaft's speed.
func (m *Mechanism) Drive(id string, speed float64) {
	if !m.inputSet[id] {
		m.inOrder = append(m.inOrder, id)
		m.inputSet[id] = true
	}
	m.Inputs[id] = speed
}

func (m *Mechanism) Output(id string) { m.Outputs = append(m.Outputs, id) }

func (m *Mechanism) index() map[string]int {
	idx := make(map[string]int, len(m.order))
	for i, id := range m.order {
		idx[id] = i
	}
	return idx
}

func (m *Mechanism) matrix() [][]float64 {
	idx, n := m.index(), len(m.order)
	rows := make([][]float64, 0, len(m.Links))
	for _, l := range m.Links {
		rows = append(rows, l.Equation(idx, n))
	}
	return rows
}

// DOF is the number of shafts minus the rank of the constraints.
func (m *Mechanism) DOF() int {
	return len(m.order) - rank(m.matrix())
}

// Solve returns the speed of every shaft given the driven ones, or ok=false
// when the system is underdetermined or inconsistent.
func (m *Mechanism) Solve() (map[string]float64, bool) {
	idx, n := m.index(), len(m.order)
	rows := m.matrix()
	rhs := make([]float64, len(rows))
	for _, id := range m.inOrder {
		row := make([]float64, n)
		row[idx[id]] = 1
		rows = append(rows, row)
		rhs = append(rhs, m.Inputs[id])
	}
	if rank(rows) < n {
		return nil, false // underdetermined
	}
	sol, ok := leastSquares(rows, rhs)
	if !ok {
		return nil, false
	}
	// Least squares always returns something; only a residual of zero means the
	// constraints were actually satisfiable.
	for i, row := range rows {
		got := 0.0
		for j, v := range row {
			got += v * sol[j]
		}
		if math.Abs(got-rhs[i]) > 1e-8 {
			return nil, false // inconsistent
		}
	}
	out := make(map[string]float64, n)
	for id, i := range idx {
		out[id] = sol[i]
	}
	return out, true
}

// CheckDOF reports whether the mechanism can move, and whether it is driven the
// right number of times.
func (m *Mechanism) CheckDOF() []Finding {
	d, k := m.DOF(), len(m.inOrder)
	switch {
	case d == 0:
		return []Finding{{"FAIL", "dof",
			"mechanism has 0 degrees of freedom: the train is locked and cannot turn"}}
	case k < d:
		return []Finding{{"WARN", "dof", fmt.Sprintf(
			"%d degrees of freedom but %d driven shafts — %d motion(s) remain undetermined",
			d, k, d-k)}}
	case k > d:
		return []Finding{{"FAIL", "dof", fmt.Sprintf(
			"%d drives on %d degrees of freedom — overdetermined, the motors work against each other",
			k, d)}}
	}
	return []Finding{{"OK", "dof", fmt.Sprintf(
		"%d degrees of freedom, %d drives: determined", d, k)}}
}

// CheckBearings wants two bearing points per shaft. Fewer than two means it
// whips under load.
func (m *Mechanism) CheckBearings() []Finding {
	var out []Finding
	for _, id := range m.order {
		s := m.shafts[id]
		if s.Bearings < 2 {
			out = append(out, Finding{"FAIL", "bearings", fmt.Sprintf(
				"shaft '%s' has %d bearing point(s). Fewer than two means it whips under load.",
				s.ID, s.Bearings)})
		}
	}
	if len(out) == 0 {
		return []Finding{{"OK", "bearings", "every shaft borne at both ends"}}
	}
	return out
}

// CheckDomains catches a transmission that spans two lattices. Technic bricks
// sit at 24 LDU vertically, liftarms at 20, so the holes do not line up.
func (m *Mechanism) CheckDomains() []Finding {
	var out []Finding
	for _, l := range m.Links {
		ids := l.Shafts()
		seen := map[string]bool{}
		for _, id := range ids {
			if s, ok := m.shafts[id]; ok {
				seen[s.Domain] = true
			}
		}
		if len(seen) > 1 {
			domains := make([]string, 0, len(seen))
			for d := range seen {
				domains = append(domains, d)
			}
			sort.Strings(domains)
			out = append(out, Finding{"FAIL", "grid", fmt.Sprintf(
				"transmission between %v crosses grid domains %v. Technic bricks sit at "+
					"24 LDU vertically, liftarms at 20 — the holes do not line up.",
				ids, domains)})
		}
	}
	if len(out) == 0 {
		return []Finding{{"OK", "grid", "one grid domain, no transitions"}}
	}
	return out
}

// CheckCenterDistances catches a spur pair that cannot sit on the lattice.
//
// A pair sits (ta+tb)/8 half studs apart, and beams offer whole half studs.
// That works out exactly when the two tooth counts SUM to a multiple of 8 —
// each being a multiple of 4 is not enough. 8t+12t lands on 2.5 half studs and
// 36t+40t on 9.5. Caught here, because the geometric layer can only respond to
// such a pair by finding no layout at all.
func (m *Mechanism) CheckCenterDistances() []Finding {
	var out []Finding
	for _, l := range m.Links {
		mesh, ok := l.(Mesh)
		if !ok || mesh.Kind != Spur {
			continue
		}
		d, _ := mesh.CenterDistanceHalfStuds()
		if math.Abs(d-math.Round(d)) < 1e-9 {
			continue
		}
		out = append(out, Finding{"FAIL", "center dist", fmt.Sprintf(
			"%s/%s: %dt and %dt mesh at %g half studs, which is off the lattice. "+
				"Tooth counts have to sum to a multiple of 8, and %d+%d = %d does not. "+
				"Pick another pair or put an idler between them.",
			mesh.A, mesh.B, mesh.TeethA, mesh.TeethB, d,
			mesh.TeethA, mesh.TeethB, mesh.TeethA+mesh.TeethB)})
	}
	if len(out) == 0 {
		return []Finding{{"OK", "center dist", "every spur pair lands on a whole half stud"}}
	}
	return out
}

// CheckClosure tests gear loops. Three shafts driving each other in a ring fix
// three center distances, and that triangle has to close on the lattice or the
// third gear will not go in.
//
// Note it places the first shaft at the origin and the second at (d, 0), then
// asks whether the THIRD lands on the lattice — without checking that the
// second one did. CheckCenterDistances is what covers that.
func (m *Mechanism) CheckClosure() []Finding {
	type pair struct{ a, b string }
	dist := map[pair]float64{}
	adj := map[string]map[string]bool{}

	key := func(a, b string) pair {
		if a > b {
			a, b = b, a
		}
		return pair{a, b}
	}
	for _, l := range m.Links {
		mesh, ok := l.(Mesh)
		if !ok || mesh.Kind != Spur {
			continue
		}
		d, _ := mesh.CenterDistanceHalfStuds()
		dist[key(mesh.A, mesh.B)] = d
		if adj[mesh.A] == nil {
			adj[mesh.A] = map[string]bool{}
		}
		if adj[mesh.B] == nil {
			adj[mesh.B] = map[string]bool{}
		}
		adj[mesh.A][mesh.B] = true
		adj[mesh.B][mesh.A] = true
	}

	names := make([]string, 0, len(adj))
	for id := range adj {
		names = append(names, id)
	}
	sort.Strings(names)

	var out []Finding
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			for k := j + 1; k < len(names); k++ {
				a, b, c := names[i], names[j], names[k]
				dab, ok1 := dist[key(a, b)]
				dbc, ok2 := dist[key(b, c)]
				dac, ok3 := dist[key(a, c)]
				if !ok1 || !ok2 || !ok3 {
					continue
				}
				if dab+dbc <= dac || dab+dac <= dbc || dbc+dac <= dab {
					out = append(out, Finding{"FAIL", "loop closure", fmt.Sprintf(
						"%s-%s-%s: center distances %g/%g/%g half studs do not form a triangle",
						a, b, c, dab, dbc, dac)})
					continue
				}
				// Third point, with A at (0,0) and B at (dab,0).
				x := (dab*dab + dac*dac - dbc*dbc) / (2 * dab)
				y := math.Sqrt(math.Max(dac*dac-x*x, 0))
				if math.Abs(x-math.Round(x)) < 1e-9 && math.Abs(y-math.Round(y)) < 1e-9 {
					out = append(out, Finding{"OK", "loop closure", fmt.Sprintf(
						"%s-%s-%s closes on the lattice: third shaft at (%.0f, %.0f) half studs",
						a, b, c, x, y)})
				} else {
					out = append(out, Finding{"FAIL", "loop closure", fmt.Sprintf(
						"%s-%s-%s does NOT close on the lattice: the third shaft would land at "+
							"(%.3f, %.3f) half studs. Pick a different tooth count or add an idler.",
						a, b, c, x, y)})
				}
			}
		}
	}
	if len(out) == 0 {
		return []Finding{{"OK", "loop closure", "no gear loops present"}}
	}
	return out
}

// Backlash accumulates play along a path of shafts, in degrees at the output.
func (m *Mechanism) Backlash(path []string) float64 {
	total, ratio := 0.0, 1.0
	for i := 0; i+1 < len(path); i++ {
		a, b := path[i], path[i+1]
		var found *Mesh
		for _, l := range m.Links {
			mesh, ok := l.(Mesh)
			if !ok {
				continue
			}
			if (mesh.A == a && mesh.B == b) || (mesh.A == b && mesh.B == a) {
				cp := mesh
				found = &cp
				break
			}
		}
		if found == nil {
			continue
		}
		total += found.BacklashDeg * ratio
		if found.A == a {
			ratio *= float64(found.TeethA) / float64(found.TeethB)
		} else {
			ratio *= float64(found.TeethB) / float64(found.TeethA)
		}
	}
	return total
}

// RunChecks is every check, in reporting order.
func (m *Mechanism) RunChecks() []Finding {
	out := m.CheckDOF()
	out = append(out, m.CheckBearings()...)
	out = append(out, m.CheckDomains()...)
	out = append(out, m.CheckCenterDistances()...)
	out = append(out, m.CheckClosure()...)
	return out
}
