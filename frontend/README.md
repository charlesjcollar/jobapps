# frontend

Single-page web UI for jobapps: one spreadsheet-like table of every job. Built for desktop Chrome
only.

> [!WARNING]
> **Local use only. Not for production.**

## Features

- Lists jobs 50 per page, newest added first, with pagination at the bottom
- Click a column header to sort by it; click again to reverse
- A filter bar above the table narrows the list: **Company** and **Title** (case-insensitive
  substring; applies when you pause typing), a **Status** multi-select (none checked = all), and
  **Hide closed** (leaves out Not pursuing, Rejected, and Expired). **Clear filters** appears when any
  filter is set. Pagination counts reflect the filtered total, and the filters are remembered in this
  browser (localStorage).
- Job titles link to the posting (opens in a new tab)
- Status, location, salary, contact name/email/phone, and notes are edited in place and **save
  automatically**: status saves on change; text fields save when you pause typing (~0.8s) or leave
  the field. The row's Updated column briefly shows "Saving…" / "Saved ✓", or "Not saved" (hover
  for the error).
- Dates in the current year show without the year; hover for the full date/time.
- **Rescan job postings** starts a scan in the bff. A modal shows each company's progress live; it
  can be closed while the scan keeps running (the button shows progress). Each company that fails
  raises a notification with the error, which stays until dismissed. The table reloads when the
  scan finishes.

## Stack

- React + TypeScript, built with Vite
- No other runtime dependencies: the API client is plain `fetch` against `/graphql`, and styling is
  one plain CSS file (`src/styles.css`, follows the OS light/dark setting)

## Layout

```
src/api.ts                    GraphQL queries, plus watchScan (reads the SSE progress stream)
src/useScan.ts                Scan state: start, follow progress, failure notifications
src/types.ts                  Types mirroring bff/graph/schema.graphqls
src/filters.ts                Filter settings: conversion to JobFilter, saving to localStorage
src/App.tsx                   Page: loading, filter, sort and page state
src/components/JobsTable.tsx  Table header and sorting
src/components/JobRow.tsx     One row; saves edits and shows save state
src/components/AutoSaveField.tsx  Text input/textarea that saves itself
src/components/Pagination.tsx
src/components/FilterBar.tsx   Filter bar (company, title, status, hide closed)
src/components/StatusFilter.tsx  Status multi-select dropdown
src/components/ScanModal.tsx  Scan progress modal
src/components/Toasts.tsx     Error notifications
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
