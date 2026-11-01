import { create } from "zustand";
import { isCallError } from "mygo-runtime";
import { Downloads, Settings, System, events } from "../mygo";
import type {
  AddRequest,
  BrowserStatus,
  Data as SettingsData,
  Download,
  Finished,
  LaunchResult,
  Preview,
  Progress,
  Stats,
} from "../mygo";
import { strings } from "../lib/strings";
import { nameOf } from "../lib/format";

export type Filter = "all" | "downloading" | "queued" | "paused" | "completed" | "error";
export type SortKey = "createdAt" | "fileName" | "totalBytes" | "status";
export type SettingsSection = "general" | "download" | "category" | "browser" | "about";

export interface Toast {
  id: number;
  kind: "info" | "error" | "success";
  text: string;
}

interface State {
  ready: boolean;
  loadError: string;
  downloads: Record<number, Download>;
  order: number[];
  progress: Record<number, Progress>;
  stats: Stats;
  filter: Filter;
  category: string | null;
  search: string;
  sortKey: SortKey;
  sortDir: "asc" | "desc";
  selection: number[];
  detailId: number | null;
  addOpen: boolean;
  addUrl: string;
  addPreview: Preview | null;
  settingsOpen: boolean;
  settingsSection: SettingsSection;
  settings: SettingsData | null;
  browser: BrowserStatus | null;
  confirmIds: number[] | null;
  toasts: Toast[];
  theme: "dark" | "light";

  init: () => Promise<void>;
  toast: (kind: Toast["kind"], text: string) => void;
  dismissToast: (id: number) => void;
  setFilter: (filter: Filter) => void;
  setCategory: (category: string | null) => void;
  setSearch: (search: string) => void;
  setSort: (key: SortKey) => void;
  toggleSortDir: () => void;
  select: (id: number, mode: "toggle" | "range" | "only") => void;
  clearSelection: () => void;
  selectAll: () => void;
  openDetail: (id: number | null) => void;
  openAdd: (url?: string) => void;
  closeAdd: () => void;
  preview: (req: AddRequest) => Promise<Preview | null>;
  addDownload: (req: AddRequest) => Promise<boolean>;
  pause: (id: number) => Promise<void>;
  resume: (id: number) => Promise<void>;
  retry: (id: number) => Promise<void>;
  remove: (ids: number[], deleteFile: boolean) => Promise<void>;
  pauseAll: () => Promise<void>;
  resumeAll: () => Promise<void>;
  reveal: (id: number) => Promise<void>;
  open: (id: number) => Promise<void>;
  copyUrl: (id: number) => Promise<void>;
  askRemove: (ids: number[]) => void;
  cancelRemove: () => void;
  openSettings: (section?: SettingsSection) => void;
  closeSettings: () => void;
  setSettingsSection: (section: SettingsSection) => void;
  saveSettings: (patch: Partial<SettingsData>) => Promise<void>;
  refreshBrowser: () => Promise<void>;
  setBrowserEnabled: (enabled: boolean) => Promise<void>;
  installBrowserHost: () => Promise<BrowserStatus | null>;
  launchBrowserExtension: () => Promise<LaunchResult | null>;
  openExtensionPage: () => Promise<string[] | null>;
  setTheme: (theme: "dark" | "light") => void;
  pasteLink: () => Promise<void>;
  dismissFinished: (f: Finished) => void;
}

// Progress arrives four times a second for every running download. It lands
// in a module-level map and one animation frame later in the store, so the
// table re-renders once per frame at most.
const pendingProgress = new Map<number, Progress>();
let frame = 0;

function flushProgress() {
  frame = 0;
  if (pendingProgress.size === 0) return;
  const next: Record<number, Progress> = {};
  for (const [id, progress] of pendingProgress) next[id] = progress;
  pendingProgress.clear();
  useApp.setState((state) => {
    const downloads = { ...state.downloads };
    for (const [id, progress] of Object.entries(next)) {
      const download = downloads[Number(id)];
      if (!download) continue;
      downloads[Number(id)] = {
        ...download,
        downloaded: progress.downloaded,
        speed: progress.speed,
        eta: progress.eta,
      };
    }
    return { progress: { ...state.progress, ...next }, downloads };
  });
}

