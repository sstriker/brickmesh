// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package calc

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

const reduction = `{
  "name": "reduction",
  "shafts": [{"id": "input", "bearings": 2}, {"id": "output", "bearings": 2}],
  "meshes": [{"a": "input", "b": "output", "teeth_a": 8, "teeth_b": 24}],
  "inputs": [{"shaft": "input", "speed": 1.0}],
  "outputs": ["output"]
}`

// The whole point of shipping this first: it answers the question without a
// parts library, a search, or a download.
func TestAReductionAnswersWithItsRatio(t *testing.T) {
	got := Check([]byte(reduction))
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	if !got.OK {
		t.Errorf("a plain 8:24 reduction should check out: %+v", got.Findings)
	}
	if len(got.States) != 1 {
		t.Fatalf("got %d states, want the single unnamed one", len(got.States))
	}
	st := got.States[0]
	if st.Name != "" {
		t.Errorf("the single state should be unnamed, got %q", st.Name)
	}
	if !st.Determined {
		t.Fatal("the state should resolve")
	}
	// Eight driving twenty-four: a third, and the other way round.
	if want := -1.0 / 3; math.Abs(st.Speeds["output"]-want) > 1e-9 {
		t.Errorf("the output turns %v, want %v", st.Speeds["output"], want)
	}
}

// A gearbox answers once per state, which is what makes the calculator worth
// more than a division.
func TestAGearboxAnswersOncePerState(t *testing.T) {
	got := Check([]byte(gearbox))
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	if len(got.States) != 3 {
		t.Fatalf("got %d states, want one per gear", len(got.States))
	}
	want := []float64{-1.0 / 3, -0.6, -1}
	for i, st := range got.States {
		if !st.Determined {
			t.Errorf("%s does not resolve", st.Name)
			continue
		}
		if math.Abs(st.Speeds["output"]-want[i]) > 1e-9 {
			t.Errorf("%s turns the output %v, want %v", st.Name,
				st.Speeds["output"], want[i])
		}
	}
}

// Someone halfway through typing gets told what is wrong, not a failure. The
// caller is a text box.
func TestADescriptionThatWillNotParseIsAnAnswer(t *testing.T) {
	for _, c := range []struct{ why, doc string }{
		{"not JSON at all", `{`},
		{"a key that is not a key", `{"shafts": [], "wheels": []}`},
		{"no shafts", `{"name": "empty"}`},
		{"a mesh naming a shaft that is not there", `{
			"shafts": [{"id": "a", "bearings": 2}],
			"meshes": [{"a": "a", "b": "ghost", "teeth_a": 8, "teeth_b": 24}]}`},
	} {
		got := Check([]byte(c.doc))
		if got.Error == "" {
			t.Errorf("%s: should have been reported, got %+v", c.why, got)
		}
		if got.OK {
			t.Errorf("%s: should not read as OK", c.why)
		}
	}
}

// A train that cannot turn is the question this is for.
func TestALockedTrainSaysSo(t *testing.T) {
	// Three gears in a ring, which cannot all turn at once.
	locked := `{
	  "name": "locked",
	  "shafts": [{"id": "a", "bearings": 2}, {"id": "b", "bearings": 2},
	             {"id": "c", "bearings": 2}],
	  "meshes": [
	    {"a": "a", "b": "b", "teeth_a": 8, "teeth_b": 24},
	    {"a": "b", "b": "c", "teeth_a": 8, "teeth_b": 24},
	    {"a": "c", "b": "a", "teeth_a": 8, "teeth_b": 24}],
	  "inputs": [{"shaft": "a", "speed": 1.0}],
	  "outputs": ["c"]
	}`
	got := Check([]byte(locked))
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	if got.OK {
		t.Error("a gear loop that cannot close should not check out")
	}
	var said bool
	for _, f := range got.Findings {
		if f.Level == "FAIL" {
			said = true
		}
	}
	if !said {
		t.Errorf("something should have failed: %+v", got.Findings)
	}
}

// The JSON is the interface across the WebAssembly boundary, so its shape is
// part of the contract rather than an implementation detail.
func TestTheJSONCarriesWhatAPageNeeds(t *testing.T) {
	raw := CheckJSON([]byte(gearbox))
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	for _, key := range []string{"name", "ok", "findings", "states", "outputs"} {
		if _, ok := back[key]; !ok {
			t.Errorf("no %q in the answer: %s", key, raw)
		}
	}
	// An error field on a good answer would have a page showing an error.
	if _, ok := back["error"]; ok {
		t.Errorf("a good answer should carry no error field: %s", raw)
	}
	if !strings.Contains(string(raw), `"determined":true`) {
		t.Error("the states should say whether they resolved")
	}
}

const gearbox = `{
  "name": "3-speed",
  "states": ["1st", "2nd", "3rd"],
  "shafts": [
    {"id": "input", "bearings": 2}, {"id": "output", "bearings": 2},
    {"id": "first", "bearings": 2}, {"id": "second", "bearings": 2},
    {"id": "third", "bearings": 2}],
  "meshes": [
    {"a": "input", "b": "first", "teeth_a": 8, "teeth_b": 24},
    {"a": "input", "b": "second", "teeth_a": 12, "teeth_b": 20},
    {"a": "input", "b": "third", "teeth_a": 16, "teeth_b": 16}],
  "couplings": [
    {"a": "output", "b": "first", "states": ["1st"]},
    {"a": "output", "b": "second", "states": ["2nd"]},
    {"a": "output", "b": "third", "states": ["3rd"]}],
  "inputs": [{"shaft": "input", "speed": 1.0}],
  "outputs": ["output"]
}`
