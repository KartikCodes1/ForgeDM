// ForgeDM browser extension — background script.
// Works as a Chrome/Edge MV3 service worker and as a Firefox MV3 background script.
const ext = globalThis.browser ?? globalThis.chrome;
const FIRST_PORT = 52771;
const PORTS = Array.from({ length: 10 }, (_, i) => FIRST_PORT + i);

const state = {
  port: null,
  token: null,
  config: { captureEnabled: true, exts: [], minSize: 0, videoButton: true, altBypass: true },
  paused: false,
  altUntil: 0,
  media: new Map(), // tabId -> Map(url -> item)
};

const BINARY_MIME = /^(application\/(octet-stream|zip|x-zip|x-7z|x-rar|vnd\.rar|x-msdownload|x-msi|x-iso9660|x-tar|gzip|x-gzip|pdf|vnd\.android|x-apple-diskimage|x-debian)|video\/|audio\/)/i;

async function load() {
  const s = await ext.storage.local.get(["token", "port", "paused", "config"]);
  state.token = s.token ?? null;
  state.port = s.port ?? null;
  state.paused = !!s.paused;
  if (s.config) state.config = s.config;
}
const ready = load();

function timeoutSignal(ms) {
  const c = new AbortController();
  setTimeout(() => c.abort(), ms);
  return c.signal;
}

async function findApp() {
  const order = state.port ? [state.port, ...PORTS.filter((p) => p !== state.port)] : PORTS;
  for (const p of order) {
    try {
      const r = await fetch(`http://127.0.0.1:${p}/v1/ping`, { headers: state.token ? { "X-ForgeDM-Token": state.token } : {}, signal: timeoutSignal(700) });
      if (!r.ok) continue;
      const j = await r.json();
      if (j.app === "ForgeDM") {
        if (state.port !== p) {
          state.port = p;
          ext.storage.local.set({ port: p });
        }
        return j;
      }
    } catch (_) {
      /* try the next port */
    }
  }
  return null;
}

async function call(path, body, ms = 4000) {
  await ready;
  if (!state.port) await findApp();
  if (!state.port) throw new Error("ForgeDM isn't running");
  const opts = { method: body === undefined ? "GET" : "POST", headers: { "Content-Type": "application/json" }, signal: timeoutSignal(ms) };
  if (state.token) opts.headers["X-ForgeDM-Token"] = state.token;
  if (body !== undefined) opts.body = JSON.stringify(body);
  let r;
  try {
    r = await fetch(`http://127.0.0.1:${state.port}${path}`, opts);
  } catch (e) {
    state.port = null;
    throw new Error("ForgeDM isn't running");
  }
  if (r.status === 401) throw new Error("not paired");
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(j.error || `ForgeDM answered ${r.status}`);
  return j;
}

function browserName() {
  const ua = navigator.userAgent;
  if (/Edg\//.test(ua)) return "Microsoft Edge";
  if (/OPR\//.test(ua)) return "Opera";
  if (/Firefox\//.test(ua)) return "Mozilla Firefox";
  if (navigator.brave) return "Brave";
  if (/Vivaldi/.test(ua)) return "Vivaldi";
  return "Google Chrome";
}

async function pair() {
  const app = await findApp();
  if (!app) throw new Error("Open ForgeDM first, then try again.");
  const r = await fetch(`http://127.0.0.1:${state.port}/v1/pair`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name: browserName() }),
    signal: timeoutSignal(125000),
  });
  const j = await r.json().catch(() => ({}));
  if (!r.ok || !j.token) throw new Error(j.error || "ForgeDM didn't allow the connection.");
  state.token = j.token;
  await ext.storage.local.set({ token: j.token });
  await refreshConfig();
  return true;
}

async function refreshConfig() {
  try {
    const c = await call("/v1/config");
    state.config = c;
    ext.storage.local.set({ config: c });
  } catch (_) {
    /* keep the last known config */
  }
}

async function cookiesFor(url) {
  try {
    const list = await ext.cookies.getAll({ url });
    return list.map((c) => `${c.name}=${c.value}`).join("; ");
  } catch (_) {
    return "";
  }
}

