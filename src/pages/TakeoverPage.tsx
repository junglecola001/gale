import { useEffect, useState } from "react";
import { BaseStyles, Button, Checkbox, ConfirmationDialog, IconButton, Label, ThemeProvider } from "@primer/react";
import { DashIcon, PauseIcon, PlayIcon, ShareIcon, SyncIcon, TrashIcon, XIcon } from "@primer/octicons-react";
import { currentWindow, isCallError, isMyGo } from "mygo-runtime";
import { Downloads, Settings } from "../mygo";
import type { Detail, Download } from "../mygo";
import { strings } from "../lib/strings";
import { errorText } from "../lib/errors";
import { formatBytes, formatETA, formatSpeed, nameOf, percentDone } from "../lib/format";
import { useThemeScheme } from "../lib/theme";
import { CategoryIcon } from "../components/CategoryIcon";
import { DownloadProgress } from "../components/ProgressBar";
import { SegmentMap } from "../components/SegmentMap";
import { StatusPill } from "../components/StatusPill";

/** messageOf is the sentence an action's failure puts on screen: the Go
 * error when there is one, the fallback for anything else. */
function messageOf(err: unknown, fallback: string): string {
  if (isCallError(err) && err.message) return err.message;
  return fallback;
}

/** segmentInfo says what a connection is doing, in the words of the window. */
function segmentInfo(segment: { pos: number; start: number; end: number }, status: string): string {
  if (segment.pos > segment.end) return strings.takeoverSegmentDone;
  if (status === "downloading") return strings.takeoverSegmentRunning;
  if (status === "paused") return strings.takeoverSegmentPaused;
  return strings.takeoverSegmentWaiting;
}

function resumeText(d: Download): string {
  if (d.resumable) return strings.takeoverResumeYes;
  if (d.status === "queued") return strings.takeoverResumeChecking;
  return strings.takeoverResumeNo;
}

function closeWindow() {
  if (isMyGo()) {
    void currentWindow.close();
    return;
  }
  window.close();
}

/** TakeoverPage is the window a browser's download opens in: what Gale took,
 * how far it got, and the connections fetching it. */
