import { IconButton } from "@primer/react";
import { FileDirectoryIcon, PauseIcon, PlayIcon, SyncIcon, TrashIcon } from "@primer/octicons-react";
import { useApp } from "../store/useApp";
import type { Download } from "../mygo";
import { strings } from "../lib/strings";

/** DownloadActions are the buttons a row shows on hover. */
export function DownloadActions({ download }: { download: Download }) {
  const pause = useApp((s) => s.pause);
  const resume = useApp((s) => s.resume);
  const retry = useApp((s) => s.retry);
  const reveal = useApp((s) => s.reveal);
  const askRemove = useApp((s) => s.askRemove);
  const running = download.status === "downloading" || download.status === "queued";

  return (
    <div className="row-actions">
      {running && (
        <IconButton icon={PauseIcon} aria-label={strings.pause} title={strings.pause} variant="invisible" size="small" onClick={() => void pause(download.id)} />
      )}
      {download.status === "paused" && (
        <IconButton icon={PlayIcon} aria-label={strings.resume} title={strings.resume} variant="invisible" size="small" onClick={() => void resume(download.id)} />
      )}
      {download.status === "error" && (
        <IconButton icon={SyncIcon} aria-label={strings.retry} title={strings.retry} variant="invisible" size="small" onClick={() => void retry(download.id)} />
      )}
      <IconButton icon={FileDirectoryIcon} aria-label={strings.folder} title={strings.folder} variant="invisible" size="small" onClick={() => void reveal(download.id)} />
      <IconButton icon={TrashIcon} aria-label={strings.remove} title={strings.remove} variant="invisible" size="small" onClick={() => askRemove([download.id])} />
    </div>
  );
}
