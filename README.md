# jobapps

A personal app for tracking job postings I may apply to.

> [!WARNING]
> **Local use only. Not for production.** There is no authentication, credentials are hardcoded,
> and the API accepts requests from any origin. Don't deploy this or expose it to a network.

The code is public, but personal data (database contents, saved postings, resumes, cover letters)
is kept out of the repo by `.gitignore` and Docker volumes.

## Architecture

```
┌────────────┐   GraphQL (HTTP + SSE)   ┌──────────────────┐            ┌──────────┐
│  frontend  │ ───────────────────────▶ │       bff        │ ─────────▶ │    db    │
│ React + TS │ ◀─── scan progress ───── │       (Go)       │            │ Postgres │
└────────────┘                          │  scan plugins ───┼──▶ company job boards
                                        └──────────────────┘
```

Clicking **Rescan job postings** in the UI starts a scan in the bff. The bff runs one plugin per
configured company, saves any postings it hasn't seen before, and streams progress back to the UI.

| Part | Folder | Description |
|------|--------|-------------|
| Database | [`db/`](db/README.md) | PostgreSQL + Flyway migrations, plus a file archive of saved postings and submitted resumes/cover letters |
| Frontend | [`frontend/`](frontend/README.md) | Simple web UI (React + TypeScript) |
| BFF | [`bff/`](bff/README.md) | GraphQL API (Go), including the job scanner and its per-company plugins |

## Plugins

Job postings come from **plugins**, one per company. A plugin knows how to read that company's
careers site and which postings to keep. When you click **Rescan job postings**, the bff runs every
plugin, up to 8 at a time, and saves the postings that pass each plugin's filters. Postings that are
already saved are skipped, so a rescan never overwrites your edits.

- **One folder per company.** Each plugin lives in `bff/plugins/<company>/`. Adding a company means
  adding a folder; removing one means deleting it and rebuilding (`docker compose up -d --build`).
- **Filters are per plugin.** Every company formats postings differently, so each plugin keeps its
  own filter settings (titles, levels, location, years of experience, ...) in a `Criteria` block at
  the top of its main file.
- **Shared code per job-board system.** Many companies host their careers page on the same system
  (Eightfold, Greenhouse, Lever, ...). The code for talking to each system lives in
  `bff/plugins/shared/` and is reused across companies.
- **Polite and fault-tolerant.** Plugins space out their requests, back off when a site
  rate-limits, and fail cleanly. One broken plugin doesn't stop the others, and its error shows up
  in the UI.
- **Testable on their own.** `go run ./cmd/plugin -v <company>` (from `bff/`) runs one plugin
  against the live site and prints what it keeps and why it rejected the rest, without touching the
  database.

Current plugins: Netflix.

> [!NOTE]
> **The plugins are tuned to my own job history and preferences**: which companies are included,
> and which roles, seniority levels, locations and experience requirements each one accepts. If
> you use this project, expect to edit each plugin's `Criteria`, and the set of companies, to fit
> your own search.

See [`bff/README.md`](bff/README.md#plugins) for how to write, test, change, and remove plugins.

## Status

Done: database, frontend, bff, scan orchestration, first company plugin (Netflix). Next: more
company plugins.

## Getting started

```sh
docker compose up -d
```

Then open http://localhost:3000.

This starts the database, applies migrations, and runs the BFF (http://localhost:8080/graphql) and
the frontend.
