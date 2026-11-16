// Sample data used when the UI runs in a normal browser (design preview and
// development). Inside the app, the real Go backend is used instead.
import type { Download, Live, Queue, Snapshot, Stats } from "./types";

type Listener = (data: any) => void;
const listeners: Record<string, Listener[]> = {};
const MB = 1 << 20;
const GB = 1 << 30;

function emit(ev: string, data: any) {
  (listeners[ev] || []).forEach((cb) => cb(data));
}

const now = Date.now();
const iso = (minsAgo: number) => new Date(now - minsAgo * 60000).toISOString();

function dl(p: Partial<Download> & { id: string; filename: string; category: string; size: number }): Download {
  return {
    url: `https://${p.id}.example.com/files/${p.filename}`,
    kind: "file",
    dir: `D:\\Downloads\\ForgeDM\\${p.category}`,
    downloaded: 0,
    resumable: true,
    status: "queued",
    queue: "main",
    connections: 16,
    checksum: {},
    post: { verify: true, scan: false, extract: false, open: false },
    createdAt: iso(30),
    activeMs: 0,
    savedMs: 0,
    tags: [],
    ...p,
  } as Download;
}

const downloads: Download[] = [
  dl({ id: "a1", filename: "ubuntu-26.04-desktop-amd64.iso", category: "Programs", size: 5.81 * GB, downloaded: 2.21 * GB, status: "downloading", url: "https://releases.ubuntu.com/26.04/ubuntu-26.04-desktop-amd64.iso", tags: ["iso"], rule: "ISOs" }),
  dl({ id: "a2", filename: "Nature_Documentary_4K_E03.mkv", category: "Video", size: 12.4 * GB, downloaded: 8.8 * GB, status: "downloading" }),
  dl({ id: "a3", filename: "Project_Assets_v12.zip", category: "Compressed", size: 2.1 * GB, downloaded: 0.46 * GB, status: "downloading", post: { verify: true, scan: false, extract: true, open: false } }),
  dl({ id: "a4", filename: "Lofi_Sessions_Vol2.flac", category: "Music", size: 640 * MB, downloaded: 576 * MB, status: "downloading", connections: 4 }),
  dl({ id: "q1", filename: "StudioSuite-2026-x64-setup.exe", category: "Programs", size: 1.84 * GB, status: "queued" }),
  dl({ id: "q2", filename: "Nature_Documentary_4K_E04.mkv", category: "Video", size: 12.1 * GB, status: "queued", queue: "night" }),
  dl({ id: "q3", filename: "game-assets-pack-2.7z", category: "Compressed", size: 6.8 * GB, status: "queued", queue: "night" }),
  dl({ id: "p1", filename: "dataset_images_part3.tar.gz", category: "Compressed", size: 8.6 * GB, downloaded: 4.64 * GB, status: "paused" }),
  dl({ id: "f1", filename: "Conference_Keynote_2026.mp4", category: "Video", size: 3.2 * GB, downloaded: 0.38 * GB, status: "failed", error: "The link has expired or needs you to sign in again. Refresh the link to continue.", errorCode: "link_expired" }),
  dl({ id: "c1", filename: "Quarterly_Report_Q3.pdf", category: "Documents", size: 12.4 * MB, downloaded: 12.4 * MB, status: "completed", completedAt: iso(105), postNote: "SHA-256 calculated", savedMs: 2000 }),
  dl({ id: "c2", filename: "design-kit-2026.zip", category: "Compressed", size: 840 * MB, downloaded: 840 * MB, status: "completed", completedAt: iso(160), postNote: "Extracted to D:\\Downloads\\ForgeDM\\Compressed\\design-kit-2026", savedMs: 90000 }),
  dl({ id: "c3", filename: "Wallpaper_Mountains_8K.png", category: "Images", size: 46 * MB, downloaded: 46 * MB, status: "completed", completedAt: iso(1500), source: "sync" }),
  dl({ id: "c4", filename: "node-v24-x64.msi", category: "Programs", size: 31 * MB, downloaded: 31 * MB, status: "completed", completedAt: iso(1560), checksum: { algo: "sha256", verified: true }, postNote: "SHA-256 verified" }),
  dl({ id: "c5", filename: "Podcast_Episode_142.mp3", category: "Music", size: 84 * MB, downloaded: 84 * MB, status: "completed", completedAt: iso(12) }),
];

