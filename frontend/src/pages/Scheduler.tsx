import { useEffect, useMemo, useState } from "react";
import { api } from "../api";
import { Switch } from "../components/ui";
import { fmtBytes, fmtNext, parseSize } from "../format";
import { Icon, catStyle, type IconName } from "../icons";
import { useAction, useStore } from "../store";
import type { Queue } from "../types";

const DAYS = [
  { d: 1, s: "M", full: "Monday" },
  { d: 2, s: "T", full: "Tuesday" },
  { d: 3, s: "W", full: "Wednesday" },
  { d: 4, s: "T", full: "Thursday" },
  { d: 5, s: "F", full: "Friday" },
  { d: 6, s: "S", full: "Saturday" },
  { d: 0, s: "S", full: "Sunday" },
];

const COLORS = ["#A78BFA", "#5B8CFF", "#FF6B9A", "#FF9F43", "#F7C948", "#2EE6D6"];
const ICONS: { value: string; icon: IconName }[] = [
  { value: "moon", icon: "moon" },
  { value: "box", icon: "box" },
  { value: "sync", icon: "sync" },
  { value: "cal", icon: "cal" },
];
const iconFor = (q: Queue): IconName => (q.icon === "bolt" ? "bolt" : q.icon === "moon" ? "moon" : q.icon === "box" ? "box" : q.icon === "sync" ? "sync" : "cal");

function blank(): Queue {
  return { id: "", name: "New queue", builtin: false, enabled: true, start: "22:00", stop: "06:00", days: [0, 1, 2, 3, 4, 5, 6], concurrent: 2, speedLimit: 0, halfBandwidth: false, whenDone: "nothing", wakeFromSleep: false, retryFailed: true, pauseOnMetered: true, color: "#5B8CFF", icon: "cal", order: [] };
}

