// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package catalog

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"brickmesh/internal/assets"
	"brickmesh/internal/extract"
)

// The two loaders read the same catalog from two encodings of it, and the
// engine cannot tell which it was handed. Nothing in the type system says so,
// so it is asserted here: the JSON is what the extractor's own tests compare
// against, and the binary is what gets published.
func TestBothLoadersAgree(t *testing.T) {
	records := []extract.Record{
		{ID: "32316.dat", Title: "Technic Beam  5", Tier: 1, Holes: []extract.Port{
			{-40, 0, 0, 0, 0, 1, 0}, {0, 0, 0, 0, 0, 1, 0}, {40, 0, 0, 0, 0, 1, 0},
		}},
		{ID: "3648b.dat", Title: "Technic Gear 24 Tooth", Tier: 1,
			Holes: []extract.Port{{0, 0, 0, 0, 0, 1, 1}}},
		{ID: "4519.dat", Title: "Technic Axle  3", Tier: 2,
			Pins: []extract.Port{{0, 0, 0, 1, 0, 0, 1}}},
		{ID: "9999.dat", Title: "No Ports At All", Tier: 3},
	}

	asJSON, err := Load(strings.NewReader(jsonFor(records)))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := assets.WriteCatalog(assets.FromRecords(records))
	if err != nil {
		t.Fatal(err)
	}
	asBinary, err := LoadBinary(raw)
	if err != nil {
		t.Fatal(err)
	}

	if len(asJSON.Parts) != len(asBinary.Parts) {
		t.Fatalf("JSON gave %d parts, binary %d",
			len(asJSON.Parts), len(asBinary.Parts))
	}
	for id, want := range asJSON.Parts {
		got, ok := asBinary.Parts[id]
		if !ok {
			t.Errorf("%s is missing from the binary catalog", id)
			continue
		}
		if got.Title != want.Title || got.Tier != want.Tier {
			t.Errorf("%s: binary has %q tier %d, JSON has %q tier %d",
				id, got.Title, got.Tier, want.Title, want.Tier)
		}
		if !reflect.DeepEqual(got.Ports, want.Ports) {
			t.Errorf("%s ports differ:\n binary %+v\n json   %+v",
				id, got.Ports, want.Ports)
		}
	}
}

// The kind byte is two bits standing for two enums, which only works while the
// enums keep their order. If either is renumbered this fails rather than
// quietly turning every axle hole into a pin hole.
func TestTheKindBitsMatchTheEnums(t *testing.T) {
	for _, c := range []struct {
		kind   uint8
		want   Port
		reason string
	}{
		{0, Port{Kind: Round, Gender: Female}, "a plain pin hole"},
		{assets.PortMale, Port{Kind: Round, Gender: Male}, "a pin"},
		{assets.PortCross, Port{Kind: Cross, Gender: Female}, "an axle hole"},
		{assets.PortMale | assets.PortCross, Port{Kind: Cross, Gender: Male}, "an axle"},
	} {
		got := fromBinary(assets.Port{Kind: c.kind})
		if got.Kind != c.want.Kind || got.Gender != c.want.Gender {
			t.Errorf("%s (byte %d) unpacked to kind %d gender %d, want %d and %d",
				c.reason, c.kind, got.Kind, got.Gender, c.want.Kind, c.want.Gender)
		}
	}
}

func TestARubbishBinaryCatalogIsAnError(t *testing.T) {
	if _, err := LoadBinary([]byte("not a catalog")); err == nil {
		t.Error("arbitrary bytes were accepted")
	}
}

// jsonFor writes the interchange format the extractor produces, so the JSON
// loader is fed exactly what it is fed in production.
func jsonFor(records []extract.Record) string {
	var b strings.Builder
	b.WriteString("[")
	for i, r := range records {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"id":"` + r.ID + `","title":"` + r.Title + `","tier":`)
		b.WriteByte('0' + r.Tier)
		b.WriteString(`,"holes":` + portsJSON(r.Holes))
		b.WriteString(`,"pins":` + portsJSON(r.Pins) + "}")
	}
	b.WriteString("]")
	return b.String()
}

func portsJSON(ports []extract.Port) string {
	if len(ports) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteString("[")
	for i, p := range ports {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("[")
		for j, v := range p {
			if j > 0 {
				b.WriteString(",")
			}
			b.WriteString(trimFloat(v))
		}
		b.WriteString("]")
	}
	b.WriteString("]")
	return b.String()
}

func trimFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}
