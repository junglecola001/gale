import { useEffect } from "react";
import { BaseStyles, ThemeProvider } from "@primer/react";
import { AddDialog } from "./components/AddDialog";
import { ConfirmDialog } from "./components/ConfirmDialog";
import { DetailDrawer } from "./components/DetailDrawer";
import { DownloadTable } from "./components/DownloadTable";
import { Sidebar } from "./components/Sidebar";
import { TitleBar } from "./components/TitleBar";
import { Toasts } from "./components/Toasts";
import { Toolbar } from "./components/Toolbar";
import { SettingsPage } from "./pages/SettingsPage";
import { useApp } from "./store/useApp";
import { useThemeScheme } from "./lib/theme";
import { formatSpeed } from "./lib/format";
import { strings } from "./lib/strings";

/** App is the window: title bar, filters, table, drawer and the footer. */
export default function App() {
  const init = useApp((s) => s.init);
  const stats = useApp((s) => s.stats);
  const settings = useApp((s) => s.settings);
  const settingsOpen = useApp((s) => s.settingsOpen);
  const selection = useApp((s) => s.selection);
  const loadError = useApp((s) => s.loadError);
  const { mode, dayScheme, nightScheme } = useThemeScheme(settings?.theme);

  useEffect(() => {
    void init();
  }, [init]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const typing =
        event.target instanceof HTMLElement &&
        (event.target.tagName === "INPUT" || event.target.tagName === "TEXTAREA" || event.target.isContentEditable);
      const state = useApp.getState();
      if (event.key === "Escape") {
        if (state.addOpen) state.closeAdd();
        else if (state.confirmIds) state.cancelRemove();
        else if (state.settingsOpen) state.closeSettings();
        else if (state.detailId !== null) state.openDetail(null);
        return;
      }
      if (event.ctrlKey && event.key.toLowerCase() === "n") {
        event.preventDefault();
        state.openAdd();
        return;
      }
      if (event.ctrlKey && event.key.toLowerCase() === "v" && !typing) {
        event.preventDefault();
        void state.pasteLink();
        return;
      }
      if (event.ctrlKey && event.key.toLowerCase() === "f") {
        event.preventDefault();
        document.getElementById("gale-search")?.focus();
        return;
      }
      if (event.ctrlKey && event.key.toLowerCase() === "a" && !typing) {
        event.preventDefault();
        state.selectAll();
        return;
      }
      if (typing) return;
      if (event.key === "Delete" || event.key === "Backspace") {
        if (state.selection.length === 0) return;
        event.preventDefault();
        state.askRemove(state.selection);
        return;
      }
      if (event.key === " " && state.selection.length > 0) {
        event.preventDefault();
        for (const id of state.selection) {
          const download = state.downloads[id];
          if (!download) continue;
          if (download.status === "downloading" || download.status === "queued") void state.pause(id);
          else if (download.status === "paused") void state.resume(id);
          else if (download.status === "error") void state.retry(id);
        }
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // Links dragged from a browser arrive as text/uri-list.
  useEffect(() => {
    const onDrop = (event: DragEvent) => {
      const text = event.dataTransfer?.getData("text/uri-list") || event.dataTransfer?.getData("text/plain");
      if (!text) return;
      event.preventDefault();
      const match = text.match(/https?:\/\/\S+/);
      if (match) useApp.getState().openAdd(match[0]);
    };
    const onDragOver = (event: DragEvent) => event.preventDefault();
    window.addEventListener("drop", onDrop);
    window.addEventListener("dragover", onDragOver);
    return () => {
      window.removeEventListener("drop", onDrop);
      window.removeEventListener("dragover", onDragOver);
    };
  }, []);

  const limit = settings
    ? settings.speedLimit > 0
      ? strings.limitOn(formatSpeed(settings.speedLimit).replace("/s", ""))
      : strings.limitOff
    : strings.limitOff;

  return (
    <ThemeProvider colorMode={mode} dayScheme={dayScheme} nightScheme={nightScheme} contextOnly>
      <BaseStyles className="app">
        <TitleBar />
        <div className="body">
          <Sidebar />
          {settingsOpen ? (
            <SettingsPage />
          ) : (
            <main className="main">
              <Toolbar />
              {loadError ? (
                <p className="drawer-error" style={{ margin: "var(--base-size-16)" }}>
                  {loadError}
                </p>
              ) : (
                <DownloadTable />
              )}
              <footer className="footer">
                <span>
                  {strings.statsFooter(stats.active, formatSpeed(stats.speed), limit)}
                  {selection.length > 0 ? ` · 已选 ${selection.length}` : ""}
                </span>
                <span className="footer-actions">
                  <button type="button" className="link" onClick={() => useApp.getState().openSettings()}>
                    {strings.settings}
                  </button>
                  <button type="button" className="link" onClick={() => useApp.getState().openSettings("browser")}>
                    {strings.browser}
                  </button>
                </span>
              </footer>
            </main>
          )}
          <DetailDrawer />
        </div>
        <AddDialog />
        <ConfirmDialog />
        <Toasts />
      </BaseStyles>
    </ThemeProvider>
  );
}
