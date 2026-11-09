import { escHtml } from "./ui.js";

export function clearWatch() {
  document.getElementById("watch-log").innerHTML =
    '<div class="empty">Waiting for value changes…</div>';
}

// Recover the live feed after a server restart or a temporary connection loss.
export function startWatch() {
  let delay = 1000;
  let stopped = false;
  let timer;
  let socket;
  const state = document.getElementById("watch-state");
  function connect() {
    const protocol = location.protocol === "https:" ? "wss:" : "ws:";
    socket = new WebSocket(`${protocol}//${location.host}/ws/watch`);
    socket.onopen = () => {
      delay = 1000;
      state.textContent = "Live";
      state.className = "badge on";
    };
    socket.onmessage = (message) => {
      let event;
      try {
        event = JSON.parse(message.data);
      } catch {
        return;
      }
      const feed = document.getElementById("watch-log");
      feed.querySelector(".empty")?.remove();
      const ts = new Date().toLocaleTimeString("en", { hour12: false });
      const line = document.createElement("div");
      line.className = "watch-event";
      line.innerHTML =
        `<span class="wts">${ts}</span><span class="waddr">${escHtml(event.addr)}</span>` +
        `<span class="wprev">${escHtml(event.kind === "alert" ? "⚠ " + event.condition : event.prev)}</span>` +
        `<span class="warrow">${event.triggered ? "✎" : "→"}</span><span class="wcur">${escHtml(event.cur)}</span>`;
      feed.prepend(line);
      while (feed.children.length > 200) feed.lastChild.remove();
    };
    socket.onclose = () => {
      if (stopped) return;
      state.textContent = "Reconnecting…";
      state.className = "badge warn";
      timer = setTimeout(connect, delay);
      delay = Math.min(delay * 2, 15000);
    };
  }
  connect();
  window.addEventListener("pagehide", () => {
    stopped = true;
    clearTimeout(timer);
    socket.close();
  });
}
