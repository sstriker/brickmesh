// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package synth

import (
	"math"
	"sort"

	"brickmesh/internal/geom"
	"brickmesh/internal/part"
	"brickmesh/internal/rigidity"
)

// maxRepairRounds bounds the repair loop. Each round joins at most two pieces,
// so a structure needing more than this is not one bridge short — it is wrong.
const maxRepairRounds = 12

// RepairConnectivity adds beams until the structure is one whole.
//
// The cover is complete by this point, so every shaft is borne; what is left is
// that the pieces hang loose. Each round takes the two largest pieces and
// bridges them.
func (s *Searcher) RepairConnectivity(chosen []Placed) ([]Placed, error) {
	for round := 0; round < maxRepairRounds; round++ {
		joints, err := rigidity.FindJoints(s.Axes, chosen, s.Inventory)
		if err != nil {
			return nil, err
		}
		comps := rigidity.Components(len(chosen), joints)
		if len(comps) <= 1 {
			return chosen, nil
		}
		sort.SliceStable(comps, func(i, j int) bool { return len(comps[i]) > len(comps[j]) })

		bridge, err := s.bridgeBetween(chosen, comps[0], comps[1])
		if err != nil {
			return nil, err
		}
		if bridge == nil {
			return chosen, nil // nothing spans it; report as loose rather than loop
		}
		chosen = append(chosen, *bridge)
	}
	return chosen, nil
}

// bridgeBetween finds the cheapest beam joining two pieces without intersecting
// what is already there.
func (s *Searcher) bridgeBetween(chosen []Placed, a, b []int) (*Placed, error) {
	holesA, err := s.holesOf(chosen, a)
	if err != nil {
		return nil, err
	}
	holesB, err := s.holesOf(chosen, b)
	if err != nil {
		return nil, err
	}
	refsA, refsB := s.holeRefs(chosen, a), s.holeRefs(chosen, b)

	occupied := map[geom.Cell]bool{}
	total := 0
	for _, p := range chosen {
		cells, err := s.absoluteCells(p)
		if err != nil {
			return nil, err
		}
		for _, c := range cells {
			if !occupied[c] {
				occupied[c] = true
				total++
			}
		}
	}

	options, err := s.ConnectorsBetween(holesA, holesB)
	if err != nil {
		return nil, err
	}
	// Shortest beam first: a bridge is dead weight, so use as little as spans it.
	sort.SliceStable(options, func(i, j int) bool {
		return s.counts[options[i].Part] < s.counts[options[j].Part]
	})

	for _, cand := range options {
		cells, err := s.absoluteCells(cand)
		if err != nil {
			return nil, err
		}
		// A bridge bears nothing, so it is free to run anywhere — including
		// straight through a driving ring, which is why it is asked the same
		// question a bearing is.
		if s.reserves(cells) {
			continue
		}
		shared := 0
		for _, c := range cells {
			if occupied[c] {
				shared++
			}
		}
		smaller := len(cells)
		if total < smaller {
			smaller = total
		}
		if shared != 0 && float64(shared) > ContactFraction*float64(smaller) {
			continue
		}
		// A bridge that reaches without being pinnable to both is not a bridge.
		// Checking it here is what keeps the repair from piling on parts that
		// change nothing.
		candHoles := s.holeRefs([]Placed{cand}, []int{0})
		if !joins(candHoles, refsA) || !joins(candHoles, refsB) {
			continue
		}
		out := cand
		return &out, nil
	}
	return nil, nil
}

// reserves reports whether any of these cells is space a turning part has.
func (s *Searcher) reserves(cells []geom.Cell) bool {
	if len(s.Reserved) == 0 {
		return false
	}
	for _, c := range cells {
		if s.Reserved[c] {
			return true
		}
	}
	return false
}

// hole is a port with the direction it faces, which is what decides whether a
// pin can pass through it and another.
type hole struct {
	pos  geom.Vec3
	axis geom.Vec3
}

func (s *Searcher) holesOf(chosen []Placed, idx []int) (map[geom.Vec3]bool, error) {
	out := map[geom.Vec3]bool{}
	for _, h := range s.holeRefs(chosen, idx) {
		out[h.pos.Round(3)] = true
	}
	return out, nil
}

func (s *Searcher) holeRefs(chosen []Placed, idx []int) []hole {
	var out []hole
	for _, i := range idx {
		pts, axis, err := part.WorldHoles(s.Axes, chosen[i], s.counts[chosen[i].Part])
		if err != nil {
			continue
		}
		for _, p := range pts {
			out = append(out, hole{pos: p.Round(3), axis: axis})
		}
	}
	return out
}

