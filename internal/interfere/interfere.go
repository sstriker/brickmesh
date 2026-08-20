// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package interfere decides whether two gears actually mesh.
//
// The decisive test is not a single collision check but a sweep: turn one gear
// a full revolution while the other stands still. If the teeth really engage,
// it is blocked for most of the turn and exactly as many narrow free windows
// remain as it has teeth, one per tooth, spaced a tooth pitch apart. That
// window width IS the backlash.
//
// If it turns freely the gears either do not touch at all or merely graze along
// the tips, however close together they stand.
//
// The reference measurement, from docs/findings.md: two 24-tooth gears mesh at
// exactly 60 LDU and jam at 58. Any change here has to reproduce that before
// its other answers mean anything.
package interfere

import (
	"context"
	"math"
	"runtime"
	"sync"

	"brickmesh/internal/collide"
	"brickmesh/internal/geom"
	"brickmesh/internal/part"
)

// Verdicts.
const (
	Meshes       = "MESHES"
	TooDeep      = "TOO DEEP"
	NoEngagement = "NO ENGAGEMENT"
	Doubtful     = "DOUBTFUL"
)

// Result is what a sweep found.
type Result struct {
	Verdict            string
	Windows            int
	ExpectedWindows    int
	WindowSpacingDeg   float64
	ExpectedSpacingDeg float64
	BacklashDeg        float64
	FreeFraction       float64
}

// MeshFor builds a collision mesh from a library part.
func MeshFor(lib part.Shapes, name string) (*collide.Mesh, error) {
	g, err := lib.Geometry(name)
	if err != nil {
		return nil, err
	}
	tris := make([]collide.Triangle, len(g.Tris))
	for i, t := range g.Tris {
		tris[i] = collide.Triangle(t)
	}
	return collide.NewMesh(tris), nil
}

// Rot is a rotation about a principal axis, in degrees. Unlike the 24 lattice
// rotations this is continuous: a sweep needs angles between them.
func Rot(axis byte, deg float64) geom.Mat3 {
	t := deg * math.Pi / 180
	c, s := math.Cos(t), math.Sin(t)
	switch axis {
	case 'x':
		return geom.Mat3{{1, 0, 0}, {0, c, -s}, {0, s, c}}
	case 'y':
		return geom.Mat3{{c, 0, s}, {0, 1, 0}, {-s, 0, c}}
	default:
		return geom.Mat3{{c, -s, 0}, {s, c, 0}, {0, 0, 1}}
	}
}

// Options for a sweep.
type Options struct {
	// Steps is how finely the revolution is sampled.
	//
	// This matters more than it looks. The free window either side of a meshed
	// tooth is a couple of degrees wide, so sampling every five degrees steps
	// straight over it and the sweep reports a confident "cannot be assembled"
	// for a pair that meshes perfectly. 144 is fine; coarser is not.
	Steps    int
	SpinAxis byte
	Workers  int
	// Fit is how deep an interference still counts as contact rather than
	// collision, in LDU. Zero keeps the old all-or-nothing answer.
	//
	// It exists because in LDraw everything is nominal, so every real fit is an
	// exact one and reads as a collision. A driving ring's dogs REST IN a
	// clutch gear's recesses; a half-width liftarm fills a 10 LDU groove to the
	// LDU; a fork's tine sits in a channel cut to take it. Coplanar faces are
	// already treated as contact by the triangle test, which covers the clean
	// case, but a fit that is a fraction of an LDU off a shared plane is not
	// coplanar and was still reading as buried.
	//
	// The measure is depth along the line between the two parts, which for two
	// coaxial parts is their axis and for a catch beside a ring is the way out
	// from the shaft — in both cases the direction they would be pulled apart
	// along. A block that clears within Fit of separation is a fit.
	Fit float64
}

