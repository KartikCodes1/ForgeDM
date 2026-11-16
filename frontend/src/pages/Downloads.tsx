import { useEffect, useMemo, useState } from "react";
import { api, win } from "../api";
import { useMenu, type MenuItem } from "../components/ui";
import { fmtBytes, fmtDuration, fmtSpeed, fmtWhen, host, pct } from "../format";
import { FileIcon, Icon, catStyle, type IconName } from "../icons";
import { useAction, useStore } from "../store";
import type { Download, Status } from "../types";

const STATUSES: { key: "all" | Status; label: string }[] = [
  { key: "all", label: "All" },
  { key: "downloading", label: "Downloading" },
  { key: "queued", label: "Queued" },
  { key: "paused", label: "Paused" },
  { key: "completed", label: "Completed" },
  { key: "failed", label: "Failed" },
];

const statusLabel: Record<Status, string> = { downloading: "Downloading", queued: "Queued", paused: "Paused", completed: "Completed", failed: "Failed" };
const barColor: Record<Status, string> = { downloading: "var(--accent)", queued: "var(--border2)", paused: "var(--yellow)", failed: "var(--red)", completed: "var(--green)" };

type SortKey = "added" | "name" | "size" | "progress" | "speed";

export function Downloads() {
  const { snap, live, filter, setFilter, open } = useStore();
  const act = useAction();
  const [status, setStatus] = useState<"all" | Status>("all");
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<SortKey>("added");
  const [sel, setSel] = useState<string[]>([]);
  const [anchor, setAnchor] = useState<string | null>(null);
  const menu = useMenu();
  const all = snap?.downloads ?? [];
  const queues = snap?.queues ?? [];

  const scoped = useMemo(
    () =>
      all.filter(
        (d) =>
          (!filter.category || d.category === filter.category) &&
          (!filter.queue || d.queue === filter.queue) &&
          (!filter.tag || (d.tags ?? []).includes(filter.tag)),
      ),
    [all, filter],
  );

  const rows = useMemo(() => {
    const q = query.trim().toLowerCase();
    let list = scoped.filter((d) => (status === "all" || d.status === status) && (!q || d.filename.toLowerCase().includes(q) || d.url.toLowerCase().includes(q) || (d.tags ?? []).some((t) => t.toLowerCase().includes(q))));
    const val = (d: Download) => {
      switch (sort) {
        case "name":
          return d.filename.toLowerCase();
        case "size":
          return d.size;
        case "progress":
          return d.size > 0 ? (live[d.id]?.downloaded ?? d.downloaded) / d.size : 0;
        case "speed":
          return live[d.id]?.speed ?? 0;
        default:
          return d.createdAt;
      }
    };
    list = [...list].sort((a, b) => {
      const x = val(a);
      const y = val(b);
      const r = x < y ? -1 : x > y ? 1 : 0;
      return sort === "name" ? r : -r;
    });
    return list;
  }, [scoped, status, query, sort, live]);

  useEffect(() => {
    setSel((s) => s.filter((id) => all.some((d) => d.id === id)));
  }, [all]);

  const counts = useMemo(() => {
    const c: Record<string, number> = { all: scoped.length };
    for (const d of scoped) c[d.status] = (c[d.status] ?? 0) + 1;
    return c;
  }, [scoped]);

  const selected = all.filter((d) => sel.includes(d.id));
  const one = selected.length === 1 ? selected[0] : null;

  const click = (e: React.MouseEvent, d: Download) => {
    if (e.shiftKey && anchor) {
      const ids = rows.map((r) => r.id);
      const [a, b] = [ids.indexOf(anchor), ids.indexOf(d.id)].sort((x, y) => x - y);
      setSel(ids.slice(a, b + 1));
    } else if (e.ctrlKey || e.metaKey) {
      setSel((s) => (s.includes(d.id) ? s.filter((x) => x !== d.id) : [...s, d.id]));
      setAnchor(d.id);
    } else {
      setSel([d.id]);
      setAnchor(d.id);
    }
  };

  const toggle = (id: string) => setSel((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]));

  const contextItems = (ds: Download[]): MenuItem[] => {
    const ids = ds.map((d) => d.id);
    const d = ds[0];
    const items: MenuItem[] = [];
    if (ds.length === 1 && d.status === "completed") {
      items.push({ label: "Open", icon: "external", onClick: () => act(api.openFile(d.id)) });
      items.push({ label: "Show in folder", icon: "folder", onClick: () => act(api.showInFolder(d.id)) });
    }
    if (ds.some((x) => x.status !== "completed" && x.status !== "downloading")) items.push({ label: "Start now", icon: "play", onClick: () => api.resume(ids) });
    if (ds.some((x) => x.status === "downloading" || x.status === "queued")) items.push({ label: "Pause", icon: "pause", onClick: () => api.pause(ids) });
    if (ds.length === 1) items.push({ label: "Progress and details", icon: "info", onClick: () => open({ kind: "details", id: d.id }) });
    if (ds.length === 1 && d.status === "failed") items.push({ label: "Refresh link…", icon: "refresh", onClick: () => open({ kind: "refresh", id: d.id }) });
    if (ds.length === 1 && d.status === "completed") items.push({ label: "Download again", icon: "refresh", onClick: () => act(api.redownload(d.id)) });
    items.push({ label: "Copy link", icon: "copy", onClick: () => win.copy(ds.map((x) => x.url).join("\n")) });
    items.push({ sep: true, label: "", onClick: () => {} });
    for (const q of queues) {
      if (ds.every((x) => x.queue === q.id)) continue;
      items.push({ label: `Move to ${q.name}`, icon: "queue", onClick: () => api.moveToQueue(ids, q.id) });
    }
    items.push({ sep: true, label: "", onClick: () => {} });
    items.push({ label: ds.length > 1 ? `Remove ${ds.length} downloads…` : "Remove…", icon: "trash", danger: true, onClick: () => open({ kind: "remove", ids }) });
    return items;
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT")) return;
      if (e.key === "Delete" && sel.length) open({ kind: "remove", ids: sel });
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "a") {
        e.preventDefault();
        setSel(rows.map((r) => r.id));
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [sel, rows, open]);

  const totalBytes = scoped.reduce((a, d) => a + Math.max(0, d.size), 0);
  const scopeLabel = filter.category ? filter.category : filter.queue ? queues.find((q) => q.id === filter.queue)?.name : filter.tag ? `#${filter.tag}` : "";

  const tools: { label: string; icon: IconName; onClick: () => void; disabled?: boolean; danger?: boolean }[] = [
    { label: "Start", icon: "play", disabled: !selected.some((d) => d.status !== "completed" && d.status !== "downloading"), onClick: () => api.resume(sel) },
    { label: "Pause", icon: "pause", disabled: !selected.some((d) => d.status === "downloading" || d.status === "queued"), onClick: () => api.pause(sel) },
    { label: "Pause all", icon: "stop", onClick: () => api.pauseAll() },
    { label: "Refresh link", icon: "refresh", disabled: !(one && one.status !== "completed"), onClick: () => one && open({ kind: "refresh", id: one.id }) },
    { label: "Delete", icon: "trash", danger: true, disabled: sel.length === 0, onClick: () => open({ kind: "remove", ids: sel }) },
  ];

  return (
    <>
      <div className="page-head">
        <div style={{ flex: "1 1 260px" }}>
          <h1>{scopeLabel || "Downloads"}</h1>
          <p>
            {scoped.length} files · {fmtBytes(totalBytes)}
            {scopeLabel && (
              <>
                {" · "}
                <button className="btn ghost xs" style={{ verticalAlign: "middle" }} onClick={() => setFilter({})}>Show all downloads</button>
              </>
            )}
          </p>
        </div>
        <label style={{ position: "relative", display: "flex", alignItems: "center", flex: "0 1 280px" }}>
          <span className="sr-only">Search downloads</span>
          <span className="faint" style={{ position: "absolute", left: 12, display: "grid" }}>
            <Icon name="search" size={16} stroke={2} />
          </span>
          <input className="input" type="search" placeholder="Search name, site or tag" value={query} onChange={(e) => setQuery(e.target.value)} style={{ height: 40, paddingLeft: 36 }} />
        </label>
        <button className="btn" onClick={() => open({ kind: "import" })}>
          <Icon name="doc" size={16} />Import list
        </button>
        <button className="btn primary" onClick={() => open({ kind: "add" })}>
          <Icon name="plus" size={16} stroke={2.2} />Add link
        </button>
      </div>

      <div role="tablist" aria-label="Filter by status" style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
        {STATUSES.map((s) => (
          <button key={s.key} role="tab" aria-selected={status === s.key} className="filter-chip" onClick={() => setStatus(s.key)}>
            {s.label}
            <span className="n">{counts[s.key] ?? 0}</span>
          </button>
        ))}
      </div>

      <div role="toolbar" aria-label="Download actions" className="card tight" style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 6, padding: 6, borderRadius: 12 }}>
        {tools.map((t) => (
          <button key={t.label} className={`tool${t.danger ? " danger" : ""}`} disabled={t.disabled} onClick={t.onClick}>
            <Icon name={t.icon} size={16} stroke={1.9} />
            {t.label}
          </button>
        ))}
        <button className="tool" disabled={sel.length === 0} onClick={(e) => {
          const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
          menu.open(r.left, r.bottom + 4, queues.map((q) => ({ label: q.name, icon: "queue" as IconName, onClick: () => api.moveToQueue(sel, q.id) })));
        }}>
          <Icon name="queue" size={16} />Move to queue
          <Icon name="chevDown" size={14} />
        </button>
        <div style={{ flex: 1 }} />
        <label className="muted" style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13 }}>
          Sort
          <select className="select" style={{ height: 34, width: "auto" }} value={sort} onChange={(e) => setSort(e.target.value as SortKey)}>
            <option value="added">Date added</option>
            <option value="name">Name</option>
            <option value="size">Size</option>
            <option value="progress">Progress</option>
            <option value="speed">Speed</option>
          </select>
        </label>
      </div>

      <div className="table-wrap">
        <div role="table" aria-label="Downloads" aria-rowcount={rows.length}>
          <div role="row" className="trow head">
            <span role="columnheader" style={{ display: "grid", placeItems: "center" }}>
              <input type="checkbox" aria-label="Select all" checked={rows.length > 0 && sel.length === rows.length} onChange={(e) => setSel(e.target.checked ? rows.map((r) => r.id) : [])} />
            </span>
            <span role="columnheader">File</span>
            <span role="columnheader">Size</span>
            <span role="columnheader">Progress</span>
            <span role="columnheader">Speed</span>
            <span role="columnheader">Time left</span>
            <span role="columnheader">Status</span>
            <span role="columnheader">Added</span>
            <span role="columnheader"><span className="sr-only">Actions</span></span>
          </div>
          {rows.length === 0 && (
            <div className="empty">
              <div className="big">{all.length === 0 ? "No downloads yet" : "Nothing matches"}</div>
              <span>{all.length === 0 ? "Add a link, or click a download link in your browser once the extension is connected." : "Try another filter or search."}</span>
              {all.length === 0 && (
                <button className="btn primary" onClick={() => open({ kind: "add" })}>
                  <Icon name="plus" size={16} stroke={2.2} />Add a link
                </button>
              )}
            </div>
          )}
          {rows.map((d) => {
            const l = live[d.id];
            const done = l?.downloaded ?? d.downloaded;
            const size = l?.size ?? d.size;
            const p = d.status === "completed" ? 100 : pct(done, size);
            const isSel = sel.includes(d.id);
            const meta = d.status === "failed" ? d.error || "Failed" : d.postState ? `${d.postState[0].toUpperCase()}${d.postState.slice(1)}…` : d.postNote || [host(d.url), l ? `${l.conns} connections` : d.queue !== "main" ? queues.find((q) => q.id === d.queue)?.name : ""].filter(Boolean).join(" · ");
            return (
              <div
                key={d.id}
                role="row"
                aria-selected={isSel}
                className={`trow${isSel ? " selected" : ""}`}
                onClick={(e) => click(e, d)}
                onDoubleClick={() => (d.status === "completed" ? act(api.openFile(d.id)) : open({ kind: "details", id: d.id }))}
                onContextMenu={(e) => {
                  e.preventDefault();
                  const ds = isSel ? selected : [d];
                  if (!isSel) setSel([d.id]);
                  menu.open(e.clientX, e.clientY, contextItems(ds));
                }}
              >
                <span role="cell" style={{ display: "grid", placeItems: "center" }} onClick={(e) => e.stopPropagation()}>
                  <input type="checkbox" aria-label={`Select ${d.filename}`} checked={isSel} onChange={() => toggle(d.id)} />
                </span>
                <span role="cell" style={{ display: "flex", alignItems: "center", gap: 12, minWidth: 0 }}>
                  <FileIcon category={d.category} size={34} radius={10} />
                  <span style={{ minWidth: 0 }}>
                    <span className="ellipsis" style={{ display: "block", fontWeight: 500 }}>{d.filename}</span>
                    <span className="ellipsis" style={{ display: "block", fontSize: 12, color: d.status === "failed" || d.checksum?.mismatch ? "var(--bad-text)" : "var(--faint)" }}>{meta}</span>
                  </span>
                </span>
                <span role="cell" className="mono" style={{ fontSize: 13, color: "var(--text2)" }}>{fmtBytes(size)}</span>
                <span role="cell" style={{ display: "flex", alignItems: "center", gap: 10, paddingRight: 16 }}>
                  <span className="bar grow">
                    <span style={{ width: `${p}%`, background: barColor[d.status] }} />
                  </span>
                  <span className="mono muted" style={{ width: 38, textAlign: "right", fontSize: 12 }}>{size > 0 || d.status === "completed" ? `${p}%` : "—"}</span>
                </span>
                <span role="cell" className="mono" style={{ fontSize: 13 }}>{l ? fmtSpeed(l.speed) : "—"}</span>
                <span role="cell" className="mono" style={{ fontSize: 13, color: "var(--text2)" }}>{l ? fmtDuration(l.eta) : "—"}</span>
                <span role="cell">
                  <span className={`pill ${d.status}`}>{statusLabel[d.status]}</span>
                </span>
                <span role="cell" className="muted" style={{ fontSize: 13 }}>{fmtWhen(d.createdAt)}</span>
                <span role="cell">
                  <button className="btn icon sm ghost" aria-label={`More actions for ${d.filename}`} onClick={(e) => {
                    e.stopPropagation();
                    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
                    const ds = isSel ? selected : [d];
                    if (!isSel) setSel([d.id]);
                    menu.open(r.right - 220, r.bottom + 4, contextItems(ds));
                  }}>
                    <Icon name="more" size={16} />
                  </button>
                </span>
              </div>
            );
          })}
        </div>
      </div>

      {one && <SelectedPanel d={one} />}
      {menu.node}
    </>
  );
}

