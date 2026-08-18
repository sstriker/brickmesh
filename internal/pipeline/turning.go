// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"math"
	"strings"

	"brickmesh/internal/geom"
	"brickmesh/internal/ldr"
	"brickmesh/internal/mech"
	"brickmesh/internal/shadow"
)

// turning works out what moves when a shaft turns, and about which axis.
//
// The rule is what an axle does to the hole it is in. A round hole lets the
// axle spin inside it: that is a bearing, the part stays put, and rotation
// stops there. A cross-shaped hole cannot let it spin, so the part is keyed to
// the axle and goes round with it.
//
// And it does not stop at that part. Whatever is pinned to a keyed part goes
// round too — about the same axis, not about its own pin. A liftarm keyed to a
// shaft sweeps its own length; a beam pinned to the end of that liftarm sweeps
// a wider circle about the same shaft. So the axis is inherited along the
// joints rather than recomputed at each one.
//
// But only if the joint holds it still. One pin is a hinge: the part is carried
// round the shaft and is also free to swing about the pin, so it does not sweep
// a circle at all, it can be anywhere within reach of one. Two pins in
// different places settle it, because turning about either would break the
// other. Two pins on the same line do not — that is exactly the case a hinge
// is made of.
//
// So a part reached by a single pin is not given an axis. It is reported as
// free to swing, which is a fact about the mechanism rather than about
// clearance, and it is left out of the sweep rather than swept about an axis it
// is not held to.
//
// Reaching one part from two different axes means it cannot turn at all: it is
// keyed to two shafts, or keyed to one and pinned to something that is not
// going anywhere. That is a locked mechanism and is reported rather than
// resolved by choosing an axis.
type turning struct {
	// about maps a part index to the axis it turns about.
	about map[int]axis
	// locked names parts reached from more than one axis.
	locked []string
	// swinging names parts carried by a single pin, which are free to turn
	// about that pin as well as about the shaft that carries it.
	swinging []string
}

// axis is a line in the model: a point on it and a direction.
type axis struct {
	at  geom.Vec3
	dir geom.Vec3
}

func (a axis) sameAs(b axis) bool {
	if math.Abs(math.Abs(a.dir.Dot(b.dir))-1) > 1e-6 {
		return false
	}
	d := b.at.Sub(a.at)
	return d.Sub(a.dir.Scale(d.Dot(a.dir))).Len() < 1e-6
}

// port is one of a placed part's connection points, in the model's coordinates.
type port struct {
	at    geom.Vec3
	dir   geom.Vec3
	cross bool // an axle keys into it rather than spinning inside it
}

// portsOf reads a placed part's connection points from the shadow library.
func portsOf(lib *shadow.Library, p ldr.Part) []port {
	var out []port
	for _, s := range lib.Snaps(strings.TrimSuffix(p.Name, ".dat")) {
		if s.Kind != "SNAP_CYL" {
			continue
		}
		out = append(out, port{
			at:  p.Rot.Apply(s.Pos).Add(p.Pos),
			dir: p.Rot.Apply(s.Axis()).Unit(),
			// Sections are a kind and a size: A is axle-shaped, R round, S
			// solid. One A section is enough to key the whole hole.
			cross: hasAxleSection(s.Secs),
		})
	}
	return out
}

func hasAxleSection(secs string) bool {
	for _, f := range strings.Fields(secs) {
		if f == "A" {
			return true
		}
	}
	return false
}

// whatTurns seeds from the shafts and spreads outward.
func whatTurns(res *Result, deps Deps) turning {
	got := turning{about: map[int]axis{}}
	if res.Model == nil || deps.Shadow == nil {
		return got
	}
	shafts := shaftAxes(res)
	ports := make([][]port, len(res.Model.Parts))
	for i, p := range res.Model.Parts {
		ports[i] = portsOf(deps.Shadow, p)
	}

	// Seeded two ways. The gears, rings and joiners are on a shaft by
	// construction; anything else has to be keyed to one through a cross hole.
	for i, p := range res.Model.Parts {
		if isAxle(p.Name) {
			continue // an axle is round about its own axis; sweeping says nothing
		}
		if a, ok := onAShaft(p, ports[i], shafts, turns(p)); ok {
			got.about[i] = a
		}
	}

	// Then outward: pinned to something that turns means turning about the same
	// axis. Repeated until nothing new is reached, which is at most once per
	// part.
	for again := true; again; {
		again = false
		for i := range res.Model.Parts {
			a, ok := got.about[i]
			if !ok {
				continue
			}
			for j := range res.Model.Parts {
				if i == j || isAxle(res.Model.Parts[j].Name) {
					continue
				}
				pins := pinsBetween(ports[i], ports[j])
				if len(pins) == 0 {
					continue
				}
				if !holdsStill(pins) {
					// A hinge. Carried round, and free to swing about the pin
					// while it goes: no one axis describes where it can be.
					got.swinging = appendOnce(got.swinging, res.Model.Parts[j].Name)
					continue
				}
				if was, seen := got.about[j]; seen {
					if !was.sameAs(a) {
						got.locked = appendOnce(got.locked, res.Model.Parts[j].Name)
					}
					continue
				}
				got.about[j] = a
				again = true
			}
		}
	}
	return got
}

