import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api";
import { Modal, Seg } from "../components/ui";
import { fmtBytes, parseSize } from "../format";
import { Icon } from "../icons";
import { useAction, useStore } from "../store";
import type { AddOptions, ProbeView } from "../types";

const CATS = ["Video", "Music", "Documents", "Compressed", "Programs", "Images", "Other"];
const ARCHIVE = /\.(zip|7z|rar|tar|tgz|tar\.gz|tar\.bz2|tbz2)$/i;

async function readClipboard(): Promise<string> {
  try {
    if (window.runtime?.ClipboardGetText) return (await window.runtime.ClipboardGetText()) ?? "";
    return await navigator.clipboard.readText();
  } catch {
    return "";
  }
}

export function AddDownload({ initialUrl, onClose }: { initialUrl?: string; onClose: () => void }) {
  const { snap, go, open } = useStore();
  const act = useAction();
  const [url, setUrl] = useState(initialUrl ?? "");
  const [probe, setProbe] = useState<ProbeView | null>(null);
  const [checking, setChecking] = useState(false);
  const [name, setName] = useState("");
  const [category, setCategory] = useState("");
  const [dir, setDir] = useState("");
  const [conns, setConns] = useState(0);
  const [limit, setLimit] = useState("");
  const [tags, setTags] = useState("");
  const [verify, setVerify] = useState(snap?.settings.verifyChecksum ?? true);
  const [openAfter, setOpenAfter] = useState(false);
  const [scan, setScan] = useState(snap?.settings.scanAfter ?? false);
  const [extract, setExtract] = useState(false);
  const [queue, setQueue] = useState("main");
  const [variant, setVariant] = useState("");
  const [referer, setReferer] = useState("");
  const [cookies, setCookies] = useState("");
  const [user, setUser] = useState("");
  const [pass, setPass] = useState("");
  const [checksum, setChecksum] = useState("");
  const [ignoreDup, setIgnoreDup] = useState(false);
  const [busy, setBusy] = useState(false);
  const touched = useRef({ name: false, dir: false, category: false });
  const seq = useRef(0);

  useEffect(() => {
    if (initialUrl) return;
    readClipboard().then((t) => {
      const v = t.trim();
      if (/^https?:\/\/\S+$/i.test(v) && !url) setUrl(v);
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const headers = useMemo(() => {
    const h: Record<string, string> = {};
    if (referer.trim()) h["Referer"] = referer.trim();
    if (cookies.trim()) h["Cookie"] = cookies.trim();
    if (user) h["Authorization"] = "Basic " + btoa(unescape(encodeURIComponent(`${user}:${pass}`)));
    return h;
  }, [referer, cookies, user, pass]);

  useEffect(() => {
    const v = url.trim();
    if (!/^https?:\/\/\S+\.\S+/i.test(v)) {
      setProbe(null);
      return;
    }
    const my = ++seq.current;
    setChecking(true);
    const t = setTimeout(async () => {
      const p = await api.probe(v, Object.keys(headers).length ? headers : undefined).catch((e) => ({ error: String(e) }) as any);
      if (my !== seq.current) return;
      setChecking(false);
      setProbe(p);
      if (!p) return;
      if (!touched.current.name && p.filename) setName(p.filename);
      if (!touched.current.category && p.decision) setCategory(p.decision.category);
      if (!touched.current.dir && p.decision) setDir(p.decision.dir);
      if (p.decision) {
        setQueue(p.decision.queue || "main");
        if (p.decision.extract) setExtract(true);
        if (p.decision.open) setOpenAfter(true);
        if (p.decision.tags?.length && !tags) setTags(p.decision.tags.join(", "));
      }
      if (p.variants?.length) setVariant(p.variants[0].url);
      setIgnoreDup(false);
    }, 450);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [url, headers]);

  const isArchive = ARCHIVE.test(name);
  const dup = probe?.duplicate && !ignoreDup ? probe.duplicate : null;
  const queues = snap?.queues ?? [];

  const submit = async (start: boolean) => {
    const v = url.trim();
    if (!v) return;
    setBusy(true);
    const hls = probe?.kind === "hls";
    const o: AddOptions = {
      url: hls && variant ? variant : v,
      kind: hls ? "hls" : undefined,
      filename: name.trim() || undefined,
      dir: dir.trim() || undefined,
      category: category || undefined,
      tags: tags.split(/[,\s]+/).map((t) => t.replace(/^#/, "").trim()).filter(Boolean),
      queue,
      connections: conns,
      speedLimit: limit.trim() ? Math.max(0, parseSize(limit) || 0) : 0,
      post: { verify: verify || !!checksum.trim(), scan, extract: extract && isArchive, open: openAfter },
      checksum: checksum.trim() || undefined,
      start,
      headers: Object.keys(headers).length ? headers : undefined,
      size: probe && probe.size > 0 ? probe.size : undefined,
      rule: probe?.decision?.ruleName || undefined,
    };
    const d = await act(api.add(o), start ? undefined : `Added to ${queues.find((q) => q.id === queue)?.name ?? "the queue"}`);
    setBusy(false);
    if (d) {
      onClose();
      if (start) open({ kind: "details", id: d.id });
    }
  };

  const checks: string[] = [];
  if (probe && !probe.error) {
    if (probe.kind === "hls") checks.push(`Video stream${probe.variants?.length ? ` · ${probe.variants.length} qualities` : ""}`);
    else if (probe.contentType) checks.push(typeLabel(name, probe.contentType));
    if (probe.size >= 0) checks.push(fmtBytes(probe.size));
    checks.push(probe.resumable ? "Can resume" : "Can't resume if interrupted");
    if (probe.resumable && probe.kind !== "hls") checks.push("Fast multi-connection download");
  }

  return (
    <Modal title="New download" onClose={onClose} width={760}>
      <form onSubmit={(e) => { e.preventDefault(); submit(true); }} className="stack" style={{ gap: 16 }}>
        <h1>New download</h1>
        <div className="field">
          <label htmlFor="add-url">Link</label>
          <div style={{ display: "flex", gap: 8 }}>
            <input id="add-url" className="input mono" type="url" required placeholder="https://example.com/file.zip" value={url} onChange={(e) => setUrl(e.target.value)} style={{ height: 44, borderColor: probe && !probe.error ? "var(--accent-line)" : undefined }} />
            <button type="button" className="btn" style={{ height: 44 }} onClick={async () => setUrl((await readClipboard()).trim())}>Paste</button>
          </div>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 8, minHeight: 26, alignItems: "center" }}>
            {checking && <span className="muted" style={{ fontSize: 12 }}>Checking the link…</span>}
            {!checking && probe?.error && <span style={{ fontSize: 12, color: "var(--bad-text)" }}>{probe.error} You can still try to download it.</span>}
            {!checking && checks.map((c) => (
              <span key={c} className="chip accent">
                <Icon name="check" size={12} stroke={2.8} />{c}
              </span>
            ))}
          </div>
        </div>

        {dup && (
          <div role="status" className="notice warn">
            <span style={{ color: "var(--orange)", display: "grid" }}><Icon name="warn" size={18} stroke={2} /></span>
            <div className="grow">This file is already in your list ({dup.status === "completed" ? "downloaded" : dup.status}).</div>
            <button type="button" className="btn xs ghost" onClick={() => { onClose(); go("downloads"); }}>Show it</button>
            <button type="button" className="btn xs" onClick={() => setIgnoreDup(true)}>Download again</button>
          </div>
        )}

        {probe?.kind === "hls" && (probe.variants?.length ?? 0) > 0 && (
          <div className="field">
            <label htmlFor="add-q">Quality</label>
            <select id="add-q" className="select" value={variant} onChange={(e) => setVariant(e.target.value)}>
              {probe.variants!.map((v) => <option key={v.url} value={v.url}>{v.label}{v.bandwidth ? ` · ${(v.bandwidth / 1e6).toFixed(1)} Mbps` : ""}</option>)}
            </select>
          </div>
        )}

        <div style={{ display: "flex", gap: 14 }}>
          <div className="field" style={{ flex: 2 }}>
            <label htmlFor="add-name">File name</label>
            <input id="add-name" className="input" value={name} onChange={(e) => { touched.current.name = true; setName(e.target.value); }} placeholder="Detected from the link" />
          </div>
          <div className="field" style={{ flex: 1 }}>
            <label htmlFor="add-cat">Category</label>
            <select id="add-cat" className="select" value={category} onChange={(e) => {
              touched.current.category = true;
              setCategory(e.target.value);
              if (!touched.current.dir) {
                const c = snap?.categories.find((x) => x.name === e.target.value);
                if (c) setDir(c.dir);
              }
            }}>
              <option value="">Automatic</option>
              {CATS.map((c) => <option key={c}>{c}</option>)}
            </select>
          </div>
        </div>

        <div className="field">
          <label htmlFor="add-dir">Save to</label>
          <div style={{ display: "flex", gap: 8 }}>
            <input id="add-dir" className="input" value={dir} onChange={(e) => { touched.current.dir = true; setDir(e.target.value); }} placeholder={snap?.settings.baseDir} />
            <button type="button" className="btn" onClick={async () => { const d = await api.chooseFolder(dir); if (d) { touched.current.dir = true; setDir(d); } }}>Browse</button>
          </div>
          {probe?.decision?.ruleName && (
            <div className="muted" style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12 }}>
              <span className="chip accent">Save rule</span>
              <span className="ellipsis grow">{probe.decision.ruleName}</span>
              <button type="button" className="btn xs ghost" onClick={() => { onClose(); go("settings"); }}>Edit rules</button>
            </div>
          )}
        </div>

        <div style={{ display: "flex", flexWrap: "wrap", gap: 14, alignItems: "flex-end" }}>
          <fieldset style={{ flex: "2 1 300px" }}>
            <legend className="label" style={{ marginBottom: 8 }}>Connections</legend>
            <Seg accent label="Connections" value={conns} onChange={setConns} options={[{ value: 0, label: "Auto" }, { value: 4, label: "4" }, { value: 8, label: "8" }, { value: 16, label: "16" }, { value: 32, label: "32" }]} />
          </fieldset>
          <div className="field" style={{ flex: "1 1 160px" }}>
            <label htmlFor="add-limit">Speed limit</label>
            <input id="add-limit" className="input" placeholder="No limit" value={limit} onChange={(e) => setLimit(e.target.value)} />
          </div>
          <div className="field" style={{ flex: "1 1 160px" }}>
            <label htmlFor="add-tags">Tags</label>
            <input id="add-tags" className="input" placeholder="work, iso" value={tags} onChange={(e) => setTags(e.target.value)} />
          </div>
        </div>

        <fieldset>
          <legend className="label" style={{ marginBottom: 10 }}>When it finishes</legend>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 20 }}>
            <label className="check"><input type="checkbox" checked={verify} onChange={(e) => setVerify(e.target.checked)} />Calculate checksum</label>
            <label className="check"><input type="checkbox" checked={openAfter} onChange={(e) => setOpenAfter(e.target.checked)} />Open the file</label>
            <label className="check"><input type="checkbox" checked={scan} onChange={(e) => setScan(e.target.checked)} />Scan with Microsoft Defender</label>
            <label className="check" style={{ color: isArchive ? undefined : "var(--faint)" }}><input type="checkbox" disabled={!isArchive} checked={extract && isArchive} onChange={(e) => setExtract(e.target.checked)} />Extract archive</label>
          </div>
        </fieldset>

        <details className="card tight" style={{ padding: 0 }}>
          <summary style={{ display: "flex", alignItems: "center", gap: 10, padding: "12px 14px", fontSize: 13, fontWeight: 500, cursor: "pointer", listStyle: "none" }}>
            <Icon name="chevRight" size={16} stroke={2} />
            Advanced
            <span className="faint" style={{ fontWeight: 400 }}>Referrer, cookies, login, expected checksum</span>
          </summary>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(2, minmax(0, 1fr))", gap: 12, padding: "0 14px 14px" }}>
            <div className="field"><label htmlFor="adv-ref">Referrer page</label><input id="adv-ref" className="input mono" value={referer} onChange={(e) => setReferer(e.target.value)} placeholder="https://…" /></div>
            <div className="field"><label htmlFor="adv-cookie">Cookies</label><input id="adv-cookie" className="input mono" value={cookies} onChange={(e) => setCookies(e.target.value)} placeholder="name=value; other=value" /></div>
            <div className="field"><label htmlFor="adv-user">User name</label><input id="adv-user" className="input" autoComplete="off" value={user} onChange={(e) => setUser(e.target.value)} /></div>
            <div className="field"><label htmlFor="adv-pass">Password</label><input id="adv-pass" className="input" type="password" autoComplete="off" value={pass} onChange={(e) => setPass(e.target.value)} /></div>
            <div className="field" style={{ gridColumn: "span 2" }}><label htmlFor="adv-sum">Expected checksum (MD5, SHA-1, SHA-256 or SHA-512)</label><input id="adv-sum" className="input mono" value={checksum} onChange={(e) => setChecksum(e.target.value)} placeholder="Paste the checksum from the download page to verify the file" /></div>
          </div>
        </details>

        <div className="actions">
          <button type="button" className="btn ghost" onClick={onClose}>Cancel</button>
          <div style={{ flex: 1 }} />
          <label htmlFor="add-queue" className="sr-only">Queue</label>
          <select id="add-queue" className="select" style={{ width: "auto" }} value={queue} onChange={(e) => setQueue(e.target.value)}>
            {queues.map((q) => <option key={q.id} value={q.id}>{q.name}</option>)}
          </select>
          <button type="button" className="btn" disabled={busy || !url.trim()} onClick={() => submit(false)}>Download later</button>
          <button type="submit" className="btn primary" disabled={busy || !url.trim()}>
            <Icon name="down" size={16} stroke={2.2} />Start download
          </button>
        </div>
      </form>
    </Modal>
  );
}

function typeLabel(name: string, ct: string): string {
  const ext = name.split(".").pop()?.toUpperCase();
  if (ext && ext.length <= 5 && name.includes(".")) return `${ext} file`;
  return ct.split(";")[0];
}
