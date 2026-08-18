// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"math"

	"brickmesh/internal/geom"
	"brickmesh/internal/interfere"
	"brickmesh/internal/ldr"
	"brickmesh/internal/mech"
	"brickmesh/internal/teeth"
)

// applyPhase turns each gear about its own axis so its teeth interleave with
// its partner's rather than passing through them.
//
// A shaft that already carries a phased gear is left alone: once one gear on it
// is set, the shaft's rotation is fixed and every other gear on it comes along.
// Trying to set a second would only undo the first, and the difference between
// what the two want is what backlash has to absorb — which mech reports
// separately.
func applyPhase(m *mech.Mechanism, res *Result, deps Deps) {
	if res.Layout == nil || res.Model == nil {
		return
	}
	fixed := map[string]bool{}
	var turned, doubted int

	for _, link := range m.Links {
		mesh, ok := link.(mech.Mesh)
		if !ok || mesh.Kind != mech.Spur {
			continue // only a parallel pair has a phase this can read
		}
		placeA, okA := res.Layout.Place[mesh.A]
		placeB, okB := res.Layout.Place[mesh.B]
		if !okA || !okB {
			continue
		}
		toward := placeB.Point.Sub(placeA.Point)
		toward = toward.Sub(placeA.Direction.Scale(toward.Dot(placeA.Direction)))
		if toward.Len() < 1e-9 {
			continue
		}
		// A gear's teeth are read from the part file, so the reading happens in
		// the part's own frame. buildModel places gears with alignZTo, so that
		// frame always has the axis along Z and the line of centers has to be
		// carried back into it. Handing the world direction to a reader of local
		// geometry is how the 40t warning fired for a gearbox with no 40t in it.
		rotA, okRA := alignZTo(placeA.Direction)
		rotB, okRB := alignZTo(placeB.Direction)
		if !okRA || !okRB {
			continue
		}
		localZ := geom.Vec3{Z: 1}

		phase, err := teeth.MeshPhase(deps.Lib,
			gearPart(mesh.TeethA), mesh.TeethA, localZ,
			gearPart(mesh.TeethB), mesh.TeethB, localZ,
			rotA.Transpose().Apply(toward))
		if err != nil {
			continue // no part for it, or nothing to read; already reported
		}
		// B reads its own line of centers, which points the other way.
		phaseB, err := teeth.MeshPhase(deps.Lib,
			gearPart(mesh.TeethB), mesh.TeethB, localZ,
			gearPart(mesh.TeethA), mesh.TeethA, localZ,
			rotB.Transpose().Apply(toward.Scale(-1)))
		if err != nil {
			continue
		}
		// Each gear's own reading of "a tooth toward my partner"; B then backs
		// off half a pitch so a gap meets that tooth.
		phase.RotB = wrapPitch(phaseB.RotA+phase.PitchB/2, phase.PitchB)
		if phase.SharpA < teeth.TrustThreshold || phase.SharpB < teeth.TrustThreshold {
			doubted++
		}

		for _, side := range []struct {
			shaft string
			teeth int
			angle float64
			axis  geom.Vec3
		}{
			{mesh.A, mesh.TeethA, phase.RotA, placeA.Direction},
			{mesh.B, mesh.TeethB, phase.RotB, placeB.Direction},
		} {
			if fixed[side.shaft] {
				continue
			}
			if turnGears(res.Model, side.shaft, side.teeth, side.angle, side.axis) {
				fixed[side.shaft] = true
				turned++
			}
		}
	}

	if turned == 0 {
		return
	}
	detail := fmt.Sprintf("%d gear(s) turned so their teeth interleave", turned)
	level := "OK"
	if doubted > 0 {
		level = "WARN"
		detail += fmt.Sprintf("; %d pair(s) read below a sharpness of %g, so their "+
			"phase is a guess — the 40t is the usual culprit",
			doubted, teeth.TrustThreshold)
	}
	res.Findings = append(res.Findings, mech.Finding{
		Level: level, Check: "tooth phase", Detail: detail,
	})
}

// turnGears rotates every gear of a tooth count on a shaft about that shaft.
func turnGears(model *ldr.Model, shaft string, count int, angle float64,
	axis geom.Vec3) bool {

	want := gearPart(count)
	turnedAny := false
	for i := range model.Parts {
		p := &model.Parts[i]
		if p.Name != want {
			continue
		}
		if got, ok := shaftFromLabel(p.Label); !ok || got != shaft {
			continue
		}
		// About the shaft, which is where the gear turns anyway.
		p.Rot = interfere.RotAbout(axis, angle).Mul(p.Rot)
		turnedAny = true
	}
	return turnedAny
}

// wrapPitch folds an angle into one pitch window.
func wrapPitch(deg, pitch float64) float64 {
	v := math.Mod(deg, pitch)
	if v < 0 {
		v += pitch
	}
	return v
}

func gearPart(teethCount int) string {
	if name, ok := GearParts[teethCount]; ok {
		return name
	}
	return ""
}
