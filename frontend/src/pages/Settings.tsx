import { useState, type ReactNode } from "react";
import { api } from "../api";
import { Seg, Switch, ToggleRow } from "../components/ui";
import { fmtBytes, fmtWhen, parseSize } from "../format";
import { Icon, catStyle } from "../icons";
import { useAction, useStore } from "../store";
import type { Rule, Settings as S } from "../types";

const SECTIONS = [
  { id: "general", label: "General" },
  { id: "rules", label: "Save rules" },
  { id: "browser", label: "Browser integration" },
  { id: "speed", label: "Connections and speed" },
  { id: "finish", label: "When downloads finish" },
  { id: "look", label: "Appearance" },
  { id: "advanced", label: "Advanced" },
] as const;

type Section = (typeof SECTIONS)[number]["id"];

export function describeCondition(c: { field: string; value: string }): string {
  switch (c.field) {
    case "ext":
      return `File ends in ${c.value.split(/[ ,;]+/).filter(Boolean).map((e) => "." + e.replace(/^\./, "")).join(" or ")}`;
    case "type":
      return `Type is ${c.value}`;
    case "site":
      return `Site is ${c.value}`;
    case "sizeOver":
      return `Size is over ${fmtBytes(Number(c.value))}`;
    case "sizeUnder":
      return `Size is under ${fmtBytes(Number(c.value))}`;
    case "name":
      return `Name contains "${c.value}"`;
    case "url":
      return `Link contains "${c.value}"`;
  }
  return c.value;
}

export function describeActions(r: Rule, queueName: (id: string) => string): string[] {
  const a = r.actions;
  const out: string[] = [];
  if (a.dir) out.push(`Save to ${a.dir}`);
  if (a.category) out.push(`Category ${a.category}`);
  if (a.queue) out.push(`Add to ${queueName(a.queue)}`);
  for (const t of a.tags ?? []) out.push(`Tag #${t}`);
  if (a.extract) out.push("Extract when done");
  if (a.open) out.push("Open when done");
  if (!out.length) out.push("Use the category folder");
  return out;
}

