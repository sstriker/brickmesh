# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Build the parts catalog the Go engine reads.

Run once. It fetches the LDCad shadow library, expands the grid notation into
individual ports, drops everything that is not a real orderable part, and
writes a single JSON file.

The output is a JSON ARRAY, one object per part, with `id`, `title`, `tier`,
`holes` and `pins`. That is the shape internal/catalog expects; the in-memory
dict this module works with is keyed by part id and names that field `part`,
so the two are deliberately converted here rather than left to diverge.
"""
from __future__ import annotations

import argparse
import json
import os
import sys

from . import catalog, snap


def to_records(cat: dict, max_tier: int = 3) -> list[dict]:
    """
    Turn the extractor's dict into the engine's array, keeping only parts up to
    `max_tier` and only those that actually have a port.

    Pure: no downloads, so this is the part that can be tested directly.
    """
    out = []
    for pid, entry in sorted(cat.items()):
        title = entry.get("title", "")
        tier = catalog.tier_of(title)
        if tier > max_tier:
            continue
        holes = entry.get("holes") or []
        pins = entry.get("pins") or []
        if not holes and not pins:
            continue
        out.append({
            "id": pid,
            "title": title,
            "tier": tier,
            "holes": [[float(v) for v in row] for row in holes],
            "pins": [[float(v) for v in row] for row in pins],
        })
    return out


def build(max_tier: int = 3, limit: int | None = None,
          infer_holes: bool = False, log=print) -> list[dict]:
    """Fetch, expand, filter. Returns the records ready to be written."""
    log("fetching the LDCad shadow library ...")
    root = snap.ensure_library()
    parts_dir = os.path.join(root, "parts")

    log("expanding port grids ...")
    cat = catalog.build(limit)
    log(f"  {len(cat)} parts with port data")

    log("dropping subparts and obsolete entries ...")
    cat = catalog.usable(cat, parts_dir)
    log(f"  {len(cat)} usable parts")

    if infer_holes:
        # Downloads the geometry of every part, so it is opt-in.
        log("inferring hole rows the shadow library only half describes ...")
        cat = catalog.infer_missing_holes(cat)

    records = to_records(cat, max_tier)
    ports = sum(len(r["holes"]) + len(r["pins"]) for r in records)
    log(f"  {len(records)} parts at tier <= {max_tier}, {ports} ports")
    return records


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m brickmesh_extract.build",
        description="Build the parts catalog for the brickmesh engine.")
    parser.add_argument("--out", default="data/catalog.json",
                        help="where to write the catalog (default: %(default)s)")
    parser.add_argument("--tier", type=int, default=3, choices=(1, 2, 3),
                        help="1 common, 2 all Technic, 3 the whole library "
                             "(default: %(default)s)")
    parser.add_argument("--limit", type=int, default=None,
                        help="stop after this many parts; for a quick trial run")
    parser.add_argument("--infer-holes", action="store_true",
                        help="fill in hole rows the shadow library only names "
                             "once. Downloads every part's geometry, so it is slow")
    args = parser.parse_args(argv)

    records = build(max_tier=args.tier, limit=args.limit,
                    infer_holes=args.infer_holes,
                    log=lambda m: print(m, file=sys.stderr))
    if not records:
        print("no parts survived filtering; refusing to write an empty catalog",
              file=sys.stderr)
        return 1

    parent = os.path.dirname(os.path.abspath(args.out))
    os.makedirs(parent, exist_ok=True)
    with open(args.out, "w", encoding="utf-8") as fh:
        json.dump(records, fh)
    print(f"wrote {args.out}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
