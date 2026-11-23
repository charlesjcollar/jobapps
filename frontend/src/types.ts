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
  Pick<Job, "status" | "notes" | "contactName" | "contactEmail" | "contactPhone">
>;
