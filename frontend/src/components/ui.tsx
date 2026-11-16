import { useEffect, useRef, useState, type ReactNode } from "react";
import { Icon, type IconName } from "../icons";
import { useStore } from "../store";

export function Switch({ on, onChange, label, disabled }: { on: boolean; onChange: (v: boolean) => void; label: string; disabled?: boolean }) {
  return (
    <button type="button" role="switch" aria-checked={on} aria-label={label} className="switch" disabled={disabled} onClick={() => onChange(!on)}>
      <span />
    </button>
  );
}

export function Seg<T extends string | number>({ value, options, onChange, label, accent }: { value: T; options: { value: T; label: string }[]; onChange: (v: T) => void; label: string; accent?: boolean }) {
  return (
    <div role="group" aria-label={label} className={`seg${accent ? " accent" : ""}`}>
      {options.map((o) => (
        <button key={String(o.value)} type="button" aria-pressed={o.value === value} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function ToggleRow({ label, hint, on, onChange }: { label: string; hint?: string; on: boolean; onChange: (v: boolean) => void }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
      <span style={{ flex: 1 }}>
        <span style={{ display: "block", fontSize: 13, fontWeight: 500 }}>{label}</span>
        {hint && <span className="hint" style={{ display: "block" }}>{hint}</span>}
      </span>
      <Switch on={on} onChange={onChange} label={label} />
    </div>
  );
}

export function Modal({ title, onClose, children, width, labelledBy }: { title?: string; onClose: () => void; children: ReactNode; width?: number; labelledBy?: string }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null;
    const first = ref.current?.querySelector<HTMLElement>("input, select, textarea, button:not([data-skipfocus])");
    first?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      prev?.focus?.();
    };
  }, [onClose]);
  return (
    <div className="backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div ref={ref} className="modal" role="dialog" aria-modal="true" aria-label={title} aria-labelledby={labelledBy} style={width ? { width: `min(${width}px, calc(100vw - 48px))` } : undefined}>
        {children}
      </div>
    </div>
  );
}

export interface MenuItem {
  label: string;
  icon?: IconName;
  onClick: () => void;
  danger?: boolean;
  sep?: boolean;
  disabled?: boolean;
}

export function useMenu() {
  const [menu, setMenu] = useState<{ x: number; y: number; items: MenuItem[] } | null>(null);
  useEffect(() => {
    if (!menu) return;
    const close = () => setMenu(null);
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close();
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", onKey);
    window.addEventListener("blur", close);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("blur", close);
    };
  }, [menu]);
  const open = (x: number, y: number, items: MenuItem[]) => {
    const h = items.length * 36 + 16;
    setMenu({ x: Math.min(x, window.innerWidth - 230), y: Math.min(y, window.innerHeight - h - 8), items });
  };
  const node = menu ? (
    <div className="menu" role="menu" style={{ left: menu.x, top: menu.y }} onMouseDown={(e) => e.stopPropagation()}>
      {menu.items.map((it, i) =>
        it.sep ? (
          <hr key={i} />
        ) : (
          <button
            key={i}
            role="menuitem"
            className={it.danger ? "danger" : ""}
            disabled={it.disabled}
            onClick={() => {
              setMenu(null);
              it.onClick();
            }}
          >
            {it.icon && <Icon name={it.icon} size={15} />}
            {it.label}
          </button>
        ),
      )}
    </div>
  ) : null;
  return { open, node };
}

export function Toasts() {
  const { toasts } = useStore();
  return (
    <div className="toasts" role="status" aria-live="polite">
      {toasts.map((t) => (
        <div key={t.id} className="toast">
          <span style={{ color: t.kind === "error" ? "var(--bad-text)" : t.kind === "success" ? "var(--ok-text)" : "var(--accent-text)", marginTop: 1 }}>
            <Icon name={t.kind === "error" ? "warn" : t.kind === "success" ? "check" : "info"} size={16} stroke={2.2} />
          </span>
          <span style={{ flex: 1 }}>{t.text}</span>
        </div>
      ))}
    </div>
  );
}

export function Empty({ icon, title, children }: { icon: IconName; title: string; children?: ReactNode }) {
  return (
    <div className="empty">
      <span className="icon-tile" style={{ width: 52, height: 52, borderRadius: 14, background: "var(--accent-soft)", color: "var(--accent-text)" }}>
        <Icon name={icon} size={24} />
      </span>
      <div className="big">{title}</div>
      {children}
    </div>
  );
}

export function StatTile({ label, icon, color, value, unit, children }: { label: string; icon: IconName; color: string; value: string; unit: string; children?: ReactNode }) {
  return (
    <div className="card" style={{ padding: 18, borderRadius: 16, display: "flex", flexDirection: "column", gap: 10 }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <span className="muted" style={{ fontSize: 13 }}>{label}</span>
        <span className="icon-tile" style={{ width: 28, height: 28, borderRadius: 8, background: `color-mix(in srgb, ${color} 15%, transparent)`, color }}>
          <Icon name={icon} size={16} />
        </span>
      </div>
      <div style={{ display: "flex", alignItems: "baseline", gap: 6, minWidth: 0 }}>
        <span className="display" style={{ fontSize: 32, fontWeight: 600, letterSpacing: "-0.02em" }}>{value}</span>
        <span className="muted ellipsis">{unit}</span>
      </div>
      <div style={{ height: 34, display: "flex", alignItems: "center" }}>{children}</div>
    </div>
  );
}
