import { api } from "./api.js";
import { log } from "./ui.js";
import { refreshStatus } from "./status.js";

function sessPath() {
  return document.getElementById("sess-path").value || "memdroid.json";
}
export async function sessSave() {
  const r = await api("/session/save", { path: sessPath() });
  log(r.error ? r.error : `Saved → ${r.path}`, !r.error);
}
export async function sessLoad() {
  const r = await api("/session/load", { path: sessPath() });
  log(r.error ? r.error : `Loaded ← ${r.path}`, !r.error);
  await refreshStatus(true);
}
