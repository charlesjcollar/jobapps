# frontend

Single-page web UI for jobapps: one spreadsheet-like table of every job. Built for desktop Chrome
only.

> [!WARNING]
> **Local use only. Not for production.**

## Features

- Lists jobs 50 per page, newest added first, with pagination at the bottom
- Click a column header to sort by it; click again to reverse
- Job titles link to the posting (opens in a new tab)
- Status, contact name/email/phone, and notes are edited in place and **save automatically**:
  status saves on change; text fields save when you pause typing (~0.8s) or leave the field.
  Each row shows "Saving…", "Saved", or "Not saved" (hover for the error).

## Stack

- React + TypeScript, built with Vite
- No other runtime dependencies: the API client is plain `fetch` against `/graphql`, and styling is
  one plain CSS file (`src/styles.css`, follows the OS light/dark setting)

## Layout

```
src/api.ts                    GraphQL queries (listJobs, updateJob)
src/types.ts                  Types mirroring bff/graph/schema.graphqls
src/App.tsx                   Page: loading, sort and page state
src/components/JobsTable.tsx  Table header and sorting
src/components/JobRow.tsx     One row; saves edits and shows save state
src/components/AutoSaveField.tsx  Text input/textarea that saves itself
src/components/Pagination.tsx
```

## Running

With the full stack (from the repo root): `docker compose up -d --build`, then open
http://localhost:3000. nginx serves the build and proxies `/graphql` to the BFF.

For development with hot reload:

```sh
docker compose up -d db flyway bff   # from the repo root
npm install
npm run dev                          # http://localhost:3000, proxies /graphql to localhost:8080
```

Set `BFF_URL` to proxy somewhere else, e.g. `BFF_URL=http://localhost:18080 npm run dev`.

`npm run build` type-checks and builds to `dist/`.
