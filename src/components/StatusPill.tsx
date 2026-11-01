import { Label } from "@primer/react";
import type { LabelProps } from "@primer/react";
import type { Status } from "../mygo";
import { statusLabel } from "../lib/strings";

const variants: Record<Status, LabelProps["variant"]> = {
  queued: "default",
  downloading: "accent",
  paused: "attention",
  completed: "success",
  error: "danger",
};

/** StatusPill is the colored badge of a download's status; a failure shows
 * its reason on hover. */
export function StatusPill({ status, error }: { status: Status; error?: string }) {
  return (
    <Label variant={variants[status]} size="large" title={error}>
      {statusLabel[status]}
    </Label>
  );
}