const speeds: Record<string, number> = { a1: 21.4 * MB, a2: 15.8 * MB, a3: 7.9 * MB, a4: 3.5 * MB };
const history: number[] = [];
for (let i = 0; i < 60; i++) history.push((40 + Math.sin(i / 5) * 8 + Math.random() * 6) * MB);

const queues: Queue[] = [
  { id: "main", name: "Main queue", builtin: true, enabled: true, start: "", stop: "", days: [], concurrent: 4, speedLimit: 0, halfBandwidth: false, whenDone: "nothing", wakeFromSleep: false, retryFailed: true, pauseOnMetered: false, color: "#2EE6D6", icon: "bolt", order: [], active: true },
  { id: "night", name: "Night queue", builtin: false, enabled: true, start: "01:00", stop: "07:00", days: [1, 2, 3, 4, 5], concurrent: 3, speedLimit: 0, halfBandwidth: false, whenDone: "shutdown", wakeFromSleep: true, retryFailed: true, pauseOnMetered: true, color: "#A78BFA", icon: "moon", order: [], active: false, nextRun: new Date(new Date().setHours(25, 0, 0, 0)).toISOString() },
  { id: "weekend", name: "Weekend backups", builtin: false, enabled: true, start: "09:00", stop: "13:00", days: [6], concurrent: 2, speedLimit: 0, halfBandwidth: true, whenDone: "nothing", wakeFromSleep: false, retryFailed: true, pauseOnMetered: false, color: "#5B8CFF", icon: "box", order: [], active: false },
];

function stats(): Stats {
  const week = [8.1, 21.7, 15.2, 26.9, 31.4, 12.4, 18.4].map((g, i) => ({
    date: new Date(now - (6 - i) * 86400000).toISOString().slice(0, 10),
    bytes: g * GB,
    files: [6, 14, 9, 17, 21, 7, 23][i],
    savedMs: [1.2, 2.5, 1.9, 3.1, 4.0, 1.1, 2.23][i] * 3600000,
  }));
  return {
    series: { "1m": history.slice(-60), "10m": history.map((v) => v * 0.95), "1h": history.map((v) => v * 0.9), "24h": history.concat(history).slice(0, 96).map((v, i) => v * (0.3 + (i % 24) / 30)) },
    week,
    today: week[6],
    yesterday: week[5],
    peak: 56.7 * MB,
  };
}

