import type { CSSProperties } from "react";

export const paths = {
  dash: "M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h6v6h-6z",
  down: "M12 4v11M7 10l5 5 5-5M5 20h14",
  cal: "M4 6h16v14H4zM4 10h16M9 3v4M15 3v4",
  globe: "M12 3a9 9 0 1 0 0 18a9 9 0 1 0 0-18zM3 12h18M12 3c3 3.5 3 14.5 0 18M12 3c-3 3.5-3 14.5 0 18",
  sliders: "M4 7h10M18 7h2M4 17h4M12 17h8M14 4v6M8 14v6",
  video: "M3 5h18v14H3zM10 9l5 3-5 3z",
  music: "M9 18V5l11-2v13M9 18a3 3 0 1 1-6 0a3 3 0 1 1 6 0zM20 16a3 3 0 1 1-6 0a3 3 0 1 1 6 0z",
  doc: "M6 3h8l5 5v13H6zM14 3v5h5M9 13h7M9 17h7",
  zip: "M4 4h16v4H4zM5 8v12h14V8M10 12h4",
  app: "M12 3l8 4.5v9L12 21l-8-4.5v-9zM4 7.5l8 4.5 8-4.5M12 12v9",
  img: "M4 4h16v16H4zM4 16l5-5 4 4 3-3 4 4",
  file: "M6 3h8l5 5v13H6zM14 3v5h5",
  plus: "M12 5v14M5 12h14",
  pause: "M8 5v14M16 5v14",
  play: "M7 4l12 8-12 8z",
  stop: "M6 6h12v12H6z",
  trash: "M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13",
  search: "M11 4a7 7 0 1 0 0 14a7 7 0 1 0 0-14zM20 20l-4-4",
  folder: "M3 6h6l2 2h10v11H3z",
  bolt: "M13 3L5 14h6l-1 7 8-11h-6z",
  link: "M10 14a4 4 0 0 0 5.66 0l3-3a4 4 0 0 0-5.66-5.66l-1 1M14 10a4 4 0 0 0-5.66 0l-3 3a4 4 0 0 0 5.66 5.66l1-1",
  check: "M5 12l5 5 9-10",
  x: "M6 6l12 12M18 6L6 18",
  min: "M5 12h14",
  max: "M6 6h12v12H6z",
  chevDown: "M6 9l6 6 6-6",
  chevRight: "M9 6l6 6-6 6",
  bell: "M6 16v-5a6 6 0 0 1 12 0v5l2 2H4zM10 21h4",
  shield: "M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6zM8.5 12l2.5 2.5 4.5-5",
  clock: "M12 3a9 9 0 1 0 0 18a9 9 0 1 0 0-18zM12 7v5l3 2",
  refresh: "M20 11a8 8 0 0 0-14.9-3M4 4v4h4M4 13a8 8 0 0 0 14.9 3M20 20v-4h-4",
  more: "M5 12h.01M12 12h.01M19 12h.01",
  moon: "M20 14.5A8 8 0 1 1 9.5 4a6.5 6.5 0 0 0 10.5 10.5z",
  box: "M4 4h16v4H4zM5 8v12h14V8M10 12h4",
  gauge: "M4 18a8 8 0 1 1 16 0M12 18l4-6",
  external: "M14 4h6v6M20 4l-9 9M18 14v6H4V6h6",
  grip: "M9 6h.01M15 6h.01M9 12h.01M15 12h.01M9 18h.01M15 18h.01",
  queue: "M8 6h12M8 12h12M8 18h12M4 6h.01M4 12h.01M4 18h.01",
  arrow: "M5 12h14M13 6l6 6-6 6",
  warn: "M12 3l9 16H3zM12 10v4M12 17h.01",
  copy: "M9 9h11v11H9zM5 15H4V4h11v1",
  tag: "M3 12V4h8l10 10-8 8zM7.5 8h.01",
  edit: "M4 20h4L19 9l-4-4L4 16zM14 6l4 4",
  puzzle: "M4 7h4a2 2 0 1 1 4 0h4v4a2 2 0 1 1 0 4v4h-4a2 2 0 1 0-4 0H4v-4a2 2 0 1 0 0-4z",
  upload: "M12 20V9M7 14l5-5 5 5M5 4h14",
  power: "M12 3v9M6.3 7.5a8 8 0 1 0 11.4 0",
  sync: "M20 11a8 8 0 0 0-14.9-3M4 4v4h4M4 13a8 8 0 0 0 14.9 3M20 20v-4h-4",
  info: "M12 3a9 9 0 1 0 0 18a9 9 0 1 0 0-18zM12 11v6M12 7.5h.01",
  hash: "M5 9h14M5 15h14M10 4l-2 16M16 4l-2 16",
};

export type IconName = keyof typeof paths;

export function Icon({ name, size = 18, stroke = 1.8, style, className }: { name: IconName; size?: number; stroke?: number; style?: CSSProperties; className?: string }) {
  const sw = name === "more" || name === "grip" ? 3 : stroke;
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={sw} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" style={style} className={className}>
      <path d={paths[name]} />
    </svg>
  );
}

export function Logo({ size = 24 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true">
      <rect x="1" y="1" width="22" height="22" rx="7" fill="var(--accent)" />
      <path d="M12 6v9M8 11.5l4 4 4-4M7.5 18.5h9" fill="none" stroke="var(--accent-ink)" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export const categoryStyle: Record<string, { icon: IconName; color: string }> = {
  Video: { icon: "video", color: "#FF9F43" },
  Music: { icon: "music", color: "#FF6B9A" },
  Documents: { icon: "doc", color: "#5B8CFF" },
  Compressed: { icon: "zip", color: "#F7C948" },
  Programs: { icon: "app", color: "#2EE6D6" },
  Images: { icon: "img", color: "#A78BFA" },
  Other: { icon: "file", color: "#8A94AD" },
};

export function catStyle(c: string) {
  return categoryStyle[c] ?? categoryStyle.Other;
}

export function FileIcon({ category, size = 40, radius = 11 }: { category: string; size?: number; radius?: number }) {
  const s = catStyle(category);
  return (
    <span className="icon-tile" style={{ width: size, height: size, borderRadius: radius, background: `color-mix(in srgb, ${s.color} 14%, transparent)`, color: s.color }}>
      <Icon name={s.icon} size={Math.round(size / 2)} />
    </span>
  );
}
