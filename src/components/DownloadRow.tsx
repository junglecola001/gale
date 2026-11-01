import * as ContextMenu from "@radix-ui/react-context-menu";
import { Checkbox } from "@primer/react";
import { CopyIcon, FileDirectoryIcon, PlayIcon, PauseIcon, RocketIcon, SyncIcon, TrashIcon } from "@primer/octicons-react";
import { CategoryIcon } from "./CategoryIcon";
import { DownloadActions } from "./DownloadActions";
import { DownloadProgress } from "./ProgressBar";
import { StatusPill } from "./StatusPill";
import { useApp } from "../store/useApp";
import { errorText } from "../lib/errors";
import { formatBytes, formatETA, formatSpeed, hostOf, nameOf } from "../lib/format";
import { strings } from "../lib/strings";
import type { Download } from "../mygo";

/** DownloadRow is one line of the table, with its right-click menu. */
export function DownloadRow({
  download,
  selected,
  onSelect,
  onOpen,
}: {
  download: Download;
  selected: boolean;
  onSelect: (id: number, mode: "toggle" | "range" | "only") => void;
  onOpen: (id: number) => void;
}) {
  const state = useApp.getState;
  const name = nameOf(download);
  const percent = download.totalBytes > 0 ? (download.downloaded / download.totalBytes) * 100 : 0;
  const failed = download.status === "error";
  const running = download.status === "downloading" || download.status === "queued";

  return (
    <ContextMenu.Root
      onOpenChange={(open) => {
        if (open) state().select(download.id, "only");
      }}
    >
      <ContextMenu.Trigger asChild>
        <div
          role="row"
          tabIndex={0}
          aria-selected={selected}
          data-id={download.id}
          className={`tbl-row${selected ? " is-selected" : ""}`}
          onClick={(event) => {
            if (event.target instanceof HTMLElement && event.target.closest("button, input")) return;
            if (event.shiftKey) {
              onSelect(download.id, "range");
              return;
            }
            if (event.ctrlKey || event.metaKey) {
              onSelect(download.id, "toggle");
              return;
            }
            onOpen(download.id);
          }}
          onKeyDown={(event) => {
            if (event.key === "Enter") onOpen(download.id);
          }}
        >
          <div className="tbl-cell tbl-check">
            <Checkbox
              checked={selected}
              aria-label={`选择 ${name}`}
              onClick={(event) => event.stopPropagation()}
              onChange={() => onSelect(download.id, "toggle")}
            />
          </div>
          <div className="tbl-cell tbl-name">
            <CategoryIcon category={download.category} />
            <span className="tbl-name-text">
              <span className="tbl-name-title" title={name}>
                {name}
              </span>
              <span className="tbl-name-host">{hostOf(download.url)}</span>
            </span>
          </div>
          <div className="tbl-cell tbl-num">{formatBytes(download.totalBytes)}</div>
          <div className="tbl-cell tbl-progress">
            <DownloadProgress
              done={download.downloaded}
              total={download.totalBytes}
              status={download.status}
              percent={percent}
            />
            <span className="tbl-percent">{download.totalBytes > 0 ? `${Math.round(percent)}%` : "—"}</span>
          </div>
          <div className="tbl-cell tbl-num">
            {download.status === "downloading" ? formatSpeed(download.speed ?? 0) : "—"}
          </div>
          <div className="tbl-cell tbl-num">
            {download.status === "downloading" ? formatETA(download.eta ?? -1) : "—"}
          </div>
          <div className="tbl-cell tbl-status">
            <StatusPill status={download.status} error={failed ? errorText(download.errorCode, download.errorMessage) : undefined} />
          </div>
          <div className="tbl-cell tbl-actions">
            <DownloadActions download={download} />
          </div>
        </div>
      </ContextMenu.Trigger>
      <ContextMenu.Portal>
        <ContextMenu.Content className="menu">
          <ContextMenu.Item className="menu-item" onSelect={() => void state().open(download.id)}>
            <RocketIcon size={14} /> {strings.open}
          </ContextMenu.Item>
          <ContextMenu.Item className="menu-item" onSelect={() => void state().reveal(download.id)}>
            <FileDirectoryIcon size={14} /> {strings.folder}
          </ContextMenu.Item>
          <ContextMenu.Item className="menu-item" onSelect={() => void state().copyUrl(download.id)}>
            <CopyIcon size={14} /> {strings.copyUrl}
          </ContextMenu.Item>
          <ContextMenu.Separator className="menu-sep" />
          {running && (
            <ContextMenu.Item className="menu-item" onSelect={() => void state().pause(download.id)}>
              <PauseIcon size={14} /> {strings.pause}
            </ContextMenu.Item>
          )}
          {download.status === "paused" && (
            <ContextMenu.Item className="menu-item" onSelect={() => void state().resume(download.id)}>
              <PlayIcon size={14} /> {strings.resume}
            </ContextMenu.Item>
          )}
          {failed && (
            <ContextMenu.Item className="menu-item" onSelect={() => void state().retry(download.id)}>
              <SyncIcon size={14} /> {strings.retry}
            </ContextMenu.Item>
          )}
          <ContextMenu.Separator className="menu-sep" />
          <ContextMenu.Item className="menu-item menu-danger" onSelect={() => state().askRemove([download.id])}>
            <TrashIcon size={14} /> {strings.remove}
          </ContextMenu.Item>
        </ContextMenu.Content>
      </ContextMenu.Portal>
    </ContextMenu.Root>
  );
}