// FitTolerance is how deep an interference reads as contact rather than
// collision, in LDU, for the question "is this part buried in that one".
//
// A quarter of an LDU. An 18947 at its engaged distance of 30 clears at 0.25,
// and the same ring pushed half a stud too far in at 29.5 does not — so it
// admits a fit without admitting a ring in the wrong place.
//
// Deliberately not used for "does this engage". Those are different questions
// and want different tolerances: burial is depth BEYOND the touch, engagement
// IS the touch. A first-generation ring at its engaged distance blocks 29% of a
// revolution in sixteen windows at no tolerance at all, and a tenth of an LDU
// of slack frees it completely — so measuring engagement with a tolerance
// measures it away. See clutch.System.EngageFit.
//
// The clearance check's own touchTolerance is 1.0 and applies to bounding
// boxes, which is a coarser question about parts that merely graze.
const FitTolerance = 0.25

// MeshLock turns B a full revolution against a stationary A and reports what it
// found.
//
// Takes a context because a sweep is the innermost expensive thing the engine
// does — a few hundred pairs of meshes through a BVH, per pair of parts — and
// it is on the critical path of a run rather than off in a measurement. A
// cancelled sweep returns an error rather than a Result: a partial revolution
// says nothing, and a verdict from one would be a lie.
func MeshLock(ctx context.Context, a *collide.Mesh, ta collide.Transform,
	b *collide.Mesh, tb collide.Transform, teethB int, opts Options) (Result, error) {

	if opts.Steps <= 0 {
		opts.Steps = 144
	}
	if opts.SpinAxis == 0 {
		opts.SpinAxis = 'z'
	}
	if opts.Workers <= 0 {
		opts.Workers = workers()
	}

	free, err := sweep(ctx, a, ta, b, tb, opts)
	if err != nil {
		return Result{}, err
	}
	return classify(free, opts.Steps, teethB), nil
}

// workers is how many goroutines a sweep splits over.
//
// One where the runtime cannot give us more. Under WebAssembly goroutines are
// cooperative on a single thread and NumCPU reports 1, so the parallelism has
// to come from the host running whole searches side by side rather than from
// here. Asking for it anyway would only add scheduling.
func workers() int {
	if n := runtime.NumCPU(); n > 1 {
		return n
	}
	return 1
}

// sweep reports, for each sampled angle, whether B is free of A there.
func sweep(ctx context.Context, a *collide.Mesh, ta collide.Transform,
	b *collide.Mesh, tb collide.Transform, opts Options) ([]bool, error) {

	free := make([]bool, opts.Steps)
	// The way the two would come apart: from A towards B. Two coaxial parts
	// give their shared axis; a catch beside a ring gives the way out from the
	// shaft. Concentric parts give nothing, and then Fit cannot apply.
	apart := tb.Pos.Sub(ta.Pos)
	if l := apart.Len(); l > 1e-9 {
		apart = apart.Scale(1 / l)
	} else {
		apart = geom.Vec3{}
	}
	var wg sync.WaitGroup
	step := make(chan int)
	go func() {
		defer close(step)
		for k := 0; k < opts.Steps; k++ {
			select {
			case step <- k:
			case <-ctx.Done():
				return
			}
		}
	}()

	for w := 0; w < opts.Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := range step {
				ang := float64(k) * 360 / float64(opts.Steps)
				turned := collide.Transform{
					Rot: tb.Rot.Mul(Rot(opts.SpinAxis, ang)),
					Pos: tb.Pos,
				}
				if !collide.Intersects(a, ta, b, turned) {
					free[k] = true
					continue
				}
				// Blocked. Shallow enough to be contact? Only asked of the
				// angles that block, so a sweep that clears costs nothing.
				free[k] = clearsWithin(a, ta, b, turned, apart, opts.Fit)
			}
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return free, nil
}

