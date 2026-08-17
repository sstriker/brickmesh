// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package connect joins two loose pieces of a structure.
//
// Two parts can only meet directly when their ports coincide with the same
// direction. In a real mechanism that is rare: the bearings carry shafts
// perpendicular to each other, so their holes point every which way.
//
// What is needed is a CHAIN of intermediate parts carrying one port direction
// over to the other. That is a Steiner tree problem, and between two pieces it
// reduces to a shortest path: every part added widens the set of reachable
// ports, and the cheapest route is the one that first touches a port of the
// other piece.
//
// A* with, as its heuristic, the remaining distance divided by how far a part
// can reach. Without a heuristic the search wanders: there are close to a
// thousand placements per port.
package connect

import (
	"container/heap"
	"math"
	"sort"

	"brickmesh/internal/catalog"
	"brickmesh/internal/geom"
	"brickmesh/internal/voxel"
)

const (
	// MaxReach is the longest part in the inventory, in LDU.
	MaxReach = 220.0
	// heuristicWeight scales the estimate. Without it A* degenerates to
	// breadth-first and the frontier explodes.
	heuristicWeight = 6.0
	// contactFraction separates touching from intersecting, as in the
	// structural search.
	contactFraction = 0.12
	// frontierWidth is how many ports of a partial chain are expanded. Sorted
	// by distance to the target first, so this keeps the nearest rather than an
	// arbitrary handful.
	frontierWidth = 8
)

// Port is a connection point in world coordinates.
type Port struct {
	Pos    geom.Vec3
	Axis   geom.Vec3
	Kind   catalog.PortKind
	Gender catalog.Gender
}

// portKey identifies a port for the frontier. Holes have no direction, so the
// axis is made sign-free.
type portKey struct{ pos, axis geom.Vec3 }

func keyOf(pos, axis geom.Vec3) portKey {
	a := axis.Unit()
	for _, v := range [3]float64{a.X, a.Y, a.Z} {
		if math.Abs(v) > 1e-9 {
			if v < 0 {
				a = a.Scale(-1)
			}
			break
		}
	}
	return portKey{pos.Round(2), a.Round(3)}
}

// PortsOf returns a placed part's ports in world coordinates.
func PortsOf(p catalog.Placement) []Port {
	r := geom.Rotations[p.Rot]
	out := make([]Port, 0, len(p.Part.Ports))
	for _, port := range p.Part.Ports {
		out = append(out, Port{
			Pos:    r.Apply(port.Pos).Add(p.Origin),
			Axis:   r.Apply(port.Axis),
			Kind:   port.Kind,
			Gender: port.Gender,
		})
	}
	return out
}

// Options tunes the search.
type Options struct {
	MaxParts int
	Beam     int
	// Blocked is space the chain may not occupy, beyond the two pieces.
	Blocked map[geom.Cell]bool
	// Cost is what adding a part is worth. Defaults to one each.
	Cost func(catalog.Placement) float64
}

// Connect returns the cheapest chain of parts tying piece A to piece B, or nil
// when there is none within the limits.
func Connect(cat *catalog.Catalog, rast *voxel.Rasterizer,
	pieceA, pieceB []catalog.Placement, opts Options) ([]catalog.Placement, error) {

	if opts.MaxParts <= 0 {
		opts.MaxParts = 3
	}
	if opts.Beam <= 0 {
		opts.Beam = 14
	}
	if opts.Cost == nil {
		opts.Cost = func(catalog.Placement) float64 { return 1 }
	}

	portsA, portsB := allPorts(pieceA), allPorts(pieceB)
	if len(portsA) == 0 || len(portsB) == 0 {
		return nil, nil
	}

	s := &search{
		cat: cat, rast: rast, opts: opts,
		targets: portsB,
		goal:    map[portKey]bool{},
		seen:    map[string]bool{},
		cells:   map[catalog.Placement][]geom.Cell{},
	}
	for _, p := range portsB {
		s.goal[keyOf(p.Pos, p.Axis)] = true
	}
	base, err := s.occupiedBy(append(append([]catalog.Placement{}, pieceA...), pieceB...))
	if err != nil {
		return nil, err
	}
	for c := range opts.Blocked {
		base[c] = true
	}
	s.base = base

	start := make([]portKey, 0, len(portsA))
	for _, p := range portsA {
		start = append(start, keyOf(p.Pos, p.Axis))
	}
	return s.run(start)
}

func allPorts(piece []catalog.Placement) []Port {
	var out []Port
	for _, p := range piece {
		out = append(out, PortsOf(p)...)
	}
	return out
}

