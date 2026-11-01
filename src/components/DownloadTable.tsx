import { useEffect, useMemo, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { SkeletonBox } from "@primer/react";
import { SortAscIcon, SortDescIcon } from "@primer/octicons-react";
import { DownloadRow } from "./DownloadRow";
import { EmptyState } from "./EmptyState";
import { useApp } from "../store/useApp";
import { nameOf } from "../lib/format";
import { strings } from "../lib/strings";
import type { Download } from "../mygo";

const columns: { key: "fileName" | "totalBytes" | "status" | null; label: string; className: string }[] = [
  { key: null, label: "", className: "tbl-check" },
  { key: "fileName", label: strings.colName, className: "tbl-name" },
  { key: "totalBytes", label: strings.colSize, className: "tbl-num" },
  { key: null, label: strings.colProgress, className: "tbl-progress" },
  { key: null, label: strings.colSpeed, className: "tbl-num" },
  { key: null, label: strings.colEta, className: "tbl-num" },
  { key: "status", label: strings.colStatus, className: "tbl-status" },
  { key: null, label: "", className: "tbl-actions" },
];

const ROW_HEIGHT = 48;

/** DownloadTable lists the downloads, virtualized past 150 rows. */
export function DownloadTable() {
  const downloads = useApp((s) => s.downloads);
  const order = useApp((s) => s.order);
  const filter = useApp((s) => s.filter);
  const category = useApp((s) => s.category);
  const search = useApp((s) => s.search);
  const sortKey = useApp((s) => s.sortKey);
  const sortDir = useApp((s) => s.sortDir);
  const selection = useApp((s) => s.selection);
  const ready = useApp((s) => s.ready);
  const setSort = useApp((s) => s.setSort);
  const select = useApp((s) => s.select);
  const openDetail = useApp((s) => s.openDetail);

  const [active, setActive] = useState<number | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  const rows = useMemo(() => {
    const needle = search.trim().toLowerCase();
    const list = order
      .map((id) => downloads[id])
      .filter((d): d is Download => Boolean(d))
      .filter((d) => {
        if (filter !== "all" && d.status !== filter) return false;
        if (category && d.category !== category) return false;
        if (needle) {
          const haystack = `${nameOf(d)} ${d.url}`.toLowerCase();
          if (!haystack.includes(needle)) return false;
        }
        return true;
      });
    const direction = sortDir === "asc" ? 1 : -1;
    list.sort((a, b) => {
      switch (sortKey) {
        case "fileName":
          return nameOf(a).localeCompare(nameOf(b), "zh-CN") * direction;
        case "totalBytes":
          return (a.totalBytes - b.totalBytes) * direction;
        case "status":
          return a.status.localeCompare(b.status) * direction;
        default:
          return (a.createdAt - b.createdAt) * direction;
      }
    });
    return list;
  }, [order, downloads, filter, category, search, sortKey, sortDir]);

  const selected = useMemo(() => new Set(selection), [selection]);

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
  });

  useEffect(() => {
    if (active === null) return;
    const index = rows.findIndex((r) => r.id === active);
    if (index >= 0) virtualizer.scrollToIndex(index, { align: "auto" });
  }, [active, rows, virtualizer]);

  const move = (delta: number) => {
    if (rows.length === 0) return;
    const index = rows.findIndex((r) => r.id === active);
    const next = Math.min(rows.length - 1, Math.max(0, (index < 0 ? 0 : index) + delta));
    const target = rows[next];
    if (target) {
      setActive(target.id);
      select(target.id, "only");
    }
  };

  if (!ready) {
    return (
      <div className="tbl" aria-busy>
        {[0, 1, 2].map((i) => (
          <div className="tbl-skeleton" key={i}>
            <SkeletonBox width="14px" height="14px" />
            <SkeletonBox width="45%" height="10px" />
            <SkeletonBox width="48px" height="10px" />
            <SkeletonBox width="70%" height="10px" />
          </div>
        ))}
      </div>
    );
  }

  if (rows.length === 0) {
    return <EmptyState filtered={filter !== "all" || Boolean(category) || search.trim() !== ""} />;
  }

  return (
    <div className="tbl-wrap">
      <div className="tbl-head" role="row">
        {columns.map((column, i) => (
          <div key={i} className={`tbl-cell ${column.className}`}>
            {column.key ? (
              <button
                type="button"
                className={`tbl-sort${sortKey === column.key ? " is-active" : ""}`}
                onClick={() => setSort(column.key!)}
                aria-label={`按${column.label}排序`}
              >
                {column.label}
                {sortKey === column.key ? sortDir === "asc" ? <SortAscIcon size={12} /> : <SortDescIcon size={12} /> : null}
              </button>
            ) : (
              column.label
            )}
          </div>
        ))}
      </div>
      <div
        className="tbl-scroll"
        ref={scrollRef}
        role="table"
        aria-label="下载列表"
        tabIndex={0}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown") {
            event.preventDefault();
            move(1);
          } else if (event.key === "ArrowUp") {
            event.preventDefault();
            move(-1);
          } else if (event.key === "Enter" && active !== null) {
            openDetail(active);
          }
        }}
      >
        <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
          {virtualizer.getVirtualItems().map((item) => {
            const row = rows[item.index];
            if (!row) return null;
            return (
              <div
                key={row.id}
                style={{
                  position: "absolute",
                  top: 0,
                  left: 0,
                  width: "100%",
                  transform: `translateY(${item.start}px)`,
                }}
              >
                <DownloadRow download={row} selected={selected.has(row.id)} onSelect={select} onOpen={openDetail} />
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
