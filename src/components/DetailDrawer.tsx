import { useEffect, useState } from "react";
import { Button, IconButton, Label, UnderlineNav } from "@primer/react";
import { PauseIcon, PlayIcon, ShareIcon, SyncIcon, TrashIcon, XIcon } from "@primer/octicons-react";
import { Downloads } from "../mygo";
import type { Detail } from "../mygo";
import { useApp } from "../store/useApp";
import { errorText } from "../lib/errors";
import { formatBytes, formatETA, formatSpeed, formatTime, nameOf } from "../lib/format";
import { strings } from "../lib/strings";
import { CategoryIcon } from "./CategoryIcon";
import { SegmentMap } from "./SegmentMap";
import { StatusPill } from "./StatusPill";

type Tab = "overview" | "segments" | "logs";

const tabs: { id: Tab; label: string }[] = [
  { id: "overview", label: strings.detailOverview },
  { id: "segments", label: strings.detailSegments },
  { id: "logs", label: strings.detailLogs },
];

/** DetailDrawer is the panel on the right: what the download is, its
 * segments, and its log. */
export function DetailDrawer() {
  const detailId = useApp((s) => s.detailId);
  const openDetail = useApp((s) => s.openDetail);
  const pause = useApp((s) => s.pause);
  const resume = useApp((s) => s.resume);
  const retry = useApp((s) => s.retry);
  const reveal = useApp((s) => s.reveal);
  const open = useApp((s) => s.open);
  const askRemove = useApp((s) => s.askRemove);
  const [detail, setDetail] = useState<Detail | null>(null);
  const [logs, setLogs] = useState<string[]>([]);
  const [tab, setTab] = useState<Tab>("overview");

  useEffect(() => {
    if (detailId === null) {
      setDetail(null);
      return;
    }
    let alive = true;
    const read = async () => {
      try {
        const next = await Downloads.get(detailId);
        if (alive) setDetail(next);
      } catch {
        if (alive) setDetail(null);
      }
    };
    read();
    const timer = setInterval(read, 250);
    return () => {
      alive = false;
      clearInterval(timer);
    };
  }, [detailId]);

  useEffect(() => {
    if (detailId === null || tab !== "logs") return;
    let alive = true;
    const read = async () => {
      try {
        const lines = await Downloads.logs(detailId);
        if (alive) setLogs(lines);
      } catch {
        // The download is gone; the drawer closes on its own.
      }
    };
    read();
    const timer = setInterval(read, 2000);
    return () => {
      alive = false;
      clearInterval(timer);
    };
  }, [detailId, tab]);

  useEffect(() => {
    setTab("overview");
  }, [detailId]);

  if (detailId === null || !detail) return null;
  const d = detail.download;
  const name = nameOf(d);
  const running = d.status === "downloading" || d.status === "queued";

  return (
    <aside className="drawer" aria-label="下载详情">
      <header className="drawer-head">
        <CategoryIcon category={d.category} />
        <div className="drawer-title">
          <h2 title={name}>{name}</h2>
          <p>{d.url}</p>
        </div>
        <IconButton icon={XIcon} aria-label={strings.close} variant="invisible" size="small" onClick={() => openDetail(null)} />
      </header>

      <UnderlineNav aria-label="下载详情" variant="flush">
        {tabs.map((t) => (
          <UnderlineNav.Item key={t.id} aria-current={tab === t.id ? "page" : undefined} onSelect={() => setTab(t.id)}>
            {t.label}
          </UnderlineNav.Item>
        ))}
      </UnderlineNav>

      <div className="drawer-body">
        {tab === "overview" && (
          <>
            <div className="drawer-status">
              <StatusPill status={d.status} />
              {d.resumable && <Label variant="secondary">支持续传</Label>}
              {d.status === "error" && <p className="drawer-error">{errorText(d.errorCode, d.errorMessage)}</p>}
            </div>
            <SegmentMap segments={detail.segments} />
            <dl className="stats-grid">
              <div>
                <dt>{strings.detailSize}</dt>
                <dd>{formatBytes(d.totalBytes)}</dd>
              </div>
              <div>
                <dt>{strings.detailDownloaded}</dt>
                <dd>{formatBytes(d.downloaded)}</dd>
              </div>
              <div>
                <dt>{strings.detailSpeed}</dt>
                <dd>{formatSpeed(d.speed ?? 0)}</dd>
              </div>
              <div>
                <dt>{strings.detailEta}</dt>
                <dd>{formatETA(d.eta ?? -1)}</dd>
              </div>
              <div>
                <dt>{strings.detailConnections}</dt>
                <dd>{detail.segments.length || 1}</dd>
              </div>
              <div>
                <dt>{strings.detailCreated}</dt>
                <dd>{formatTime(d.createdAt)}</dd>
              </div>
            </dl>
            <div className="drawer-actions">
              {running && (
                <Button leadingVisual={PauseIcon} onClick={() => void pause(d.id)}>
                  {strings.pause}
                </Button>
              )}
              {d.status === "paused" && (
                <Button variant="primary" leadingVisual={PlayIcon} onClick={() => void resume(d.id)}>
                  {strings.resume}
                </Button>
              )}
              {d.status === "error" && (
                <Button variant="primary" leadingVisual={SyncIcon} onClick={() => void retry(d.id)}>
                  {strings.retry}
                </Button>
              )}
              {d.status === "completed" && (
                <Button variant="primary" onClick={() => void open(d.id)}>
                  {strings.open}
                </Button>
              )}
              <Button leadingVisual={ShareIcon} onClick={() => void reveal(d.id)}>
                {strings.folder}
              </Button>
              <Button variant="danger" leadingVisual={TrashIcon} onClick={() => askRemove([d.id])}>
                {strings.remove}
              </Button>
            </div>
          </>
        )}

        {tab === "segments" && (
          <table className="segments">
            <thead>
              <tr>
                <th>#</th>
                <th>{strings.detailSegmentRange}</th>
                <th>{strings.detailSegmentProgress}</th>
              </tr>
            </thead>
            <tbody>
              {detail.segments.map((s) => (
                <tr key={s.index}>
                  <td className="num">{s.index}</td>
                  <td className="mono">
                    {s.start}–{s.end}
                  </td>
                  <td className="mono">
                    {s.pos > s.end
                      ? "完成"
                      : `${formatBytes(Math.max(0, s.pos - s.start))} / ${formatBytes(s.end - s.start + 1)}`}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        {tab === "logs" && <pre className="logs">{logs.length === 0 ? "（暂无日志）" : logs.join("\n")}</pre>}
      </div>
    </aside>
  );
}
