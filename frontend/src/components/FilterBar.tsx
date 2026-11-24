import { isFiltered, type Filters } from "../filters";
import StatusFilter from "./StatusFilter";

interface Props {
  filters: Filters;
  onChange: (filters: Filters) => void;
  onClear: () => void;
}

export default function FilterBar({ filters, onChange, onClear }: Props) {
  const set = (patch: Partial<Filters>) => onChange({ ...filters, ...patch });

  return (
    <div className="filter-bar">
      <input
        type="search"
        className="filter-control filter-text"
        placeholder="Company"
        aria-label="Filter by company"
        value={filters.company}
        onChange={(e) => set({ company: e.target.value })}
      />
      <input
        type="search"
        className="filter-control filter-text"
        placeholder="Title"
        aria-label="Filter by title"
        value={filters.title}
        onChange={(e) => set({ title: e.target.value })}
      />
      <StatusFilter selected={filters.statuses} onChange={(statuses) => set({ statuses })} />
      <label className="filter-check" title="Hide Not pursuing, Rejected, and Expired">
        <input type="checkbox" checked={filters.hideClosed} onChange={(e) => set({ hideClosed: e.target.checked })} />
        Hide closed
      </label>
      {isFiltered(filters) && (
        <button className="link-button" onClick={onClear}>
          Clear filters
        </button>
      )}
    </div>
  );
}
