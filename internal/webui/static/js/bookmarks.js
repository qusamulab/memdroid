import { api } from "./api.js";
import { log, escHtml, actionArgs, tableMessage } from "./ui.js";

export async function loadBookmarks() {
  const list = await api("/bookmark/list");
  const tb = document.getElementById("bm-table");
  if (list.error) {
    tableMessage("bm-table", 6, list.error, true);
    return;
  }
  if (!list.length) {
    tb.innerHTML = '<tr><td class="empty" colspan="6">No bookmarks</td></tr>';
    return;
  }
  tb.innerHTML = list
    .map(
      (b) =>
        `<tr>
      <td style="color:var(--text3)">${b.index}</td>
      <td class="addr">${escHtml(b.addr)}</td>
      <td>${escHtml(b.label)}</td>
      <td style="color:var(--text2)">${escHtml(b.type)}</td>
      <td class="val">${escHtml(b.value)}</td>
      <td><button class="btn sm danger" data-click="bmRemove" data-args="${actionArgs(b.index)}">✕</button></td>
    </tr>`,
    )
    .join("");
}
export async function bmAdd() {
  const r = await api("/bookmark/add", {
    addr: document.getElementById("bm-addr").value,
    label: document.getElementById("bm-label").value,
  });
  log(r.error || "Bookmark added", !r.error);
  await loadBookmarks();
}
export async function bmRemove(idx) {
  const r = await api("/bookmark/remove", { index: idx });
  log(r.error || "Removed", !r.error);
  await loadBookmarks();
}
export async function bmModifyAll() {
  const r = await api("/bookmark/modify-all", {
    value: document.getElementById("bm-modval").value,
  });
  log(r.error || `Modified ${r.count} bookmarks`, !r.error);
  await loadBookmarks();
}
