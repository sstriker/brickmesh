// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package shadow reads the LDCad shadow library.
//
// The shadow library (Roland Melkert, CC BY-SA 4.0) annotates LDraw parts with
// snap metadata: exactly where every axle hole and pin sits and which way it
// points. That is authoritative, so it replaces any bounding-box guess at a
// part's axis.
//
// It carries no gear meshing information — there is no gear meta in the whole
// library, by design — so this fixes axes and grids, not tooth engagement.
//
// The share-alike condition of BY-SA carries through to data derived from it.
// See ATTRIBUTION.md: the library is fetched, never vendored, and neither it
// nor the catalog built from it belongs in this repository.
package shadow

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sstriker/brickmesh/internal/geom"
)

const (
	URL     = "https://codeload.github.com/RolandMelkert/LDCadShadowLibrary/tar.gz/refs/heads/main"
	rootDir = "LDCadShadowLibrary-main"
)

var (
	metaRe  = regexp.MustCompile(`!LDCAD\s+(SNAP_\w+)\s*(.*)`)
	attrRe  = regexp.MustCompile(`\[(\w+)=([^\]]*)\]`)
	titleRe = regexp.MustCompile(`shadow info for\s+"(.*)"\s*$`)
)

// DefaultCacheDir matches the Python extractor's snap.SHADOW_DIR.
func DefaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".cache/brickmesh-shadow"
	}
	return filepath.Join(home, ".cache", "brickmesh-shadow")
}

// Ensure returns the extracted library root, downloading it once if needed.
func Ensure(dest string) (string, error) {
	if dest == "" {
		dest = DefaultCacheDir()
	}
	root := filepath.Join(dest, rootDir)
	if fi, err := os.Stat(root); err == nil && fi.IsDir() {
		return root, nil
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(URL)
	if err != nil {
		return "", fmt.Errorf("fetching the shadow library: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching the shadow library: %s", resp.Status)
	}

	// Streamed straight into place: there is no reason to leave the archive
	// beside the tree it was extracted to, and a cache has to carry whatever
	// is there.
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	if err := extract(tar.NewReader(gz), dest); err != nil {
		return "", err
	}
	return root, nil
}

// extract unpacks the archive, refusing any entry that would land outside dest.
func extract(tr *tar.Reader, dest string) error {
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.Clean("/"+hdr.Name))
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("refusing entry outside the destination: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			// Bounded so a hostile archive cannot fill the disk.
			if _, err := io.CopyN(f, tr, 64<<20); err != nil && err != io.EOF {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
		// Links and devices are skipped: the library holds neither.
	}
}

// Snap is one connection point declared by the shadow library.
type Snap struct {
	Kind   string // SNAP_CYL / SNAP_INCL / ...
	Gender string // M / F / ""
	Pos    geom.Vec3
	Ori    geom.Mat3
	Group  string // a named connection; see Generic
	Secs   string
	Grid   string
	Ref    string
}

// Axis is the direction the snap points. LDCad cylinders point along +Y before
// the orientation matrix is applied.
func (s Snap) Axis() geom.Vec3 {
	return s.Ori.Apply(geom.Vec3{Y: 1})
}

// Generic reports whether this is an ordinary hole or pin.
//
// Snaps carrying a [group=...] belong to one specific connection: a crane arm,
// a door hinge, an electrical plug, a ball joint. Treating those as ordinary
// ports is what gives a liftarm a phantom male port along its whole length,
// when that is really the slot for a crane-arm clamp.
func (s Snap) Generic() bool { return s.Group == "" }

// Axle reports a cross-shaped section rather than a round one.
func (s Snap) Axle() bool { return strings.HasPrefix(strings.TrimSpace(s.Secs), "A") }

// Library is an extracted shadow library on disk.
type Library struct{ Root string }

func Open(root string) *Library { return &Library{Root: root} }

func (l *Library) partsDir() string { return filepath.Join(l.Root, "parts") }

// Snaps returns every snap declared for a part, or nil when the library has no
// file for it.
func (l *Library) Snaps(part string) []Snap {
	name := part
	if !strings.HasSuffix(name, ".dat") {
		name += ".dat"
	}
	var text []byte
	for _, sub := range []string{"parts", "p"} {
		b, err := os.ReadFile(filepath.Join(l.Root, sub, name))
		if err == nil {
			text = b
			break
		}
	}
	if text == nil {
		return nil
	}

	var out []Snap
	for _, line := range strings.Split(string(text), "\n") {
		m := metaRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		attrs := map[string]string{}
		for _, a := range attrRe.FindAllStringSubmatch(m[2], -1) {
			attrs[a[1]] = strings.TrimSpace(a[2])
		}
		s := Snap{
			Kind:   m[1],
			Gender: attrs["gender"],
			Group:  attrs["group"],
			Secs:   attrs["secs"],
			Grid:   attrs["grid"],
			Ref:    attrs["ref"],
			Pos:    parseVec(attrs["pos"]),
			Ori:    parseMat(attrs["ori"]),
		}
		out = append(out, s)
	}
	return out
}

// RotationAxis is the part's true rotation axis, taken from its snap data
// rather than guessed from a bounding box. The second return value says where
// it came from; ok is false when the library cannot answer.
func (l *Library) RotationAxis(part string) (axis geom.Vec3, source string, ok bool) {
	snaps := l.Snaps(part)
	for _, s := range snaps {
		if s.Kind == "SNAP_CYL" && s.Gender == "F" && s.Axle() {
			return s.Axis(), "LDCad shadow library, axle hole", true
		}
	}
	for _, s := range snaps {
		if s.Kind == "SNAP_CYL" && s.Gender == "F" {
			return s.Axis(), "LDCad shadow library, round hole", true
		}
	}
	// Many parts define their hole indirectly, through an include pointing at a
	// generic hole definition. Those are always female.
	for _, s := range snaps {
		if s.Kind == "SNAP_INCL" &&
			(strings.Contains(s.Ref, "hole") || strings.Contains(s.Ref, "conn")) {
			return s.Axis(), "LDCad shadow library, include " + s.Ref, true
		}
	}
	return geom.Vec3{}, "", false
}

// Parts lists every part the library describes, sorted.
func (l *Library) Parts() ([]string, error) {
	entries, err := os.ReadDir(l.partsDir())
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if n := e.Name(); strings.HasSuffix(n, ".dat") {
			out = append(out, strings.TrimSuffix(n, ".dat"))
		}
	}
	// ReadDir already sorts by filename, which is the order the extractor uses.
	return out, nil
}

