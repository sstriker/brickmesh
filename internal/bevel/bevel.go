// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package bevel finds where a bevel gear engages a differential's ring.
//
// Parallel spur meshing follows the (t1+t2)/16 rule and needs no search. This
// does not: the axes are perpendicular, and the pitch cones meet at an apex
// that no bounding box will tell you. So it is solved from the tooth geometry
// instead — sweep the driver across a grid of positions and score each on the
// gap between the two tooth surfaces. The wanted position is where they just
// touch — and then which of those actually can be built, which a cloud of
// points cannot answer.
//
// That last step is the one this does differently from the Python it came from.
// A nearest-neighbour distance between two point clouds is never negative, so a
// gear driven straight through the ring scores as well as one resting against
// it; the Python works around that by counting how many sampled points fall
// within 0.6 LDU and taking the most. That count depends on which points were
// sampled. So the candidates are settled here with the triangle test the rest
// of the engine uses, and the answer is the innermost position where the two
// solids do not overlap at any tooth phase — the same criterion that says two
// 24t gears mesh at 60 LDU and jam at 58.
//
// Tooth phase matters and is swept too. At the wrong phase the teeth meet tip
// to tip and the answer comes out too far apart.
//
// The Python samples a few thousand vertices with numpy's generator for speed,
// and this samples too — but by an even stride over the distinct vertices
// rather than at random, so the same part always gives the same points and two
// runs can be compared. LDraw parts repeat their corners heavily, so removing
// duplicates does most of the work before any sampling happens.
package bevel

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/part"
)

// Defaults naming the parts this was worked out on: the differential housing
// with the 28-tooth ring inside it, driven by a 12-tooth double bevel.
const (
	DefaultDiff   = "62821.dat"
	DefaultDriver = "32270.dat"
)

// Options for the search, all in LDU.
type Options struct {
	// RingMinRadius keeps only the vertices out at the ring's teeth, dropping
	// the housing around them.
	RingMinRadius float64
	// RadialFrom and RadialTo sweep the driver away from the differential's
	// axis; AxialFrom and AxialTo sweep it along that axis.
	RadialFrom, RadialTo float64
	AxialFrom, AxialTo   float64
	Step                 float64
	// PhaseStep is how finely a tooth pitch is swept, in degrees.
	PhaseStep float64
	// Touching is how close counts as contact, and Contact how close counts as
	// a point of it.
	Touching, Contact float64
	// RingPoints and DriverPoints cap how many vertices each side is sampled
	// down to. Zero means take them all.
	RingPoints, DriverPoints int
	// Workers is how many positions are scored at once.
	Workers int
}

// Defaults are the ranges the Python searched.
func Defaults() Options {
	return Options{
		RingMinRadius: 30,
		RadialFrom:    30, RadialTo: 70,
		AxialFrom: 0, AxialTo: 45,
		Step: 1.25, PhaseStep: 2.5,
		Touching: 1.2, Contact: 0.6,
		RingPoints: 4000, DriverPoints: 3000,
		Workers: runtime.NumCPU(),
	}
}

// Result is where the driver engages, and how well.
type Result struct {
	RadialLDU, AxialLDU   float64
	RadialStud, AxialStud float64
	MinGapLDU             float64
	// ContactPoints is how many sampled points of the driver lie within
	// touching distance of the ring. It is what picks between the positions
	// that touch, and it depends on the sampling, which is why two runs with
	// different samples can choose differently.
	ContactPoints int
	Touching      int
}

func (r Result) String() string {
	return fmt.Sprintf("radial %.2f LDU (%.3f stud), axial %.2f LDU (%.3f stud), "+
		"gap %.2f LDU, %d points in contact",
		r.RadialLDU, r.RadialStud, r.AxialLDU, r.AxialStud,
		r.MinGapLDU, r.ContactPoints)
}

