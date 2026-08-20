// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package teeth

import (
	"testing"

	"github.com/sstriker/brickmesh/internal/collide"
	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/interfere"
)

// The point of a phase is that the gears can actually be put together. Reading
// the angles is only worth doing if the pair goes from interpenetrating to
// assemblable, so that is what this checks: solid at zero, free once turned.
func TestPhaseTurnsAJamIntoAFit(t *testing.T) {
	lib := requireLibraries(t)

	const part, count = "3648b.dat", 24
	// The reference distance from docs/findings.md: two 24t mesh at 60 LDU.
	const dist = 60.0

	mesh, err := interfere.MeshFor(lib, part)
	if err != nil {
		t.Fatal(err)
	}
	z := geom.Vec3{Z: 1}
	toward := geom.Vec3{X: 1}

	phase, err := MeshPhase(lib, part, count, z, part, count, z, toward)
	if err != nil {
		t.Fatal(err)
	}

	at := func(rotA, rotB float64) bool {
		a := collide.Transform{Rot: interfere.Rot('z', rotA)}
		b := collide.Transform{
			Rot: interfere.Rot('z', rotB),
			Pos: geom.Vec3{X: dist},
		}
		return collide.Intersects(mesh, a, mesh, b)
	}

	if !at(0, 0) {
		t.Error("unphased, two 24t at 60 LDU should have teeth passing through " +
			"each other; if they do not, the phase is solving nothing")
	}
	if at(phase.RotA, phase.RotB) {
		t.Errorf("phased to %.2f/%.2f the teeth still collide, so the phase is wrong",
			phase.RotA, phase.RotB)
	}
}
