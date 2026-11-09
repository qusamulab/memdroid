import { api } from "./api.js";
import { log, escHtml, actionArgs, showPanel, tableMessage } from "./ui.js";
import { refreshStatus } from "./status.js";

let allProcs = [];
export async function loadProcessList() {
  allProcs = await api("/process/list");
  if (allProcs.error) {
    tableMessage("proc-table", 3, allProcs.error, true);
    allProcs = [];
    return;
  }
  filterProc();
}
function renderProcs(list) {
  const tb = document.getElementById("proc-table");
  if (!list.length) {
    tb.innerHTML =
      '<tr><td class="empty" colspan="3">No matching processes. Try another name or refresh the list.</td></tr>';
    return;
  }
  tb.innerHTML = list
    .map(
      (p) =>
        `<tr><td style="color:var(--text2)">${p.pid}</td>
     <td>${escHtml(p.name)}</td>
     <td><button class="btn sm" data-click="doAttach" data-args="${actionArgs(p.pid, p.name)}">Attach</button></td></tr>`,
    )
    .join("");
}
export function filterProc() {
  const q = document.getElementById("proc-filter").value.toLowerCase();
  renderProcs(
    allProcs.filter(
      (p) => p.name.toLowerCase().includes(q) || String(p.pid).includes(q),
    ),
  );
}
export async function searchProc() {
  const name = document.getElementById("proc-search").value.trim();
  if (!name) {
    log("Enter a process name to search for.", false);
    return;
  }
  const matches = await api("/process/search", { name });
  if (matches.error) {
    log(matches.error, false);
    return;
  }
  if (!matches.length) {
    log("No matching process found", false);
    return;
  }
  if (matches.length === 1) {
    await doAttach(matches[0].pid, matches[0].name);
    return;
  }
  renderProcs(matches);
  log(`Found ${matches.length} matches — click Attach`);
}
export async function doAttach(pid, name = "") {
  const r = await api("/process/attach", { pid, name });
  log(r.error || `Attached PID ${pid}`, !r.error);
  await refreshStatus(true);
  if (!r.error) showPanel("search");
}
export async function attachPID() {
  const pid = Number(document.getElementById("attach-pid").value);
  if (!Number.isInteger(pid) || pid <= 0) {
    log("Enter a positive process ID.", false);
    return;
  }
  await doAttach(pid);
}
export async function detach() {
  const r = await api("/process/detach", {});
  log(r.error || "Detached", !r.error);
  await refreshStatus(true);
}
export async function stopProc() {
  const r = await api("/process/stop", {});
  log(r.error || "Stopped", !r.error);
}
export async function continueProc() {
  const r = await api("/process/continue", {});
  log(r.error || "Continued", !r.error);
}
