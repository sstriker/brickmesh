// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/sstriker/brickmesh/internal/collide"
	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/interfere"
	"github.com/sstriker/brickmesh/internal/ldr"
	"github.com/sstriker/brickmesh/internal/mech"
	"github.com/sstriker/brickmesh/internal/teeth"
)

// checkClearance asks the blunt question: is any part inside any other part.
//
// It used to ask a narrower one — does this ring clear its gear, does this
// joiner clear the structure — and narrow questions are how things get missed.
// Gears were not on the list, so models came out with beams drawn through them
// and nothing said a word. Enumerating which parts to compare is a list that is
// wrong the moment a new kind of part is placed.
//
// So the rule is that nothing shares space, and the exceptions are written down
// once, in mayBeInside. Every pair, every time. It is O(n^2) in the parts, but
// the parts number in the tens and a box test rejects nearly all of it.
func checkClearance(ctx context.Context, res *Result, deps Deps) error {
	if res.Model == nil || deps.Lib == nil {
		return nil
	}
	// What turns is worked out from the joints rather than from what a part is
	// called: keyed to a shaft by a cross hole, or pinned to something that is.
	// A part swept about the wrong axis is worse than one not swept at all.
	spin := checkTurning(res, deps)
	parts := res.Model.Parts

	// A part with no triangles cannot be swept, cannot be compared, and was
	// being skipped without a word — so a model missing half its geometry was
	// reported as a model in which nothing collides. Say so instead: this is
	// the difference between "checked and clear" and "not checked".
	missing := withoutGeometry(deps, parts)
	if len(missing) > 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "FAIL", Check: "clearance", Detail: fmt.Sprintf(
				"no geometry for %s, so nothing was checked against %s. "+
					"A clear verdict here would be a verdict on the parts that "+
					"were left, not on the model",
				strings.Join(missing, ", "), plural(len(missing)))})
	}
	clashes := 0
	for i := 0; i < len(parts); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		for j := i + 1; j < len(parts); j++ {
			a, b := parts[i], parts[j]
			if mayBeInside(a, b) {
				continue
			}
			inside, overlap, err := sharesSpace(ctx, deps, a, b, spin, i, j)
			if err != nil {
				return err
			}
			if !inside {
				continue
			}
			clashes++
			res.Findings = append(res.Findings, mech.Finding{
				Level: "FAIL", Check: "clearance", Detail: fmt.Sprintf(
					"%s at %+v is inside %s at %+v, by %.1f LDU. Nothing may share "+
						"space with anything but the few fits in mayBeInside",
					a.Name, a.Pos, b.Name, b.Pos, overlap)})
		}
	}
	// Only clear when everything was actually looked at. Saying "no two of the
	// 9 parts share space" beside "no geometry for 3648b.dat" is two answers to
	// one question, and the reassuring one is the one that gets read.
	if clashes == 0 && len(missing) == 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "OK", Check: "clearance", Detail: fmt.Sprintf(
				"no two of the %d parts share space", len(parts))})
	}
	return nil
}

// sharesSpace reports whether two placed parts occupy the same space, and by
// how much.
//
// A part that turns is swept a full revolution rather than tested where it
// happens to sit: a gear that clears a beam at rest and strikes it a quarter
// turn later is not a gear that clears a beam.
// withoutGeometry names the placed parts whose triangles are not available,
// once each, in the order they first appear.
func withoutGeometry(deps Deps, parts []ldr.Part) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range parts {
		if seen[p.Name] {
			continue
		}
		seen[p.Name] = true
		if _, err := interfere.MeshFor(deps.Lib, p.Name); err != nil {
			out = append(out, p.Name)
		}
	}
	sort.Strings(out)
	return out
}

