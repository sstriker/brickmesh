// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker
//
// A small WebGL viewer for a built model.
//
// Written rather than pulled in, because the page fetches nothing outside its
// own directory and a general 3D library is several hundred kilobytes to draw
// what is here: one buffer of triangles, flat shaded, that the user can turn.
//
// Double-sided on purpose. LDraw parts are open shells with inconsistent
// winding — the differential alone has 835 boundary loops, see
// docs/findings.md — so back-face culling makes a model look full of holes.

"use strict";

// A plain script, like the rest of the page: one global, loaded in order.

// GROUPS has to match calc.DrawGroups. A mat4 costs four uniform vectors and
// WebGL 1 promises only 128, so this is not a number to raise without counting.
const GROUPS = 24;

const VERTEX_SHADER = `
  attribute vec3 position;
  attribute vec3 normal;
  attribute vec3 colour;
  attribute float flagged;
  attribute float group;
  uniform mat4 modelView;
  uniform mat4 projection;
  uniform mat4 groupXf[${GROUPS}];
  varying vec3 vNormal;
  varying vec3 vColour;
  varying float vFlagged;
  void main() {
    // Index 0 is the identity and never written, so a part in no group costs
    // the same lookup as one in a group and needs no branch.
    mat4 xf = groupXf[int(group)];
    vNormal = mat3(modelView) * mat3(xf) * normal;
    vColour = colour;
    vFlagged = flagged;
    gl_Position = projection * modelView * xf * vec4(position, 1.0);
  }
`;

const FRAGMENT_SHADER = `
  precision mediump float;
  varying vec3 vNormal;
  varying vec3 vColour;
  varying float vFlagged;
  void main() {
    // Lit from the camera, and from below at a fraction, so the underside of a
    // gear is dark but not black. abs() is the double-sided part: a face whose
    // winding points away is still a face.
    vec3 n = normalize(vNormal);
    float key = abs(n.z);
    float fill = abs(dot(n, normalize(vec3(-0.4, 0.7, 0.5))));
    vec3 base = vColour;
    // A part a finding is about is pulled towards a warning colour rather than
    // replaced by one, so it still reads as the part it is.
    base = mix(base, vec3(1.0, 0.35, 0.1), vFlagged * 0.85);
    gl_FragColor = vec4(base * (0.35 + 0.5 * key + 0.25 * fill), 1.0);
  }
`;

