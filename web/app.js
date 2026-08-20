// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker
//
// The page. It asks the worker and lays out the answer; the engine is the
// WebAssembly module the worker runs, and this file deliberately contains no
// rules about gears and no waiting.

"use strict";

// Every one of these has to exist in the repository's examples directory.
// TestThePageOnlyOffersExamplesThatExist checks it, because two of them did not
// and the buttons simply 404'd — the sort of thing that is invisible until
// somebody clicks, and was invisible until a browser did.
const EXAMPLES = [
  ["reduction", "examples/reduction.json"],
  ["subtractor", "examples/subtractor.json"],
  ["2-speed gearbox", "examples/gearbox-2-speed.json"],
  ["3-speed compound", "examples/gearbox-3-speed-compound.json"],
  ["auto-shifting", "examples/gearbox-2-speed-auto.json"],
];

const specEl = document.getElementById("spec");
const buildEl = document.getElementById("build");
const downloadsEl = document.getElementById("downloads");
const canvasEl = document.getElementById("view");
const viewer = createViewer(canvasEl);

const answerEl = document.getElementById("answer");
const statusEl = document.getElementById("status");

// The last build, and which check is lit. Showing a model is not the point —
// Stud.io draws better. Showing which parts a verdict is about is.
let lastBuilt = null;
let lit = "";

// highlight redraws with one check's parts picked out. An empty check clears it.
async function highlight(check) {
  if (!viewer || !lastBuilt || !lastBuilt.ldr) return;
  lit = check === lit ? "" : check;
  // A redraw, not a rebuild: the mechanism has not changed, only which parts
  // are picked out. Re-solving a compound gearbox to recolour it would cost
  // seconds for a click.
  const { triangles } = await ask("draw", { flag: lit });
  if (triangles) {
    viewer.load(triangles);
    canvasEl.hidden = false;
  }
  render(lastBuilt);
}

// The engine lives in a worker. The page's job is to ask and to lay out the
// answer; nothing here knows about gears, and nothing here blocks.
const worker = new Worker("worker.js");
const waiting = new Map();
let nextID = 1;

worker.onmessage = (event) => {
  const { id, answer, progress, triangles } = event.data;
  const pending = waiting.get(id);
  if (!pending) return; // an answer to a question already overtaken
  if (progress !== undefined) {
    statusEl.textContent = `${progress}…`;
    return;
  }
  waiting.delete(id);
  pending.resolve({ answer, triangles });
};

worker.onerror = (event) => {
  statusEl.textContent = `the engine stopped: ${event.message}`;
};

function ask(kind, extra = {}) {
  const id = nextID++;
  return new Promise((resolve) => {
    waiting.set(id, { resolve });
    worker.postMessage({ id, kind, spec: specEl.value, ...extra });
  });
}

// A run per keystroke would be wasteful even at a millisecond a go, and the
// answer flickering as you type mid-word is worse than a beat of delay.
let pending = null;
function scheduleCheck() {
  clearTimeout(pending);
  pending = setTimeout(check, 150);
}

let latest = 0;
async function check() {
  const mine = ++latest;
  const { answer: result } = await ask("check");
  // Only the most recent question gets to draw: answers can arrive out of
  // order, and an older one overwriting a newer is a page that lags behind
  // what was typed.
  if (mine === latest) render(result);
}

function render(result) {
  answerEl.replaceChildren();

  if (result.error) {
    answerEl.append(
      verdict("broken", "This description cannot be read"),
      note(result.error),
    );
    return;
  }

  answerEl.append(result.ok
    ? verdict("pass", "This mechanism works")
    : verdict("fail", "This mechanism will not work"));

  const states = result.states || [];
  const outputs = result.outputs || [];
  if (states.length && outputs.length) {
    answerEl.append(speedTable(states, outputs));
  }
  for (const f of result.findings || []) {
    answerEl.append(finding(f));
  }
}

// One row per state, one column per output: the answer to "what does it turn
// at", which is the question most people arrive with.
function speedTable(states, outputs) {
  const table = el("table");
  const cap = el("caption");
  cap.textContent = states.length > 1
    ? "Turns of each output per turn of the input, state by state"
    : "Turns of each output per turn of the input";
  table.append(cap);

  const head = el("tr");
  head.append(el("th", states.length > 1 ? "state" : ""));
  for (const o of outputs) head.append(el("th", o));
  table.append(head);

  for (const st of states) {
    const row = el("tr");
    row.append(el("td", st.name || "—"));
    for (const o of outputs) {
      const cell = el("td");
      cell.className = "num";
      cell.textContent = st.determined && st.speeds
        ? formatRatio(st.speeds[o])
        : "not determined";
      row.append(cell);
    }
    table.append(row);
  }
  return table;
}