type search struct {
	cat     *catalog.Catalog
	rast    *voxel.Rasterizer
	opts    Options
	targets []Port
	goal    map[portKey]bool
	base    map[geom.Cell]bool
	seen    map[string]bool
	cells   map[catalog.Placement][]geom.Cell
	counter int
}

// node is one partial chain.
type node struct {
	f, g     float64
	order    int
	chain    []catalog.Placement
	frontier []portKey
}

type queue []*node

func (q queue) Len() int      { return len(q) }
func (q queue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q queue) Less(i, j int) bool {
	if q[i].f != q[j].f {
		return q[i].f < q[j].f
	}
	return q[i].order < q[j].order
}
func (q *queue) Push(x any) { *q = append(*q, x.(*node)) }
func (q *queue) Pop() any {
	old := *q
	n := old[len(old)-1]
	*q = old[:len(old)-1]
	return n
}

func (s *search) run(start []portKey) ([]catalog.Placement, error) {
	q := &queue{{f: 0, g: 0, frontier: sortedKeys(start)}}
	heap.Init(q)

	for q.Len() > 0 {
		cur := heap.Pop(q).(*node)
		sig := signature(cur.frontier)
		if s.seen[sig] {
			continue
		}
		s.seen[sig] = true

		if s.reachesGoal(cur.frontier) {
			return cur.chain, nil
		}
		if len(cur.chain) >= s.opts.MaxParts {
			continue
		}
		if err := s.expand(q, cur); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (s *search) reachesGoal(frontier []portKey) bool {
	for _, k := range frontier {
		if s.goal[k] {
			return true
		}
	}
	return false
}

// expand pushes every worthwhile extension of a chain.
func (s *search) expand(q *queue, cur *node) error {
	occupied, err := s.occupiedBy(cur.chain)
	if err != nil {
		return err
	}
	for c := range s.base {
		occupied[c] = true
	}

	cands, err := s.candidates(cur, occupied)
	if err != nil {
		return err
	}
	scored := make([]scoredCandidate, 0, len(cands))
	for _, c := range cands {
		scored = append(scored, scoredCandidate{placement: c, h: s.heuristic(c)})
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].h < scored[j].h })
	if len(scored) > s.opts.Beam {
		scored = scored[:s.opts.Beam]
	}

	for _, sc := range scored {
		g := cur.g + s.opts.Cost(sc.placement)
		s.counter++
		heap.Push(q, &node{
			f:        g + sc.h,
			g:        g,
			order:    s.counter,
			chain:    append(append([]catalog.Placement{}, cur.chain...), sc.placement),
			frontier: mergeFrontier(cur.frontier, sc.placement),
		})
	}
	return nil
}

type scoredCandidate struct {
	placement catalog.Placement
	h         float64
}

// candidates are the parts that can attach to the nearest ports of the chain:
// a pin into a reachable hole, or hole on hole with a pin between.
func (s *search) candidates(cur *node, occupied map[geom.Cell]bool) ([]catalog.Placement, error) {
	frontier := s.nearestFirst(cur.frontier)
	if len(frontier) > frontierWidth {
		frontier = frontier[:frontierWidth]
	}

	var out []catalog.Placement
	var buf []catalog.Placement
	for _, k := range frontier {
		for _, want := range []struct {
			gender catalog.Gender
			kind   catalog.PortKind
		}{
			{catalog.Male, catalog.Round},
			{catalog.Female, catalog.Round},
		} {
			buf = s.cat.Lookup(k.pos, k.axis, want.gender, want.kind, buf)
			for _, pl := range buf {
				if inChain(cur.chain, pl) {
					continue
				}
				ok, err := s.compatible(pl, occupied)
				if err != nil {
					return nil, err
				}
				if ok {
					out = append(out, pl)
				}
			}
		}
	}
	return out, nil
}

// nearestFirst sorts the frontier by distance to the closest target, so the
// ports most likely to lead somewhere are expanded. Truncating arbitrarily
// would throw away precisely the useful ones.
func (s *search) nearestFirst(frontier []portKey) []portKey {
	out := append([]portKey(nil), frontier...)
	dist := make(map[portKey]float64, len(out))
	for _, k := range out {
		best := math.Inf(1)
		for _, t := range s.targets {
			if d := k.pos.Sub(t.Pos).Len(); d < best {
				best = d
			}
		}
		dist[k] = best
	}
	sort.SliceStable(out, func(i, j int) bool { return dist[out[i]] < dist[out[j]] })
	return out
}

