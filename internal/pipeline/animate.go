// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"strings"

	"brickmesh/internal/ldcad"
	"brickmesh/internal/ldr"
	"brickmesh/internal/mech"
	"brickmesh/internal/synth"
)

// animate groups the parts by the shaft they turn with, and writes a script
// that turns each group at the ratio the functional layer solved for.
//
// A gearbox gets one animation per state, because that is what a state is: the
// same parts turning at different speeds. The freewheeling gears keep turning
// in every state — they are always meshed — which is what makes a gearbox worth
// watching rather than merely looking at.
func animate(m *mech.Mechanism, res *Result, opts Options) {
	groupOf := shaftGroups(res)
	if len(groupOf) == 0 {
		return
	}

	// One group per shaft, centered on its own line, which is what it turns
	// about.
	seen := map[string]bool{}
	for _, id := range m.Order() {
		name, ok := groupOf[id]
		if !ok || seen[name] {
			continue
		}
		place, ok := res.Layout.Place[id]
		if !ok {
			continue
		}
		seen[name] = true
		res.Model.Groups = append(res.Model.Groups, ldr.Group{
			Name: name, Center: place.Point.Scale(synth.HalfStud),
		})
	}
	tagParts(res, groupOf)

	script := &ldcad.Script{
		Model: m.Name, Seconds: opts.Seconds, InputTurns: opts.InputTurns,
	}
	states := m.States()
	if len(states) == 0 {
		states = []string{""}
	}
	for _, state := range states {
		if ani, ok := animationFor(m, res, groupOf, state); ok {
			script.Animations = append(script.Animations, ani)
		}
	}
	if len(script.Animations) == 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "WARN", Check: "animation",
			Detail: "no state resolves to definite speeds, so there is nothing to animate",
		})
		return
	}
	res.Script = script
	res.Model.Script = opts.ScriptName
	res.Findings = append(res.Findings, mech.Finding{
		Level: "OK", Check: "animation", Detail: animationSummary(script),
	})
}

func animationSummary(s *ldcad.Script) string {
	names := make([]string, 0, len(s.Animations))
	for _, a := range s.Animations {
		names = append(names, a.Name)
	}
	return "animating " + strings.Join(names, ", ") +
		"; every group turns at the ratio the mechanism solved for"
}

// shaftGroups names a group per shaft that carries something.
func shaftGroups(res *Result) map[string]string {
	out := map[string]string{}
	for _, st := range res.Stations {
		out[st.Shaft] = "shaft_" + st.Shaft
	}
	for _, a := range res.axles {
		out[a.shaft] = "shaft_" + a.shaft
	}
	return out
}

// tagParts puts each gear and axle in the group of the shaft it turns with.
func tagParts(res *Result, groupOf map[string]string) {
	for i := range res.Model.Parts {
		p := &res.Model.Parts[i]
		shaft, ok := shaftFromLabel(p.Label)
		if !ok {
			continue
		}
		if name, ok := groupOf[shaft]; ok {
			p.Group = name
		}
	}
}

// shaftFromLabel reads the shaft out of a label like "24t on shaft 'first'" or
// "axle 10 for shaft 'input'". The labels are written when the parts are
// placed, so this saves carrying a second index alongside the model.
func shaftFromLabel(label string) (string, bool) {
	i := strings.Index(label, "'")
	if i < 0 {
		return "", false
	}
	rest := label[i+1:]
	j := strings.Index(rest, "'")
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

// animationFor works out how fast each group turns in one state.
func animationFor(m *mech.Mechanism, res *Result, groupOf map[string]string,
	state string) (ldcad.Animation, bool) {

	speeds, ok := m.Solve(state)
	if !ok {
		return ldcad.Animation{}, false
	}
	name := state
	if name == "" {
		name = m.Name
	}
	ani := ldcad.Animation{Name: name}
	for _, id := range m.Order() {
		group, ok := groupOf[id]
		if !ok {
			continue
		}
		place, ok := res.Layout.Place[id]
		if !ok {
			continue
		}
		ani.Turning = append(ani.Turning, ldcad.Turning{
			Group: group, Axis: place.Direction, Speed: speeds[id],
		})
	}
	if len(ani.Turning) == 0 {
		return ldcad.Animation{}, false
	}
	return ani, true
}
