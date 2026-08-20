// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"math"
	"sort"

	"brickmesh/internal/mech"
	"brickmesh/internal/part"
	"brickmesh/internal/rigidity"
	"brickmesh/internal/synth"
)

// checkLoadPaths asks where the force between two meshed gears goes.
//
// Meshing gears push their shafts apart. The force is along the line of
// centres, it is proportional to the torque, and it is the load that decides
// whether a gearbox holds its mesh or spreads and skips. Nothing here had ever
// asked about it: the structure was checked for holding together and for not
// folding, both of which a pair of brackets can do while letting two shafts
// drift apart under load.
//
// What is checked is the path, not the magnitude. Whether a frame survives a
// given torque needs numbers this engine does not have — the failure limits in
// internal/torque are unverified estimates and say so. Whether the force has
// somewhere to go is geometry, and geometry is what is here.
//
// The measure is how many joints the force crosses. Both shafts borne by one
// part is the whole point of a wall: the load is taken in the part, between two
// holes, and no pin sees it. Every joint after that is a pin in shear and a
// little more give.
func checkLoadPaths(res *Result, deps Deps, m *mech.Mechanism) {
	if res.Structure == nil || deps.Shadow == nil || len(res.Structure.Parts) == 0 {
		return
	}
	frame := make([]part.Placed, 0, len(res.Structure.Parts))
	for _, p := range res.Structure.Parts {
		frame = append(frame, part.Placed(p))
	}
	joints, err := rigidity.FindJointsWith(deps.Shadow, frame, nil, res.Axles)
	if err != nil {
		return
	}
	adj := make([][]int, len(frame))
	for _, j := range joints {
		adj[j.A] = append(adj[j.A], j.B)
		adj[j.B] = append(adj[j.B], j.A)
	}
	bears := bearingParts(res, deps, frame)
	inModel := frameIndexInModel(res, frame)

	worst, unheld := -1, []string{}
	var loose []int
	// The frame parts actually taking the load, so the report can point at
	// them. Worth doing for a passing model too: "which parts hold this?" is a
	// question, not only "what is broken?".
	holding := map[int]bool{}
	direct := 0
	pairs := 0
	for _, link := range m.Links {
		mesh, ok := link.(mech.Mesh)
		if !ok {
			continue
		}
		a, b := bears[mesh.A], bears[mesh.B]
		if len(a) == 0 || len(b) == 0 {
			unheld = append(unheld, fmt.Sprintf("%s-%s", mesh.A, mesh.B))
			loose = append(loose, gearsOnShafts(res, mesh.A, mesh.B)...)
			continue
		}
		pairs++
		hops := shortestHops(adj, a, b)
		if hops < 0 {
			unheld = append(unheld, fmt.Sprintf("%s-%s", mesh.A, mesh.B))
			loose = append(loose, gearsOnShafts(res, mesh.A, mesh.B)...)
			continue
		}
		if hops == 0 {
			direct++
		}
		for _, i := range append(append([]int{}, a...), b...) {
			if at, ok := inModel[i]; ok {
				holding[at] = true
			}
		}
		if hops > worst {
			worst = hops
		}
	}

	if len(unheld) > 0 {
		sort.Strings(unheld)
		res.Findings = append(res.Findings, mech.Finding{
			Level: "FAIL", Check: "load path", Parts: loose, Detail: fmt.Sprintf(
				"nothing in the frame ties the shafts of %v together, so the "+
					"force between their teeth has nowhere to go: under load "+
					"they spread and the gears skip", unheld)})
		return
	}
	if pairs == 0 {
		return
	}
	held := make([]int, 0, len(holding))
	for i := range holding {
		held = append(held, i)
	}
	sort.Ints(held)

	switch {
	case worst == 0:
		res.Findings = append(res.Findings, mech.Finding{
			Level: "OK", Check: "load path", Parts: held, Detail: fmt.Sprintf(
				"all %d gear pair(s) are borne by one part, so the force pushing "+
					"each pair apart is taken inside a beam and no pin carries it",
				pairs)})
	default:
		res.Findings = append(res.Findings, mech.Finding{
			Level: "OK", Check: "load path", Parts: held, Detail: fmt.Sprintf(
				"%d of %d gear pair(s) are borne by one part; the worst crosses "+
					"%d joint(s), so that much of the separating force is carried "+
					"by pins in shear rather than by a beam",
				direct, pairs, worst)})
	}
}

// bearingParts indexes which frame parts hold each shaft.
func bearingParts(res *Result, deps Deps, frame []part.Placed) map[string][]int {
	out := map[string][]int{}
	for id, place := range res.Layout.Place {
		dir := place.Direction.Unit()
		origin := place.Point.Scale(synth.HalfStud)
		for i, p := range frame {
			ports, err := part.WorldPorts(deps.Shadow, p)
			if err != nil {
				continue
			}
			for _, h := range ports {
				if h.Cross {
					continue // a shaft seizes in one; it is not a bearing
				}
				// On the shaft's line, and facing along it.
				d := h.Pos.Sub(origin)
				if d.Sub(dir.Scale(d.Dot(dir))).Len() > 1e-6 {
					continue
				}
				if math.Abs(math.Abs(h.Axis.Unit().Dot(dir))-1) > 1e-6 {
					continue
				}
				out[id] = append(out[id], i)
				break
			}
		}
	}
	return out
}

// shortestHops is the fewest joints between any part bearing one shaft and any
// part bearing the other. Zero means one part bears both.
func shortestHops(adj [][]int, from, to []int) int {
	target := make(map[int]bool, len(to))
	for _, i := range to {
		target[i] = true
	}
	dist := make(map[int]int, len(adj))
	queue := make([]int, 0, len(adj))
	for _, i := range from {
		if target[i] {
			return 0
		}
		dist[i] = 0
		queue = append(queue, i)
	}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		for _, next := range adj[at] {
			if _, seen := dist[next]; seen {
				continue
			}
			dist[next] = dist[at] + 1
			if target[next] {
				return dist[next]
			}
			queue = append(queue, next)
		}
	}
	return -1
}

// gearsOnShafts indexes the placed gears riding either of two shafts, so a
// finding about a pair can point at them rather than name them.
func gearsOnShafts(res *Result, a, b string) []int {
	var out []int
	for i, p := range res.Model.Parts {
		shaft, ok := shaftFromLabel(p.Label)
		if ok && (shaft == a || shaft == b) {
			out = append(out, i)
		}
	}
	return out
}

// frameIndexInModel maps a structure part to where it sits in the drawn model,
// so a finding about the frame can point at something the page can light up.
func frameIndexInModel(res *Result, frame []part.Placed) map[int]int {
	out := make(map[int]int, len(frame))
	for i, f := range frame {
		for j, p := range res.Model.Parts {
			if p.Name == f.Part && p.Pos.Sub(f.Origin).Len() < 1e-6 {
				out[i] = j
				break
			}
		}
	}
	return out
}
