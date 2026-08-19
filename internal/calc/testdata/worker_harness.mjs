// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker
//
// Runs web/worker.js as written, with just enough of a worker around it: the
// globals it expects, importScripts, and a fetch that reads a directory laid
// out like the site.
//
// Used by wasm_test.go. The worker is what the page actually talks to, and node
// has neither a DOM Worker nor importScripts — so rather than leave it
// untested, the environment is supplied here and the real file is run in it.
import fs from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { webcrypto } from "node:crypto";
import { createRequire } from "node:module";

const [dir, specFile] = process.argv.slice(2);

const sandbox = {
  console, setTimeout, clearTimeout, performance, TextEncoder, TextDecoder,
  crypto: webcrypto, WebAssembly, URL, Uint8Array, Promise, JSON, Object, Array,
  String, Number, Math, Error, fs, require: createRequire(import.meta.url),
};
sandbox.self = sandbox;
sandbox.globalThis = sandbox;

// The worker fetches relative to itself; here that is the site directory.
sandbox.fetch = async (rel) => {
  const file = path.join(dir, rel);
  if (!fs.existsSync(file)) {
    return { ok: false, status: 404, statusText: "not found" };
  }
  const buf = fs.readFileSync(file);
  return {
    ok: true, status: 200,
    clone() { return this; },
    arrayBuffer: async () => buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength),
  };
};
sandbox.importScripts = (rel) => {
  vm.runInContext(fs.readFileSync(path.join(dir, rel), "utf8"), sandbox, { filename: rel });
};

const answers = [];
sandbox.postMessage = (msg) => answers.push(msg);

vm.createContext(sandbox);
vm.runInContext(fs.readFileSync(path.join(dir, "worker.js"), "utf8"), sandbox,
  { filename: "worker.js" });

const spec = fs.readFileSync(specFile, "utf8");
const started = Date.now();
await sandbox.onmessage({ data: { id: 1, kind: "build", spec, animate: true } });

const answer = answers.find((m) => m.answer);
const progress = answers.filter((m) => m.progress).map((m) => m.progress);
if (!answer) throw new Error("the worker never answered");
const a = answer.answer;
if (a.error) throw new Error(a.error);
console.log(`${path.basename(specFile)}: ok=${a.ok} parts=${a.parts} ` +
  `ldr=${a.ldr.length}B lua=${(a.lua || "").length}B ` +
  `in ${((Date.now() - started) / 1000).toFixed(1)}s`);
console.log(`  progress reported: ${progress.join(" -> ")}`);
console.log(`  answer id matches the question: ${answer.id === 1}`);
