import { api } from "./api.js";
import { log, escHtml, actionArgs, showPanel, tableMessage } from "./ui.js";
import { getStatus, refreshStatus } from "./status.js";

export async function loadDevices() {
  const list = await api("/device/list");
  if (list.error) {
    tableMessage("device-table", 2, list.error, true);
    return;
  }
  if (!list.length) {
    tableMessage(
      "device-table",
      2,
      "No devices found. Connect via USB, authorize debugging, and refresh.",
    );
    return;
  }
  document.getElementById("device-table").innerHTML = list
    .map(
      (device) =>
        `<tr><td class="addr">${escHtml(device.serial)}</td><td><button class="btn sm primary" data-click="selectDevice" data-args="${actionArgs(device.serial)}">${getStatus()?.device === device.serial ? "Selected · Continue →" : "Select device"}</button></td></tr>`,
    )
    .join("");
}

export async function selectDevice(serial) {
  const result = await api("/device/select", { serial });
  log(result.error || `Device selected: ${serial}`, !result.error);
  await refreshStatus(true);
  if (!result.error) showPanel("process");
}

export async function connectWifi() {
  const addr = document.getElementById("wifi-addr").value.trim();
  if (!addr) {
    log("Enter the device IP address and ADB port.", false);
    return;
  }
  const result = await api("/device/connect-wifi", { addr });
  log(
    result.error || `Connected: ${addr}. Select the device to continue.`,
    !result.error,
  );
  await refreshStatus(true);
  await loadDevices();
}

export async function disconnectWifi() {
  const addr = document.getElementById("wifi-addr").value.trim();
  if (!addr) {
    log("Enter the device address to disconnect.", false);
    return;
  }
  const result = await api("/device/disconnect-wifi", { addr });
  log(result.error || `Disconnected: ${addr}`, !result.error);
  await refreshStatus(true);
  await loadDevices();
}
