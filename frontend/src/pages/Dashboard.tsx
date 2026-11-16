import { useMemo, useState } from "react";
import { api } from "../api";
import { AreaChart, Donut, SegBar, Sparkline, WeekBars } from "../components/charts";
import { Seg, StatTile, Switch } from "../components/ui";
import { fmtBytes, fmtDuration, fmtMs, fmtNext, fmtSpeed, host, pct, splitBytes, splitSpeed } from "../format";
import { FileIcon, Icon, catStyle } from "../icons";
import { useStore } from "../store";
import type { Settings } from "../types";

const RANGES = [
  { value: "1m", label: "1m", sub: "Last 60 seconds" },
  { value: "10m", label: "10m", sub: "Last 10 minutes" },
  { value: "1h", label: "1h", sub: "Last hour" },
  { value: "24h", label: "24h", sub: "Last 24 hours" },
];

export function Dashboard() {
  const { snap, tick, go, open } = useStore();
  const [range, setRange] = useState("1m");
  const downloads = snap?.downloads ?? [];
  const byId = useMemo(() => Object.fromEntries(downloads.map((d) => [d.id, d])), [downloads]);
  const stats = tick?.stats ?? snap?.stats;
  const speed = tick?.speed ?? 0;
  const activeList = (tick?.live ?? []).filter((l) => byId[l.id]);
  const queued = downloads.filter((d) => d.status === "queued").length;
  const series = stats?.series[range] ?? [];
  const recentSpeeds = (stats?.series["1m"] ?? []).slice(-24);
  const peak = Math.max(0, ...series);
  const nonzero = series.filter((v) => v > 0);
  const avg = nonzero.length ? nonzero.reduce((a, b) => a + b, 0) / nonzero.length : 0;
  const conns = activeList.reduce((a, l) => a + l.conns, 0);
  const today = stats?.today;
  const yest = stats?.yesterday;
  const nextQueue = (snap?.queues ?? []).filter((q) => q.id !== "main" && q.enabled && q.nextRun && !q.nextRun.startsWith("0001")).sort((a, b) => (a.nextRun! < b.nextRun! ? -1 : 1))[0];
  const runningSaved = activeList.reduce((a, l) => a + (byId[l.id]?.savedMs ?? 0), 0);
  const [savedVal, savedUnit] = (() => {
    const ms = (today?.savedMs ?? 0) + runningSaved;
    const t = fmtMs(ms);
    return [t, "today"];
  })();
  const [sNum, sUnit] = splitSpeed(speed);
  const [tNum, tUnit] = splitBytes(today?.bytes ?? 0);
  const diff = (today?.bytes ?? 0) - (yest?.bytes ?? 0);

  const libSlices = (snap?.categories ?? []).filter((c) => c.bytes > 0).map((c) => ({ label: c.name, value: c.bytes, color: catStyle(c.name).color }));
  const libTotal = libSlices.reduce((a, s) => a + s.value, 0);
  const libFiles = (snap?.categories ?? []).reduce((a, c) => a + c.count, 0);

  const week = (stats?.week ?? []).map((d, i, arr) => ({
    label: i === arr.length - 1 ? "Today" : new Date(d.date + "T12:00:00").toLocaleDateString([], { weekday: "short" }),
    value: d.bytes,
    today: i === arr.length - 1,
  }));
  const weekTotal = week.reduce((a, d) => a + d.value, 0);
  const weekMax = Math.max(1, ...week.map((d) => d.value));

  const finished = downloads
    .filter((d) => d.status === "completed")
    .sort((a, b) => ((a.completedAt ?? "") < (b.completedAt ?? "") ? 1 : -1))
    .slice(0, 4);

  const subtitle = [
    `${activeList.length} downloading`,
    `${queued} queued`,
    nextQueue ? `${nextQueue.name} starts ${fmtNext(nextQueue.nextRun).replace(/^(Today|Tonight|Tomorrow) at/, (m) => m.toLowerCase())}` : "",
  ].filter(Boolean).join(" · ");

  return (
    <>
      <div className="page-head">
        <div style={{ flex: "1 1 280px" }}>
          <h1>Dashboard</h1>
          <p>{subtitle}</p>
        </div>
        <button className="btn" onClick={() => (activeList.length ? api.pauseAll() : api.resumeAll())}>
          <Icon name={activeList.length ? "pause" : "play"} size={16} stroke={2} />
          {activeList.length ? "Pause all" : "Resume all"}
        </button>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: 16 }}>
        <StatTile label="Current speed" icon="bolt" color="var(--accent)" value={sNum} unit={sUnit}>
          <Sparkline values={recentSpeeds} />
        </StatTile>
        <StatTile label="Active downloads" icon="down" color="var(--violet)" value={String(activeList.length)} unit={`running · ${queued} queued`}>
          <div style={{ display: "flex", gap: 4, height: 34, alignItems: "flex-end", width: "100%" }}>
            {activeList.slice(0, 8).map((l) => (
              <span key={l.id} style={{ flex: 1, height: 34, borderRadius: 4, background: catStyle(byId[l.id]?.category).color }} />
            ))}
            <span style={{ flex: Math.max(1, 8 - activeList.length), height: 34, borderRadius: 4, border: "1px dashed var(--border2)" }} />
          </div>
        </StatTile>
        <StatTile label="Downloaded today" icon="folder" color="var(--blue)" value={tNum} unit={`${tUnit} · ${today?.files ?? 0} file${today?.files === 1 ? "" : "s"}`}>
          <span className="muted" style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13 }}>
            <span className="chip accent" style={{ fontWeight: 600 }}>{diff >= 0 ? "+" : "−"}{fmtBytes(Math.abs(diff))}</span>
            compared with yesterday
          </span>
        </StatTile>
        <StatTile label="Time saved by acceleration" icon="clock" color="var(--orange)" value={savedVal} unit={savedUnit}>
          <span className="muted" style={{ fontSize: 13 }}>Compared with a single connection</span>
        </StatTile>
      </div>

      <div className="row">
        <section className="card stack" style={{ flex: "2 1 560px", gap: 16 }} aria-labelledby="tp-h">
          <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 12 }}>
            <div style={{ flex: 1 }}>
              <h2 id="tp-h">Live throughput</h2>
              <p className="sub">{RANGES.find((r) => r.value === range)?.sub} · all downloads</p>
            </div>
            <Seg label="Time range" value={range} onChange={setRange} options={RANGES} />
          </div>
          <AreaChart values={series} length={range === "24h" ? 96 : 60} label={`Download speed, ${RANGES.find((r) => r.value === range)?.sub.toLowerCase()}`} current={range === "1m" ? speed : undefined} />
          <div className="muted" style={{ display: "flex", flexWrap: "wrap", gap: 24, paddingLeft: 44, fontSize: 13 }}>
            <span>Peak <strong style={{ color: "var(--text)", fontWeight: 600 }}>{fmtSpeed(peak)}</strong></span>
            <span>Average <strong style={{ color: "var(--text)", fontWeight: 600 }}>{fmtSpeed(avg)}</strong></span>
            <span>Connections <strong style={{ color: "var(--text)", fontWeight: 600 }}>{conns} open</strong></span>
          </div>
        </section>

        <section className="card stack" style={{ flex: "1 1 300px", gap: 16 }} aria-labelledby="bw-h">
          <div>
            <h2 id="bw-h">Bandwidth split</h2>
            <p className="sub">{speed > 0 ? `Share of the current ${fmtSpeed(speed)}` : "How the speed is shared between downloads"}</p>
          </div>
          {activeList.length > 0 ? (
            <>
              <div aria-hidden="true" style={{ display: "flex", gap: 3, height: 14 }}>
                {activeList.map((l) => (
                  <span key={l.id} style={{ flex: Math.max(l.speed, 1), borderRadius: 4, background: catStyle(byId[l.id]?.category).color }} />
                ))}
              </div>
              <div className="stack" style={{ gap: 14 }}>
                {activeList.slice(0, 5).map((l) => (
                  <div key={l.id} className="stack" style={{ gap: 6 }}>
                    <div style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13 }}>
                      <span style={{ width: 8, height: 8, borderRadius: 3, background: catStyle(byId[l.id]?.category).color, flex: "none" }} />
                      <span className="ellipsis grow">{byId[l.id]?.filename}</span>
                      <span className="mono muted" style={{ fontSize: 12 }}>{fmtSpeed(l.speed)}</span>
                    </div>
                    <div className="bar" style={{ height: 5 }}>
                      <span style={{ width: `${speed > 0 ? (l.speed / speed) * 100 : 0}%`, background: catStyle(byId[l.id]?.category).color }} />
                    </div>
                  </div>
                ))}
              </div>
            </>
          ) : (
            <p className="muted" style={{ margin: 0, fontSize: 13 }}>Nothing is downloading right now.</p>
          )}
          <SmartPriority settings={snap?.settings} />
        </section>
      </div>

      <div className="row">
        <section className="card stack" style={{ flex: "2 1 560px", gap: 6 }} aria-labelledby="ad-h">
          <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 8 }}>
            <h2 id="ad-h" style={{ flex: 1 }}>Active downloads</h2>
            <button className="btn ghost xs" onClick={() => go("downloads")}>View all</button>
          </div>
          {activeList.length === 0 && (
            <div className="muted" style={{ display: "flex", alignItems: "center", gap: 12, padding: "18px 4px", fontSize: 13 }}>
              <span style={{ flex: 1 }}>No active downloads. Paste a link or click a download link in your browser.</span>
              <button className="btn primary sm" onClick={() => open({ kind: "add" })}>
                <Icon name="plus" size={15} stroke={2.2} />New download
              </button>
            </div>
          )}
          {activeList.map((l) => {
            const d = byId[l.id];
            const color = catStyle(d.category).color;
            return (
              <div key={l.id} style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 14, padding: 12, borderRadius: 14, background: "var(--card2)" }}>
                <FileIcon category={d.category} />
                <div className="grow stack" style={{ flex: "1 1 220px", gap: 7 }}>
                  <div style={{ display: "flex", alignItems: "baseline", gap: 10 }}>
                    <button className="ellipsis grow" style={{ border: 0, background: "none", padding: 0, textAlign: "left", fontWeight: 600, color: "var(--text)" }} onClick={() => open({ kind: "details", id: d.id })}>
                      {d.filename}
                    </button>
                    <span className="mono muted" style={{ fontSize: 12, whiteSpace: "nowrap" }}>
                      {fmtBytes(l.downloaded)} / {fmtBytes(l.size)} · {pct(l.downloaded, l.size)}%
                    </span>
                  </div>
                  <SegBar map={l.map} color={color} done={l.size > 0 ? l.downloaded / l.size : 0} />
                  <div className="faint ellipsis" style={{ fontSize: 12 }}>
                    {host(d.url)} · {l.conns} connection{l.conns === 1 ? "" : "s"}{d.resumable ? " · resumable" : " · can't resume"}{d.post?.extract ? " · extract when done" : ""}
                  </div>
                </div>
                <div className="stack" style={{ alignItems: "flex-end", gap: 2, minWidth: 92 }}>
                  <span className="mono" style={{ fontSize: 13, fontWeight: 600 }}>{fmtSpeed(l.speed)}</span>
                  <span className="muted" style={{ fontSize: 12 }}>{fmtDuration(l.eta)} left</span>
                </div>
                <button className="btn icon" aria-label={`Pause ${d.filename}`} onClick={() => api.pause([d.id])}>
                  <Icon name="pause" size={16} stroke={2} />
                </button>
              </div>
            );
          })}
        </section>

        <section className="card stack" style={{ flex: "1 1 300px", gap: 16 }} aria-labelledby="lib-h">
          <div>
            <h2 id="lib-h">Library</h2>
            <p className="sub">Space used by category</p>
          </div>
          <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 20 }}>
            <Donut slices={libSlices} center={fmtBytes(libTotal, 2).replace(/\.0+ /, " ")} sub={`${libFiles} file${libFiles === 1 ? "" : "s"}`} label={`Library by category: ${libSlices.map((s) => `${s.label} ${fmtBytes(s.value)}`).join(", ")}`} />
            <div className="stack grow" style={{ flex: "1 1 140px", gap: 8 }}>
              {libSlices.map((s) => (
                <button key={s.label} className="side-link" style={{ height: 26, padding: "0 4px", fontSize: 13 }} onClick={() => go("downloads", { category: s.label })}>
                  <span style={{ width: 8, height: 8, borderRadius: 3, background: s.color, flex: "none" }} />
                  <span>{s.label}</span>
                  <span className="count">{fmtBytes(s.value, 0)}</span>
                </button>
              ))}
              {libSlices.length === 0 && <span className="muted" style={{ fontSize: 13 }}>Finished downloads show up here.</span>}
            </div>
          </div>
        </section>
      </div>

      <div className="row">
        <section className="card stack" style={{ flex: "1 1 340px", gap: 16 }} aria-labelledby="wk-h">
          <div style={{ display: "flex", alignItems: "baseline", gap: 10 }}>
            <h2 id="wk-h" style={{ flex: 1 }}>Last 7 days</h2>
            <span className="mono muted" style={{ fontSize: 13 }}>{fmtBytes(weekTotal)}</span>
          </div>
          <WeekBars days={week} max={weekMax} />
        </section>

        <section className="card stack" style={{ flex: "1 1 300px", gap: 12 }} aria-labelledby="nx-h">
          <h2 id="nx-h">Up next</h2>
          {(snap?.queues ?? [])
            .filter((q) => q.id !== "main" && q.enabled)
            .slice(0, 2)
            .map((q) => (
              <div key={q.id} className="stack" style={{ gap: 10, padding: 14, borderRadius: 14, background: `color-mix(in srgb, ${q.color} 9%, var(--card2))`, border: `1px solid color-mix(in srgb, ${q.color} 28%, transparent)` }}>
                <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <span className="icon-tile" style={{ width: 34, height: 34, borderRadius: 10, background: `color-mix(in srgb, ${q.color} 18%, transparent)`, color: q.color }}>
                    <Icon name={q.icon === "moon" ? "moon" : q.icon === "box" ? "box" : q.icon === "sync" ? "sync" : "cal"} size={18} />
                  </span>
                  <div className="grow">
                    <div style={{ fontWeight: 600 }}>{q.name}</div>
                    <div style={{ fontSize: 12, color: "var(--text2)" }}>{q.active ? "Running now" : fmtNext(q.nextRun)}{q.stop ? ` · until ${q.stop}` : ""}</div>
                  </div>
                </div>
                <div style={{ fontSize: 13, color: "var(--text2)" }}>{q.count ?? 0} files · {fmtBytes(q.bytes ?? 0)} · {q.concurrent} at a time</div>
                <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
                  {q.wakeFromSleep && <span className="chip">Wake PC from sleep</span>}
                  {q.whenDone !== "nothing" && <span className="chip">{q.whenDone === "shutdown" ? "Shut down when done" : q.whenDone === "sleep" ? "Sleep when done" : q.whenDone === "hibernate" ? "Hibernate when done" : "Close ForgeDM when done"}</span>}
                  {q.pauseOnMetered && <span className="chip">Pauses on metered networks</span>}
                </div>
              </div>
            ))}
          {(snap?.projects ?? []).filter((p) => p.syncEnabled).slice(0, 1).map((p) => (
            <div key={p.id} style={{ display: "flex", alignItems: "center", gap: 10, padding: "12px 14px", borderRadius: 14, background: "var(--card2)" }}>
              <span className="icon-tile" style={{ width: 34, height: 34, borderRadius: 10, background: "color-mix(in srgb, var(--blue) 16%, transparent)", color: "var(--blue)" }}>
                <Icon name="sync" size={18} />
              </span>
              <div className="grow">
                <div className="ellipsis" style={{ fontWeight: 600 }}>Site sync · {p.name}</div>
                <div className="muted" style={{ fontSize: 12 }}>{["Sundays", "Mondays", "Tuesdays", "Wednesdays", "Thursdays", "Fridays", "Saturdays"][p.syncDay]} at {p.syncTime} · new files only</div>
              </div>
            </div>
          ))}
          <button className="btn" style={{ marginTop: "auto" }} onClick={() => go("scheduler")}>Open scheduler</button>
        </section>

        <section className="card stack" style={{ flex: "1 1 300px", gap: 6 }} aria-labelledby="rc-h">
          <h2 id="rc-h" style={{ marginBottom: 6 }}>Recently finished</h2>
          {finished.length === 0 && <p className="muted" style={{ margin: 0, fontSize: 13 }}>Finished downloads will appear here.</p>}
          {finished.map((d) => (
            <div key={d.id} style={{ display: "flex", alignItems: "center", gap: 12, padding: "8px 0" }}>
              <FileIcon category={d.category} size={34} radius={10} />
              <div className="grow">
                <div className="ellipsis" style={{ fontWeight: 500 }}>{d.filename}</div>
                <div className="ellipsis" style={{ display: "flex", alignItems: "center", gap: 5, fontSize: 12, color: d.checksum?.mismatch ? "var(--bad-text)" : "var(--ok-text)" }}>
                  <Icon name={d.checksum?.mismatch ? "warn" : "check"} size={12} stroke={2.6} />
                  <span className="ellipsis">{d.postNote || `Saved to ${d.category}`}</span>
                </div>
              </div>
              <button className="btn icon sm ghost" aria-label={`Open ${d.filename}`} onClick={() => api.openFile(d.id)}>
                <Icon name="external" size={15} />
              </button>
            </div>
          ))}
        </section>
      </div>
    </>
  );
}

function SmartPriority({ settings }: { settings?: Settings }) {
  if (!settings) return null;
  return (
    <div style={{ marginTop: "auto", display: "flex", alignItems: "center", gap: 10, padding: 12, borderRadius: 12, background: "var(--card2)", fontSize: 13 }}>
      <span style={{ color: "var(--orange)", display: "grid" }}>
        <Icon name="gauge" size={16} />
      </span>
      <span className="grow muted">Smart priority starts small files first</span>
      <Switch on={settings.smartPriority} onChange={(v) => api.saveSettings({ ...settings, smartPriority: v })} label="Smart priority" />
    </div>
  );
}
