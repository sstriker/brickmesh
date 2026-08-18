// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker
//
// Runs the compiled WebAssembly module the way the page does, and prints what
// it answers. Used by wasm_test.go to check that the answer in a browser is the
// same as the answer in Go.
//
//   node harness.mjs <dir with brickmesh.wasm and wasm_exec.js> <spec file>

import fs from "node:fs";
import path from "node:path";
import { webcrypto } from "node:crypto";
import { createRequire } from "node:module";

const [dir, specFile] = process.argv.slice(2);

globalThis.require = createRequire(import.meta.url);
globalThis.fs = fs;
globalThis.crypto = webcrypto;
globalThis.performance = performance;

await import(path.resolve(dir, "wasm_exec.js"));

const go = new globalThis.Go();
const mod = await WebAssembly.instantiate(
  fs.readFileSync(path.join(dir, "brickmesh.wasm")), go.importObject);

// go.run settles only when main returns, and main blocks forever so the module
// stays callable. Start it and wait for the flag it sets, exactly as the page
// does — awaiting go.run would hang, and not waiting calls into it too early.
go.run(mod.instance);
for (let i = 0; i < 500 && !globalThis.brickmeshReady; i++) {
  await new Promise((r) => setTimeout(r, 10));
}
if (!globalThis.brickmeshReady) throw new Error("the module never became ready");

process.stdout.write(globalThis.brickmeshCheck(fs.readFileSync(specFile, "utf8")));
