// Types mirroring the Go structs sent over the Wails bridge.

export type Status = "queued" | "downloading" | "paused" | "completed" | "failed";
export type Kind = "file" | "hls";

export interface PostActions {
  verify: boolean;
  scan: boolean;
  extract: boolean;
  open: boolean;
}

export interface Checksum {
  algo?: string;
  expected?: string;
  actual?: string;
  verified?: boolean;
  mismatch?: boolean;
}

export interface Segment {
  start: number;
  end: number;
  done: number;
}

export interface Download {
  id: string;
  url: string;
  finalUrl?: string;
  pageUrl?: string;
  kind: Kind;
  filename: string;
  dir: string;
  size: number;
  downloaded: number;
  resumable: boolean;
  status: Status;
  error?: string;
  errorCode?: string;
  category: string;
  tags?: string[];
  queue: string;
  connections: number;
  speedLimit?: number;
  segments?: Segment[];
  contentType?: string;
  checksum: Checksum;
  post: PostActions;
  postState?: string;
  postNote?: string;
  source?: string;
  rule?: string;
  createdAt: string;
  startedAt?: string;
  completedAt?: string;
  activeMs: number;
  savedMs: number;
  manual?: boolean;
}

export interface Live {
  id: string;
  status: Status;
  downloaded: number;
  size: number;
  speed: number;
  eta: number;
  conns: number;
  map?: number[];
  peak: number;
  history?: number[];
  parts?: { start: number; end: number; done: number; active: boolean }[];
}

export interface Day {
  date: string;
  bytes: number;
  files: number;
  savedMs: number;
}

export interface Stats {
  series: Record<string, number[]>;
  week: Day[];
  today: Day;
  yesterday: Day;
  peak: number;
}

export interface Browser {
  id: string;
  name: string;
  pairedAt: string;
  lastSeen: string;
}

export interface Settings {
  baseDir: string;
  categoryDirs: Record<string, string>;
  maxActive: number;
  defaultConnections: number;
  speedLimitOn: boolean;
  speedLimit: number;
  proxyMode: string;
  proxyUrl: string;
  userAgent: string;
  retryCount: number;
  smartPriority: boolean;
  captureEnabled: boolean;
  captureExts: string[];
  captureMinSize: number;
  askOnCapture: boolean;
  clipboardWatch: boolean;
  videoButton: boolean;
  altBypass: boolean;
  verifyChecksum: boolean;
  scanAfter: boolean;
  openWhenDone: boolean;
  deleteArchive: boolean;
  closeToTray: boolean;
  startWithSystem: boolean;
  keepAwake: boolean;
  notifications: boolean;
  dropTarget: boolean;
  theme: string;
  accent: string;
  mica: boolean;
  density: string;
  apiPort: number;
  tags: string[];
  onboarded: boolean;
}

export interface Condition {
  field: string;
  value: string;
}

export interface Rule {
  id: string;
  name: string;
  enabled: boolean;
  conditions: Condition[];
  actions: {
    dir?: string;
    category?: string;
    tags?: string[];
    queue?: string;
    extract?: boolean;
    open?: boolean;
  };
}

export interface Queue {
  id: string;
  name: string;
  builtin: boolean;
  enabled: boolean;
  start: string;
  stop: string;
  days: number[];
  concurrent: number;
  speedLimit: number;
  halfBandwidth: boolean;
  whenDone: string;
  wakeFromSleep: boolean;
  retryFailed: boolean;
  pauseOnMetered: boolean;
  color: string;
  icon: string;
  order: string[];
  active?: boolean;
  nextRun?: string;
  count?: number;
  bytes?: number;
}

export interface Project {
  id: string;
  name: string;
  url: string;
  depth: number;
  sameSite: boolean;
  types: string[];
  exts: string;
  minSize: number;
  dir: string;
  keepStructure: boolean;
  syncEnabled: boolean;
  syncDay: number;
  syncTime: string;
  lastRun: string;
  lastFound: number;
}

export interface Category {
  name: string;
  count: number;
  bytes: number;
  dir: string;
  exts: string[];
}

export interface Snapshot {
  downloads: Download[];
  settings: Settings;
  rules: Rule[];
  queues: Queue[];
  projects: Project[];
  categories: Category[];
  browsers: Browser[];
  stats: Stats;
  diskFree: number;
  diskTotal: number;
  port: number;
  version: string;
  dataDir: string;
}

export interface Tick {
  live: Live[];
  speed: number;
  active: number;
  stats: Stats;
}

export interface Variant {
  url: string;
  bandwidth: number;
  resolution: string;
  label: string;
}

export interface Decision {
  category: string;
  dir: string;
  tags: string[] | null;
  queue: string;
  extract: boolean;
  open: boolean;
  ruleId: string;
  ruleName: string;
}

export interface ProbeView {
  url: string;
  finalUrl: string;
  filename: string;
  size: number;
  resumable: boolean;
  contentType: string;
  kind: Kind;
  variants?: Variant[];
  decision: Decision;
  duplicate?: Download;
  error?: string;
}

export interface AddOptions {
  url: string;
  pageUrl?: string;
  kind?: string;
  filename?: string;
  dir?: string;
  category?: string;
  tags?: string[];
  queue?: string;
  connections?: number;
  speedLimit?: number;
  post: PostActions;
  checksum?: string;
  start: boolean;
  headers?: Record<string, string>;
  size?: number;
  source?: string;
  rule?: string;
}

export interface Capture {
  url: string;
  filename?: string;
  referrer?: string;
  pageUrl?: string;
  pageTitle?: string;
  mime?: string;
  size?: number;
  kind?: string;
}

export interface PendingCapture {
  id: string;
  capture: Capture;
  browser: string;
  probe?: ProbeView;
  options: AddOptions;
}

export interface GrabItem {
  url: string;
  name: string;
  type: string;
  ext: string;
  size: number;
  page: string;
  path: string;
  have?: boolean;
}

export interface GrabProgress {
  pages: number;
  found: number;
  sized: number;
  stage: string;
  current: string;
}

export interface BatchLink {
  url: string;
  name: string;
  type: string;
}
