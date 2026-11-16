// Bridge to the Go side. Inside the ForgeDM app, Wails injects window.go and
// window.runtime. In a normal browser (design preview / development) a mock
// with sample data is used instead.
import type {
  AddOptions, Download, GrabItem, Project, ProbeView, Queue, Rule, Settings, Snapshot, PostActions,
} from "./types";
import { mock } from "./mock";

declare global {
  interface Window {
    go?: any;
    runtime?: any;
  }
}

export const inApp = () => !!window.go?.app?.App;

function call<T>(name: string, ...args: unknown[]): Promise<T> {
  if (inApp()) return window.go.app.App[name](...args) as Promise<T>;
  return mock.call(name, args) as Promise<T>;
}

export const api = {
  snapshot: () => call<Snapshot>("GetSnapshot"),
  probe: (url: string, headers?: Record<string, string>) => call<ProbeView>("Probe", url, headers ?? null),
  add: (o: AddOptions) => call<Download>("Add", o),
  addMany: (list: AddOptions[]) => call<number>("AddMany", list),
  resume: (ids: string[]) => call<void>("Resume", ids),
  pause: (ids: string[]) => call<void>("Pause", ids),
  enqueue: (ids: string[]) => call<void>("Enqueue", ids),
  pauseAll: () => call<void>("PauseAll"),
  resumeAll: () => call<void>("ResumeAll"),
  remove: (ids: string[], deleteFiles: boolean) => call<void>("Remove", ids, deleteFiles),
  redownload: (id: string) => call<void>("Redownload", id),
  refreshLink: (id: string, url: string) => call<void>("RefreshLink", id, url),
  setTags: (id: string, tags: string[]) => call<void>("SetTags", id, tags),
  moveToQueue: (ids: string[], queue: string) => call<void>("MoveToQueue", ids, queue),
  setSpeedLimit: (id: string, bps: number) => call<void>("SetSpeedLimit", id, bps),
  setPost: (id: string, post: PostActions) => call<void>("SetPost", id, post),
  rename: (id: string, name: string) => call<void>("Rename", id, name),
  reorder: (queue: string, ids: string[]) => call<void>("Reorder", queue, ids),
  openFile: (id: string) => call<void>("OpenFile", id),
  showInFolder: (id: string) => call<void>("ShowInFolder", id),
  openFolder: (path: string) => call<void>("OpenFolder", path),
  details: (id: string) => call<Download>("GetDetails", id),
  closeDetails: () => call<void>("CloseDetails"),
  saveSettings: (s: Settings) => call<void>("SaveSettings", s),
  setSpeedLimiter: (on: boolean) => call<void>("SetSpeedLimiter", on),
  setDropTarget: (on: boolean) => call<void>("SetDropTarget", on),
  saveRules: (r: Rule[]) => call<void>("SaveRules", r),
  saveQueue: (q: Queue) => call<Queue>("SaveQueue", q),
  deleteQueue: (id: string) => call<void>("DeleteQueue", id),
  runQueue: (id: string) => call<void>("RunQueue", id),
  stopQueue: (id: string) => call<void>("StopQueue", id),
  chooseFolder: (start: string) => call<string>("ChooseFolder", start),
  importList: () => call<string[] | null>("ImportList"),
  exportSettings: () => call<string>("ExportSettings"),
  importSettings: () => call<void>("ImportSettings"),
  answerPairing: (id: string, allow: boolean) => call<void>("AnswerPairing", id, allow),
  removeBrowser: (id: string) => call<void>("RemoveBrowser", id),
  answerCapture: (id: string, o: AddOptions, action: "start" | "later" | "cancel") => call<void>("AnswerCapture", id, o, action),
  answerBatch: (id: string, urls: string[], dir: string, queue: string, start: boolean) => call<number>("AnswerBatch", id, urls, dir, queue, start),
  dismissClipboard: (url: string) => call<void>("DismissClipboard", url),
  extensionFolder: () => call<string>("ExtensionFolder"),
  startScan: (req: { url: string; depth: number; sameSite: boolean }) => call<string>("StartScan", req),
  cancelScan: (id: string) => call<void>("CancelScan", id),
  downloadGrabbed: (items: GrabItem[], dir: string, keep: boolean, queue: string, start: boolean) => call<number>("DownloadGrabbed", items, dir, keep, queue, start),
  saveProject: (p: Project) => call<Project>("SaveProject", p),
  deleteProject: (id: string) => call<void>("DeleteProject", id),
  closeWindow: () => call<void>("CloseWindow"),
  quit: () => call<void>("Quit"),
  markOnboarded: () => call<void>("MarkOnboarded"),
};

/** Subscribe to an event from the Go side. Returns an unsubscribe function. */
export function on<T = any>(event: string, cb: (data: T) => void): () => void {
  if (window.runtime?.EventsOn) {
    const off = window.runtime.EventsOn(event, cb);
    return typeof off === "function" ? off : () => window.runtime.EventsOff(event);
  }
  return mock.on(event, cb);
}

export const win = {
  minimise: () => window.runtime?.WindowMinimise?.(),
  toggleMaximise: () => window.runtime?.WindowToggleMaximise?.(),
  openURL: (u: string) => (window.runtime?.BrowserOpenURL ? window.runtime.BrowserOpenURL(u) : window.open(u, "_blank")),
  copy: async (text: string) => {
    if (window.runtime?.ClipboardSetText) return window.runtime.ClipboardSetText(text);
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      /* ignore */
    }
  },
};
