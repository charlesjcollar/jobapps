import type { Toast } from "../useScan";

interface Props {
  toasts: Toast[];
  onDismiss: (id: number) => void;
}

/** Error notifications. They stay until dismissed so the details can be read and copied. */
export default function Toasts({ toasts, onDismiss }: Props) {
  if (toasts.length === 0) return null;
  return (
    <div className="toasts" role="region" aria-label="Notifications">
      {toasts.map((t) => (
        <div key={t.id} className="toast" role="alert">
          <div className="toast-head">
            <strong>{t.title}</strong>
            <button className="toast-close" onClick={() => onDismiss(t.id)} aria-label="Dismiss">
              ×
            </button>
          </div>
          <pre>{t.detail}</pre>
        </div>
      ))}
    </div>
  );
}
