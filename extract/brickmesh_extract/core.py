# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""Model definition, geometric validation and .ldr export."""
from __future__ import annotations

from dataclasses import dataclass, field

import numpy as np

from . import ldraw, snap, teeth
from .ldraw import LDU_STUD

# LEGO gear geometry: pitch radius in mm = teeth / 2.
# 1 mm = 2.5 LDU, so pitch radius in LDU = teeth * 1.25.
LDU_PER_MM = 2.5
PITCH_LDU_PER_TOOTH = 1.25

IDENTITY = np.eye(3)


def rot(axis: str, deg: float) -> np.ndarray:
    t = np.radians(deg)
    c, s = np.cos(t), np.sin(t)
    if axis == "x":
        return np.array([[1, 0, 0], [0, c, -s], [0, s, c]])
    if axis == "y":
        return np.array([[c, 0, s], [0, 1, 0], [-s, 0, c]])
    if axis == "z":
        return np.array([[c, -s, 0], [s, c, 0], [0, 0, 1]])
    raise ValueError(axis)


def infer_native_axis(g: ldraw.PartGeometry) -> tuple[int, str]:
    """
    Work out which axis a part rotates about in its default orientation.

    Disc-like (two equal large dims, one small)  -> axis is the SHORT one.
    Rod-like  (two equal small dims, one large)  -> axis is the LONG one.
    Returns (axis_index, confidence) where confidence is 'high' or 'check'.
    """
    s = g.size
    order = np.argsort(s)
    lo, mid, hi = s[order[0]], s[order[1]], s[order[2]]

    if abs(mid - hi) < 0.05 * hi and lo < 0.7 * hi:      # disc
        return int(order[0]), "high"
    if abs(lo - mid) < 0.05 * hi and hi > 1.4 * mid:     # rod
        return int(order[2]), "high"
    return int(order[0]), "check"


def radial_profile(g: ldraw.PartGeometry, axis: int, bins: int = 40):
    """
    Max radius as a function of position along `axis`. Locates features such as
    the ring gear inside a differential housing, so a mating gear can be placed
    in the correct plane instead of guessed at.
    """
    other = [i for i in range(3) if i != axis]
    v = g.verts
    pos = v[:, axis]
    rad = np.hypot(v[:, other[0]], v[:, other[1]])
    edges = np.linspace(pos.min(), pos.max(), bins + 1)
    idx = np.clip(np.digitize(pos, edges) - 1, 0, bins - 1)
    out = []
    for b in range(bins):
        m = idx == b
        if m.any():
            out.append(((edges[b] + edges[b + 1]) / 2, float(rad[m].max())))
    return out


def gear_plane(g: ldraw.PartGeometry, axis: int) -> float:
    """Position along `axis` of the largest-diameter feature."""
    prof = radial_profile(g, axis)
    return max(prof, key=lambda p: p[1])[0]


def bevel_placement(apex, diff_axis, radius: float, azimuth_deg: float):
    """
    Position of the pinion around the crown. The engagement stays
    perpendicular; the azimuth only turns the whole sub-assembly about the axis
    of the differential.

    Returns (position, orientation, bearing-points-on-the-grid).
    """
    d = np.asarray(diff_axis, dtype=float); d /= np.linalg.norm(d)
    tmp = np.array([1.0, 0, 0]) if abs(d[0]) < 0.9 else np.array([0, 1.0, 0])
    u = np.cross(d, tmp); u /= np.linalg.norm(u)
    v = np.cross(d, u)
    t = np.radians(azimuth_deg)
    direction = np.cos(t) * u + np.sin(t) * v
    pos = np.asarray(apex, dtype=float) + radius * direction

    # Where along this axis does a bearing land on the hole grid?
    # Tolerance in LDU, not exact: an azimuth passed in as 36.87 degrees is a
    # rounding of arctan(3/4) and would otherwise fall just outside it.
    # For exact work, give the azimuth as a triple rather than in degrees.
    bearings = []
    for dist in np.arange(20.0, 320.0, 20.0):
        p = np.asarray(apex, dtype=float) + dist * direction
        err = max(abs(x - 20.0 * round(x / 20.0)) for x in p)
        if err < 0.5:
            bearings.append((float(dist), np.round(p, 1), float(err)))
    return pos, direction, bearings


