// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

//go:build !js

package calc

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The browser build is a compiled artifact that no other test touches, and a
// page that loads but answers wrongly looks exactly like a page that works. So
// the module is built, run under a real JavaScript runtime with the same glue
// the page uses, and its answer compared against the one this package gives
// directly. Same approach as the extractor against the Python, and the Lua
// against LDCad's API.
func TestTheBrowserBuildAnswersTheSame(t *testing.T) {
	node := findNode(t)
	dir := t.TempDir()
	buildWASM(t, dir)

	for _, spec := range specsUnderTest(t) {
		t.Run(filepath.Base(spec), func(t *testing.T) {
			description, err := os.ReadFile(spec)
			if err != nil {
				t.Fatal(err)
			}

			out, err := exec.Command(node,
				filepath.Join("testdata", "harness.mjs"), dir, spec).Output()
			if err != nil {
				if ee, ok := err.(*exec.ExitError); ok {
					t.Fatalf("running the module: %v\n%s", err, ee.Stderr)
				}
				t.Fatal(err)
			}

			// Compared as decoded values rather than as bytes: the claim is
			// that the answers agree, not that two encoders order keys alike.
			var inBrowser, inGo any
			if err := json.Unmarshal(out, &inBrowser); err != nil {
				t.Fatalf("the module did not answer with JSON: %v\n%s", err, out)
			}
			if err := json.Unmarshal(CheckJSON(description), &inGo); err != nil {
				t.Fatal(err)
			}
			gotBrowser, _ := json.Marshal(inBrowser)
			gotGo, _ := json.Marshal(inGo)
			if string(gotBrowser) != string(gotGo) {
				t.Errorf("the browser and Go disagree.\nbrowser: %s\ngo:      %s",
					gotBrowser, gotGo)
			}
		})
	}
}

// A description that will not read has to come back as an answer rather than as
// a crash, because in a browser a crash takes the exported function with it and
// the page dies on the first typo.
func TestTheBrowserBuildSurvivesRubbish(t *testing.T) {
	node := findNode(t)
	dir := t.TempDir()
	buildWASM(t, dir)

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{ this is not a mechanism"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node,
		filepath.Join("testdata", "harness.mjs"), dir, bad).Output()
	if err != nil {
		t.Fatalf("the module fell over on a bad description: %v", err)
	}
	var got Result
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if got.Error == "" {
		t.Errorf("should have said what was wrong with it: %s", out)
	}
}

func findNode(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("BRICKMESH_NODE"); p != "" {
		return p
	}
	p, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node on the path; set BRICKMESH_NODE to run the browser build")
	}
	return p
}

// buildWASM compiles the module and puts the Go distribution's glue beside it,
// which is the pair the page loads.
func buildWASM(t *testing.T, dir string) {
	t.Helper()
	build := exec.Command("go", "build", "-o",
		filepath.Join(dir, "brickmesh.wasm"), "brickmesh/cmd/brickmesh-wasm")
	build.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building for the browser: %v\n%s", err, out)
	}

	root, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatal(err)
	}
	goroot := string(root[:len(root)-1])
	for _, candidate := range []string{
		filepath.Join(goroot, "lib", "wasm", "wasm_exec.js"),
		filepath.Join(goroot, "misc", "wasm", "wasm_exec.js"),
	} {
		glue, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, "wasm_exec.js"), glue, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatalf("no wasm_exec.js under %s", goroot)
}

// specsUnderTest is every example in the repository, so a new one is covered
// without anybody remembering to add it here.
func specsUnderTest(t *testing.T) []string {
	t.Helper()
	found, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.json"))
	if err != nil || len(found) == 0 {
		t.Fatalf("no examples found: %v", err)
	}
	return found
}
