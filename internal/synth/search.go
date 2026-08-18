// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package synth

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"sort"
	"sync"

	"brickmesh/internal/geom"
	"brickmesh/internal/layout"
	"brickmesh/internal/progress"
	"brickmesh/internal/voxel"
)

// HalfStud in LDU.
const HalfStud = 10.0

// ContactFraction is how much overlap counts as touching rather than
// intersecting.
//
// Two parts joined by a pin lie flat against each other and their surfaces
// coincide. On a 5 LDU grid that cannot be told apart from intersecting. So not
// an absolute requirement of zero shared cells, but a threshold: touching
// shares an edge, intersecting shares a substantial part of the smaller part.
const ContactFraction = 0.12

// Requirement is a bearing point a shaft needs.
type Requirement struct {
	Shaft     string
	Point     geom.Vec3
	Direction geom.Vec3
}

// BearingRequirements picks two bearing points per shaft, as far apart as the
// free stretches allow. Far apart, because a short bearing base lets the shaft
// whip anyway.
func BearingRequirements(l *layout.Layout, stations []layout.Station,
	perShaft int, reach float64) []Requirement {

	return BearingRequirementsWith(l, stations, perShaft, reach, nil)
}

// onLine relabels every station on a placement's line as belonging to the shaft
// being asked about, so the free-interval search sees all of them.
func onLine(l *layout.Layout, stations []layout.Station,
	pl layout.Placement) []layout.Station {

	out := make([]layout.Station, 0, len(stations))
	for _, st := range stations {
		p, ok := l.Place[st.Shaft]
		if !ok || p.Key() != pl.Key() {
			continue
		}
		st.Shaft = "" // the caller asks by shaft; on a line they are all one
		out = append(out, st)
	}
	return out
}

// BearingRequirementsWith is BearingRequirements told what else is on the
// shafts, keyed by shaft, in half studs.
func BearingRequirementsWith(l *layout.Layout, stations []layout.Station,
	perShaft int, reach float64, taken map[string][][2]float64) []Requirement {

	if perShaft <= 0 {
		perShaft = 2
	}
	if reach <= 0 {
		reach = 8
	}

	shafts := make([]string, 0, len(l.Place))
	for id := range l.Place {
		shafts = append(shafts, id)
	}
	sort.Strings(shafts)

	var reqs []Requirement
	for _, id := range shafts {
		pl := l.Place[id]
		// Judged along the line rather than the named shaft. Two shafts a
		// coupling holds together are the same piece of axle, so a gear on one
		// blocks the other just as surely — and a bearing was being asked for
		// exactly where another shaft's gear already sat.
		points := latticePointsAlong(pl,
			layout.FreeIntervalsWith(onLine(l, stations, pl), "", reach, taken[id]))
		if len(points) < perShaft {
			continue
		}
		chosen := points
		if perShaft == 2 {
			chosen = []geom.Vec3{points[0], points[len(points)-1]}
		} else if len(points) > perShaft {
			chosen = points[:perShaft]
		}
		for _, w := range chosen {
			reqs = append(reqs, Requirement{Shaft: id, Point: w, Direction: pl.Direction})
		}
	}
	return dedupeRequirements(reqs)
}

// latticePointsAlong walks the free stretches of a shaft and keeps the whole
// half-stud positions.
func latticePointsAlong(pl layout.Placement, free [][2]float64) []geom.Vec3 {
	var out []geom.Vec3
	for _, span := range free {
		for t := math.Ceil(span[0]); t <= math.Floor(span[1]); t++ {
			w := pl.Point.Scale(HalfStud).Add(pl.Direction.Scale(t * HalfStud))
			if w.OnLattice(HalfStud) {
				out = append(out, w)
			}
		}
	}
	return out
}

