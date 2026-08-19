// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker
//
// Runs web/view.js's camera maths and checks that a model ends up in front of
// the camera and on the screen.
//
// A transposed or mis-ordered matrix throws every vertex behind the eye or off
// to infinity, and the page shows a blank canvas and no error at all — there is
// nothing to catch it in the browser. So it is caught here.
import fs from "node:fs";
import path from "node:path";
import vm from "node:vm";

const [dir] = process.argv.slice(2);

// view.js reaches for the DOM only when a viewer is constructed; the matrix
// helpers it is being asked about are plain functions.
const sandbox = { Math, Infinity, console, Number, Array, Object };
sandbox.window = sandbox;
sandbox.getComputedStyle = () => ({ backgroundColor: "rgb(255,255,255)" });
vm.createContext(sandbox);
vm.runInContext(fs.readFileSync(path.join(dir, "view.js"), "utf8"), sandbox,
  { filename: "view.js" });

// Column-major, the way WebGL wants it: out = M * v.
function apply(m, v) {
  const out = [0, 0, 0, 0];
  for (let r = 0; r < 4; r++) {
    for (let c = 0; c < 4; c++) out[r] += m[c * 4 + r] * v[c];
  }
  return out;
}

const radius = 100;
const centre = [30, -20, 15];
let worst = { depth: Infinity, ndc: 0 };

// Every angle the viewer can be turned to, and the corners of the model at each.
for (let yaw = -Math.PI; yaw <= Math.PI; yaw += Math.PI / 6) {
  for (let pitch = -1.5; pitch <= 1.5; pitch += 0.25) {
    const view = { centre, radius, yaw, pitch, distance: radius * 2.6 };
    const mv = sandbox.orbit(view);
    const proj = sandbox.perspective(0.9, 4 / 3, radius / 50, radius * 20);

    // The centre of the model must land dead ahead, at the middle of the screen.
    const mid = apply(proj, apply(mv, [...centre, 1]));
    if (Math.abs(mid[0] / mid[3]) > 1e-6 || Math.abs(mid[1] / mid[3]) > 1e-6) {
      throw new Error(`the model centre is off-screen at yaw ${yaw}: ` +
        `${mid[0] / mid[3]}, ${mid[1] / mid[3]}`);
    }

    // Points on the bounding sphere, which is what `radius` is: half the
    // diagonal of the model's box, so no vertex lies outside it.
    for (let a = 0; a < Math.PI * 2; a += Math.PI / 8) {
      for (let b = -Math.PI / 2; b <= Math.PI / 2; b += Math.PI / 8) {
        {
          const p = [
            centre[0] + radius * Math.cos(b) * Math.cos(a),
            centre[1] + radius * Math.sin(b),
            centre[2] + radius * Math.cos(b) * Math.sin(a), 1];
          const eye = apply(mv, p);
          // In front of the camera, which looks down -z.
          if (eye[2] >= 0) {
            throw new Error(`a corner is behind the camera at yaw ${yaw}, ` +
              `pitch ${pitch}: eye z = ${eye[2]}`);
          }
          worst.depth = Math.min(worst.depth, -eye[2]);
          const clip = apply(proj, eye);
          if (clip[3] <= 0) throw new Error(`w = ${clip[3]}`);
          worst.ndc = Math.max(worst.ndc,
            Math.abs(clip[0] / clip[3]), Math.abs(clip[1] / clip[3]));
        }
      }
    }
    // Turning must not stretch the model: the same rotation applied to a unit
    // vector has to come back a unit vector.
    const len = Math.hypot(...apply(mv, [1, 0, 0, 0]).slice(0, 3));
    if (Math.abs(len - 1) > 1e-9) {
      throw new Error(`the view matrix scales by ${len} at yaw ${yaw}`);
    }
  }
}

// The whole model has to be on screen at the distance the viewer picks, or the
// first thing a user sees is a gearbox with its ends cut off.
if (worst.ndc > 1) {
  throw new Error(`the default framing crops the model: it reaches ` +
    `${worst.ndc.toFixed(2)} of the screen, where 1 is the edge`);
}
console.log(`centred at every angle; nearest point ${worst.depth.toFixed(1)} ` +
  `in front of the eye; model fills ${(worst.ndc * 100).toFixed(0)}% to the edge`);
