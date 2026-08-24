// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Command brickmesh-assets builds the two files a browser downloads.
//
//	brickmesh-assets --out web/data --tier 2
//
// The port index goes in catalog.bin and is meant to be fetched whole; the
// triangles go in meshes.bin and are meant to be fetched by byte range, a part
// at a time, once the search has decided which parts it wants. See
// docs/architecture.md for why those are different strategies.
//
// Both are derived from the LDCad shadow library and the LDraw parts library.
// Publishing them means publishing data derived from those, which carries their
// terms — see ATTRIBUTION.md, and put the notice next to the download.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sstriker/brickmesh/internal/assets"
	"github.com/sstriker/brickmesh/internal/extract"
	"github.com/sstriker/brickmesh/internal/ldraw"
	"github.com/sstriker/brickmesh/internal/pipeline"
	"github.com/sstriker/brickmesh/internal/progress"
	"github.com/sstriker/brickmesh/internal/shadow"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		out      = flag.String("out", "web/data", "directory to write the two files into")
		tier     = flag.Uint("tier", 2, "1 common, 2 all Technic, 3 the whole library")
		meshTier = flag.Uint("mesh-tier", 0,
			"build meshes only up to this tier; 0 means the same as --tier")
		limit    = flag.Int("limit", 0, "stop after this many parts; 0 for all")
		skipMesh = flag.Bool("no-meshes", false, "write only catalog.bin")
		cacheDir = flag.String("shadow-cache", "",
			"where the shadow library lives (default ~/.cache/brickmesh-shadow)")
	)
	flag.Parse()

	if *tier < 1 || *tier > 3 {
		return fmt.Errorf("tier must be 1, 2 or 3")
	}
	if *meshTier == 0 {
		*meshTier = *tier
	}

	// Building the whole library is minutes of work and a lot of it is fetching
	// parts, so it has to be possible to give up on it.
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	logf := func(m string) { fmt.Fprintln(os.Stderr, m) }
	logf("fetching the LDCad shadow library ...")
	root, err := shadow.Ensure(*cacheDir)
	if err != nil {
		return err
	}
	records, err := extract.Build(shadow.Open(root), extract.Options{
		MaxTier: uint8(*tier), Limit: *limit, Log: logf,
		// The parts library, so a beam's holes come from the hole primitives it
		// places rather than from the single hole its shadow file declares.
		Geom: ldraw.New(""),
	})
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return fmt.Errorf("no parts survived filtering; refusing to write empty files")
	}
	// Whatever the tier, the parts the engine actually places have to be in
	// here. Tier grades how common a part is; it does not know what this engine
	// puts in a model, and when the two were allowed to disagree the site
	// shipped tier 1 and got no gears. See pipeline.Placeable.
	titles := ldraw.New("")
	records, added := assets.WithPlaceable(records, pipeline.Placeable(),
		titles.Title)
	if len(added) > 0 {
		logf(fmt.Sprintf("added %d parts the engine places that tier %d left out: %s",
			len(added), *tier, strings.Join(added, " ")))
	}

	// Sorted once, here, and used for both files. A part's index is what
	// addresses it in each of them.
	catalog := assets.Sorted(assets.FromRecords(records))
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}

	rawCatalog, err := assets.WriteCatalog(catalog)
	if err != nil {
		return err
	}
	if err := write(filepath.Join(*out, "catalog.bin"), rawCatalog); err != nil {
		return err
	}
	// The terms travel with the data. Both licences require that anyone the
	// files reach can find out what they may do with them, and a notice left
	// behind in a repository does not reach someone who downloaded a file.
	if err := write(filepath.Join(*out, "LICENSE.txt"), []byte(assetLicense)); err != nil {
		return err
	}
	logf(fmt.Sprintf("catalog.bin: %d parts, %d ports, %s",
		len(catalog.Parts), portCount(catalog), size(len(rawCatalog))))

	if *skipMesh {
		return nil
	}
	rawMeshes, built, missing, err := buildMeshes(ctx, catalog, uint8(*meshTier), logf)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return fmt.Errorf("stopped before the meshes were finished")
		}
		return err
	}
	if err := write(filepath.Join(*out, "meshes.bin"), rawMeshes); err != nil {
		return err
	}
	logf(fmt.Sprintf("meshes.bin: %d of %d parts have geometry, %s",
		built, len(catalog.Parts), size(len(rawMeshes))))
	if len(missing) > 0 {
		// Named rather than counted. These are parts the shadow library knows
		// about and the parts mirror does not have, which is a gap in the data
		// rather than in this program — and one nobody can chase without the
		// numbers.
		logf(fmt.Sprintf("  %d part(s) had no geometry to read: %s",
			len(missing), sample(missing, 8)))
		logf("  they keep their index as empty entries, so the two files stay " +
			"aligned; anything placed from one will not draw")
	}
	return nil
}

