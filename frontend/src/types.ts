// Mirrors bff/graph/schema.graphqls.

export const STATUSES = [
  "NEW",
  "INTERESTED",
  "APPLIED",
  "INTERVIEWING",
  "OFFER",
  "USER_REJECTED",
  "COMPANY_REJECTED",
  "EXPIRED",
] as const;

export type JobStatus = (typeof STATUSES)[number];

export const STATUS_LABELS: Record<JobStatus, string> = {
  NEW: "New",
  INTERESTED: "Interested",
  APPLIED: "Applied",
  INTERVIEWING: "Interviewing",
  OFFER: "Offer",
  USER_REJECTED: "Not pursuing",
  COMPANY_REJECTED: "Rejected",
  EXPIRED: "Expired",
};

export interface Job {
  id: string;
  companyName: string;
  jobTitle: string;
  postingId: string;
  link: string;
  postedDate: string | null;
  retrievedDate: string;
  status: JobStatus;
  lastUpdated: string;
  notes: string | null;
  contactName: string | null;
  contactEmail: string | null;
  contactPhone: string | null;
  salaryRange: string | null;
  location: string | null;
}

export type JobSortField =
  | "COMPANY_NAME"
  | "JOB_TITLE"
  | "POSTED_DATE"
  | "RETRIEVED_DATE"
  | "STATUS"
  | "LAST_UPDATED";

export type SortDirection = "ASC" | "DESC";

export interface JobSort {
  field: JobSortField;
  direction: SortDirection;
}

/** Fields the UI can edit. null clears the value. */
export type JobUpdate = Partial<
  Pick<Job, "status" | "notes" | "contactName" | "contactEmail" | "contactPhone" | "salaryRange" | "location">
>;

export type PluginRunStatus = "PENDING" | "RUNNING" | "SUCCEEDED" | "FAILED";

export interface PluginRun {
  name: string;
  status: PluginRunStatus;
  startedAt: string | null;
  finishedAt: string | null;
  jobsFound: number;
  jobsAdded: number;
  jobsExpired: number;
  error: string | null;
}

export interface Scan {
  id: string;
  startedAt: string;
  finishedAt: string | null;
  running: boolean;
  plugins: PluginRun[];
}

/** Narrows the jobs list. Omitted fields don't filter; set fields are combined with AND. */
export interface JobFilter {
  /** Only these statuses. An empty list matches nothing, so omit it instead. */
  statuses?: JobStatus[];
  excludedStatuses?: JobStatus[];
  /** Case-insensitive substring. */
  companyName?: string;
  /** Case-insensitive substring. */
  jobTitle?: string;
}
