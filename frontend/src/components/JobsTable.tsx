import type { Job, JobSort, JobSortField } from "../types";
import JobRow from "./JobRow";

interface Column {
  label: string;
  sortField?: JobSortField;
  className?: string;
}

const COLUMNS: Column[] = [
  { label: "Company", sortField: "COMPANY_NAME" },
  { label: "Title", sortField: "JOB_TITLE" },
  { label: "Posting ID" },
  { label: "Posted", sortField: "POSTED_DATE" },
  { label: "Added", sortField: "RETRIEVED_DATE" },
  { label: "Status", sortField: "STATUS" },
  { label: "Contact" },
  { label: "Email" },
  { label: "Phone" },
  { label: "Notes", className: "notes" },
  { label: "Updated", sortField: "LAST_UPDATED" },
  { label: "", className: "save-state" },
];

// Dates sort newest first on the first click; everything else A→Z.
const DESC_FIRST: JobSortField[] = ["POSTED_DATE", "RETRIEVED_DATE", "LAST_UPDATED"];

interface Props {
  jobs: Job[];
  sort: JobSort;
  onSortChange: (sort: JobSort) => void;
  onJobChange: (job: Job) => void;
}

export default function JobsTable({ jobs, sort, onSortChange, onJobChange }: Props) {
  const clickHeader = (field: JobSortField) => {
    if (field === sort.field) {
      onSortChange({ field, direction: sort.direction === "ASC" ? "DESC" : "ASC" });
    } else {
      onSortChange({ field, direction: DESC_FIRST.includes(field) ? "DESC" : "ASC" });
    }
  };

  return (
    <table>
      <thead>
        <tr>
          {COLUMNS.map((col) => {
            if (!col.sortField) {
              return <th key={col.label} className={col.className}>{col.label}</th>;
            }
            const active = col.sortField === sort.field;
            return (
              <th
                key={col.label}
                className={col.className}
                aria-sort={active ? (sort.direction === "ASC" ? "ascending" : "descending") : "none"}
              >
                <button className={`sort ${active ? "active" : ""}`} onClick={() => clickHeader(col.sortField!)}>
                  {col.label}
                  <span className="sort-arrow" aria-hidden>
                    {active ? (sort.direction === "ASC" ? "▲" : "▼") : "↕"}
                  </span>
                </button>
              </th>
            );
          })}
        </tr>
      </thead>
      <tbody>
        {jobs.map((job) => (
          <JobRow key={job.id} job={job} onChange={onJobChange} />
        ))}
      </tbody>
    </table>
  );
}
