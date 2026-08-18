// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package layout is the geometric layer: from a functional graph to shafts on
// the lattice.
//
// The functional layer says what is connected to what. This works out where
// those shafts lie. Every transmission imposes a requirement:
//
//	spur meshing   ->  shafts parallel, perpendicular distance (ta+tb)/8 half studs
//	bevel meshing  ->  shafts perpendicular AND intersecting
//	differential   ->  all three ports on the same line
//
// That is a constraint problem on a lattice, so finite and enumerable. No
// optimization is needed, only a search with backtracking.
//
// The unit is the half stud (10 LDU). A center distance lands on a whole half
// stud when the two tooth counts SUM to a multiple of 8, which covers most
// pairs but not all: 8t+12t and 36t+40t fall on a quarter stud instead.
package layout

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"brickmesh/internal/clutch"
	"brickmesh/internal/geom"
	"brickmesh/internal/mech"
)

// AxisDirs are the canonical shaft directions.
var AxisDirs = []geom.Vec3{{X: 1}, {Y: 1}, {Z: 1}}

// PythagoreanDirs can be switched on to allow angled shafts. They cost search
// space and rarely come out more compact.
var PythagoreanDirs = []geom.Vec3{
	{X: 0.8, Y: 0.6}, {X: 0.6, Y: 0.8},
	{X: 0.8, Z: 0.6}, {X: 0.6, Z: 0.8},
	{Y: 0.8, Z: 0.6}, {Y: 0.6, Z: 0.8},
}

