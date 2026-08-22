// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/ldr"
	"github.com/sstriker/brickmesh/internal/part"
)

// Every line the engine put a shaft on, its own frame offers back.
//
// The check that matters for fitting into a model somebody else built: if the
// bearings a frame provides cannot be found by reading it, nothing can be
// fitted to it.
func TestAFrameOffersBackTheLinesItsShaftsRunOn(t *testing.T) {
	deps := requireLibraries(t)
	for _, name := range []string{"reduction", "gearbox-2-speed"} {
		t.Run(name, func(t *testing.T) {
			res := runSpec(t, deps, filepath.Join("..", "..", "examples", name+".json"))
			parts, err := ldr.Decode(strings.NewReader(res.Model.Encode()))
			if err != nil {
				t.Fatal(err)
			}
			offered := map[[6]float64]bool{}
			for _, b := range Inspect(parts).Bearings(deps.Shadow) {
				offered[lineKey(b.At, b.Axis)] = true
			}
			for id, place := range res.Layout.Place {
				want := lineKey(place.Point.Scale(10), place.Direction.Unit())
				if !offered[want] {
					t.Errorf("shaft %q runs on a line the frame does not offer "+
						"back when read: nothing could be fitted to this model", id)
				}
			}
		})
	}
}

// A cross hole is where a shaft is keyed, not where it turns, so it is not a
// bearing. Getting this backwards offers a mechanism somewhere it would seize.
func TestACrossHoleIsNotABearing(t *testing.T) {
	src := holesAt{
		"cross.dat": {{Axis: geom.Vec3{X: 1}, Cross: true}},
		"round.dat": {{Axis: geom.Vec3{X: 1}}},
	}
	r := &Reading{Parts: []Found{
		{Placed: ldr.Placed{Name: "cross.dat", Rot: geom.Rotations[0]}, Class: classStructure},
		{Placed: ldr.Placed{Name: "cross.dat",
			Rot: geom.Rotations[0], Pos: geom.Vec3{X: 100}}, Class: classStructure},
	}}
	if got := r.Bearings(src); len(got) != 0 {
		t.Errorf("two cross holes on a line offered %d bearing(s); an axle is "+
			"keyed in one, not turning", len(got))
	}
	for i := range r.Parts {
		r.Parts[i].Name = "round.dat"
	}
	if got := r.Bearings(src); len(got) != 1 {
		t.Errorf("two round holes on a line should offer one bearing, got %d",
			len(got))
	}
}

// One hole holds a pin. It takes two to hold a shaft.
func TestOneHoleIsNotABearing(t *testing.T) {
	src := holesAt{"round.dat": {{Axis: geom.Vec3{X: 1}}}}
	r := &Reading{Parts: []Found{
		{Placed: ldr.Placed{Name: "round.dat", Rot: geom.Rotations[0]}, Class: classStructure},
	}}
	if got := r.Bearings(src); len(got) != 0 {
		t.Errorf("one hole offered %d bearing(s)", len(got))
	}
}

// A gear's bore is not support: it is a thing the shaft carries.
func TestAGearsBoreIsNotABearing(t *testing.T) {
	src := holesAt{"round.dat": {{Axis: geom.Vec3{X: 1}}}}
	r := &Reading{Parts: []Found{
		{Placed: ldr.Placed{Name: "round.dat", Rot: geom.Rotations[0]},
			Class: classGear, Teeth: 16},
		{Placed: ldr.Placed{Name: "round.dat", Rot: geom.Rotations[0],
			Pos: geom.Vec3{X: 100}}, Class: classGear, Teeth: 16},
	}}
	if got := r.Bearings(src); len(got) != 0 {
		t.Errorf("two gear bores offered %d bearing(s); a gear is carried by a "+
			"shaft, not carrying one", len(got))
	}
}

// holesAt is a stand-in for the shadow library, so what counts as a bearing can
// be asked without a part that happens to have the right holes.
type holesAt map[string][]part.Hole

func (h holesAt) Holes(name string) []part.Hole { return h[name] }

func (h holesAt) RotationAxis(string) (geom.Vec3, string, bool) {
	return geom.Vec3{}, "", false
}
