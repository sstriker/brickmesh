// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"regexp"
	"strconv"
	"sync"
)

// Titles is anything that can say what a part calls itself.
type Titles interface {
	Title(part string) (string, error)
}

// toothRe reads a count out of a gear's own title. Only "Technic Gear N Tooth":
// a gear rack has teeth and no ratio, and a turntable's title mentions teeth it
// does not mesh with.
var toothRe = regexp.MustCompile(`^Technic Gear\s+(\d+)\s+Tooth\b`)

// LibraryTeeth reads tooth counts out of part titles.
//
// Cached, because a model has thousands of parts and most of them are asked
// about once per pair.
type LibraryTeeth struct {
	From Titles
	mu   sync.Mutex
	seen map[string]int
}

// Teeth is how many a part has, and whether its title said so at all.
func (l *LibraryTeeth) Teeth(part string) (int, bool) {
	if l == nil || l.From == nil {
		return 0, false
	}
	l.mu.Lock()
	if l.seen == nil {
		l.seen = map[string]int{}
	}
	if n, ok := l.seen[part]; ok {
		l.mu.Unlock()
		return n, n > 0
	}
	l.mu.Unlock()

	n := 0
	if title, err := l.From.Title(part); err == nil {
		if m := toothRe.FindStringSubmatch(title); m != nil {
			n, _ = strconv.Atoi(m[1])
		}
	}
	l.mu.Lock()
	l.seen[part] = n
	l.mu.Unlock()
	return n, n > 0
}