export function Scheduler() {
  const { snap } = useStore();
  const act = useAction();
  const queues = snap?.queues ?? [];
  const [selId, setSelId] = useState<string>(() => queues.find((q) => q.id !== "main")?.id ?? "main");
  const [draft, setDraft] = useState<Queue | null>(null);
  const sel = queues.find((q) => q.id === selId) ?? queues[0];

  useEffect(() => {
    if (sel && (!draft || draft.id !== sel.id)) setDraft({ ...sel });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sel?.id]);

  const items = useMemo(() => {
    if (!sel || !snap) return [];
    const list = snap.downloads.filter((d) => d.queue === sel.id && d.status !== "completed");
    const order = sel.order ?? [];
    return list.sort((a, b) => {
      const ia = order.indexOf(a.id);
      const ib = order.indexOf(b.id);
      if (ia >= 0 && ib >= 0) return ia - ib;
      if (ia >= 0) return -1;
      if (ib >= 0) return 1;
      return a.createdAt < b.createdAt ? -1 : 1;
    });
  }, [sel, snap]);

  const [dragId, setDragId] = useState<string | null>(null);
  const dirty = draft && sel && JSON.stringify({ ...draft, active: undefined, nextRun: undefined, count: undefined, bytes: undefined }) !== JSON.stringify({ ...sel, active: undefined, nextRun: undefined, count: undefined, bytes: undefined });

  const save = async () => {
    if (!draft) return;
    const q = await act(api.saveQueue(draft), "Queue saved");
    if (q) setSelId(q.id);
  };

  const create = async () => {
    const q = await act(api.saveQueue(blank()));
    if (q) {
      setSelId(q.id);
      setDraft(q);
    }
  };

  const drop = (targetId: string) => {
    if (!dragId || !sel || dragId === targetId) return;
    const ids = items.map((i) => i.id);
    const from = ids.indexOf(dragId);
    const to = ids.indexOf(targetId);
    ids.splice(to, 0, ids.splice(from, 1)[0]);
    api.reorder(sel.id, ids);
    setDragId(null);
  };

  const set = <K extends keyof Queue>(k: K, v: Queue[K]) => setDraft((d) => (d ? { ...d, [k]: v } : d));
  const isMain = draft?.id === "main";

  return (
    <>
      <div className="page-head">
        <div style={{ flex: "1 1 300px" }}>
          <h1>Queues &amp; Scheduler</h1>
          <p>Run downloads overnight, on weekends or on repeat, and choose what happens when they finish.</p>
        </div>
        <button className="btn primary" onClick={create}>
          <Icon name="plus" size={16} stroke={2.2} />New queue
        </button>
      </div>

      <div className="row" style={{ alignItems: "flex-start" }}>
        <div className="stack" style={{ flex: "1 1 320px", minWidth: 0, gap: 20 }}>
          <section aria-label="Queues" className="stack" style={{ gap: 10 }}>
            {queues.map((q) => {
              const on = q.id === sel?.id;
              return (
                <button key={q.id} aria-pressed={on} onClick={() => setSelId(q.id)} style={{ display: "flex", alignItems: "center", gap: 12, padding: 14, borderRadius: 14, textAlign: "left", background: on ? `color-mix(in srgb, ${q.color} 9%, var(--card))` : "var(--card)", border: `1px solid ${on ? `color-mix(in srgb, ${q.color} 40%, transparent)` : "var(--border)"}`, color: "var(--text)" }}>
                  <span className="icon-tile" style={{ width: 38, height: 38, borderRadius: 11, background: `color-mix(in srgb, ${q.color} 16%, transparent)`, color: q.color }}>
                    <Icon name={iconFor(q)} />
                  </span>
                  <span className="grow">
                    <span style={{ display: "block", fontWeight: 600 }}>{q.name}</span>
                    <span className="muted ellipsis" style={{ display: "block", fontSize: 12 }}>
                      {q.id === "main" ? `${q.concurrent} at a time · always on` : `${q.start}${q.stop ? `–${q.stop}` : ""} · ${dayText(q.days)}`} · {q.count ?? 0} files
                    </span>
                  </span>
                  <span className="chip" style={{ background: `color-mix(in srgb, ${q.color} 15%, transparent)`, color: q.color, fontWeight: 600 }}>
                    {q.active ? "Running" : !q.enabled ? "Off" : q.id === "main" ? "On" : shortNext(q.nextRun)}
                  </span>
                </button>
              );
            })}
          </section>

          {sel && (
            <section className="card stack" aria-labelledby="it-h" style={{ gap: 4, padding: 18 }}>
              <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginBottom: 8 }}>
                <h2 id="it-h" style={{ flex: 1, fontSize: 16 }}>In this queue</h2>
                <span className="muted" style={{ fontSize: 12 }}>{items.length} files · {fmtBytes(items.reduce((a, d) => a + Math.max(0, d.size - d.downloaded), 0))}{items.length > 1 ? " · drag to reorder" : ""}</span>
              </div>
              {items.length === 0 && <p className="muted" style={{ margin: "4px 0", fontSize: 13 }}>No downloads waiting. In the Downloads list, right-click a download and choose "Move to {sel.name}".</p>}
              {items.map((d, i) => (
                <div key={d.id} draggable onDragStart={() => setDragId(d.id)} onDragOver={(e) => e.preventDefault()} onDrop={() => drop(d.id)} style={{ display: "flex", alignItems: "center", gap: 10, height: 46, padding: "0 8px", borderRadius: 10, background: dragId === d.id ? "var(--raised)" : "var(--card2)", cursor: "grab" }}>
                  <span style={{ color: "var(--faint)", display: "grid" }}><Icon name="grip" size={14} /></span>
                  <span className="mono faint" style={{ width: 18, fontSize: 12 }}>{i + 1}</span>
                  <span style={{ width: 8, height: 8, borderRadius: 3, background: catStyle(d.category).color, flex: "none" }} />
                  <span className="ellipsis grow" style={{ fontSize: 13 }}>{d.filename}</span>
                  <span className={`pill ${d.status}`} style={{ padding: "2px 8px" }}>{d.status === "downloading" ? "Now" : d.status === "queued" ? "Waiting" : d.status[0].toUpperCase() + d.status.slice(1)}</span>
                  <span className="mono muted" style={{ fontSize: 12 }}>{fmtBytes(d.size)}</span>
                </div>
              ))}
            </section>
          )}
        </div>

        {draft && (
          <section className="card stack" aria-labelledby="ed-h" style={{ flex: "2 1 560px", gap: 22, padding: 22 }}>
            <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 12 }}>
              <span className="icon-tile" style={{ width: 42, height: 42, borderRadius: 12, background: `color-mix(in srgb, ${draft.color} 16%, transparent)`, color: draft.color }}>
                <Icon name={iconFor(draft)} size={20} />
              </span>
              <div className="grow">
                {draft.builtin ? (
                  <h2 id="ed-h" style={{ fontSize: 20 }}>{draft.name}</h2>
                ) : (
                  <input id="ed-h" aria-label="Queue name" className="input display" value={draft.name} onChange={(e) => set("name", e.target.value)} style={{ height: 38, fontSize: 18, fontWeight: 600, background: "transparent", border: "1px solid transparent", padding: "0 6px", marginLeft: -6 }} />
                )}
                <div style={{ fontSize: 13, color: draft.color }}>{sel?.active ? "Running now" : isMain ? "Starts downloads as soon as they're added" : `Next run ${fmtNext(sel?.nextRun).replace(/^\w/, (c) => c.toLowerCase())}`}</div>
              </div>
              <span className="muted" style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 13 }}>
                On
                <Switch on={draft.enabled} onChange={(v) => set("enabled", v)} label="Queue on" />
              </span>
              {!isMain && (sel?.active ? (
                <button className="btn" onClick={() => api.stopQueue(draft.id)}>Stop</button>
              ) : (
                <button className="btn" onClick={() => api.runQueue(draft.id)}>Run now</button>
              ))}
            </div>

            <div style={{ display: "grid", gridTemplateColumns: "repeat(3, minmax(0, 1fr))", gap: 16 }}>
              {!isMain && (
                <>
                  <div className="field">
                    <label htmlFor="q-start">Start at</label>
                    <input id="q-start" className="input" type="time" value={draft.start} onChange={(e) => set("start", e.target.value)} />
                  </div>
                  <div className="field">
                    <label htmlFor="q-stop">Stop at</label>
                    <input id="q-stop" className="input" type="time" value={draft.stop} onChange={(e) => set("stop", e.target.value)} />
                    <span className="hint">Leave empty to run until the queue is done</span>
                  </div>
                </>
              )}
              <div className="field">
                <label htmlFor="q-conc">Files at once</label>
                <select id="q-conc" className="select" value={draft.concurrent} onChange={(e) => set("concurrent", Number(e.target.value))}>
                  {[1, 2, 3, 4, 5, 6, 8, 10].map((n) => <option key={n} value={n}>{n}</option>)}
                </select>
              </div>
              {!isMain && (
                <fieldset style={{ gridColumn: "span 2" }}>
                  <legend className="label" style={{ marginBottom: 8 }}>Repeat on</legend>
                  <div style={{ display: "flex", gap: 6 }}>
                    {DAYS.map((d) => {
                      const on = draft.days.includes(d.d);
                      return (
                        <button key={d.d} type="button" aria-pressed={on} aria-label={d.full} onClick={() => set("days", on ? draft.days.filter((x) => x !== d.d) : [...draft.days, d.d])} style={{ flex: 1, height: 42, borderRadius: 10, fontWeight: 600, border: `1px solid ${on ? `color-mix(in srgb, ${draft.color} 50%, transparent)` : "var(--border)"}`, background: on ? `color-mix(in srgb, ${draft.color} 22%, var(--card2))` : "var(--card2)", color: on ? "var(--text)" : "var(--faint)" }}>
                          {d.s}
                        </button>
                      );
                    })}
                  </div>
                </fieldset>
              )}
              <div className="field">
                <label htmlFor="q-done">When finished</label>
                <select id="q-done" className="select" value={draft.whenDone} onChange={(e) => set("whenDone", e.target.value)}>
                  <option value="nothing">Do nothing</option>
                  <option value="shutdown">Shut down PC</option>
                  <option value="sleep">Sleep</option>
                  <option value="hibernate">Hibernate</option>
                  <option value="exit">Close ForgeDM</option>
                </select>
              </div>
              <div className="field">
                <label htmlFor="q-limit">Speed limit</label>
                <input id="q-limit" className="input" placeholder="No limit (e.g. 5 MB)" defaultValue={draft.speedLimit ? fmtBytes(draft.speedLimit) : ""} key={draft.id + "lim"} onBlur={(e) => {
                  const v = e.target.value.trim();
                  set("speedLimit", v ? Math.max(0, parseSize(v) || 0) : 0);
                }} />
              </div>
              {!isMain && (
                <div className="field">
                  <span className="label">Color and icon</span>
                  <div style={{ display: "flex", gap: 6, alignItems: "center", flexWrap: "wrap" }}>
                    {COLORS.map((c) => (
                      <button key={c} type="button" aria-label={`Color ${c}`} aria-pressed={draft.color === c} onClick={() => set("color", c)} style={{ width: 24, height: 24, borderRadius: "50%", background: c, border: draft.color === c ? "2px solid var(--text)" : "2px solid transparent" }} />
                    ))}
                    {ICONS.map((i) => (
                      <button key={i.value} type="button" className="btn icon sm" aria-label={`Icon ${i.value}`} aria-pressed={draft.icon === i.value} onClick={() => set("icon", i.value)} style={{ borderColor: draft.icon === i.value ? "var(--text2)" : undefined }}>
                        <Icon name={i.icon} size={15} />
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>

            <div style={{ display: "grid", gridTemplateColumns: "repeat(2, minmax(0, 1fr))", gap: 10 }}>
              {!isMain && <Toggle label="Wake PC from sleep" hint="So the queue starts on time (needs wake timers allowed in Windows power settings)" on={draft.wakeFromSleep} onChange={(v) => set("wakeFromSleep", v)} />}
              <Toggle label="Retry failed downloads" hint="Network failures get another try when the queue runs" on={draft.retryFailed} onChange={(v) => set("retryFailed", v)} />
              {!isMain && <Toggle label="Pause on metered networks" hint="Mobile hotspots and capped Wi-Fi" on={draft.pauseOnMetered} onChange={(v) => set("pauseOnMetered", v)} />}
              <Toggle label="Use half the bandwidth" hint="Leave room for calls, games and streaming" on={draft.halfBandwidth} onChange={(v) => set("halfBandwidth", v)} />
            </div>

            <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
              <button className="btn primary" disabled={!dirty} onClick={save}>Save changes</button>
              {dirty && <button className="btn ghost" onClick={() => sel && setDraft({ ...sel })}>Undo</button>}
              <div style={{ flex: 1 }} />
              {!draft.builtin && (
                <button className="btn danger" onClick={async () => {
                  await act(api.deleteQueue(draft.id), "Queue deleted. Its downloads moved to the main queue.");
                  setSelId("main");
                }}>Delete queue</button>
              )}
            </div>

            <WeekTimeline queues={queues} />
          </section>
        )}
      </div>
    </>
  );
}

function Toggle({ label, hint, on, onChange }: { label: string; hint: string; on: boolean; onChange: (v: boolean) => void }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 12, padding: "12px 14px", borderRadius: 12, background: "var(--card2)" }}>
      <span className="grow">
        <span style={{ display: "block", fontSize: 13, fontWeight: 500 }}>{label}</span>
        <span className="hint" style={{ display: "block" }}>{hint}</span>
      </span>
      <Switch on={on} onChange={onChange} label={label} />
    </div>
  );
}

