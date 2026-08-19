// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package assets

import (
	"sort"
	"strings"

	"brickmesh/internal/extract"
)

// WithPlaceable adds any part the engine can place that the tier filter or the
// shadow library left out, and returns the names it had to add.
//
// The two sets are different and were allowed to disagree. Tier grades how
// common a part is, from its title; the placeable set is what this engine puts
// in a model. Gears are titled "Technic Gear ..." and so grade tier 2, the site
// ships tier 1, and the result was a published site whose every model had no
// gear geometry at all — the renderer drew gearboxes without gears and the
// clearance sweep skipped them and reported that nothing collided.
//
// Parts with no shadow file are the other half: 3647 and 32270 carry no ports,
// so the extractor drops them at every tier. They come in here with an empty
// port list, which is honest — the library describes none — while their
// triangles, which is what the sweep and the renderer actually read, still ship.
//
// Added parts are graded tier 1. That is not a fudge: tier decides what ships,
// and a part the engine reaches for in every model is as common as parts get.
//
// The list is passed in rather than imported so this package stays underneath
// the engine rather than beside it.
func WithPlaceable(records []extract.Record, placeable []string) ([]extract.Record, []string) {
	have := make(map[string]int, len(records))
	for i, r := range records {
		have[r.ID] = i
	}
	var added []string
	for _, name := range placeable {
		id := strings.TrimSuffix(name, ".dat")
		if i, ok := have[id]; ok {
			if records[i].Tier > 1 {
				records[i].Tier = 1
			}
			continue
		}
		records = append(records, extract.Record{
			ID: id, Title: "Technic (placed by brickmesh)", Tier: 1,
			Holes: []extract.Port{}, Pins: []extract.Port{},
		})
		added = append(added, id)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	sort.Strings(added)
	return records, added
}
