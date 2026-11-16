import { useEffect, useState } from "react";
import { on } from "./api";
import { Sidebar, StatusBar, TitleBar } from "./components/shell";
import { Toasts } from "./components/ui";
import { AddDownload } from "./dialogs/AddDownload";
import { Details } from "./dialogs/Details";
import { BatchDialog, CaptureDialog, ClipboardBanner, ConnectDialog, ImportDialog, PairDialog, RefreshDialog, RemoveDialog, RuleDialog } from "./dialogs/Other";
import { Dashboard } from "./pages/Dashboard";
import { Downloads } from "./pages/Downloads";
import { Grabber } from "./pages/Grabber";
import { Scheduler } from "./pages/Scheduler";
import { Settings } from "./pages/Settings";
import { useStore } from "./store";
import type { BatchLink, PendingCapture } from "./types";

function inkFor(hex: string): string {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex);
  if (!m) return "#03201D";
  const n = parseInt(m[1], 16);
  const [r, g, b] = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((v) => {
    const c = v / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  const L = 0.2126 * r + 0.7152 * g + 0.0722 * b;
  return L > 0.35 ? "#06161A" : "#FFFFFF";
}

function useTheme() {
  const { snap } = useStore();
  const s = snap?.settings;
  useEffect(() => {
    if (!s) return;
    const root = document.documentElement;
    const apply = () => {
      const dark = window.matchMedia("(prefers-color-scheme: dark)").matches;
      root.dataset.theme = s.theme === "system" ? (dark ? "midnight" : "daylight") : s.theme || "midnight";
    };
    apply();
    root.dataset.density = s.density || "comfortable";
    const accent = s.accent || "#2EE6D6";
    root.style.setProperty("--accent", accent);
    root.style.setProperty("--accent-ink", inkFor(accent));
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    mq.addEventListener("change", apply);
    return () => mq.removeEventListener("change", apply);
  }, [s?.theme, s?.density, s?.accent, s]);
}

export function App() {
  const { page, dialog, close, open, snap } = useStore();
  const [captures, setCaptures] = useState<PendingCapture[]>([]);
  const [pairs, setPairs] = useState<{ id: string; name: string }[]>([]);
  const [batches, setBatches] = useState<{ id: string; pageUrl: string; links: BatchLink[]; browser: string }[]>([]);
  const [clip, setClip] = useState<{ url: string; name: string; size: number } | null>(null);
  const [dragOver, setDragOver] = useState(false);
  useTheme();

  useEffect(() => {
    const offs = [
      on<PendingCapture>("capture", (pc) => setCaptures((l) => (l.some((x) => x.id === pc.id) ? l.map((x) => (x.id === pc.id ? pc : x)) : [...l, pc]))),
      on<{ id: string; name: string }>("pair", (p) => setPairs((l) => [...l.filter((x) => x.id !== p.id), p])),
      on<string>("pair:closed", (id) => setPairs((l) => l.filter((x) => x.id !== id))),
      on<{ id: string; pageUrl: string; links: BatchLink[]; browser: string }>("batch", (b) => setBatches((l) => [...l, b])),
      on<{ url: string; name: string; size: number }>("clipboard", (c) => setClip(c)),
      on<string>("open:add", (url) => open({ kind: "add", url: url || undefined })),
    ];
    return () => offs.forEach((f) => f());
  }, [open]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "n") {
        e.preventDefault();
        open({ kind: "add" });
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  // Dropping a link anywhere on the window opens the New download dialog.
  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    const text = e.dataTransfer.getData("text/uri-list") || e.dataTransfer.getData("text/plain");
    const link = text.split(/\s+/).find((s) => /^https?:\/\//i.test(s));
    if (link) open({ kind: "add", url: link });
  };

  if (!snap) {
    return (
      <div className="app">
        <TitleBar />
        <div className="empty" style={{ flex: 1 }}>Loading ForgeDM…</div>
      </div>
    );
  }

  const pair = pairs[0];
  const capture = !pair ? captures[0] : undefined;
  const batch = !pair && !capture ? batches[0] : undefined;

  return (
    <div
      className="app"
      onDragOver={(e) => {
        if (Array.from(e.dataTransfer.types).some((t) => t === "text/uri-list" || t === "text/plain")) {
          e.preventDefault();
          setDragOver(true);
        }
      }}
      onDragLeave={(e) => e.currentTarget === e.target && setDragOver(false)}
      onDrop={onDrop}
    >
      <TitleBar />
      <div className="body">
        <Sidebar />
        <main className="main" style={dragOver ? { outline: "2px dashed var(--accent)", outlineOffset: -8 } : undefined}>
          {clip && <ClipboardBanner clip={clip} onDismiss={() => setClip(null)} />}
          {page === "dashboard" && <Dashboard />}
          {page === "downloads" && <Downloads />}
          {page === "scheduler" && <Scheduler />}
          {page === "grabber" && <Grabber />}
          {page === "settings" && <Settings />}
        </main>
      </div>
      <StatusBar />
      <Toasts />

      {dialog?.kind === "add" && <AddDownload initialUrl={dialog.url} onClose={close} />}
      {dialog?.kind === "details" && <Details id={dialog.id} onClose={close} />}
      {dialog?.kind === "refresh" && <RefreshDialog id={dialog.id} onClose={close} />}
      {dialog?.kind === "remove" && <RemoveDialog ids={dialog.ids} onClose={close} />}
      {dialog?.kind === "import" && <ImportDialog onClose={close} />}
      {dialog?.kind === "rule" && <RuleDialog id={dialog.id} onClose={close} />}
      {dialog?.kind === "connect" && <ConnectDialog onClose={close} />}

      {pair && <PairDialog req={pair} onDone={() => setPairs((l) => l.slice(1))} />}
      {capture && <CaptureDialog key={capture.id} pc={capture} onDone={() => setCaptures((l) => l.filter((x) => x.id !== capture.id))} />}
      {batch && <BatchDialog key={batch.id} batch={batch} onDone={() => setBatches((l) => l.slice(1))} />}
    </div>
  );
}
