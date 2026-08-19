// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package calc

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"brickmesh/internal/assets"
	"brickmesh/internal/extract"
	"brickmesh/internal/ldraw"
	"brickmesh/internal/part"
	"brickmesh/internal/pipeline"
	"brickmesh/internal/shadow"
	"brickmesh/internal/spec"
	"brickmesh/internal/voxel"
)

// The claim the browser build rests on: a mechanism placed from the published
// files is the same mechanism placed from the libraries.
//
// The mechanism, exactly — every gear, ring, joiner and axle in the same place.
// That is what the files have to reproduce, and anything else means the
// generation lost something.
//
// The frame is not held to that, and should not be. It is the result of a
// search whose ties are broken on scores computed from vertex positions, and
// the published meshes store float32 where the parser hands out float64. A
// difference in the last bits can flip a tie and give a mirror-image cover that
// is just as good. What matters about the frame is that it passes its own
// checks, which is asserted instead.
func TestTheSameModelComesOutOfThePublishedFiles(t *testing.T) {
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	lib := ldraw.New("")
	root, err := shadow.Ensure("")
	if err != nil {
		t.Fatal(err)
	}
	ports := extract.Ports{Lib: shadow.Open(root)}

	for _, path := range examples(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			doc, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			fromLibrary := buildWith(t, pipeline.Deps{
				Lib: lib, Shadow: ports, Rast: voxel.NewRasterizer(lib),
			}, doc)

			// The same parts, published and read back.
			parts := publish(t, lib, ports, fromLibrary)
			fromFiles := parts.Build(context.Background(), doc, true)

			if fromFiles.Error != "" {
				t.Fatalf("building from the published files: %s", fromFiles.Error)
			}
			if fromFiles.LDR == "" {
				for _, f := range fromFiles.Findings {
					t.Logf("  %s [%s] %s", f.Level, f.Check, f.Detail)
				}
				t.Fatalf("no model came out; ok=%v parts=%d",
					fromFiles.OK, fromFiles.Parts)
			}
			// Compared as numbers, not as text. The published meshes store
			// float32 where the parser hands out float64, so anything worked
			// out from a vertex — the tooth phase, which is an angle read off
			// the teeth — comes out differing in the last bits. That is the
			// format's precision showing through and not a difference in the
			// model: a rotation out by 1e-16 is the same rotation.
			same, why := sameMechanism(fromFiles.LDR, fromLibrary)
			if !same {
				t.Error(why)
			}
			for _, f := range fromFiles.Findings {
				if f.Level == "FAIL" {
					t.Errorf("the model built from the files does not check out: "+
						"[%s] %s", f.Check, f.Detail)
				}
			}
		})
	}
}

// buildWith places a mechanism and returns the model as text.
// examples is every mechanism in the repository, so a new one is covered
// without anybody remembering to add it here.
func examples(t *testing.T) []string {
	t.Helper()
	found, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.json"))
	if err != nil || len(found) == 0 {
		t.Fatalf("no examples found: %v", err)
	}
	return found
}