// shaftAxes is every line a shaft runs along.
func shaftAxes(res *Result) []axis {
	var out []axis
	seen := map[[6]float64]bool{}
	for _, a := range res.Axles {
		key := [6]float64{a.Point.X, a.Point.Y, a.Point.Z, a.Dir.X, a.Dir.Y, a.Dir.Z}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, axis{at: a.Point, dir: a.Dir.Unit()})
	}
	return out
}

// onAShaft reports whether a part is carried round by a shaft: either because
// it is one of the parts that ride one by construction, or because it has a
// cross hole with a shaft through it.
func onAShaft(p ldr.Part, ports []port, shafts []axis, byConstruction bool) (axis, bool) {
	for _, s := range shafts {
		if byConstruction {
			// A gear's own axis is the shaft it sits on; find which.
			own := axis{at: p.Pos, dir: p.Rot.Apply(geom.Vec3{Z: 1}).Unit()}
			if own.sameAs(s) {
				return s, true
			}
			continue
		}
		for _, port := range ports {
			if !port.cross {
				continue // round: the axle spins inside it and nothing follows
			}
			if (axis{at: port.at, dir: port.dir}).sameAs(s) {
				return s, true
			}
		}
	}
	return axis{}, false
}

// pinsBetween is every place two parts share a pin: round ports at the same
// point, facing the same way.
func pinsBetween(a, b []port) []port {
	var out []port
	for _, x := range a {
		if x.cross {
			continue
		}
		for _, y := range b {
			if y.cross || x.at.Sub(y.at).Len() > 1e-6 {
				continue
			}
			if math.Abs(math.Abs(x.dir.Dot(y.dir))-1) < 1e-6 {
				out = append(out, x)
			}
		}
	}
	return out
}

// holdsStill reports whether a set of pins fixes a part rather than hinging it.
//
// One pin never does. Two or more do only if they are in different places and
// not all on one line: pins strung along a single line leave the part free to
// spin about that line, which is what a hinge with a long pin is.
func holdsStill(pins []port) bool {
	if len(pins) < 2 {
		return false
	}
	first := pins[0]
	for _, p := range pins[1:] {
		d := p.at.Sub(first.at)
		if d.Len() < 1e-6 {
			continue // the same pin found twice
		}
		// Off the line through the first pin along its own axis: that is a
		// second point of restraint and the part cannot turn.
		if d.Sub(first.dir.Scale(d.Dot(first.dir))).Len() > 1e-6 {
			return true
		}
	}
	return false
}

func appendOnce(list []string, name string) []string {
	for _, s := range list {
		if s == name {
			return list
		}
	}
	return append(list, name)
}

// checkTurning reports what the propagation found that the model cannot do.
func checkTurning(res *Result, deps Deps) turning {
	got := whatTurns(res, deps)
	if len(got.swinging) > 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "WARN", Check: "turning", Detail: fmt.Sprintf(
				"%v hang on a single pin from something that turns, so they are "+
					"carried round and free to swing about that pin at the same "+
					"time. Where they end up is not decided by the mechanism, so "+
					"they are left out of the clearance sweep: pin them in a "+
					"second place, or expect them to find their own way",
				got.swinging)})
	}
	if len(got.locked) > 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "FAIL", Check: "turning", Detail: fmt.Sprintf(
				"%v would have to turn about two axes at once: keyed to one "+
					"shaft and carried by another, or keyed to a shaft and pinned "+
					"to something that is not going anywhere. Nothing turns",
				got.locked)})
	}
	return got
}
