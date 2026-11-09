import { api } from "./api.js";
import { setStatus } from "./ui.js";

let latest;
let inFlight;

export function getStatus() {
  return latest;
}

// Coalesce polling and action refreshes; a slow response never starts a second poll.
export function refreshStatus(fresh = false) {
  if (inFlight) return fresh ? inFlight.then(() => refreshStatus()) : inFlight;
  inFlight = updateStatus().finally(() => {
    inFlight = undefined;
  });
  return inFlight;
}

async function updateStatus() {
  const status = await api("/status");
  const connection = document.getElementById("connection-state");
  const notice = document.getElementById("connection-alert");
  connection.textContent = status.error ? "Disconnected" : "Server online";
  connection.classList.toggle("online", !status.error);
  notice.hidden = !status.error;
  notice.textContent = status.error || "";
  if (status.error) {
    setStatus({});
    return;
  }
  latest = status;
  setStatus(status);

  function badge(id, text, active) {
    const el = document.getElementById(id);
    el.replaceChildren();
    const dot = document.createElement("span");
    dot.className = "dot";
    el.append(dot, document.createTextNode(text));
    el.className = "badge " + (active ? "on" : "off");
  }
  badge("sb-device", status.device || "No device", !!status.device);
  badge(
    "sb-attach",
    status.attached ? "PID " + status.pid : "Not attached",
    status.attached,
  );
  document.getElementById("sb-type").textContent = status.value_type;
  document.getElementById("sb-candidates").textContent =
    status.candidates.toLocaleString() + " candidates";
  const type = document.getElementById("value-type");
  if (!type.disabled) type.value = status.value_type;
  const frozen = document.getElementById("sb-frozen");
  frozen.textContent = "❄ " + (status.frozen || []).length + " frozen";
  frozen.style.display = status.frozen?.length ? "" : "none";
  const undo = document.getElementById("sb-undo");
  undo.textContent = "↩ undo " + status.undo_depth;
  undo.style.display = status.undo_depth > 0 ? "" : "none";

  document
    .getElementById("step-device")
    .classList.toggle("complete", !!status.device);
  document
    .getElementById("step-process")
    .classList.toggle("complete", status.attached);
  document
    .getElementById("step-search")
    .classList.toggle("complete", status.candidates > 0);
  document.getElementById("step-device-detail").textContent =
    status.device || "Select an ADB target";
  document.getElementById("step-process-detail").textContent = status.attached
    ? "PID " + status.pid
    : "Choose a running app";
  document.getElementById("step-search-detail").textContent =
    status.candidates > 0
      ? status.candidates.toLocaleString() + " candidates"
      : "Find and refine values";
  const summary = document.getElementById("search-summary");
  summary.hidden = status.candidates === 0;
  document.getElementById("search-summary-text").textContent =
    status.candidates.toLocaleString() + " candidate addresses in this session";
}