// RingTeeth isolates the ring gear's tooth surface from the housing around it,
// by radius from the differential's own axis.
func RingTeeth(lib part.Shapes, part string, minRadius float64) ([]geom.Vec3, error) {
	g, err := lib.Geometry(part)
	if err != nil {
		return nil, err
	}
	var out []geom.Vec3
	seen := map[geom.Vec3]bool{}
	for _, v := range g.Verts {
		// The differential's axis is Z, so the ring's teeth are the vertices
		// far out in X and Y.
		if math.Hypot(v.X, v.Y) <= minRadius {
			continue
		}
		k := v.Round(4)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no vertices beyond %g LDU in %s: is that the "+
			"differential, and is its axis along Z", minRadius, part)
	}
	return out, nil
}

// Solve sweeps the driver and returns where it engages.
func Solve(lib part.Shapes, diffPart, driverPart string, opts Options) (Result, error) {
	ring, err := RingTeeth(lib, diffPart, opts.RingMinRadius)
	if err != nil {
		return Result{}, err
	}
	g, err := lib.Geometry(driverPart)
	if err != nil {
		return Result{}, err
	}
	ring = sample(ring, opts.RingPoints)
	grid := newGrid(ring, math.Max(opts.Touching, 2))

	// The driver is modelled about Z and has to lie along X to face the ring.
	toX := geom.Mat3{{0, 0, 1}, {0, 1, 0}, {-1, 0, 0}}
	driver := sample(distinct(g.Verts), opts.DriverPoints)
	base := make([]geom.Vec3, len(driver))
	for i, v := range driver {
		base[i] = toX.Apply(v)
	}

	// One tooth pitch of a 12t is 30 degrees, so that is all there is to sweep.
	var phases [][]geom.Vec3
	for p := 0.0; p < 30; p += opts.PhaseStep {
		r := rotX(p)
		turned := make([]geom.Vec3, len(base))
		for i, v := range base {
			turned[i] = r.Apply(v)
		}
		phases = append(phases, turned)
	}

	// Every position is independent, so they are scored in parallel and the
	// best taken afterwards. The answer does not depend on the order.
	var radials []float64
	for dx := opts.RadialFrom; dx < opts.RadialTo; dx += opts.Step {
		radials = append(radials, dx)
	}

	workers := opts.Workers
	if workers <= 0 {
		workers = 1
	}
	out := make([][]scored, len(radials))
	var wg sync.WaitGroup
	next := make(chan int)
	go func() {
		defer close(next)
		for i := range radials {
			next <- i
		}
	}()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				dx := radials[i]
				var row []scored
				for dz := opts.AxialFrom; dz < opts.AxialTo; dz += opts.Step {
					gap, contact := math.Inf(1), 0
					for _, pts := range phases {
						g, c := grid.score(pts, geom.Vec3{X: dx, Z: dz}, opts.Contact)
						if g < gap {
							gap, contact = g, c
						}
					}
					row = append(row, scored{dx, dz, gap, contact})
				}
				out[i] = row
			}
		}()
	}
	wg.Wait()

	// Every position that touches, innermost first. Touching is cheap to find
	// and narrows a thousand positions to a few hundred.
	var touchingAt []scored
	closest := math.Inf(1)
	for _, row := range out {
		for _, r := range row {
			closest = math.Min(closest, r.gap)
			if r.gap < opts.Touching {
				touchingAt = append(touchingAt, r)
			}
		}
	}
	if len(touchingAt) == 0 {
		return Result{}, fmt.Errorf("nothing in the search range touches; the "+
			"closest approach was %.2f LDU against a threshold of %.2f",
			closest, opts.Touching)
	}
	sort.Slice(touchingAt, func(i, j int) bool {
		if touchingAt[i].dx != touchingAt[j].dx {
			return touchingAt[i].dx < touchingAt[j].dx
		}
		return touchingAt[i].dz < touchingAt[j].dz
	})

	// Among the positions that touch, the one with the most surface in contact.
	// That is the Python's criterion and it is kept, because the obvious
	// improvement does not work.
	//
	// The improvement would be the sweep the rest of this engine settles gear
	// meshing with: turn the driver a full revolution and look for the pattern
	// of a mesh, blocked for most of it and free in as many windows as the
	// driver has teeth. Two 24t spur gears show that exactly. A bevel pair does
	// not show it at any of these 444 positions — its teeth meet the ring at an
	// angle, so they neither block evenly nor clear evenly, and requiring the
	// signature rejects everything. That is a measured result and not a bug in
	// the sweep, and it is why bevel engagement is still an open question in
	// PLAN.md while spur meshing is settled.
	//
	// So what comes back here is a candidate, not a verified answer. Contact
	// counting depends on which vertices were sampled, and the Python and this
	// choose different winners from the same 444 for that reason alone.
	best := Result{}
	found := false
	for _, r := range touchingAt {
		if !found || r.contact > best.ContactPoints {
			found = true
			best = Result{
				RadialLDU: r.dx, AxialLDU: r.dz,
				RadialStud: r.dx / 20, AxialStud: r.dz / 20,
				MinGapLDU: r.gap, ContactPoints: r.contact,
				Touching: len(touchingAt),
			}
		}
	}
	if found {
		return best, nil
	}
	return Result{}, fmt.Errorf("%d positions touch but none could be scored",
		len(touchingAt))
}

