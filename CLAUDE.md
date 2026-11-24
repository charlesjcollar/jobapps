# CLAUDE.md

Guidance for Claude Code when working in this repo.

## Project overview

Personal app for managing job postings I may apply to. Three parts:

1. `db/` — Postgres + Flyway migrations
2. `frontend/` — React + TypeScript web UI
3. `bff/` — Go GraphQL API. Also runs scans: one plugin per company (`bff/plugins/`), orchestrated
   by `bff/scan/`, with progress streamed to the frontend over SSE.

Jobs enter the database only through scans. There is one scan plugin per company (folder
`bff/plugins/<company>/`), and I pick the companies by hand (based on culture, ethics, etc.). Expect
dozens, eventually hundreds. Scan history is in memory only, by design.

Each posting also has a folder at `db/<company_name>/<posting_id>/` holding the saved posting page and the resume/cover letter submitted (personal documents, gitignored). See `db/README.md`.

See `README.md` and each folder's `README.md` for details.

## Constraints

- **Local only.** This app never runs in production; it only runs on my machine via `docker compose`.
  Don't add auth, secrets management, TLS, or other production hardening. Hardcoded local credentials
  are fine.
- **The repo is public, my data is not.** The code is published to GitHub, but nothing personal may be
  committed: no database contents or dumps, no files from the posting archive (`db/<company>/<posting_id>/`),
  no resumes or cover letters, no real job data in tests or fixtures (use made-up companies). Database
  data lives only in the `db-data` Docker volume. When adding anything that writes files, make sure
  `.gitignore` covers it.

## General preferences

<!-- Add general coding preferences here (style, comments, testing, commits, etc.) -->

## Database

<!-- DB-specific preferences -->

## Frontend (React / TypeScript)

<!-- Frontend-specific preferences -->

## BFF (Go)

<!-- Go-specific preferences -->

## Scan plugins

- Removing a plugin must stay "delete its folder". Nothing outside the folder may reference a specific
  company; `plugins/all/all_gen.go` is generated (`go run ./plugins/internal/gen`).
- Each plugin's filters live in a `Criteria` value at the top of its main file, so they're easy to edit.
- Code shared by companies on the same job-board system goes in `bff/plugins/shared/<system>/`.
- All HTTP goes through `plugins/shared/web` (per-host pacing, retries, 429 backoff). Respect sites'
  rate limits: never work around them (e.g. by mimicking browser sessions).
- Every plugin calls `scan.ExpireMissing` right after fetching the site's list of open postings, with
  the IDs of every listed posting (before filters), and only when the list is complete. It marks
  saved New/Interested/Not pursuing jobs that the company no longer lists as Expired. Plugins run
  it themselves; the orchestrator only supplies the database through `ctx`.
- Plugins fail cleanly: return partial results with a descriptive error (URL, status, what step),
  log rejections at Debug with the reason, and never panic on unexpected responses.
- Tests use `httptest` with made-up fixtures in `testdata/`, never live sites or real postings.
- Debug a plugin with `go run ./cmd/plugin -v <id>` (no database).

<!-- Plugin-specific preferences -->
