import { useEffect, useState } from "react";
import { api } from "../api";
import { Modal } from "../components/ui";
import { fmtBytes, parseSize } from "../format";
import { FileIcon, Icon, catStyle } from "../icons";
import { useAction, useStore } from "../store";
import type { BatchLink, Condition, PendingCapture, Rule } from "../types";

const CATS = ["Video", "Music", "Documents", "Compressed", "Programs", "Images", "Other"];

export function RemoveDialog({ ids, onClose }: { ids: string[]; onClose: () => void }) {
  const { snap } = useStore();
  const [del, setDel] = useState(false);
  const list = (snap?.downloads ?? []).filter((d) => ids.includes(d.id));
  const finished = list.filter((d) => d.status === "completed").length;
  const one = list.length === 1 ? list[0] : null;
  return (
    <Modal title="Remove downloads" onClose={onClose} width={520}>
      <h1>{one ? `Remove ${one.filename}?` : `Remove ${list.length} downloads?`}</h1>
      <p className="muted" style={{ margin: 0 }}>They disappear from your list. Unfinished parts are deleted.</p>
      {finished > 0 && (
        <label className="check">
          <input type="checkbox" checked={del} onChange={(e) => setDel(e.target.checked)} />
          Also delete the {finished === 1 ? "downloaded file" : `${finished} downloaded files`} from disk
        </label>
      )}
      <div className="actions">
        <div style={{ flex: 1 }} />
        <button className="btn ghost" onClick={onClose}>Cancel</button>
        <button className="btn danger" onClick={() => { api.remove(ids, del); onClose(); }}>{del ? "Remove and delete" : "Remove"}</button>
      </div>
    </Modal>
  );
}

export function RefreshDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const { snap, toast } = useStore();
  const d = snap?.downloads.find((x) => x.id === id);
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  if (!d) return null;
  const submit = async () => {
    setBusy(true);
    setErr("");
    try {
      await api.refreshLink(id, url);
      toast("success", "Link updated. The download continues where it stopped.");
      onClose();
    } catch (e: any) {
      const msg = typeof e === "string" ? e : e?.message ?? "Couldn't use that link";
      if (msg.includes("starts over")) {
        toast("info", "The new link points to a different file, so the download starts over.");
        onClose();
      } else setErr(msg);
    }
    setBusy(false);
  };
  return (
    <Modal title="Refresh link" onClose={onClose} width={620}>
      <form className="stack" style={{ gap: 14 }} onSubmit={(e) => { e.preventDefault(); submit(); }}>
        <h1>Refresh the link</h1>
        <div style={{ display: "flex", gap: 12, alignItems: "center" }}>
          <FileIcon category={d.category} size={40} />
          <div className="grow">
            <div className="ellipsis" style={{ fontWeight: 600 }}>{d.filename}</div>
            <div className="muted" style={{ fontSize: 12 }}>{fmtBytes(d.downloaded)} of {fmtBytes(d.size)} already downloaded</div>
          </div>
        </div>
        <p className="muted" style={{ margin: 0, fontSize: 13 }}>
          Some sites give links that stop working after a while. Open the download page in your browser again, copy the new download link and paste it here. If it's the same file, ForgeDM keeps what's already downloaded.
        </p>
        <div className="field">
          <label htmlFor="ref-url">New link</label>
          <input id="ref-url" className="input mono" type="url" required value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://…" />
        </div>
        {err && <div className="notice bad">{err}</div>}
        <div className="actions">
          <div style={{ flex: 1 }} />
          <button type="button" className="btn ghost" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn primary" disabled={busy || !url.trim()}>Continue download</button>
        </div>
      </form>
    </Modal>
  );
}