// dedupeRequirements folds the differential ports together: they share a line,
// so their bearing requirements coincide.
func dedupeRequirements(reqs []Requirement) []Requirement {
	type key struct{ p, a geom.Vec3 }
	seen := map[key]bool{}
	out := reqs[:0]
	for _, r := range reqs {
		k := key{r.Point.Round(3), absVec(r.Direction).Round(3)}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	return out
}

func absVec(v geom.Vec3) geom.Vec3 {
	return geom.Vec3{X: math.Abs(v.X), Y: math.Abs(v.Y), Z: math.Abs(v.Z)}
}

// Searcher finds structures that bear a layout's shafts.
type Searcher struct {
	Rast      *voxel.Rasterizer
	Axes      AxisSource
	Inventory []Beam
	// Taken is shaft already spoken for by something that is not a gear — a
	// driving ring and the joiner under it — in half studs, keyed by shaft.
	// Nothing may be pinned through it.
	Taken map[string][][2]float64
	// Reserved is space no beam may enter at all, however little of it.
	//
	// Unlike the ordinary overlap rule this admits no contact fraction: the
	// parts that live here turn, and a beam that shares a cell with a turning
	// part is not touching it, it is inside it.
	Reserved map[geom.Cell]bool

	counts map[string]int
	rots   map[string][]int
	axes   map[string]geom.Vec3
	mu     sync.Mutex
}

func NewSearcher(r *voxel.Rasterizer, axes AxisSource, inventory []Beam) *Searcher {
	if inventory == nil {
		inventory = Beams
	}
	return &Searcher{
		Rast: r, Axes: axes, Inventory: inventory,
		counts: HoleCounts(inventory),
		rots:   map[string][]int{},
		axes:   map[string]geom.Vec3{},
	}
}

func (s *Searcher) rotations(part string) ([]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.rots[part]; ok {
		return r, nil
	}
	r, err := s.Rast.DistinctRotations(part)
	if err != nil {
		return nil, err
	}
	s.rots[part] = r
	return r, nil
}

func (s *Searcher) localAxis(part string) (geom.Vec3, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.axes[part]; ok {
		return a, nil
	}
	a, err := LocalHoleAxis(s.Axes, part)
	if err != nil {
		return geom.Vec3{}, err
	}
	s.axes[part] = a
	return a, nil
}

// CandidatesFor lists every way to lay a load-bearing part so that one of its
// holes lands on point with the hole axis along direction.
func (s *Searcher) CandidatesFor(point, direction geom.Vec3) ([]Placed, error) {
	d := direction.Unit()
	var out []Placed
	for _, beam := range s.Inventory {
		local, err := s.localAxis(beam.Part)
		if err != nil {
			continue // no shadow data: cannot place it responsibly
		}
		rots, err := s.rotations(beam.Part)
		if err != nil {
			return nil, err
		}
		offsets := HoleOffsets(beam.Holes)
		for _, ri := range rots {
			r := geom.Rotations[ri]
			if math.Abs(math.Abs(r.Apply(local).Dot(d))-1) > 1e-6 {
				continue
			}
			for _, off := range offsets {
				origin := point.Sub(r.Apply(off))
				if !origin.OnLattice(HalfStud) {
					continue
				}
				out = append(out, Placed{Part: beam.Part, Rot: ri, Origin: origin.Round(3)})
			}
		}
	}
	return out, nil
}

// candidate is one placement with everything the search needs about it
// precomputed, so the restarts do no geometry.
type candidate struct {
	placed Placed
	cells  voxel.Cells // absolute, already offset
	holes  []geom.Vec3
	covers []int
	volume float64
}

// entersReserved reports whether a candidate would put material where a turning
// part already is.
//
// No contact fraction here, unlike the ordinary overlap rule. A beam that
// shares a cell with a driving ring is not resting against it: the ring turns,
// and the beam is inside it.
func (s *Searcher) entersReserved(c *candidate) bool {
	return s.reserves(c.cells)
}

// Solution is one structure that bears every shaft.
type Solution struct {
	Parts     []Placed
	Count     int
	BBoxStud3 float64
}

// Options tunes the search.
type Options struct {
	MaxParts int
	Restarts int
	Seed     int64
	Workers  int
	// Progress is told after each restart finishes. Optional.
	//
	// A restart is the right unit: long enough that reporting one costs
	// nothing, short enough that the count keeps moving.
	Progress progress.Func
}

// Synthesize covers the bearing requirements with as few beams as possible.
//
// This is not a search through a tree of partial solutions — that explodes,
// because with ten requirements and nearly two hundred candidates each there
// are more combinations than is sensible. It is a covering problem: every
// candidate beam covers a subset of the requirements, and the smallest cover
// whose parts do not intersect is what we want.
//
// Choosing greedily on "most still-uncovered requirements per stud^3" reaches a
// good solution quickly, and a few randomized restarts usually get below that
// again. The restarts are independent once the candidate pool is built, so they
// run in parallel.
func (s *Searcher) Synthesize(ctx context.Context, l *layout.Layout,
	stations []layout.Station, opts Options) ([]Solution, error) {

	if opts.MaxParts <= 0 {
		opts.MaxParts = 10
	}
	if opts.Restarts <= 0 {
		opts.Restarts = 60
	}
	if opts.Workers <= 0 {
		opts.Workers = workers()
	}

	reqs := BearingRequirementsWith(l, stations, 2, 8, s.Taken)
	if len(reqs) == 0 {
		return nil, nil
	}
	pool, err := s.buildPool(reqs)
	if err != nil {
		return nil, err
	}
	if len(pool) == 0 {
		return nil, nil
	}

	results := s.runRestarts(ctx, pool, len(reqs), opts)
	// What it found before it was stopped is worth having: a browser that
	// cancels a search on a changed input still has something to draw, and a
	// cover found on restart three is as valid as one found on restart sixty.
	return dedupeSolutions(results), ctx.Err()
}

// workers is how many restarts run side by side.
//
// One where the runtime cannot give us more. Under WebAssembly goroutines are
// cooperative on a single thread and NumCPU reports 1, so parallelism comes
// from the host running a whole search per worker — which the restarts suit
// exactly, since they share nothing. Set Restarts to 1 and vary Seed to own one
// restart per worker.
func workers() int {
	if n := runtime.NumCPU(); n > 1 {
		return n
	}
	return 1
}

// buildPool collects every candidate for every requirement, then works out
// which other requirements each one happens to cover: a beam can bear more
// shafts than the one it was generated for.
func (s *Searcher) buildPool(reqs []Requirement) ([]*candidate, error) {
	byPlacement := map[Placed]*candidate{}
	var order []Placed

	for _, r := range reqs {
		placements, err := s.CandidatesFor(r.Point, r.Direction)
		if err != nil {
			return nil, err
		}
		for _, p := range placements {
			if _, seen := byPlacement[p]; seen {
				continue
			}
			c, err := s.describe(p)
			if err != nil {
				return nil, err
			}
			byPlacement[p] = c
			order = append(order, p)
		}
	}

	pool := make([]*candidate, 0, len(order))
	for _, p := range order {
		c := byPlacement[p]
		for i, r := range reqs {
			if c.satisfies(r) {
				c.covers = append(c.covers, i)
			}
		}
		if len(c.covers) > 0 && !s.entersReserved(c) {
			pool = append(pool, c)
		}
	}
	return pool, nil
}

// describe precomputes a candidate's cells, holes and volume.
func (s *Searcher) describe(p Placed) (*candidate, error) {
	rel, err := s.Rast.Voxels(p.Part, p.Rot)
	if err != nil {
		return nil, err
	}
	shift := geom.Cell{
		X: int32(math.Round(p.Origin.X / voxel.Pitch)),
		Y: int32(math.Round(p.Origin.Y / voxel.Pitch)),
		Z: int32(math.Round(p.Origin.Z / voxel.Pitch)),
	}
	cells := make(voxel.Cells, len(rel))
	for i, c := range rel {
		cells[i] = c.Add(shift)
	}

	holes, _, err := WorldHoles(s.Axes, p, s.counts[p.Part])
	if err != nil {
		return nil, err
	}
	rounded := make([]geom.Vec3, len(holes))
	for i, h := range holes {
		rounded[i] = h.Round(3)
	}

	g, err := s.Rast.Lib.Geometry(p.Part)
	if err != nil {
		return nil, err
	}
	size := g.Size()
	return &candidate{
		placed: p, cells: cells, holes: rounded,
		volume: size.X * size.Y * size.Z / 8000.0,
	}, nil
}

// satisfies reports whether this placement actually bears the requirement: a
// hole on the point, with the hole axis along the shaft.
func (c *candidate) satisfies(r Requirement) bool {
	for _, h := range c.holes {
		if h.Sub(r.Point.Round(3)).Len() < 1e-6 {
			return true
		}
	}
	return false
}

func dedupeSolutions(results []Solution) []Solution {
	seen := map[string]bool{}
	var out []Solution
	for _, r := range results {
		key := ""
		for _, p := range r.Parts {
			key += fmt.Sprintf("%s/%d/%v;", p.Part, p.Rot, p.Origin)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count < out[j].Count
		}
		return out[i].BBoxStud3 < out[j].BBoxStud3
	})
	return out
}

