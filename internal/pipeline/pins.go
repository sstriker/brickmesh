// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"math"
	"sort"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/ldr"
	"github.com/sstriker/brickmesh/internal/mech"
	"github.com/sstriker/brickmesh/internal/part"
	"github.com/sstriker/brickmesh/internal/rigidity"
)

// Pins are what the joints are made of.
//
// Every check the engine ran counted joints and none of them placed one, so a
// model came out as beams lying against each other with nothing through them.
// It reads as correct in every report and looks broken the moment it is opened:
// the frame of a two-speed gearbox was four parts and no fasteners.
//
// They are real parts. They cost something, they occupy the holes they go
// through, and leaving them out understates both the part count and what is
// left free for anything else to use.
const (
	// PinPart is the ordinary black friction pin, two studs end to end. Its
	// shadow entry is 3673 included whole, and 3673's is a centered cylinder of
	// sections 2+16+4+16+2 — 40 LDU, which is what reaches through two parts
	// lying against each other.
	PinPart = "2780.dat"
	// AxlePinPart is half pin, half axle: sections 2+16+2 then 20 of axle. It
	// is what joins a round hole to a cross hole, where a plain pin would spin
	// and an axle would seize.
	AxlePinPart = "3749.dat"
	// LongPinPart is three studs, for a run of holes deeper than two parts.
	LongPinPart = "6558.dat"
)

// placePins puts a pin through every joint the structure relies on.
//
// One per joint, positioned midway between the two holes it passes through and
// turned to lie along them. Joints that an axle already fills are left alone:
// the axle is the fastener there, and a pin in the same hole would be a second
// thing in one place.
func placePins(res *Result, deps Deps, model *ldr.Model) error {
	if res.Structure == nil || deps.Shadow == nil {
		return nil
	}
	frame := make([]part.Placed, 0, len(res.Structure.Parts))
	for _, p := range res.Structure.Parts {
		frame = append(frame, part.Placed(p))
	}
	joints, err := rigidity.FindJoints(deps.Shadow, frame, nil)
	if err != nil {
		return err
	}

	pinAxis, err := localPinAxis(deps, PinPart)
	if err != nil {
		// Nothing describes the pin, so it cannot be turned to face the joint.
		// Said rather than skipped: a frame with no fasteners is the thing this
		// exists to stop.
		res.Findings = append(res.Findings, mech.Finding{
			Level: "WARN", Check: "pins", Detail: fmt.Sprintf(
				"no pin was placed: %v", err)})
		return nil
	}

	placed, skipped := 0, 0
	for _, run := range pinRuns(joints) {
		if onAnAxleAt(res, run.at, run.axis) {
			continue // the shaft is the fastener there
		}
		name, ok := run.part()
		if !ok {
			skipped++
			continue
		}
		rot, ok := rotationTaking(pinAxis, run.axis)
		if !ok {
			skipped++
			continue
		}
		model.Add(name, colour(name), geom.Rotations[rot], run.at,
			fmt.Sprintf("pin joining the frame at %v", run.at.Round(1)))
		placed++
	}
	if skipped > 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "WARN", Check: "pins", Detail: fmt.Sprintf(
				"%d joint(s) have no pin in them, so the frame is counted as "+
					"holding in places where nothing holds it", skipped)})
	}
	if placed > 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "OK", Check: "pins", Detail: fmt.Sprintf(
				"%d pin(s) placed, one per joint the frame relies on", placed)})
	}
	return nil
}