function createViewer(canvas) {
  const gl = canvas.getContext("webgl", { antialias: true, alpha: false });
  if (!gl) return null;

  const program = link(gl, VERTEX_SHADER, FRAGMENT_SHADER);
  gl.useProgram(program);
  const attr = {
    position: gl.getAttribLocation(program, "position"),
    normal: gl.getAttribLocation(program, "normal"),
    colour: gl.getAttribLocation(program, "colour"),
    flagged: gl.getAttribLocation(program, "flagged"),
    group: gl.getAttribLocation(program, "group"),
  };
  const uniform = {
    modelView: gl.getUniformLocation(program, "modelView"),
    projection: gl.getUniformLocation(program, "projection"),
    groupXf: gl.getUniformLocation(program, "groupXf[0]"),
  };
  const buffer = gl.createBuffer();

  gl.enable(gl.DEPTH_TEST);
  gl.disable(gl.CULL_FACE); // see above: open shells, mixed winding

  const view = {
    vertices: 0,
    centre: [0, 0, 0],
    radius: 100,
    yaw: -0.6,
    pitch: -0.5,
    distance: 400,
    // One transform per group, flat, as WebGL wants them. All identity until
    // something animates: a model that is not moving still draws through the
    // same path, which is one path to get right rather than two.
    groupXf: identityGroups(),
  };

  // load takes the buffer the worker sent: eleven floats a vertex — position,
  // normal, colour, whether a finding is about this part, and which animation
  // group it belongs to.
  function load(bytes) {
    const data = new Float32Array(bytes);
    const stride = 11;
    view.vertices = data.length / stride;

    let lo = [Infinity, Infinity, Infinity];
    let hi = [-Infinity, -Infinity, -Infinity];
    for (let i = 0; i < data.length; i += stride) {
      for (let k = 0; k < 3; k++) {
        lo[k] = Math.min(lo[k], data[i + k]);
        hi[k] = Math.max(hi[k], data[i + k]);
      }
    }
    view.centre = lo.map((v, k) => (v + hi[k]) / 2);
    view.radius = Math.max(1, Math.hypot(hi[0] - lo[0], hi[1] - lo[1], hi[2] - lo[2]) / 2);
    view.distance = view.radius * 2.6;

    gl.bindBuffer(gl.ARRAY_BUFFER, buffer);
    gl.bufferData(gl.ARRAY_BUFFER, data, gl.STATIC_DRAW);
    draw();
  }

  function draw() {
    const width = canvas.clientWidth || 1;
    const height = canvas.clientHeight || 1;
    const scale = Math.min(window.devicePixelRatio || 1, 2);
    if (canvas.width !== width * scale || canvas.height !== height * scale) {
      canvas.width = width * scale;
      canvas.height = height * scale;
    }
    gl.viewport(0, 0, canvas.width, canvas.height);
    // Matches the page rather than a fixed colour, so the model does not sit in
    // a white box on a dark page.
    const paper = getComputedStyle(canvas).backgroundColor.match(/[\d.]+/g) || [255, 255, 255];
    gl.clearColor(paper[0] / 255, paper[1] / 255, paper[2] / 255, 1);
    gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);
    if (!view.vertices) return;

    gl.bindBuffer(gl.ARRAY_BUFFER, buffer);
    const size = 11 * 4;
    for (const [name, offset] of [["position", 0], ["normal", 12], ["colour", 24]]) {
      gl.enableVertexAttribArray(attr[name]);
      gl.vertexAttribPointer(attr[name], 3, gl.FLOAT, false, size, offset);
    }
    gl.enableVertexAttribArray(attr.flagged);
    gl.vertexAttribPointer(attr.flagged, 1, gl.FLOAT, false, size, 36);
    gl.enableVertexAttribArray(attr.group);
    gl.vertexAttribPointer(attr.group, 1, gl.FLOAT, false, size, 40);
    gl.uniformMatrix4fv(uniform.groupXf, false, view.groupXf);
    gl.uniformMatrix4fv(uniform.projection, false,
      perspective(0.9, width / height, view.radius / 50, view.radius * 20));
    gl.uniformMatrix4fv(uniform.modelView, false, orbit(view));
    gl.drawArrays(gl.TRIANGLES, 0, view.vertices);
  }

  // Dragging turns it; the wheel, a pinch, or the keys move in and out.
  // Nothing else: this is for looking at what came out, not for building in.
  //
  // Four ways in and out rather than one, because the first was a wheel and
  // only a wheel. Nothing on the page said so, and the canvas sets
  // touch-action: none — so on a touchscreen there was no way to zoom at all,
  // and the page did not admit it.

  // zoomBy multiplies the distance and redraws, which is what every one of
  // them ends up doing.
  function zoomBy(factor) {
    view.distance = clamp(view.distance * factor,
      view.radius * 0.35, view.radius * 12);
    draw();
  }

  // The pointers currently down, by id. One turns the model; two pinch.
  const down = new Map();
  let pinch = 0;

  const spread = () => {
    const [a, b] = [...down.values()];
    return Math.hypot(a.x - b.x, a.y - b.y);
  };

  canvas.addEventListener("pointerdown", (e) => {
    down.set(e.pointerId, { x: e.clientX, y: e.clientY });
    // Capture so a drag that leaves the canvas keeps turning it — but not at
    // the cost of the whole handler. It throws for a pointer the browser does
    // not consider active, and then nothing below runs and the view is stuck.
    try {
      canvas.setPointerCapture(e.pointerId);
    } catch {
      // No capture, which only costs us drags that wander off the canvas.
    }
    if (down.size === 2) pinch = spread();
  });
  canvas.addEventListener("pointermove", (e) => {
    const was = down.get(e.pointerId);
    if (!was) return;
    down.set(e.pointerId, { x: e.clientX, y: e.clientY });
    if (down.size >= 2) {
      // Two fingers: the distance between them is the zoom, and neither one
      // turns the model while that is happening.
      const now = spread();
      if (pinch > 0 && now > 0) zoomBy(pinch / now);
      pinch = now;
      return;
    }
    view.yaw += (e.clientX - was.x) * 0.01;
    view.pitch = clamp(view.pitch + (e.clientY - was.y) * 0.01, -1.5, 1.5);
    draw();
  });
  for (const end of ["pointerup", "pointercancel", "pointerleave"]) {
    canvas.addEventListener(end, (e) => {
      down.delete(e.pointerId);
      if (down.size < 2) pinch = 0;
    });
  }
  canvas.addEventListener("wheel", (e) => {
    e.preventDefault();
    // The raw delta, not its sign: a trackpad sends many small ones and a
    // mouse a few large, and stepping by a fixed amount makes one crawl and
    // the other jump.
    zoomBy(Math.exp(clamp(e.deltaY, -100, 100) * 0.0015));
  }, { passive: false });

  // Keys, for anyone not using a pointer at all. The canvas has to be
  // focusable to receive them, which is what tabIndex is for here.
  canvas.tabIndex = 0;
  canvas.addEventListener("keydown", (e) => {
    const step = { "+": 1 / 1.2, "=": 1 / 1.2, "-": 1.2, _: 1.2 }[e.key];
    if (step) {
      e.preventDefault();
      zoomBy(step);
      return;
    }
    if (e.key === "0") {
      e.preventDefault();
      reset();
    }
  });

  // reset puts it back where load left it, since it is easy to zoom into the
  // middle of a gearbox and lose the thread.
  function reset() {
    view.yaw = -0.6;
    view.pitch = -0.5;
    view.distance = view.radius * 2.6;
    draw();
  }

  window.addEventListener("resize", draw);

  // setGroups takes one 16-float transform per group, in the order the build
  // reported, and draws. Index 0 in the shader is the identity for parts in no
  // group, so the first group given lands at 1.
  function setGroups(transforms) {
    const flat = identityGroups();
    for (let g = 0; g < transforms.length && g + 1 < GROUPS; g++) {
      flat.set(transforms[g], (g + 1) * 16);
    }
    view.groupXf = flat;
    draw();
  }

  // rest puts every group back to the identity, for when an animation stops.
  function rest() {
    view.groupXf = identityGroups();
    draw();
  }

  return { load, draw, zoomBy, reset, setGroups, rest };
}