// runRestarts runs the greedy cover many times over, in parallel.
func (s *Searcher) runRestarts(ctx context.Context, pool []*candidate, nReqs int,
	opts Options) []Solution {

	var (
		mu   sync.Mutex
		out  []Solution
		done int
		next = make(chan int)
		wg   sync.WaitGroup
	)
	go func() {
		defer close(next)
		for i := 0; i < opts.Restarts; i++ {
			select {
			case next <- i:
			case <-ctx.Done():
				return
			}
		}
	}()

	for w := 0; w < opts.Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for attempt := range next {
				// The feeder stops handing out work when the context is done,
				// but a worker can already be holding some. Checking here is
				// what makes "cancelled" mean stopped rather than nearly.
				if ctx.Err() != nil {
					return
				}
				report := func() {
					mu.Lock()
					done++
					at := done
					mu.Unlock()
					opts.Progress.Step(progress.StageStructure, at, opts.Restarts)
				}
				// Each restart has its own generator, seeded from the run seed
				// and the attempt, so the whole search is reproducible however
				// the work happens to be shared out.
				rng := rand.New(rand.NewSource(opts.Seed + int64(attempt)))
				sol, ok := s.greedyCover(pool, nReqs, opts.MaxParts, attempt > 0, rng)
				if !ok {
					report()
					continue
				}
				// The cover bears every shaft but the pieces may hang loose.
				repaired, err := s.RepairConnectivity(sol.Parts)
				if err != nil {
					report()
					continue
				}
				sol.Parts = repaired
				sol.Count = len(repaired)
				sol.BBoxStud3 = s.boundingVolume(repaired)
				mu.Lock()
				out = append(out, sol)
				mu.Unlock()
				report()
			}
		}()
	}
	wg.Wait()
	return out
}

