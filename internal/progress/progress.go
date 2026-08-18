// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package progress is how a long search says where it has got to.
//
// It exists because of where this is going. In a browser the structural search
// runs in a Web Worker and the user is watching: a run that reports nothing and
// cannot be stopped is a run that reads as a hung tab. Retrofitting that into
// an engine built on run-to-completion calls means changing every signature on
// the path, so the shape is settled here while the path is short.
//
// Deliberately small. A stage name and a count is enough to drive a progress
// bar, and per-restart granularity costs nothing — a restart takes long enough
// that reporting one is free, and short enough that the bar keeps moving.
package progress

import "fmt"

// Stages, named once so a caller can switch on them rather than match strings.
const (
	StageLayout    = "layout"
	StageStructure = "structure"
	StageModel     = "model"
	StagePhase     = "tooth phase"
	StageClearance = "clearance"
	StageAnimation = "animation"
)

// Report is one step of a long-running search.
type Report struct {
	Stage string
	// Done and Total count whatever the stage counts — restarts, parts, pairs.
	// Total is zero when the stage does not know how much work it has.
	Done, Total int
	// Note is for anything worth saying that a count cannot say.
	Note string
}

func (r Report) String() string {
	if r.Total > 0 {
		return fmt.Sprintf("%s: %d/%d %s", r.Stage, r.Done, r.Total, r.Note)
	}
	if r.Note != "" {
		return fmt.Sprintf("%s: %s", r.Stage, r.Note)
	}
	return r.Stage
}

// Func receives reports. The zero value is a valid do-nothing sink, which is
// what keeps every call site from having to check for one.
type Func func(Report)

// Report sends one, or does nothing if there is nowhere to send it.
//
// A method on the func type rather than a free function so that callers write
// opts.Progress.Report(...) and never test for nil.
func (f Func) Report(r Report) {
	if f != nil {
		f(r)
	}
}

// Step is the common case: this many of that many, in a stage.
func (f Func) Step(stage string, done, total int) {
	f.Report(Report{Stage: stage, Done: done, Total: total})
}
