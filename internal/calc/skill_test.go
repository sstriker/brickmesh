// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package calc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The plugin carries its own copy of the skill, because a plugin has to be
// installable without the rest of the repository. A copy that has drifted is
// worse than no copy: it is the version people would get.
func TestThePluginShipsTheSkillThisRepositoryUses(t *testing.T) {
	mine := read(t, filepath.Join("..", "..", ".claude", "skills", "brickmesh", "SKILL.md"))
	theirs := read(t, filepath.Join("..", "..", "plugin", "brickmesh", "skills", "brickmesh", "SKILL.md"))
	if mine != theirs {
		t.Errorf("plugin/brickmesh/skills/brickmesh/SKILL.md has drifted from " +
			".claude/skills/brickmesh/SKILL.md — copy the one this repository " +
			"actually uses over the one that gets published")
	}
}

// The marketplace has to point at a plugin that is there, and the plugin has to
// declare the skill the marketplace advertises. Both are one-line mistakes that
// only show up when somebody tries to install.
func TestTheMarketplacePointsAtSomethingThatExists(t *testing.T) {
	var market struct {
		Name    string                `json:"name"`
		Owner   struct{ Name string } `json:"owner"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	raw := read(t, filepath.Join("..", "..", ".claude-plugin", "marketplace.json"))
	if err := json.Unmarshal([]byte(raw), &market); err != nil {
		t.Fatalf("marketplace.json is not valid JSON: %v", err)
	}
	if market.Name == "" || market.Owner.Name == "" {
		t.Error("a marketplace needs a name and an owner, or it will not load")
	}
	if len(market.Plugins) == 0 {
		t.Fatal("a marketplace with no plugins offers nothing")
	}
	for _, p := range market.Plugins {
		if !strings.HasPrefix(p.Source, "./") {
			continue // a git or archive source, which is not ours to check here
		}
		at := filepath.Join("..", "..", filepath.FromSlash(strings.TrimPrefix(p.Source, "./")))
		manifest := filepath.Join(at, ".claude-plugin", "plugin.json")
		if _, err := os.Stat(manifest); err != nil {
			t.Errorf("%s points at %s and there is no %s under it",
				market.Name, p.Source, filepath.Join(".claude-plugin", "plugin.json"))
			continue
		}
		var plugin struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(read(t, manifest)), &plugin); err != nil {
			t.Errorf("%s: %v", manifest, err)
			continue
		}
		if plugin.Name != p.Name {
			t.Errorf("the marketplace calls it %q and its manifest calls it %q; "+
				"installing uses the marketplace's name", p.Name, plugin.Name)
		}
		// A plugin whose skills directory is empty installs and does nothing.
		skills, err := os.ReadDir(filepath.Join(at, "skills"))
		if err != nil || len(skills) == 0 {
			t.Errorf("%s ships no skills, so installing it changes nothing", p.Name)
		}
	}
}

// The site offers the two commands, and they have to name the marketplace and
// plugin that actually exist. Getting either wrong is a copy-paste that fails
// for every visitor.
func TestThePageOffersInstallCommandsThatMatchTheMarketplace(t *testing.T) {
	page := read(t, filepath.Join("..", "..", "web", "index.html"))
	var market struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name string `json:"name"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal([]byte(read(t,
		filepath.Join("..", "..", ".claude-plugin", "marketplace.json"))), &market); err != nil {
		t.Fatal(err)
	}
	want := "/plugin install " + market.Plugins[0].Name + "@" + market.Name
	if !strings.Contains(page, want) {
		t.Errorf("the page should offer %q; it says something else", want)
	}
	if !strings.Contains(page, "/plugin marketplace add sstriker/brickmesh") {
		t.Error("the page should say which marketplace to add first")
	}
}

func read(t *testing.T, at string) string {
	t.Helper()
	raw, err := os.ReadFile(at)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