export function TakeoverPage({ id }: { id: number }) {
  const [detail, setDetail] = useState<Detail | null>(null);
  const [gone, setGone] = useState(false);
  const [theme, setTheme] = useState<string | undefined>(undefined);
  const [expanded, setExpanded] = useState(true);
  const [confirming, setConfirming] = useState(false);
  const [deleteFile, setDeleteFile] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const { mode, dayScheme, nightScheme } = useThemeScheme(theme);

  const download = detail?.download;
  const live = download?.status === "downloading" || download?.status === "queued";

  useEffect(() => {
    let alive = true;
    Settings.get()
      .then((value) => {
        if (alive) setTheme(value.theme);
      })
      .catch(() => {
        // Without the settings the window keeps the system appearance.
      });
    return () => {
      alive = false;
    };
  }, []);

  // A running download changes many times a second; a settled one only when
  // the user does something, so the poll slows down to a check of its row.
  useEffect(() => {
    let alive = true;
    const read = async () => {
      try {
        const next = await Downloads.get(id);
        if (!alive) return;
        setDetail(next);
        setGone(false);
      } catch {
        if (alive) setGone(true);
      }
    };
    void read();
    const timer = setInterval(read, live ? 400 : 2000);
    return () => {
      alive = false;
      clearInterval(timer);
    };
  }, [id, live]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || confirming) return;
      closeWindow();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [confirming]);

  const act = async (fn: () => Promise<void>) => {
    setBusy(true);
    try {
      await fn();
      setError("");
    } catch (err) {
      setError(messageOf(err, strings.loadFailed));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    setConfirming(false);
    try {
      await Downloads.remove(id, deleteFile);
      closeWindow();
    } catch (err) {
      setError(messageOf(err, strings.loadFailed));
    }
  };

  const percent = download ? percentDone(download.downloaded, download.totalBytes) : 0;
  const name = download ? nameOf(download) : "";

  useEffect(() => {
    document.title = name ? `${name} · ${strings.appName}` : strings.takeoverTitle;
  }, [name]);

  return (
    <ThemeProvider colorMode={mode} dayScheme={dayScheme} nightScheme={nightScheme} contextOnly>
      <BaseStyles className="app takeover">
        <header className="tb">
          <div className="tb-brand">
            <span className="tb-logo" aria-hidden>
              ◈
            </span>
            <span className="tb-name">{strings.appName}</span>
            <span className="tb-sub">{strings.takeoverTitle}</span>
          </div>
          <div className="tb-right">
            <div className="tb-controls">
              <button
                type="button"
                aria-label="最小化"
                onClick={() => {
                  if (isMyGo()) void currentWindow.minimize();
                }}
              >
                <DashIcon size={14} />
              </button>
              <button type="button" className="tb-close" aria-label={strings.close} onClick={closeWindow}>
                <XIcon size={14} />
              </button>
            </div>
          </div>
        </header>

        <div className="takeover-body">
          {gone ? (
            <p className="takeover-note">{strings.takeoverGone}</p>
          ) : !download || !detail ? (
            <p className="takeover-note">{strings.loading}</p>
          ) : (
            <>
              <header className="drawer-head">
                <CategoryIcon category={download.category} />
                <div className="drawer-title">
                  <h2 title={name}>{name}</h2>
                  <p title={download.url}>{download.url}</p>
                </div>
                <IconButton
                  icon={ShareIcon}
                  aria-label={strings.folder}
                  variant="invisible"
                  size="small"
                  onClick={() => void act(() => Downloads.reveal(id))}
                />
              </header>

              <div className="drawer-status">
                <StatusPill status={download.status} error={download.errorMessage} />
                {download.resumable && <Label variant="secondary">{strings.addResumable}</Label>}
              </div>
              {download.status === "error" && (
                <p className="drawer-error">{errorText(download.errorCode, download.errorMessage)}</p>
              )}

              <div className="tbl-progress takeover-progress">
                <DownloadProgress
                  done={download.downloaded}
                  total={download.totalBytes}
                  status={download.status}
                  percent={percent}
                />
                <span className="tbl-percent">{percent}%</span>
              </div>

              <dl className="stats-grid takeover-stats">
                <div>
                  <dt>{strings.takeoverFileSize}</dt>
                  <dd>{formatBytes(download.totalBytes)}</dd>
                </div>
                <div>
                  <dt>{strings.detailDownloaded}</dt>
                  <dd>{formatBytes(download.downloaded)}</dd>
                </div>
                <div>
                  <dt>{strings.detailSpeed}</dt>
                  <dd>{formatSpeed(download.speed ?? 0)}</dd>
                </div>
                <div>
                  <dt>{strings.detailEta}</dt>
                  <dd>{formatETA(download.eta ?? -1)}</dd>
                </div>
                <div>
                  <dt>{strings.detailConnections}</dt>
                  <dd>{detail.segments.length || download.connections || 1}</dd>
                </div>
                <div>
                  <dt>{strings.takeoverResume}</dt>
                  <dd>{resumeText(download)}</dd>
                </div>
              </dl>

              <div className="takeover-actions">
                <Button size="small" onClick={() => setExpanded((value) => !value)}>
                  {expanded ? strings.takeoverHide : strings.takeoverShow}
                </Button>
                <span className="takeover-spacer" />
                {live && (
                  <Button
                    size="small"
                    leadingVisual={PauseIcon}
                    disabled={busy}
                    onClick={() => void act(() => Downloads.pause(id))}
                  >
                    {strings.pause}
                  </Button>
                )}
                {download.status === "paused" && (
                  <Button
                    size="small"
                    variant="primary"
                    leadingVisual={PlayIcon}
                    disabled={busy}
                    onClick={() => void act(() => Downloads.resume(id))}
                  >
                    {strings.resume}
                  </Button>
                )}
                {download.status === "error" && (
                  <Button
                    size="small"
                    variant="primary"
                    leadingVisual={SyncIcon}
                    disabled={busy}
                    onClick={() => void act(() => Downloads.retry(id))}
                  >
                    {strings.retry}
                  </Button>
                )}
                {download.status === "completed" && (
                  <Button
                    size="small"
                    variant="primary"
                    disabled={busy}
                    onClick={() => void act(() => Downloads.open(id))}
                  >
                    {strings.open}
                  </Button>
                )}
                <Button
                  size="small"
                  variant="danger"
                  leadingVisual={TrashIcon}
                  disabled={busy}
                  onClick={() => {
                    setDeleteFile(download.status !== "completed");
                    setConfirming(true);
                  }}
                >
                  {download.status === "completed" ? strings.remove : strings.takeoverCancel}
                </Button>
              </div>

              {expanded && (
                <section className="takeover-details">
                  <p className="takeover-caption">{strings.takeoverCaption}</p>
                  <SegmentMap segments={detail.segments} />
                  <div className="takeover-table">
                    <table className="segments">
                      <thead>
                        <tr>
                          <th className="takeover-col-num">#</th>
                          <th>{strings.detailDownloaded}</th>
                          <th>{strings.takeoverInfo}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {detail.segments.length === 0 ? (
                          <tr>
                            <td colSpan={3} className="takeover-empty-row">
                              {strings.takeoverNoSegments}
                            </td>
                          </tr>
                        ) : (
                          detail.segments.map((segment) => (
                            <tr key={segment.index}>
                              <td className="num">{segment.index}</td>
                              <td className="num">
                                {formatBytes(Math.max(0, segment.pos - segment.start))}
                                <span className="takeover-seg-total">
                                  {" / "}
                                  {formatBytes(segment.end - segment.start + 1)}
                                </span>
                              </td>
                              <td>{segmentInfo(segment, download.status)}</td>
                            </tr>
                          ))
                        )}
                      </tbody>
                    </table>
                  </div>
                </section>
              )}
            </>
          )}

          {error && <p className="drawer-error">{error}</p>}
        </div>

        {confirming && download && (
          <ConfirmationDialog
            title={strings.takeoverRemoveTitle}
            confirmButtonContent={strings.confirm}
            cancelButtonContent={strings.cancel}
            confirmButtonType="danger"
            onClose={(gesture) => {
              if (gesture === "confirm") void remove();
              else setConfirming(false);
            }}
          >
            <p style={{ margin: 0 }}>{strings.takeoverRemoveHint(name)}</p>
            <label
              style={{
                display: "flex",
                alignItems: "center",
                gap: "var(--base-size-8)",
                marginTop: "var(--base-size-12)",
              }}
            >
              <Checkbox checked={deleteFile} onChange={(event) => setDeleteFile(event.target.checked)} />
              <span>{strings.removeFile}</span>
            </label>
          </ConfirmationDialog>
        )}
      </BaseStyles>
    </ThemeProvider>
  );
}
