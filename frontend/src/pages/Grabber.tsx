import { useEffect, useMemo, useRef, useState } from "react";
import { api, on } from "../api";
import { Switch } from "../components/ui";
import { fmtBytes, fmtWhen } from "../format";
import { FileIcon, Icon, catStyle } from "../icons";
import { useAction, useStore } from "../store";
import type { GrabItem, GrabProgress, Project } from "../types";

const TYPES = ["Images", "Video", "Music", "Documents", "Compressed", "Programs", "Other"];
const DAY_NAMES = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

export function Grabber() {
  const { snap, go, toast } = useStore();
  const act = useAction();
  const [url, setUrl] = useState("");
  const [depth, setDepth] = useState(1);
  const [sameSite, setSameSite] = useState(true);
  const [scanId, setScanId] = useState<string | null>(null);
  const [progress, setProgress] = useState<GrabProgress | null>(null);
  const [items, setItems] = useState<GrabItem[] | null>(null);
  const [error, setError] = useState("");
  const [seconds, setSeconds] = useState(0);
  const [type, setType] = useState<string>("all");
  const [minSize, setMinSize] = useState(0);
  const [hideHave, setHideHave] = useState(true);
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [dir, setDir] = useState("");
  const [keep, setKeep] = useState(true);
  const [queue, setQueue] = useState("main");
  const [project, setProject] = useState<Project | null>(null);
  const [sync, setSync] = useState(false);
  const [syncDay, setSyncDay] = useState(0);
  const [syncTime, setSyncTime] = useState("09:00");
  const finished = useRef(new Set<string>());

  useEffect(() => {
    const a = on<{ id: string; progress: GrabProgress }>("grabber", (e) => setProgress(e.progress));
    const b = on<{ id: string; items: GrabItem[]; error: string; seconds: number }>("grabber:done", (e) => {
      finished.current.add(e.id);
      setScanId(null);
      setItems(e.items ?? []);
      setError(e.error);
      setSeconds(e.seconds);
      const types = new Set((e.items ?? []).map((i) => i.type));
      const first = TYPES.find((t) => types.has(t));
      setType(first ?? "all");
      setPicked(new Set((e.items ?? []).filter((i) => !i.have && (!first || i.type === first)).map((i) => i.url)));
    });
    return () => {
      a();
      b();
    };
  }, []);

  useEffect(() => {
    if (!dir && snap) setDir(`${snap.settings.baseDir}\\Grabbed`);
  }, [snap, dir]);

  const counts = useMemo(() => {
    const c: Record<string, number> = { all: items?.length ?? 0 };
    for (const i of items ?? []) c[i.type] = (c[i.type] ?? 0) + 1;
    return c;
  }, [items]);

  const visible = useMemo(
    () => (items ?? []).filter((i) => (type === "all" || i.type === type) && (!hideHave || !i.have) && (minSize <= 0 || i.size < 0 || i.size >= minSize)),
    [items, type, hideHave, minSize],
  );

  const chosen = (items ?? []).filter((i) => picked.has(i.url));
  const chosenBytes = chosen.reduce((a, i) => a + Math.max(0, i.size), 0);
  const byType = TYPES.map((t) => ({ t, n: chosen.filter((i) => i.type === t).length })).filter((x) => x.n > 0);

  const scan = async () => {
    if (!url.trim()) return;
    setItems(null);
    setError("");
    setProgress({ pages: 0, found: 0, sized: 0, stage: "pages", current: url });
    const id = await act(api.startScan({ url: url.trim(), depth, sameSite }));
    // A quick scan can finish before startScan returns.
    if (id && !finished.current.has(id)) setScanId(id);
  };

  const loadProject = (p: Project) => {
    setProject(p);
    setUrl(p.url);
    setDepth(p.depth);
    setSameSite(p.sameSite);
    setDir(p.dir);
    setKeep(p.keepStructure);
    setSync(p.syncEnabled);
    setSyncDay(p.syncDay);
    setSyncTime(p.syncTime || "09:00");
    setMinSize(p.minSize);
  };

  const download = async (start: boolean) => {
    const n = await act(api.downloadGrabbed(chosen, dir, keep, queue, start));
    if (n !== undefined) {
      setItems((list) => list?.map((i) => (picked.has(i.url) ? { ...i, have: true } : i)) ?? null);
      setPicked(new Set());
      toast("success", `${n} file${n === 1 ? "" : "s"} added to ${snap?.queues.find((q) => q.id === queue)?.name ?? "the queue"}`);
    }
  };

  const saveProject = async () => {
    const p: Project = {
      id: project?.id ?? "",
      name: project?.name ?? new URL(url).hostname,
      url,
      depth,
      sameSite,
      types: type === "all" ? [] : [type],
      exts: "",
      minSize,
      dir,
      keepStructure: keep,
      syncEnabled: sync,
      syncDay,
      syncTime,
      lastRun: project?.lastRun ?? "",
      lastFound: project?.lastFound ?? 0,
    };
    const saved = await act(api.saveProject(p), sync ? `Saved. ForgeDM will check for new files every ${DAY_NAMES[syncDay]} at ${syncTime}.` : "Project saved");
    if (saved) setProject(saved);
  };

  const scanning = !!scanId;

  return (
    <>
      <div className="page-head">
        <div style={{ flex: "1 1 300px" }}>
          <h1>Site Grabber</h1>
          <p>Scan a page or a whole site, pick the files you want and download them together.</p>
        </div>
      </div>

      {(snap?.projects.length ?? 0) > 0 && (
        <div style={{ display: "flex", flexWrap: "wrap", gap: 8, alignItems: "center" }}>
          <span className="side-head" style={{ padding: 0, marginRight: 4 }}>Saved</span>
          {snap!.projects.map((p) => (
            <button key={p.id} className="filter-chip" aria-pressed={project?.id === p.id} onClick={() => loadProject(p)} title={p.url}>
              {p.syncEnabled && <Icon name="sync" size={13} />}
              {p.name}
              <span className="n">{p.lastRun && !p.lastRun.startsWith("0001") ? fmtWhen(p.lastRun) : "new"}</span>
            </button>
          ))}
        </div>
      )}

      <section className="card stack" aria-label="Scan settings" style={{ gap: 14, padding: 18 }}>
        <form style={{ display: "flex", flexWrap: "wrap", gap: 12, alignItems: "flex-end" }} onSubmit={(e) => { e.preventDefault(); scanning ? api.cancelScan(scanId!) : scan(); }}>
          <div className="field" style={{ flex: "3 1 340px" }}>
            <label htmlFor="g-url">Start page</label>
            <input id="g-url" className="input mono" type="url" required placeholder="https://example.com/gallery" value={url} onChange={(e) => setUrl(e.target.value)} style={{ height: 44 }} />
          </div>
          <div className="field" style={{ flex: "1 1 180px" }}>
            <label htmlFor="g-depth">How deep</label>
            <select id="g-depth" className="select" value={depth} onChange={(e) => setDepth(Number(e.target.value))} style={{ height: 44 }}>
              <option value={0}>This page only</option>
              <option value={1}>Page + linked pages</option>
              <option value={3}>3 levels</option>
              <option value={-1}>Whole site (up to 300 pages)</option>
            </select>
          </div>
          <label className="check" style={{ height: 44, padding: "0 12px", borderRadius: 10, background: "var(--card2)", border: "1px solid var(--border)" }}>
            <input type="checkbox" checked={sameSite} onChange={(e) => setSameSite(e.target.checked)} />
            Same site only
          </label>
          <button className={`btn ${scanning ? "" : "primary"}`} type="submit" style={{ height: 44, padding: "0 20px" }}>
            <Icon name={scanning ? "stop" : "search"} size={16} stroke={2.2} />
            {scanning ? "Stop scan" : items ? "Scan again" : "Scan"}
          </button>
        </form>
        {progress && (
          <div className="muted" style={{ display: "flex", alignItems: "center", gap: 12, fontSize: 13 }}>
            {scanning ? (
              <>
                <span style={{ color: "var(--accent-text)", fontWeight: 500 }}>{progress.stage === "sizes" ? "Checking sizes…" : "Scanning…"}</span>
                <span>{progress.pages} pages · {progress.found} files found{progress.stage === "sizes" ? ` · ${progress.sized} sized` : ""}</span>
                <span className="ellipsis grow mono faint" style={{ fontSize: 12 }}>{progress.stage === "pages" ? progress.current : ""}</span>
              </>
            ) : error ? (
              <span style={{ color: "var(--bad-text)" }}>{error}</span>
            ) : items ? (
              <>
                <span style={{ display: "flex", alignItems: "center", gap: 6, color: "var(--ok-text)", fontWeight: 500 }}>
                  <Icon name="check" size={14} stroke={2.6} />Scan complete
                </span>
                {progress.pages} pages checked · {items.length} files found in {Math.round(seconds)} seconds
              </>
            ) : null}
          </div>
        )}
      </section>

      {items && (
        <>
          <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 8 }}>
            {["all", ...TYPES].filter((t) => t === "all" || counts[t]).map((t) => (
              <button key={t} className="filter-chip" aria-pressed={type === t} onClick={() => setType(t)}>
                {t === "all" ? "All" : t}
                <span className="n">{counts[t] ?? 0}</span>
              </button>
            ))}
            <div style={{ flex: 1 }} />
            <label className="muted" style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13 }}>
              Smallest file
              <select className="select" style={{ height: 34, width: "auto" }} value={minSize} onChange={(e) => setMinSize(Number(e.target.value))}>
                <option value={0}>Any size</option>
                <option value={100 << 10}>100 KB</option>
                <option value={1 << 20}>1 MB</option>
                <option value={5 << 20}>5 MB</option>
                <option value={50 << 20}>50 MB</option>
              </select>
            </label>
            <label className="check">
              <input type="checkbox" checked={hideHave} onChange={(e) => setHideHave(e.target.checked)} />
              Hide files I already have
            </label>
          </div>

          <div className="row" style={{ alignItems: "flex-start" }}>
            <section aria-label="Files found" className="grow" style={{ flex: "2 1 560px" }}>
              <div style={{ display: "flex", gap: 10, marginBottom: 12, alignItems: "center" }}>
                <button className="btn sm" onClick={() => setPicked(new Set([...picked, ...visible.map((i) => i.url)]))}>Select all shown</button>
                <button className="btn sm ghost" onClick={() => setPicked(new Set())}>Clear selection</button>
                <span className="muted" style={{ fontSize: 13 }}>{visible.length} shown</span>
              </div>
              {visible.length === 0 ? (
                <div className="card empty">Nothing here with these filters.</div>
              ) : (
                <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(190px, 1fr))", gap: 14 }}>
                  {visible.slice(0, 400).map((it) => (
                    <Tile key={it.url} it={it} on={picked.has(it.url)} toggle={() => setPicked((s) => {
                      const n = new Set(s);
                      n.has(it.url) ? n.delete(it.url) : n.add(it.url);
                      return n;
                    })} />
                  ))}
                </div>
              )}
              {visible.length > 400 && <p className="muted" style={{ fontSize: 13 }}>Showing the first 400. Use the filters to narrow down.</p>}
            </section>

            <aside className="card stack" aria-labelledby="selh" style={{ flex: "1 1 300px", gap: 16, position: "sticky", top: 0 }}>
              <div>
                <h2 id="selh">Your selection</h2>
                <div style={{ display: "flex", alignItems: "baseline", gap: 8, marginTop: 6 }}>
                  <span className="display" style={{ fontSize: 30, fontWeight: 600 }}>{chosen.length}</span>
                  <span className="muted">files · {fmtBytes(chosenBytes)}</span>
                </div>
              </div>
              {byType.length > 0 && (
                <>
                  <div aria-hidden="true" style={{ display: "flex", gap: 3, height: 10 }}>
                    {byType.map((x) => <span key={x.t} style={{ flex: x.n, borderRadius: 3, background: catStyle(x.t === "Images" ? "Images" : x.t).color }} />)}
                  </div>
                  <div className="stack" style={{ gap: 6, fontSize: 13 }}>
                    {byType.map((x) => (
                      <div key={x.t} style={{ display: "flex", alignItems: "center", gap: 8 }}>
                        <span style={{ width: 8, height: 8, borderRadius: 3, background: catStyle(x.t).color }} />
                        <span className="grow">{x.t}</span>
                        <span className="mono muted">{x.n}</span>
                      </div>
                    ))}
                  </div>
                </>
              )}
              <div className="field">
                <label htmlFor="g-dir">Save to</label>
                <div style={{ display: "flex", gap: 8 }}>
                  <input id="g-dir" className="input" value={dir} onChange={(e) => setDir(e.target.value)} />
                  <button className="btn" onClick={async () => { const d = await api.chooseFolder(dir); if (d) setDir(d); }}>Browse</button>
                </div>
              </div>
              <label className="check"><input type="checkbox" checked={keep} onChange={(e) => setKeep(e.target.checked)} />Keep the site's folder structure</label>
              <div className="field">
                <label htmlFor="g-q">Queue</label>
                <select id="g-q" className="select" value={queue} onChange={(e) => setQueue(e.target.value)}>
                  {(snap?.queues ?? []).map((q) => <option key={q.id} value={q.id}>{q.name}</option>)}
                </select>
              </div>
              <button className="btn primary" style={{ height: 44 }} disabled={chosen.length === 0} onClick={() => download(true)}>
                <Icon name="down" size={16} stroke={2.2} />Download {chosen.length} file{chosen.length === 1 ? "" : "s"}
              </button>
              <div className="stack" style={{ gap: 10, padding: 14, borderRadius: 12, background: "var(--card2)" }}>
                <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <span className="grow">
                    <span style={{ display: "block", fontSize: 13, fontWeight: 500 }}>Sync every week</span>
                    <span className="hint">Checks the site again and downloads only new files</span>
                  </span>
                  <Switch on={sync} onChange={setSync} label="Sync every week" />
                </div>
                {sync && (
                  <div style={{ display: "flex", gap: 8 }}>
                    <select aria-label="Day" className="select" value={syncDay} onChange={(e) => setSyncDay(Number(e.target.value))}>
                      {DAY_NAMES.map((d, i) => <option key={d} value={i}>{d}</option>)}
                    </select>
                    <input aria-label="Time" className="input" type="time" value={syncTime} onChange={(e) => setSyncTime(e.target.value)} style={{ width: 120 }} />
                  </div>
                )}
                <button className="btn sm" onClick={saveProject}>{project ? "Update saved project" : "Save as project"}</button>
              </div>
              <button className="btn ghost sm" onClick={() => go("downloads")}>Go to downloads</button>
            </aside>
          </div>
        </>
      )}

      {!items && !scanning && (
        <div className="card empty">
          <span className="icon-tile" style={{ width: 52, height: 52, borderRadius: 14, background: "var(--accent-soft)", color: "var(--accent-text)" }}>
            <Icon name="globe" size={24} />
          </span>
          <div className="big">Find every file on a page</div>
          <span style={{ maxWidth: 520 }}>Paste a gallery, a course page or a download index. ForgeDM lists the images, videos, documents and archives it links to, with their sizes, so you can pick what to download.</span>
        </div>
      )}
    </>
  );
}

