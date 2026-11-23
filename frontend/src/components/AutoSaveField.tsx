import { useEffect, useRef, useState } from "react";

const SAVE_DELAY_MS = 800;

interface Props {
  value: string | null;
  /** Called with the new value (null when blank) after typing pauses or on blur. */
  onSave: (value: string | null) => void;
  multiline?: boolean;
  type?: "text" | "email" | "tel";
  placeholder?: string;
  label: string;
}

/**
 * A text input that saves itself: after the user stops typing for a moment,
 * and immediately on blur. While focused it ignores outside updates to
 * `value` so a save response never clobbers what's being typed.
 */
export default function AutoSaveField({ value, onSave, multiline, type = "text", placeholder, label }: Props) {
  const [draft, setDraft] = useState(value ?? "");
  const focused = useRef(false);
  const timer = useRef<number | undefined>(undefined);
  const lastSaved = useRef(value ?? "");

  useEffect(() => {
    if (!focused.current) {
      setDraft(value ?? "");
      lastSaved.current = value ?? "";
    }
  }, [value]);

  useEffect(() => () => window.clearTimeout(timer.current), []);

  const save = (text: string) => {
    window.clearTimeout(timer.current);
    if (text === lastSaved.current) return;
    lastSaved.current = text;
    onSave(text.trim() === "" ? null : text);
  };

  const onChange = (text: string) => {
    setDraft(text);
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => save(text), SAVE_DELAY_MS);
  };

  const common = {
    value: draft,
    placeholder,
    "aria-label": label,
    onFocus: () => {
      focused.current = true;
    },
    onBlur: () => {
      focused.current = false;
      save(draft);
    },
  };

  return multiline ? (
    <textarea {...common} rows={2} onChange={(e) => onChange(e.target.value)} />
  ) : (
    <input {...common} type={type} onChange={(e) => onChange(e.target.value)} />
  );
}