// coverState is one restart's working set.
type coverState struct {
	uncovered map[int]bool
	chosen    []*candidate
	occupied  map[geom.Cell]bool
	holes     map[geom.Vec3]bool
}

func newCoverState(nReqs int) *coverState {
	un := make(map[int]bool, nReqs)
	for i := 0; i < nReqs; i++ {
		un[i] = true
	}
	return &coverState{uncovered: un, occupied: map[geom.Cell]bool{},
		holes: map[geom.Vec3]bool{}}
}

func (st *coverState) take(c *candidate) {
	st.chosen = append(st.chosen, c)
	for _, cell := range c.cells {
		st.occupied[cell] = true
	}
	for _, h := range c.holes {
		st.holes[h] = true
	}
	for _, i := range c.covers {
		delete(st.uncovered, i)
	}
}

// compatible separates touching from intersecting.
func (st *coverState) compatible(c *candidate) bool {
	shared := 0
	for _, cell := range c.cells {
		if st.occupied[cell] {
			shared++
		}
	}
	if shared == 0 {
		return true
	}
	smaller := len(c.cells)
	if len(st.occupied) < smaller {
		smaller = len(st.occupied)
	}
	return float64(shared) <= ContactFraction*float64(smaller)
}

// score ranks a candidate: requirements gained per unit of bulk, with a nudge
// toward parts that share a hole with what is already placed.
func (st *coverState) score(c *candidate, gain int) float64 {
	shared := 1
	if len(st.chosen) > 0 {
		shared = 0
		for _, h := range c.holes {
			if st.holes[h] {
				shared++
			}
		}
	}
	if shared > 2 {
		shared = 2
	}
	v := math.Sqrt(c.volume)
	if v <= 0 {
		v = 1
	}
	return (float64(gain) + 0.5*float64(shared)) / v
}

func (s *Searcher) greedyCover(pool []*candidate, nReqs, maxParts int,
	jitter bool, rng *rand.Rand) (Solution, bool) {

	st := newCoverState(nReqs)
	for len(st.uncovered) > 0 && len(st.chosen) < maxParts {
		best, bestScore := -1, math.Inf(-1)
		for i, c := range pool {
			gain := 0
			for _, r := range c.covers {
				if st.uncovered[r] {
					gain++
				}
			}
			if gain == 0 || !st.compatible(c) {
				continue
			}
			score := st.score(c, gain)
			if jitter {
				score *= 0.7 + 0.6*rng.Float64()
			}
			if score > bestScore {
				best, bestScore = i, score
			}
		}
		if best < 0 {
			break
		}
		st.take(pool[best])
	}
	if len(st.uncovered) > 0 {
		return Solution{}, false
	}

	parts := make([]Placed, 0, len(st.chosen))
	for _, c := range st.chosen {
		parts = append(parts, c.placed)
	}
	sort.Slice(parts, func(i, j int) bool {
		if parts[i].Part != parts[j].Part {
			return parts[i].Part < parts[j].Part
		}
		return less(parts[i].Origin, parts[j].Origin)
	})
	return Solution{Parts: parts, Count: len(parts),
		BBoxStud3: s.boundingVolume(parts)}, true
}

func less(a, b geom.Vec3) bool {
	if a.X != b.X {
		return a.X < b.X
	}
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.Z < b.Z
}

// boundingVolume is the box the whole structure occupies, in cubic studs.
func (s *Searcher) boundingVolume(parts []Placed) float64 {
	if len(parts) == 0 {
		return 0
	}
	first := true
	var lo, hi geom.Vec3
	for _, p := range parts {
		g, err := s.Rast.Lib.Geometry(p.Part)
		if err != nil {
			continue
		}
		r := geom.Rotations[p.Rot]
		for _, v := range g.Verts {
			w := r.Apply(v).Add(p.Origin)
			if first {
				lo, hi, first = w, w, false
				continue
			}
			lo = geom.Vec3{X: math.Min(lo.X, w.X), Y: math.Min(lo.Y, w.Y), Z: math.Min(lo.Z, w.Z)}
			hi = geom.Vec3{X: math.Max(hi.X, w.X), Y: math.Max(hi.Y, w.Y), Z: math.Max(hi.Z, w.Z)}
		}
	}
	s2 := hi.Sub(lo)
	return s2.X * s2.Y * s2.Z / 8000.0
}
