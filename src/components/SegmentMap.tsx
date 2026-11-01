import type { Segment } from "../mygo";

// The map is drawn in a viewBox of this width: a download's byte offsets are
// far past the precision a single-precision SVG coordinate keeps, so the
// bytes are scaled to it first.
const UNITS = 1000;

// Running segments in turn, kept away from the green of a finished one.
const colors = [
  "var(--bgColor-accent-emphasis)",
  "var(--bgColor-done-emphasis)",
  "var(--bgColor-attention-emphasis)",
  "var(--bgColor-severe-emphasis)",
  "var(--bgColor-danger-emphasis)",
  "var(--bgColor-accent-emphasis)",
];

/** SegmentMap draws a download's segments to scale: what is done, what each
 * connection is fetching, and what is left. */
export function SegmentMap({ segments }: { segments: Segment[] }) {
  if (segments.length === 0) {
    return <div className="segmap-empty">尚无分段</div>;
  }
  const total = segments.reduce((max, s) => Math.max(max, s.end + 1), 1);
  const scale = (bytes: number) => (bytes / total) * UNITS;
  return (
    <div className="segmap">
      <svg viewBox={`0 0 ${UNITS} 32`} preserveAspectRatio="none" role="img" aria-label="分段进度">
        {segments.map((segment, i) => {
          const width = Math.max(0.5, scale(segment.end - segment.start + 1));
          const done = segment.pos > segment.end;
          const started = segment.pos > segment.start;
          const color = done
            ? "var(--bgColor-success-emphasis)"
            : started
              ? (colors[i % colors.length] ?? colors[0])
              : "var(--bgColor-neutral-muted)";
          const percent = Math.round(((segment.pos - segment.start) / (segment.end - segment.start + 1)) * 100);
          return (
            <rect
              key={segment.index}
              x={scale(segment.start)}
              y={0}
              width={width}
              height={32}
              style={{ fill: color }}
              opacity={done ? 1 : started ? 0.85 : 1}
            >
              <title>{`#${segment.index} ${segment.start}-${segment.end} · ${percent}%`}</title>
            </rect>
          );
        })}
      </svg>
    </div>
  );
}