// identityGroups is the transform table with nothing moved.
function identityGroups() {
  const flat = new Float32Array(GROUPS * 16);
  for (let g = 0; g < GROUPS; g++) {
    flat[g * 16] = flat[g * 16 + 5] = flat[g * 16 + 10] = flat[g * 16 + 15] = 1;
  }
  return flat;
}

function clamp(v, lo, hi) { return Math.max(lo, Math.min(hi, v)); }

function orbit(view) {
  const cp = Math.cos(view.pitch), sp = Math.sin(view.pitch);
  const cy = Math.cos(view.yaw), sy = Math.sin(view.yaw);
  // Rotate about Y then X, then step back along Z; column-major for WebGL.
  const m = [
    cy, sp * sy, -cp * sy, 0,
    0, cp, sp, 0,
    sy, -sp * cy, cp * cy, 0,
    0, 0, 0, 1,
  ];
  const c = view.centre;
  m[12] = -(m[0] * c[0] + m[4] * c[1] + m[8] * c[2]);
  m[13] = -(m[1] * c[0] + m[5] * c[1] + m[9] * c[2]);
  m[14] = -(m[2] * c[0] + m[6] * c[1] + m[10] * c[2]) - view.distance;
  return m;
}

function perspective(fov, aspect, near, far) {
  const f = 1 / Math.tan(fov / 2);
  return [
    f / aspect, 0, 0, 0,
    0, f, 0, 0,
    0, 0, (far + near) / (near - far), -1,
    0, 0, (2 * far * near) / (near - far), 0,
  ];
}

function link(gl, vertexSource, fragmentSource) {
  const program = gl.createProgram();
  for (const [type, source] of [[gl.VERTEX_SHADER, vertexSource],
                                [gl.FRAGMENT_SHADER, fragmentSource]]) {
    const shader = gl.createShader(type);
    gl.shaderSource(shader, source);
    gl.compileShader(shader);
    if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
      throw new Error(gl.getShaderInfoLog(shader));
    }
    gl.attachShader(program, shader);
  }
  gl.linkProgram(program);
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
    throw new Error(gl.getProgramInfoLog(program));
  }
  return program;
}
