import { Checkbox, ConfirmationDialog } from "@primer/react";
import { useEffect, useState } from "react";
import { useApp } from "../store/useApp";
import { nameOf } from "../lib/format";
import { strings } from "../lib/strings";

/** ConfirmDialog asks before a download and its file are deleted. */
export function ConfirmDialog() {
  const ids = useApp((s) => s.confirmIds);
  const downloads = useApp((s) => s.downloads);
  const cancel = useApp((s) => s.cancelRemove);
  const remove = useApp((s) => s.remove);
  const [deleteFile, setDeleteFile] = useState(true);

  useEffect(() => {
    setDeleteFile(true);
  }, [ids]);

  if (!ids || ids.length === 0) return null;
  const first = ids[0] !== undefined ? downloads[ids[0]] : undefined;
  const title = ids.length > 1 ? strings.removeMany(ids.length) : first ? strings.removeOne(nameOf(first)) : "";

  return (
    <ConfirmationDialog
      title={strings.removeTitle}
      confirmButtonContent={strings.confirm}
      cancelButtonContent={strings.cancel}
      confirmButtonType="danger"
      onClose={(gesture) => {
        if (gesture === "confirm") void remove(ids, deleteFile);
        else cancel();
      }}
    >
      <p style={{ margin: 0 }}>{title}</p>
      <label style={{ display: "flex", alignItems: "center", gap: "var(--base-size-8)", marginTop: "var(--base-size-12)" }}>
        <Checkbox checked={deleteFile} onChange={(event) => setDeleteFile(event.target.checked)} />
        <span>{strings.removeFile}</span>
      </label>
    </ConfirmationDialog>
  );
}
