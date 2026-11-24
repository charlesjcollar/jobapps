import { STATUSES, type JobFilter, type JobStatus } from "./types";

/** The filter bar's settings, as the user sees them. */
export interface Filters {
  company: string;
  title: string;
  statuses: JobStatus[]; // empty = all
  hideClosed: boolean;
}

export const NO_FILTERS: Filters = { company: "", title: "", statuses: [], hideClosed: false };

export const CLOSED_STATUSES: JobStatus[] = ["USER_REJECTED", "COMPANY_REJECTED", "EXPIRED"];

export function isFiltered(f: Filters): boolean {
  return f.company.trim() !== "" || f.title.trim() !== "" || f.statuses.length > 0 || f.hideClosed;
}

/** Converts to the API's JobFilter, leaving out unset fields. */
export function toJobFilter(f: Filters): JobFilter {
  const filter: JobFilter = {};
  if (f.company.trim()) filter.companyName = f.company.trim();
  if (f.title.trim()) filter.jobTitle = f.title.trim();
  if (f.statuses.length) filter.statuses = f.statuses;
  if (f.hideClosed) filter.excludedStatuses = CLOSED_STATUSES;
  return filter;
}

// Saved per browser in localStorage. Storage may be unavailable, so every access is guarded.
const STORAGE_KEY = "jobapps.filters";

export function loadFilters(): Filters {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return NO_FILTERS;
    const saved = JSON.parse(raw) as Partial<Filters>;
    return {
      company: typeof saved.company === "string" ? saved.company : "",
      title: typeof saved.title === "string" ? saved.title : "",
      statuses: Array.isArray(saved.statuses)
        ? STATUSES.filter((s) => saved.statuses!.includes(s))
        : [],
      hideClosed: saved.hideClosed === true,
    };
  } catch {
    return NO_FILTERS;
  }
}

export function saveFilters(f: Filters) {
  try {
    if (isFiltered(f)) localStorage.setItem(STORAGE_KEY, JSON.stringify(f));
    else localStorage.removeItem(STORAGE_KEY);
  } catch {
    // Not saved; the filters still work for this session.
  }
}