// clearsWithin reports whether pulling B off A by up to fit separates them.
//
// A few steps rather than a binary search: fit is under an LDU in practice, the
// answer only has to be good to a fraction of one, and this runs on the angles
// that block, which in a real fit is most of them.
func clearsWithin(a *collide.Mesh, ta collide.Transform, b *collide.Mesh,
	tb collide.Transform, apart geom.Vec3, fit float64) bool {

	if fit <= 0 || apart == (geom.Vec3{}) {
		return false
	}
	const steps = 4
	for i := 1; i <= steps; i++ {
		off := fit * float64(i) / steps
		moved := collide.Transform{Rot: tb.Rot, Pos: tb.Pos.Add(apart.Scale(off))}
		if !collide.Intersects(a, ta, b, moved) {
			return true
		}
	}
	return false
}

// classify turns the free angles into a verdict.
func classify(free []bool, steps, teethB int) Result {
	expectedSpacing := 360.0 / float64(teethB)
	res := Result{
		ExpectedWindows:    teethB,
		ExpectedSpacingDeg: expectedSpacing,
	}

	count := 0
	for _, f := range free {
		if f {
			count++
		}
	}
	res.FreeFraction = float64(count) / float64(steps)

	switch count {
	case 0:
		res.Verdict = TooDeep
		return res
	case steps:
		res.Verdict = NoEngagement
		res.BacklashDeg = 360
		return res
	}

	windows := windowsOf(free, steps)
	res.Windows = len(windows)

	stepDeg := 360.0 / float64(steps)
	res.BacklashDeg = medianLength(windows) * stepDeg
	res.WindowSpacingDeg = medianSpacing(windows, stepDeg, len(free))

	if len(windows) == teethB &&
		math.Abs(res.WindowSpacingDeg-expectedSpacing) < expectedSpacing*0.15 {
		res.Verdict = Meshes
	} else {
		res.Verdict = Doubtful
	}
	return res
}

// windowsOf groups consecutive free angles, joining across the wrap point: a
// window straddling zero is one window, not two.
func windowsOf(free []bool, steps int) [][]int {
	var windows [][]int
	var cur []int
	for k := 0; k < steps; k++ {
		if free[k] {
			cur = append(cur, k)
			continue
		}
		if len(cur) > 0 {
			windows = append(windows, cur)
			cur = nil
		}
	}
	if len(cur) > 0 {
		windows = append(windows, cur)
	}
	if len(windows) > 1 {
		first, last := windows[0], windows[len(windows)-1]
		if first[0] == 0 && last[len(last)-1] == steps-1 {
			windows[0] = append(last, first...)
			windows = windows[:len(windows)-1]
		}
	}
	return windows
}

func medianLength(windows [][]int) float64 {
	lengths := make([]float64, len(windows))
	for i, w := range windows {
		lengths[i] = float64(len(w))
	}
	return median(lengths)
}

func medianSpacing(windows [][]int, stepDeg float64, steps int) float64 {
	if len(windows) < 2 {
		return 360
	}
	starts := make([]float64, len(windows))
	for i, w := range windows {
		starts[i] = float64(w[0]) * stepDeg
	}
	gaps := make([]float64, 0, len(starts)-1)
	for i := 1; i < len(starts); i++ {
		gaps = append(gaps, starts[i]-starts[i-1])
	}
	return median(gaps)
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}

// RotAbout is a rotation by an angle in degrees about an arbitrary unit axis.
//
// Rodrigues' formula. Rot only handles the principal axes, and a gear's shaft
// can point along any of the lattice directions.
func RotAbout(axis geom.Vec3, deg float64) geom.Mat3 {
	a := axis.Unit()
	t := deg * math.Pi / 180
	c, s := math.Cos(t), math.Sin(t)
	k := 1 - c
	return geom.Mat3{
		{c + a.X*a.X*k, a.X*a.Y*k - a.Z*s, a.X*a.Z*k + a.Y*s},
		{a.Y*a.X*k + a.Z*s, c + a.Y*a.Y*k, a.Y*a.Z*k - a.X*s},
		{a.Z*a.X*k - a.Y*s, a.Z*a.Y*k + a.X*s, c + a.Z*a.Z*k},
	}
}