export function Settings() {
  const { snap, open, toast } = useStore();
  const act = useAction();
  const [section, setSection] = useState<Section>("general");
  if (!snap) return null;
  const s = snap.settings;
  const update = (patch: Partial<S>) => act(api.saveSettings({ ...s, ...patch }));
  const queueName = (id: string) => snap.queues.find((q) => q.id === id)?.name ?? id;

  const moveRule = (i: number, dir: -1 | 1) => {
    const list = [...snap.rules];
    const j = i + dir;
    if (j < 0 || j >= list.length) return;
    [list[i], list[j]] = [list[j], list[i]];
    act(api.saveRules(list));
  };

  return (
    <>
      <div className="page-head">
        <div style={{ flex: "1 1 300px" }}>
          <h1>Settings</h1>
          <p>Changes save automatically.</p>
        </div>
        <button className="btn" onClick={async () => { const p = await act(api.exportSettings()); if (p) toast("success", `Settings exported to ${p}`); }}>Export settings</button>
        <button className="btn" onClick={() => act(api.importSettings())}>Import settings</button>
      </div>

      <div role="tablist" aria-label="Settings sections" style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
        {SECTIONS.map((x) => (
          <button key={x.id} role="tab" aria-selected={section === x.id} className="filter-chip" onClick={() => setSection(x.id)}>{x.label}</button>
        ))}
      </div>

      {section === "general" && (
        <>
          <Card title="Where downloads go">
            <FolderField label="Main download folder" value={s.baseDir} onChange={(v) => update({ baseDir: v })} hint="Each category gets its own subfolder here unless you pick a different one below." />
            <div className="stack" style={{ gap: 8 }}>
              {snap.categories.map((c) => (
                <div key={c.name} style={{ display: "flex", alignItems: "center", gap: 12, padding: "10px 12px", borderRadius: 12, background: "var(--card2)" }}>
                  <span style={{ width: 8, height: 8, borderRadius: 3, background: catStyle(c.name).color }} />
                  <span style={{ width: 110, fontWeight: 500 }}>{c.name}</span>
                  <span className="mono muted ellipsis grow" style={{ fontSize: 12 }}>{c.dir}</span>
                  <button className="btn xs" onClick={async () => {
                    const d = await api.chooseFolder(c.dir);
                    if (d) update({ categoryDirs: { ...s.categoryDirs, [c.name]: d } });
                  }}>Change</button>
                  {s.categoryDirs?.[c.name] && <button className="btn xs ghost" onClick={() => { const m = { ...s.categoryDirs }; delete m[c.name]; update({ categoryDirs: m }); }}>Reset</button>}
                </div>
              ))}
            </div>
          </Card>
          <Card title="Startup and window">
            <ToggleRow label="Start ForgeDM when Windows starts" hint="Opens quietly in the system tray" on={s.startWithSystem} onChange={(v) => update({ startWithSystem: v })} />
            <ToggleRow label="Closing the window keeps ForgeDM in the tray" hint="Downloads keep going in the background" on={s.closeToTray} onChange={(v) => update({ closeToTray: v })} />
            <ToggleRow label="Keep the PC awake while downloading" hint="Stops Windows from sleeping in the middle of a download" on={s.keepAwake} onChange={(v) => update({ keepAwake: v })} />
            <ToggleRow label="Show the floating drop target" hint="A small always-on-top bubble: drag links onto it to download, and see the live speed" on={s.dropTarget} onChange={(v) => api.setDropTarget(v)} />
            <ToggleRow label="Notifications" hint="When downloads and queues finish, or something needs your attention" on={s.notifications} onChange={(v) => update({ notifications: v })} />
          </Card>
        </>
      )}

      {section === "rules" && (
        <Card title="Save rules" sub="ForgeDM checks these from top to bottom when a download starts. The first rule that matches decides where the file goes and what happens next." action={<button className="btn primary sm" onClick={() => open({ kind: "rule" })}><Icon name="plus" size={15} stroke={2.2} />Add rule</button>}>
          {snap.rules.length === 0 && <p className="muted" style={{ margin: 0 }}>No rules yet. Files go to the folder for their category.</p>}
          <div className="stack" style={{ gap: 8 }}>
            {snap.rules.map((r, i) => (
              <div key={r.id} style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 10, padding: "10px 12px", borderRadius: 12, background: "var(--card2)", opacity: r.enabled ? 1 : 0.55 }}>
                <span className="stack" style={{ gap: 2 }}>
                  <button className="btn icon sm ghost" style={{ width: 22, height: 16 }} aria-label="Move up" disabled={i === 0} onClick={() => moveRule(i, -1)}><Icon name="chevDown" size={14} style={{ transform: "rotate(180deg)" }} /></button>
                  <button className="btn icon sm ghost" style={{ width: 22, height: 16 }} aria-label="Move down" disabled={i === snap.rules.length - 1} onClick={() => moveRule(i, 1)}><Icon name="chevDown" size={14} /></button>
                </span>
                <span style={{ fontSize: 11, fontWeight: 700, letterSpacing: "0.06em", color: "var(--faint)" }}>IF</span>
                {r.conditions.map((c, k) => <span key={k} className="chip" style={{ borderRadius: 8, fontSize: 13, padding: "5px 10px" }}>{describeCondition(c)}</span>)}
                <span className="faint" style={{ display: "grid" }}><Icon name="arrow" size={16} stroke={2} /></span>
                {describeActions(r, queueName).map((t) => <span key={t} className="chip accent" style={{ borderRadius: 8, fontSize: 13, padding: "5px 10px" }}>{t}</span>)}
                <div style={{ flex: 1 }} />
                <Switch on={r.enabled} onChange={(v) => act(api.saveRules(snap.rules.map((x) => (x.id === r.id ? { ...x, enabled: v } : x))))} label={`Rule ${r.name || i + 1} on`} />
                <button className="btn icon sm ghost" aria-label={`Edit rule ${r.name || i + 1}`} onClick={() => open({ kind: "rule", id: r.id })}><Icon name="edit" size={15} /></button>
                <button className="btn icon sm ghost" aria-label={`Delete rule ${r.name || i + 1}`} onClick={() => act(api.saveRules(snap.rules.filter((x) => x.id !== r.id)))}><Icon name="trash" size={15} /></button>
              </div>
            ))}
          </div>
          <span className="hint">Anything that matches no rule goes to the folder for its category.</span>
        </Card>
      )}

      {section === "browser" && (
        <>
          <Card title="Connected browsers" action={<button className="btn primary sm" onClick={() => open({ kind: "connect" })}><Icon name="puzzle" size={15} />Connect a browser</button>}>
            {snap.browsers.length === 0 && <p className="muted" style={{ margin: 0 }}>No browser is connected yet. Install the ForgeDM extension, then click <strong>Connect</strong> in it.</p>}
            {snap.browsers.map((b) => (
              <div key={b.id} style={{ display: "flex", alignItems: "center", gap: 12, height: 52, padding: "0 12px", borderRadius: 12, background: "var(--card2)" }}>
                <span className="icon-tile" style={{ width: 30, height: 30, borderRadius: 8, background: "var(--raised)", color: "var(--text2)" }}><Icon name="globe" size={16} /></span>
                <span className="grow" style={{ fontWeight: 500 }}>{b.name}</span>
                <span style={{ fontSize: 12, color: Date.now() - new Date(b.lastSeen).getTime() < 15 * 60000 ? "var(--ok-text)" : "var(--muted)" }}>
                  {Date.now() - new Date(b.lastSeen).getTime() < 15 * 60000 ? "Connected" : `Last seen ${fmtWhen(b.lastSeen)}`}
                </span>
                <button className="btn xs ghost" onClick={() => act(api.removeBrowser(b.id), `${b.name} disconnected`)}>Disconnect</button>
              </div>
            ))}
            <span className="hint">Browsers talk to ForgeDM on this PC only (port {snap.port || s.apiPort}). Web pages can't use this connection.</span>
          </Card>
          <Card title="Taking over downloads">
            <ToggleRow label="Catch downloads from the browser" hint="Clicking a download link sends it to ForgeDM" on={s.captureEnabled} onChange={(v) => update({ captureEnabled: v })} />
            <ToggleRow label="Ask before starting" hint="Shows a small window where you can rename, pick a folder or let the browser handle it" on={s.askOnCapture} onChange={(v) => update({ askOnCapture: v })} />
            <ToggleRow label="Hold Alt to skip ForgeDM" hint="Alt-click a link to let the browser download that one file" on={s.altBypass} onChange={(v) => update({ altBypass: v })} />
            <ToggleRow label='Show the "Download this video" button' hint="Appears on video players in web pages (not for copy-protected streaming)" on={s.videoButton} onChange={(v) => update({ videoButton: v })} />
            <ToggleRow label="Watch the clipboard for links" hint="Offers to download links you copy" on={s.clipboardWatch} onChange={(v) => update({ clipboardWatch: v })} />
            <ExtList value={s.captureExts} onChange={(v) => update({ captureExts: v })} />
            <div className="field" style={{ maxWidth: 320 }}>
              <label htmlFor="minsize">Only catch files larger than</label>
              <input id="minsize" className="input" placeholder="Any size" defaultValue={s.captureMinSize ? fmtBytes(s.captureMinSize) : ""} onBlur={(e) => update({ captureMinSize: e.target.value.trim() ? Math.max(0, parseSize(e.target.value) || 0) : 0 })} />
            </div>
          </Card>
        </>
      )}

      {section === "speed" && (
        <Card title="Connections and speed">
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: 16 }}>
            <div className="field">
              <label htmlFor="conns">Connections per download</label>
              <select id="conns" className="select" value={s.defaultConnections} onChange={(e) => update({ defaultConnections: Number(e.target.value) })}>
                <option value={0}>Automatic (recommended)</option>
                {[1, 2, 4, 8, 16, 24, 32].map((n) => <option key={n} value={n}>{n}</option>)}
              </select>
              <span className="hint">Automatic uses 1–16 depending on the file size</span>
            </div>
            <div className="field">
              <label htmlFor="maxact">Downloads at the same time</label>
              <select id="maxact" className="select" value={s.maxActive} onChange={(e) => update({ maxActive: Number(e.target.value) })}>
                {[1, 2, 3, 4, 5, 6, 8, 10, 12].map((n) => <option key={n} value={n}>{n}</option>)}
              </select>
            </div>
            <div className="field">
              <label htmlFor="limit">Speed limit</label>
              <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
                <input id="limit" className="input" defaultValue={fmtBytes(s.speedLimit)} onBlur={(e) => { const n = parseSize(e.target.value); if (n > 0) update({ speedLimit: n }); }} />
                <Switch on={s.speedLimitOn} onChange={(v) => api.setSpeedLimiter(v)} label="Speed limit on" />
              </div>
              <span className="hint">Per second, for all downloads together, e.g. 5 MB</span>
            </div>
            <div className="field">
              <label htmlFor="retries">Retries per connection</label>
              <select id="retries" className="select" value={s.retryCount} onChange={(e) => update({ retryCount: Number(e.target.value) })}>
                {[2, 3, 5, 8, 12, 20].map((n) => <option key={n} value={n}>{n}</option>)}
              </select>
            </div>
            <div className="field">
              <label htmlFor="proxy">Proxy</label>
              <select id="proxy" className="select" value={s.proxyMode} onChange={(e) => update({ proxyMode: e.target.value })}>
                <option value="system">Use Windows settings</option>
                <option value="none">No proxy</option>
                <option value="custom">Custom</option>
              </select>
            </div>
            {s.proxyMode === "custom" && (
              <div className="field">
                <label htmlFor="proxyurl">Proxy address</label>
                <input id="proxyurl" className="input mono" placeholder="http://127.0.0.1:8080" defaultValue={s.proxyUrl} onBlur={(e) => update({ proxyUrl: e.target.value.trim() })} />
              </div>
            )}
          </div>
          <ToggleRow label="Smart priority" hint="When several files wait, small ones start first" on={s.smartPriority} onChange={(v) => update({ smartPriority: v })} />
          <div className="field">
            <label htmlFor="ua">Browser identity (user agent)</label>
            <input id="ua" className="input mono" placeholder="ForgeDM default" defaultValue={s.userAgent} onBlur={(e) => update({ userAgent: e.target.value.trim() })} />
            <span className="hint">Downloads caught from the browser always use that browser's identity</span>
          </div>
        </Card>
      )}

      {section === "finish" && (
        <Card title="When downloads finish" sub="These apply to downloads caught from the browser. In the New download window you choose them for each file.">
          <ToggleRow label="Calculate the SHA-256 checksum" hint="Shown in the download details so you can compare it with the publisher's" on={s.verifyChecksum} onChange={(v) => update({ verifyChecksum: v })} />
          <ToggleRow label="Scan with Microsoft Defender" hint="Checks each finished file for viruses" on={s.scanAfter} onChange={(v) => update({ scanAfter: v })} />
          <ToggleRow label="Open the file" on={s.openWhenDone} onChange={(v) => update({ openWhenDone: v })} />
          <ToggleRow label="Delete archives after extracting them" hint="For zip, 7z, rar and tar files unpacked by a save rule" on={s.deleteArchive} onChange={(v) => update({ deleteArchive: v })} />
        </Card>
      )}

      {section === "look" && (
        <Card title="Appearance">
          <fieldset>
            <legend className="label" style={{ marginBottom: 10 }}>Theme</legend>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(4, minmax(0, 1fr))", gap: 10, maxWidth: 640 }}>
              {[
                { id: "midnight", name: "Midnight", bg: "#0A0E17", side: "#0D1220", card: "#18213A" },
                { id: "graphite", name: "Graphite", bg: "#1A1A1C", side: "#232326", card: "#2E2E33" },
                { id: "daylight", name: "Daylight", bg: "#F3F5F9", side: "#E4E8F0", card: "#FFFFFF" },
                { id: "system", name: "Match Windows", bg: "linear-gradient(90deg,#0A0E17 50%,#F3F5F9 50%)", side: "#2E2E33", card: "#8A94AD" },
              ].map((t) => (
                <button key={t.id} aria-pressed={s.theme === t.id} onClick={() => update({ theme: t.id })} style={{ display: "flex", flexDirection: "column", gap: 8, padding: 8, borderRadius: 12, background: "var(--card2)", border: `2px solid ${s.theme === t.id ? "var(--accent)" : "transparent"}`, textAlign: "left", color: "var(--text)" }}>
                  <span style={{ display: "flex", gap: 4, height: 64, padding: 6, borderRadius: 8, background: t.bg }}>
                    <span style={{ width: "22%", borderRadius: 4, background: t.side }} />
                    <span className="stack" style={{ flex: 1, gap: 4 }}>
                      <span style={{ height: 18, borderRadius: 4, background: t.card }} />
                      <span style={{ flex: 1, borderRadius: 4, background: t.card, display: "flex", alignItems: "flex-end", padding: 4 }}>
                        <span style={{ height: 4, width: "60%", borderRadius: 2, background: s.accent }} />
                      </span>
                    </span>
                  </span>
                  <span style={{ fontSize: 12, fontWeight: 500 }}>{t.name}</span>
                </button>
              ))}
            </div>
          </fieldset>
          <fieldset>
            <legend className="label" style={{ marginBottom: 10 }}>Accent color</legend>
            <div style={{ display: "flex", gap: 10 }}>
              {[["Cyan", "#2EE6D6"], ["Orange", "#FF9F43"], ["Violet", "#A78BFA"], ["Pink", "#FF6B9A"], ["Blue", "#5B8CFF"], ["Lime", "#A3E635"]].map(([n, c]) => (
                <button key={c} aria-label={n} aria-pressed={s.accent.toUpperCase() === c} onClick={() => update({ accent: c })} style={{ width: 36, height: 36, borderRadius: "50%", padding: 3, background: "transparent", border: `2px solid ${s.accent.toUpperCase() === c ? "var(--text)" : "transparent"}` }}>
                  <span style={{ display: "block", width: 26, height: 26, borderRadius: "50%", background: c }} />
                </button>
              ))}
            </div>
          </fieldset>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <span className="grow" style={{ fontSize: 13, fontWeight: 500 }}>List density</span>
            <Seg label="List density" value={s.density} onChange={(v) => update({ density: v })} options={[{ value: "comfortable", label: "Comfortable" }, { value: "compact", label: "Compact" }]} />
          </div>
        </Card>
      )}

      {section === "advanced" && (
        <Card title="Advanced">
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))", gap: 12 }}>
            <Info label="Version" value={`ForgeDM ${snap.version}`} />
            <Info label="Browser connection port" value={String(snap.port || s.apiPort)} />
            <Info label="Settings and download list" value={snap.dataDir} mono />
          </div>
          <div style={{ display: "flex", gap: 10, flexWrap: "wrap" }}>
            <button className="btn" onClick={() => act(api.openFolder(snap.dataDir))}><Icon name="folder" size={16} />Open settings folder</button>
            <button className="btn" onClick={() => act(api.openFolder(s.baseDir))}><Icon name="folder" size={16} />Open download folder</button>
          </div>
        </Card>
      )}
    </>
  );
}

