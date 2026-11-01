import { ActionList, ActionMenu, Button } from "@primer/react";
import { PauseIcon, PlayIcon, PlusIcon, SortAscIcon, SortDescIcon, TriangleDownIcon } from "@primer/octicons-react";
import { useApp, type SortKey } from "../store/useApp";
import { strings } from "../lib/strings";

const sortOptions: { key: SortKey; label: string }[] = [
  { key: "createdAt", label: strings.sortCreated },
  { key: "fileName", label: strings.sortName },
  { key: "totalBytes", label: strings.sortSize },
  { key: "status", label: strings.sortStatus },
];

/** Toolbar holds the actions above the table: a new download, all start and
 * all pause, and the ordering. */
export function Toolbar() {
  const openAdd = useApp((s) => s.openAdd);
  const pauseAll = useApp((s) => s.pauseAll);
  const resumeAll = useApp((s) => s.resumeAll);
  const sortKey = useApp((s) => s.sortKey);
  const sortDir = useApp((s) => s.sortDir);
  const setSort = useApp((s) => s.setSort);
  const toggleSortDir = useApp((s) => s.toggleSortDir);
  const current = sortOptions.find((o) => o.key === sortKey)?.label ?? strings.sortCreated;

  return (
    <div className="toolbar">
      <Button variant="primary" leadingVisual={PlusIcon} onClick={() => openAdd()}>
        {strings.newDownload}
      </Button>
      <Button leadingVisual={PlayIcon} onClick={() => void resumeAll()}>
        {strings.resumeAll}
      </Button>
      <Button leadingVisual={PauseIcon} onClick={() => void pauseAll()}>
        {strings.pauseAll}
      </Button>
      <div className="toolbar-spacer" />
      <ActionMenu>
        <ActionMenu.Button trailingVisual={TriangleDownIcon} leadingVisual={sortDir === "asc" ? SortAscIcon : SortDescIcon}>
          {current}
        </ActionMenu.Button>
        <ActionMenu.Overlay align="end">
          <ActionList selectionVariant="single">
            {sortOptions.map((option) => (
              <ActionList.Item key={option.key} selected={option.key === sortKey} onSelect={() => setSort(option.key)}>
                {option.label}
              </ActionList.Item>
            ))}
            <ActionList.Divider />
            <ActionList.Item role="menuitem" onSelect={toggleSortDir}>
              {sortDir === "asc" ? strings.sortAsc : strings.sortDesc}
            </ActionList.Item>
          </ActionList>
        </ActionMenu.Overlay>
      </ActionMenu>
    </div>
  );
}
