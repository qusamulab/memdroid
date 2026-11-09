import { api } from "./api.js";
import { log, escHtml, actionArgs, tableMessage } from "./ui.js";
import { refreshStatus } from "./status.js";

let currentPage = 0;
let total = 0;
let requestID = 0;

export async function loadCandidates(page = currentPage) {
  const request = ++requestID;
  const pageSize = Number(document.getElementById("cand-pagesize").value);
  const result = await api(
    `/search/candidates?page=${page}&page_size=${pageSize}`,
  );
  // Ignore an older page response if the user has already navigated elsewhere.
  if (request !== requestID) return;
  if (result.error) {
    tableMessage("cand-table", 3, result.error, true);
    document.getElementById("cand-pagination").style.display = "none";
    return;
  }
  total = result.total;
  const pages = Math.max(1, Math.ceil(total / pageSize));
  if (page >= pages) return loadCandidates(pages - 1);
  currentPage = page;
  document.getElementById("cand-count").textContent = total.toLocaleString();
  if (!result.items?.length) {
    document.getElementById("cand-table").innerHTML =
      '<tr><td class="empty" colspan="3">No candidate addresses yet.<br>Run a search or try a different value.<br><button type="button" class="btn sm" data-panel="search">Go to search →</button></td></tr>';
  } else {
    document.getElementById("cand-table").innerHTML = result.items
      .map(
        (candidate) =>
          `<tr><td class="addr">${escHtml(candidate.addr)}</td><td class="val">${escHtml(candidate.value)}</td><td>
      <button class="btn sm" data-click="prefillModify" data-args="${actionArgs(candidate.addr, candidate.value)}">Edit</button>
      <button class="btn sm success" data-click="prefillFreeze" data-args="${actionArgs(candidate.addr, candidate.value)}">Freeze</button>
      <button class="btn sm" data-click="prefillPointer" data-args="${actionArgs(candidate.addr)}">Pointer</button>
      </td></tr>`,
      )
      .join("");
  }
  document.getElementById("cand-pageinfo").textContent =
    `Page ${page + 1} of ${pages} · ${total.toLocaleString()} results`;
  document.getElementById("cand-pagination").style.display =
    total > pageSize ? "" : "none";
  for (const [id, disabled] of [
    ["cand-prev", page === 0],
    ["cand-next", page >= pages - 1],
  ]) {
    const button = document.getElementById(id);
    button.dataset.unavailable = String(disabled);
    button.disabled = disabled;
  }
}

export function candPage(delta) {
  const pages = Math.ceil(
    total / Number(document.getElementById("cand-pagesize").value),
  );
  const next = currentPage + delta;
  if (next >= 0 && next < pages) return loadCandidates(next);
}

export async function freezeAll() {
  const result = await api("/memory/freeze-all", {});
  log(result.error || `Frozen ${result.count} addresses`, !result.error);
  await refreshStatus(true);
}