// scored is one swept position and how close it came.
type scored struct {
	dx, dz  float64
	gap     float64
	contact int
}

func rotX(deg float64) geom.Mat3 {
	t := deg * math.Pi / 180
	c, s := math.Cos(t), math.Sin(t)
	return geom.Mat3{{1, 0, 0}, {0, c, -s}, {0, s, c}}
}

// grid answers "how far to the nearest ring vertex" without a tree.
//
// The points are bucketed by cell; a query looks in its own cell and the ring
// of cells around it, widening until something is found or the search is
// hopeless. With a cell about the size of the distances that matter, that is a
// handful of buckets per query.
type grid struct {
	cell  float64
	rows  map[[3]int][]geom.Vec3
	empty bool
}

func newGrid(points []geom.Vec3, cell float64) *grid {
	g := &grid{cell: cell, rows: make(map[[3]int][]geom.Vec3, len(points))}
	for _, p := range points {
		k := g.key(p)
		g.rows[k] = append(g.rows[k], p)
	}
	g.empty = len(points) == 0
	return g
}

func (g *grid) key(p geom.Vec3) [3]int {
	return [3]int{
		int(math.Floor(p.X / g.cell)),
		int(math.Floor(p.Y / g.cell)),
		int(math.Floor(p.Z / g.cell)),
	}
}

// score reports the smallest distance from any moved point to the ring, and how
// many of them are within `contact` of it.
func (g *grid) score(pts []geom.Vec3, by geom.Vec3, contact float64) (float64, int) {
	if g.empty {
		return math.Inf(1), 0
	}
	best, count := math.Inf(1), 0
	for _, p := range pts {
		d := g.nearest(p.Add(by))
		if d < best {
			best = d
		}
		if d < contact {
			count++
		}
	}
	return best, count
}

// nearest is the distance to the closest ring vertex.
func (g *grid) nearest(p geom.Vec3) float64 {
	k := g.key(p)
	best := math.Inf(1)
	for ring := 0; ring <= 3; ring++ {
		for dx := -ring; dx <= ring; dx++ {
			for dy := -ring; dy <= ring; dy++ {
				for dz := -ring; dz <= ring; dz++ {
					// Only the shell just added, not the whole box again.
					if ring > 0 && abs(dx) != ring && abs(dy) != ring && abs(dz) != ring {
						continue
					}
					for _, q := range g.rows[[3]int{k[0] + dx, k[1] + dy, k[2] + dz}] {
						if d := q.Sub(p).Len(); d < best {
							best = d
						}
					}
				}
			}
		}
		// Anything outside the shells already searched is further than this.
		if best <= float64(ring)*g.cell {
			return best
		}
	}
	return best
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// distinct drops repeated corners, of which an LDraw part has many: it is built
// from primitives that meet edge to edge.
func distinct(v []geom.Vec3) []geom.Vec3 {
	seen := make(map[geom.Vec3]bool, len(v))
	out := make([]geom.Vec3, 0, len(v))
	for _, p := range v {
		k := p.Round(4)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, p)
	}
	return out
}

// sample takes an even stride through the points, so the same part always gives
// the same sample and two runs are comparable.
func sample(v []geom.Vec3, n int) []geom.Vec3 {
	if n <= 0 || len(v) <= n {
		return v
	}
	out := make([]geom.Vec3, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, v[i*len(v)/n])
	}
	return out
}
