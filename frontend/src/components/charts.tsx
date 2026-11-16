import { useId } from "react";
import { fmtBytes } from "../format";

const MB = 1 << 20;

/** Rounds a speed up to a tidy axis maximum. */
export function niceMax(v: number): number {
  const mb = Math.max(v / MB, 1);
  const steps = [1, 2, 5, 10, 20, 30, 40, 50, 60, 80, 100, 150, 200, 300, 500, 1000, 2000, 5000];
  for (const s of steps) if (mb <= s * 0.92) return s * MB;
  return Math.ceil(mb / 1000) * 1000 * MB;
}

function toPath(values: number[], w: number, h: number, max: number): string {
  if (values.length === 0) return "";
  const n = values.length;
  return values
    .map((v, i) => {
      const x = n === 1 ? w : (i / (n - 1)) * w;
      const y = h - Math.min(1, v / max) * h;
      return `${i === 0 ? "M" : "L"}${x.toFixed(1)} ${y.toFixed(1)}`;
    })
    .join(" ");
}

export function AreaChart({ values, height = 200, label, showAxis = true, color = "var(--accent)", current }: { values: number[]; height?: number; label: string; showAxis?: boolean; color?: string; current?: number }) {
  const id = useId().replace(/:/g, "");
  const data = values.length > 1 ? values : [0, ...(values.length ? values : [0])];
  const max = niceMax(Math.max(...data, 1));
  const W = 600;
  const line = toPath(data, W, height, max);
  const area = `${line} L${W} ${height} L0 ${height} Z`;
  const last = data[data.length - 1];
  const lastY = height - Math.min(1, last / max) * height;
  const ticks = [1, 2 / 3, 1 / 3, 0];
  return (
    <div style={{ display: "flex", gap: 10 }}>
      {showAxis && (
        <div aria-hidden="true" className="mono faint" style={{ position: "relative", width: 34, height, fontSize: 11 }}>
          {ticks.map((t) => (
            <span key={t} style={{ position: "absolute", right: 0, top: Math.max(0, (1 - t) * height - 7) }}>
              {Math.round((max * t) / MB)}
            </span>
          ))}
        </div>
      )}
      <div style={{ position: "relative", flex: 1, minWidth: 0, height }}>
        <svg viewBox={`0 0 ${W} ${height}`} preserveAspectRatio="none" role="img" aria-label={label} style={{ position: "absolute", inset: 0, width: "100%", height }}>
          <defs>
            <linearGradient id={`g${id}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0" stopColor={color} stopOpacity="0.32" />
              <stop offset="1" stopColor={color} stopOpacity="0" />
            </linearGradient>
          </defs>
          {ticks.slice(0, 3).map((t) => (
            <path key={t} d={`M0 ${((1 - t) * height).toFixed(1)}H${W}`} stroke="var(--grid)" strokeWidth="1" strokeDasharray="4 4" vectorEffect="non-scaling-stroke" />
          ))}
          <path d={`M0 ${height - 0.5}H${W}`} stroke="var(--border2)" strokeWidth="1" vectorEffect="non-scaling-stroke" />
          <path d={area} fill={`url(#g${id})`} />
          <path d={line} fill="none" stroke={color} strokeWidth="2.2" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
        </svg>
        {current !== undefined && current > 0 && (
          <>
            <span aria-hidden="true" style={{ position: "absolute", right: -6, top: lastY - 6, width: 12, height: 12, borderRadius: "50%", background: color, border: "3px solid var(--card)" }} />
            <span className="mono" style={{ position: "absolute", right: 10, top: Math.max(0, lastY - 36), padding: "4px 8px", borderRadius: 7, background: "var(--raised)", fontSize: 12, fontWeight: 600 }}>
              {fmtBytes(current)}/s
            </span>
          </>
        )}
      </div>
    </div>
  );
}

export function Sparkline({ values, height = 34, color = "var(--accent)" }: { values: number[]; height?: number; color?: string }) {
  const data = values.length > 1 ? values : [0, 0];
  const min = Math.min(...data);
  const max = Math.max(...data);
  const W = 120;
  const n = data.length;
  const d = data
    .map((v, i) => {
      const x = (i / (n - 1)) * W;
      const y = max === min ? height / 2 : height - 2 - ((v - min) / (max - min)) * (height - 4);
      return `${i === 0 ? "M" : "L"}${x.toFixed(1)} ${y.toFixed(1)}`;
    })
    .join(" ");
  return (
    <svg viewBox={`0 0 ${W} ${height}`} preserveAspectRatio="none" aria-hidden="true" style={{ width: "100%", height }}>
      <path d={d} fill="none" stroke={color} strokeWidth="2" vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
    </svg>
  );
}

export interface Slice {
  label: string;
  value: number;
  color: string;
}

export function Donut({ slices, size = 140, thickness = 16, center, sub, label }: { slices: Slice[]; size?: number; thickness?: number; center: string; sub: string; label: string }) {
  const r = (size - thickness) / 2;
  const C = 2 * Math.PI * r;
  const total = slices.reduce((a, s) => a + s.value, 0);
  let off = 0;
  return (
    <div style={{ position: "relative", width: size, height: size, flex: "none" }}>
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} role="img" aria-label={label} style={{ transform: "rotate(-90deg)" }}>
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--track)" strokeWidth={thickness} />
        {total > 0 &&
          slices.map((s) => {
            const len = (s.value / total) * C;
            const gap = len > 4 ? 2 : 0;
            const el = <circle key={s.label} cx={size / 2} cy={size / 2} r={r} fill="none" stroke={s.color} strokeWidth={thickness} strokeDasharray={`${Math.max(0, len - gap)} ${C - len + gap}`} strokeDashoffset={-off} />;
            off += len;
            return el;
          })}
      </svg>
      <div style={{ position: "absolute", inset: 0, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center" }}>
        <span className="display" style={{ fontSize: 22, fontWeight: 600 }}>{center}</span>
        <span className="muted" style={{ fontSize: 12 }}>{sub}</span>
      </div>
    </div>
  );
}

export function Ring({ value, size = 136, thickness = 12, children }: { value: number; size?: number; thickness?: number; children?: React.ReactNode }) {
  const r = (size - thickness) / 2;
  const C = 2 * Math.PI * r;
  const v = Math.max(0, Math.min(1, value));
  return (
    <div style={{ position: "relative", width: size, height: size, flex: "none" }}>
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} role="img" aria-label={`${Math.round(v * 100)}% done`} style={{ transform: "rotate(-90deg)" }}>
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--track)" strokeWidth={thickness} />
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--accent)" strokeWidth={thickness} strokeLinecap="round" strokeDasharray={`${v * C} ${C}`} style={{ transition: "stroke-dasharray 0.5s linear" }} />
      </svg>
      <div style={{ position: "absolute", inset: 0, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center" }}>{children}</div>
    </div>
  );
}

/** Small bar made of N cells, each showing how far one part of the file is. */
export function SegBar({ map, color = "var(--accent)", cells = 8, done }: { map?: number[]; color?: string; cells?: number; done?: number }) {
  let parts: number[];
  if (map && map.length) {
    parts = [];
    const per = map.length / cells;
    for (let i = 0; i < cells; i++) {
      const slice = map.slice(Math.floor(i * per), Math.floor((i + 1) * per));
      parts.push(slice.length ? slice.reduce((a, b) => a + b, 0) / slice.length : 0);
    }
  } else {
    parts = Array(cells).fill(done ?? 0);
  }
  return (
    <div className="segbar" aria-hidden="true">
      {parts.map((p, i) => (
        <span key={i}>
          <span style={{ width: `${Math.round(p * 100)}%`, background: color }} />
        </span>
      ))}
    </div>
  );
}

export function WeekBars({ days, max }: { days: { label: string; value: number; today?: boolean }[]; max: number }) {
  const m = Math.max(max, 1);
  return (
    <>
      <div role="img" aria-label={`Data downloaded per day: ${days.map((d) => `${d.label} ${fmtBytes(d.value)}`).join(", ")}`} style={{ display: "flex", alignItems: "flex-end", gap: 10, height: 170, paddingTop: 10, borderBottom: "1px solid var(--border2)" }}>
        {days.map((d) => (
          <div key={d.label} style={{ flex: 1, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "flex-end", gap: 6, height: 160 }}>
            <span className="mono muted" style={{ fontSize: 11 }}>{d.value > 0 ? fmtBytes(d.value).replace(" ", "") : ""}</span>
            <span style={{ width: "100%", maxWidth: 34, borderRadius: "7px 7px 3px 3px", height: Math.max(3, Math.round((d.value / m) * 128)), background: d.today ? "var(--accent)" : "var(--border2)", transition: "height 0.4s" }} />
          </div>
        ))}
      </div>
      <div aria-hidden="true" style={{ display: "flex", gap: 10, marginTop: -8 }}>
        {days.map((d) => (
          <span key={d.label} style={{ flex: 1, textAlign: "center", fontSize: 12, color: d.today ? "var(--accent-text)" : "var(--faint)" }}>{d.label}</span>
        ))}
      </div>
    </>
  );
}
