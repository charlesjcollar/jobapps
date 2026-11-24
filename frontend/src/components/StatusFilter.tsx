import { useEffect, useRef, useState } from "react";
import { STATUSES, STATUS_LABELS, type JobStatus } from "../types";

interface Props {
  selected: JobStatus[]; // empty = all
  onChange: (selected: JobStatus[]) => void;
}

/** "Status: All" button that opens a checkbox list. Closes on outside click or Escape. */
export default function StatusFilter({ selected, onChange }: Props) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: PointerEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpen(false);
        buttonRef.current?.focus();
      }
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  const toggle = (status: JobStatus) => {
    const next = selected.includes(status) ? selected.filter((s) => s !== status) : [...selected, status];
    onChange(STATUSES.filter((s) => next.includes(s))); // keep workflow order
  };

  const summary =
    selected.length === 0 ? "All" : selected.length === 1 ? STATUS_LABELS[selected[0]] : `${selected.length} selected`;

  return (
    <div className="dropdown" ref={ref}>
      <button
        ref={buttonRef}
        className={`button filter-control${selected.length ? " active" : ""}`}
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="true"
        aria-expanded={open}
      >
        Status: {summary} <span className="caret">▾</span>
      </button>
      {open && (
        <div className="dropdown-menu">
          {STATUSES.map((s) => (
            <label key={s} className="dropdown-option">
              <input type="checkbox" checked={selected.includes(s)} onChange={() => toggle(s)} />
              {STATUS_LABELS[s]}
            </label>
          ))}
          <div className="dropdown-footer">
            <button className="link-button" onClick={() => onChange([])} disabled={selected.length === 0}>
              Show all
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
