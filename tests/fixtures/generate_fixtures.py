# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Generate the synthetic parts the tests run against.

These are NOT LDraw parts. The repository deliberately ships neither the LDraw
parts library nor the LDCad shadow library (see ATTRIBUTION.md), and the shadow
library's share-alike condition would carry over to anything derived from it.
So the fixtures here are plain boxes and a square tube, written by this script
in the LDraw and LDCad file formats but authored from scratch under the same
Apache-2.0 license as the rest of the repository.

That is enough to exercise the parsing, the grid expansion, the voxel grid and
the hole probe. It is not enough for tooth engagement: `interfere` and `bevel`
answer questions about real gear teeth, and a synthetic box cannot stand in for
those.

Run `python tests/fixtures/generate_fixtures.py` to rewrite the files;
test_fixtures.py checks that what is committed matches what this produces.
"""
from __future__ import annotations

import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
LDRAW_DIR = HERE / "ldraw"
SHADOW_DIR = HERE / "shadow" / "LDCadShadowLibrary-main" / "parts"

COLOR = 16  # the LDraw "main color" placeholder


def quad(a, b, c, d) -> str:
    nums = " ".join(f"{v:g}" for p in (a, b, c, d) for v in p)
    return f"4 {COLOR} {nums}"


def face(axis: int, coord: float, a0: float, a1: float,
         b0: float, b1: float, positive: bool) -> str:
    """
    One axis-aligned rectangle, wound so its normal points along +axis when
    `positive`, and -axis otherwise. `a` and `b` are the two remaining axes in
    x, y, z order.

    Everything here is built from this so the winding stays consistent; a mesh
    with mixed winding still looks right but reports a nonsense volume.
    """
    def point(a: float, b: float) -> tuple[float, float, float]:
        if axis == 0:
            return (coord, a, b)
        if axis == 1:
            return (a, coord, b)
        return (a, b, coord)

    corners = [point(a0, b0), point(a1, b0), point(a1, b1), point(a0, b1)]
    # For x and z that ordering already faces +axis; for y it faces -y.
    if (axis == 1) == positive:
        corners.reverse()
    return quad(*corners)


def box(cx: float, cy: float, cz: float,
        sx: float, sy: float, sz: float) -> list[str]:
    """Six outward-facing quads."""
    x0, x1 = cx - sx / 2, cx + sx / 2
    y0, y1 = cy - sy / 2, cy + sy / 2
    z0, z1 = cz - sz / 2, cz + sz / 2
    return [
        face(0, x1, y0, y1, z0, z1, True), face(0, x0, y0, y1, z0, z1, False),
        face(1, y1, x0, x1, z0, z1, True), face(1, y0, x0, x1, z0, z1, False),
        face(2, z1, x0, x1, y0, y1, True), face(2, z0, x0, x1, y0, y1, False),
    ]


def tube_along_x(length: float, outer: float, hole: float) -> list[str]:
    """
    A square tube: a box with a square hole bored straight through X.

    The hole has to stay open all the way through, because that is what the
    hole probe looks for and what the voxel grid must not silt up.

    Every face is split at the bore edges (+/-h) so that no edge of one face
    meets the middle of another. Without that the walls span the full width
    while the end caps stop at the bore, and the T-junctions left behind make
    the mesh non-manifold — which reads as a solid with zero volume.
    """
    x0, x1 = -length / 2, length / 2
    o, h = outer / 2, hole / 2
    bands = ((-o, -h), (-h, h), (h, o))
    out = []

    # Outer skin, each wall cut into three bands so it lines up with the caps.
    for lo, hi in bands:
        out.append(face(1, +o, x0, x1, lo, hi, True))    # +Y wall, banded in Z
        out.append(face(1, -o, x0, x1, lo, hi, False))   # -Y wall, banded in Z
        out.append(face(2, +o, x0, x1, lo, hi, True))    # +Z wall, banded in Y
        out.append(face(2, -o, x0, x1, lo, hi, False))   # -Z wall, banded in Y

    # The bore, facing inward: normals point back toward the axis.
    out.append(face(1, +h, x0, x1, -h, h, False))
    out.append(face(1, -h, x0, x1, -h, h, True))
    out.append(face(2, +h, x0, x1, -h, h, False))
    out.append(face(2, -h, x0, x1, -h, h, True))

    # Each end is a 3x3 grid of cells with the middle one left open.
    for x, positive in ((x1, True), (x0, False)):
        for ylo, yhi in bands:
            for zlo, zhi in bands:
                if (ylo, yhi) == (-h, h) and (zlo, zhi) == (-h, h):
                    continue                      # the bore
                out.append(face(0, x, ylo, yhi, zlo, zhi, positive))
    return out


def part(title: str, name: str, body: list[str]) -> str:
    head = [
        f"0 {title}",
        f"0 Name: {name}",
        "0 Author: brickmesh test fixtures",
        "0 !LDRAW_ORG Unofficial_Part",
        "0 !LICENSE Apache-2.0, authored for brickmesh; NOT an LDraw part",
        "",
    ]
    return "\n".join(head + body) + "\n"


def subfile(matrix_translation, name: str) -> str:
    """A type-1 reference, identity rotation."""
    x, y, z = matrix_translation
    return f"1 {COLOR} {x:g} {y:g} {z:g} 1 0 0 0 1 0 0 0 1 {name}"


def ldraw_files() -> dict[str, str]:
    return {
        # A cube: every dimension equal, so no axis stands out.
        "fixcube.dat": part("Test Cube 40 LDU", "fixcube.dat",
                            box(0, 0, 0, 40, 40, 40)),
        # Rod-like: two equal small dimensions and one long, so the rotation
        # axis heuristic should pick the LONG one (X).
        "fixbeam.dat": part("Test Beam 5 x 1 x 1", "fixbeam.dat",
                            box(0, 0, 0, 100, 20, 20)),
        # Disc-like: two equal large dimensions and one thin, so the heuristic
        # should pick the SHORT one (Z).
        "fixdisc.dat": part("Test Disc 40 x 40 x 8", "fixdisc.dat",
                            box(0, 0, 0, 40, 40, 8)),
        # A square hole bored through X.
        "fixtube.dat": part("Test Tube 40 x 40 with 12 LDU Bore", "fixtube.dat",
                            tube_along_x(100, 40, 12)),
        # A slim connector, long along Y: what actually bridges two beams whose
        # hole planes are 40 LDU apart. Thin enough that meeting them counts as
        # touching rather than intersecting.
        "fixpin.dat": part("Test Pin 48 LDU", "fixpin.dat",
                           box(0, 0, 0, 8, 48, 8)),
        # Two references to the SAME subfile at different places. Deduplicating
        # repeated subfiles would collapse this into one cube, which is the bug
        # ldraw._resolve warns about.
        "fixpair.dat": part("Test Pair of Cubes", "fixpair.dat",
                            [subfile((-40, 0, 0), "fixcube.dat"),
                             subfile((40, 0, 0), "fixcube.dat")]),
    }


def shadow_files() -> dict[str, str]:
    """
    Shadow-style snap metadata, in the LDCad meta format but written here.

    fixbeam gets a row of five holes 20 LDU apart via the grid notation, one
    axle hole, and one grouped snap that must NOT be treated as a plain port.
    """
    beam = [
        '0 LDCad shadow info for "Test Beam 5 x 1 x 1"',
        "0 !LDCAD SNAP_CYL [gender=F] [caps=one] [secs=R 6 8] "
        "[pos=0 0 0] [ori=1 0 0 0 0 -1 0 1 0] [grid=C 5 1 20 0]",
        "0 !LDCAD SNAP_CYL [gender=F] [caps=one] [secs=A 6 8] "
        "[pos=0 0 40] [ori=1 0 0 0 0 -1 0 1 0]",
        "0 !LDCAD SNAP_CYL [gender=M] [caps=one] [secs=R 6 8] "
        "[pos=50 0 0] [ori=0 0 1 0 1 0 -1 0 0]",
        "0 !LDCAD SNAP_CYL [gender=F] [group=craneArm] [secs=R 4 8] "
        "[pos=0 10 0] [ori=1 0 0 0 1 0 0 0 1]",
        "",
    ]
    cube = [
        '0 LDCad shadow info for "Test Cube 40 LDU"',
        "0 !LDCAD SNAP_CYL [gender=F] [caps=one] [secs=R 6 8] "
        "[pos=0 0 0] [ori=1 0 0 0 0 -1 0 1 0]",
        "",
    ]
    # A part whose hole is only reachable through an include reference, which
    # is the fallback branch in snap.rotation_axis.
    incl = [
        '0 LDCad shadow info for "Test Part with an Included Hole"',
        "0 !LDCAD SNAP_INCL [ref=confh-pinhole] [pos=0 0 0] "
        "[ori=1 0 0 0 0 -1 0 1 0]",
        "",
    ]
    # A subpart. LDraw marks these with a ~ and they cannot be ordered on their
    # own, so the catalog has to drop them however many ports they advertise.
    sub = [
        '0 LDCad shadow info for "~Test Beam Subpart"',
        "0 !LDCAD SNAP_CYL [gender=F] [caps=one] [secs=R 6 8] "
        "[pos=0 0 0] [ori=1 0 0 0 0 -1 0 1 0]",
        "",
    ]
    return {
        "fixbeam.dat": "\n".join(beam),
        "fixcube.dat": "\n".join(cube),
        "fixincl.dat": "\n".join(incl),
        "fixsub.dat": "\n".join(sub),
    }


def main() -> int:
    LDRAW_DIR.mkdir(parents=True, exist_ok=True)
    SHADOW_DIR.mkdir(parents=True, exist_ok=True)
    for name, text in ldraw_files().items():
        (LDRAW_DIR / name).write_text(text, encoding="utf-8")
    for name, text in shadow_files().items():
        (SHADOW_DIR / name).write_text(text, encoding="utf-8")
    print(f"wrote {len(ldraw_files())} parts to {LDRAW_DIR}")
    print(f"wrote {len(shadow_files())} shadow files to {SHADOW_DIR}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