def azimuth_from_triple(a: int, b: int) -> float:
    """Exact azimuth from a Pythagorean triple, without rounding loss."""
    return float(np.degrees(np.arctan2(b, a)))


def buildable_azimuths(max_reach_studs: int = 8) -> list[tuple[float, int, int, int]]:
    """Azimuths where a bearing lands on the grid, from Pythagorean triples."""
    out = []
    for a in range(0, max_reach_studs + 1):
        for b in range(0, max_reach_studs + 1):
            if a == 0 and b == 0:
                continue
            c2 = a * a + b * b
            c = int(round(np.sqrt(c2)))
            if c * c == c2 and c <= max_reach_studs:
                out.append((float(np.degrees(np.arctan2(b, a))), c, a, b))
    seen, uniq = set(), []
    for ang, c, a, b in sorted(out):
        k = round(ang, 3)
        if k not in seen:
            seen.add(k); uniq.append((ang, c, a, b))
    return uniq


BUILDABLE_AZIMUTHS = {
    0.00:   (1, 0, 1),      # (leg a, leg b, reach c) - already on the grid
    36.87:  (4, 3, 5),
    53.13:  (3, 4, 5),
    90.00:  (0, 1, 1),
    22.62:  (12, 5, 13),
    67.38:  (5, 12, 13),
}


def azimuth_frame(diff_axis: str, azimuth_deg: float):
    """
    The pinion may sit anywhere along the circumference of the crown: the
    engagement stays perpendicular, only the whole assembly turns. This gives
    the orientation and the direction vector that go with it.

    The freedom is continuous for the gears but quantized for the bearings: a
    bearing at distance d and angle theta lands at (d cos, d sin) and both have
    to fall on the grid. That is only possible at Pythagorean angles.
    """
    ax = "xyz".index(diff_axis)
    perp = [i for i in range(3) if i != ax]
    t = np.radians(azimuth_deg)
    direction = np.zeros(3)
    direction[perp[0]] = np.cos(t)
    direction[perp[1]] = np.sin(t)

    nearest = min(BUILDABLE_AZIMUTHS, key=lambda a: abs(a - azimuth_deg))
    err = abs(nearest - azimuth_deg)
    legs = BUILDABLE_AZIMUTHS[nearest]
    return {
        "direction": direction,
        "buildable": err < 0.01,
        "nearest_buildable_deg": nearest,
        "error_deg": err,
        "legs": legs,
        "reach_studs": legs[2],
        "note": ("lands on the grid" if err < 0.01 else
                 f"NOT buildable: nearest is {nearest:.2f} deg "
                 f"({legs[0]}-{legs[1]}-{legs[2]}), {err:.2f} deg off"),
    }


def orient_for_hole_axis(part: str, world_axis) -> np.ndarray:
    """
    Orientation putting a part's hole along the requested world axis. Takes the
    hole axis from the LDCad shadow library instead of guessing at it.
    """
    got = snap.rotation_axis(part)
    if got is None:
        raise ValueError(f"{part}: no shadow data, hole axis unknown")
    src = got[0] / np.linalg.norm(got[0])
    dst = np.asarray(world_axis, dtype=float)
    dst = dst / np.linalg.norm(dst)
    v = np.cross(src, dst)
    c = float(np.dot(src, dst))
    if np.linalg.norm(v) < 1e-9:
        return np.eye(3) if c > 0 else -np.eye(3)
    vx = np.array([[0, -v[2], v[1]], [v[2], 0, -v[0]], [-v[1], v[0], 0]])
    return np.eye(3) + vx + vx @ vx * (1 / (1 + c))