function SelectedPanel({ d }: { d: Download }) {
  const { live, open } = useStore();
  const act = useAction();
  const l = live[d.id];
  const map = l?.map ?? (d.status === "completed" ? Array(16).fill(1) : d.size > 0 ? Array(16).fill(d.downloaded / d.size) : []);
  const cells = map.length > 16 ? Array.from({ length: 16 }, (_, i) => (map[i * 2] + map[i * 2 + 1]) / 2) : map;
  const color = catStyle(d.category).color;
  return (
    <section className="card" aria-labelledby="sel-h" style={{ display: "flex", flexWrap: "wrap", gap: 24 }}>
      <div className="stack" style={{ flex: "1 1 300px", minWidth: 0, gap: 12 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <FileIcon category={d.category} size={44} radius={12} />
          <div style={{ minWidth: 0 }}>
            <h2 id="sel-h" className="ellipsis" style={{ fontSize: 16, fontFamily: "Inter, sans-serif" }}>{d.filename}</h2>
            <div className="mono muted ellipsis" style={{ fontSize: 12 }}>{d.url}</div>
          </div>
        </div>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
          <span className="chip">{d.category}</span>
          {(d.tags ?? []).map((t) => (
            <span key={t} className="chip">#{t}</span>
          ))}
          {d.rule && <span className="chip accent">Rule: {d.rule}</span>}
        </div>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginTop: 4 }}>
          {d.status === "completed" ? (
            <button className="btn primary sm" onClick={() => act(api.openFile(d.id))}>Open file</button>
          ) : (
            <button className="btn primary sm" onClick={() => open({ kind: "details", id: d.id })}>Open progress window</button>
          )}
          <button className="btn sm" onClick={() => act(api.showInFolder(d.id))}>Show in folder</button>
          <button className="btn sm" onClick={() => win.copy(d.url)}>Copy link</button>
        </div>
      </div>
      <div className="stack" style={{ flex: "2 1 420px", minWidth: 0, gap: 12 }}>
        <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: 10 }}>
          <span style={{ fontSize: 13, fontWeight: 600 }}>File map</span>
          <span className="muted" style={{ fontSize: 12 }}>{l ? `${l.conns} connections · each bar is one part of the file` : "Each bar is one part of the file"}</span>
        </div>
        <div role="img" aria-label={`File map, ${pct(l?.downloaded ?? d.downloaded, d.size)}% done`} style={{ display: "grid", gridTemplateColumns: `repeat(${Math.max(1, cells.length)}, minmax(0, 1fr))`, gap: 3 }}>
          {cells.map((v, i) => (
            <span key={i} style={{ height: 34, borderRadius: 5, background: "var(--track)", overflow: "hidden", display: "flex", flexDirection: "column", justifyContent: "flex-end" }}>
              <span style={{ display: "block", height: `${Math.round(v * 100)}%`, background: d.status === "completed" ? "var(--green)" : color, transition: "height .5s" }} />
            </span>
          ))}
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(4, minmax(0, 1fr))", gap: 12 }}>
          <Info label="Downloaded" value={`${fmtBytes(l?.downloaded ?? d.downloaded)} / ${fmtBytes(d.size)}`} />
          <Info label="Speed" value={l ? fmtSpeed(l.speed) : "—"} />
          <Info label="Resume" value={d.resumable ? "Supported" : "Not supported"} color={d.resumable ? "var(--ok-text)" : "var(--bad-text)"} />
          <Info label="Checksum" value={d.checksum?.verified ? "Verified" : d.checksum?.mismatch ? "Mismatch" : d.checksum?.actual ? "Calculated" : d.post?.verify ? "At finish" : "Off"} />
        </div>
      </div>
    </section>
  );
}

function Info({ label, value, color }: { label: string; value: string; color?: string }) {
  return (
    <div style={{ minWidth: 0 }}>
      <div className="faint" style={{ fontSize: 12 }}>{label}</div>
      <div className="mono ellipsis" style={{ fontWeight: 600, color }}>{value}</div>
    </div>
  );
}
