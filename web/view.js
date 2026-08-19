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

const VERTEX_SHADER = `
  attribute vec3 position;
  attribute vec3 normal;
  attribute vec3 colour;
  uniform mat4 modelView;
  uniform mat4 projection;
  varying vec3 vNormal;
  varying vec3 vColour;
  void main() {
    vNormal = mat3(modelView) * normal;
    vColour = colour;
    gl_Position = projection * modelView * vec4(position, 1.0);
  }
`;

const FRAGMENT_SHADER = `
  precision mediump float;
  varying vec3 vNormal;
  varying vec3 vColour;
  void main() {
    // Lit from the camera, and from below at a fraction, so the underside of a
    // gear is dark but not black. abs() is the double-sided part: a face whose
    // winding points away is still a face.
    vec3 n = normalize(vNormal);
    float key = abs(n.z);
    float fill = abs(dot(n, normalize(vec3(-0.4, 0.7, 0.5))));
    gl_FragColor = vec4(vColour * (0.35 + 0.5 * key + 0.25 * fill), 1.0);
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
  };
  const uniform = {
    modelView: gl.getUniformLocation(program, "modelView"),
    projection: gl.getUniformLocation(program, "projection"),
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
  };

  // load takes the buffer the worker sent: nine floats a vertex, position,
  // normal and colour interleaved.
  function load(bytes) {
    const data = new Float32Array(bytes);
    const stride = 9;
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
    const size = 9 * 4;
    for (const [name, offset] of [["position", 0], ["normal", 12], ["colour", 24]]) {
      gl.enableVertexAttribArray(attr[name]);
      gl.vertexAttribPointer(attr[name], 3, gl.FLOAT, false, size, offset);
    }
    gl.uniformMatrix4fv(uniform.projection, false,
      perspective(0.9, width / height, view.radius / 50, view.radius * 20));
    gl.uniformMatrix4fv(uniform.modelView, false, orbit(view));
    gl.drawArrays(gl.TRIANGLES, 0, view.vertices);
  }

  // Dragging turns it; the wheel moves in and out. Nothing else: this is for
  // looking at what came out, not for building in.
  let dragging = null;
  canvas.addEventListener("pointerdown", (e) => {
    dragging = { x: e.clientX, y: e.clientY };
    canvas.setPointerCapture(e.pointerId);
  });
  canvas.addEventListener("pointermove", (e) => {
    if (!dragging) return;
    view.yaw += (e.clientX - dragging.x) * 0.01;
    view.pitch = clamp(view.pitch + (e.clientY - dragging.y) * 0.01, -1.5, 1.5);
    dragging = { x: e.clientX, y: e.clientY };
    draw();
  });
  for (const end of ["pointerup", "pointercancel", "pointerleave"]) {
    canvas.addEventListener(end, () => { dragging = null; });
  }
  canvas.addEventListener("wheel", (e) => {
    e.preventDefault();
    view.distance = clamp(view.distance * (1 + Math.sign(e.deltaY) * 0.12),
      view.radius * 1.2, view.radius * 12);
    draw();
  }, { passive: false });
  window.addEventListener("resize", draw);

  return { load, draw };
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
