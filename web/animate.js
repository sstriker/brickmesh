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

  // A steady state only, for now: the gears turn at their solved ratios and
  // each ring sits where that state puts it. The engine also describes the
  // SHIFT — the ring travelling between gears, with the shafts it feeds held
  // still while it is in neither — and that is the more interesting picture.
  // It is segmented rather than steady, so it needs its own path through here.
  const states = anim.animations
    .filter((a) => a.turning || a.sliding || a.swinging)
    .map((a) => a.name);

  function at(name, turns) {
    const a = anim.animations.find((x) => x.name === name) || anim.animations[0];
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
      // Where it sits, against where the model was built: the parts are baked
      // at the engaged end, so at 0 there is nothing to do.
      const e = toView(s.engaged), d = toView(s.disengaged);
      const f = s.at || 0;
      const move = mat4Move([(d[0] - e[0]) * f, (d[1] - e[1]) * f, (d[2] - e[2]) * f]);
      // A ring turns with its shaft as well as sliding along it. A catch on a
      // shaft-parallel axle does not, and says so with a speed of zero.
      if (s.speed) {
        const spin = mat4Turn(toView(s.axis), -turns * s.speed * 2 * Math.PI, e);
        spin[12] += move[12]; spin[13] += move[13]; spin[14] += move[14];
        put(s.group, spin);
      } else {
        put(s.group, move);
      }
    }
    for (const w of a.swinging || []) {
      // Half either side of rest, and `at` says how far across.
      const half = (w.clearAngle - w.engagedAngle) || 0;
      const angle = (w.engagedAngle + half * (w.at || 0)) * Math.PI / 180;
      put(w.group, mat4Turn(toView(w.axis), -angle, toView(w.pivot)));
    }
    return out;
  }

  return { states, at };
}