function Tile({ it, on, toggle }: { it: GrabItem; on: boolean; toggle: () => void }) {
  const [broken, setBroken] = useState(false);
  const isImg = it.type === "Images" && !broken;
  return (
    <label style={{ position: "relative", display: "flex", flexDirection: "column", gap: 8, padding: 8, borderRadius: 14, background: "var(--card)", border: `1px solid ${on ? "var(--accent-line)" : "var(--border)"}`, boxShadow: on ? "inset 0 0 0 1px var(--accent-line)" : "none", cursor: "pointer", opacity: it.have ? 0.55 : 1 }}>
      <span style={{ position: "relative", height: 118, borderRadius: 9, overflow: "hidden", display: "grid", placeItems: "center", background: `color-mix(in srgb, ${catStyle(it.type).color} 10%, var(--card2))` }}>
        {isImg ? <img src={it.url} alt="" loading="lazy" referrerPolicy="no-referrer" onError={() => setBroken(true)} style={{ width: "100%", height: "100%", objectFit: "cover" }} /> : <FileIcon category={it.type} size={48} radius={13} />}
        <span style={{ position: "absolute", top: 8, left: 8, width: 24, height: 24, borderRadius: 7, background: "rgba(10,14,23,0.7)", display: "grid", placeItems: "center" }}>
          <input type="checkbox" checked={on} onChange={toggle} aria-label={`Select ${it.name}`} style={{ accentColor: "var(--accent)", width: 16, height: 16, margin: 0 }} />
        </span>
        {it.have && <span className="chip" style={{ position: "absolute", bottom: 8, right: 8 }}>Already downloaded</span>}
      </span>
      <span className="stack" style={{ gap: 2, padding: "0 4px 4px" }}>
        <span className="ellipsis" style={{ fontSize: 13, fontWeight: 500 }} title={it.name}>{it.name}</span>
        <span className="mono muted" style={{ display: "flex", justifyContent: "space-between", fontSize: 11 }}>
          <span>{it.ext.toUpperCase()}</span>
          <span>{fmtBytes(it.size)}</span>
        </span>
      </span>
    </label>
  );
}