// sample lists a few of a long list, so a log line stays a line.
func sample(all []string, n int) string {
	if len(all) <= n {
		return strings.Join(all, " ")
	}
	return fmt.Sprintf("%s and %d more",
		strings.Join(all[:n], " "), len(all)-n)
}

// buildMeshes reads every part's triangles, in the catalog's own order.
//
// A part whose geometry cannot be read gets an empty entry rather than being
// left out. Leaving it out would shift every part after it by one, and the
// index is the only thing tying the two files together.
func buildMeshes(ctx context.Context, catalog assets.Catalog, maxTier uint8,
	logf func(string)) (raw []byte, built int, missingIDs []string, err error) {

	lib := ldraw.New("")
	report := progress.Func(func(r progress.Report) {
		if r.Done%200 == 0 || r.Done == r.Total {
			logf(r.String())
		}
	})

	meshes := make([]assets.Mesh, len(catalog.Parts))
	for i, p := range catalog.Parts {
		if err := ctx.Err(); err != nil {
			return nil, 0, nil, err
		}
		report.Report(progress.Report{
			Stage: "meshes", Done: i + 1, Total: len(catalog.Parts), Note: p.ID,
		})
		if p.Tier > maxTier {
			continue // beyond what was asked for: an empty entry keeps the index
		}
		g, err := lib.Geometry(p.ID)
		if err != nil {
			missingIDs = append(missingIDs, p.ID)
			continue
		}
		meshes[i] = assets.IndexTriangles(g.Tris)
		built++
	}

	raw, err = assets.WriteMeshes(meshes)
	return raw, built, missingIDs, err
}

// assetLicense is written beside the generated files.
//
// Two licences, because the two files are derived from different things and one
// of them is share-alike. Spelled out rather than pointed at, since whoever
// ends up with these may have nothing else of the project.
const assetLicense = `These files are generated from two libraries, and carry their terms.

catalog.bin
    Where every part's holes and pins are, and which way they point.
    Derived from the LDCad Shadow Library by Roland Melkert, which is
    licensed CC BY-SA 4.0 — so this file is too. If you pass it on, or
    anything made from it, it stays CC BY-SA 4.0.
    https://creativecommons.org/licenses/by-sa/4.0/

meshes.bin
    Part geometry, as indexed triangles. Derived from The LDraw Parts
    Library, redistributable under CCAL 2.0 (Creative Commons Attribution
    License 2.0) — so this file is too, and it must carry attribution.
    https://creativecommons.org/licenses/by/2.0/

    The LDraw Parts Library is the work of LDraw.org volunteers. The LDraw
    Steering Committee holds an attribution to "The LDraw Parts Library"
    to be sufficient in a derivative work in lieu of a full author list.
    LDraw is a trademark of the LDraw.org organisation.

Neither library is affiliated with the LEGO Group. LEGO is a trademark of
the LEGO Group, which does not sponsor, authorize or endorse this project.

The software that generated these files is separate, and is under the
Apache License 2.0. Reading data under a licence does not put the reader
under it.
`

func portCount(c assets.Catalog) int {
	n := 0
	for _, p := range c.Parts {
		n += len(p.Ports)
	}
	return n
}

func write(path string, b []byte) error {
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func size(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