function extOf(path) {
  const clean = (path || "").split(/[?#]/)[0];
  const base = clean.split(/[\\/]/).pop() || "";
  const i = base.lastIndexOf(".");
  return i > 0 ? base.slice(i + 1).toLowerCase() : "";
}

function baseName(p) {
  return (p || "").split(/[\\/]/).pop() || "";
}

function shouldCapture(item) {
  const c = state.config;
  if (state.paused || !state.token || !c.captureEnabled) return false;
  if (Date.now() < state.altUntil && c.altBypass) return false;
  const url = item.finalUrl || item.url || "";
  if (!/^https?:/i.test(url)) return false;
  if (item.byExtensionId) return false;
  if (c.minSize > 0 && item.fileSize > 0 && item.fileSize < c.minSize) return false;
  const e = extOf(item.filename) || extOf(url);
  const exts = (c.exts || []).map((x) => x.toLowerCase().replace(/^\./, ""));
  if (e && exts.includes(e)) return true;
  if (!e && item.mime && BINARY_MIME.test(item.mime)) return true;
  return false;
}

async function sendDownload(info) {
  const cookies = await cookiesFor(info.url);
  return call("/v1/download", { userAgent: navigator.userAgent, cookies, ...info }, 8000);
}

// ---- catching browser downloads ----

ext.downloads.onCreated.addListener(async (item) => {
  await ready;
  if (!shouldCapture(item)) return;
  try {
    if (ext.downloads.pause) await ext.downloads.pause(item.id).catch(() => {});
    const url = item.finalUrl || item.url;
    await sendDownload({
      url,
      filename: baseName(item.filename) || undefined,
      referrer: item.referrer || undefined,
      pageUrl: item.referrer || undefined,
      mime: item.mime || undefined,
      size: item.fileSize > 0 ? item.fileSize : item.totalBytes > 0 ? item.totalBytes : undefined,
    });
    await ext.downloads.cancel(item.id).catch(() => {});
    await ext.downloads.erase({ id: item.id }).catch(() => {});
  } catch (e) {
    // ForgeDM isn't available: let the browser carry on.
    if (ext.downloads.resume) ext.downloads.resume(item.id).catch(() => {});
  }
});

// ---- context menus ----

function setupMenus() {
  ext.contextMenus.removeAll(() => {
    ext.contextMenus.create({ id: "forgedm-link", title: "Download with ForgeDM", contexts: ["link", "video", "audio", "image"] });
    ext.contextMenus.create({ id: "forgedm-all", title: "Download all links with ForgeDM", contexts: ["page", "selection"] });
  });
}
ext.runtime.onInstalled.addListener(setupMenus);
ext.runtime.onStartup?.addListener(setupMenus);

ext.contextMenus.onClicked.addListener(async (info, tab) => {
  try {
    if (info.menuItemId === "forgedm-link") {
      const url = info.linkUrl || info.srcUrl;
      if (!url || !/^https?:/i.test(url)) return notify("ForgeDM can only download http and https links.");
      await sendDownload({ url, referrer: tab?.url, pageUrl: tab?.url, pageTitle: tab?.title });
    } else if (info.menuItemId === "forgedm-all" && tab?.id !== undefined) {
      const links = await ext.tabs.sendMessage(tab.id, { type: "collectLinks", selection: !!info.selectionText });
      if (!links?.length) return notify("No links found on this page.");
      const cookies = await cookiesFor(tab.url);
      await call("/v1/batch", { links, pageUrl: tab.url, cookies, userAgent: navigator.userAgent });
    }
  } catch (e) {
    notify(e.message === "not paired" ? "Connect the extension to ForgeDM first (click the ForgeDM icon)." : e.message);
  }
});

function notify(message) {
  try {
    ext.action?.setBadgeText({ text: "!" });
    ext.action?.setBadgeBackgroundColor({ color: "#FF6B6B" });
    ext.action?.setTitle({ title: `ForgeDM: ${message}` });
    setTimeout(() => {
      ext.action?.setBadgeText({ text: "" });
      ext.action?.setTitle({ title: "ForgeDM" });
    }, 6000);
  } catch (_) {
    /* ignore */
  }
}

// ---- finding videos on pages ----

function mediaFor(tabId) {
  let m = state.media.get(tabId);
  if (!m) {
    m = new Map();
    state.media.set(tabId, m);
  }
  return m;
}

ext.webRequest.onHeadersReceived.addListener(
  (d) => {
    if (d.tabId < 0 || d.method !== "GET" || d.statusCode >= 400) return;
    const h = Object.fromEntries((d.responseHeaders || []).map((x) => [x.name.toLowerCase(), x.value || ""]));
    const ct = (h["content-type"] || "").toLowerCase();
    const url = d.url;
    const e = extOf(url);
    if (e === "ts" || e === "m4s" || (!ct.includes("mpegurl") && /(seg|chunk|frag)[-_]?\d/i.test(url))) return;
    let kind = null;
    if (ct.includes("mpegurl") || e === "m3u8") kind = "hls";
    else if ((ct.startsWith("video/") || ct.startsWith("audio/")) && !ct.includes("mp2t")) kind = "file";
    if (!kind) return;
    let size = -1;
    const cr = h["content-range"];
    if (cr && cr.includes("/")) size = parseInt(cr.split("/").pop(), 10) || -1;
    else if (h["content-length"]) size = parseInt(h["content-length"], 10) || -1;
    if (kind === "file" && size > 0 && size < 300 * 1024) return;
    const key = url.split("#")[0];
    const m = mediaFor(d.tabId);
    if (!m.has(key)) {
      m.set(key, { url: key, kind, mime: ct.split(";")[0], size, at: Date.now() });
      ext.storage.session?.set({ [`media_${d.tabId}`]: [...m.values()].slice(-40) }).catch?.(() => {});
    }
    updateBadge(d.tabId);
  },
  { urls: ["<all_urls>"], types: ["media", "xmlhttprequest", "other"] },
  ["responseHeaders"],
);

function updateBadge(tabId) {
  const n = state.media.get(tabId)?.size ?? 0;
  try {
    ext.action?.setBadgeText({ tabId, text: n ? String(Math.min(n, 9)) : "" });
    ext.action?.setBadgeBackgroundColor({ tabId, color: "#2EE6D6" });
  } catch (_) {
    /* ignore */
  }
}

function forgetTab(tabId) {
  state.media.delete(tabId);
  ext.storage.session?.remove(`media_${tabId}`).catch?.(() => {});
}
ext.tabs.onRemoved.addListener((tabId) => forgetTab(tabId));
ext.tabs.onUpdated.addListener((tabId, info) => {
  if (info.status === "loading" && info.url) {
    forgetTab(tabId);
    updateBadge(tabId);
  }
});

function parseMaster(text, base) {
  const lines = text.split(/\r?\n/);
  const out = [];
  for (let i = 0; i < lines.length; i++) {
    const l = lines[i].trim();
    if (!l.startsWith("#EXT-X-STREAM-INF:")) continue;
    const bw = /BANDWIDTH=(\d+)/i.exec(l);
    const res = /RESOLUTION=(\d+)x(\d+)/i.exec(l);
    let j = i + 1;
    while (j < lines.length && (lines[j].trim() === "" || lines[j].trim().startsWith("#"))) j++;
    if (j < lines.length) {
      out.push({ url: new URL(lines[j].trim(), base).href, bandwidth: bw ? +bw[1] : 0, label: res ? `${res[2]}p` : bw ? `${Math.round(+bw[1] / 1000)} kbps` : "Video" });
    }
  }
  return out.sort((a, b) => b.bandwidth - a.bandwidth);
}

async function listMedia(tabId) {
  let m = state.media.get(tabId);
  if (!m && ext.storage.session) {
    const saved = (await ext.storage.session.get(`media_${tabId}`).catch(() => ({})))[`media_${tabId}`];
    if (saved?.length) {
      m = new Map(saved.map((x) => [x.url, x]));
      state.media.set(tabId, m);
    }
  }
  if (!m) return [];
  const items = [...m.values()];
  const variantsOf = new Set();
  const out = [];
  for (const it of items.filter((x) => x.kind === "hls")) {
    try {
      const r = await fetch(it.url, { credentials: "include", signal: timeoutSignal(5000) });
      const t = await r.text();
      if (t.includes("#EXT-X-STREAM-INF")) {
        const vs = parseMaster(t, it.url);
        vs.forEach((v) => variantsOf.add(v.url));
        vs.forEach((v) => out.push({ url: v.url, kind: "hls", label: v.label, detail: "Video stream" }));
      } else if (t.includes("#EXT-X-KEY") && /METHOD=(SAMPLE-AES|ISO-23001)/i.test(t)) {
        out.push({ url: it.url, kind: "protected", label: "Protected", detail: "Copy-protected video" });
      } else if (!t.includes("#EXT-X-ENDLIST")) {
        out.push({ url: it.url, kind: "live", label: "Live", detail: "Live stream (can't be saved)" });
      } else {
        out.push({ url: it.url, kind: "hls", label: "Video", detail: "Video stream" });
      }
    } catch (_) {
      out.push({ url: it.url, kind: "hls", label: "Video", detail: "Video stream" });
    }
  }
  const filtered = out.filter((o, i) => !(o.kind === "hls" && o.label === "Video" && variantsOf.has(o.url)) && out.findIndex((x) => x.url === o.url) === i);
  for (const it of items.filter((x) => x.kind === "file")) {
    const type = it.mime.startsWith("audio/") ? "Audio" : (it.mime.split("/")[1] || "video").toUpperCase();
    filtered.push({ url: it.url, kind: "file", label: type, detail: it.size > 0 ? fmtSize(it.size) : it.mime, size: it.size });
  }
  return filtered;
}

function fmtSize(n) {
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`;
  if (n < 1024 ** 3) return `${(n / 1024 / 1024).toFixed(0)} MB`;
  return `${(n / 1024 ** 3).toFixed(1)} GB`;
}

// ---- messages from content scripts and the popup ----

ext.runtime.onMessage.addListener((msg, sender, reply) => {
  (async () => {
    await ready;
    switch (msg?.type) {
      case "alt":
        state.altUntil = msg.down ? Date.now() + 3000 : Date.now() + 600;
        return { ok: true };
      case "config":
        return { videoButton: state.config.videoButton && !!state.token, paired: !!state.token };
      case "getMedia":
        return listMedia(sender.tab?.id ?? msg.tabId);
      case "downloadMedia": {
        const tab = sender.tab;
        const kind = msg.item.kind === "hls" ? "hls" : "file";
        await sendDownload({ url: msg.item.url, kind, referrer: tab?.url, pageUrl: tab?.url, pageTitle: msg.title || tab?.title, filename: msg.filename || undefined });
        return { ok: true };
      }
      case "status": {
        const app = await findApp();
        if (!app) return { running: false, paired: !!state.token, paused: state.paused };
        let st = null;
        if (app.paired) st = await call("/v1/status").catch(() => null);
        else if (state.token) {
          state.token = null;
          ext.storage.local.remove("token");
        }
        return { running: true, paired: !!app.paired, paused: state.paused, status: st };
      }
      case "pair":
        await pair();
        return { ok: true };
      case "setPaused":
        state.paused = !!msg.paused;
        await ext.storage.local.set({ paused: state.paused });
        return { ok: true };
      case "show":
        await call("/v1/show", {});
        return { ok: true };
      case "mediaForTab":
        return listMedia(msg.tabId);
      case "downloadForTab": {
        const tab = await ext.tabs.get(msg.tabId);
        await sendDownload({ url: msg.item.url, kind: msg.item.kind === "hls" ? "hls" : "file", referrer: tab?.url, pageUrl: tab?.url, pageTitle: tab?.title });
        return { ok: true };
      }
    }
    return null;
  })()
    .then(reply)
    .catch((e) => reply({ error: e.message || String(e) }));
  return true;
});

// Refresh settings from the app now and then.
setInterval(() => {
  if (state.token) refreshConfig();
}, 60000);
ready.then(() => state.token && refreshConfig());
