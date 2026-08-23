// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker
//
// The animation the engine solved, as transforms a shader can use.
//
// The engine already describes every motion: which group turns about which
// axis at what ratio, which slides between which two points. It renders that
// twice — as Lua for LDCad, and as the data this reads. One description, two
// renderings, so the page cannot drift from the file you open in LDCad.
//
// Nothing here decides anything about the mechanism. If the page shows a shaft
// turning the wrong way, the ratio is wrong in the engine and the .lua is wrong
// with it.

"use strict";

// toView moves a point from LDraw's coordinates into the ones the draw buffer
// uses, which is a single sign: LDraw has +Y downward and the camera does not.
function toView(v) {
  return [v.X, -v.Y, v.Z];
}

// mat4Identity, column-major, as WebGL wants it.
function mat4Identity() {
  return [1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1];
}

// mat4Turn is a rotation of `angle` radians about `axis` through the point
// `through`, both already in view coordinates.
function mat4Turn(axis, angle, through) {
  const len = Math.hypot(axis[0], axis[1], axis[2]);
  if (len < 1e-12) return mat4Identity();
  const [x, y, z] = [axis[0] / len, axis[1] / len, axis[2] / len];
  const c = Math.cos(angle), s = Math.sin(angle), t = 1 - c;
  // Row-major first, because that is how the formula reads.
  const r = [
    [t * x * x + c, t * x * y - s * z, t * x * z + s * y],
    [t * x * y + s * z, t * y * y + c, t * y * z - s * x],
    [t * x * z - s * y, t * y * z + s * x, t * z * z + c],
  ];
  // Turning about a line, not about the origin: move the point to the origin,
  // turn, put it back. The whole of that is a translation of p - Rp.
  const p = through;
  const tx = p[0] - (r[0][0] * p[0] + r[0][1] * p[1] + r[0][2] * p[2]);
  const ty = p[1] - (r[1][0] * p[0] + r[1][1] * p[1] + r[1][2] * p[2]);
  const tz = p[2] - (r[2][0] * p[0] + r[2][1] * p[1] + r[2][2] * p[2]);
  return [
    r[0][0], r[1][0], r[2][0], 0,
    r[0][1], r[1][1], r[2][1], 0,
    r[0][2], r[1][2], r[2][2], 0,
    tx, ty, tz, 1,
  ];
}

// mat4Move is a plain translation.
function mat4Move(d) {
  const m = mat4Identity();
  m[12] = d[0]; m[13] = d[1]; m[14] = d[2];
  return m;
}

