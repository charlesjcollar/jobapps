# jobapps

A personal app for tracking job postings I may apply to.

> [!WARNING]
> **Local use only. Not for production.** There is no authentication, credentials are hardcoded,
> and the API accepts requests from any origin. Don't deploy this or expose it to a network.

The code is public, but personal data (database contents, saved postings, resumes, cover letters)
is kept out of the repo by `.gitignore` and Docker volumes.

## Architecture

```
┌────────────┐     HTTP/JSON     ┌──────────┐            ┌──────────┐
│  frontend  │ ────────────────▶ │   bff    │ ─────────▶ │    db    │
│ React + TS │                   │   (Go)   │            │          │
└────────────┘                   └──────────┘            └──────────┘
                                                              ▲
                                 ┌──────────┐                 │
                                 │  engine  │ ────────────────┘
                                 │ (scanner)│  writes discovered postings
                                 └──────────┘
```

| Part | Folder | Description |
|------|--------|-------------|
| Database | [`db/`](db/README.md) | PostgreSQL + Flyway migrations, plus a file archive of saved postings and submitted resumes/cover letters |
| Frontend | [`frontend/`](frontend/README.md) | Simple web UI (React + TypeScript) |
| BFF | [`bff/`](bff/README.md) | Backend-for-frontend API (Go) |
| Engine | [`engine/`](engine/README.md) | Background job that scans sources for new postings |

## Build order

1. Database
2. Frontend
3. BFF
4. Engine

## Getting started

```sh
docker compose up -d
```

Then open http://localhost:3000.

This starts the database, applies migrations, and runs the BFF (http://localhost:8080/graphql) and
the frontend. The engine will be added once it's built.