@dataclass
class Part:
    ldraw_name: str
    pos: np.ndarray                      # LDU
    orient: np.ndarray = field(default_factory=lambda: IDENTITY.copy())
    color: int = 71
    label: str = ""
    teeth: int | None = None             # set for gears
    shaft: str | None = None             # name of the shaft it sits on

    def __post_init__(self):
        self.pos = np.asarray(self.pos, dtype=float)
        self.orient = np.asarray(self.orient, dtype=float)

    @property
    def geo(self) -> ldraw.PartGeometry:
        return ldraw.geometry(self.ldraw_name)

    @property
    def native_axis(self) -> tuple[np.ndarray, str]:
        """Rotation axis in the part's own frame, plus where it came from."""
        got = snap.rotation_axis(self.ldraw_name)
        if got is not None:
            return got
        idx, conf = infer_native_axis(self.geo)
        v = np.zeros(3); v[idx] = 1.0
        return v, f"bounding box heuristic ({conf})"

    @property
    def axis_vec(self) -> np.ndarray:
        v, _ = self.native_axis
        return self.orient @ v

    def world_verts(self) -> np.ndarray:
        return self.geo.verts @ self.orient.T + self.pos

    def world_bbox(self):
        w = self.world_verts()
        return w.min(axis=0), w.max(axis=0)


@dataclass
class Finding:
    level: str      # OK / WARN / FAIL
    check: str
    detail: str


