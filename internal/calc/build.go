// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package calc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"brickmesh/internal/assets"
	"brickmesh/internal/pipeline"
	"brickmesh/internal/spec"
	"brickmesh/internal/voxel"
)

// Parts is a catalogue and a mesh blob, ready to build with.
//
// Held between calls because a page solves the same mechanism many times as it
// is edited, and the parts do not change in between.
type Parts struct {
	shapes *assets.Shapes
	rast   *voxel.Rasterizer
}

// Load reads the two published files.
//
// Whole, both of them. The mesh blob is laid out to be fetched a part at a time
// by byte range, and that is the right thing over a network — but a caller that
// already has the bytes should not be made to pretend otherwise, and tier 1 is
// five megabytes.
func Load(catalog, meshes []byte) (*Parts, error) {
	cat, err := assets.ReadCatalog(catalog)
	if err != nil {
		return nil, err
	}
	shapes, err := assets.FromBytes(cat, meshes)
	if err != nil {
		return nil, err
	}
	return &Parts{shapes: shapes, rast: voxel.NewRasterizer(shapes)}, nil
}

// Built is a mechanism placed: the findings, the model, and the animation.
type Built struct {
	Result
	// LDR is the model, ready to be written to a .ldr file.
	LDR string `json:"ldr,omitempty"`
	// Lua is the LDCad animation, when one was asked for.
	Lua string `json:"lua,omitempty"`
	// Parts counts what went into it.
	Parts int `json:"parts"`
}

// Build places a mechanism and returns the files for it.
//
// The checks come back whatever happens, because a mechanism that fails them is
// exactly the one worth explaining. A model is only returned when it passed.
func (p *Parts) Build(ctx context.Context, description []byte, animate bool) Built {
	out := Built{Result: Check(description)}
	if out.Error != "" || !out.OK {
		return out
	}

	s, err := spec.Read(bytes.NewReader(description))
	if err != nil {
		out.Error = err.Error()
		return out
	}
	m, err := s.Build()
	if err != nil {
		out.Error = err.Error()
		return out
	}

	res, err := pipeline.Run(ctx, m, pipeline.Deps{
		Lib: p.shapes, Shadow: p.shapes, Rast: p.rast,
	}, pipeline.Options{
		Restarts: 24, Seed: 1,
		Animate: animate, ScriptName: "model.lua",
	})
	if err != nil {
		out.Error = err.Error()
		return out
	}

	// The findings from placing it, on top of the ones from checking it.
	out.Findings = out.Findings[:0]
	out.OK = true
	for _, f := range res.Findings {
		if f.Level == "FAIL" {
			out.OK = false
		}
		out.Findings = append(out.Findings, Finding(f))
	}
	if res.Model != nil {
		out.LDR = res.Model.Encode()
		out.Parts = len(res.Model.Parts)
	}
	if res.Script != nil {
		out.Lua = res.Script.Render()
	}
	return out
}

// BuildJSON is Build with the answer encoded, which is the shape a WebAssembly
// export wants.
func (p *Parts) BuildJSON(ctx context.Context, description []byte, animate bool) []byte {
	raw, err := json.Marshal(p.Build(ctx, description, animate))
	if err != nil {
		return []byte(fmt.Sprintf(`{"error":%q}`, err.Error()))
	}
	return raw
}
