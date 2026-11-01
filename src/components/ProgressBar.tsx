import { ProgressBar } from "@primer/react";
import type { Status } from "../mygo";

/** ProgressBar draws a download's progress: a Primer bar, a shimmer while
 * the size is unknown, and the success color when it is done. */
export function DownloadProgress({
  done,
  total,
  status,
  percent,
  className,
}: {
  done: number;
  total: number;
  status: Status;
  percent: number;
  className?: string;
}) {
  const unknown = total <= 0;
  const complete = status === "completed" || (!unknown && done >= total);
  const failed = status === "error";
  if (!unknown) {
    const classes = ["tbl-progress-bar", className].filter(Boolean).join(" ");
    return (
      <span className={classes} aria-label={`已下载 ${Math.round(percent)}%`}>
        <ProgressBar
          progress={Math.min(100, Math.max(0, percent))}
          bg={failed ? "danger" : complete ? "success" : "accent"}
          barSize="small"
        />
      </span>
    );
  }
  const classes = ["pb", "pb-unknown", className].filter(Boolean).join(" ");
  return (
    <div className={classes} role="progressbar" aria-valuetext="大小未知">
      <div className="pb-fill" style={{ width: "30%" }} />
    </div>
  );
}