func plural(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

func sharesSpace(ctx context.Context, deps Deps, a, b ldr.Part,
	spin turning, ia, ib int) (bool, float64, error) {

	alo, ahi, err := placedBox(deps, a)
	if err != nil {
		return false, 0, nil
	}
	blo, bhi, err := placedBox(deps, b)
	if err != nil {
		return false, 0, nil
	}
	// Widened to the circle each turning part sweeps out before the boxes are
	// compared, so the cheap rejection stays sound for something rotating.
	axisA, spinsA := spin.about[ia]
	axisB, spinsB := spin.about[ib]
	if spinsA {
		alo, ahi, _ = sweptBox(deps, a, axisA)
	}
	if spinsB {
		blo, bhi, _ = sweptBox(deps, b, axisB)
	}
	overlap := overlapOf(alo, ahi, blo, bhi)
	if overlap < touchTolerance {
		return false, 0, nil // apart, or meeting face to face, which is ordinary
	}

	ma, err := interfere.MeshFor(deps.Lib, a.Name)
	if err != nil {
		return false, 0, nil
	}
	mb, err := interfere.MeshFor(deps.Lib, b.Name)
	if err != nil {
		return false, 0, nil
	}
	if spinsA || spinsB {
		rider, ta, other, tb := ma, a, mb, b
		if !spinsA {
			rider, ta, other, tb = mb, b, ma, a
		}
		spinAxis := axisA
		if !spinsA {
			spinAxis = axisB
		}
		got, err := interfere.MeshLock(ctx,
			other, collide.Transform{Rot: tb.Rot, Pos: tb.Pos},
			rider, collide.Transform{Rot: ta.Rot, Pos: ta.Pos},
			16, interfere.Options{Steps: 72, SpinAxis: principal(spinAxis.dir),
				Fit: interfere.FitTolerance})
		if err != nil {
			return false, 0, err
		}
		return got.Verdict != interfere.NoEngagement, overlap, nil
	}
	return collide.Intersects(ma, collide.Transform{Rot: a.Rot, Pos: a.Pos},
		mb, collide.Transform{Rot: b.Rot, Pos: b.Pos}), overlap, nil
}

// mayBeInside is every pair of things meant to occupy the same space.
//
// Short on purpose, and every entry is a fit the shadow library describes: an
// axle in a bore, a ring on the ridges of its joiner, a ring's dogs in a gear's
// recesses, two gears meshing. Anything else sharing space is a model that
// cannot be built.
func mayBeInside(a, b ldr.Part) bool {
	ka, kb := classOf(a), classOf(b)
	if ka > kb {
		ka, kb = kb, ka
	}
	switch {
	case ka == classAxle || kb == classAxle:
		return true // an axle goes through bores, joiners and beam holes
	case ka == classPin || kb == classPin:
		// A pin is the same fit as an axle and for the same reason: it is a
		// cylinder that lives inside the holes of the things it fastens. It is
		// only here at all because the joints are now built rather than merely
		// counted.
		return true
	case ka == classGear && kb == classRing:
		return true // dogs in the recesses: the engagement itself
	case ka == classGear && kb == classGear:
		return true // meshing
	case ka == classRing && kb == classJoiner:
		return true // the ring is splined to it
	case ka == classRing && kb == classSelector:
		// A catch's fork straddles the ring's groove. It is a loose fit — the
		// ring turns inside it — but LDraw models nominal surfaces, so the two
		// touch. Measured against the official 8448, which puts them exactly
		// here.
		return true
	}
	return false
}

// What a part is, for deciding what it may be inside.
const (
	classGear = iota
	classRing
	classJoiner
	classAxle
	classPin
	classSelector
	classStructure
	// A marker is a liftarm on the end of a shaft, put there to be looked at.
	// It turns with its shaft, so it is not structure — and the frame checks
	// mean it when they say a part that turns holds nothing.
	classMarker
)

func classOf(p ldr.Part) int {
	switch {
	case isRing(p.Name):
		return classRing
	case isJoiner(p.Name):
		return classJoiner
	case isAxle(p.Name):
		return classAxle
	case isPin(p.Name):
		return classPin
	case isSelector(p.Name):
		return classSelector
	case isMarker(p):
		return classMarker
	}
	if _, _, ok := gearFromLabel(p.Label); ok {
		return classGear
	}
	return classStructure
}

// isMarker is the flag on a shaft end. Asked of the label rather than the part,
// because the same liftarm is perfectly good structure somewhere else — what
// makes it a marker is that this put it there to be watched.
func isMarker(p ldr.Part) bool {
	return p.Name == MarkerPart && strings.Contains(p.Label, "marker on shaft")
}

// isPin is the fasteners the engine places. Named rather than inferred, the
// same way the axles are: a part is a pin here because this put it there.
func isPin(name string) bool {
	return name == PinPart || name == AxlePinPart || name == LongPinPart
}

func isAxle(name string) bool {
	for _, a := range AxleParts {
		if a == name {
			return true
		}
	}
	return false
}

// turns reports whether a part rides a shaft, and so has to be swept rather
// than tested where it stands.
func turns(p ldr.Part) bool {
	switch classOf(p) {
	case classGear, classRing, classJoiner, classMarker:
		return true
	}
	return false
}

// touchTolerance is how much two parts may share before it is an overlap
// rather than a touch, in LDU.
//
// Parts that meet face to face are ordinary, and the triangle test counts two
// coplanar faces as intersecting: a bearing right against the end of a ring's
// travel reads as blocked at every angle, which is the same answer a ring
// buried in a beam gives. Boxes that overlap by less than this are touching,
// and touching is allowed.
const touchTolerance = 1.0

// placedBox is a part's bounding box where it stands.
func placedBox(deps Deps, p ldr.Part) (geom.Vec3, geom.Vec3, error) {
	g, err := deps.Lib.Geometry(p.Name)
	if err != nil {
		return geom.Vec3{}, geom.Vec3{}, err
	}
	lo := geom.Vec3{X: math.Inf(1), Y: math.Inf(1), Z: math.Inf(1)}
	hi := geom.Vec3{X: math.Inf(-1), Y: math.Inf(-1), Z: math.Inf(-1)}
	for _, t := range g.Tris {
		for _, v := range [3]geom.Vec3{t[0], t[1], t[2]} {
			w := p.Rot.Apply(v).Add(p.Pos)
			lo = geom.Vec3{X: math.Min(lo.X, w.X), Y: math.Min(lo.Y, w.Y), Z: math.Min(lo.Z, w.Z)}
			hi = geom.Vec3{X: math.Max(hi.X, w.X), Y: math.Max(hi.Y, w.Y), Z: math.Max(hi.Z, w.Z)}
		}
	}
	return lo, hi, nil
}

// sweptBox is the box a turning part needs: its own, widened across its axis to
// the circle it sweeps out.
// sweptBox is the space a part needs while it turns: its own extent along the
// axis, widened across it to the circle it sweeps.
//
// The axis is the one it inherited, not its own. A liftarm keyed to a shaft
// sweeps about that shaft; a beam pinned to the end of the liftarm sweeps a
// wider circle about the same shaft, and measuring it about its own centre
// would understate it badly.
func sweptBox(deps Deps, p ldr.Part, about axis) (geom.Vec3, geom.Vec3, error) {
	g, err := deps.Lib.Geometry(p.Name)
	if err != nil {
		return geom.Vec3{}, geom.Vec3{}, err
	}
	dir := about.dir.Unit()
	// Both ends from infinity. Starting the far end at zero was harmless while
	// distances were measured from the part's own origin, since those straddle
	// it — but they are measured from a point on the axis now, and a part
	// entirely on one side of that point had its box stretched back to it.
	var radius float64
	along, alongLo := math.Inf(-1), math.Inf(1)
	for _, t := range g.Tris {
		for _, v := range [3]geom.Vec3{t[0], t[1], t[2]} {
			// Measured from a point on the axis, so a part away from it gets
			// the radius of the circle it actually travels.
			w := p.Rot.Apply(v).Add(p.Pos).Sub(about.at)
			d := w.Dot(dir)
			along = math.Max(along, d)
			alongLo = math.Min(alongLo, d)
			radius = math.Max(radius, w.Sub(dir.Scale(d)).Len())
		}
	}
	u, v := teeth.Frame(dir)
	lo := geom.Vec3{X: math.Inf(1), Y: math.Inf(1), Z: math.Inf(1)}
	hi := geom.Vec3{X: math.Inf(-1), Y: math.Inf(-1), Z: math.Inf(-1)}
	for _, d := range []float64{alongLo, along} {
		for _, a := range []float64{-radius, radius} {
			for _, b := range []float64{-radius, radius} {
				w := dir.Scale(d).Add(u.Scale(a)).Add(v.Scale(b)).Add(about.at)
				lo = geom.Vec3{X: math.Min(lo.X, w.X), Y: math.Min(lo.Y, w.Y), Z: math.Min(lo.Z, w.Z)}
				hi = geom.Vec3{X: math.Max(hi.X, w.X), Y: math.Max(hi.Y, w.Y), Z: math.Max(hi.Z, w.Z)}
			}
		}
	}
	return lo, hi, nil
}

// overlapOf is how far two boxes share, along whichever axis they share least.
// Zero or less means they are apart.
func overlapOf(alo, ahi, blo, bhi geom.Vec3) float64 {
	return math.Min(
		math.Min(math.Min(ahi.X, bhi.X)-math.Max(alo.X, blo.X),
			math.Min(ahi.Y, bhi.Y)-math.Max(alo.Y, blo.Y)),
		math.Min(ahi.Z, bhi.Z)-math.Max(alo.Z, blo.Z))
}

// principal names the axis a sweep spins about. The shafts run along lattice
// directions, so one of the three always fits.
func principal(d geom.Vec3) byte {
	switch {
	case math.Abs(d.X) > 0.9:
		return 'x'
	case math.Abs(d.Y) > 0.9:
		return 'y'
	}
	return 'z'
}
