// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package calc

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/sstriker/brickmesh/internal/extract"
	"github.com/sstriker/brickmesh/internal/ldraw"
	"github.com/sstriker/brickmesh/internal/shadow"
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

// A finding that knows which parts it is about has to reach the buffer, or the
// page can describe the problem and not point at it — which is the whole reason
// there is a model on the page at all.
func TestFlaggedPartsAreMarkedInTheBuffer(t *testing.T) {
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
	if built := parts.Build(context.Background(), doc, false); built.Error != "" {
		t.Fatal(built.Error)
	}

	plain := markedVertices(t, parts.Draw())
	if plain != 0 {
		t.Errorf("%d vertices are marked before anything is flagged", plain)
	}

	// Marking directly, since a passing model has no findings that point at
	// anything: the first part alone, and it must be the first part alone.
	all := DrawFlagging(parts.drawn, parts.shapes, map[int]bool{0: true})
	marked := markedVertices(t, all)
	if marked == 0 {
		t.Error("flagging the first part marked nothing")
	}
	total := len(all) / (drawStride * 4)
	if marked == total {
		t.Errorf("flagging one part of %d marked every one of the %d vertices",
			len(parts.drawn.Parts), total)
	}
	// And it is that part's own triangles, which is the count its geometry has.
	g, err := parts.shapes.Geometry(parts.drawn.Parts[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	if want := len(g.Tris) * 3; marked != want {
		t.Errorf("%d vertices marked, and %s has %d", marked,
			parts.drawn.Parts[0].Name, want)
	}

	// A check nobody reported marks nothing, and must not mark everything.
	parts.Flag("no such check")
	if got := markedVertices(t, parts.Draw()); got != 0 {
		t.Errorf("flagging a check that was never reported marked %d vertices", got)
	}
	parts.Flag("")
	if got := markedVertices(t, parts.Draw()); got != 0 {
		t.Errorf("clearing the flag left %d vertices marked", got)
	}
}

func markedVertices(t *testing.T, raw []byte) int {
	t.Helper()
	const stride = drawStride * 4
	n := 0
	for v := 0; v*stride < len(raw); v++ {
		at := v*stride + 9*4 // the tenth float
		if at+4 > len(raw) {
			break
		}
		if math.Float32frombits(binary.LittleEndian.Uint32(raw[at:])) != 0 {
			n++
		}
	}
	return n
}