// pinRuns turns joints into the pins that realise them.
//
// Joints on the same hole line whose spans touch are one pin: three parts
// stacked at one hole are two joints and one pin through all three. Joints on
// the same line that do not touch are separate pins, which is why this merges
// intervals rather than keeping one per line — a frame can use the same line
// twice, a long way apart, and dropping one of those would leave a joint the
// rigidity count is relying on with nothing in it.
func pinRuns(joints []rigidity.Joint) []pinRun {
	type line struct{ base, axis geom.Vec3 }
	spans := map[line][][2]float64{}
	var order []line

	for _, j := range joints {
		along := j.Axis.Unit()
		base := j.Point.Sub(along.Scale(j.Point.Dot(along)))
		k := line{base.Round(3), j.Axis.Round(3)}
		if _, seen := spans[k]; !seen {
			order = append(order, k)
		}
		lo, hi := j.Point.Dot(along), j.Mate.Dot(along)
		if lo > hi {
			lo, hi = hi, lo
		}
		spans[k] = append(spans[k], [2]float64{lo, hi})
	}

	var out []pinRun
	for _, k := range order {
		runs := spans[k]
		sort.Slice(runs, func(i, j int) bool { return runs[i][0] < runs[j][0] })
		along := k.axis.Unit()
		cur := runs[0]
		flush := func(r [2]float64) {
			mid := (r[0] + r[1]) / 2
			out = append(out, pinRun{
				at:     k.base.Add(along.Scale(mid)),
				axis:   k.axis,
				length: r[1] - r[0],
			})
		}
		for _, r := range runs[1:] {
			if r[0] <= cur[1]+1e-6 {
				cur[1] = math.Max(cur[1], r[1]) // touching: one pin covers both
				continue
			}
			flush(cur)
			cur = r
		}
		flush(cur)
	}
	sort.SliceStable(out, func(i, j int) bool { return less(out[i].at, out[j].at) })
	return out
}

// pinRun is one pin: where it sits, which way it lies, and how far apart the
// outermost holes it has to reach are.
type pinRun struct {
	at     geom.Vec3
	axis   geom.Vec3
	length float64
}

// part is the shortest pin that reaches across the run.
//
// Only two are needed. Joints exist only between holes within a pin's reach, so
// no run can be longer than that; the long pin is here because two joints that
// touch can merge into a run longer than either.
func (r pinRun) part() (string, bool) {
	switch {
	case r.length <= rigidity.PinReach+1e-6:
		return PinPart, true
	case r.length <= 60+1e-6:
		return LongPinPart, true
	}
	return "", false
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

// onAnAxleAt reports whether a shaft already runs along this line. The axle is
// the fastener there, and a pin in the same hole would be two things in one
// place.
func onAnAxleAt(res *Result, at, axis geom.Vec3) bool {
	for _, a := range res.Axles {
		if a.Covers(at, axis) {
			return true
		}
	}
	return false
}

// localPinAxis is the direction a pin points in its own frame, taken from the
// shape rather than from its ports.
//
// The ports were the obvious source and they are wrong. 2780 declares its pin
// by including 3673 whole, and 3673 says X twice — but walking 2780's own
// subfiles turns up another port facing Y, from a piece of the friction slot,
// and which of the two comes first is an accident of sorting. Picking the first
// laid one pin across the holes it was meant to go through, which looked right
// in the file because the joint it happened to be tested on ran the same way.
//
// A pin is long in exactly one direction and short in the other two, so the
// shape answers unambiguously.
func localPinAxis(deps Deps, name string) (geom.Vec3, error) {
	if deps.Lib == nil {
		return geom.Vec3{}, fmt.Errorf("no parts library to measure %s", name)
	}
	g, err := deps.Lib.Geometry(name)
	if err != nil {
		return geom.Vec3{}, err
	}
	lo, hi := g.BBox()
	size := hi.Sub(lo)
	switch {
	case size.X >= size.Y && size.X >= size.Z:
		return geom.Vec3{X: 1}, nil
	case size.Y >= size.Z:
		return geom.Vec3{Y: 1}, nil
	}
	return geom.Vec3{Z: 1}, nil
}

// rotationTaking finds a lattice rotation that turns from onto to.
func rotationTaking(from, to geom.Vec3) (int, bool) {
	f, t := from.Unit(), to.Unit()
	for i, r := range geom.Rotations {
		// Sign-free: a pin is symmetric end to end, so either way along the
		// hole is the same pin.
		if math.Abs(math.Abs(r.Apply(f).Dot(t))-1) < 1e-6 {
			return i, true
		}
	}
	return 0, false
}
