import { useCallback, useEffect, useRef, useState } from "react";
import { listJobs, type JobPage } from "./api";
import { isFiltered, loadFilters, NO_FILTERS, saveFilters, toJobFilter, type Filters } from "./filters";
import type { Job, JobFilter, JobSort } from "./types";
import FilterBar from "./components/FilterBar";
import JobsTable from "./components/JobsTable";
import Pagination from "./components/Pagination";
import ScanModal from "./components/ScanModal";
import Toasts from "./components/Toasts";
import { useScan } from "./useScan";

const PAGE_SIZE = 50;
const DEFAULT_SORT: JobSort = { field: "RETRIEVED_DATE", direction: "DESC" };
const TYPING_DELAY_MS = 300;

const filterKey = (f: Filters) => JSON.stringify(toJobFilter(f));

export default function App() {
  const [sort, setSort] = useState<JobSort>(DEFAULT_SORT);
  const [page, setPage] = useState(0);
  const [data, setData] = useState<JobPage | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [reloadKey, setReloadKey] = useState(0);

  // `filters` is what the filter bar shows; `appliedFilter` (a serialized JobFilter) is what the
  // table was queried with. Typing is debounced; other filter changes apply immediately.
  const [filters, setFilters] = useState<Filters>(loadFilters);
  const [appliedFilter, setAppliedFilter] = useState(() => filterKey(filters));
  const [dataFiltered, setDataFiltered] = useState(false); // whether `data` came from a filtered query
  const appliedRef = useRef(appliedFilter);
  const typingTimer = useRef<number | undefined>(undefined);

  useEffect(() => () => window.clearTimeout(typingTimer.current), []);

  const applyFilters = (f: Filters) => {
    window.clearTimeout(typingTimer.current);
    const key = filterKey(f);
    if (key === appliedRef.current) return;
    appliedRef.current = key;
    setAppliedFilter(key);
    setPage(0);
  };

  const changeFilters = (f: Filters) => {
    const typed = f.company !== filters.company || f.title !== filters.title;
    setFilters(f);
    saveFilters(f);
    if (typed) {
      window.clearTimeout(typingTimer.current);
      typingTimer.current = window.setTimeout(() => applyFilters(f), TYPING_DELAY_MS);
    } else {
      applyFilters(f);
    }
  };

  const clearFilters = () => {
    setFilters(NO_FILTERS);
    saveFilters(NO_FILTERS);
    applyFilters(NO_FILTERS);
  };

  // Reload the table when a scan finishes, since it may have added jobs.
  const scan = useScan(useCallback(() => setReloadKey((k) => k + 1), []));
  const scanning = scan.scan?.running ?? false;
  const scanDone = scan.scan?.plugins.filter((p) => p.status === "SUCCEEDED" || p.status === "FAILED").length ?? 0;

  useEffect(() => {
    let stale = false;
    setLoading(true);
    const filter = JSON.parse(appliedFilter) as JobFilter;
    listJobs(filter, sort, PAGE_SIZE, page * PAGE_SIZE)
      .then((d) => {
        if (stale) return;
        setData(d);
        setDataFiltered(Object.keys(filter).length > 0);
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
  }, [appliedFilter, sort, page, reloadKey]);

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

      {data && (data.totalCount > 0 || dataFiltered || isFiltered(filters)) && (
        <FilterBar filters={filters} onChange={changeFilters} onClear={clearFilters} />
      )}

      {data && data.totalCount === 0 && !dataFiltered && (
        <p className="empty muted">No jobs yet. Click “Rescan job postings” to look for new ones.</p>
      )}

      {data && data.totalCount === 0 && dataFiltered && (
        <p className="empty muted">
          No jobs match these filters.{" "}
          <button className="link-button" onClick={clearFilters}>
            Clear filters
          </button>
        </p>
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