// joins reports whether a candidate could take a pin to any of these holes.
//
// Both conditions matter and the second is the one that catches wishful
// bridges: the holes must face the same way, and the gap between them must lie
// ALONG that shared direction. A beam laid across two holes that face along the
// line separating them cannot be pinned to either, however exactly it reaches.
func joins(cand []hole, targets []hole) bool {
	for _, c := range cand {
		for _, t := range targets {
			if math.Abs(math.Abs(c.axis.Dot(t.axis))-1) > 1e-6 {
				continue
			}
			d := t.pos.Sub(c.pos)
			along := d.Dot(c.axis)
			if d.Sub(c.axis.Scale(along)).Len() > 1e-6 {
				continue
			}
			if math.Abs(along) <= rigidity.PinReach+1e-6 {
				return true
			}
		}
	}
	return false
}

func (s *Searcher) absoluteCells(p Placed) ([]geom.Cell, error) {
	c, err := s.describe(p)
	if err != nil {
		return nil, err
	}
	return c.cells, nil
}

// ConnectorsBetween finds beams tying two separate pieces together.
//
// These bear no shaft at all; they exist purely to create connectivity.
// Generated deliberately rather than blindly: take a hole from one piece and one
// from the other, and if they lie on a straight line with a whole number of
// studs between them, a beam of the right length spans both.
func (s *Searcher) ConnectorsBetween(holesA, holesB map[geom.Vec3]bool) ([]Placed, error) {
	seen := map[Placed]bool{}
	var out []Placed

	for ha := range holesA {
		for hb := range holesB {
			span, ok := studSpan(ha, hb)
			if !ok {
				continue
			}
			found, err := s.beamsSpanning(ha, hb, span)
			if err != nil {
				return nil, err
			}
			for _, p := range found {
				if !seen[p] {
					seen[p] = true
					out = append(out, p)
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Part != out[j].Part {
			return out[i].Part < out[j].Part
		}
		return less(out[i].Origin, out[j].Origin)
	})
	return out, nil
}

// studSpan reports how many studs apart two holes are, and whether they line up
// along one axis at all.
func studSpan(a, b geom.Vec3) (int, bool) {
	v := b.Sub(a)
	length := v.Len()
	if length < 1e-6 {
		return 0, false
	}
	k := length / part.Stud
	if math.Abs(k-math.Round(k)) > 1e-6 {
		return 0, false // not on the hole pitch
	}
	d := v.Scale(1 / length)
	axes := 0
	for _, c := range [3]float64{d.X, d.Y, d.Z} {
		if math.Abs(c) > 1e-6 {
			axes++
		}
	}
	if axes != 1 {
		return 0, false // straight directions only
	}
	return int(math.Round(k)), true
}

// beamsSpanning lists the beams long enough to reach across, laid so both holes
// fall on them.
func (s *Searcher) beamsSpanning(ha, hb geom.Vec3, span int) ([]Placed, error) {
	d := hb.Sub(ha).Unit()
	var out []Placed

	for _, beam := range s.Inventory {
		if beam.Holes < span+1 {
			continue
		}
		local, err := s.localAxis(beam.Part)
		if err != nil {
			continue
		}
		rots, err := s.rotations(beam.Part)
		if err != nil {
			return nil, err
		}
		offsets := part.HoleOffsets(beam.Holes)

		for _, ri := range rots {
			r := geom.Rotations[ri]
			// The beam's length has to lie along d, and its hole axis across it.
			if math.Abs(math.Abs(r.Apply(geom.Vec3{Z: 1}).Dot(d))-1) > 1e-6 {
				continue
			}
			if math.Abs(r.Apply(local).Dot(d)) > 1e-6 {
				continue
			}
			// A bridge whose holes land exactly on the targets sits in the
			// same plane as the parts it joins, and runs straight through
			// them. Real construction lays it alongside and pins through, so
			// the sideways offsets are tried as well: a part's width along the
			// hole axis, either way, which is within a pin's reach.
			holeAxis := r.Apply(local)
			for _, shift := range []float64{0, part.Stud, -part.Stud} {
				beside := holeAxis.Scale(shift)
				for _, off := range offsets {
					origin := ha.Add(beside).Sub(r.Apply(off))
					if !origin.OnLattice(HalfStud) {
						continue
					}
					if !spansBoth(r, origin, offsets, ha.Add(beside), hb.Add(beside)) {
						continue
					}
					out = append(out, Placed{Part: beam.Part, Rot: ri, Origin: origin.Round(3)})
				}
			}
		}
	}
	return out, nil
}

func spansBoth(r geom.Mat3, origin geom.Vec3, offsets []geom.Vec3, ha, hb geom.Vec3) bool {
	var reachesA, reachesB bool
	for _, off := range offsets {
		h := r.Apply(off).Add(origin).Round(3)
		if h == ha.Round(3) {
			reachesA = true
		}
		if h == hb.Round(3) {
			reachesB = true
		}
	}
	return reachesA && reachesB
}

// AbsoluteCells is absoluteCells, for tests in other packages.
func (s *Searcher) AbsoluteCells(p Placed) ([]geom.Cell, error) {
	return s.absoluteCells(p)
}
