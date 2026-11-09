import { api } from "./api.js";
import { log, showPanel } from "./ui.js";
import { refreshStatus } from "./status.js";

export async function setType() {
  const type = document.getElementById("value-type").value;
  const result = await api("/search/type", { type });
  log(
    result.error ||
      `Value type changed to ${type}. Previous candidates cleared.`,
    !result.error,
  );
  await refreshStatus(true);
}

export function regionChanged() {
  document.getElementById("custom-region").hidden =
    document.getElementById("search-region").value !== "custom";
}

export async function searchValue() {
  const value = document.getElementById("search-val").value.trim();
  if (!value) {
    log("Enter a value to search for.", false);
    return;
  }
  const region = document.getElementById("search-region").value;
  const body = {
    value,
    type: document.getElementById("value-type").value,
    region,
  };
  if (region === "custom") {
    body.region_start = document.getElementById("region-start").value.trim();
    body.region_end = document.getElementById("region-end").value.trim();
  }
  await presentResults(await api("/search/value", body));
}

export async function filter(mode) {
  const body = { mode };
  if (mode === "value") {
    body.value = document.getElementById("filter-val").value.trim();
    if (!body.value) {
      log("Enter the current value to filter by.", false);
      return;
    }
  }
  await presentResults(await api("/search/filter", body));
}

export function filterExact() {
  return filter("value");
}

export async function searchReset() {
  const result = await api("/search/reset", {});
  log(
    result.error || "Search cleared. Start a new scan when ready.",
    !result.error,
  );
  await refreshStatus(true);
}

export async function searchPattern() {
  const pattern = document.getElementById("pattern-val").value.trim();
  if (!pattern) {
    log("Enter a byte pattern, such as FF 00 ?? 01.", false);
    return;
  }
  await presentResults(await api("/search/pattern", { pattern }));
}

export async function searchString() {
  const value = document.getElementById("string-val").value;
  if (!value) {
    log("Enter text to search for.", false);
    return;
  }
  await presentResults(
    await api("/search/string", {
      value,
      encoding: document.getElementById("string-enc").value,
    }),
  );
}

async function presentResults(result) {
  if (result.error) {
    log(result.error, false);
    return;
  }
  const count = result.candidates;
  log(
    count > 0
      ? `${count.toLocaleString()} candidate addresses found.${result.truncated ? " Result limit reached; narrow your search." : ""}`
      : "No matches. Try another value, type, or memory region.",
  );
  await refreshStatus(true);
  // Filtering stays on the search panel so repeated refinement takes one click.
  document.getElementById("search-summary").hidden = false;
  if (!count)
    document.getElementById("search-summary-text").textContent =
      "No matches. Adjust the value or memory region and try again.";
  document.dispatchEvent(new CustomEvent("candidateschanged"));
}

export function prefillSearch(start, end) {
  showPanel("search");
  document.getElementById("search-region").value = "custom";
  document.getElementById("region-start").value = start;
  document.getElementById("region-end").value = end;
  regionChanged();
  document.getElementById("search-val").focus();
  log(`Search range selected: ${start} – ${end}`);
}
