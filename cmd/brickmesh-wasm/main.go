// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

//go:build js && wasm

// Command brickmesh-wasm exposes the functional layer to a page.
//
// Deliberately thin. Everything worth testing is in internal/calc, which builds
// and runs anywhere; this file is the boundary and nothing else, because code
// behind a js/wasm build tag cannot be run by `go test` on a developer's
// machine or in CI.
//
// Build:
//
//	GOOS=js GOARCH=wasm go build -o web/brickmesh.wasm ./cmd/brickmesh-wasm
package main

import (
	"context"
	"fmt"
	"syscall/js"

	"github.com/sstriker/brickmesh/internal/calc"
	"github.com/sstriker/brickmesh/internal/geom"
)

func main() {
	js.Global().Set("brickmeshCheck", js.FuncOf(check))
	js.Global().Set("brickmeshLoadParts", js.FuncOf(loadParts))
	js.Global().Set("brickmeshBuild", js.FuncOf(build))
	js.Global().Set("brickmeshDraw", js.FuncOf(draw))
	js.Global().Set("brickmeshReady", js.ValueOf(true))
	// A WebAssembly module whose main returns is torn down, taking the exported
	// functions with it. Blocking forever is how a Go module stays callable.
	select {}
}

// check is the one exported function: a description in, an answer out, both as
// strings.
//
// Strings rather than objects on purpose. Building a JS object from Go means
// reflection, which is what TinyGo does not have — keeping the boundary to
// strings leaves that door open. See docs/architecture.md.
func check(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return `{"error":"brickmeshCheck needs a mechanism description"}`
	}
	return string(calc.CheckJSON([]byte(args[0].String())))
}

// parts is the catalogue and mesh blob, once loaded. Held because a page solves
// the same mechanism many times as it is edited and the parts do not change.
var parts *calc.Parts

// loadParts takes the two published files as byte arrays.
//
// Arrays rather than URLs: fetching is the page's job, and it already has to do
// it for everything else. Keeping the network out of here leaves this callable
// from a test with the bytes in hand.
func loadParts(_ js.Value, args []js.Value) any {
	if len(args) < 2 {
		return `{"error":"brickmeshLoadParts needs the catalogue and the meshes"}`
	}
	catalog := make([]byte, args[0].Length())
	js.CopyBytesToGo(catalog, args[0])
	meshes := make([]byte, args[1].Length())
	js.CopyBytesToGo(meshes, args[1])

	got, err := calc.Load(catalog, meshes)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	parts = got
	return `{"ok":true}`
}

// build places a mechanism and returns the model and the animation with it.
func build(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return `{"error":"brickmeshBuild needs a mechanism description"}`
	}
	if parts == nil {
		return `{"error":"the parts have not been loaded yet"}`
	}
	animate := len(args) > 1 && args[1].Truthy()
	// Three more, optional: how big the frame may be in studs, per axis, with
	// zero meaning no bound on that axis. A bound is not a preference — a frame
	// outside it is not a candidate — so the page can ask for something that
	// cannot be built and be told so.
	var envelope geom.Vec3
	for i, at := range []*float64{&envelope.X, &envelope.Y, &envelope.Z} {
		if len(args) > i+2 && args[i+2].Type() == js.TypeNumber {
			*at = args[i+2].Float()
		}
	}
	parts.SetEnvelope(envelope)
	return string(parts.BuildJSON(context.Background(), []byte(args[0].String()), animate))
}

// draw hands back the last built model as triangles, optionally with the parts
// of one check picked out: brickmeshDraw("clearance") lights up whatever the
// clearance findings are about, brickmeshDraw() lights up nothing.
//
// Bytes rather than JSON, and a separate call rather than a field on the
// answer: it is six megabytes of float for a compound gearbox, which is
// reasonable as a buffer to upload and absurd as a number in a string.
func draw(_ js.Value, args []js.Value) any {
	if parts == nil {
		return js.Null()
	}
	check := ""
	if len(args) > 0 && args[0].Type() == js.TypeString {
		check = args[0].String()
	}
	parts.Flag(check)
	raw := parts.Draw()
	if len(raw) == 0 {
		return js.Null()
	}
	out := js.Global().Get("Uint8Array").New(len(raw))
	js.CopyBytesToJS(out, raw)
	return out
}
