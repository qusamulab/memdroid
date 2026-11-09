import { api } from "./api.js";
import { log, escHtml, showPanel } from "./ui.js";

export async function runPointerScan() {
  const addr = document.getElementById("ptr-addr").value.trim();
  const depth = parseInt(document.getElementById("ptr-depth").value) || 0;
  const offset = document.getElementById("ptr-offset").value.trim();
  const body = { addr };
  if (depth) body.max_depth = depth;
  if (offset) body.max_offset = parseInt(offset, 16) || parseInt(offset);
  log("Pointer scan started…");
  const r = await api("/pointer/scan", body);
  if (r.error) {
    log(r.error, false);
    return;
  }
  const tree = document.getElementById("ptr-tree");
  if (!r.chains || !r.chains.length) {
    tree.innerHTML = '<span class="empty">No chains found</span>';
    log("No chains found", false);
    return;
  }

  // Group chains by base module label
  const groups = {};
  for (const c of r.chains) {
    const key = c.label || "(unknown)";
    if (!groups[key]) groups[key] = [];
    groups[key].push(c);
  }

  let html = "";
  for (const [label, chains] of Object.entries(groups)) {
    html += `<div style="margin-bottom:12px">`;
    html += `<div style="color:var(--green);font-weight:600;margin-bottom:4px">${escHtml(label)}</div>`;
    for (const c of chains) {
      const offsets = (c.offsets || []).map((o) => {
        const hex = "0x" + Math.abs(o).toString(16);
        return o >= 0 ? "+" + hex : "-" + hex;
      });
      const indent = "&nbsp;&nbsp;&nbsp;&nbsp;";
      html += `<div style="color:var(--text)">${indent}<span class="addr">${escHtml(c.base)}</span>`;
      for (let i = 0; i < offsets.length; i++) {
        html += ` <span style="color:var(--yellow)">${escHtml(offsets[i])}</span>`;
      }
      html += `</div>`;
      // Show indented path breakdown
      let pathHtml = indent + indent;
      pathHtml += `<span style="color:var(--text3);font-size:11.5px">${escHtml(c.path)}</span>`;
      html += `<div>${pathHtml}</div>`;
    }
    html += `</div>`;
  }
  tree.innerHTML = html;
  log(
    `Found ${r.chains.length} chains in ${Object.keys(groups).length} modules`,
  );
}

export function prefillPointer(addr) {
  showPanel("pointer");
  document.getElementById("ptr-addr").value = addr;
}
