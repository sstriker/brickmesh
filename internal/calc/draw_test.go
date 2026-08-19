// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package calc

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"testing"

	"brickmesh/internal/extract"
	"brickmesh/internal/ldraw"
	"brickmesh/internal/shadow"
)

// The buffer the page uploads. Nothing on the page can tell whether it holds a
// model or noise — it draws either — so what it holds is checked here.
func TestTheDrawBufferIsTheModel(t *testing.T) {
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	lib := ldraw.New("")
	root, err := shadow.Ensure("")
	if err != nil {
		t.Fatal(err)
	}
	parts := publish(t, lib, extract.NewPorts(shadow.Open(root), lib), "")

	doc, err := os.ReadFile("../../examples/reduction.json")
	if err != nil {
		t.Fatal(err)
	}
	built := parts.Build(context.Background(), doc, false)
	if built.Error != "" {
		t.Fatal(built.Error)
	}
	raw := parts.Draw()
	if len(raw) == 0 {
		t.Fatal("no triangles came back for a model of 9 parts")
	}

	const stride = drawStride * 4
	if len(raw)%(stride*3) != 0 {
		t.Fatalf("%d bytes is not a whole number of triangles at %d bytes a vertex",
			len(raw), stride)
	}
	vertices := len(raw) / stride

	// Every value has to be a number: one NaN in a buffer and the whole model
	// vanishes, because the card takes it as a coordinate.
	var lo, hi [3]float64
	for k := range lo {
		lo[k], hi[k] = math.Inf(1), math.Inf(-1)
	}
	for v := 0; v < vertices; v++ {
		at := v * stride
		for f := 0; f < drawStride; f++ {
			got := float64(math.Float32frombits(
				binary.LittleEndian.Uint32(raw[at+f*4:])))
			if math.IsNaN(got) || math.IsInf(got, 0) {
				t.Fatalf("vertex %d value %d is %v", v, f, got)
			}
			if f < 3 {
				lo[f] = math.Min(lo[f], got)
				hi[f] = math.Max(hi[f], got)
			}
			if f >= 6 && (got < 0 || got > 1) {
				t.Fatalf("vertex %d colour %d is %v, outside 0..1", v, f-6, got)
			}
		}
		// The normal has to be a unit vector, or the shading is arbitrary.
		var length float64
		for f := 3; f < 6; f++ {
			n := float64(math.Float32frombits(
				binary.LittleEndian.Uint32(raw[at+f*4:])))
			length += n * n
		}
		if length > 0 && math.Abs(math.Sqrt(length)-1) > 1e-3 {
			t.Fatalf("vertex %d has a normal of length %v", v, math.Sqrt(length))
		}
	}

	// And it has to be the size of the mechanism it came from: two shafts two
	// studs apart with gears on them, not a point and not a kilometre.
	if span := hi[0] - lo[0]; span < 40 || span > 2000 {
		t.Errorf("the model spans %v LDU along its shafts, which is not a "+
			"reduction", span)
	}
	if span := hi[2] - lo[2]; span < 40 {
		t.Errorf("the model spans %v LDU across, but its shafts are 40 apart", span)
	}
}

// A model that was never built has no triangles, and saying so beats handing
// back an empty buffer the page would draw as nothing.
func TestNothingBuiltMeansNothingToDraw(t *testing.T) {
	if got := Draw(nil, nil); got != nil {
		t.Errorf("got %d bytes for no model", len(got))
	}
}