func buildWith(t *testing.T, deps pipeline.Deps, doc []byte) string {
	t.Helper()
	s, err := spec.Read(strings.NewReader(string(doc)))
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Build()
	if err != nil {
		t.Fatal(err)
	}
	res, err := pipeline.Run(context.Background(), m, deps, pipeline.Options{
		Restarts: 24, Seed: 1, Animate: true, ScriptName: "model.lua",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Model == nil {
		t.Fatal("no model")
	}
	return res.Model.Encode()
}

// publishInto writes the two published files into a directory, the way the
// workflow does, so a worker can fetch them.
func publishInto(t *testing.T, dir string) {
	t.Helper()
	lib := ldraw.New("")
	root, err := shadow.Ensure("")
	if err != nil {
		t.Fatal(err)
	}
	rawCatalog, rawMeshes := publishedBytes(t, lib, extract.Ports{Lib: shadow.Open(root)}, "")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{
		"catalog.bin": rawCatalog, "meshes.bin": rawMeshes,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// publish writes the two files for every part the model needs, and a few more,
// then reads them back the way a browser would.
func publish(t *testing.T, lib *ldraw.Library, ports extract.Ports, model string) *Parts {
	t.Helper()
	rawCatalog, rawMeshes := publishedBytes(t, lib, ports, model)
	parts, err := Load(rawCatalog, rawMeshes)
	if err != nil {
		t.Fatal(err)
	}
	return parts
}

// publishedBytes builds the two files.
func publishedBytes(t *testing.T, lib *ldraw.Library, ports extract.Ports,
	model string) ([]byte, []byte) {

	t.Helper()
	catalog := assets.Catalog{}
	var meshes []assets.Mesh

	// Everything the engine can place, not only what this model used: the
	// browser gets one catalogue for every mechanism.
	ids := map[string]bool{}
	for _, p := range pipeline.GearParts {
		ids[p] = true
	}
	for _, p := range pipeline.AxleParts {
		ids[p] = true
	}
	for _, b := range beamsOf() {
		ids[b] = true
	}
	for _, extra := range []string{
		"6539.dat", "6538a.dat", "6542a.dat",
		"18947.dat", "18948.dat", "18946.dat", "81346.dat",
	} {
		ids[extra] = true
	}
	for _, line := range strings.Split(model, "\n") {
		if f := strings.Fields(line); len(f) >= 15 && f[0] == "1" {
			ids[f[14]] = true
		}
	}

	var names []string
	for id := range ids {
		names = append(names, id)
	}
	sortStrings(names)
	for _, id := range names {
		g, err := lib.Geometry(id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		p := assets.Part{ID: id, Title: g.Title, Tier: 1}
		for _, h := range ports.Holes(id) {
			kind := uint8(0)
			if h.Cross {
				kind |= assets.PortCross
			}
			p.Ports = append(p.Ports, assets.Port{
				X: float32(h.Pos.X), Y: float32(h.Pos.Y), Z: float32(h.Pos.Z),
				AX: float32(h.Axis.X), AY: float32(h.Axis.Y), AZ: float32(h.Axis.Z),
				Kind: kind,
			})
		}
		catalog.Parts = append(catalog.Parts, p)
	}

	sorted := assets.Sorted(catalog)
	for _, p := range sorted.Parts {
		g, err := lib.Geometry(p.ID)
		if err != nil {
			t.Fatal(err)
		}
		meshes = append(meshes, assets.IndexTriangles(g.Tris))
	}
	rawCatalog, err := assets.WriteCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	rawMeshes, err := assets.WriteMeshes(meshes)
	if err != nil {
		t.Fatal(err)
	}
	return rawCatalog, rawMeshes
}

func beamsOf() []string {
	var out []string
	for _, b := range part.Beams {
		out = append(out, b.Part)
	}
	return out
}

func sortStrings(v []string) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

// sameMechanism compares the turning parts of two models, to a tolerance that
// admits the float32 the mesh blob stores and nothing looser.
func sameMechanism(a, b string) (bool, string) {
	pa, pb := mechanismOf(placements(a)), mechanismOf(placements(b))
	if len(pa) != len(pb) {
		return false, fmt.Sprintf("%d parts against %d", len(pa), len(pb))
	}
	for i := range pa {
		if pa[i].name != pb[i].name {
			return false, fmt.Sprintf("part %d is %s in one and %s in the other",
				i+1, pa[i].name, pb[i].name)
		}
		for k := range pa[i].nums {
			// A thousandth of an LDU is far below anything that can be built,
			// and far above the 1e-7 a float32 costs.
			if math.Abs(pa[i].nums[k]-pb[i].nums[k]) > 1e-3 {
				return false, fmt.Sprintf("%s: number %d is %v in one and %v in "+
					"the other", pa[i].name, k+1, pa[i].nums[k], pb[i].nums[k])
			}
		}
	}
	return true, ""
}

// mechanismOf keeps the parts that make the mechanism and drops the frame.
//
// The frame is beams, and which beams a search picks is not part of what the
// files have to reproduce. Everything else — the gears, the rings, the joiners
// they slide on, the axles through them — is.
func mechanismOf(all []placement) []placement {
	frame := map[string]bool{}
	for _, b := range part.Beams {
		frame[b.Part] = true
	}
	var out []placement
	for _, p := range all {
		if !frame[p.name] {
			out = append(out, p)
		}
	}
	return out
}

type placement struct {
	name string
	nums []float64
}

// placements reads the type-1 lines: a colour, a position, a matrix, a part.
func placements(model string) []placement {
	var out []placement
	for _, line := range strings.Split(model, "\n") {
		f := strings.Fields(line)
		if len(f) < 15 || f[0] != "1" {
			continue
		}
		p := placement{name: f[14]}
		for _, v := range f[2:14] {
			n, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil
			}
			p.nums = append(p.nums, n)
		}
		out = append(out, p)
	}
	return out
}
