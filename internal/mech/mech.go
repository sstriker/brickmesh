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
	"strings"

	"brickmesh/internal/clutch"
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
	// EngagedIn reports whether the link constrains anything in a given state.
	// Gears are always meshed; only a coupling comes and goes.
	EngagedIn(state string) bool
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

// EngagedIn is always true: gears that mesh, mesh. What a gearbox changes is
// which gear is locked to its shaft, not which teeth touch.
func (m Mesh) EngagedIn(string) bool { return true }

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

// EngagedIn is always true; a differential has no disengaged state.
func (d Differential) EngagedIn(string) bool { return true }

func (d Differential) Equation(index map[string]int, n int) []float64 {
	row := make([]float64, n)
	row[index[d.Case]] = 2
	row[index[d.OutA]] = -1
	row[index[d.OutB]] = -1
	return row
}

// Coupling locks two coaxial shafts together, one to one.
//
// This is the dog ring of a gearbox. The gears themselves are always meshed and
// always turning; what a shift changes is which of them is locked to the output
// shaft. A gear that freewheels is therefore its own shaft, coupled to the one
// it rides on only in the states where its ratio is selected.
//
// Because the gear rides on the shaft, the two are coaxial: the geometric layer
// puts them on one line, as it does the ports of a differential.
type Coupling struct {
	A, B string
	Name string // for reporting: "dog ring", "clutch"
	// States it is engaged in. Empty means always, which is a permanent
	// coupling rather than a shift.
	States []string
}

func (c Coupling) Shafts() []string { return []string{c.A, c.B} }

func (c Coupling) EngagedIn(state string) bool {
	if len(c.States) == 0 {
		return true
	}
	for _, s := range c.States {
		if s == state {
			return true
		}
	}
	return false
}

// Equation is w_a - w_b = 0: locked together, turning as one.
func (c Coupling) Equation(index map[string]int, n int) []float64 {
	row := make([]float64, n)
	row[index[c.A]] = 1
	row[index[c.B]] = -1
	return row
}

// Finding is one line of a report.
type Finding struct {
	Level  string // OK / WARN / FAIL
	Check  string
	Detail string
	// Parts are the placed parts the finding is about, by index into the model.
	//
	// The point of drawing a model here was never to draw it — Stud.io does
	// that better, and says so in docs/architecture.md. It was to show WHICH
	// parts a finding is about: the pair that shares space, the shaft nothing
	// bears, the gear pair whose force has nowhere to go. A sentence naming
	// coordinates is a sentence the reader has to go and find.
	//
	// Optional. A finding about the whole mechanism leaves it empty.
	Parts []int
}

// Mechanism is the graph of shafts and the transmissions between them.
type Mechanism struct {
	Name   string
	order  []string // insertion order, which fixes the matrix columns
	shafts map[string]*Shaft
	Links  []Link
	// states a gearbox can be shifted into, in order. Empty for a mechanism
	// with only one, which is most of them.
	states   []string
	Inputs   map[string]float64
	inOrder  []string
	Outputs  []string
	inputSet map[string]bool
	// shiftPoints make the box change gear on its own. Nil for one shifted by
	// hand, which is most of them.
	shiftPoints *ShiftPoints
}

// Shifts sets when the box changes gear on its own.
func (m *Mechanism) Shifts(p ShiftPoints) { m.shiftPoints = &p }

