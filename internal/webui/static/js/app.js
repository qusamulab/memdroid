import { bindActions, clearLog, log, showPanel } from "./ui.js";
import { getStatus, refreshStatus } from "./status.js";
import { startWatch, clearWatch } from "./watch.js";
import * as device from "./device.js";
import * as process from "./process.js";
import * as search from "./search.js";
import * as candidates from "./candidates.js";
import * as pointer from "./pointer.js";
import * as memory from "./memory.js";
import * as maps from "./maps.js";
import * as bookmarks from "./bookmarks.js";
import * as session from "./session.js";

bindActions({
  clearLog,
  clearWatch,
  ...device,
  ...process,
  ...search,
  ...candidates,
  ...pointer,
  ...memory,
  ...maps,
  ...bookmarks,
  ...session,
});

// Opening a panel refreshes its data; no extra Refresh click is required.
const loaders = {
  device: device.loadDevices,
  process: () => getStatus()?.device && process.loadProcessList(),
  candidates: candidates.loadCandidates,
  memory: memory.loadFrozen,
  maps: () => getStatus()?.attached && maps.loadMaps(),
  bookmarks: bookmarks.loadBookmarks,
};
document.addEventListener("panelchange", (event) => {
  Promise.resolve(loaders[event.detail]?.()).catch((error) =>
    log(error.message, false),
  );
});

await refreshStatus();
showPanel("device", false);
startWatch();
// Schedule the next poll after completion so slow transports cannot stack work.
async function poll() {
  await refreshStatus();
  setTimeout(poll, 2000);
}
setTimeout(poll, 2000);

document.addEventListener("candidateschanged", () => {
  if (document.getElementById("panel-candidates").classList.contains("active"))
    void candidates.loadCandidates(0);
});
