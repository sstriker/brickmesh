// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package ldraw

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// LibraryURL is the official complete parts library.
//
// Not a mirror. The one this used to fetch from is a snapshot of part of the
// library from before 2015 or so, and quietly lacks every part released since:
// 17% of the Technic parts the shadow library knows about were missing, among
// them the axle joiner for the 3L driving ring, which is why the newer shifting
// system could not be measured at all. See docs/findings.md.
const LibraryURL = "https://library.ldraw.org/library/updates/complete.zip"

// LibraryRoot is the directory the archive extracts into, under the cache.
const LibraryRoot = "ldraw"

// What is kept out of the archive. It carries models, documentation and
// binaries as well, and none of that is read here.
var (
	keepDirs  = []string{"parts/", "p/"}
	keepFiles = []string{"CAlicense.txt", "CAreadme.txt"}
)

// Ensure downloads and extracts the parts library, and returns its root.
//
// Once. The archive is 144 MB and the tree it becomes is larger, so this is
// exactly the sort of thing a cache is for — and the same reasoning as the
// shadow library next door, which is fetched the same way.
//
// The licence files come out with the parts on purpose. The library is
// redistributable under CCAL 2.0, whose condition is that anyone you pass it to
// can find out the terms; shipping the parts and leaving the licence behind
// would not meet that.
func Ensure(dest string) (string, error) {
	if dest == "" {
		dest = DefaultCacheDir()
	}
	root := filepath.Join(dest, LibraryRoot)
	if fi, err := os.Stat(filepath.Join(root, "parts")); err == nil && fi.IsDir() {
		return root, nil
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}

	// One fetch, however many want it. `go test ./...` runs a process per
	// package and on a cold cache they all miss at once; without this they each
	// pull 144 MB, which is slow for us and rude to the server. The first to
	// claim the lock fetches and the rest wait for the tree to appear.
	//
	// A lock left behind by something that died would block every later run, so
	// waiting gives up after a while and fetches anyway. Two fetches is a waste;
	// a cache that can never be filled again is worse.
	if locked, err := claim(dest); err == nil && !locked {
		if root, ok := waitForLibrary(root, libraryWait); ok {
			return root, nil
		}
	} else if err == nil {
		defer os.Remove(lockPath(dest))
	}

	// To a file rather than to memory: a zip is read back to front, so it has
	// to be seekable, and holding 144 MB to avoid a temporary file is a poor
	// trade on a machine that might not have it to spare.
	tmp, err := os.CreateTemp(dest, "complete-*.zip")
	if err != nil {
		return "", err
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Get(LibraryURL)
	if err != nil {
		return "", fmt.Errorf("fetching the parts library: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching the parts library: %s", resp.Status)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return "", fmt.Errorf("downloading the parts library: %w", err)
	}

	zr, err := zip.OpenReader(tmp.Name())
	if err != nil {
		return "", fmt.Errorf("reading the parts library: %w", err)
	}
	defer zr.Close()

	// Extracted beside the destination and moved into place at the end, so an
	// interrupted download cannot leave a half a library looking like a whole
	// one — the check at the top of this function only looks for the directory.
	staging, err := os.MkdirTemp(dest, "ldraw-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)

	for _, f := range zr.File {
		rel, keep := wanted(f.Name)
		if !keep {
			continue
		}
		if err := extractOne(f, filepath.Join(staging, rel)); err != nil {
			return "", err
		}
	}
	if _, err := os.Stat(filepath.Join(staging, "parts")); err != nil {
		return "", fmt.Errorf("the archive held no parts directory: %w", err)
	}
	if err := os.Rename(staging, root); err != nil {
		// Someone else got there first. Several processes can be here at once —
		// `go test ./...` runs a process per package, and on a cold cache they
		// all miss and all fetch — so losing the race is an ordinary outcome
		// and not a failure. Their tree is as good as ours.
		if fi, statErr := os.Stat(filepath.Join(root, "parts")); statErr == nil && fi.IsDir() {
			return root, nil
		}
		return "", fmt.Errorf("putting the library in place: %w", err)
	}
	return root, nil
}

// wanted reports whether an archive entry is one of the ones kept, and what to
// call it once the leading "ldraw/" is off.
func wanted(name string) (string, bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	rel := strings.TrimPrefix(name, LibraryRoot+"/")
	if rel == name && !strings.HasPrefix(name, LibraryRoot+"/") {
		// Some builds of the archive have no leading directory at all.
		rel = name
	}
	if strings.HasSuffix(rel, "/") {
		return "", false
	}
	// A zip may name any path it likes, including one that climbs out of where
	// it is being written. Refuse those rather than trust the archive.
	if path.IsAbs(rel) || strings.Contains(rel, "..") {
		return "", false
	}
	for _, f := range keepFiles {
		if rel == f {
			return rel, true
		}
	}
	for _, d := range keepDirs {
		if strings.HasPrefix(rel, d) {
			return rel, true
		}
	}
	return "", false
}

func extractOne(f *zip.File, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, rc); err != nil {
		return fmt.Errorf("extracting %s: %w", f.Name, err)
	}
	return nil
}

// findInRoot looks a part up in the extracted library.
func (l *Library) findInRoot(name string) (string, bool) {
	if l.Root == "" {
		return "", false
	}
	atomic.AddInt64(&l.reads, 1)
	for _, dir := range SearchDirs {
		b, err := os.ReadFile(filepath.Join(l.Root, filepath.FromSlash(dir),
			filepath.FromSlash(name)))
		if err == nil {
			return string(b), true
		}
	}
	return "", false
}

// libraryWait is how long to wait for another process's download before giving
// up on it and fetching too. The archive takes a couple of minutes on a good
// connection and CI is not always on one.
const libraryWait = 15 * time.Minute

func lockPath(dest string) string { return filepath.Join(dest, ".fetching") }

// claim reports whether this process is the one that should fetch.
func claim(dest string) (bool, error) {
	f, err := os.OpenFile(lockPath(dest), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return false, nil // someone else has it
		}
		return false, err
	}
	return true, f.Close()
}

// waitForLibrary polls for the tree another process is extracting.
func waitForLibrary(root string, limit time.Duration) (string, bool) {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if fi, err := os.Stat(filepath.Join(root, "parts")); err == nil && fi.IsDir() {
			return root, true
		}
		time.Sleep(2 * time.Second)
	}
	return "", false
}