// ShiftPointsSet is the schedule the box was given, if any.
func (m *Mechanism) ShiftPointsSet() (ShiftPoints, bool) {
	if m.shiftPoints == nil {
		return ShiftPoints{}, false
	}
	return *m.shiftPoints, true
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

// Couple locks two coaxial shafts together in the named states. No states means
// always.
func (m *Mechanism) Couple(a, b, name string, states ...string) {
	m.Links = append(m.Links, Coupling{A: a, B: b, Name: name, States: states})
}

// State declares a position the mechanism can be shifted into. The order is
// kept, since first, second and third read better than an alphabetical list.
func (m *Mechanism) State(name string) {
	for _, s := range m.states {
		if s == name {
			return
		}
	}
	m.states = append(m.states, name)
}

// States lists the declared states. A mechanism with none has exactly one
// unnamed state, in which every unconditional link is engaged.
func (m *Mechanism) States() []string { return append([]string(nil), m.states...) }

// statesToCheck is the declared states, or the single unnamed one.
func (m *Mechanism) statesToCheck() []string {
	if len(m.states) == 0 {
		return []string{""}
	}
	return m.states
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

// matrix is the constraints that hold in a state. A link that is not engaged
// there constrains nothing, which is exactly what a disengaged dog ring does.
func (m *Mechanism) matrix(state string) [][]float64 {
	idx, n := m.index(), len(m.order)
	rows := make([][]float64, 0, len(m.Links))
	for _, l := range m.Links {
		if !l.EngagedIn(state) {
			continue
		}
		rows = append(rows, l.Equation(idx, n))
	}
	return rows
}

// DOF is the number of shafts minus the rank of the constraints in a state.
// Pass "" for a mechanism that has only one.
func (m *Mechanism) DOF(state string) int {
	return len(m.order) - rank(m.matrix(state))
}

// Solve returns the speed of every shaft given the driven ones, or ok=false
// when the system is underdetermined or inconsistent.
func (m *Mechanism) Solve(state string) (map[string]float64, bool) {
	idx, n := m.index(), len(m.order)
	rows := m.matrix(state)
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
// right number of times — in every state it can be shifted into.
func (m *Mechanism) CheckDOF() []Finding {
	var out []Finding
	for _, state := range m.statesToCheck() {
		for _, f := range m.checkDOFIn(state) {
			f.Detail = withState(state, f.Detail)
			out = append(out, f)
		}
	}
	return out
}

// withState labels a finding when there is more than one state to tell apart.
func withState(state, detail string) string {
	if state == "" {
		return detail
	}
	return fmt.Sprintf("in %q: %s", state, detail)
}

func (m *Mechanism) checkDOFIn(state string) []Finding {
	d, k := m.DOF(state), len(m.inOrder)
	switch {
	case d == 0:
		return []Finding{{Level: "FAIL", Check: "dof",
			Detail: "mechanism has 0 degrees of freedom: the train is locked and cannot turn"}}
	case k < d:
		return []Finding{{Level: "WARN", Check: "dof", Detail: fmt.Sprintf(
			"%d degrees of freedom but %d driven shafts — %d motion(s) remain undetermined",
			d, k, d-k)}}
	case k > d:
		return []Finding{{Level: "FAIL", Check: "dof", Detail: fmt.Sprintf(
			"%d drives on %d degrees of freedom — overdetermined, the motors work against each other",
			k, d)}}
	}
	return []Finding{{Level: "OK", Check: "dof", Detail: fmt.Sprintf(
		"%d degrees of freedom, %d drives: determined", d, k)}}
}

// Which tooth counts can be shifted is a fact about the parts, so it is kept
// with them in internal/clutch and read from there. It was wrong here for a
// while — 24 was on the list, and no driving ring has ever gripped a 24t gear.

// CheckShiftable reports gears on a shifted shaft that no driving ring can
// engage.
//
// This is a warning rather than a failure because the engine does not model the
// selector: it knows a coupling exists, not how you mean to work it. But if the
// intent is a driving ring, these are the gears that cannot.
func (m *Mechanism) CheckShiftable() []Finding {
	// Only the side being gripped. A coupling's A rides the ring and its B is
	// the gear the ring locks to, so a gear fixed to A needs no clutch of its
	// own — it turns with the shaft whatever the ring is doing. Flagging both
	// sides warned about the driving gears of the next stage in a compound
	// gearbox, which are nobody's business.
	shifted := map[string]bool{}
	for _, l := range m.Links {
		c, ok := l.(Coupling)
		if !ok || len(c.States) == 0 {
			continue // permanent couplings are not shifts
		}
		shifted[c.B] = true
	}
	if len(shifted) == 0 {
		return nil
	}

	var out []Finding
	for _, l := range m.Links {
		mesh, ok := l.(Mesh)
		if !ok {
			continue
		}
		for _, side := range []struct {
			shaft string
			teeth int
		}{{mesh.A, mesh.TeethA}, {mesh.B, mesh.TeethB}} {
			if !shifted[side.shaft] || clutch.Shiftable(side.teeth) {
				continue
			}
			out = append(out, Finding{Level: "WARN", Check: "shiftable", Detail: fmt.Sprintf(
				"shaft '%s' is shifted but carries a %dt, and no driving ring has a "+
					"clutch gear that size. %v can be shifted; use one of those, or "+
					"shift this another way.",
				side.shaft, side.teeth, clutch.ShiftableTeeth())})
		}
	}
	if len(out) == 0 {
		out = append(out, Finding{Level: "OK", Check: "shiftable",
			Detail: "every shifted gear has a driving-ring variant"})
	}
	return out
}

// CheckStates reports what each state does, which for a gearbox is the ratio it
// selects. A state that leaves the output unsolvable is a state that does not
// select anything.
func (m *Mechanism) CheckStates() []Finding {
	if len(m.states) == 0 {
		return nil
	}
	var out []Finding
	for _, state := range m.states {
		sol, ok := m.Solve(state)
		if !ok {
			out = append(out, Finding{Level: "WARN", Check: "shift", Detail: withState(state,
				"the speeds do not resolve; this state selects nothing definite")})
			continue
		}
		out = append(out, Finding{Level: "OK", Check: "shift", Detail: withState(state, ratioReport(m, sol))})
	}
	return out
}

// ratioReport describes the outputs in terms of the inputs.
func ratioReport(m *Mechanism, sol map[string]float64) string {
	if len(m.Outputs) == 0 || len(m.inOrder) == 0 {
		return "solved"
	}
	in := sol[m.inOrder[0]]
	parts := make([]string, 0, len(m.Outputs))
	for _, out := range m.Outputs {
		if in == 0 {
			parts = append(parts, fmt.Sprintf("%s at %.3f", out, sol[out]))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s turns %.3f per turn of %s",
			out, sol[out]/in, m.inOrder[0]))
	}
	return strings.Join(parts, ", ")
}

// CheckBearings wants two bearing points per shaft. Fewer than two means it
// whips under load.
func (m *Mechanism) CheckBearings() []Finding {
	var out []Finding
	for _, id := range m.order {
		s := m.shafts[id]
		if s.Bearings < 2 {
			out = append(out, Finding{Level: "FAIL", Check: "bearings", Detail: fmt.Sprintf(
				"shaft '%s' has %d bearing point(s). Fewer than two means it whips under load.",
				s.ID, s.Bearings)})
		}
	}
	if len(out) == 0 {
		return []Finding{{Level: "OK", Check: "bearings", Detail: "every shaft borne at both ends"}}
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
			out = append(out, Finding{Level: "FAIL", Check: "grid", Detail: fmt.Sprintf(
				"transmission between %v crosses grid domains %v. Technic bricks sit at "+
					"24 LDU vertically, liftarms at 20 — the holes do not line up.",
				ids, domains)})
		}
	}
	if len(out) == 0 {
		return []Finding{{Level: "OK", Check: "grid", Detail: "one grid domain, no transitions"}}
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
		out = append(out, Finding{Level: "FAIL", Check: "center dist", Detail: fmt.Sprintf(
			"%s/%s: %dt and %dt mesh at %g half studs, which is off the lattice. "+
				"Tooth counts have to sum to a multiple of 8, and %d+%d = %d does not. "+
				"Pick another pair or put an idler between them.",
			mesh.A, mesh.B, mesh.TeethA, mesh.TeethB, d,
			mesh.TeethA, mesh.TeethB, mesh.TeethA+mesh.TeethB)})
	}
	if len(out) == 0 {
		return []Finding{{Level: "OK", Check: "center dist", Detail: "every spur pair lands on a whole half stud"}}
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
					out = append(out, Finding{Level: "FAIL", Check: "loop closure", Detail: fmt.Sprintf(
						"%s-%s-%s: center distances %g/%g/%g half studs do not form a triangle",
						a, b, c, dab, dbc, dac)})
					continue
				}
				// Third point, with A at (0,0) and B at (dab,0).
				x := (dab*dab + dac*dac - dbc*dbc) / (2 * dab)
				y := math.Sqrt(math.Max(dac*dac-x*x, 0))
				if math.Abs(x-math.Round(x)) < 1e-9 && math.Abs(y-math.Round(y)) < 1e-9 {
					out = append(out, Finding{Level: "OK", Check: "loop closure", Detail: fmt.Sprintf(
						"%s-%s-%s closes on the lattice: third shaft at (%.0f, %.0f) half studs",
						a, b, c, x, y)})
				} else {
					out = append(out, Finding{Level: "FAIL", Check: "loop closure", Detail: fmt.Sprintf(
						"%s-%s-%s does NOT close on the lattice: the third shaft would land at "+
							"(%.3f, %.3f) half studs. Pick a different tooth count or add an idler.",
						a, b, c, x, y)})
				}
			}
		}
	}
	if len(out) == 0 {
		return []Finding{{Level: "OK", Check: "loop closure", Detail: "no gear loops present"}}
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
	out = append(out, m.CheckStates()...)
	out = append(out, m.CheckShiftable()...)
	if m.shiftPoints != nil {
		out = append(out, m.CheckShiftPoints(*m.shiftPoints)...)
	}
	return out
}
