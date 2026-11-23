interface Props {
  page: number; // 0-based
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
}

export default function Pagination({ page, pageSize, total, onPageChange }: Props) {
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const first = total === 0 ? 0 : page * pageSize + 1;
  const last = Math.min(total, (page + 1) * pageSize);

  return (
    <nav className="pagination" aria-label="Pagination">
      <span className="muted">
        {first}–{last} of {total}
      </span>
      <div className="pager">
        <button onClick={() => onPageChange(0)} disabled={page === 0} aria-label="First page">«</button>
        <button onClick={() => onPageChange(page - 1)} disabled={page === 0}>Previous</button>
        <span>
          Page {page + 1} of {pageCount}
        </span>
        <button onClick={() => onPageChange(page + 1)} disabled={page >= pageCount - 1}>Next</button>
        <button onClick={() => onPageChange(pageCount - 1)} disabled={page >= pageCount - 1} aria-label="Last page">»</button>
      </div>
    </nav>
  );
}