// Titles reads part titles out of the shadow headers.
//
// Every shadow file opens with the LDraw title quoted back:
//
//	0 LDCad shadow info for "Technic Beam  7 x  5 with Open Center  5 x  3"
//
// Those titles drive both the tier and the subpart filter, and this is the only
// copy of them on disk once the library is extracted. Reading them from LDraw
// instead would mean fetching a few thousand parts to look at their first line.
// The ~ and = prefixes are carried through, which is what the filter needs.
func (l *Library) Titles() (map[string]string, error) {
	entries, err := os.ReadDir(l.partsDir())
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".dat") {
			continue
		}
		f, err := os.Open(filepath.Join(l.partsDir(), name))
		if err != nil {
			continue
		}
		buf := make([]byte, 512)
		n, _ := f.Read(buf)
		f.Close()
		first, _, _ := strings.Cut(string(buf[:n]), "\n")
		if m := titleRe.FindStringSubmatch(strings.TrimSpace(first)); m != nil {
			out[strings.TrimSuffix(name, ".dat")] = strings.TrimSpace(m[1])
		}
	}
	return out, nil
}

func parseVec(s string) geom.Vec3 {
	f := parseFloats(s)
	if len(f) < 3 {
		return geom.Vec3{}
	}
	return geom.Vec3{X: f[0], Y: f[1], Z: f[2]}
}

func parseMat(s string) geom.Mat3 {
	f := parseFloats(s)
	if len(f) < 9 {
		return geom.Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	}
	var m geom.Mat3
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			m[r][c] = f[r*3+c]
		}
	}
	return m
}

func parseFloats(s string) []float64 {
	fields := strings.Fields(s)
	out := make([]float64, 0, len(fields))
	for _, t := range fields {
		v, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}
