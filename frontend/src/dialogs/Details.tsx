import { useEffect, useState } from "react";
import { api, win } from "../api";
import { AreaChart, Ring } from "../components/charts";
import { Modal } from "../components/ui";
import { fmtBytes, fmtDuration, fmtMs, fmtSpeed, fmtWhen, pct } from "../format";
import { FileIcon, Icon, catStyle } from "../icons";
import { useAction, useStore } from "../store";
import type { Download } from "../types";

const statusText: Record<string, string> = { downloading: "Downloading", queued: "Waiting in queue", paused: "Paused", completed: "Finished", failed: "Failed" };

export function Details({ id, onClose }: { id: string; onClose: () => void }) {
  const { snap, live, open } = useStore();
  const act = useAction();
  const [detail, setDetail] = useState<Download | null>(null);
  const d = snap?.downloads.find((x) => x.id === id) ?? detail;
  const l = live[id];

  useEffect(() => {
    api.details(id).then((x) => x && setDetail(x)).catch(() => {});
    return () => {
      api.closeDetails();
    };
  }, [id]);

  if (!d) return null;
  const done = l?.downloaded ?? d.downloaded;
  const size = l?.size ?? d.size;
  const frac = d.status === "completed" ? 1 : size > 0 ? done / size : 0;
  const color = catStyle(d.category).color;
  const map = l?.map ?? (d.status === "completed" ? Array(32).fill(1) : size > 0 ? Array(32).fill(frac) : []);
  const parts = (l?.parts ?? []).filter((p) => p.end >= 0).sort((a, b) => Number(b.active) - Number(a.active) || a.start - b.start);
  const history = l?.history ?? [];
  const mainQueue = snap?.queues.find((q) => q.id === "main");
  const shutdownAfter = mainQueue?.whenDone === "shutdown";
  const avg = d.activeMs > 0 ? (d.downloaded / d.activeMs) * 1000 : 0;

  return (
    <Modal title={`Progress of ${d.filename}`} onClose={onClose} width={920}>
      <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
        <FileIcon category={d.category} size={48} radius={13} />
        <div className="grow">
          <h1 className="ellipsis" style={{ fontSize: 20 }}>{d.filename}</h1>
          <div className="mono muted ellipsis" style={{ fontSize: 12 }}>{d.url}</div>
        </div>
        <span className={`pill ${d.status}`}>{d.postState ? `${d.postState[0].toUpperCase()}${d.postState.slice(1)}…` : statusText[d.status]}</span>
      </div>

      {d.status === "failed" && (
        <div className="notice bad">
          <span style={{ color: "var(--bad-text)", display: "grid" }}><Icon name="warn" size={18} stroke={2} /></span>
          <span className="grow">{d.error}</span>
          {d.errorCode === "link_expired" ? (
            <button className="btn sm" onClick={() => open({ kind: "refresh", id: d.id })}>Refresh link</button>
          ) : (
            <button className="btn sm" onClick={() => api.resume([d.id])}>Try again</button>
          )}
        </div>
      )}

      <div style={{ display: "flex", alignItems: "center", gap: 28 }}>
        <Ring value={frac}>
          <span className="display" style={{ fontSize: 30, fontWeight: 600 }}>{Math.floor(frac * 100)}%</span>
          <span className="muted" style={{ fontSize: 12, textAlign: "center", lineHeight: 1.3 }}>{fmtBytes(done)}<br />of {fmtBytes(size)}</span>
        </Ring>
        <div className="grow" style={{ display: "grid", gridTemplateColumns: "repeat(3, minmax(0, 1fr))", gap: 12 }}>
          {d.status === "completed" ? (
            <>
              <Stat label="Average speed" value={fmtSpeed(avg)} accent />
              <Stat label="Took" value={fmtMs(d.activeMs)} />
              <Stat label="Time saved" value={d.savedMs > 1000 ? fmtMs(d.savedMs) : "—"} />
            </>
          ) : (
            <>
              <Stat label="Speed" value={l ? fmtSpeed(l.speed) : "—"} accent />
              <Stat label="Time left" value={l ? fmtDuration(l.eta) : "—"} />
              <Stat label="Connections" value={l ? `${l.conns} active` : `${d.connections || "Auto"}`} />
            </>
          )}
          <div className="card tight" style={{ padding: "12px 14px" }}>
            <div className="faint" style={{ fontSize: 12 }}>Resume</div>
            <div style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 15, fontWeight: 600, color: d.resumable ? "var(--ok-text)" : "var(--bad-text)" }}>
              <Icon name={d.resumable ? "shield" : "warn"} size={15} stroke={2} />{d.resumable ? "Supported" : "Not supported"}
            </div>
          </div>
          <Stat label={d.status === "completed" ? "Finished" : "Peak speed"} value={d.status === "completed" ? fmtWhen(d.completedAt) : l ? fmtSpeed(l.peak) : "—"} />
          <div className="card tight" style={{ padding: "12px 14px", minWidth: 0 }}>
            <div className="faint" style={{ fontSize: 12 }}>Saving to</div>
            <button className="ellipsis" style={{ display: "block", width: "100%", border: 0, background: "none", padding: 0, textAlign: "left", fontWeight: 600, color: "var(--text)" }} title={d.dir} onClick={() => act(api.openFolder(d.dir))}>{d.dir}</button>
          </div>
        </div>
      </div>

      {map.length > 0 && (
        <section className="stack" style={{ gap: 8 }} aria-labelledby="map-h">
          <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between" }}>
            <h2 id="map-h" style={{ margin: 0, fontSize: 13, fontWeight: 600 }}>File map</h2>
            <span className="muted" style={{ fontSize: 12 }}>{d.kind === "hls" ? "Each block is a group of video pieces" : "The file is split into parts that download at the same time"}</span>
          </div>
          <div role="img" aria-label={`File map, ${pct(done, size)}% done`} style={{ display: "flex", gap: 2, height: 28, padding: 3, borderRadius: 8, background: "var(--card)", border: "1px solid var(--border)" }}>
            {map.map((v, i) => (
              <span key={i} style={{ flex: 1, display: "flex", borderRadius: 4, background: "var(--track)", overflow: "hidden" }}>
                <span style={{ width: `${Math.round(v * 100)}%`, background: d.status === "completed" ? "var(--green)" : color, transition: "width .5s" }} />
              </span>
            ))}
          </div>
          {size > 0 && (
            <div className="mono faint" aria-hidden="true" style={{ display: "flex", justifyContent: "space-between", fontSize: 11 }}>
              <span>0</span><span>{fmtBytes(size / 4)}</span><span>{fmtBytes(size / 2)}</span><span>{fmtBytes((size * 3) / 4)}</span><span>{fmtBytes(size)}</span>
            </div>
          )}
        </section>
      )}

      {d.status !== "completed" && (
        <div style={{ display: "flex", gap: 16 }}>
          <section className="card tight stack grow" style={{ gap: 10 }} aria-labelledby="sp-h">
            <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between" }}>
              <h2 id="sp-h" style={{ margin: 0, fontSize: 13, fontWeight: 600, fontFamily: "Inter" }}>Speed</h2>
              <span className="muted" style={{ fontSize: 12 }}>Last minute</span>
            </div>
            {history.length > 1 ? <AreaChart values={history} height={110} showAxis={false} label="Speed over the last minute" color={color} /> : <div className="muted" style={{ height: 110, display: "grid", placeItems: "center", fontSize: 13 }}>{d.status === "downloading" ? "Measuring…" : "Not downloading"}</div>}
          </section>
          <section className="card tight stack grow" style={{ gap: 6 }} aria-labelledby="cn-h">
            <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between" }}>
              <h2 id="cn-h" style={{ margin: 0, fontSize: 13, fontWeight: 600, fontFamily: "Inter" }}>Parts</h2>
              <span className="muted" style={{ fontSize: 12 }}>{parts.length ? `${parts.filter((p) => p.active).length} downloading · ${parts.length} total` : ""}</span>
            </div>
            {parts.length === 0 && <span className="muted" style={{ fontSize: 13 }}>{d.kind === "hls" ? "Video pieces download several at a time." : "Parts appear once the download is running."}</span>}
            {parts.slice(0, 6).map((p, i) => {
              const len = p.end - p.start + 1;
              return (
                <div key={p.start} style={{ display: "grid", gridTemplateColumns: "26px minmax(0,1fr) 120px", alignItems: "center", gap: 10, height: 24, fontSize: 12 }}>
                  <span className="mono faint">#{i + 1}</span>
                  <span className="bar" style={{ height: 5 }}><span style={{ width: `${pct(p.done, len)}%`, background: p.active ? color : "var(--green)" }} /></span>
                  <span className="mono" style={{ textAlign: "right", color: "var(--text2)" }}>{fmtBytes(p.done)} / {fmtBytes(len)}</span>
                </div>
              );
            })}
          </section>
        </div>
      )}

      {d.status === "completed" && (
        <div className="card tight stack" style={{ gap: 8 }}>
          {d.postNote && <div style={{ display: "flex", alignItems: "center", gap: 8, color: d.checksum?.mismatch ? "var(--bad-text)" : "var(--ok-text)", fontSize: 13 }}><Icon name={d.checksum?.mismatch ? "warn" : "check"} size={15} stroke={2.4} />{d.postNote}</div>}
          {d.checksum?.actual && (
            <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
              <span className="faint" style={{ fontSize: 12, width: 70 }}>{(d.checksum.algo || "sha256").toUpperCase()}</span>
              <span className="mono ellipsis grow" style={{ fontSize: 12 }}>{d.checksum.actual}</span>
              <button className="btn xs" onClick={() => win.copy(d.checksum.actual!)}>Copy</button>
            </div>
          )}
        </div>
      )}

      {d.status !== "completed" && (
        <div className="card tight" style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 22, fontSize: 13, padding: "12px 16px" }}>
          <label className="check"><input type="checkbox" checked={d.post.verify} onChange={(e) => api.setPost(d.id, { ...d.post, verify: e.target.checked })} />Calculate checksum when done</label>
          <label className="check"><input type="checkbox" checked={d.post.open} onChange={(e) => api.setPost(d.id, { ...d.post, open: e.target.checked })} />Open file when done</label>
          <label className="check"><input type="checkbox" checked={shutdownAfter} onChange={(e) => mainQueue && api.saveQueue({ ...mainQueue, whenDone: e.target.checked ? "shutdown" : "nothing" })} />Shut down when all downloads finish</label>
          <label className="muted" style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: 8 }}>
            Speed limit
            <select className="select" style={{ height: 32, width: "auto" }} value={d.speedLimit ?? 0} onChange={(e) => api.setSpeedLimit(d.id, Number(e.target.value))}>
              <option value={0}>No limit</option>
              {[20, 10, 5, 2, 1, 0.5].map((m) => <option key={m} value={Math.round(m * 1048576)}>{m >= 1 ? `${m} MB/s` : `${m * 1024} KB/s`}</option>)}
            </select>
          </label>
        </div>
      )}

      <div className="actions">
        <button className="btn ghost" onClick={onClose}>{d.status === "completed" ? "Close" : "Hide"}</button>
        <div style={{ flex: 1 }} />
        {d.status === "completed" ? (
          <>
            <button className="btn" onClick={() => act(api.showInFolder(d.id))}><Icon name="folder" size={16} />Show in folder</button>
            <button className="btn primary" onClick={() => act(api.openFile(d.id))}><Icon name="external" size={16} />Open</button>
          </>
        ) : (
          <>
            <button className="btn danger" onClick={() => open({ kind: "remove", ids: [d.id] })}>Cancel download</button>
            {d.status === "downloading" || d.status === "queued" ? (
              <button className="btn primary" onClick={() => api.pause([d.id])}><Icon name="pause" size={16} stroke={2.2} />Pause</button>
            ) : (
              <button className="btn primary" onClick={() => api.resume([d.id])}><Icon name="play" size={16} stroke={2.2} />{d.status === "failed" ? "Try again" : "Resume"}</button>
            )}
          </>
        )}
      </div>
    </Modal>
  );
}

function Stat({ label, value, accent }: { label: string; value: string; accent?: boolean }) {
  return (
    <div className="card tight" style={{ padding: "12px 14px", minWidth: 0 }}>
      <div className="faint" style={{ fontSize: 12 }}>{label}</div>
      <div className="mono ellipsis" style={{ fontSize: 18, fontWeight: 600, color: accent ? "var(--accent-text)" : undefined }}>{value}</div>
    </div>
  );
}
