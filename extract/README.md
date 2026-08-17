# Extractor

Run this once. It fetches the LDraw parts library and the LDCad shadow library,
expands the grids, filters out everything that is not a real orderable part,
and writes `data/catalog.json` for the Go engine.

```
python -m brickmesh_extract.build --out ../data/catalog.json --tier 2
```

Why this stays in Python rather than moving to Go: the parsing is full of edge
cases that each had to be found — group snaps that are not pins, subparts that
do not exist separately, hole rows the shadow library only half describes.
Rewriting that gains nothing and costs you the same bugs again.
