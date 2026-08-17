# Extractor

Run this once. It fetches the LDraw parts library and the LDCad shadow library,
expands the grids, filters out everything that is not a real orderable part,
and writes `data/catalog.json` for the Go engine.

```console
python -m brickmesh_extract.build --out ../data/catalog.json --tier 2
```

Installed as a package it is also on the path as `brickmesh-extract`. Tiers are
1 for common parts, 2 for all Technic, 3 for the whole library. `--limit` cuts
the run short for a quick trial, and `--infer-holes` fills in hole rows the
shadow library only names once — that one downloads the geometry of every part,
so it is slow.

The output is a JSON array, one object per part with `id`, `title`, `tier`,
`holes` and `pins`, which is what `internal/catalog` decodes. Note that the
dict this module passes around internally is keyed by part id and names that
field `part`; `to_records` is the single place where the two meet, and the
fixture in `internal/catalog/testdata/` is checked against it from both sides.

Why this stays in Python rather than moving to Go: the parsing is full of edge
cases that each had to be found — group snaps that are not pins, subparts that
do not exist separately, hole rows the shadow library only half describes.
Rewriting that gains nothing and costs you the same bugs again.
