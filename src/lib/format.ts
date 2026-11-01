import type { Download } from "../mygo";

const UNITS = ["B", "KiB", "MiB", "GiB", "TiB"];

/** formatBytes shows a byte count in binary units, one decimal place. */
export function formatBytes(bytes: number): string {
  if (bytes < 0) return "—";
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < UNITS.length - 1) {
    value /= 1024;
    unit++;
  }
  const digits = unit === 0 ? 0 : 1;
  return `${value.toFixed(digits)} ${UNITS[unit]}`;
}

/** formatSpeed is a byte rate, e.g. "12.1 MiB/s". */
export function formatSpeed(bytesPerSecond: number): string {
  if (bytesPerSecond <= 0) return "—";
  return `${formatBytes(bytesPerSecond)}/s`;
}

/** formatETA shows seconds as h:mm:ss or m:ss. */
export function formatETA(seconds: number): string {
  if (seconds <= 0) return "—";
  const total = Math.round(seconds);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const pad = (v: number) => String(v).padStart(2, "0");
  if (h > 0) return `${h}:${pad(m)}:${pad(s)}`;
  return `${m}:${pad(s)}`;
}

/** percentDone is 0–100, 0 when the size is unknown. */
export function percentDone(downloaded: number, total: number): number {
  if (total <= 0) return 0;
  return Math.min(100, Math.max(0, Math.round((downloaded / total) * 100)));
}

/** formatTime is a local date and time, e.g. "10月31日 18:04". */
export function formatTime(ms: number): string {
  if (!ms) return "—";
  const date = new Date(ms);
  const month = date.getMonth() + 1;
  const day = date.getDate();
  const pad = (v: number) => String(v).padStart(2, "0");
  return `${month}月${day}日 ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/** hostOf is the host of a URL, for the second line of a row. */
export function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return "";
  }
}

/** nameOf is the name to show: the file's, else the URL's last segment. */
export function nameOf(download: Download): string {
  if (download.fileName) return download.fileName;
  const url = download.url.split("#")[0]?.split("?")[0] ?? "";
  const trimmed = url.replace(/\/+$/, "");
  const segment = trimmed.slice(trimmed.lastIndexOf("/") + 1);
  try {
    return decodeURIComponent(segment) || download.url;
  } catch {
    return segment || download.url;
  }
}