// createAnimator turns one build's animation data into frames.
//
// `anim` is the engine's script and `groups` the group names in the order the
// draw buffer numbers them. What comes back knows the states and can be asked
// for the transforms at a moment in time.
function createAnimator(anim, groups) {
  if (!anim || !anim.animations || !groups || !groups.length) return null;

  const states = anim.animations
    .filter((a) => a.turning || a.sliding || a.swinging)
    .map((a) => a.name);

  // SHIFT is the share of a segment a ring spends moving, at the end of it. The
  // same quarter the .lua uses, and for the same reason: a ring that jumps
  // between gears between two frames has not been shown shifting at all.
  const SHIFT = 0.25;

  function at(name, turns) {
    const a = anim.animations.find((x) => x.name === name) || anim.animations[0];
    if (a.segments && a.segments.length) return walk(a, turns);
    return steady(a, turns);
  }

  // walk is the shift: a run through the states, one after another, with each
  // ring travelling near the end of its segment and every shaft it feeds
  // holding still while it is in neither gear.
  function walk(a, turns) {
    const segs = a.segments;
    const total = anim.inputTurns || 1;
    const t = ((turns / total) % 1 + 1) % 1;

    // Which segment, and how far into it. Equal shares unless it says
    // otherwise, which is all there is to go on for a box shifted by hand.
    const frac = segs.map((s) => s.fraction || 1 / segs.length);
    let seg = segs.length - 1, acc = 0;
    for (let k = 0; k < segs.length; k++) {
      if (t < acc + frac[k]) { seg = k; break; }
      acc += frac[k];
    }
    const u = clamp01((t - acc) / (frac[seg] || 1));
    const f = u > 1 - SHIFT ? (u - (1 - SHIFT)) / SHIFT : 0;
    const nxt = Math.min(seg + 1, segs.length - 1);

    const out = groups.map(() => mat4Identity());
    const put = (group, m) => {
      const i = groups.indexOf(group);
      if (i >= 0) out[i] = m;
    };

    // How far a group has turned by now, skipping the part of each segment its
    // drive was cut. Every completed segment in full, then this one so far.
    const angleHolding = (per, holds) => {
      let a2 = 0;
      for (let k = 0; k < seg; k++) {
        const part = holds && holds[k] ? 1 - SHIFT : 1;
        a2 += per[k] * part * frac[k] * total;
      }
      let held = u;
      if (holds && holds[seg] && held > 1 - SHIFT) held = 1 - SHIFT;
      return a2 + per[seg] * held * frac[seg] * total;
    };

    (a.turning || []).forEach((t0, i) => {
      const per = segs.map((s) => (s.turning[i] || t0).speed);
      const gone = angleHolding(per, t0.holds);
      put(t0.group, mat4Turn(toView(t0.axis), -gone * 2 * Math.PI,
        toView(t0.through)));
    });
    (a.sliding || []).forEach((s0, i) => {
      const per = segs.map((s) => (s.sliding[i] || s0).speed);
      const here = (segs[seg].sliding[i] || s0).at || 0;
      const there = (segs[nxt].sliding[i] || s0).at || 0;
      const where = here + (there - here) * f;
      put(s0.group, slide(s0, where, angleHolding(per, s0.holds)));
    });
    (a.swinging || []).forEach((w0, i) => {
      const here = (segs[seg].swinging[i] || w0).at || 0;
      const there = (segs[nxt].swinging[i] || w0).at || 0;
      put(w0.group, swing(w0, here + (there - here) * f));
    });
    return out;
  }

  function steady(a, turns) {
    const out = groups.map(() => mat4Identity());
    const put = (group, m) => {
      const i = groups.indexOf(group);
      if (i >= 0) out[i] = m;
    };

    for (const t of a.turning || []) {
      // A reflection reverses the sense of a turn, and moving into view
      // coordinates is a reflection. Same axis, opposite angle.
      put(t.group, mat4Turn(toView(t.axis), -turns * t.speed * 2 * Math.PI,
        toView(t.through)));
    }
    for (const s of a.sliding || []) {
      put(s.group, slide(s, s.at || 0, turns * s.speed));
    }
    for (const w of a.swinging || []) {
      put(w.group, swing(w, w.at || 0));
    }
    return out;
  }

  return { states, at };
}

// slide places a ring: along its shaft by `where` of its travel, and turned by
// `gone` turns about that same shaft.
//
// The parts are baked at the engaged end, so where = 0 is no movement at all. A
// ring turns with its shaft as well as sliding along it; a catch on a
// shaft-parallel axle does not, and says so with a speed of zero.
function slide(s, where, gone) {
  const e = toView(s.engaged), d = toView(s.disengaged);
  const move = [(d[0] - e[0]) * where, (d[1] - e[1]) * where, (d[2] - e[2]) * where];
  if (!s.speed) return mat4Move(move);
  const m = mat4Turn(toView(s.axis), -gone * 2 * Math.PI, e);
  m[12] += move[0]; m[13] += move[1]; m[14] += move[2];
  return m;
}

// swing turns a catch to match where it has pushed its ring: between its two
// angles, `where` of the way across.
function swing(w, where) {
  const half = (w.clearAngle - w.engagedAngle) || 0;
  const angle = (w.engagedAngle + half * where) * Math.PI / 180;
  return mat4Turn(toView(w.axis), -angle, toView(w.pivot));
}

function clamp01(v) { return v < 0 ? 0 : v > 1 ? 1 : v; }