// A ratio reads better as a ratio. -0.333 is 1:3 the other way round, and the
// sign is which way it turns, which matters and is easy to lose.
function formatRatio(v) {
  if (v === undefined || v === null) return "—";
  if (v === 0) return "0 (does not turn)";
  const mag = Math.abs(v);
  const sense = v < 0 ? " reversed" : "";
  const inverse = 1 / mag;
  if (Math.abs(inverse - Math.round(inverse)) < 1e-6 && inverse > 1) {
    return `1:${Math.round(inverse)}${sense}`;
  }
  return `${mag.toFixed(3).replace(/0+$/, "").replace(/\.$/, "")}${sense}`;
}

function finding(f) {
  const box = el("div");
  box.className = `finding ${f.level}`;
  box.append(el("span", f.level, "level"), el("span", f.check, "check"),
             el("span", f.detail, "detail"));

  // A finding that knows which parts it is about can point at them. Clicking it
  // lights them in the model, and clicking again puts them back — which is the
  // whole reason there is a model on this page.
  if (f.parts && f.parts.length) {
    box.classList.add("points");
    if (f.check === lit) box.classList.add("lit");
    box.title = `show the ${f.parts.length} part(s) this is about`;
    box.addEventListener("click", () => highlight(f.check));
  }
  return box;
}

function verdict(kind, text) {
  const p = el("p", text);
  p.className = `verdict ${kind}`;
  return p;
}

function note(text) {
  const p = el("p", text);
  p.className = "hint";
  return p;
}

function el(tag, text, cls) {
  const node = document.createElement(tag);
  if (text !== undefined) node.textContent = text;
  if (cls) node.className = cls;
  return node;
}

// Examples come from the repository's own examples directory rather than being
// copied into this file, so they cannot drift from the ones the command line
// and the tests use.
async function loadExample(path) {
  statusEl.textContent = "loading…";
  try {
    const res = await fetch(path);
    if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
    specEl.value = (await res.text()).trimEnd();
    statusEl.textContent = "";
    check();
  } catch (err) {
    statusEl.textContent = `could not load ${path}: ${err.message}`;
  }
}

function buildExampleButtons() {
  const bar = document.getElementById("examples");
  for (const [label, path] of EXAMPLES) {
    const button = el("button", label);
    button.type = "button";
    button.addEventListener("click", () => loadExample(path));
    bar.append(button);
  }
}



// Building places the gears, frames them and writes the files. It happens in
// the worker, so the page stays alive while it does — which matters: a compound
// gearbox takes half a minute, and a frozen tab looks like a crash.
async function buildModel() {
  downloadsEl.hidden = true;
  buildEl.disabled = true;
  try {
    const { answer: built, triangles } = await ask("build", { animate: true });
    lastBuilt = built;
    statusEl.textContent = "";
    if (built.error) {
      render({ error: built.error });
      return;
    }
    render(built);
    if (built.ldr) offerDownloads(built);
    if (triangles && viewer) {
      viewer.load(triangles);
      canvasEl.hidden = false;
    }
  } finally {
    buildEl.disabled = false;
  }
}

// The two files, as things to save. Made here rather than fetched: they were
// just computed in this tab and never existed anywhere else.
function offerDownloads(built) {
  downloadsEl.replaceChildren();
  const name = (JSON.parse(specEl.value).name || "model")
    .replace(/[^a-z0-9]+/gi, "-").replace(/^-|-$/g, "").toLowerCase();

  downloadsEl.append(el("span", `${built.parts} parts:`, "count"));
  downloadsEl.append(saveAs(`${name}.ldr`, built.ldr,
    "the model, which Stud.io opens directly"));
  if (built.lua) {
    downloadsEl.append(saveAs(`${name}.lua`, built.lua,
      "the LDCad animation; keep it beside the .ldr"));
  }
  downloadsEl.hidden = false;
}

function saveAs(filename, text, title) {
  const link = el("a", filename);
  link.href = URL.createObjectURL(new Blob([text], { type: "text/plain" }));
  link.download = filename;
  link.title = title;
  return link;
}

async function start() {
  buildExampleButtons();
  specEl.addEventListener("input", scheduleCheck);
  specEl.addEventListener("input", () => {
    // What is drawn is the model that was built, not the description as it is
    // being typed: leaving it up would be showing an answer to a question that
    // has changed.
    downloadsEl.hidden = true;
    canvasEl.hidden = true;
  });
  buildEl.addEventListener("click", buildModel);

  statusEl.textContent = "";
  document.getElementById("wasm-note").textContent =
    "The engine runs in your browser; nothing is sent anywhere.";
  await loadExample(EXAMPLES[2][1]);
}

start();