function Card({ title, sub, action, children }: { title: string; sub?: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section className="card stack" style={{ gap: 16, padding: 22 }}>
      <div style={{ display: "flex", flexWrap: "wrap", alignItems: "flex-start", gap: 12 }}>
        <div style={{ flex: "1 1 360px" }}>
          <h2 style={{ fontSize: 18 }}>{title}</h2>
          {sub && <p className="sub">{sub}</p>}
        </div>
        {action}
      </div>
      {children}
    </section>
  );
}

function FolderField({ label, value, onChange, hint }: { label: string; value: string; onChange: (v: string) => void; hint?: string }) {
  return (
    <div className="field">
      <label>{label}</label>
      <div style={{ display: "flex", gap: 8 }}>
        <input className="input mono" value={value} readOnly aria-label={label} />
        <button className="btn" onClick={async () => { const d = await api.chooseFolder(value); if (d) onChange(d); }}>Change</button>
        <button className="btn" onClick={() => api.openFolder(value)}>Open</button>
      </div>
      {hint && <span className="hint">{hint}</span>}
    </div>
  );
}

function ExtList({ value, onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const [add, setAdd] = useState("");
  return (
    <div className="field">
      <span className="label">File types ForgeDM takes over</span>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 6, alignItems: "center" }}>
        {value.map((e) => (
          <span key={e} className="chip mono" style={{ borderRadius: 7, paddingRight: 4 }}>
            {e}
            <button aria-label={`Remove ${e}`} onClick={() => onChange(value.filter((x) => x !== e))} style={{ border: 0, background: "transparent", color: "var(--faint)", padding: 0, display: "grid" }}><Icon name="x" size={12} /></button>
          </span>
        ))}
        <form onSubmit={(ev) => { ev.preventDefault(); const v = add.trim().toLowerCase().replace(/^\./, ""); if (v && !value.includes(v)) onChange([...value, v]); setAdd(""); }}>
          <input aria-label="Add a file type" className="input mono" placeholder="+ add" value={add} onChange={(e) => setAdd(e.target.value)} style={{ height: 28, width: 90, fontSize: 12, padding: "0 8px" }} />
        </form>
      </div>
    </div>
  );
}

function Info({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div style={{ padding: "12px 14px", borderRadius: 12, background: "var(--card2)", minWidth: 0 }}>
      <div className="faint" style={{ fontSize: 12 }}>{label}</div>
      <div className={`ellipsis${mono ? " mono" : ""}`} style={{ fontWeight: 600, fontSize: mono ? 12 : 14 }} title={value}>{value}</div>
    </div>
  );
}