function snapshot(): Snapshot {
  const cats = ["Video", "Music", "Documents", "Compressed", "Programs", "Images", "Other"].map((name) => {
    const list = downloads.filter((d) => d.category === name);
    const extra: Record<string, [number, number]> = { Video: [30, 640 * GB], Music: [17, 120 * GB], Documents: [16, 98 * GB], Compressed: [22, 190 * GB], Programs: [8, 250 * GB], Images: [15, 84 * GB], Other: [0, 0] };
    return { name, count: list.length + extra[name][0], bytes: list.reduce((a, d) => a + d.size, 0) + extra[name][1], dir: `D:\\Downloads\\ForgeDM\\${name}`, exts: [] };
  });
  return {
    downloads: downloads.map((d) => ({ ...d })),
    settings: {
      baseDir: "D:\\Downloads\\ForgeDM", categoryDirs: {}, maxActive: 4, defaultConnections: 0, speedLimitOn: false, speedLimit: 5 * MB,
      proxyMode: "system", proxyUrl: "", userAgent: "", retryCount: 5, smartPriority: true, captureEnabled: true,
      captureExts: ["exe", "msi", "zip", "7z", "rar", "iso", "mp4", "mkv", "mp3", "flac", "pdf", "apk"], captureMinSize: 0, askOnCapture: true,
      clipboardWatch: true, videoButton: true, altBypass: true, verifyChecksum: true, scanAfter: false, openWhenDone: false, deleteArchive: false,
      closeToTray: true, startWithSystem: false, keepAwake: true, notifications: true, dropTarget: false, theme: "midnight", accent: "#2EE6D6",
      mica: false, density: "comfortable", apiPort: 52771, tags: ["iso", "work", "watch-later", "backup"], onboarded: true,
    },
    rules: [
      { id: "r1", name: "ISOs", enabled: true, conditions: [{ field: "ext", value: "iso, img" }], actions: { dir: "D:\\Software\\ISOs", tags: ["iso"] } },
      { id: "r2", name: "Installers from example.com", enabled: true, conditions: [{ field: "site", value: "downloads.example.com" }, { field: "type", value: "Programs" }], actions: { dir: "D:\\Software\\Installers" } },
      { id: "r3", name: "Videos", enabled: true, conditions: [{ field: "type", value: "Video" }], actions: { dir: "D:\\Videos", open: true } },
      { id: "r4", name: "Unpack archives", enabled: true, conditions: [{ field: "type", value: "Compressed" }], actions: { extract: true } },
      { id: "r5", name: "Big files at night", enabled: true, conditions: [{ field: "sizeOver", value: String(4 * GB) }], actions: { queue: "night" } },
    ],
    queues: queues.map((q) => ({ ...q, count: downloads.filter((d) => d.queue === q.id && d.status !== "completed").length, bytes: downloads.filter((d) => d.queue === q.id && d.status !== "completed").reduce((a, d) => a + d.size - d.downloaded, 0) })),
    projects: [
      { id: "p1", name: "Mountain wallpapers", url: "https://gallery.example.org/collections/mountains", depth: 1, sameSite: true, types: ["Images"], exts: "", minSize: MB, dir: "D:\\Pictures\\Wallpapers", keepStructure: true, syncEnabled: true, syncDay: 0, syncTime: "09:00", lastRun: iso(1500), lastFound: 12 },
    ],
    categories: cats,
    browsers: [
      { id: "b1", name: "Google Chrome", pairedAt: iso(9000), lastSeen: iso(2) },
      { id: "b2", name: "Microsoft Edge", pairedAt: iso(7000), lastSeen: iso(400) },
    ],
    stats: stats(),
    diskFree: 412 * GB,
    diskTotal: 1000 * GB,
    port: 52771,
    version: "1.0.0",
    dataDir: "C:\\Users\\you\\AppData\\Roaming\\ForgeDM",
  };
}

function segMap(d: Download): number[] {
  const frac = d.size > 0 ? d.downloaded / d.size : 0;
  return Array.from({ length: 32 }, (_, i) => {
    const base = frac * 1.6 - (i % 16) * 0.05;
    return Math.max(0, Math.min(1, base + ((i * 37) % 10) / 100));
  });
}

let ticking = false;
function startTicks() {
  if (ticking) return;
  ticking = true;
  setInterval(() => {
    const live: Live[] = [];
    let total = 0;
    for (const d of downloads) {
      if (d.status !== "downloading") continue;
      const s = speeds[d.id] * (0.85 + Math.random() * 0.3);
      d.downloaded = Math.min(d.size, d.downloaded + s);
      total += s;
      if (d.downloaded >= d.size) {
        d.status = "completed";
        d.completedAt = new Date().toISOString();
        emit("state", snapshot());
        continue;
      }
      live.push({ id: d.id, status: d.status, downloaded: d.downloaded, size: d.size, speed: s, eta: (d.size - d.downloaded) / s, conns: d.connections, map: segMap(d), peak: s * 1.2, history: history.slice(-40).map((v) => (v * s) / (48 * MB)) });
    }
    history.push(total);
    history.shift();
    emit("tick", { live, speed: total, active: live.length, stats: stats() });
  }, 1000);
}

function setStatus(ids: string[], status: Download["status"]) {
  for (const d of downloads) if (ids.includes(d.id) && d.status !== "completed") d.status = status;
  emit("state", snapshot());
}

