// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker
//
// The engine, off the page's thread.
//
// Placing a mechanism takes seconds — the structural search tries dozens of
// restarts and each one rasterises parts and tests them against each other —
// and WebAssembly runs on whatever thread starts it. On the page's thread that
// is a frozen tab. Here it is a message.
//
// One worker rather than one per restart, which is what docs/architecture.md
// suggests and is still the right shape for later: the restarts share nothing,
// so they split across workers whenever that is worth doing. This is the step
// that makes the page usable, not the step that makes the search fast.

"use strict";

importScripts("wasm_exec.js");

let ready = null;

// start loads the module once, and every later message waits on the same
// promise rather than starting a second one.
function start() {
  if (ready) return ready;
  ready = (async () => {
    const go = new Go();
    const response = await fetch("brickmesh.wasm");
    if (!response.ok) {
      throw new Error(`brickmesh.wasm: ${response.status} ${response.statusText}`);
    }
    let wasm;
    try {
      wasm = await WebAssembly.instantiateStreaming(response.clone(), go.importObject);
    } catch {
      wasm = await WebAssembly.instantiate(await response.arrayBuffer(), go.importObject);
    }
    // go.run settles only when the module's main returns, and this one never
    // does: a Go module whose main returns takes its exports with it. So it is
    // started, not awaited, and what we wait for is the flag main sets.
    go.run(wasm.instance);
    for (let i = 0; i < 500 && !self.brickmeshReady; i++) {
      await new Promise((r) => setTimeout(r, 10));
    }
    if (!self.brickmeshReady) throw new Error("the engine did not start");
  })();
  return ready;
}

let partsLoaded = false;

async function loadParts() {
  if (partsLoaded) return;
  const [catalog, meshes] = await Promise.all([
    bytes("data/catalog.bin"),
    bytes("data/meshes.bin"),
  ]);
  const answer = JSON.parse(self.brickmeshLoadParts(catalog, meshes));
  if (answer.error) throw new Error(answer.error);
  partsLoaded = true;
}

async function bytes(path) {
  const res = await fetch(path);
  if (!res.ok) throw new Error(`${path}: ${res.status} ${res.statusText}`);
  return new Uint8Array(await res.arrayBuffer());
}

// Each message carries an id, so an answer can be matched to its question: the
// page may well have edited the description again while this was working.
self.onmessage = async (event) => {
  const { id, kind, spec, animate, flag } = event.data;
  try {
    await start();
    if (kind === "check") {
      self.postMessage({ id, answer: JSON.parse(self.brickmeshCheck(spec)) });
      return;
    }
    self.postMessage({ id, progress: "fetching the parts" });
    await loadParts();
    self.postMessage({ id, progress: "placing the gears and finding a frame" });
    const answer = JSON.parse(self.brickmeshBuild(spec, !!animate));
    // The triangles come back as bytes and are handed over rather than copied:
    // a compound gearbox is six megabytes, and the worker has no use for it
    // afterwards.
    // A check name lights up the parts its findings are about; nothing lights
    // up the model plain.
    const drawn = answer.ldr ? self.brickmeshDraw(flag || "") : null;
    if (drawn) {
      const buffer = drawn.buffer.slice(drawn.byteOffset, drawn.byteOffset + drawn.length);
      self.postMessage({ id, answer, triangles: buffer }, [buffer]);
    } else {
      self.postMessage({ id, answer });
    }
  } catch (err) {
    self.postMessage({ id, answer: { error: String(err.message || err) } });
  }
};
