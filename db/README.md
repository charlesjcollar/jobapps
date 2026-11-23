# db

PostgreSQL database for job postings, with schema managed by [Flyway](https://flywaydb.org/).

## Running locally

From the repo root:

```sh
docker compose up -d
```

This starts Postgres (`db`), then runs a one-shot `flyway` container that applies any pending
migrations and exits. Data persists in the `db-data` Docker volume.

Connect with:

```sh
docker compose exec db psql -U jobapps -d jobapps
```

To wipe the database and start fresh: `docker compose down -v`.

## Migrations

Migrations live in [`migrations/`](migrations/) and follow Flyway's naming convention:
`V<version>__<description>.sql` (e.g. `V2__add_salary_column.sql`). Never edit a migration that has
already been applied — add a new one instead.

## Schema

### `jobs`

| Column | Type | Notes |
|--------|------|-------|
| `id` | `BIGINT` | Primary key, auto-generated |
| `company_name` | `TEXT` | |
| `job_title` | `TEXT` | |
| `posting_id` | `TEXT` | The company's ID for the posting |
| `link` | `TEXT` | URL to the posting |
| `posted_date` | `DATE` | Nullable — not always available |
| `retrieved_date` | `TIMESTAMPTZ` | Defaults to insert time |
| `status` | `TEXT` | See below; defaults to `NEW` |
| `last_updated` | `TIMESTAMPTZ` | Set automatically by trigger on every update |
| `notes` | `TEXT` | Nullable, free-form |
| `contact_name` | `TEXT` | Nullable. Recruiter or hiring manager, once the company reaches out |
| `contact_email` | `TEXT` | Nullable |
| `contact_phone` | `TEXT` | Nullable |
| `salary_range` | `TEXT` | Nullable, free text as the posting states it (e.g. `$150k–$190k`) |
| `location` | `TEXT` | Nullable, free text (e.g. `Remote (US)`, `Seattle, WA`) |

`(company_name, posting_id)` is unique so scans can skip postings already saved.

**Statuses:** `NEW`, `INTERESTED`, `APPLIED`, `INTERVIEWING`, `OFFER`, `USER_REJECTED`, `COMPANY_REJECTED`, `EXPIRED` (posting came down before I applied)

## Posting archive

Alongside the database, `db/` holds a file archive with one folder per posting:

```
db/
├── migrations/
├── Acme Corp/
│   └── REQ-12345/
│       ├── posting.html        # rendered copy of the job posting page
│       ├── resume.pdf          # resume submitted for this job
│       └── cover_letter.pdf    # cover letter, if one was sent
└── Another Company/
    └── 98765/
        └── ...
```

- The path is `db/<company_name>/<posting_id>/`, using the same values as the `jobs` row, so a row
  maps straight to its folder.
- The rendered posting is saved by hand for now. Whether scan plugins save it automatically is
  undecided.
- The resume and cover letter are added when applying, so there is a record of exactly what was
  submitted.

Postings often come down before interviews start. The archive keeps the full job description, along
with the resume and cover letter that went with the application.

These folders hold personal documents, so `.gitignore` excludes everything in `db/` except
`migrations/` and `README.md`.
