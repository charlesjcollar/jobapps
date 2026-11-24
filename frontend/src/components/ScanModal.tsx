import { useEffect } from "react";
import type { PluginRun, Scan } from "../types";

interface Props {
  scan: Scan | null;
  onClose: () => void;
}

export default function ScanModal({ scan, onClose }: Props) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const plugins = scan?.plugins ?? [];
  const done = plugins.filter((p) => p.status === "SUCCEEDED" || p.status === "FAILED").length;
  const failed = plugins.filter((p) => p.status === "FAILED").length;
  const added = plugins.reduce((n, p) => n + p.jobsAdded, 0);
  const expired = plugins.reduce((n, p) => n + p.jobsExpired, 0);
  const running = !scan || scan.running;

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" role="dialog" aria-modal="true" aria-labelledby="scan-title" onClick={(e) => e.stopPropagation()}>
        <h2 id="scan-title">{running ? "Scanning job postings" : "Scan complete"}</h2>

        {!scan && <p className="muted">Starting…</p>}

        {scan && plugins.length === 0 && (
          <p className="muted">No companies are configured to scan yet.</p>
        )}

        {plugins.length > 0 && (
          <>
            <div className="progress" role="progressbar" aria-valuemin={0} aria-valuemax={plugins.length} aria-valuenow={done}>
              <div className="progress-bar" style={{ width: `${(done / plugins.length) * 100}%` }} />
            </div>
            <p className="muted progress-label">
              {done} of {plugins.length} companies scanned
              {failed > 0 && <span className="error"> · {failed} failed</span>}
            </p>

            <ul className="plugin-list">
              {plugins.map((p) => (
                <PluginRow key={p.name} run={p} />
              ))}
            </ul>
          </>
        )}

        <footer>
          {scan && !running && plugins.length > 0 && (
            <span>
              {added === 0 ? "No new jobs" : `${added} new job${added === 1 ? "" : "s"} added`}
              {expired > 0 && ` · ${expired} expired`}
            </span>
          )}
          <button className="button" onClick={onClose}>
            {running ? "Run in background" : "Close"}
          </button>
        </footer>
      </div>
    </div>
  );
}

function PluginRow({ run }: { run: PluginRun }) {
  return (
    <li className={`plugin ${run.status.toLowerCase()}`}>
      <span className="plugin-icon" aria-hidden>
        {run.status === "PENDING" && "○"}
        {run.status === "RUNNING" && <span className="spinner" />}
        {run.status === "SUCCEEDED" && "✓"}
        {run.status === "FAILED" && "✕"}
      </span>
      <span className="plugin-name">{run.name}</span>
      <span className="plugin-detail">
        {run.status === "PENDING" && <span className="muted">Waiting</span>}
        {run.status === "RUNNING" && <span className="muted">Scanning…</span>}
        {run.status === "SUCCEEDED" && (
          <span className="muted">
            <strong className={run.jobsAdded > 0 ? "ok" : undefined}>{run.jobsAdded} new</strong>
            {run.jobsExpired > 0 && <> · {run.jobsExpired} expired</>} · {run.jobsFound} found
          </span>
        )}
        {run.status === "FAILED" && <span className="error">Failed</span>}
      </span>
      {run.error && <pre className="plugin-error">{run.error}</pre>}
    </li>
  );
}
