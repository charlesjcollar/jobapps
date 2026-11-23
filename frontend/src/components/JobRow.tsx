import { useEffect, useRef, useState } from "react";
import { updateJob } from "../api";
import { STATUSES, STATUS_LABELS, type Job, type JobStatus, type JobUpdate } from "../types";
import { formatDate, formatDateTime, shortDate } from "../format";
import AutoSaveField from "./AutoSaveField";

type SaveState = { kind: "idle" } | { kind: "saving" } | { kind: "saved" } | { kind: "error"; message: string };

const SAVED_DISPLAY_MS = 2000;

interface Props {
  job: Job;
  onChange: (job: Job) => void;
}

export default function JobRow({ job, onChange }: Props) {
  const [saveState, setSaveState] = useState<SaveState>({ kind: "idle" });
  // Only the newest save's response is applied, in case responses arrive out of order.
  const latestSave = useRef(0);

  // "Saved" shows briefly, then the Updated column goes back to the date.
  useEffect(() => {
    if (saveState.kind !== "saved") return;
    const t = window.setTimeout(() => setSaveState({ kind: "idle" }), SAVED_DISPLAY_MS);
    return () => window.clearTimeout(t);
  }, [saveState]);

  const save = async (update: JobUpdate, previous?: Job) => {
    const seq = ++latestSave.current;
    setSaveState({ kind: "saving" });
    try {
      const updated = await updateJob(job.id, update);
      if (seq !== latestSave.current) return;
      onChange(updated);
      setSaveState({ kind: "saved" });
    } catch (err) {
      if (seq !== latestSave.current) return;
      if (previous) onChange(previous);
      setSaveState({ kind: "error", message: err instanceof Error ? err.message : String(err) });
    }
  };

  const setStatus = (status: JobStatus) => {
    const previous = job;
    onChange({ ...job, status }); // optimistic; reverted if the save fails
    save({ status }, previous);
  };

  return (
    <tr>
      <td className="company">{job.companyName}</td>
      <td className="title">
        <a href={job.link} target="_blank" rel="noopener noreferrer" title={job.link}>
          {job.jobTitle}
          <span aria-hidden className="external">↗</span>
        </a>
        <div className="posting-id mono muted" title="Posting ID">{job.postingId}</div>
      </td>
      <td className="location">
        <AutoSaveField label="Location" placeholder="Location" multiline value={job.location} onSave={(v) => save({ location: v })} />
      </td>
      <td className="salary">
        <AutoSaveField label="Salary range" placeholder="Salary" multiline value={job.salaryRange} onSave={(v) => save({ salaryRange: v })} />
      </td>
      <td className="date" title={job.postedDate ? formatDate(job.postedDate) : undefined}>
        {job.postedDate ? shortDate(job.postedDate) : <span className="muted">—</span>}
      </td>
      <td className="date" title={formatDateTime(job.retrievedDate)}>{shortDate(job.retrievedDate)}</td>
      <td>
        <select
          value={job.status}
          data-status={job.status}
          aria-label="Status"
          onChange={(e) => setStatus(e.target.value as JobStatus)}
        >
          {STATUSES.map((s) => (
            <option key={s} value={s}>
              {STATUS_LABELS[s]}
            </option>
          ))}
        </select>
      </td>
      <td className="contact">
        <AutoSaveField label="Contact name" placeholder="Name" value={job.contactName} onSave={(v) => save({ contactName: v })} />
      </td>
      <td className="contact">
        <AutoSaveField label="Contact email" placeholder="Email" type="email" value={job.contactEmail} onSave={(v) => save({ contactEmail: v })} />
      </td>
      <td className="contact">
        <AutoSaveField label="Contact phone" placeholder="Phone" type="tel" value={job.contactPhone} onSave={(v) => save({ contactPhone: v })} />
      </td>
      <td className="notes">
        <AutoSaveField label="Notes" placeholder="Notes" multiline value={job.notes} onSave={(v) => save({ notes: v })} />
      </td>
      <td className="date updated" title={formatDateTime(job.lastUpdated)} aria-live="polite">
        {saveState.kind === "idle" && shortDate(job.lastUpdated)}
        {saveState.kind === "saving" && <span className="muted">Saving…</span>}
        {saveState.kind === "saved" && <span className="ok">Saved ✓</span>}
        {saveState.kind === "error" && (
          <span className="error" title={saveState.message}>
            Not saved
          </span>
        )}
      </td>
    </tr>
  );
}
