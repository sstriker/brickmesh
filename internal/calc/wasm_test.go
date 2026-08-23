// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

//go:build !js

package calc

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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
		filepath.Join(dir, "brickmesh.wasm"), "github.com/sstriker/brickmesh/cmd/brickmesh-wasm")
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

// The worker is what the page talks to, and nothing else exercises it.
//
// A page that loads and a module that answers do not add up to a working page
// if the message between them is wrong. node has neither a DOM Worker nor
// importScripts, so the harness supplies both and runs web/worker.js as
// written: a build message in, progress and an answer out.
func TestTheWorkerBuildsAModel(t *testing.T) {
	node := findNode(t)
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1: a build needs real parts")
	}

	dir := t.TempDir()
	buildWASM(t, dir)
	for _, name := range []string{"worker.js"} {
		src, err := os.ReadFile(filepath.Join("..", "..", "web", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), src, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	publishInto(t, filepath.Join(dir, "data"))

	spec := filepath.Join("..", "..", "examples", "reduction.json")
	out, err := exec.Command(node,
		filepath.Join("testdata", "worker_harness.mjs"), dir, spec).CombinedOutput()
	if err != nil {
		t.Fatalf("running the worker: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{
		"ok=true",
		// The two things a page needs back, and the progress it shows meanwhile.
		"fetching the parts -> placing the gears and finding a frame",
		"answer id matches the question: true",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the worker did not report %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "parts=0") {
		t.Errorf("the worker built an empty model:\n%s", got)
	}
}

// The camera the page draws with.
//
// A transposed or mis-ordered matrix throws every vertex behind the eye, and
// the page shows a blank canvas and no error — WebGL does not complain about a
// model it cannot see. There is nothing in the browser to catch that, so the
// matrices are checked here: the model centred at every angle the viewer can be
// turned to, nothing behind the eye, rotation that does not scale, and the
// whole model inside the frame at the distance the viewer picks.
func TestTheViewerPutsTheModelInFrontOfTheCamera(t *testing.T) {
	node := findNode(t)
	out, err := exec.Command(node,
		filepath.Join("testdata", "view_harness.mjs"), filepath.Join("..", "..", "web")).CombinedOutput()
	if err != nil {
		t.Fatalf("the camera maths: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}

// The page's example buttons have to point at examples that exist.
//
// Two of the four did not. They fetched, got a 404, and put the message in the
// status line — which nothing here could see, because nothing here had ever
// clicked one. It took a browser to find, and this is the cheap version of that
// browser so it cannot come back.
func TestThePageOnlyOffersExamplesThatExist(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "web", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	paths := regexp.MustCompile(`"(examples/[^"]+\.json)"`).FindAllStringSubmatch(string(src), -1)
	if len(paths) == 0 {
		t.Fatal("the page offers no examples at all, or they are written another way now")
	}
	for _, m := range paths {
		at := filepath.Join("..", "..", m[1])
		if _, err := os.Stat(at); err != nil {
			t.Errorf("the page offers %q and there is no such file: clicking "+
				"that button fetches a 404", m[1])
		}
	}
	t.Logf("%d example(s) offered, all present", len(paths))
}

// Everything the page asks for has to be in what gets published.
//
// The site is assembled by copying a named list of files, and index.html loads
// whatever it loads. Those two drifted: view.js was added to the page and never
// added to the list, so the published site 404'd on it and app.js died at its
// first line — no examples, no buttons, a permanent "Loading the engine…".
//
// The browser test could not see it. It serves the repository's web directory,
// where every file is present by construction; what Pages serves is the copy.
// So this compares the page's own references against the workflow that builds
// the thing a visitor gets.
func TestThePublishedSiteCarriesEverythingThePageAsksFor(t *testing.T) {
	page, err := os.ReadFile(filepath.Join("..", "..", "web", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	flow, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "pages.yml"))
	if err != nil {
		t.Fatal(err)
	}
	refs := regexp.MustCompile(`(?:src|href)="([^"]+)"`).FindAllStringSubmatch(string(page), -1)
	if len(refs) == 0 {
		t.Fatal("index.html references nothing at all, or another way now")
	}
	checked := 0
	for _, m := range refs {
		ref := m[1]
		switch {
		case strings.Contains(ref, "://"), strings.HasPrefix(ref, "#"),
			strings.HasPrefix(ref, "data/"), strings.HasPrefix(ref, "examples/"),
			strings.HasPrefix(ref, "skill/"):
			// Off-site, in-page, or generated into the site by an earlier step.
			continue
		}
		checked++
		if _, err := os.Stat(filepath.Join("..", "..", "web", ref)); err != nil {
			t.Errorf("index.html asks for %q and web/%s does not exist", ref, ref)
			continue
		}
		if !strings.Contains(string(flow), "web/"+ref) {
			t.Errorf("index.html asks for %q and the Pages workflow never copies "+
				"web/%s into the site. It works here and 404s once published",
				ref, ref)
		}
	}
	t.Logf("%d local reference(s) checked", checked)
}

// The page, in a browser.
//
// Everything else about it is checked without one: the module's answers against
// the engine's, the worker's messages, the camera's arithmetic. None of that
// can link a shader. A vertex attribute that does not exist, a varying spelled
// two ways, a stride off by one — all compile in Go, pass every buffer check,
// and give a person a blank canvas.
//
// So this serves the page over http, builds a model, reads the pixels back, and
// clicks a finding to see them change. It needs playwright and a chromium, and
// skips without them; CI has both.
func TestThePageWorksInABrowser(t *testing.T) {
	node := findNode(t)
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1: the page needs real parts to build one")
	}
	if os.Getenv("BRICKMESH_PLAYWRIGHT") == "" {
		t.Skip("set BRICKMESH_PLAYWRIGHT to a playwright-core install to run the browser")
	}

	site := t.TempDir()
	buildWASM(t, site)
	for _, name := range []string{"index.html", "app.js", "view.js", "animate.js",
		"style.css", "worker.js"} {
		src, err := os.ReadFile(filepath.Join("..", "..", "web", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(site, name), src, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(site, "examples"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, spec := range examples(t) {
		src, err := os.ReadFile(spec)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(site, "examples", filepath.Base(spec)), src, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	publishInto(t, filepath.Join(site, "data"))

	out, err := exec.Command(node,
		filepath.Join("testdata", "browser_harness.mjs"), site).CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("the page does not work in a browser: %v", err)
	}
}