class Model:
    def __init__(self, name: str):
        self.name = name
        self.parts: list[Part] = []
        self.declared_meshes: list[tuple[int, int]] = []
        self.expected: list[tuple[str, str]] = []
        self.verified_bevels: dict[tuple[str, str], str] = {}

    def add(self, part: Part) -> int:
        self.parts.append(part)
        return len(self.parts) - 1

    def mesh(self, a: int, b: int):
        self.declared_meshes.append((a, b))

    # ---------- checks ----------

    def check_axis_confidence(self) -> list[Finding]:
        out = []
        for p in self.parts:
            if p.teeth is None:
                continue          # axles and beams need no rotation axis
            _, src = p.native_axis
            if src.startswith("LDCad"):
                out.append(Finding("OK", "axis direction",
                                   f"{p.label or p.ldraw_name}: {src}"))
            else:
                out.append(Finding("WARN", "axis direction",
                                   f"{p.label or p.ldraw_name}: no shadow data, "
                                   f"fell back on {src} — verify this"))
        return out

    def check_phase(self) -> list[Finding]:
        """Which tooth phase each gear needs in order to mesh."""
        out = []
        for a, b in self.declared_meshes:
            pa, pb = self.parts[a], self.parts[b]
            if pa.teeth is None or pb.teeth is None:
                continue
            va, vb = pa.axis_vec, pb.axis_vec
            if abs(abs(float(np.dot(va, vb))) - 1.0) > 1e-6:
                continue                      # bevel: phase not derivable this way
            d = pb.pos - pa.pos
            perp = d - np.dot(d, va) * va
            if np.linalg.norm(perp) < 1e-6:
                continue
            try:
                r = teeth.mesh_phase(pa.ldraw_name, pa.teeth, pa.native_axis[0],
                                     pb.ldraw_name, pb.teeth, pb.native_axis[0], perp)
            except Exception as exc:
                out.append(Finding("WARN", "tooth phase",
                                   f"{pa.label}/{pb.label}: phase not derivable ({exc})"))
                continue
            worst = min(r["sharpness_a"], r["sharpness_b"])
            lvl = "OK" if worst > 0.45 else "WARN"
            out.append(Finding(
                lvl, "tooth phase",
                f"{pa.label}: rotate {r['rot_a_deg']:.1f} deg about its own axis "
                f"(pitch {r['pitch_a_deg']:.1f}); {pb.label}: rotate {r['rot_b_deg']:.1f} deg "
                f"(pitch {r['pitch_b_deg']:.1f}). Tooth detection {worst:.2f}"))
        return out

    def check_axle_phase_conflicts(self) -> list[Finding]:
        """
        Two meshing gears on the same shaft: their relative phase is fixed, so
        you can rarely set both engagements perfectly at the same time.
        """
        out = []
        by_shaft: dict[str, list[Part]] = {}
        meshed_idx = {i for m in self.declared_meshes for i in m}
        for i, p in enumerate(self.parts):
            if p.shaft and p.teeth is not None and i in meshed_idx:
                by_shaft.setdefault(p.shaft, []).append(p)
        for shaft, ps in by_shaft.items():
            if len(ps) > 1:
                names = ", ".join(x.label or x.ldraw_name for x in ps)
                out.append(Finding(
                    "WARN", "tooth phase",
                    f"shaft '{shaft}' carries {len(ps)} meshing gears ({names}). "
                    f"Their relative orientation is fixed, so setting both engagements "
                    f"exactly is impossible — backlash has to absorb the difference."))
        return out

    def check_meshes(self) -> list[Finding]:
        out = []
        for a, b in self.declared_meshes:
            pa, pb = self.parts[a], self.parts[b]
            if pa.teeth is None or pb.teeth is None:
                out.append(Finding("WARN", "mesh", f"{pa.label}/{pb.label}: tooth count missing, skipped"))
                continue

            va, vb = pa.axis_vec, pb.axis_vec
            parallel = abs(abs(float(np.dot(va, vb))) - 1.0) < 1e-6

            if not parallel:
                key = tuple(sorted((pa.label, pb.label)))
                if key in self.verified_bevels:
                    out.append(Finding(
                        "OK", "mesh",
                        f"{pa.label}/{pb.label}: bevel, manually verified "
                        f"({self.verified_bevels[key]})"))
                else:
                    out.append(Finding(
                        "FAIL", "mesh",
                        f"{pa.label}/{pb.label}: BEVEL ENGAGEMENT, POSITION NOT "
                        f"VERIFIED. Not derivable from geometry. Measure it physically "
                        f"or let Stud.io snap the two parts together, and put the "
                        f"measurement in verified_bevels. Until then this model is wrong."))
                continue

            # distance measured perpendicular to the shared axis
            d = pb.pos - pa.pos
            perp = d - np.dot(d, va) * va
            actual = float(np.linalg.norm(perp))
            want = (pa.teeth + pb.teeth) * PITCH_LDU_PER_TOOTH
            err = actual - want

            if abs(err) < 0.5:
                out.append(Finding(
                    "OK", "mesh",
                    f"{pa.label} ({pa.teeth}t) / {pb.label} ({pb.teeth}t): "
                    f"{actual:.1f} LDU = {actual/LDU_STUD:.3f} stud ✓"))
            else:
                out.append(Finding(
                    "FAIL", "mesh",
                    f"{pa.label} ({pa.teeth}t) / {pb.label} ({pb.teeth}t): "
                    f"{actual:.1f} LDU, should be {want:.1f} LDU "
                    f"({want/LDU_STUD:.3f} stud). Deviation {err:+.1f} LDU."))
        return out

    def check_grid(self) -> list[Finding]:
        out = []
        for p in self.parts:
            for i, ax in enumerate("XYZ"):
                v = p.pos[i]
                if abs(v / 10.0 - round(v / 10.0)) > 1e-6:
                    out.append(Finding(
                        "WARN", "grid",
                        f"{p.label or p.ldraw_name}: {ax}={v:.2f} LDU does not lie on a "
                        f"half stud (10 LDU). Deliberate? Otherwise it cannot be built."))
        return out

    def _is_bearing(self, pa: Part, pb: Part) -> bool:
        """
        Does an axle pass through a hole of the other part? Then the contact is
        wanted, not an error. Hole position follows from the shadow axis plus
        the fixed LEGO pitch of 20 LDU along the length of the part.
        """
        for axle, host in ((pa, pb), (pb, pa)):
            try:
                haxis, src = host.native_axis
            except Exception:
                continue
            if not src.startswith("LDCad"):
                continue
            hole_axis_w = host.orient @ haxis
            axle_axis_w = axle.axis_vec
            if abs(abs(float(np.dot(hole_axis_w, axle_axis_w))) - 1.0) > 1e-3:
                continue
            # convert the shaft line into the host part's local frame
            rel = host.orient.T @ (axle.pos - host.pos)
            perp = rel - np.dot(rel, haxis) * haxis
            d = np.linalg.norm(perp)
            # holes lie at multiples of 20 LDU along the length direction
            if d < 2.0 or abs(d / 20.0 - round(d / 20.0)) < 0.12:
                return True
        return False

    def check_collisions(self, tol: float = 1.0) -> list[Finding]:
        """
        Real intersection at triangle level through FCL. Bounding boxes gave
        too many false reports: an axle passing through a hole always
        overlapped.
        """
        import trimesh

        out = []
        meshed = {tuple(sorted(m)) for m in self.declared_meshes}

        mgr = trimesh.collision.CollisionManager()
        idx_by_name = {}
        for i, p in enumerate(self.parts):
            mesh = p.geo.mesh()
            if mesh is None:
                continue
            mm = mesh.copy()
            T = np.eye(4); T[:3, :3] = p.orient; T[:3, 3] = p.pos
            mm.apply_transform(T)
            key = f"{i}"
            mgr.add_object(key, mm)
            idx_by_name[key] = i

        hit, pairs = mgr.in_collision_internal(return_names=True)
        if not hit:
            out.append(Finding("OK", "collision", "no intersections found (FCL)"))
            return out

        for a, b in pairs:
            i, j = idx_by_name[a], idx_by_name[b]
            pa, pb = self.parts[i], self.parts[j]
            if tuple(sorted((i, j))) in meshed:
                continue                       # meshing gears do touch
            if pa.shaft and pa.shaft == pb.shaft:
                continue                       # same shaft
            names = {(pa.label, pb.label), (pb.label, pa.label)}
            if names & set(self.expected):
                continue
            if self._is_bearing(pa, pb):
                out.append(Finding("OK", "collision",
                                   f"{pa.label} through {pb.label}: shaft bearing, wanted contact"))
                continue
            out.append(Finding(
                "FAIL", "collision",
                f"{pa.label or pa.ldraw_name} intersects {pb.label or pb.ldraw_name} "
                f"— a real intersection, not just a bounding box"))
        if not any(f.level == "FAIL" for f in out):
            out.append(Finding("OK", "collision", "every contact is expected (FCL)"))
        return out

    def solve_phases(self) -> list[str]:
        """
        Compute the tooth phase each engagement needs and apply it to the
        orientation of the gears. A shaft already carrying a gear that has been
        set is not rotated again: that phase is fixed by then.
        """
        log, locked = [], set()
        for a, b in self.declared_meshes:
            pa, pb = self.parts[a], self.parts[b]
            if pa.teeth is None or pb.teeth is None:
                continue
            va, vb = pa.axis_vec, pb.axis_vec
            if abs(abs(float(np.dot(va, vb))) - 1.0) > 1e-6:
                continue
            d = pb.pos - pa.pos
            perp = d - np.dot(d, va) * va
            if np.linalg.norm(perp) < 1e-6:
                continue
            r = teeth.mesh_phase(pa.ldraw_name, pa.teeth, pa.native_axis[0],
                                 pb.ldraw_name, pb.teeth, pb.native_axis[0], perp)
            for part, key, deg in ((pa, a, r["rot_a_deg"]), (pb, b, r["rot_b_deg"])):
                if part.shaft in locked:
                    log.append(f"  {part.label}: phase skipped, shaft "
                               f"'{part.shaft}' has already been set")
                    continue
                axis_local, _ = part.native_axis
                idx = int(np.argmax(np.abs(axis_local)))
                part.orient = part.orient @ rot("xyz"[idx], deg)
                if part.shaft:
                    locked.add(part.shaft)
                log.append(f"  {part.label}: {deg:+.2f} deg about {'XYZ'[idx]}")
        return log

    def run_checks(self) -> list[Finding]:
        return (self.check_axis_confidence() + self.check_meshes()
                + self.check_phase() + self.check_axle_phase_conflicts()
                + self.check_grid() + self.check_collisions())

    # ---------- export ----------

    def to_ldr(self) -> str:
        lines = [f"0 {self.name}", "0 Name: model.ldr",
                 "0 Author: brickcheck", "0 !LDRAW_ORG Model", ""]
        for p in self.parts:
            m = p.orient.flatten()
            x, y, z = p.pos
            nums = " ".join(f"{v:g}" for v in [x, y, z, *m])
            if p.label:
                lines.append(f"0 // {p.label}")
            lines.append(f"1 {p.color} {nums} {p.ldraw_name}")
        lines.append("0")
        return "\n".join(lines)