// heuristic estimates what is left to bridge.
//
// Only ports sharing an axis can ever be connected. Measuring distance without
// that distinction rewards exactly the candidates sitting uselessly on top of
// the target with a hole across it, and prunes away the perpendicular
// connectors that do solve it.
func (s *search) heuristic(pl catalog.Placement) float64 {
	ports := PortsOf(pl)
	if len(ports) == 0 {
		return math.Inf(1)
	}
	best := math.Inf(1)
	bestAlign := 0.0
	for _, p := range ports {
		pa := p.Axis.Unit()
		for _, t := range s.targets {
			align := math.Abs(pa.Dot(t.Axis.Unit()))
			if align > bestAlign {
				bestAlign = align
			}
			if align < 1-1e-6 {
				continue
			}
			if d := p.Pos.Sub(t.Pos).Len(); d < best {
				best = d
			}
		}
	}
	if math.IsInf(best, 1) {
		// No axis matches at all: score on how close the directions come, since
		// that is what a next link has to fix.
		return heuristicWeight * (1 + (1 - bestAlign))
	}
	return heuristicWeight * best / MaxReach
}

func (s *search) compatible(pl catalog.Placement, occupied map[geom.Cell]bool) (bool, error) {
	cells, err := s.cellsOf(pl)
	if err != nil {
		return false, err
	}
	shared := 0
	for _, c := range cells {
		if occupied[c] {
			shared++
		}
	}
	if shared == 0 {
		return true, nil
	}
	smaller := len(cells)
	if len(occupied) < smaller {
		smaller = len(occupied)
	}
	return float64(shared) <= contactFraction*float64(smaller), nil
}

func (s *search) occupiedBy(parts []catalog.Placement) (map[geom.Cell]bool, error) {
	out := map[geom.Cell]bool{}
	for _, p := range parts {
		cells, err := s.cellsOf(p)
		if err != nil {
			return nil, err
		}
		for _, c := range cells {
			out[c] = true
		}
	}
	return out, nil
}

func (s *search) cellsOf(p catalog.Placement) ([]geom.Cell, error) {
	if c, ok := s.cells[p]; ok {
		return c, nil
	}
	rel, err := s.rast.Voxels(p.Part.ID, int(p.Rot))
	if err != nil {
		return nil, err
	}
	shift := geom.Cell{
		X: int32(math.Round(p.Origin.X / voxel.Pitch)),
		Y: int32(math.Round(p.Origin.Y / voxel.Pitch)),
		Z: int32(math.Round(p.Origin.Z / voxel.Pitch)),
	}
	out := make([]geom.Cell, len(rel))
	for i, c := range rel {
		out[i] = c.Add(shift)
	}
	s.cells[p] = out
	return out, nil
}

func inChain(chain []catalog.Placement, p catalog.Placement) bool {
	for _, c := range chain {
		if c == p {
			return true
		}
	}
	return false
}

// mergeFrontier adds a part's ports to the reachable set.
func mergeFrontier(frontier []portKey, pl catalog.Placement) []portKey {
	seen := make(map[portKey]bool, len(frontier))
	out := append([]portKey(nil), frontier...)
	for _, k := range frontier {
		seen[k] = true
	}
	for _, p := range PortsOf(pl) {
		k := keyOf(p.Pos, p.Axis)
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return sortedKeys(out)
}

func sortedKeys(keys []portKey) []portKey {
	out := append([]portKey(nil), keys...)
	sort.SliceStable(out, func(i, j int) bool { return lessKey(out[i], out[j]) })
	return out
}

func lessKey(a, b portKey) bool {
	for _, pair := range [][2]float64{
		{a.pos.X, b.pos.X}, {a.pos.Y, b.pos.Y}, {a.pos.Z, b.pos.Z},
		{a.axis.X, b.axis.X}, {a.axis.Y, b.axis.Y}, {a.axis.Z, b.axis.Z},
	} {
		if pair[0] != pair[1] {
			return pair[0] < pair[1]
		}
	}
	return false
}

// signature identifies a frontier so the same reachable set is not explored
// twice.
func signature(keys []portKey) string {
	b := make([]byte, 0, len(keys)*16)
	for _, k := range keys {
		b = appendFloat(b, k.pos.X)
		b = appendFloat(b, k.pos.Y)
		b = appendFloat(b, k.pos.Z)
		b = appendFloat(b, k.axis.X)
		b = appendFloat(b, k.axis.Y)
		b = appendFloat(b, k.axis.Z)
	}
	return string(b)
}

func appendFloat(b []byte, v float64) []byte {
	bits := math.Float64bits(v)
	for i := 0; i < 8; i++ {
		b = append(b, byte(bits>>(8*i)))
	}
	return b
}
