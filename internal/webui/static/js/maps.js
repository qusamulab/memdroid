import { api } from "./api.js";
import { log, escHtml, actionArgs, tableMessage } from "./ui.js";

let allMaps = [];
export async function loadMaps() {
  allMaps = await api("/maps");
  if (allMaps.error) {
    tableMessage("maps-table", 5, allMaps.error, true);
    allMaps = [];
    return;
  }
  filterMaps();
}
function renderMaps(list) {
  const tb = document.getElementById("maps-table");
  if (!list.length) {
    tb.innerHTML = '<tr><td class="empty" colspan="5">—</td></tr>';
    return;
  }
  tb.innerHTML = list
    .map((r) => {
      const kb = (r.size / 1024).toFixed(0);
      return `<tr>
      <td class="addr">${escHtml(r.start)}</td>
      <td class="addr">${escHtml(r.end)}</td>
      <td style="color:var(--text2)">${kb} KB</td>
      <td>${escHtml(r.name || "[anon]")}</td>
      <td><button class="btn sm" data-click="prefillSearch" data-args="${actionArgs(r.start, r.end)}">Search</button></td>
    </tr>`;
    })
    .join("");
}
export function filterMaps() {
  const q = document.getElementById("maps-filter").value.toLowerCase();
  renderMaps(
    allMaps.filter(
      (r) => (r.name || "").toLowerCase().includes(q) || r.start.includes(q),
    ),
  );
}
