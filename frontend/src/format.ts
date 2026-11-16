export function fmtBytes(n: number | undefined, digits = 1): string {
  if (n === undefined || n === null || n < 0 || !isFinite(n)) return "—";
  if (n < 1024) return `${Math.round(n)} B`;
  const units = ["KB", "MB", "GB", "TB", "PB"];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  const d = v >= 100 ? 0 : v >= 10 ? Math.min(digits, 1) : digits + (v < 10 && i >= 2 ? 1 : 0);
  return `${v.toFixed(d)} ${units[i]}`;
}

export function fmtSpeed(bps: number | undefined): string {
  if (!bps || bps <= 0) return "—";
  return `${fmtBytes(bps)}/s`;
}

/** Splits a value like 48.6 MB/s into number and unit for big headline numbers. */
export function splitSpeed(bps: number): [string, string] {
  if (!bps || bps <= 0) return ["0", "MB/s"];
  const s = fmtBytes(bps);
  const [n, u] = s.split(" ");
  return [n, `${u}/s`];
}

export function splitBytes(b: number): [string, string] {
  const s = fmtBytes(b);
  const [n, u] = s.split(" ");
  return [n, u ?? ""];
}

export function fmtDuration(sec: number | undefined): string {
  if (sec === undefined || !isFinite(sec) || sec <= 0) return "—";
  sec = Math.round(sec);
  if (sec < 60) return `${sec}s`;
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  if (m < 60) return `${m}m ${String(s).padStart(2, "0")}s`;
  const h = Math.floor(m / 60);
  if (h < 48) return `${h}h ${String(m % 60).padStart(2, "0")}m`;
  return `${Math.floor(h / 24)}d ${h % 24}h`;
}

export function fmtMs(ms: number): string {
  const m = Math.round(ms / 60000);
  if (m < 1) return `${Math.round(ms / 1000)}s`;
  if (m < 60) return `${m}m`;
  return `${Math.floor(m / 60)}h ${String(m % 60).padStart(2, "0")}m`;
}

export function pct(done: number, size: number): number {
  if (!size || size <= 0) return 0;
  return Math.max(0, Math.min(100, Math.floor((done / size) * 100)));
}

export function fmtWhen(isoDate?: string): string {
  if (!isoDate || isoDate.startsWith("0001")) return "—";
  const d = new Date(isoDate);
  const now = new Date();
  const time = d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  const sameDay = d.toDateString() === now.toDateString();
  if (sameDay) return `Today ${time}`;
  const y = new Date(now);
  y.setDate(now.getDate() - 1);
  if (d.toDateString() === y.toDateString()) return `Yesterday ${time}`;
  const diff = (now.getTime() - d.getTime()) / 86400000;
  if (diff < 6 && diff > 0) return `${d.toLocaleDateString([], { weekday: "short" })} ${time}`;
  return d.toLocaleDateString([], { day: "numeric", month: "short", year: d.getFullYear() === now.getFullYear() ? undefined : "numeric" });
}

export function fmtNext(isoDate?: string): string {
  if (!isoDate || isoDate.startsWith("0001")) return "Not scheduled";
  const d = new Date(isoDate);
  const now = new Date();
  const time = d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  const t = new Date(now);
  t.setDate(now.getDate() + 1);
  if (d.toDateString() === now.toDateString()) return `Today at ${time}`;
  if (d.toDateString() === t.toDateString()) return d.getHours() < 6 ? `Tonight at ${time}` : `Tomorrow at ${time}`;
  return `${d.toLocaleDateString([], { weekday: "long" })} at ${time}`;
}

export function host(u: string): string {
  try {
    return new URL(u).hostname.replace(/^www\./, "");
  } catch {
    return "";
  }
}

export const MB = 1 << 20;
export const GB = 1 << 30;

/** Parses "10 MB", "500k", "2.5 GB/s" into bytes. Returns NaN if unreadable. */
export function parseSize(s: string): number {
  const m = s.trim().toLowerCase().replace(/\/s$/, "").match(/^([\d.]+)\s*(b|k|kb|m|mb|g|gb|t|tb)?$/);
  if (!m) return NaN;
  const n = parseFloat(m[1]);
  const mult: Record<string, number> = { b: 1, k: 1024, kb: 1024, m: MB, mb: MB, g: GB, gb: GB, t: GB * 1024, tb: GB * 1024 };
  return Math.round(n * (mult[m[2] || "mb"] ?? 1));
}
