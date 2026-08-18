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
	"syscall/js"

	"brickmesh/internal/calc"
)

func main() {
	js.Global().Set("brickmeshCheck", js.FuncOf(check))
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
