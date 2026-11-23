# bff

Backend-for-frontend: a small Go GraphQL API over the `jobs` table that also runs job scans.

> [!WARNING]
> **Local use only. Not for production.** No auth, hardcoded database credentials, and CORS open to
> any origin.

The schema is in [`graph/schema.graphqls`](graph/schema.graphqls).

- **Jobs**: `job`, `jobs` (filter, sort, paginate), `updateJob`. New jobs are only added by scans.
- **Scans**: `startScan`, `currentScan`, and the `scanUpdated` subscription

## Scans

`startScan` runs every plugin in [`plugins/`](plugins/) in the background and returns right away
with the list of plugins it will run. Each plugin scans one company's job board and returns the
postings it finds. The orchestrator ([`scan/`](scan/)):

- runs up to 4 plugins at a time, each with a 2-minute timeout
- records each plugin as failed if it returns an error, panics, times out, or returns a posting
  with an empty required field. The rest keep running
- saves only postings whose `(company name, posting ID)` isn't already in the database, so plugins
  can return every posting they see
- runs one scan at a time: `startScan` during a scan returns the running scan

Progress is delivered by the `scanUpdated(id)` subscription, which sends the whole scan state
immediately and again whenever a plugin starts or finishes, then completes.

Scan state lives in memory only. Restarting the bff forgets past scans and stops one in progress.

### Writing a plugin

There is one plugin per company, and the companies are chosen by hand. Many companies host their
job board on the same system (Greenhouse, Lever, Workday, ...), so code for a given system should
live in a shared package under `plugins/` that each company's plugin builds on.

Implement `scan.Plugin` and add it to `All()` in [`plugins/plugins.go`](plugins/plugins.go):

```go
type Plugin interface {
    Name() string                                     // shown in the UI, usually the company name
    Scan(ctx context.Context) ([]store.NewJob, error) // every current posting
}
```

Respect `ctx` (it carries the timeout). Errors are shown in the UI as-is, so make them descriptive.

`DEMO_PLUGINS=true` adds five fake plugins (one always fails) for trying the UI. They save nothing.
It's on in `docker-compose.yml` until real plugins exist.

## Subscriptions over SSE

`POST /graphql/stream` takes a normal GraphQL request body containing a subscription and responds
with `text/event-stream`: an `event: next` message per result, then `event: complete`. This is the
"distinct connections" mode of the GraphQL over SSE protocol, built on the standard library.

```sh
curl -N localhost:8080/graphql/stream -H 'Content-Type: application/json' \
  -d '{"query":"subscription { scanUpdated(id: \"1\") { running plugins { name status error } } }"}'
```

## Stack

- Go standard library HTTP server
- [graph-gophers/graphql-go](https://github.com/graph-gophers/graphql-go): schema-first, no code generation
- [lib/pq](https://github.com/lib/pq): Postgres driver for `database/sql`

## Layout

```
main.go                HTTP server, config, CORS
graph/schema.graphqls  GraphQL schema (embedded into the binary)
graph/resolver.go      Job queries/mutations, input validation
graph/scan.go          Scan query/mutation/subscription
graph/stream.go        Subscriptions over SSE
graph/scalars.go       Date and DateTime scalars
scan/scan.go           Scan orchestrator
plugins/               Company scan plugins (plus demo plugins)
store/store.go         All SQL
```

## Running

With the rest of the stack (from the repo root):

```sh
docker compose up -d --build
```

Or locally against the compose database:

```sh
docker compose up -d db flyway
go run .
```

| Env var | Default |
|---------|---------|
| `DATABASE_URL` | `postgres://jobapps:jobapps@localhost:5432/jobapps?sslmode=disable` |
| `ADDR` | `:8080` |
| `DEMO_PLUGINS` | unset (set to `true` to enable demo plugins) |

Endpoints: `POST /graphql`, `POST /graphql/stream`, `GET /healthz`.

## Tests

```sh
go test ./...
```

The scan and SSE tests need nothing else. The job tests run against a real Postgres and are skipped
unless `TEST_DATABASE_URL` is set. Each one creates a throwaway schema, applies `db/migrations` into
it, and drops it afterwards, so existing data is never touched:

```sh
docker compose up -d db
TEST_DATABASE_URL='postgres://jobapps:jobapps@localhost:5432/jobapps?sslmode=disable' go test ./...
```
