import { api } from "./api.js";
import { log, escHtml, actionArgs, showPanel, tableMessage } from "./ui.js";
import { refreshStatus } from "./status.js";

export async function memModify() {
  const r = await api("/memory/modify", {
    addr: document.getElementById("mod-addr").value,
    value: document.getElementById("mod-val").value,
  });
  log(r.error || "Value written. Undo is available.", !r.error);
  await refreshStatus(true);
}
export async function memUndo() {
  const r = await api("/memory/undo", {});
  log(r.error ? r.error : `Undone (depth: ${r.undo_depth})`, !r.error);
  await refreshStatus(true);
}
export async function memFreeze() {
  const r = await api("/memory/freeze", {
    addr: document.getElementById("freeze-addr").value,
    value: document.getElementById("freeze-val").value,
  });
  log(r.error || "Frozen", !r.error);
  await loadFrozen();
  await refreshStatus(true);
}
export async function memUnfreeze() {
  const r = await api("/memory/unfreeze", {
    addr: document.getElementById("unfreeze-addr").value,
  });
  log(r.error || "Unfrozen", !r.error);
  await loadFrozen();
  await refreshStatus(true);
}
export async function loadFrozen() {
  const list = await api("/memory/frozen");
  const tb = document.getElementById("frozen-table");
  if (list.error) {
    tableMessage("frozen-table", 2, list.error, true);
    return;
  }
  if (!list.length) {
    tb.innerHTML = '<tr><td class="empty" colspan="2">None frozen</td></tr>';
    return;
  }
  tb.innerHTML = list
    .map(
      (a) =>
        `<tr><td class="addr">${escHtml(a)}</td>
     <td><button class="btn sm danger" data-click="quickUnfreeze" data-args="${actionArgs(a)}">Unfreeze</button></td></tr>`,
    )
    .join("");
}
export async function quickUnfreeze(addr) {
  const r = await api("/memory/unfreeze", { addr });
  log(r.error || "Unfrozen", !r.error);
  await loadFrozen();
  await refreshStatus(true);
}
export async function setFreezeInterval() {
  const ms = parseInt(document.getElementById("freeze-interval").value);
  if (!ms || ms <= 0) {
    log("Invalid interval", false);
    return;
  }
  const r = await api("/memory/freeze-interval", { interval_ms: ms });
  log(r.error || `Freeze interval: ${ms}ms`, !r.error);
}

export function prefillModify(addr, val) {
  showPanel("memory");
  document.getElementById("mod-addr").value = addr;
  document.getElementById("mod-val").value = val;
  document.getElementById("mod-val").focus();
}
export function prefillFreeze(addr, val) {
  showPanel("memory");
  document.getElementById("freeze-addr").value = addr;
  document.getElementById("freeze-val").value = val;
  document.getElementById("freeze-val").focus();
}

export async function loadHexdump() {
  const addr = document.getElementById("hex-addr").value.trim();
  const size = document.getElementById("hex-size").value.trim() || "256";
  if (!addr) {
    log("Address required", false);
    return;
  }
  const res = await api(
    `/memory/hexdump?addr=${encodeURIComponent(addr)}&size=${encodeURIComponent(size)}`,
  );
  if (res.error) {
    log(res.error, false);
    return;
  }
  const out = document.getElementById("hex-output");
  if (!res.hex_lines || !res.hex_lines.length) {
    out.textContent = "No data";
    return;
  }
  const lines = res.hex_lines.map((l) => {
    const off = l.offset.toString(16).padStart(8, "0");
    const hex = l.hex.padEnd(47, " ");
    return `${off}  ${hex}  |${l.ascii}|`;
  });
  out.textContent = lines.join("\n");
  log(`Hex dump loaded: ${res.hex_lines.length} lines`);
}