export function ImportDialog({ onClose }: { onClose: () => void }) {
  const { snap, toast } = useStore();
  const act = useAction();
  const [text, setText] = useState("");
  const [queue, setQueue] = useState("main");
  const [dir, setDir] = useState("");
  const links = Array.from(new Set(text.split(/[\s,;]+/).map((s) => s.trim()).filter((s) => /^https?:\/\/\S+$/i.test(s))));
  const add = async (start: boolean) => {
    const n = await act(api.addMany(links.map((url) => ({ url, dir: dir || undefined, queue, start, post: { verify: !!snap?.settings.verifyChecksum, scan: false, extract: false, open: false }, source: "import" }))));
    if (n !== undefined) {
      toast("success", `${n} download${n === 1 ? "" : "s"} added`);
      onClose();
    }
  };
  return (
    <Modal title="Add several links" onClose={onClose} width={680}>
      <h1>Add several links</h1>
      <div className="field">
        <label htmlFor="imp-text">Paste links, one per line</label>
        <textarea id="imp-text" className="input mono" rows={9} value={text} onChange={(e) => setText(e.target.value)} placeholder={"https://example.com/part1.zip\nhttps://example.com/part2.zip"} />
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <span className="hint grow">{links.length} link{links.length === 1 ? "" : "s"} found</span>
          <button className="btn sm" onClick={async () => { const l = await act(api.importList()); if (l?.length) setText((t) => (t ? t + "\n" : "") + l.join("\n")); }}>
            <Icon name="doc" size={15} />Load from a text file
          </button>
        </div>
      </div>
      <div style={{ display: "flex", gap: 12 }}>
        <div className="field grow">
          <label htmlFor="imp-dir">Save to</label>
          <div style={{ display: "flex", gap: 8 }}>
            <input id="imp-dir" className="input" value={dir} onChange={(e) => setDir(e.target.value)} placeholder="Folder for each file's category" />
            <button className="btn" onClick={async () => { const d = await api.chooseFolder(dir); if (d) setDir(d); }}>Browse</button>
          </div>
        </div>
        <div className="field" style={{ width: 200 }}>
          <label htmlFor="imp-q">Queue</label>
          <select id="imp-q" className="select" value={queue} onChange={(e) => setQueue(e.target.value)}>
            {(snap?.queues ?? []).map((q) => <option key={q.id} value={q.id}>{q.name}</option>)}
          </select>
        </div>
      </div>
      <div className="actions">
        <div style={{ flex: 1 }} />
        <button className="btn ghost" onClick={onClose}>Cancel</button>
        <button className="btn" disabled={!links.length} onClick={() => add(false)}>Add to queue</button>
        <button className="btn primary" disabled={!links.length} onClick={() => add(true)}>Start {links.length || ""} now</button>
      </div>
    </Modal>
  );
}

const FIELDS: { value: string; label: string; placeholder: string }[] = [
  { value: "type", label: "Type is", placeholder: "" },
  { value: "ext", label: "File ends in", placeholder: "iso, img" },
  { value: "site", label: "Site is", placeholder: "example.com" },
  { value: "sizeOver", label: "Size is over", placeholder: "4 GB" },
  { value: "sizeUnder", label: "Size is under", placeholder: "10 MB" },
  { value: "name", label: "Name contains", placeholder: "setup or *.part1.rar" },
  { value: "url", label: "Link contains", placeholder: "/releases/" },
];

