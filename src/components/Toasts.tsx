import { Banner } from "@primer/react";
import { useApp } from "../store/useApp";

/** Toasts are the short messages at the bottom right, drawn with Primer's
 * Banner. */
export function Toasts() {
  const toasts = useApp((s) => s.toasts);
  const dismiss = useApp((s) => s.dismissToast);
  return (
    <div className="toasts" role="status" aria-live="polite">
      {toasts.map((toast) => (
        <Banner
          key={toast.id}
          className="toast"
          variant={toast.kind === "error" ? "critical" : toast.kind === "success" ? "success" : "info"}
          title={toast.text}
          hideTitle={false}
          aria-label={toast.text}
          onDismiss={() => dismiss(toast.id)}
        />
      ))}
    </div>
  );
}
