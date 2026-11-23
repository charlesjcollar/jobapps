import { useCallback, useEffect, useState } from "react";
import { listJobs, type JobPage } from "./api";
import type { Job, JobSort } from "./types";
import JobsTable from "./components/JobsTable";
import Pagination from "./components/Pagination";
import ScanModal from "./components/ScanModal";
import Toasts from "./components/Toasts";
import { useScan } from "./useScan";

const PAGE_SIZE = 50;
const DEFAULT_SORT: JobSort = { field: "RETRIEVED_DATE", direction: "DESC" };

export default function App() {
  const [sort, setSort] = useState<JobSort>(DEFAULT_SORT);
  const [page, setPage] = useState(0);
  const [data, setData] = useState<JobPage | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [reloadKey, setReloadKey] = useState(0);

  // Reload the table when a scan finishes, since it may have added jobs.
  const scan = useScan(useCallback(() => setReloadKey((k) => k + 1), []));
  const scanning = scan.scan?.running ?? false;
  const scanDone = scan.scan?.plugins.filter((p) => p.status === "SUCCEEDED" || p.status === "FAILED").length ?? 0;

  useEffect(() => {
    let stale = false;
    setLoading(true);
    listJobs(sort, PAGE_SIZE, page * PAGE_SIZE)
      .then((d) => {
        if (stale) return;
        setData(d);
        setError(null);
      })
      .catch((err) => {
        if (!stale) setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        if (!stale) setLoading(false);
      });
    return () => {
      stale = true;
    };
  }, [sort, page, reloadKey]);

  const changeSort = (s: JobSort) => {
    setSort(s);
    setPage(0);
  };

  const replaceJob = (job: Job) =>
    setData((d) => d && { ...d, items: d.items.map((j) => (j.id === job.id ? job : j)) });

  return (
    <div className="app">
      <header>
        <h1>Job Apps</h1>
        {loading && data && <span className="muted">Loading…</span>}
        <button
          className="button primary header-action"
          onClick={scanning ? scan.openModal : scan.start}
          disabled={scan.starting}
        >
          {scanning ? `Scanning… ${scanDone}/${scan.scan!.plugins.length}` : "Rescan job postings"}
        </button>
      </header>

      {scan.modalOpen && <ScanModal scan={scan.scan} onClose={scan.closeModal} />}
      <Toasts toasts={scan.toasts} onDismiss={scan.dismissToast} />

      {error && <div className="banner error">Couldn't load jobs: {error}</div>}

      {!data && loading && <p className="muted">Loading…</p>}

      {data && data.totalCount === 0 && (
        <p className="empty muted">No jobs yet. Click “Rescan job postings” to look for new ones.</p>
      )}

      {data && data.totalCount > 0 && (
        <>
          <div className="table-wrap">
            <JobsTable jobs={data.items} sort={sort} onSortChange={changeSort} onJobChange={replaceJob} />
          </div>
          <Pagination page={page} pageSize={PAGE_SIZE} total={data.totalCount} onPageChange={setPage} />
        </>
      )}
    </div>
  );
}
