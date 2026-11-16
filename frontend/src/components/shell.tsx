import { api, win } from "../api";
import { fmtBytes, fmtSpeed } from "../format";
import { Icon, Logo, catStyle, type IconName } from "../icons";
import { useStore, type Page } from "../store";
import { Switch } from "./ui";

export function TitleBar() {
  return (
    <header className="titlebar">
      <Logo />
      <span className="brand">ForgeDM</span>
      <span className="faint" style={{ fontSize: 12 }}>Download Manager</span>
      <div className="controls">
        <button aria-label="Minimize" onClick={() => win.minimise()}>
          <Icon name="min" size={14} />
        </button>
        <button aria-label="Maximize" onClick={() => win.toggleMaximise()}>
          <Icon name="max" size={13} />
        </button>
        <button aria-label="Close" className="close" onClick={() => api.closeWindow()}>
          <Icon name="x" size={14} />
        </button>
      </div>
    </header>
  );
}

const NAV: { page: Page; label: string; icon: IconName }[] = [
  { page: "dashboard", label: "Dashboard", icon: "dash" },
  { page: "downloads", label: "Downloads", icon: "down" },
  { page: "scheduler", label: "Queues & Scheduler", icon: "cal" },
  { page: "grabber", label: "Site Grabber", icon: "globe" },
  { page: "settings", label: "Settings", icon: "sliders" },
];

const queueIcon: Record<string, IconName> = { bolt: "bolt", moon: "moon", box: "box", sync: "sync" };

export function Sidebar() {
  const { snap, page, go, filter, open, tick } = useStore();
  const active = tick?.active ?? 0;
  const s = snap?.settings;
  const usedPct = snap && snap.diskTotal > 0 ? Math.round(((snap.diskTotal - snap.diskFree) / snap.diskTotal) * 100) : 0;
  return (
    <nav className="sidebar" aria-label="Main">
      <button className="btn primary" style={{ height: 44, borderRadius: 12 }} onClick={() => open({ kind: "add" })}>
        <Icon name="plus" stroke={2.2} />
        New download
      </button>
      <div className="stack" style={{ gap: 2 }}>
        {NAV.map((n) => (
          <button key={n.page} className="nav-item" aria-current={page === n.page && !(n.page === "downloads" && (filter.category || filter.queue)) ? "page" : undefined} onClick={() => go(n.page)}>
            <Icon name={n.icon} />
            <span>{n.label}</span>
            {n.page === "downloads" && active > 0 && <span className="badge">{active}</span>}
          </button>
        ))}
      </div>
      <div className="stack" style={{ gap: 2 }}>
        <div className="side-head">Categories</div>
        {(snap?.categories ?? [])
          .filter((c) => c.name !== "Other" || c.count > 0)
          .map((c) => (
            <button key={c.name} className="side-link" aria-current={page === "downloads" && filter.category === c.name ? "true" : undefined} onClick={() => go("downloads", { category: c.name })}>
              <span style={{ width: 8, height: 8, borderRadius: 3, background: catStyle(c.name).color, flex: "none" }} />
              <span>{c.name}</span>
              <span className="count">{c.count}</span>
            </button>
          ))}
      </div>
      <div className="stack" style={{ gap: 2 }}>
        <div className="side-head">Queues</div>
        {(snap?.queues ?? []).map((q) => (
          <button key={q.id} className="side-link" aria-current={page === "downloads" && filter.queue === q.id ? "true" : undefined} onClick={() => go("downloads", { queue: q.id })}>
            <span style={{ color: q.color, display: "grid" }}>
              <Icon name={queueIcon[q.icon] ?? "queue"} size={14} />
            </span>
            <span className="ellipsis">{q.name}</span>
            <span className="count" style={{ fontFamily: "inherit" }}>{q.active ? "Running" : q.count ? String(q.count) : ""}</span>
          </button>
        ))}
      </div>
      <div className="card tight" style={{ marginTop: "auto", display: "flex", flexDirection: "column", gap: 12 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
          <span style={{ fontSize: 13, fontWeight: 500 }}>
            Speed limit
            <span className="faint" style={{ display: "block", fontSize: 12, fontWeight: 400 }}>{s ? fmtSpeed(s.speedLimit) : ""}</span>
          </span>
          <Switch on={!!s?.speedLimitOn} onChange={(v) => api.setSpeedLimiter(v)} label="Speed limit" />
        </div>
        {snap && snap.diskTotal > 0 && (
          <div className="stack" style={{ gap: 6 }}>
            <div className="muted" style={{ display: "flex", justifyContent: "space-between", fontSize: 12 }}>
              <span>Free space</span>
              <span>{fmtBytes(snap.diskFree)} of {fmtBytes(snap.diskTotal, 0)}</span>
            </div>
            <div className="bar">
              <span style={{ width: `${usedPct}%`, background: usedPct > 90 ? "var(--red)" : "var(--blue)" }} />
            </div>
          </div>
        )}
      </div>
    </nav>
  );
}

export function StatusBar() {
  const { snap, tick } = useStore();
  const recent = (snap?.browsers ?? []).filter((b) => Date.now() - new Date(b.lastSeen).getTime() < 15 * 60000);
  const names = recent.map((b) => b.name.replace(/^(Google|Microsoft|Mozilla) /, ""));
  return (
    <footer className="statusbar">
      <span style={{ display: "flex", alignItems: "center", gap: 6 }}>
        <span className="dot" style={{ background: names.length ? "var(--accent)" : "var(--faint)" }} />
        {names.length ? `${names.join(" and ")} connected` : (snap?.browsers.length ?? 0) > 0 ? "Browser extension paired" : "No browser connected"}
      </span>
      <span>↓ {fmtSpeed(tick?.speed) === "—" ? "0 B/s" : fmtSpeed(tick?.speed)}</span>
      <span>Speed limit: {snap?.settings.speedLimitOn ? fmtSpeed(snap.settings.speedLimit) : "off"}</span>
      <span style={{ marginLeft: "auto" }}>ForgeDM {snap?.version ?? ""}</span>
    </footer>
  );
}