function queueProgress(progress: Progress) {
  pendingProgress.set(progress.id, progress);
  if (!frame) frame = requestAnimationFrame(flushProgress);
}

let toastId = 0;

/** launchToast turns a launch result into the message the user needs: what
 * loaded, what needs a restart, and what must be loaded by hand. */
function launchToast(launch: LaunchResult): { kind: "success" | "info"; text: string } | null {
  const parts: string[] = [];
  if (launch.started.length > 0) parts.push(`${strings.extensionLoaded}：${launch.started.join("、")}`);
  if (launch.running.length > 0) parts.push(`${strings.extensionRestart}：${launch.running.join("、")}`);
  if (launch.manual.length > 0) parts.push(`${strings.extensionManual}：${launch.manual.join("、")}`);
  if (parts.length === 0) return null;
  return { kind: launch.started.length > 0 ? "success" : "info", text: parts.join("；") };
}

function errorMessage(err: unknown, fallback: string): string {
  // A Go error carries a sentence written for the user; anything else is a
  // programming error and only the fallback belongs on screen.
  if (isCallError(err)) return err.message || fallback;
  return fallback;
}

export const useApp = create<State>((set, get) => ({
  ready: false,
  loadError: "",
  downloads: {},
  order: [],
  progress: {},
  stats: { active: 0, queued: 0, speed: 0 },
  filter: "all",
  category: null,
  search: "",
  sortKey: "createdAt",
  sortDir: "desc",
  selection: [],
  detailId: null,
  addOpen: false,
  addUrl: "",
  addPreview: null,
  settingsOpen: false,
  settingsSection: "general",
  settings: null,
  browser: null,
  confirmIds: null,
  toasts: [],
  theme: "dark",

  async init() {
    try {
      const list = await Downloads.list();
      const downloads: Record<number, Download> = {};
      for (const d of list) downloads[d.id] = d;
      set({ downloads, order: list.map((d) => d.id), ready: true, loadError: "" });
    } catch (err) {
      set({ ready: true, loadError: errorMessage(err, strings.loadFailed) });
    }
    try {
      set({ settings: await Settings.get(), theme: (await System.darkMode()) ? "dark" : "light" });
    } catch {
      // Not in a MyGo window (the dev page in a browser): keep the media
      // query's answer.
    }
    events.downloadsUpdated.on((d) => {
      set((state) => {
        const downloads = { ...state.downloads, [d.id]: { ...d, speed: state.downloads[d.id]?.speed ?? 0 } };
        const order = state.order.includes(d.id) ? state.order : [d.id, ...state.order];
        return { downloads, order };
      });
    });
    events.downloadsProgress.on(queueProgress);
    events.downloadsFinished.on((f) => get().dismissFinished(f));
    events.statsUpdated.on((stats) => set({ stats }));
    events.uiTheme.on((theme) => {
      const value = theme === "dark" ? "dark" : "light";
      set({ theme: value });
    });
    events.uiAdd.on((req) => get().openAdd(req.url));
  },

  toast(kind, text) {
    const toast: Toast = { id: ++toastId, kind, text };
    set((state) => ({ toasts: [...state.toasts, toast] }));
    setTimeout(() => get().dismissToast(toast.id), 4000);
  },

  dismissToast(id) {
    set((state) => ({ toasts: state.toasts.filter((t) => t.id !== id) }));
  },

  setFilter(filter) {
    set({ filter, category: null });
  },
  setCategory(category) {
    set({ category, filter: "all" });
  },
  setSearch(search) {
    set({ search });
  },
  setSort(sortKey) {
    set((state) => (state.sortKey === sortKey ? { sortDir: state.sortDir === "asc" ? "desc" : "asc" } : { sortKey }));
  },
  toggleSortDir() {
    set((state) => ({ sortDir: state.sortDir === "asc" ? "desc" : "asc" }));
  },
  select(id, mode) {
    set((state) => {
      if (mode === "only") return { selection: [id] };
      if (mode === "range") {
        const anchor = state.selection[state.selection.length - 1];
        if (anchor === undefined) return { selection: [id] };
        const from = state.order.indexOf(anchor);
        const to = state.order.indexOf(id);
        if (from < 0 || to < 0) return { selection: [id] };
        const [start, end] = from < to ? [from, to] : [to, from];
        return { selection: state.order.slice(start, end + 1) };
      }
      return state.selection.includes(id)
        ? { selection: state.selection.filter((s) => s !== id) }
        : { selection: [...state.selection, id] };
    });
  },
  clearSelection() {
    set({ selection: [] });
  },
  selectAll() {
    set((state) => ({ selection: state.order }));
  },
  openDetail(id) {
    set({ detailId: id });
  },
  openAdd(url) {
    set({ addOpen: true, addUrl: url ?? "", addPreview: null });
  },
  closeAdd() {
    set({ addOpen: false, addUrl: "", addPreview: null });
  },

  async preview(req) {
    try {
      const preview = await Downloads.preview(req);
      set({ addPreview: preview });
      return preview;
    } catch (err) {
      set({ addPreview: null });
      const text = errorMessage(err, strings.loadFailed);
      get().toast("error", text);
      return null;
    }
  },

  async addDownload(req) {
    try {
      const d = await Downloads.add(req);
      set((state) => ({
        downloads: { ...state.downloads, [d.id]: d },
        order: state.order.includes(d.id) ? state.order : [d.id, ...state.order],
        addOpen: false,
        addUrl: "",
        addPreview: null,
      }));
      get().toast("success", `已添加 ${nameOf(d)}`);
      return true;
    } catch (err) {
      get().toast("error", errorMessage(err, strings.loadFailed));
      return false;
    }
  },

  async pause(id) {
    try {
      await Downloads.pause(id);
    } catch (err) {
      get().toast("error", errorMessage(err, strings.loadFailed));
    }
  },
  async resume(id) {
    try {
      await Downloads.resume(id);
    } catch (err) {
      get().toast("error", errorMessage(err, strings.loadFailed));
    }
  },
  async retry(id) {
    try {
      await Downloads.retry(id);
    } catch (err) {
      get().toast("error", errorMessage(err, strings.loadFailed));
    }
  },
  async remove(ids, deleteFile) {
    try {
      if (ids.length === 1) await Downloads.remove(ids[0]!, deleteFile);
      else await Downloads.removeMany(ids, deleteFile);
      set((state) => {
        const downloads = { ...state.downloads };
        for (const id of ids) delete downloads[id];
        return {
          downloads,
          order: state.order.filter((id) => !ids.includes(id)),
          selection: state.selection.filter((id) => !ids.includes(id)),
          detailId: state.detailId !== null && ids.includes(state.detailId) ? null : state.detailId,
          confirmIds: null,
        };
      });
    } catch (err) {
      get().toast("error", errorMessage(err, strings.loadFailed));
    }
  },
  async pauseAll() {
    try {
      await Downloads.pauseAll();
    } catch (err) {
      get().toast("error", errorMessage(err, strings.loadFailed));
    }
  },
  async resumeAll() {
    try {
      await Downloads.resumeAll();
    } catch (err) {
      get().toast("error", errorMessage(err, strings.loadFailed));
    }
  },
  async reveal(id) {
    try {
      await Downloads.reveal(id);
    } catch (err) {
      get().toast("error", errorMessage(err, strings.loadFailed));
    }
  },
  async open(id) {
    try {
      await Downloads.open(id);
    } catch (err) {
      get().toast("error", errorMessage(err, strings.loadFailed));
    }
  },
  async copyUrl(id) {
    const d = get().downloads[id];
    if (!d) return;
    try {
      await System.writeClipboardText(d.url);
      get().toast("success", strings.copied);
    } catch (err) {
      get().toast("error", errorMessage(err, strings.loadFailed));
    }
  },
  askRemove(ids) {
    if (ids.length === 0) return;
    set({ confirmIds: ids });
  },
  cancelRemove() {
    set({ confirmIds: null });
  },
  openSettings(section) {
    set({ settingsOpen: true, settingsSection: section ?? "general" });
    void get().refreshBrowser();
  },
  closeSettings() {
    set({ settingsOpen: false });
  },
  setSettingsSection(section) {
    set({ settingsSection: section });
  },
  async saveSettings(patch) {
    const current = get().settings;
    if (!current) return;
    const next = { ...current, ...patch };
    set({ settings: next });
    try {
      const saved = await Settings.update(next);
      set({ settings: saved });
      if (patch.browserHost !== undefined && saved.browserEnabled) {
        const launch = await get().launchBrowserExtension();
        const message = launch ? launchToast(launch) : null;
        if (message) get().toast(message.kind, message.text);
      }
    } catch (err) {
      get().toast("error", errorMessage(err, strings.saveFailed));
      try {
        set({ settings: await Settings.get() });
      } catch {
        // Keep the optimistic value; the next load fixes it.
      }
    }
  },
  async refreshBrowser() {
    try {
      const [browser, settings] = await Promise.all([Settings.browserStatus(), Settings.get()]);
      set({ browser, settings });
    } catch (err) {
      get().toast("error", errorMessage(err, strings.loadFailed));
    }
  },
  async setBrowserEnabled(enabled) {
    try {
      const browser = await Settings.setBrowserEnabled(enabled);
      set({ browser });
      await get().refreshBrowser();
      if (!enabled) return;
      // Turning the integration on should also make the extension usable,
      // not just register the host.
      const launch = await get().launchBrowserExtension();
      if (!launch) return;
      const message = launchToast(launch);
      if (message) get().toast(message.kind, message.text);
    } catch (err) {
      get().toast("error", errorMessage(err, strings.saveFailed));
    }
  },
  async installBrowserHost() {
    try {
      const browser = await Settings.installBrowserHost();
      set({ browser });
      const launch = await get().launchBrowserExtension();
      await get().refreshBrowser();
      const message = launch ? launchToast(launch) : null;
      if (message) get().toast(message.kind, message.text);
      else if (launch) get().toast("success", strings.hostInstalled);
      return browser;
    } catch (err) {
      get().toast("error", errorMessage(err, strings.saveFailed));
      return null;
    }
  },
  async launchBrowserExtension() {
    try {
      return await Settings.launchBrowserExtension();
    } catch (err) {
      get().toast("error", errorMessage(err, strings.loadFailed));
      return null;
    }
  },
  async openExtensionPage() {
    try {
      return await Settings.openExtensionPage();
    } catch (err) {
      get().toast("error", errorMessage(err, strings.loadFailed));
      return null;
    }
  },
  setTheme(theme) {
    set({ theme });
  },

  async pasteLink() {
    try {
      const text = await System.readClipboardText();
      const match = text.match(/https?:\/\/[^\s"'<>]+/);
      if (!match) {
        get().toast("info", strings.pasteEmpty);
        return;
      }
      get().openAdd(match[0]);
    } catch (err) {
      get().toast("error", errorMessage(err, strings.loadFailed));
    }
  },

  dismissFinished(f) {
    if (f.status === "completed") get().toast("success", `${strings.completedToast}：${f.filePath.split(/[\\/]/).pop() ?? ""}`);
    else get().toast("error", `${strings.failedToast}：${f.errorMessage}`);
  },
}));
