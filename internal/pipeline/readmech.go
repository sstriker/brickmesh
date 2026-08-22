// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"math"
	"sort"

	"github.com/sstriker/brickmesh/internal/clutch"
	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/mech"
	"github.com/sstriker/brickmesh/internal/part"
)

// Mechanism turns a reading into the graph the functional layer works on.
//
// The hard part is that an axis line is not a shaft. A gearbox's output line
// carries the output axle and, running loose on it, the gears a driving ring
// selects between — three things that turn at three speeds through the same
// piece of plastic. Treating a line as one shaft locks them together and
// reports a gearbox that cannot shift.
//
// What separates them is readable: a gear keyed to an axle has a CROSS hole and
// a gear that runs free on one has a round hole, which is what part.Hole.Cross
// says. So each line gives one shaft for whatever is keyed to it, plus a shaft
// of its own for every gear that is not.
func (r *Reading) Mechanism(src part.Holes) (*mech.Mechanism, []mech.Finding) {
	m := mech.New("read from a model")
	var findings []mech.Finding

	// A stable order, so the same model always gives the same names.
	keys := make([][6]float64, 0, len(r.Lines))
	for k := range r.Lines {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return lessKey(keys[i], keys[j]) })

	shaftOf := map[int]string{} // part index -> shaft it turns with
	lineShaft := map[[6]float64]string{}
	free := map[int]bool{}

	idle := 0
	for i, k := range keys {
		// A line carrying neither a gear nor a ring transmits nothing — a
		// control axle, or an axle holding something still. Left in, it is a
		// shaft nothing connects to and therefore a degree of freedom that
		// never resolves, and the whole reading reports "undetermined" for a
		// gearbox that is perfectly determined.
		if !carriesDrive(r, k) {
			idle++
			continue
		}
		line := fmt.Sprintf("line%d", i+1)
		lineShaft[k] = line
		m.Shaft(line, 2)
		for _, idx := range r.Lines[k] {
			f := r.Parts[idx]
			if f.Teeth > 0 && !keyedToItsAxle(src, f) {
				// Its own shaft: it turns at its own speed until something
				// locks it to the line.
				own := fmt.Sprintf("%s-%dt@%.0f", line, f.Teeth, f.Pos.Dot(f.Axis))
				m.Shaft(own, 2)
				shaftOf[idx] = own
				free[idx] = true
				continue
			}
			shaftOf[idx] = line
		}
	}

	for _, mesh := range r.Meshes {
		a, b := shaftOf[mesh.A], shaftOf[mesh.B]
		if a == "" || b == "" || a == b {
			continue
		}
		m.MeshOf(a, b, r.Parts[mesh.A].Teeth, r.Parts[mesh.B].Teeth, mesh.Kind, 0)
	}

	// A driving ring locks a loose gear to the line it rides. Which gear is a
	// question about where the ring is standing, and the answer is only about
	// the state the model was built in: a static model shows the gear a ring
	// is in, never the set of gears it could be in.
	rings, locked := 0, 0
	for i, f := range r.Parts {
		if !isRing(f.Name) {
			continue
		}
		rings++
		k := lineKey(f.Pos, f.Axis)
		gear, ok := engagedGear(r, i, k)
		if !ok {
			continue
		}
		m.Couple(lineShaft[k], shaftOf[gear], "ring as built")
		locked++
	}
	if rings > 0 {
		findings = append(findings, mech.Finding{
			Level: "WARN", Check: "read", Detail: fmt.Sprintf(
				"%d driving ring(s), %d of them engaging a gear where they "+
					"stand. What follows is this model in the state it was "+
					"built in: a ring shows the gear it is in, never the set of "+
					"gears it could be shifted to", rings, locked)})
	}
	findings = append(findings, mech.Finding{
		Level: "OK", Check: "read", Detail: fmt.Sprintf(
			"%d shaft(s) from %d axis line(s): %d gear(s) run loose on the line "+
				"they sit on and turn at their own speed, and %d line(s) carry "+
				"neither gear nor ring and so drive nothing", len(m.Order()),
			len(keys), countTrue(free), idle)})
	return m, findings
}

// carriesDrive reports whether anything on this line can transmit.
func carriesDrive(r *Reading, k [6]float64) bool {
	for _, idx := range r.Lines[k] {
		if r.Parts[idx].Teeth > 0 || isRing(r.Parts[idx].Name) {
			return true
		}
	}
	return false
}

// keyedToItsAxle reports whether a gear turns with whatever runs through it.
//
// A cross hole is keyed and a round one is not, which is the whole difference
// between a gear that drives a shaft and one that freewheels on it. A part the
// shadow library says nothing about is taken as keyed: that is the ordinary
// case, and a gearbox's loose gears are all parts it does describe.
func keyedToItsAxle(src part.Holes, f Found) bool {
	if src == nil {
		return true
	}
	for _, h := range src.Holes(f.Name) {
		// The hole down its middle, not one of the ones around the rim.
		if h.Pos.Len() > 1e-6 {
			continue
		}
		if math.Abs(math.Abs(h.Axis.Unit().Dot(geom.Vec3{Z: 1}))-1) > 1e-6 {
			continue
		}
		return h.Cross
	}
	return true
}

// engagedGear is the loose gear a ring is standing in, if any.
func engagedGear(r *Reading, ring int, line [6]float64) (int, bool) {
	f := r.Parts[ring]
	var best int
	bestGap := math.Inf(1)
	for _, idx := range r.Lines[line] {
		g := r.Parts[idx]
		if g.Teeth == 0 {
			continue
		}
		gap := math.Abs(g.Pos.Sub(f.Pos).Dot(f.Axis))
		for _, s := range clutch.Systems {
			if s.Ring != f.Name {
				continue
			}
			want := s.Engaged * 10
			if math.Abs(gap-want) < readTolerance && gap < bestGap {
				best, bestGap = idx, gap
			}
		}
	}
	return best, !math.IsInf(bestGap, 1)
}

func countTrue(m map[int]bool) int {
	n := 0
	for _, v := range m {
		if v {
			n++
		}
	}
	return n
}

// sortedLineKeys is the reading's lines in the order Mechanism names them.
func sortedLineKeys(r *Reading) [][6]float64 {
	keys := make([][6]float64, 0, len(r.Lines))
	for k := range r.Lines {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return lessKey(keys[i], keys[j]) })
	return keys
}

func indexOf(keys [][6]float64, want [6]float64) int {
	for i, k := range keys {
		if k == want {
			return i
		}
	}
	return -1
}
