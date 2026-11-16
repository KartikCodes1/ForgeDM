// ForgeDM content script: Alt-click bypass, "Download all links" and the
// "Download this video" button on web video players.
(() => {
  if (window.__forgedm) return;
  window.__forgedm = true;
  const ext = globalThis.browser ?? globalThis.chrome;
  const send = (msg) =>
    new Promise((resolve) => {
      try {
        ext.runtime.sendMessage(msg, (r) => resolve(r));
      } catch (_) {
        resolve(null);
      }
    });

  // Alt held while clicking a link lets the browser download it.
  window.addEventListener("keydown", (e) => e.key === "Alt" && send({ type: "alt", down: true }), true);
  window.addEventListener("keyup", (e) => e.key === "Alt" && send({ type: "alt", down: false }), true);
  window.addEventListener("click", (e) => e.altKey && send({ type: "alt", down: true }), true);

  // "Download all links with ForgeDM"
  ext.runtime.onMessage.addListener((msg, _sender, reply) => {
    if (msg?.type !== "collectLinks") return;
    const sel = window.getSelection();
    const inSelection = (el) => !msg.selection || !sel || sel.rangeCount === 0 || sel.containsNode(el, true);
    const seen = new Set();
    const out = [];
    const add = (url, name) => {
      try {
        const u = new URL(url, location.href);
        if (!/^https?:$/.test(u.protocol)) return;
        u.hash = "";
        const k = u.href;
        if (seen.has(k)) return;
        seen.add(k);
        out.push({ url: k, name: (name || "").trim().slice(0, 200) });
      } catch (_) {
        /* ignore bad links */
      }
    };
    document.querySelectorAll("a[href]").forEach((a) => inSelection(a) && add(a.href, a.getAttribute("download") || ""));
    document.querySelectorAll("img[src], video[src], audio[src], source[src]").forEach((m) => inSelection(m) && add(m.currentSrc || m.src, ""));
    reply(out.slice(0, 2000));
    return true;
  });

  // ---- video button ----
  let enabled = false;
  send({ type: "config" }).then((c) => {
    enabled = !!c?.videoButton;
    if (enabled) watchVideos();
  });

  const host = document.createElement("div");
  host.style.cssText = "position:fixed;z-index:2147483647;top:0;left:0;width:0;height:0;";
  const root = host.attachShadow({ mode: "closed" });
  root.innerHTML = `
    <style>
      :host { all: initial; }
      .btn { position: fixed; display: none; align-items: center; gap: 8px; height: 34px; padding: 0 12px 0 10px; border: 0; border-radius: 10px;
        background: #2EE6D6; color: #03201D; font: 600 13px/1 system-ui, "Segoe UI", sans-serif; cursor: pointer; box-shadow: 0 6px 20px rgba(0,0,0,.35); }
      .btn:hover { background: #5FF0E3; }
      .menu { position: fixed; display: none; width: 300px; padding: 6px; border-radius: 12px; background: #0E1422; border: 1px solid #22304D;
        color: #E9EDF7; font: 13px/1.3 system-ui, "Segoe UI", sans-serif; box-shadow: 0 16px 40px rgba(0,0,0,.45); }
      .head { padding: 8px 10px 6px; font-size: 12px; color: #8A94AD; }
      .item { display: flex; align-items: center; gap: 10px; width: 100%; height: 40px; padding: 0 10px; border: 0; border-radius: 8px; background: transparent; color: inherit; font: inherit; cursor: pointer; text-align: left; }
      .item:hover { background: #18213A; }
      .item[disabled] { cursor: default; opacity: .6; }
      .tag { min-width: 54px; padding: 2px 6px; border-radius: 6px; background: #1F2840; color: #C4CBDB; font-size: 11px; font-weight: 700; text-align: center; }
      .item:first-of-type .tag { background: #2EE6D6; color: #03201D; }
      .det { flex: 1; color: #A3ADC4; font-size: 12px; }
      svg { flex: none; }
    </style>
    <button class="btn" part="btn"><svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M12 4v11M7 10l5 5 5-5M5 20h14"/></svg>Download this video</button>
    <div class="menu" role="menu"></div>`;
  const btn = root.querySelector(".btn");
  const menu = root.querySelector(".menu");
  let current = null;
  let hideTimer = 0;

  function place(v) {
    const r = v.getBoundingClientRect();
    btn.style.top = `${Math.max(4, r.top + 12)}px`;
    btn.style.left = `${Math.max(4, r.right - 200)}px`;
    menu.style.top = `${Math.max(4, r.top + 54)}px`;
    menu.style.left = `${Math.max(4, r.right - 312)}px`;
  }

  function show(v) {
    if (!enabled) return;
    clearTimeout(hideTimer);
    current = v;
    if (!host.isConnected) document.documentElement.appendChild(host);
    place(v);
    btn.style.display = "inline-flex";
  }

  function hideSoon() {
    clearTimeout(hideTimer);
    hideTimer = setTimeout(() => {
      if (menu.style.display === "block") return;
      btn.style.display = "none";
    }, 1200);
  }

  function closeMenu() {
    menu.style.display = "none";
    hideSoon();
  }

  btn.addEventListener("mouseenter", () => clearTimeout(hideTimer));
  btn.addEventListener("mouseleave", hideSoon);
  btn.addEventListener("click", async (e) => {
    e.preventDefault();
    e.stopPropagation();
    if (menu.style.display === "block") return closeMenu();
    menu.innerHTML = `<div class="head">Looking for videos…</div>`;
    menu.style.display = "block";
    let items = (await send({ type: "getMedia" })) || [];
    if (items.error) items = [];
    const src = current?.currentSrc || current?.src || "";
    if (/^https?:/.test(src) && !items.some((i) => i.url === src)) items.push({ url: src, kind: "file", label: "Video", detail: "Playing now" });
    if (current?.mediaKeys) items = [{ kind: "protected", label: "Protected", detail: "This video is copy-protected", url: "" }];
    if (!items.length) {
      menu.innerHTML = `<div class="head">No downloadable video found yet. Play the video for a moment and try again. Copy-protected videos can't be saved.</div>`;
      return;
    }
    const usable = items.filter((i) => i.kind === "hls" || i.kind === "file");
    menu.innerHTML = `<div class="head">${usable.length} stream${usable.length === 1 ? "" : "s"} found on this page</div>`;
    for (const it of items.slice(0, 8)) {
      const b = document.createElement("button");
      b.className = "item";
      b.setAttribute("role", "menuitem");
      b.innerHTML = `<span class="tag"></span><span class="det"></span>`;
      b.querySelector(".tag").textContent = it.label;
      b.querySelector(".det").textContent = it.detail || "";
      if (it.kind !== "hls" && it.kind !== "file") b.disabled = true;
      b.addEventListener("click", async (ev) => {
        ev.stopPropagation();
        b.querySelector(".det").textContent = "Sending to ForgeDM…";
        const r = await send({ type: "downloadMedia", item: it, title: document.title });
        b.querySelector(".det").textContent = r?.error ? r.error : "Sent to ForgeDM";
        setTimeout(closeMenu, 900);
      });
      menu.appendChild(b);
    }
  });
  document.addEventListener("click", (e) => {
    if (menu.style.display === "block" && !e.composedPath().includes(host)) closeMenu();
  }, true);
  window.addEventListener("scroll", () => current && btn.style.display !== "none" && place(current), true);

  function hook(v) {
    if (v.__forgedm) return;
    v.__forgedm = true;
    const maybe = () => {
      const r = v.getBoundingClientRect();
      if (r.width >= 200 && r.height >= 120) show(v);
    };
    v.addEventListener("mouseenter", maybe);
    v.addEventListener("mousemove", maybe);
    v.addEventListener("mouseleave", hideSoon);
    v.addEventListener("play", maybe);
  }

  function watchVideos() {
    document.querySelectorAll("video").forEach(hook);
    new MutationObserver(() => document.querySelectorAll("video").forEach(hook)).observe(document.documentElement, { childList: true, subtree: true });
    // Many players cover the <video> with their own controls layer.
    document.addEventListener("mousemove", (e) => {
      for (const v of document.querySelectorAll("video")) {
        const r = v.getBoundingClientRect();
        if (r.width >= 200 && r.height >= 120 && e.clientX >= r.left && e.clientX <= r.right && e.clientY >= r.top && e.clientY <= r.bottom) {
          show(v);
          return;
        }
      }
    }, { passive: true });
  }
})();
