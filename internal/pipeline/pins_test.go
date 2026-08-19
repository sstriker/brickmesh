// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"math"
	"path/filepath"
	"strings"
	"testing"

	"brickmesh/internal/geom"
	"brickmesh/internal/part"
	"brickmesh/internal/rigidity"
)

// A joint the frame relies on has to have something in it.
//
// Every check counted joints and none of them placed one, so a model came out
// as parts lying against each other with nothing through them — correct in
// every report and visibly broken the moment it was opened. This is the
// property that was missing: what the rigidity count leans on is also in the
// file.
func TestEveryJointHasSomethingInIt(t *testing.T) {
	deps := requireLibraries(t)
	for _, path := range examples(t) {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".json"), func(t *testing.T) {
			res := runSpec(t, deps, path)
			if res.Structure == nil {
				t.Skip("no frame")
			}
			frame := make([]part.Placed, 0, len(res.Structure.Parts))
			for _, p := range res.Structure.Parts {
				frame = append(frame, part.Placed(p))
			}
			joints, err := rigidity.FindJoints(deps.Shadow, frame, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, j := range joints {
				if onAnAxleAt(res, j.Point, j.Axis) {
					continue // the shaft is the fastener there
				}
				if !filled(t, deps, res, j) {
					t.Errorf("the joint at %v along %v holds two frame parts "+
						"together and has neither a pin nor a shaft in it",
						j.Point, j.Axis)
				}
			}
		})
	}
}

// filled reports whether a pin lies along the joint's line and covers it.
//
// Along, not merely at: a pin turned across the holes it passes through sits at
// exactly the right point and fastens nothing, and that is what happened when
// the pin's own direction was read off its ports instead of its shape.
func filled(t *testing.T, deps Deps, res *Result, j rigidity.Joint) bool {
	t.Helper()
	along := j.Axis.Unit()
	lo, hi := j.Point.Dot(along), j.Mate.Dot(along)
	if lo > hi {
		lo, hi = hi, lo
	}
	for _, p := range res.Model.Parts {
		if !isPin(p.Name) {
			continue
		}
		// On the same line: the offset across the axis is nothing.
		d := p.Pos.Sub(j.Point)
		if d.Sub(along.Scale(d.Dot(along))).Len() > 1e-6 {
			continue
		}
		// And pointing along it. Measured from the shape, which is the one
		// description of a pin that cannot be ambiguous: it is long in one
		// direction and short in the other two.
		if !longestAlong(t, deps, p.Name, p.Rot, along) {
			continue
		}
		// And long enough to reach both holes from where it sits. Half a pin
		// either side of its centre, which is how they are drawn.
		at := p.Pos.Dot(along)
		half := pinHalfLength(p.Name)
		if at-half <= lo+1e-6 && at+half >= hi-1e-6 {
			return true
		}
	}
	return false
}

func pinHalfLength(name string) float64 {
	if name == LongPinPart {
		return 30
	}
	return 20
}

// The control: a pin has to actually be a pin, lying along its joint rather
// than merely near it. A part turned the wrong way would still be at the right
// place and would hold nothing.
func TestPinsLieAlongTheirJoints(t *testing.T) {
	deps := requireLibraries(t)
	res := runSpec(t, deps, filepath.Join("..", "..", "examples", "subtractor.json"))
	axis, err := localPinAxis(deps, PinPart)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, p := range res.Model.Parts {
		if !isPin(p.Name) {
			continue
		}
		found++
		// Its own axis, turned by its placement, must be a lattice direction.
		w := p.Rot.Apply(axis)
		if math.Abs(w.Len()-1) > 1e-6 {
			t.Errorf("a pin at %v is scaled, not rotated", p.Pos)
		}
		best := 0.0
		for _, d := range []geom.Vec3{{X: 1}, {Y: 1}, {Z: 1}} {
			best = math.Max(best, math.Abs(w.Dot(d)))
		}
		if math.Abs(best-1) > 1e-6 {
			t.Errorf("a pin at %v points %v, which is no hole direction", p.Pos, w)
		}
	}
	if found == 0 {
		t.Fatal("the subtractor's frame is two connectors and a beam pinned " +
			"together; no pins were placed, so the test above proves nothing")
	}
}

// longestAlong reports whether a placed part's longest dimension lies along d.
func longestAlong(t *testing.T, deps Deps, name string, rot geom.Mat3, d geom.Vec3) bool {
	t.Helper()
	g, err := deps.Lib.Geometry(name)
	if err != nil {
		t.Fatal(err)
	}
	lo, hi := g.BBox()
	var span, worst float64
	for _, a := range []geom.Vec3{{X: 1}, {Y: 1}, {Z: 1}} {
		// The extent along a, turned into the model's frame.
		e := hi.Sub(lo)
		reach := math.Abs(e.Dot(a))
		w := rot.Apply(a)
		if math.Abs(math.Abs(w.Dot(d))-1) < 1e-6 {
			span = reach
		}
		worst = math.Max(worst, reach)
	}
	return span > 0 && math.Abs(span-worst) < 1e-6
}