export function RuleDialog({ id, onClose }: { id?: string; onClose: () => void }) {
  const { snap } = useStore();
  const act = useAction();
  const existing = snap?.rules.find((r) => r.id === id);
  const [rule, setRule] = useState<Rule>(existing ? JSON.parse(JSON.stringify(existing)) : { id: "", name: "", enabled: true, conditions: [{ field: "type", value: "Video" }], actions: {} });
  const [sizeText, setSizeText] = useState<Record<number, string>>(() => Object.fromEntries((existing?.conditions ?? []).map((c, i) => [i, c.field.startsWith("size") ? fmtBytes(Number(c.value)) : ""])));
  const setCond = (i: number, c: Partial<Condition>) => setRule((r) => ({ ...r, conditions: r.conditions.map((x, k) => (k === i ? { ...x, ...c } : x)) }));
  const save = async () => {
    const conds = rule.conditions
      .map((c, i) => (c.field.startsWith("size") ? { ...c, value: String(parseSize(sizeText[i] ?? "") || 0) } : c))
      .filter((c) => c.value && c.value !== "0");
    if (!conds.length) return;
    const r = { ...rule, conditions: conds, name: rule.name || conds.map((c) => `${c.field} ${c.value}`).join(", ") };
    const list = existing ? (snap?.rules ?? []).map((x) => (x.id === r.id ? r : x)) : [...(snap?.rules ?? []), r];
    await act(api.saveRules(list), existing ? "Rule saved" : "Rule added");
    onClose();
  };
  const a = rule.actions;
  return (
    <Modal title={existing ? "Edit rule" : "New rule"} onClose={onClose} width={720}>
      <h1>{existing ? "Edit save rule" : "New save rule"}</h1>
      <div className="field">
        <label htmlFor="rule-name">Name</label>
        <input id="rule-name" className="input" value={rule.name} onChange={(e) => setRule({ ...rule, name: e.target.value })} placeholder="For example: Installers from work" />
      </div>
      <fieldset className="stack" style={{ gap: 8 }}>
        <legend className="label" style={{ marginBottom: 8 }}>When all of these are true</legend>
        {rule.conditions.map((c, i) => {
          const f = FIELDS.find((x) => x.value === c.field)!;
          return (
            <div key={i} style={{ display: "flex", gap: 8 }}>
              <select aria-label="Condition" className="select" style={{ width: 180 }} value={c.field} onChange={(e) => setCond(i, { field: e.target.value, value: e.target.value === "type" ? "Video" : "" })}>
                {FIELDS.map((x) => <option key={x.value} value={x.value}>{x.label}</option>)}
              </select>
              {c.field === "type" ? (
                <select aria-label="Type" className="select" value={c.value} onChange={(e) => setCond(i, { value: e.target.value })}>
                  {CATS.map((x) => <option key={x}>{x}</option>)}
                </select>
              ) : c.field.startsWith("size") ? (
                <input aria-label="Size" className="input" placeholder={f.placeholder} value={sizeText[i] ?? ""} onChange={(e) => { setSizeText({ ...sizeText, [i]: e.target.value }); setCond(i, { value: "x" }); }} />
              ) : (
                <input aria-label="Value" className="input" placeholder={f.placeholder} value={c.value} onChange={(e) => setCond(i, { value: e.target.value })} />
              )}
              <button className="btn icon" aria-label="Remove condition" disabled={rule.conditions.length === 1} onClick={() => setRule({ ...rule, conditions: rule.conditions.filter((_, k) => k !== i) })}><Icon name="x" size={15} /></button>
            </div>
          );
        })}
        <button className="btn sm ghost" style={{ alignSelf: "flex-start" }} onClick={() => setRule({ ...rule, conditions: [...rule.conditions, { field: "site", value: "" }] })}><Icon name="plus" size={14} />Add condition</button>
      </fieldset>
      <fieldset className="stack" style={{ gap: 12 }}>
        <legend className="label" style={{ marginBottom: 8 }}>Then</legend>
        <div className="field">
          <label htmlFor="rule-dir">Save to</label>
          <div style={{ display: "flex", gap: 8 }}>
            <input id="rule-dir" className="input" value={a.dir ?? ""} onChange={(e) => setRule({ ...rule, actions: { ...a, dir: e.target.value } })} placeholder="The category's folder" />
            <button className="btn" onClick={async () => { const d = await api.chooseFolder(a.dir ?? ""); if (d) setRule({ ...rule, actions: { ...a, dir: d } }); }}>Browse</button>
          </div>
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(3, minmax(0,1fr))", gap: 12 }}>
          <div className="field">
            <label htmlFor="rule-cat">Category</label>
            <select id="rule-cat" className="select" value={a.category ?? ""} onChange={(e) => setRule({ ...rule, actions: { ...a, category: e.target.value || undefined } })}>
              <option value="">Keep detected</option>
              {CATS.map((x) => <option key={x}>{x}</option>)}
            </select>
          </div>
          <div className="field">
            <label htmlFor="rule-q">Queue</label>
            <select id="rule-q" className="select" value={a.queue ?? ""} onChange={(e) => setRule({ ...rule, actions: { ...a, queue: e.target.value || undefined } })}>
              <option value="">Main queue</option>
              {(snap?.queues ?? []).filter((q) => q.id !== "main").map((q) => <option key={q.id} value={q.id}>{q.name}</option>)}
            </select>
          </div>
          <div className="field">
            <label htmlFor="rule-tags">Tags</label>
            <input id="rule-tags" className="input" value={(a.tags ?? []).join(", ")} onChange={(e) => setRule({ ...rule, actions: { ...a, tags: e.target.value.split(/[,\s]+/).map((t) => t.replace(/^#/, "")).filter(Boolean) } })} placeholder="work" />
          </div>
        </div>
        <div style={{ display: "flex", gap: 20 }}>
          <label className="check"><input type="checkbox" checked={!!a.extract} onChange={(e) => setRule({ ...rule, actions: { ...a, extract: e.target.checked } })} />Extract archives when done</label>
          <label className="check"><input type="checkbox" checked={!!a.open} onChange={(e) => setRule({ ...rule, actions: { ...a, open: e.target.checked } })} />Open when done</label>
        </div>
      </fieldset>
      <div className="actions">
        <div style={{ flex: 1 }} />
        <button className="btn ghost" onClick={onClose}>Cancel</button>
        <button className="btn primary" onClick={save}>{existing ? "Save rule" : "Add rule"}</button>
      </div>
    </Modal>
  );
}

export function ConnectDialog({ onClose }: { onClose: () => void }) {
  const [folder, setFolder] = useState("");
  useEffect(() => {
    api.extensionFolder().then((f) => setFolder(f || ""));
  }, []);
  const steps = [
    { t: "Open the extensions page", d: "In Chrome go to chrome://extensions, in Edge go to edge://extensions, in Brave go to brave://extensions." },
    { t: "Turn on Developer mode", d: "It's a switch at the top right (Chrome, Brave) or at the left (Edge)." },
    { t: "Load the ForgeDM extension", d: "Click \"Load unpacked\" and choose the extension folder that came with ForgeDM (\"extension\\chrome\" next to ForgeDM.exe)." },
    { t: "Connect", d: "Click the ForgeDM icon in the browser toolbar, then Connect. Approve it in the window ForgeDM shows." },
  ];
  return (
    <Modal title="Connect a browser" onClose={onClose} width={640}>
      <h1>Connect your browser</h1>
      <p className="muted" style={{ margin: 0 }}>The ForgeDM extension sends download links, videos and "Download all" selections to the app.</p>
      <ol className="stack" style={{ gap: 12, margin: 0, padding: 0, listStyle: "none" }}>
        {steps.map((s, i) => (
          <li key={s.t} style={{ display: "flex", gap: 12 }}>
            <span className="mono" style={{ width: 26, height: 26, borderRadius: 8, display: "grid", placeItems: "center", background: "var(--accent-soft)", color: "var(--accent-text)", fontWeight: 600, flex: "none" }}>{i + 1}</span>
            <span>
              <span style={{ display: "block", fontWeight: 600 }}>{s.t}</span>
              <span className="muted" style={{ fontSize: 13 }}>{s.d}</span>
            </span>
          </li>
        ))}
      </ol>
      <div className="notice info" style={{ fontSize: 13 }}>
        <Icon name="info" size={16} />
        <span className="grow">For Firefox, open about:debugging → This Firefox → Load Temporary Add-on, and pick manifest.json in the "extension\firefox" folder.</span>
      </div>
      <div className="actions">
        {folder && <button className="btn" onClick={() => api.openFolder(folder)}><Icon name="folder" size={16} />Open extension folder</button>}
        <div style={{ flex: 1 }} />
        <button className="btn primary" onClick={onClose}>Done</button>
      </div>
    </Modal>
  );
}

export function CaptureDialog({ pc, onDone }: { pc: PendingCapture; onDone: () => void }) {
  const { snap } = useStore();
  const act = useAction();
  const p = pc.probe;
  const [name, setName] = useState(p?.filename ?? "");
  const [dir, setDir] = useState(p?.decision?.dir ?? "");
  const [touched, setTouched] = useState(false);
  const [variant, setVariant] = useState("");
  useEffect(() => {
    if (!touched && p) {
      setName(p.filename);
      setDir(p.decision?.dir ?? "");
    }
    if (p?.variants?.length && !variant) setVariant(p.variants[0].url);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [p]);
  const category = p?.decision?.category ?? "Other";
  const answer = async (action: "start" | "later" | "cancel") => {
    await act(api.answerCapture(pc.id, { url: variant || pc.capture.url, kind: p?.kind === "hls" ? "hls" : undefined, filename: name, dir, category, queue: p?.decision?.queue, post: { verify: !!snap?.settings.verifyChecksum, scan: !!snap?.settings.scanAfter, extract: !!p?.decision?.extract, open: !!snap?.settings.openWhenDone || !!p?.decision?.open }, start: action === "start", size: p && p.size > 0 ? p.size : undefined, rule: p?.decision?.ruleName }, action));
    onDone();
  };
  return (
    <Modal title="Download caught from the browser" onClose={() => answer("cancel")} width={560}>
      <div className="muted" style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13 }}>
        <Icon name="puzzle" size={15} />ForgeDM caught a download from {pc.browser || "your browser"}
      </div>
      <div style={{ display: "flex", gap: 12, alignItems: "center" }}>
        <FileIcon category={category} size={44} radius={12} />
        <div className="grow">
          <input aria-label="File name" className="input" value={name} onChange={(e) => { setTouched(true); setName(e.target.value); }} style={{ fontWeight: 600 }} />
          <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>
            {p && p.size >= 0 ? fmtBytes(p.size) : "Checking size…"} · {new URL(pc.capture.url).hostname}
            {p?.error ? ` · ${p.error}` : ""}
          </div>
        </div>
      </div>
      {p?.variants && p.variants.length > 0 && (
        <div className="field">
          <label htmlFor="cap-q">Quality</label>
          <select id="cap-q" className="select" value={variant} onChange={(e) => setVariant(e.target.value)}>
            {p.variants.map((v) => <option key={v.url} value={v.url}>{v.label}</option>)}
          </select>
        </div>
      )}
      <div className="field">
        <label htmlFor="cap-dir">Save to</label>
        <div style={{ display: "flex", gap: 8 }}>
          <input id="cap-dir" className="input" value={dir} onChange={(e) => { setTouched(true); setDir(e.target.value); }} />
          <button className="btn" onClick={async () => { const d = await api.chooseFolder(dir); if (d) { setTouched(true); setDir(d); } }}>Browse</button>
        </div>
        {p?.decision?.ruleName && <span className="hint">Save rule: {p.decision.ruleName}</span>}
      </div>
      {p?.duplicate && <div className="notice warn">This file is already in your list.</div>}
      <div className="actions">
        <button className="btn ghost" onClick={() => answer("cancel")}>Don't download</button>
        <div style={{ flex: 1 }} />
        <button className="btn" onClick={() => answer("later")}>Later</button>
        <button className="btn primary" onClick={() => answer("start")}><Icon name="down" size={16} stroke={2.2} />Start</button>
      </div>
    </Modal>
  );
}

export function PairDialog({ req, onDone }: { req: { id: string; name: string }; onDone: () => void }) {
  const answer = (ok: boolean) => {
    api.answerPairing(req.id, ok);
    onDone();
  };
  return (
    <Modal title="Connect browser" onClose={() => answer(false)} width={480}>
      <span className="icon-tile" style={{ width: 52, height: 52, borderRadius: 14, background: "var(--accent-soft)", color: "var(--accent-text)" }}>
        <Icon name="puzzle" size={24} />
      </span>
      <h1>Connect {req.name} to ForgeDM?</h1>
      <p className="muted" style={{ margin: 0 }}>The browser extension will be able to send downloads to ForgeDM. Only allow this if you just clicked Connect in the extension.</p>
      <div className="actions">
        <div style={{ flex: 1 }} />
        <button className="btn ghost" onClick={() => answer(false)}>Decline</button>
        <button className="btn primary" onClick={() => answer(true)}>Allow</button>
      </div>
    </Modal>
  );
}

export function BatchDialog({ batch, onDone }: { batch: { id: string; pageUrl: string; links: BatchLink[]; browser: string }; onDone: () => void }) {
  const { snap, toast } = useStore();
  const [picked, setPicked] = useState<Set<string>>(() => new Set(batch.links.filter((l) => l.type !== "Other").map((l) => l.url)));
  const [dir, setDir] = useState("");
  const [queue, setQueue] = useState("main");
  const types = Array.from(new Set(batch.links.map((l) => l.type)));
  const go = async (start: boolean) => {
    const n = await api.answerBatch(batch.id, [...picked], dir, queue, start).catch(() => 0);
    toast("success", `${n} download${n === 1 ? "" : "s"} added`);
    onDone();
  };
  return (
    <Modal title="Download links" onClose={() => { api.answerBatch(batch.id, [], "", "main", false).catch(() => {}); onDone(); }} width={720}>
      <h1>{batch.links.length} links from {batch.pageUrl ? new URL(batch.pageUrl).hostname : "the browser"}</h1>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
        {types.map((t) => {
          const ofType = batch.links.filter((l) => l.type === t).map((l) => l.url);
          const all = ofType.every((u) => picked.has(u));
          return (
            <button key={t} className="filter-chip" aria-pressed={all} onClick={() => setPicked((s) => { const n = new Set(s); ofType.forEach((u) => (all ? n.delete(u) : n.add(u))); return n; })}>
              <span style={{ width: 8, height: 8, borderRadius: 3, background: catStyle(t).color }} />{t}<span className="n">{ofType.length}</span>
            </button>
          );
        })}
      </div>
      <div className="card tight" style={{ maxHeight: 300, overflowY: "auto", padding: 6 }}>
        {batch.links.map((l) => (
          <label key={l.url} style={{ display: "flex", alignItems: "center", gap: 10, padding: "6px 8px", borderRadius: 8, fontSize: 13 }}>
            <input type="checkbox" checked={picked.has(l.url)} onChange={() => setPicked((s) => { const n = new Set(s); n.has(l.url) ? n.delete(l.url) : n.add(l.url); return n; })} style={{ accentColor: "var(--accent)" }} />
            <span style={{ width: 8, height: 8, borderRadius: 3, background: catStyle(l.type).color, flex: "none" }} />
            <span className="ellipsis grow">{l.name}</span>
            <span className="faint mono ellipsis" style={{ fontSize: 11, maxWidth: 260 }}>{l.url}</span>
          </label>
        ))}
      </div>
      <div style={{ display: "flex", gap: 12 }}>
        <div className="field grow">
          <label htmlFor="b-dir">Save to</label>
          <div style={{ display: "flex", gap: 8 }}>
            <input id="b-dir" className="input" value={dir} onChange={(e) => setDir(e.target.value)} placeholder="Folder for each file's category" />
            <button className="btn" onClick={async () => { const d = await api.chooseFolder(dir); if (d) setDir(d); }}>Browse</button>
          </div>
        </div>
        <div className="field" style={{ width: 200 }}>
          <label htmlFor="b-q">Queue</label>
          <select id="b-q" className="select" value={queue} onChange={(e) => setQueue(e.target.value)}>
            {(snap?.queues ?? []).map((q) => <option key={q.id} value={q.id}>{q.name}</option>)}
          </select>
        </div>
      </div>
      <div className="actions">
        <div style={{ flex: 1 }} />
        <button className="btn" disabled={!picked.size} onClick={() => go(false)}>Add to queue</button>
        <button className="btn primary" disabled={!picked.size} onClick={() => go(true)}>Download {picked.size}</button>
      </div>
    </Modal>
  );
}

export function ClipboardBanner({ clip, onDismiss }: { clip: { url: string; name: string; size: number }; onDismiss: () => void }) {
  const { open } = useStore();
  return (
    <section aria-label="Link found in clipboard" className="notice info" style={{ borderRadius: 14, padding: "12px 14px 12px 16px" }}>
      <span className="icon-tile" style={{ width: 34, height: 34, borderRadius: 10, background: "var(--accent-soft)", color: "var(--accent-text)" }}>
        <Icon name="link" />
      </span>
      <div className="grow" style={{ minWidth: 0 }}>
        <div style={{ fontWeight: 600 }}>Link found in your clipboard</div>
        <div className="mono ellipsis muted" style={{ fontSize: 12 }}>{clip.url}{clip.size > 0 ? ` · ${fmtBytes(clip.size)}` : ""}</div>
      </div>
      <button className="btn primary sm" onClick={() => { open({ kind: "add", url: clip.url }); onDismiss(); }}>Download</button>
      <button className="btn ghost sm" onClick={() => { api.dismissClipboard(clip.url); onDismiss(); }}>Dismiss</button>
    </section>
  );
}