function dayText(days: number[]): string {
  if (!days.length || days.length === 7) return "every day";
  const s = [...days].sort();
  if (s.join() === "1,2,3,4,5") return "Mon–Fri";
  if (s.join() === "0,6") return "weekends";
  return s.map((d) => ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"][d]).join(", ");
}

function shortNext(iso?: string): string {
  const t = fmtNext(iso);
  if (t === "Not scheduled") return "Off";
  return t.replace(" at ", " ").replace("Tonight", "Tonight").replace(/day$/, "");
}

function toMin(t: string): number | null {
  const m = /^(\d{1,2}):(\d{2})$/.exec(t || "");
  return m ? Number(m[1]) * 60 + Number(m[2]) : null;
}

function WeekTimeline({ queues }: { queues: Queue[] }) {
  const sched = queues.filter((q) => q.id !== "main" && q.enabled);
  const now = new Date();
  const nowPct = ((now.getHours() * 60 + now.getMinutes()) / 1440) * 100;
  const rows = DAYS.map((day) => {
    const blocks: { left: number; width: number; color: string }[] = [];
    for (const q of sched) {
      const s = toMin(q.start);
      if (s === null) continue;
      const e = toMin(q.stop);
      const prevDay = (day.d + 6) % 7;
      const onDay = q.days.length === 0 || q.days.includes(day.d);
      const onPrev = q.days.length === 0 || q.days.includes(prevDay);
      if (e === null) {
        if (onDay) blocks.push({ left: (s / 1440) * 100, width: Math.min(8, 100 - (s / 1440) * 100), color: q.color });
      } else if (e > s) {
        if (onDay) blocks.push({ left: (s / 1440) * 100, width: ((e - s) / 1440) * 100, color: q.color });
      } else {
        if (onDay) blocks.push({ left: (s / 1440) * 100, width: 100 - (s / 1440) * 100, color: q.color });
        if (onPrev) blocks.push({ left: 0, width: (e / 1440) * 100, color: q.color });
      }
    }
    return { ...day, blocks, today: now.getDay() === day.d };
  });
  return (
    <div className="stack" style={{ gap: 10 }}>
      <div style={{ display: "flex", flexWrap: "wrap", alignItems: "baseline", gap: 14 }}>
        <h3 style={{ margin: 0, flex: 1, fontSize: 14, fontWeight: 600 }}>This week</h3>
        {sched.map((q) => (
          <span key={q.id} className="muted" style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12 }}>
            <span style={{ width: 10, height: 10, borderRadius: 3, background: q.color }} />
            {q.name}
          </span>
        ))}
      </div>
      <div role="img" aria-label={`Weekly schedule: ${sched.map((q) => `${q.name} ${q.start}${q.stop ? ` to ${q.stop}` : ""} ${dayText(q.days)}`).join("; ")}`} className="stack" style={{ gap: 6 }}>
        <div style={{ display: "flex", gap: 10 }}>
          <span style={{ width: 36 }} />
          <div className="mono faint" style={{ flex: 1, display: "flex", justifyContent: "space-between", fontSize: 11 }}>
            {["00", "03", "06", "09", "12", "15", "18", "21", "24"].map((h) => <span key={h}>{h}</span>)}
          </div>
        </div>
        {rows.map((r) => (
          <div key={r.full} style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <span style={{ width: 36, fontSize: 12, fontWeight: 500, color: r.today ? "var(--accent-text)" : "var(--muted)" }}>{r.full.slice(0, 3)}</span>
            <div style={{ position: "relative", flex: 1, height: 30, borderRadius: 8, backgroundColor: "var(--card2)", backgroundImage: "repeating-linear-gradient(to right, var(--grid) 0 1px, transparent 1px 12.5%)" }}>
              {r.blocks.map((b, i) => (
                <span key={i} style={{ position: "absolute", top: 4, bottom: 4, left: `${b.left}%`, width: `${b.width}%`, borderRadius: 6, background: b.color }} />
              ))}
              {r.today && <span style={{ position: "absolute", top: -3, bottom: -3, left: `${nowPct}%`, width: 2, borderRadius: 2, background: "var(--accent)" }} />}
            </div>
          </div>
        ))}
        <div className="faint" style={{ paddingLeft: 46, fontSize: 12 }}>The line marks now. The main queue runs any time it has files.</div>
      </div>
    </div>
  );
}