export const mock = {
  on(event: string, cb: Listener) {
    (listeners[event] ||= []).push(cb);
    startTicks();
    return () => {
      listeners[event] = (listeners[event] || []).filter((x) => x !== cb);
    };
  },
  async call(name: string, args: any[]): Promise<any> {
    switch (name) {
      case "GetSnapshot":
        return snapshot();
      case "Probe": {
        const url: string = args[0];
        const filename = decodeURIComponent(url.split("?")[0].split("/").pop() || "download");
        await new Promise((r) => setTimeout(r, 500));
        const ext = filename.split(".").pop()?.toLowerCase() || "";
        const cat = ["exe", "msi", "iso"].includes(ext) ? "Programs" : ["mp4", "mkv"].includes(ext) ? "Video" : ["zip", "7z", "rar"].includes(ext) ? "Compressed" : "Other";
        const dup = downloads.find((d) => d.url === url);
        return {
          url, finalUrl: url, filename, size: 1.84 * GB, resumable: true, contentType: "application/octet-stream", kind: "file",
          decision: { category: cat, dir: cat === "Programs" ? "D:\\Software\\Installers" : `D:\\Downloads\\ForgeDM\\${cat}`, tags: null, queue: "main", extract: cat === "Compressed", open: false, ruleId: "r2", ruleName: cat === "Programs" ? "Installers from example.com" : "" },
          duplicate: dup,
        };
      }
      case "Add": {
        const o = args[0];
        const d = dl({ id: Math.random().toString(36).slice(2, 8), filename: o.filename || "download.bin", category: o.category || "Other", size: o.size || 500 * MB, url: o.url, status: o.start ? "downloading" : "queued", queue: o.queue || "main", dir: o.dir });
        speeds[d.id] = 12 * MB;
        downloads.unshift(d);
        emit("state", snapshot());
        return d;
      }
      case "Pause":
        return setStatus(args[0], "paused");
      case "Resume":
        args[0].forEach((id: string) => (speeds[id] ||= 10 * MB));
        return setStatus(args[0], "downloading");
      case "PauseAll":
        return setStatus(downloads.map((d) => d.id), "paused");
      case "ResumeAll":
        return setStatus(downloads.filter((d) => d.status === "paused" || d.status === "failed").map((d) => d.id), "downloading");
      case "Remove": {
        const ids: string[] = args[0];
        for (let i = downloads.length - 1; i >= 0; i--) if (ids.includes(downloads[i].id)) downloads.splice(i, 1);
        emit("state", snapshot());
        return;
      }
      case "GetDetails":
        return downloads.find((d) => d.id === args[0]);
      case "StartScan": {
        const id = "scan1";
        let pages = 0;
        const t = setInterval(() => {
          pages += 7;
          emit("grabber", { id, progress: { pages, found: pages * 6, sized: 0, stage: "pages", current: args[0].url } });
          if (pages >= 42) {
            clearInterval(t);
            const names = ["alpine-ridge-01.jpg", "glacier-dawn.jpg", "summit-fog-4k.png", "valley-light.jpg", "north-face-night.webp", "peak-reflection.jpg", "misty-pines.jpg", "ridge-sunset-8k.png", "snowfield-blue.jpg", "canyon-edge.jpg", "lake-mirror.webp", "high-pass.jpg", "trail-guide.pdf", "drone-flyover.mp4"];
            const items = names.map((n, i) => ({ url: `https://gallery.example.org/img/${n}`, name: n, type: n.endsWith(".pdf") ? "Documents" : n.endsWith(".mp4") ? "Video" : "Images", ext: n.split(".").pop(), size: [4.2, 9.8, 12.6, 2.4, 3.1, 28.4, 2.2, 31, 4.6, 2.7, 7.9, 5, 1.1, 88][i] * MB, page: args[0].url, path: "img", have: i === 3 }));
            emit("grabber:done", { id, items, error: "", seconds: 14 });
          }
        }, 400);
        return id;
      }
      case "ChooseFolder":
        return "D:\\Downloads\\Picked";
      case "SaveQueue":
        return args[0];
      case "ExtensionFolder":
        return "";
      default:
        return undefined;
    }
  },
};
