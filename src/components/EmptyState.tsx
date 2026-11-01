import { Button } from "@primer/react";
import { Blankslate } from "@primer/react/experimental";
import { DownloadIcon } from "@primer/octicons-react";
import { useApp } from "../store/useApp";
import { strings } from "../lib/strings";

/** EmptyState fills the list when there is nothing to show. */
export function EmptyState({ filtered }: { filtered: boolean }) {
  const openAdd = useApp((s) => s.openAdd);
  const pasteLink = useApp((s) => s.pasteLink);
  return (
    <div className="empty">
      <Blankslate spacious>
        <Blankslate.Visual>
          <DownloadIcon size={32} />
        </Blankslate.Visual>
        <Blankslate.Heading as="h2">{filtered ? strings.emptyFilter : strings.emptyTitle}</Blankslate.Heading>
        {!filtered && <Blankslate.Description>{strings.emptyHint}</Blankslate.Description>}
        <div className="empty-actions">
          <Button variant="primary" onClick={() => openAdd()}>
            {strings.newDownload}
          </Button>
          {!filtered && <Button onClick={() => void pasteLink()}>粘贴链接</Button>}
        </div>
      </Blankslate>
    </div>
  );
}
