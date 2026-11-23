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

## Status

Done: database, frontend, bff, scan orchestration. Next: real company scan plugins.

## Getting started

```sh
docker compose up -d
```

Then open http://localhost:3000.

This starts the database, applies migrations, and runs the BFF (http://localhost:8080/graphql) and
the frontend.