// SumOfTwoSquares returns every (a,b) with a^2+b^2 = n, signs and order
// included. These are the lattice offsets that put two parallel shafts exactly
// sqrt(n) half studs apart.
func SumOfTwoSquares(n int) [][2]int {
	if n < 0 {
		return nil
	}
	seen := map[[2]int]bool{}
	r := int(math.Sqrt(float64(n)))
	for a := -r - 1; a <= r+1; a++ {
		b2 := n - a*a
		if b2 < 0 {
			continue
		}
		b := int(math.Round(math.Sqrt(float64(b2))))
		if b*b != b2 {
			continue
		}
		seen[[2]int{a, b}] = true
		if b != 0 {
			seen[[2]int{a, -b}] = true
		}
	}
	out := make([][2]int, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}

// Placement is a shaft as an infinite line: a point on it plus a direction.
// Point is in half studs.
type Placement struct {
	Point     geom.Vec3
	Direction geom.Vec3
}

// NewPlacement normalizes the direction.
func NewPlacement(point, direction geom.Vec3) Placement {
	return Placement{Point: point, Direction: direction.Unit()}
}

// Key identifies a placement for deduplication.
func (p Placement) Key() [6]float64 {
	r := func(v float64) float64 { return math.Round(v*1e6) / 1e6 }
	return [6]float64{
		r(p.Point.X), r(p.Point.Y), r(p.Point.Z),
		r(p.Direction.X), r(p.Direction.Y), r(p.Direction.Z),
	}
}

func perpBasis(d geom.Vec3) (geom.Vec3, geom.Vec3) {
	tmp := geom.Vec3{X: 1}
	if math.Abs(d.X) >= 0.9 {
		tmp = geom.Vec3{Y: 1}
	}
	u := d.Cross(tmp).Unit()
	return u, d.Cross(u)
}

// ParallelDistance is the perpendicular offset between two parallel shafts.
// ok is false when they are not parallel.
func ParallelDistance(p, q Placement) (float64, bool) {
	if math.Abs(math.Abs(p.Direction.Dot(q.Direction))-1) > 1e-9 {
		return 0, false
	}
	v := q.Point.Sub(p.Point)
	return v.Sub(p.Direction.Scale(v.Dot(p.Direction))).Len(), true
}

// AxesIntersect reports whether two non-parallel shafts meet at a point, which
// a bevel pair requires.
func AxesIntersect(p, q Placement) bool {
	n := p.Direction.Cross(q.Direction)
	if n.Len() < 1e-6 {
		return false
	}
	return math.Abs(q.Point.Sub(p.Point).Dot(n.Unit())) < 1e-6
}

func Perpendicular(p, q Placement) bool {
	return math.Abs(p.Direction.Dot(q.Direction)) < 1e-9
}

// LineDistance is the shortest distance between two infinite lines, in half
// studs.
func LineDistance(p, q Placement) float64 {
	n := p.Direction.Cross(q.Direction)
	if n.Len() < 1e-9 { // parallel
		v := q.Point.Sub(p.Point)
		return v.Sub(p.Direction.Scale(v.Dot(p.Direction))).Len()
	}
	return math.Abs(q.Point.Sub(p.Point).Dot(n.Unit()))
}

// Layout is one solution: a placement for every shaft.
type Layout struct {
	Mech  *mech.Mechanism
	Place map[string]Placement
}

// BBoxVolume is the bounding volume, used to prefer compact solutions.
func (l *Layout) BBoxVolume() float64 {
	if len(l.Place) == 0 {
		return 0
	}
	first := true
	var lo, hi geom.Vec3
	for _, p := range l.Place {
		if first {
			lo, hi, first = p.Point, p.Point, false
			continue
		}
		lo = geom.Vec3{X: math.Min(lo.X, p.Point.X), Y: math.Min(lo.Y, p.Point.Y),
			Z: math.Min(lo.Z, p.Point.Z)}
		hi = geom.Vec3{X: math.Max(hi.X, p.Point.X), Y: math.Max(hi.Y, p.Point.Y),
			Z: math.Max(hi.Z, p.Point.Z)}
	}
	s := hi.Sub(lo)
	return (s.X + 1) * (s.Y + 1) * (s.Z + 1)
}

// Satisfied reports whether a link's geometric requirement holds between two
// placed shafts.
func (l *Layout) Satisfied(link mech.Link, a, b string) bool {
	p, q := l.Place[a], l.Place[b]
	m, ok := link.(mech.Mesh)
	if !ok {
		return true
	}
	switch m.Kind {
	case mech.Spur:
		d, parallel := ParallelDistance(p, q)
		if !parallel {
			return false
		}
		want, _ := m.CenterDistanceHalfStuds()
		return math.Abs(d-want) < 1e-6
	case mech.Bevel:
		return Perpendicular(p, q) && AxesIntersect(p, q)
	case mech.Worm:
		return Perpendicular(p, q)
	}
	return true
}

// Options tunes the search.
type Options struct {
	Seed         string
	AllowAngled  bool
	MaxSolutions int
	Span         int
	Radius       map[string]float64
}

// classing is the shaft graph collapsed to classes: differential ports share a
// line, so they move together.
type classing struct {
	classes map[string][]string
	reps    []string
	adj     map[string]map[string]bool
	find    func(string) string
}

func classify(m *mech.Mechanism, shafts []string) classing {
	parent := map[string]string{}
	for _, s := range shafts {
		parent[s] = s
	}
	find := func(x string) string {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	// Ports of a differential share a line, and so do the two halves of a
	// coupling: a gear that a dog ring locks to a shaft is riding on it.
	for _, l := range m.Links {
		switch v := l.(type) {
		case mech.Differential:
			for _, other := range []string{v.OutA, v.OutB} {
				parent[find(other)] = find(v.Case)
			}
		case mech.Coupling:
			parent[find(v.B)] = find(v.A)
		}
	}

	classes := map[string][]string{}
	var reps []string
	for _, s := range shafts {
		r := find(s)
		if _, seen := classes[r]; !seen {
			reps = append(reps, r)
		}
		classes[r] = append(classes[r], s)
	}

	adj := map[string]map[string]bool{}
	for _, r := range reps {
		adj[r] = map[string]bool{}
	}
	for _, l := range m.Links {
		mesh, ok := l.(mech.Mesh)
		if !ok {
			continue
		}
		ra, rb := find(mesh.A), find(mesh.B)
		if ra != rb {
			adj[ra][rb] = true
			adj[rb][ra] = true
		}
	}
	return classing{classes: classes, reps: reps, adj: adj, find: find}
}

// search carries the state of one Realize call.
type search struct {
	mech      *mech.Mechanism
	cl        classing
	order     []string
	dirs      []geom.Vec3
	span      int
	max       int
	radiusOf  func(string) float64
	solutions []*Layout
}

// Realize searches lattice positions for every shaft, returning solutions
// sorted by bounding volume: compact first.
func Realize(m *mech.Mechanism, opts Options) []*Layout {
	if opts.MaxSolutions <= 0 {
		opts.MaxSolutions = 20
	}
	if opts.Span <= 0 {
		opts.Span = 6
	}
	dirs := AxisDirs
	if opts.AllowAngled {
		dirs = append(append([]geom.Vec3{}, AxisDirs...), PythagoreanDirs...)
	}

	shafts := m.Order()
	if len(shafts) == 0 {
		return nil
	}
	seed := opts.Seed
	if seed == "" {
		seed = shafts[0]
	}

	cl := classify(m, shafts)
	radius := opts.Radius
	if radius == nil {
		radius = map[string]float64{}
	}

	s := &search{
		mech:  m,
		cl:    cl,
		order: breadthFirst(cl.find(seed), cl.reps, cl.adj),
		dirs:  dirs,
		span:  opts.Span,
		max:   opts.MaxSolutions,
		radiusOf: func(rep string) float64 {
			best := 1.0
			for _, id := range cl.classes[rep] {
				if r, ok := radius[id]; ok && r > best {
					best = r
				}
			}
			return best
		},
	}
	s.backtrack(0, map[string]Placement{})

	sort.SliceStable(s.solutions, func(i, j int) bool {
		return s.solutions[i].BBoxVolume() < s.solutions[j].BBoxVolume()
	})
	return s.solutions
}

func (s *search) backtrack(i int, placed map[string]Placement) {
	if len(s.solutions) >= s.max {
		return
	}
	if i == len(s.order) {
		s.record(placed)
		return
	}
	rep := s.order[i]
	for _, cand := range candidates(s.mech, rep, placed, s.cl.adj, s.cl.classes, s.dirs, s.span) {
		trial := make(map[string]Placement, len(placed)+1)
		for k, v := range placed {
			trial[k] = v
		}
		trial[rep] = cand
		if !s.meshesFit(rep, trial) || !s.clearOfPlaced(rep, cand, placed) {
			continue
		}
		s.backtrack(i+1, trial)
	}
}

// meshesFit checks every transmission between this class and the ones already
// placed.
func (s *search) meshesFit(rep string, trial map[string]Placement) bool {
	for _, other := range sortedKeys(s.cl.adj[rep]) {
		if _, isPlaced := trial[other]; !isPlaced {
			continue
		}
		for _, a := range s.cl.classes[rep] {
			for _, b := range s.cl.classes[other] {
				probe := &Layout{Mech: s.mech, Place: map[string]Placement{
					a: trial[rep], b: trial[other]}}
				for _, link := range linksBetween(s.mech, a, b) {
					if !probe.Satisfied(link, a, b) {
						return false
					}
				}
			}
		}
	}
	return true
}

// clearOfPlaced keeps shafts that do not mesh out of each other's way. Without
// it two differentials happily land on the same line — nothing in the graph
// forbids it.
func (s *search) clearOfPlaced(rep string, cand Placement, placed map[string]Placement) bool {
	for _, other := range sortedPlaced(placed) {
		if s.cl.adj[rep][other] {
			continue // meshing pair: already handled
		}
		need := s.radiusOf(rep) + s.radiusOf(other)
		if LineDistance(cand, placed[other]) < need-1e-9 {
			return false
		}
	}
	return true
}

func (s *search) record(placed map[string]Placement) {
	l := &Layout{Mech: s.mech, Place: map[string]Placement{}}
	for rep, p := range placed {
		for _, id := range s.cl.classes[rep] {
			l.Place[id] = p
		}
	}
	s.solutions = append(s.solutions, l)
}

func breadthFirst(seed string, reps []string, adj map[string]map[string]bool) []string {
	var order []string
	seen := map[string]bool{}
	queue := []string{seed}
	for len(queue) > 0 {
		r := queue[0]
		queue = queue[1:]
		if seen[r] {
			continue
		}
		seen[r] = true
		order = append(order, r)
		for _, n := range sortedKeys(adj[r]) {
			if !seen[n] {
				queue = append(queue, n)
			}
		}
	}
	for _, r := range reps {
		if !seen[r] {
			order = append(order, r)
		}
	}
	return order
}

// candidates lists where a class can go, given what is already placed.
func candidates(m *mech.Mechanism, rep string, placed map[string]Placement,
	adj map[string]map[string]bool, classes map[string][]string,
	dirs []geom.Vec3, span int) []Placement {

	var anchor string
	for _, n := range sortedKeys(adj[rep]) {
		if _, ok := placed[n]; ok {
			anchor = n
			break
		}
	}
	if anchor == "" {
		return []Placement{NewPlacement(geom.Vec3{}, AxisDirs[0])}
	}
	p := placed[anchor]

	var link mech.Link
	for _, a := range classes[anchor] {
		for _, b := range classes[rep] {
			if ls := linksBetween(m, a, b); len(ls) > 0 {
				link = ls[0]
				break
			}
		}
		if link != nil {
			break
		}
	}

	var out []Placement
	if mesh, ok := link.(mech.Mesh); ok && mesh.Kind == mech.Spur {
		// The offsets are whole half studs, so a lattice position exists only
		// when the squared center distance is a whole number. It is not when
		// the tooth counts fail to sum to a multiple of 8: 8t+12t needs 2.5
		// half studs and 36t+40t needs 9.5. Rounding the square would place the
		// gear a fraction of a stud out; mech.CheckCenterDistances reports such
		// a pair up front.
		d, _ := mesh.CenterDistanceHalfStuds()
		d2 := d * d
		if math.Abs(d2-math.Round(d2)) < 1e-9 {
			u, v := perpBasis(p.Direction)
			for _, ab := range SumOfTwoSquares(int(math.Round(d2))) {
				for t := -span; t <= span; t++ {
					pt := p.Point.
						Add(u.Scale(float64(ab[0]))).
						Add(v.Scale(float64(ab[1]))).
						Add(p.Direction.Scale(float64(t)))
					out = append(out, NewPlacement(pt, p.Direction))
				}
			}
		}
	} else {
		for _, d := range dirs { // bevel or worm: perpendicular
			if math.Abs(d.Dot(p.Direction)) > 1e-9 {
				continue
			}
			for t := -span; t <= span; t++ {
				out = append(out, NewPlacement(p.Point.Add(p.Direction.Scale(float64(t))), d))
			}
		}
	}

	seen := map[[6]float64]bool{}
	uniq := out[:0]
	for _, c := range out {
		k := c.Key()
		if seen[k] {
			continue
		}
		seen[k] = true
		uniq = append(uniq, c)
	}
	return uniq
}

func linksBetween(m *mech.Mechanism, a, b string) []mech.Link {
	var out []mech.Link
	for _, l := range m.Links {
		mesh, ok := l.(mech.Mesh)
		if !ok {
			continue
		}
		if (mesh.A == a && mesh.B == b) || (mesh.A == b && mesh.B == a) {
			out = append(out, l)
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedPlaced(m map[string]Placement) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// --------------------------------------------------------------------------
// stations: where the gears sit along each shaft
// --------------------------------------------------------------------------

// GearThickness in half studs, used to find overlap on a shaft and to leave the
// free stretches where bearings may go.
//
// Measured from the parts rather than recalled: every standard gear is one stud
// thick, 20 LDU, which is two half studs. The 24t comes out at 19.2 and the
// rest at exactly 20. An earlier table had the 16t, 24t, 36t and 40t at one
// half stud, which is half their real width and would let two of them sit 10
// LDU apart and be called clear.
var GearThickness = map[int]float64{
	8: 2, 12: 2, 16: 2, 20: 2, 24: 2, 28: 2, 36: 2, 40: 2,
}

func thicknessOf(teeth int) float64 {
	if t, ok := GearThickness[teeth]; ok {
		return t
	}
	return 2
}

// EffectiveRadius in half studs. Follows from the pitch rule: radius in studs
// is teeth/16.
func EffectiveRadius(teeth int) float64 { return float64(teeth) / 8.0 }

// Station is a gear at an axial position on a shaft.
type Station struct {
	Shaft     string
	Teeth     int
	Axial     float64 // along the shaft direction, half studs
	Thickness float64
	Origin    string // where the value came from
}

// Span is the stretch of shaft the gear occupies.
func (s Station) Span() (lo, hi float64) {
	return s.Axial - s.Thickness/2, s.Axial + s.Thickness/2
}

type stationKey struct {
	shaft string
	teeth int
	link  int
}

// stationSet keeps stations in the order they were derived, which decides which
// anchor a later pair propagates from.
type stationSet struct {
	byKey map[stationKey]Station
	order []stationKey
}

func (s *stationSet) put(k stationKey, st Station) {
	if _, exists := s.byKey[k]; !exists {
		s.order = append(s.order, k)
	}
	s.byKey[k] = st
}

// firstAnchored finds a station on a shaft whose position is absolute rather
// than chosen.
//
// Only a bevel pair anchors: its shafts intersect in a point, so its gears have
// nowhere else to be. A spur pair's plane is free, and two spur pairs sharing a
// shaft are two different gears at two different places along it — which is
// what a gearbox is. Propagating from any station at all would stack them.
func (s *stationSet) firstAnchored(shaft string) (stationKey, bool) {
	for _, k := range s.order {
		st := s.byKey[k]
		if st.Shaft == shaft && strings.HasPrefix(st.Origin, "bevel") {
			return k, true
		}
	}
	return stationKey{}, false
}

// SolveStations determines the axial position of every gear.
//
// Bevel pairs give an absolute position: the shafts intersect in a point, and
// each gear stands the effective radius of the other away from it. Parallel
// pairs give only an equality — both gears lie in the same plane. So the bevels
// are anchored first, then the equalities propagate.
func SolveStations(m *mech.Mechanism, l *Layout) ([]Station, []mech.Finding) {
	stations := &stationSet{byKey: map[stationKey]Station{}}
	findings := anchorBevels(m, l, stations)
	findings = append(findings, propagateSpurPairs(m, l, stations, shiftedShafts(m))...)

	result := mergeSharedGears(stations)
	findings = append(findings, checkOverlap(result, l)...)
	findings = append(findings, checkOnLattice(result)...)

	if len(findings) == 0 {
		findings = append(findings, mech.Finding{Level: "OK", Check: "station",
			Detail: fmt.Sprintf("%d gear stations determined, no conflicts", len(result))})
	}
	return result, findings
}

// anchorBevels fixes the pairs whose position is absolute.
func anchorBevels(m *mech.Mechanism, l *Layout, stations *stationSet) []mech.Finding {
	var findings []mech.Finding
	axialOf := func(shaft string, world geom.Vec3) float64 {
		p := l.Place[shaft]
		return world.Sub(p.Point).Dot(p.Direction)
	}

	for i, link := range m.Links {
		mesh, ok := link.(mech.Mesh)
		if !ok || mesh.Kind != mech.Bevel {
			continue
		}
		pa, pb := l.Place[mesh.A], l.Place[mesh.B]
		n := pa.Direction.Cross(pb.Direction)
		denom := pb.Direction.Cross(n).Dot(pa.Direction)
		if math.Abs(denom) < 1e-9 {
			findings = append(findings, mech.Finding{Level: "FAIL", Check: "station",
				Detail: fmt.Sprintf("%s/%s: the shafts do not intersect", mesh.A, mesh.B)})
			continue
		}
		// Intersection point of two intersecting lines.
		t := -pb.Direction.Cross(n).Dot(pa.Point.Sub(pb.Point)) / denom
		p := pa.Point.Add(pa.Direction.Scale(t))

		// Each gear at the effective radius of the other from the intersection.
		for _, side := range []struct {
			shaft        string
			teeth, other int
		}{
			{mesh.A, mesh.TeethA, mesh.TeethB},
			{mesh.B, mesh.TeethB, mesh.TeethA},
		} {
			off := EffectiveRadius(side.other)
			pos := p.Add(l.Place[side.shaft].Direction.Scale(off))
			stations.put(stationKey{side.shaft, side.teeth, i}, Station{
				Shaft: side.shaft, Teeth: side.teeth,
				Axial:     axialOf(side.shaft, pos),
				Thickness: thicknessOf(side.teeth),
				Origin: fmt.Sprintf("bevel with %dt, effective radius %g half studs",
					side.other, off),
			})
		}
	}
	return findings
}

// propagateSpurPairs puts both gears of a parallel pair in one plane.
//
// A pair with nothing anchoring it is free to sit anywhere along its shafts, so
// pairs are handed successive slots rather than all landing on zero. That is
// what a multi-speed gearbox needs: three pairs sharing an input shaft have to
// be spread along it, not stacked in one plane.
// RingRoomHalfStuds is the space a driving ring needs beside the gear it
// engages.
//
// The ring runs four half studs along its shaft and sits with its face against
// the gear's, so engaged it already reaches four half studs past the gear's
// center. Disengaging slides it one further. Reserving less than that is what
// used to leave a ring with nowhere to go but through its neighbor. The number
// comes from internal/clutch, which measures it.
//
// Taken from whichever system needs the most, since the layout runs before the
// hardware for a given shift is chosen and reserving too little is what leaves
// a ring nowhere to go.
var RingRoomHalfStuds = mostRoom()

func mostRoom() float64 {
	var most float64
	for _, s := range clutch.Systems {
		if r := s.Room(); r > most {
			most = r
		}
	}
	return most
}

// shiftedShafts are those a shift engages, which need room beside their gear
// for the ring that does the engaging.
func shiftedShafts(m *mech.Mechanism) map[string]bool {
	out := map[string]bool{}
	for _, l := range m.Links {
		c, ok := l.(mech.Coupling)
		if !ok || len(c.States) == 0 {
			continue
		}
		out[c.A] = true
		out[c.B] = true
	}
	return out
}

func propagateSpurPairs(m *mech.Mechanism, l *Layout, stations *stationSet,
	shifted map[string]bool) []mech.Finding {
	var findings []mech.Finding
	// What is already on each LINE, as spans in half studs. Keyed by line and
	// not by shaft name, because the gears of a gearbox ride on differently
	// named shafts that a coupling holds on one axis — they are as much in each
	// other's way as if they shared a name.
	used := map[[6]float64][][2]float64{}
	for i, link := range m.Links {
		mesh, ok := link.(mech.Mesh)
		if !ok || mesh.Kind != mech.Spur {
			continue
		}
		var base float64
		ka, foundA := stations.firstAnchored(mesh.A)
		kb, foundB := stations.firstAnchored(mesh.B)
		switch {
		case foundA && !foundB:
			base = stations.byKey[ka].Axial
		case foundB && !foundA:
			base = stations.byKey[kb].Axial
		case foundA && foundB:
			a, b := stations.byKey[ka].Axial, stations.byKey[kb].Axial
			if math.Abs(a-b) > 1e-6 {
				findings = append(findings, mech.Finding{Level: "FAIL", Check: "station",
					Detail: fmt.Sprintf(
						"%s/%s: both shafts already anchored in different planes "+
							"(%.2f and %.2f) - they do not mesh", mesh.A, mesh.B, a, b)})
			}
			base = a
		default:
			// Neither end is fixed, so the pair may sit anywhere along its
			// shafts: take the first whole half stud where both gears clear
			// whatever is already there. Stepping by a fixed width is not
			// enough, since the next gear along may be the thicker one.
			base = firstFreeSlot(used[lineOf(l, mesh.A)], used[lineOf(l, mesh.B)],
				thicknessOf(mesh.TeethA), thicknessOf(mesh.TeethB))
		}

		lineA, lineB := lineOf(l, mesh.A), lineOf(l, mesh.B)
		used[lineA] = append(used[lineA],
			reserve(base, thicknessOf(mesh.TeethA), shifted[mesh.A]))
		used[lineB] = append(used[lineB],
			reserve(base, thicknessOf(mesh.TeethB), shifted[mesh.B]))

		for _, side := range []struct {
			shaft string
			teeth int
		}{{mesh.A, mesh.TeethA}, {mesh.B, mesh.TeethB}} {
			k := stationKey{side.shaft, side.teeth, i}
			if _, exists := stations.byKey[k]; exists {
				continue
			}
			stations.put(k, Station{Shaft: side.shaft, Teeth: side.teeth, Axial: base,
				Thickness: thicknessOf(side.teeth), Origin: "parallel, same plane"})
		}
	}
	return findings
}

// reserve is the space a gear takes, plus room for a driving ring when the gear
// is one a shift engages. Without it three gears pack tight against each other
// and the ring for the middle one has nowhere to go.
func reserve(center, thickness float64, shifted bool) [2]float64 {
	span := spanAt(center, thickness)
	if shifted {
		span[1] += RingRoomHalfStuds
	}
	return span
}

func spanAt(center, thickness float64) [2]float64 {
	return [2]float64{center - thickness/2, center + thickness/2}
}

// firstFreeSlot is the lowest whole half stud at which a pair of gears clears
// everything already on both shafts.
func firstFreeSlot(usedA, usedB [][2]float64, thickA, thickB float64) float64 {
	const limit = 64 // a shaft that long is a different problem
	for base := 0.0; base < limit; base++ {
		if clearOf(usedA, spanAt(base, thickA)) && clearOf(usedB, spanAt(base, thickB)) {
			return base
		}
	}
	return limit
}

func clearOf(used [][2]float64, span [2]float64) bool {
	for _, u := range used {
		if math.Min(u[1], span[1])-math.Max(u[0], span[0]) > 1e-6 {
			return false
		}
	}
	return true
}

// mergeSharedGears folds duplicates together. Two gears with the same tooth
// count on the same shaft in the same plane are really ONE gear driving two
// things at once.
func mergeSharedGears(stations *stationSet) []Station {
	type mergeKey struct {
		shaft string
		teeth int
		axial float64
	}
	merged := map[mergeKey]*Station{}
	var order []mergeKey
	for _, k := range stations.order {
		st := stations.byKey[k]
		mk := mergeKey{st.Shaft, st.Teeth, math.Round(st.Axial*1e6) / 1e6}
		if existing, ok := merged[mk]; ok {
			existing.Origin += " (shared with a second mesh)"
			continue
		}
		cp := st
		merged[mk] = &cp
		order = append(order, mk)
	}
	out := make([]Station, 0, len(order))
	for _, mk := range order {
		out = append(out, *merged[mk])
	}
	return out
}

// checkOverlap catches two gears sharing the same stretch of shaft.
//
// Grouped by line rather than by shaft name. A coupling makes its two shafts
// coaxial and a differential's three ports share one axis, so gears on
// differently named shafts can still be the same place — which is exactly where
// a gearbox puts them.
func checkOverlap(stations []Station, l *Layout) []mech.Finding {
	perShaft := map[[6]float64][]Station{}
	var order [][6]float64
	for _, st := range stations {
		line := lineOf(l, st.Shaft)
		if _, seen := perShaft[line]; !seen {
			order = append(order, line)
		}
		perShaft[line] = append(perShaft[line], st)
	}

	var findings []mech.Finding
	for _, line := range order {
		sts := append([]Station(nil), perShaft[line]...)
		sort.SliceStable(sts, func(i, j int) bool { return sts[i].Axial < sts[j].Axial })
		for i := 0; i < len(sts); i++ {
			for j := i + 1; j < len(sts); j++ {
				lo1, hi1 := sts[i].Span()
				lo2, hi2 := sts[j].Span()
				if math.Min(hi1, hi2)-math.Max(lo1, lo2) > 1e-6 {
					findings = append(findings, mech.Finding{Level: "FAIL", Check: "station",
						Detail: fmt.Sprintf(
							"on the line through '%s': %dt at %.2f and %dt at %.2f overlap",
							sts[i].Shaft, sts[i].Teeth, sts[i].Axial,
							sts[j].Teeth, sts[j].Axial)})
				}
			}
		}
	}
	return findings
}

func checkOnLattice(stations []Station) []mech.Finding {
	var findings []mech.Finding
	for _, st := range stations {
		if math.Abs(st.Axial-math.Round(st.Axial)) > 1e-6 {
			findings = append(findings, mech.Finding{Level: "WARN", Check: "station",
				Detail: fmt.Sprintf("shaft '%s': %dt at %.3f half studs, not on the lattice",
					st.Shaft, st.Teeth, st.Axial)})
		}
	}
	return findings
}

// LineOf identifies the axis a shaft lies on. Shafts that a coupling or a
// differential holds coaxial share one.
func LineOf(l *Layout, shaft string) [6]float64 { return lineOf(l, shaft) }

func lineOf(l *Layout, shaft string) [6]float64 {
	if l == nil {
		return [6]float64{}
	}
	if p, ok := l.Place[shaft]; ok {
		return p.Key()
	}
	return [6]float64{}
}

// FreeIntervals are the stretches of a shaft with no gear on them: where the
// bearings can go.
func FreeIntervals(stations []Station, shaft string, reach float64) [][2]float64 {
	return FreeIntervalsWith(stations, shaft, reach, nil)
}

// FreeIntervalsWith is FreeIntervals with more of the shaft already spoken for.
//
// A driving ring and the joiner it slides on take up shaft that carries no gear
// at all, and nothing may be pinned through it: a joiner is 20 LDU across and a
// beam's hole is 12, so a bearing there is not a tight fit but an impossibility.
func FreeIntervalsWith(stations []Station, shaft string, reach float64,
	taken [][2]float64) [][2]float64 {

	occ := append([][2]float64(nil), taken...)
	for _, s := range stations {
		if s.Shaft != shaft {
			continue
		}
		lo, hi := s.Span()
		occ = append(occ, [2]float64{lo, hi})
	}
	sort.Slice(occ, func(i, j int) bool {
		if occ[i][0] != occ[j][0] {
			return occ[i][0] < occ[j][0]
		}
		return occ[i][1] < occ[j][1]
	})

	var free [][2]float64
	cursor, bounded := -reach, false
	for _, span := range occ {
		if span[0]-cursor > 0.5 {
			free = append(free, shrink([2]float64{cursor, span[0]}, bounded, true))
		}
		cursor, bounded = math.Max(cursor, span[1]), true
	}
	if reach-cursor > 0.5 {
		free = append(free, shrink([2]float64{cursor, reach}, bounded, false))
	}
	out := free[:0]
	for _, f := range free {
		// A gap exactly a beam wide leaves one place to put it, in the middle,
		// which comes out of the shrink as a single point rather than a range.
		if f[1] >= f[0] {
			out = append(out, f)
		}
	}
	return out
}

// BearingHalf is half the thickness of the beam that will provide a bearing,
// in half studs.
const BearingHalf = 1.0

// shrink pulls a gap in by half a beam at each end that something sits against.
//
// A bearing is a point, but the beam giving it is a stud thick, so one placed
// hard against a gear ends up half inside it — which is what put liftarms at
// the same place on a shaft as the gears.
//
// Only at the ends that abut something. The far ends are the limit of how far
// along the shaft we bothered to look, and nothing is there to be inside of;
// pulling in from those would only shorten the bearing base, and a short
// bearing base lets the shaft whip.
func shrink(span [2]float64, loBounded, hiBounded bool) [2]float64 {
	if loBounded {
		span[0] += BearingHalf
	}
	if hiBounded {
		span[1] -= BearingHalf
	}
	return span
}
