// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker
//
// The page. Everything it knows how to do is ask brickmeshCheck and lay out the
// answer; the engine is the WebAssembly module, and this file deliberately
// contains no rules about gears.

"use strict";

const EXAMPLES = [
  ["reduction", "examples/reduction.json"],
  ["subtractor", "examples/subtractor.json"],
  ["3-speed gearbox", "examples/gearbox-3-speed.json"],
  ["auto-shifting", "examples/gearbox-3-speed-auto.json"],
];

const specEl = document.getElementById("spec");
const answerEl = document.getElementById("answer");
const statusEl = document.getElementById("status");

let ready = false;

// A run per keystroke would be wasteful even at a millisecond a go, and the
// answer flickering as you type mid-word is worse than a beat of delay.
let pending = null;
function scheduleCheck() {
  clearTimeout(pending);
  pending = setTimeout(check, 150);
}

function check() {
  if (!ready) return;
  let result;
  try {
    result = JSON.parse(brickmeshCheck(specEl.value));
  } catch (err) {
    render({ error: `the engine did not answer: ${err}` });
    return;
  }
  render(result);
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

// waitFor polls until a condition holds, or gives up.
function waitFor(cond, tries = 200, every = 10) {
  return new Promise((resolve, reject) => {
    (function attempt(left) {
      if (cond()) return resolve();
      if (left <= 0) return reject(new Error("the engine did not start"));
      setTimeout(() => attempt(left - 1), every);
    })(tries);
  });
}

async function start() {
  buildExampleButtons();
  specEl.addEventListener("input", scheduleCheck);

  const go = new Go();
  try {
    const wasm = await WebAssembly.instantiateStreaming(
      fetch("brickmesh.wasm"), go.importObject);
    // go.run settles only when the module's main returns, and this one never
    // does — a Go module whose main returns is torn down, taking its exported
    // functions with it. So it is started, not awaited, and what we wait for is
    // the flag main sets once the exports are in place. Awaiting go.run hangs;
    // not waiting at all calls brickmeshCheck before it exists.
    go.run(wasm.instance);
    await waitFor(() => globalThis.brickmeshReady === true);
  } catch (err) {
    statusEl.textContent =
      `could not start the engine: ${err}. It needs to be served over http, ` +
      `not opened from a file, and built with "make web".`;
    return;
  }
  ready = true;
  statusEl.textContent = "";
  document.getElementById("wasm-note").textContent =
    "The engine runs in your browser; nothing is sent anywhere.";
  await loadExample(EXAMPLES[2][1]);
}

start();
