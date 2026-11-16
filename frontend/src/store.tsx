import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { api, on } from "./api";
import type { Live, Snapshot, Tick } from "./types";

export type Page = "dashboard" | "downloads" | "scheduler" | "grabber" | "settings";

export interface Toast {
  id: number;
  kind: "info" | "success" | "error";
  text: string;
}

export interface DownloadsFilter {
  category?: string;
  queue?: string;
  tag?: string;
}

interface StoreValue {
  snap: Snapshot | null;
  live: Record<string, Live>;
  tick: Tick | null;
  page: Page;
  go: (p: Page, f?: DownloadsFilter) => void;
  filter: DownloadsFilter;
  setFilter: (f: DownloadsFilter) => void;
  refresh: () => Promise<void>;
  toasts: Toast[];
  toast: (kind: Toast["kind"], text: string) => void;
  dialog: Dialog | null;
  open: (d: Dialog) => void;
  close: () => void;
}

export type Dialog =
  | { kind: "add"; url?: string }
  | { kind: "details"; id: string }
  | { kind: "refresh"; id: string }
  | { kind: "remove"; ids: string[] }
  | { kind: "import" }
  | { kind: "queue"; id?: string }
  | { kind: "rule"; id?: string }
  | { kind: "connect" };

const Ctx = createContext<StoreValue | null>(null);

export function StoreProvider({ children }: { children: ReactNode }) {
  const [snap, setSnap] = useState<Snapshot | null>(null);
  const [tick, setTick] = useState<Tick | null>(null);
  const [page, setPage] = useState<Page>("dashboard");
  const [filter, setFilter] = useState<DownloadsFilter>({});
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const toastId = useRef(1);

  const refresh = useCallback(async () => {
    const s = await api.snapshot();
    if (s) setSnap(s);
  }, []);

  const toast = useCallback((kind: Toast["kind"], text: string) => {
    const id = toastId.current++;
    setToasts((t) => [...t.slice(-3), { id, kind, text }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), kind === "error" ? 7000 : 4000);
  }, []);

  useEffect(() => {
    refresh();
    const offs = [
      on<Snapshot>("state", (s) => setSnap(s)),
      on<Tick>("tick", (t) => setTick(t)),
      on<{ kind: Toast["kind"]; text: string }>("toast", (t) => toast(t.kind, t.text)),
    ];
    return () => offs.forEach((f) => f());
  }, [refresh, toast]);

  const live = useMemo(() => {
    const m: Record<string, Live> = {};
    for (const l of tick?.live ?? []) m[l.id] = l;
    return m;
  }, [tick]);

  const go = useCallback((p: Page, f?: DownloadsFilter) => {
    setPage(p);
    if (f) setFilter(f);
    else if (p !== "downloads") setFilter({});
  }, []);

  const value: StoreValue = {
    snap, live, tick, page, go, filter, setFilter, refresh, toasts, toast, dialog,
    open: setDialog, close: () => setDialog(null),
  };
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useStore(): StoreValue {
  const v = useContext(Ctx);
  if (!v) throw new Error("useStore outside provider");
  return v;
}

/** Runs an API call and shows a toast if it fails. */
export function useAction() {
  const { toast } = useStore();
  return useCallback(
    async <T,>(p: Promise<T>, ok?: string): Promise<T | undefined> => {
      try {
        const r = await p;
        if (ok) toast("success", ok);
        return r;
      } catch (e: any) {
        toast("error", typeof e === "string" ? e : e?.message || "Something went wrong");
        return undefined;
      }
    },
    [toast],
  );
}
