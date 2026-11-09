let currentStatus = {};
const pending = new Set();

export function escHtml(value) {
  return String(value).replace(
    /[&<>"']/g,
    (c) =>
      ({
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
        "'": "&#39;",
      })[c],
  );
}

// JSON data is escaped for an HTML attribute, never interpreted as code.
export function actionArgs(...args) {
  return escHtml(JSON.stringify(args));
}

export function log(message, ok = true) {
  const line = document.createElement("div");
  line.className = "log-line " + (ok ? "ok" : "err");
  const ts = new Date().toLocaleTimeString("en", { hour12: false });
  line.innerHTML = `<span class="ts">${ts}</span>${escHtml(message)}`;
  const history = document.getElementById("log");
  history.prepend(line);
  while (history.children.length > 80) history.lastChild.remove();

  const toast = document.createElement("div");
  toast.className = "toast " + (ok ? "ok" : "err");
  const text = document.createElement("span");
  text.textContent = message;
  const close = document.createElement("button");
  close.type = "button";
  close.textContent = "×";
  close.setAttribute("aria-label", "Dismiss notification");
  close.addEventListener("click", () => toast.remove());
  toast.append(text, close);
  const notifications = document.getElementById("notifications");
  notifications.append(toast);
  while (notifications.children.length > 3) notifications.firstChild.remove();
  // Errors remain available in the activity history after the toast closes.
  setTimeout(() => toast.remove(), ok ? 5000 : 12000);
}

export function clearLog() {
  document.getElementById("log").replaceChildren();
}

export function showPanel(name, focus = true) {
  const target = document.getElementById("panel-" + name);
  if (!target) return;
  document.querySelectorAll(".panel").forEach((panel) => {
    panel.classList.toggle("active", panel === target);
    panel.hidden = panel !== target;
  });
  document.querySelectorAll("nav [data-panel]").forEach((button) => {
    const active = button.dataset.panel === name;
    button.classList.toggle("active", active);
    if (active) button.setAttribute("aria-current", "page");
    else button.removeAttribute("aria-current");
  });
  if (focus) {
    const heading = target.querySelector("h1");
    heading.tabIndex = -1;
    heading.focus({ preventScroll: true });
    document.getElementById("content").scrollTop = 0;
  }
  document.dispatchEvent(new CustomEvent("panelchange", { detail: name }));
}

export function setStatus(status) {
  currentStatus = status;
  updateControls();
}

function updateControls() {
  const available = {
    device: !!currentStatus.device,
    attached: !!currentStatus.attached,
    candidates: !!currentStatus.attached && currentStatus.candidates > 0,
    undo: currentStatus.undo_depth > 0,
  };
  const reasons = {
    device: "Select a device first.",
    attached: "Attach a process first.",
    candidates: "Run a search to find candidates first.",
    undo: "No changes to undo.",
  };
  document
    .querySelectorAll(
      "[data-requires], [data-click], [data-change], [data-enter]",
    )
    .forEach((control) => {
      // Inputs remain editable so values can be prepared before attaching.
      const action =
        control.dataset.click ||
        control.dataset.change ||
        control.dataset.enter;
      const busy = pending.has(actionGroup(action));
      const blocked =
        control.tagName === "BUTTON" &&
        control.dataset.requires &&
        !available[control.dataset.requires];
      control.disabled = !!(
        busy ||
        blocked ||
        control.dataset.unavailable === "true"
      );
      if (blocked) control.title = reasons[control.dataset.requires];
      else control.removeAttribute("title");
    });
  const activity = document.getElementById("activity-state");
  activity.textContent = pending.size ? "Working…" : "Ready";
  activity.classList.toggle("working", pending.size > 0);
}

// A type change or filter must not race an in-flight scan in the same session.
function actionGroup(name) {
  if (
    [
      "searchValue",
      "searchPattern",
      "searchString",
      "filter",
      "filterExact",
      "setType",
      "searchReset",
    ].includes(name)
  )
    return "scan";
  if (["attachPID", "searchProc", "doAttach", "detach"].includes(name))
    return "process";
  return name;
}

export function bindActions(actions) {
  async function invoke(control, name) {
    const group = actionGroup(name);
    if (!actions[name] || pending.has(group)) return;
    const requirement = control.dataset.requires;
    if (
      (requirement === "device" && !currentStatus.device) ||
      (requirement === "attached" && !currentStatus.attached) ||
      (requirement === "candidates" && !currentStatus.candidates)
    ) {
      log(
        requirement === "device"
          ? "Select a device first."
          : requirement === "attached"
            ? "Attach a process first."
            : "Run a search first.",
        false,
      );
      return;
    }
    const args = JSON.parse(control.dataset.args || "[]");
    const hadFocus = document.activeElement === control;
    pending.add(group);
    const button = control.tagName === "BUTTON";
    const label = control.textContent;
    if (button) control.textContent = "Working…";
    control.setAttribute("aria-busy", "true");
    updateControls();
    try {
      await actions[name](...args);
    } catch (error) {
      log(error.message || "Operation failed. Please try again.", false);
    } finally {
      pending.delete(group);
      if (button) control.textContent = label;
      control.removeAttribute("aria-busy");
      updateControls();
      if (
        hadFocus &&
        control.isConnected &&
        !control.closest(".panel")?.hidden &&
        document.activeElement === document.body
      )
        control.focus({ preventScroll: true });
    }
  }
  document.addEventListener("click", (event) => {
    const panel = event.target.closest("[data-panel]");
    if (panel) {
      showPanel(panel.dataset.panel);
      return;
    }
    const control = event.target.closest("[data-click]");
    if (control && !control.disabled)
      void invoke(control, control.dataset.click);
  });
  for (const kind of ["change", "input"]) {
    document.addEventListener(kind, (event) => {
      const control = event.target.closest(`[data-${kind}]`);
      if (control && !control.disabled)
        void invoke(control, control.dataset[kind]);
    });
  }
  document.addEventListener("keydown", (event) => {
    if (event.key !== "Enter" || event.isComposing) return;
    const control = event.target.closest("[data-enter]");
    if (control && !control.disabled) {
      event.preventDefault();
      void invoke(control, control.dataset.enter);
    }
  });
  updateControls();
}

export function tableMessage(id, columns, message, error = false) {
  document.getElementById(id).innerHTML =
    `<tr><td class="empty${error ? " error" : ""}" colspan="${columns}">${escHtml(message)}</td></tr>`;
}
