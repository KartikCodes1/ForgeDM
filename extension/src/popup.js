const ext = globalThis.browser ?? globalThis.chrome;
const $ = (id) => document.getElementById(id);
const send = (msg) => new Promise((resolve) => ext.runtime.sendMessage(msg, (r) => resolve(r)));

function fmt(n) {
  if (!n || n <= 0) return ["0", "MB/s"];
  const units = ["B/s", "KB/s", "MB/s", "GB/s"];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return [n >= 100 ? n.toFixed(0) : n.toFixed(1), units[i]];
}

function show(which) {
  for (const id of ["offline", "unpaired", "paired"]) $(id).hidden = id !== which;
}

async function refresh() {
  const r = await send({ type: "status" });
  const state = $("state");
  if (!r || !r.running) {
    state.textContent = "Not running";
    state.className = "state bad";
    show("offline");
    return;
  }
  if (!r.paired) {
    state.textContent = "Not connected";
    state.className = "state";
    show("unpaired");
    return;
  }
  state.textContent = "Connected";
  state.className = "state ok";
  show("paired");
  $("catch").checked = !r.paused;
  const s = r.status;
  const [num, unit] = fmt(s?.speed ?? 0);
  $("speed").textContent = num;
  $("unit").textContent = unit;
  $("summary").textContent = s ? `${s.active} downloading · ${s.queued} waiting` : "";
  const items = $("items");
  items.textContent = "";
  for (const it of (s?.items ?? []).slice(0, 4)) {
    const li = document.createElement("li");
    li.innerHTML = `<div class="name"><span></span><span></span></div><div class="bar"><i></i></div>`;
    li.querySelector(".name span:first-child").textContent = it.name;
    li.querySelector(".name span:last-child").textContent = `${Math.floor(it.progress * 100)}%`;
    li.querySelector(".bar i").style.width = `${Math.floor(it.progress * 100)}%`;
    items.appendChild(li);
  }
}

async function loadMedia() {
  const [tab] = await ext.tabs.query({ active: true, currentWindow: true });
  if (!tab) return;
  const list = (await send({ type: "mediaForTab", tabId: tab.id })) || [];
  const usable = Array.isArray(list) ? list.filter((m) => m.kind === "hls" || m.kind === "file") : [];
  $("media").hidden = usable.length === 0;
  const ul = $("mediaList");
  ul.textContent = "";
  for (const m of usable.slice(0, 6)) {
    const li = document.createElement("li");
    const b = document.createElement("button");
    b.innerHTML = `<span class="tag"></span><span class="det"></span>`;
    b.querySelector(".tag").textContent = m.label;
    b.querySelector(".det").textContent = m.detail || m.url;
    b.addEventListener("click", async () => {
      b.querySelector(".det").textContent = "Sending…";
      const r = await send({ type: "downloadForTab", tabId: tab.id, item: m });
      b.querySelector(".det").textContent = r?.error ? r.error : "Sent to ForgeDM";
    });
    li.appendChild(b);
    ul.appendChild(li);
  }
}

$("connect").addEventListener("click", async () => {
  $("connect").disabled = true;
  $("pairmsg").textContent = "Approve the connection in the ForgeDM window…";
  const r = await send({ type: "pair" });
  $("connect").disabled = false;
  $("pairmsg").textContent = r?.error ?? "";
  refresh();
});
$("catch").addEventListener("change", (e) => send({ type: "setPaused", paused: !e.target.checked }));
$("open").addEventListener("click", () => send({ type: "show" }));

refresh();
loadMedia();
setInterval(refresh, 1500);
