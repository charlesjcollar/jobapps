import { useRef, useState } from "react";
import { updateJob } from "../api";
import { STATUSES, STATUS_LABELS, type Job, type JobStatus, type JobUpdate } from "../types";
import { formatDate, formatDateTime, formatTimestampDate } from "../format";
import AutoSaveField from "./AutoSaveField";

type SaveState = { kind: "idle" } | { kind: "saving" } | { kind: "saved" } | { kind: "error"; message: string };

interface Props {
  job: Job;
  onChange: (job: Job) => void;
}

export default function JobRow({ job, onChange }: Props) {
  const [saveState, setSaveState] = useState<SaveState>({ kind: "idle" });
  // Only the newest save's response is applied, in case responses arrive out of order.
  const latestSave = useRef(0);

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
      </td>
      <td className="mono">{job.postingId}</td>
      <td className="date">{job.postedDate ? formatDate(job.postedDate) : <span className="muted">—</span>}</td>
      <td className="date" title={formatDateTime(job.retrievedDate)}>{formatTimestampDate(job.retrievedDate)}</td>
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
      <td className="date" title={formatDateTime(job.lastUpdated)}>{formatTimestampDate(job.lastUpdated)}</td>
      <td className="save-state">
        {saveState.kind === "saving" && <span className="muted">Saving…</span>}
        {saveState.kind === "saved" && <span className="ok">Saved</span>}
        {saveState.kind === "error" && (
          <span className="error" title={saveState.message}>
            Not saved
          </span>
        )}
      </td>
    </tr>
  );
}
