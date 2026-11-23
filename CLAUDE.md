# CLAUDE.md

Guidance for Claude Code when working in this repo.

## Project overview

Personal app for managing job postings I may apply to. Four parts, built in this order:

1. `db/` — lightweight database
2. `frontend/` — React + TypeScript web UI
3. `bff/` — Go backend-for-frontend API
4. `engine/` — background job scanner

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

## Engine

<!-- Engine-specific preferences -->
