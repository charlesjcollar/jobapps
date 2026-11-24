// Package store holds all SQL access to the jobs table.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// ErrDuplicate is returned when an update would give a job the same
// (company_name, posting_id) as another job.
var ErrDuplicate = errors.New("another job already has this company name and posting ID")

// Statuses in workflow order. Must match the CHECK constraint in db/migrations.
var Statuses = []string{"NEW", "INTERESTED", "APPLIED", "INTERVIEWING", "OFFER", "USER_REJECTED", "COMPANY_REJECTED", "EXPIRED"}

const dateLayout = "2006-01-02"

type Job struct {
	ID            int64
	CompanyName   string
	JobTitle      string
	PostingID     string
	Link          string
	PostedDate    *time.Time
	RetrievedDate time.Time
	Status        string
	LastUpdated   time.Time
	Notes         *string
	ContactName   *string
	ContactEmail  *string
	ContactPhone  *string
	SalaryRange   *string
	Location      *string
}

const jobColumns = `id, company_name, job_title, posting_id, link, posted_date, retrieved_date, status, last_updated, notes,
	contact_name, contact_email, contact_phone, salary_range, location`

type scanner interface{ Scan(dest ...any) error }

func scanJob(row scanner) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.CompanyName, &j.JobTitle, &j.PostingID, &j.Link,
		&j.PostedDate, &j.RetrievedDate, &j.Status, &j.LastUpdated, &j.Notes,
		&j.ContactName, &j.ContactEmail, &j.ContactPhone, &j.SalaryRange, &j.Location)
	return j, err
}

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Get returns the job with the given id, or nil if there is none.
func (s *Store) Get(ctx context.Context, id int64) (*Job, error) {
	j, err := scanJob(s.db.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// SortField values. Anything else falls back to SortRetrievedDate.
const (
	SortCompanyName   = "COMPANY_NAME"
	SortJobTitle      = "JOB_TITLE"
	SortPostedDate    = "POSTED_DATE"
	SortRetrievedDate = "RETRIEVED_DATE"
	SortStatus        = "STATUS"
	SortLastUpdated   = "LAST_UPDATED"
)

type ListParams struct {
	Statuses         []string // nil = no filter; empty = match nothing
	ExcludedStatuses []string
	CompanyName      string // case-insensitive substring; "" = no filter
	JobTitle         string
	SortField        string
	Desc             bool
	Limit            int
	Offset           int
}

// List returns one page of jobs and the total number matching the filter.
func (s *Store) List(ctx context.Context, p ListParams) ([]Job, int, error) {
	where, args := buildWhere(p)

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM jobs`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT ` + jobColumns + ` FROM jobs` + where + buildOrderBy(p) +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	rows, err := s.db.QueryContext(ctx, query, append(args, p.Limit, p.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	jobs := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, j)
	}
	return jobs, total, rows.Err()
}

func buildWhere(p ListParams) (string, []any) {
	var conds []string
	var args []any
	add := func(cond string, arg any) {
		args = append(args, arg)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}

	if p.Statuses != nil {
		add(`status = ANY($%d)`, pq.Array(p.Statuses))
	}
	if len(p.ExcludedStatuses) > 0 {
		add(`NOT (status = ANY($%d))`, pq.Array(p.ExcludedStatuses))
	}
	if p.CompanyName != "" {
		add(`company_name ILIKE '%%' || $%d || '%%'`, escapeLike(p.CompanyName))
	}
	if p.JobTitle != "" {
		add(`job_title ILIKE '%%' || $%d || '%%'`, escapeLike(p.JobTitle))
	}

	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func buildOrderBy(p ListParams) string {
	dir := " ASC"
	if p.Desc {
		dir = " DESC"
	}

	var expr string
	switch p.SortField {
	case SortCompanyName:
		expr = "lower(company_name)" + dir
	case SortJobTitle:
		expr = "lower(job_title)" + dir
	case SortPostedDate:
		expr = "posted_date" + dir + " NULLS LAST"
	case SortStatus:
		expr = "array_position(ARRAY['" + strings.Join(Statuses, "','") + "'], status)" + dir
	case SortLastUpdated:
		expr = "last_updated" + dir
	default:
		expr = "retrieved_date" + dir
	}
	// Tie-break on id so paging is stable.
	return " ORDER BY " + expr + ", id" + dir
}

// escapeLike escapes LIKE wildcards so user input matches literally.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Update describes the changes to make to one job. Nil pointers are left unchanged.
// For the nullable columns, Set* / a non-nil Nullable says whether to write the
// value (a nil value clears it).
type Update struct {
	CompanyName *string
	JobTitle    *string
	PostingID   *string
	Link        *string
	Status      *string

	SetPostedDate bool
	PostedDate    *time.Time

	Notes        *Nullable
	ContactName  *Nullable
	ContactEmail *Nullable
	ContactPhone *Nullable
	SalaryRange  *Nullable
	Location     *Nullable
}

// Nullable is a value for a nullable text column. Value nil writes NULL.
type Nullable struct{ Value *string }

// UpdateJob applies u to the job with the given id and returns the updated job,
// or nil if there is no such job.
func (s *Store) UpdateJob(ctx context.Context, id int64, u Update) (*Job, error) {
	var sets []string
	var args []any
	set := func(col string, val any) {
		args = append(args, val)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}

	if u.CompanyName != nil {
		set("company_name", *u.CompanyName)
	}
	if u.JobTitle != nil {
		set("job_title", *u.JobTitle)
	}
	if u.PostingID != nil {
		set("posting_id", *u.PostingID)
	}
	if u.Link != nil {
		set("link", *u.Link)
	}
	if u.Status != nil {
		set("status", *u.Status)
	}
	if u.SetPostedDate {
		set("posted_date", formatDate(u.PostedDate))
	}
	for _, c := range []struct {
		col string
		v   *Nullable
	}{
		{"notes", u.Notes},
		{"contact_name", u.ContactName},
		{"contact_email", u.ContactEmail},
		{"contact_phone", u.ContactPhone},
		{"salary_range", u.SalaryRange},
		{"location", u.Location},
	} {
		if c.v != nil {
			set(c.col, c.v.Value)
		}
	}

	if len(sets) == 0 {
		return s.Get(ctx, id)
	}

	args = append(args, id)
	query := fmt.Sprintf(`UPDATE jobs SET %s WHERE id = $%d RETURNING %s`,
		strings.Join(sets, ", "), len(args), jobColumns)
	j, err := scanJob(s.db.QueryRowContext(ctx, query, args...))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	case isUniqueViolation(err):
		return nil, ErrDuplicate
	case err != nil:
		return nil, err
	}
	return &j, nil
}

type NewJob struct {
	CompanyName string
	JobTitle    string
	PostingID   string
	Link        string
	PostedDate  *time.Time
	SalaryRange *string // optional
	Location    *string // optional
}

// InsertNew saves the jobs whose (company_name, posting_id) isn't already stored,
// skipping repeats within the batch, and returns the ones that were saved.
func (s *Store) InsertNew(ctx context.Context, jobs []NewJob) ([]Job, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO jobs (company_name, job_title, posting_id, link, posted_date, salary_range, location)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (company_name, posting_id) DO NOTHING
		RETURNING `+jobColumns)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	inserted := []Job{}
	for _, n := range jobs {
		j, err := scanJob(stmt.QueryRowContext(ctx, n.CompanyName, n.JobTitle, n.PostingID, n.Link, formatDate(n.PostedDate),
			blankToNil(n.SalaryRange), blankToNil(n.Location)))
		if errors.Is(err, sql.ErrNoRows) {
			continue // already saved, or repeated earlier in this batch
		}
		if err != nil {
			return nil, err
		}
		inserted = append(inserted, j)
	}
	return inserted, tx.Commit()
}

// CompanyJobs returns the company's saved jobs whose status is one of statuses.
func (s *Store) CompanyJobs(ctx context.Context, company string, statuses []string) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+jobColumns+` FROM jobs
		WHERE company_name = $1 AND status = ANY($2) ORDER BY id`, company, pq.Array(statuses))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// MarkExpired sets the given jobs to EXPIRED, but only those whose status is
// still one of onlyIf, so a status changed in the meantime is left alone. It
// returns the jobs it changed.
func (s *Store) MarkExpired(ctx context.Context, ids []int64, onlyIf []string) ([]Job, error) {
	if len(ids) == 0 {
		return []Job{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `UPDATE jobs SET status = 'EXPIRED'
		WHERE id = ANY($1) AND status = ANY($2)
		RETURNING `+jobColumns, pq.Array(ids), pq.Array(onlyIf))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// formatDate sends dates as YYYY-MM-DD strings so time zones can't shift them.
func formatDate(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(dateLayout)
	return &s
}

func blankToNil(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	v := strings.TrimSpace(*s)
	return &v
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
