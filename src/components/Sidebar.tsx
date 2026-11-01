import { CounterLabel, NavList } from "@primer/react";
import { GearIcon, GlobeIcon } from "@primer/octicons-react";
import { useMemo } from "react";
import { useApp, type Filter } from "../store/useApp";
import { categories, filters, strings } from "../lib/strings";
import type { Status } from "../mygo";

const statusOrder: Filter[] = ["all", "downloading", "queued", "paused", "completed", "error"];

/** Sidebar filters the list by status and by category. */
export function Sidebar() {
  const downloads = useApp((s) => s.downloads);
  const filter = useApp((s) => s.filter);
  const category = useApp((s) => s.category);
  const setFilter = useApp((s) => s.setFilter);
  const setCategory = useApp((s) => s.setCategory);
  const openSettings = useApp((s) => s.openSettings);
  const settingsOpen = useApp((s) => s.settingsOpen);

  const counts = useMemo(() => {
    const byStatus: Record<string, number> = { all: 0 };
    const byCategory: Record<string, number> = {};
    for (const d of Object.values(downloads)) {
      byStatus.all = (byStatus.all ?? 0) + 1;
      byStatus[d.status] = (byStatus[d.status] ?? 0) + 1;
      byCategory[d.category] = (byCategory[d.category] ?? 0) + 1;
    }
    return { byStatus, byCategory };
  }, [downloads]);

  const counter = (n: number) =>
    n > 0 ? (
      <NavList.TrailingVisual>
        <CounterLabel>{n}</CounterLabel>
      </NavList.TrailingVisual>
    ) : null;

  return (
    <nav className="sb" aria-label="筛选">
      <NavList aria-label="按状态筛选">
        <NavList.Group title="状态">
          {statusOrder.map((key) => {
            const count = key === "all" ? (counts.byStatus.all ?? 0) : (counts.byStatus[key as Status] ?? 0);
            return (
              <NavList.Item
                key={key}
                as="button"
                type="button"
                aria-current={filter === key && !category ? "page" : undefined}
                onClick={() => {
                  setFilter(key);
                  if (settingsOpen) useApp.getState().closeSettings();
                }}
              >
                {filters[key as keyof typeof filters]}
                {counter(count)}
              </NavList.Item>
            );
          })}
        </NavList.Group>
        <NavList.Group title="分类">
          {categories.map((c) => (
            <NavList.Item
              key={c.id}
              as="button"
              type="button"
              aria-current={category === c.id ? "page" : undefined}
              onClick={() => {
                setCategory(c.id);
                if (settingsOpen) useApp.getState().closeSettings();
              }}
            >
              {c.label}
              {counter(counts.byCategory[c.id] ?? 0)}
            </NavList.Item>
          ))}
        </NavList.Group>
      </NavList>

      <div className="sb-foot">
        <NavList aria-label="应用">
          <NavList.Item
            as="button"
            type="button"
            aria-current={settingsOpen ? "page" : undefined}
            onClick={() => openSettings()}
          >
            <NavList.LeadingVisual>
              <GearIcon />
            </NavList.LeadingVisual>
            {strings.settings}
          </NavList.Item>
          <NavList.Item as="button" type="button" onClick={() => openSettings("browser")}>
            <NavList.LeadingVisual>
              <GlobeIcon />
            </NavList.LeadingVisual>
            {strings.browser}
          </NavList.Item>
        </NavList>
      </div>
    </nav>
  );
}
