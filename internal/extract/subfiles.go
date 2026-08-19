// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package extract

import (
	"sort"
	"strings"

	"brickmesh/internal/geom"
	"brickmesh/internal/part"
	"brickmesh/internal/shadow"
)

// unmoved is the frame a part is read in, before any subfile moves it.
var unmoved = geom.Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}

// maxSubfileDepth bounds the walk. LDraw parts nest a few levels — part,
// subpart, primitive — and anything deeper is a cycle.
const maxSubfileDepth = 8

// EntryForWith builds the ports of one part from its own shadow file and from
// the shadow files of every primitive it places.
//
// A beam's shadow file declares one hole. The other twelve are `beamhole.dat`
// references in the LDraw part, and `p/beamhole.dat` in the shadow library is
// what says a beamhole is a hole. LDCad resolves that at load time; reading the
// part's own shadow file alone gives a thirteen-hole beam with one hole in it,
// which is what the structural search was working around by synthesising hole
// positions from a count and giving every part a single hole axis.
//
// Recursion stops at any subfile the shadow library describes: a shadow file
// for a primitive describes that primitive completely, and descending into it
// would count the same hole again from its parts.
func EntryForWith(lib *shadow.Library, refs part.Subfiles, part string) *Entry {
	e := EntryFor(lib, part)
	if refs == nil {
		return e
	}
	if e == nil {
		e = &Entry{Part: part}
	}
	collect(lib, refs, filename(part), unmoved, geom.Vec3{}, 0, e)
	dedupe(e)
	if len(e.Holes) == 0 && len(e.Pins) == 0 {
		return nil
	}
	return e
}

// filename turns a part id into the file that holds it.
//
// The shadow library enumerates bare ids and the parts library is a directory
// of .dat files, and the two have been mistaken for each other before — with
// the same symptom both times, which is nothing at all: a lookup that misses
// returns no ports, and a part with no ports is indistinguishable from a part
// this does not describe. Hence also the count Build logs.
func filename(part string) string {
	if strings.Contains(part, ".") {
		return part
	}
	return part + ".dat"
}

func collect(lib *shadow.Library, refs part.Subfiles, name string,
	rot geom.Mat3, pos geom.Vec3, depth int, into *Entry) {

	if depth >= maxSubfileDepth {
		return
	}
	children, err := refs.Refs(name)
	if err != nil {
		return // a primitive the library does not carry is not an error here
	}
	for _, c := range children {
		childRot := rot.Mul(c.Rot)
		childPos := rot.Apply(c.Pos).Add(pos)
		if sub := EntryFor(lib, c.Name); sub != nil {
			place(sub, childRot, childPos, into)
			continue // described here; its own parts would say it again
		}
		collect(lib, refs, c.Name, childRot, childPos, depth+1, into)
	}
}

// place moves a subfile's ports into the frame of the part that holds it.
func place(sub *Entry, rot geom.Mat3, pos geom.Vec3, into *Entry) {
	move := func(p Port) Port {
		at := rot.Apply(geom.Vec3{X: p[0], Y: p[1], Z: p[2]}).Add(pos)
		ax := rot.Apply(geom.Vec3{X: p[3], Y: p[4], Z: p[5]}).Unit()
		return Port{
			roundTo(at.X, 2), roundTo(at.Y, 2), roundTo(at.Z, 2),
			roundTo(ax.X, 3), roundTo(ax.Y, 3), roundTo(ax.Z, 3),
			p[6],
		}
	}
	for _, p := range sub.Holes {
		into.Holes = append(into.Holes, move(p))
		if p[6] != 0 {
			into.AxleHoles++
		}
	}
	for _, p := range sub.Pins {
		into.Pins = append(into.Pins, move(p))
	}
}

// dedupe drops ports that are the same connection point reached twice.
//
// A beam hole is a primitive placed once, but its two rims are separate
// primitives placed at either face, and a part often places a hole primitive
// and then a shadow file names the same hole again. They are one hole.
//
// A hole has no direction — a pin goes through it either way — so two ports at
// the same point on the same line are the same port whichever way their axes
// were written. Pins are matched the same way, since the same reasoning applies
// to which end of a pin the shadow file happened to describe.
func dedupe(e *Entry) {
	e.Holes, e.AxleHoles = unique(e.Holes)
	e.Pins, _ = unique(e.Pins)
}

func unique(ports []Port) ([]Port, int) {
	type key struct {
		x, y, z, ax, ay, az, cross float64
	}
	seen := map[key]bool{}
	out := make([]Port, 0, len(ports))
	axles := 0
	for _, p := range ports {
		// Sign-free axis, so the same hole written either way collapses.
		ax, ay, az := p[3], p[4], p[5]
		if ax < 0 || (ax == 0 && ay < 0) || (ax == 0 && ay == 0 && az < 0) {
			ax, ay, az = -ax, -ay, -az
		}
		k := key{p[0], p[1], p[2], zeroed(ax), zeroed(ay), zeroed(az), p[6]}
		if seen[k] {
			continue
		}
		seen[k] = true
		q := p
		q[3], q[4], q[5] = zeroed(ax), zeroed(ay), zeroed(az)
		out = append(out, q)
		if p[6] != 0 {
			axles++
		}
	}
	sort.Slice(out, func(i, j int) bool {
		for k := 0; k < 7; k++ {
			if out[i][k] != out[j][k] {
				return out[i][k] < out[j][k]
			}
		}
		return false
	})
	return out, axles
}

// zeroed turns -0 into 0, so two axes that differ only in the sign of a zero
// land in the same bucket.
func zeroed(v float64) float64 {
	if v == 0 {
		return 0
	}
	return v
}
