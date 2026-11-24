# bff

Backend-for-frontend: a small Go GraphQL API over the `jobs` table that also runs job scans.

> [!WARNING]
> **Local use only. Not for production.** No auth, hardcoded database credentials, and CORS open to
> any origin.

The schema is in [`graph/schema.graphqls`](graph/schema.graphqls).

- **Jobs**: `job`, `jobs` (filter, sort, paginate), `updateJob`. New jobs are only added by scans.
- **Scans**: `startScan`, `currentScan`, and the `scanUpdated` subscription

## Scans

`startScan` runs every registered plugin in the background and returns right away with the list of
plugins it will run. Each plugin scans one company's job board and returns the postings that pass
its filters. The orchestrator ([`scan/`](scan/)):

- runs up to 8 plugins at a time (`SCAN_CONCURRENCY`), each with a 2-minute timeout unless the
  plugin asks for more (`Timeout()`)
- records a plugin as failed if it returns an error, panics, times out, or returns a posting with
  an empty required field. The rest keep running
- saves partial results: a plugin that fails partway can return what it found before the error,
  and those postings are still saved
- saves only postings whose `(company name, posting ID)` isn't already in the database, so plugins
  can return every match they see
- runs one scan at a time: `startScan` during a scan returns the running scan

### Auto-expiration

Each plugin also marks saved postings that the company no longer lists as `EXPIRED`. Right after
fetching the site's list of open postings, the plugin calls `scan.ExpireMissing(ctx, companyName,
openPostingIDs)`, a shared helper in [`scan/expire.go`](scan/expire.go). The plugin runs it, not
the orchestrator; the orchestrator only passes the database in through `ctx`, the same way it
passes the logger.

- Only jobs that are **New**, **Interested** or **Not pursuing** are expired. Anything I've applied
  to (or further) keeps its status.
- The IDs passed are **every** open posting the site listed, before filters. A saved job that now
  fails a filter is still open, so it isn't expired.
- It's skipped when the list is incomplete (the fetch failed or stopped early), and when the site
  lists nothing at all, since both more likely mean the site broke.
- Safety limit: if more than half of a company's expirable saved jobs (and more than 5) go missing
  in one scan, nothing is expired and the run fails with an explanation. That pattern more likely
  means the site changed how it lists or identifies postings.
- The scan modal shows how many postings each plugin expired.

Progress is delivered by the `scanUpdated(id)` subscription, which sends the whole scan state
immediately and again whenever a plugin starts or finishes, then completes.

Scan state lives in memory only. Restarting the bff forgets past scans and stops one in progress.

## Plugins

```
plugins/
  registry.go          Register / All / Get
  all/all_gen.go       generated: imports every company folder
  internal/gen/        the generator
  shared/              code shared between companies (not plugins themselves)
    web/               polite HTTP client: per-host pacing, retries, backoff on 429
    eightfold/         client for career sites hosted on Eightfold
    greenhouse/        client for Greenhouse job boards (one request returns every posting)
    jobtext/           HTML to text, years-of-experience and salary parsing
  netflix/             one folder per company
    netflix.go         Criteria (the filters) at the top, then the scan
    netflix_test.go
    testdata/          made-up API responses for tests
  demo/                fake plugins, only registered when DEMO_PLUGINS=true
```

### Running one plugin (debugging)

`cmd/plugin` runs a single plugin against the live site and prints what it keeps. It never writes
to the database.

```sh
go run ./cmd/plugin                # list plugins
go run ./cmd/plugin netflix        # run it; print kept postings
go run ./cmd/plugin -v netflix     # also log each rejected posting with the reason, and each request
go run ./cmd/plugin -json netflix  # JSON output
go run ./cmd/plugin -db netflix    # also read the database and list saved postings that would
                                   # expire (read-only; DATABASE_URL or the local docker database)
```

Without `-db`, the expiration check is skipped.

### Changing a plugin's filters

Each plugin keeps its filters in a `Criteria` value at the top of its main file (e.g.
`netflix/netflix.go`). Edit it, check with `go run ./cmd/plugin -v <id>`, then rebuild.

### Adding a plugin

1. Create `plugins/<company>/` with a type implementing `scan.Plugin`:

   ```go
   type Plugin interface {
       Name() string                                     // shown in the UI
       Scan(ctx context.Context) ([]store.NewJob, error) // postings that pass the filters
   }
   ```

   Register it from `init()`: `plugins.Register("<company>", New(...))`. Implement
   `Timeout() time.Duration` too if it needs longer than 2 minutes.
2. Use `shared/web` for HTTP so requests are paced and retried. If the company's job board runs on
   a system another plugin already uses, build on that client in `shared/`. Otherwise add one there.
3. Log with `scan.Logger(ctx)`: `Debug` for each rejected posting and its reason, `Warn` for
   problems that don't stop the scan. Return errors that say what failed and where (URL, status),
   since they're shown in the UI.
4. Call `scan.ExpireMissing(ctx, companyName, openPostingIDs)` right after fetching the list of
   open postings (see [Auto-expiration](#auto-expiration)). Pass every posting the site listed,
   not just the ones that pass the filters, and build the IDs exactly as for `store.NewJob`. Skip
   it if the list is incomplete. Return its error along with the postings.
5. Write tests against an `httptest` server serving made-up responses in `testdata/`.
6. Regenerate the import list: `go run ./plugins/internal/gen` (the Docker build does this too).

### Removing a plugin

Delete its folder. `docker compose up -d --build` regenerates the import list. To run Go locally
afterwards, run `go run ./plugins/internal/gen` first; until then the build fails with
`package jobapps/bff/plugins/<company> is not in std`.

### Netflix

Netflix's careers site is on Eightfold. The plugin searches for "Software Engineer", applies the
title, level, country and work-arrangement (no hybrid or on-site) checks to the search results, and then fetches the full posting
only for the jobs that pass, to check years of experience and read the salary range. The site
rate-limits its search API at around 6 requests a minute (it sends no rate-limit headers), so the
plugin waits 12 seconds between search pages and 3 seconds between posting fetches, backs off for
20 seconds or more on a 429, and allows itself 15 minutes. A scan takes about 4 minutes.

### Airbnb

Airbnb's careers site lists postings from Greenhouse (board `airbnb`), so the plugin makes a single
request to the public Greenhouse Job Board API and gets every posting with its description. It
keeps US postings whose `Workplace Type` is `Remote`, which the site labels "Live and Work
Anywhere". A posting counts as US if its location or one of its offices says United States, USA or
US. It then applies the title rules and the years-of-experience rule. If no posting has a
`Workplace Type` field at all, the run fails rather than quietly keeping nothing, since that means
Airbnb renamed the field.

### Alpaca

Alpaca's careers page also lists postings from Greenhouse (board `alpaca`): one request, no detail
fetches. Alpaca is remote-first and has no workplace-type field, so the plugin reads the free-text
location ("Remote - Americas", "Global Anywhere", "Remote - Europe, Americas"):

- The location or an office must say Remote or Anywhere.
- The location must name North America, Americas, America, Global, Anywhere, United States, USA or
  US. "South America" doesn't count as "America". If the location is just "Remote", the offices
  decide.
- Titles tied to another country or region (Canada, APAC, EMEA, Europe, Japan, Singapore, ...) are
  dropped unless they also name an allowed region ("US or Canada").
- Title rules are the same as the other plugins, plus Alpaca's "Full-Stack" spelling, "Product
  Engineer" titles, and excluding "Quality" (quality-engineering roles).

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
plugins/               Company scan plugins (see above)
cmd/plugin/            CLI to run one plugin without the database
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
| `SCAN_CONCURRENCY` | `8` |
| `DEMO_PLUGINS` | unset (set to `true` to enable demo plugins) |

Endpoints: `POST /graphql`, `POST /graphql/stream`, `GET /healthz`.

## Tests

```sh
go test ./...
```

The scan, plugin and SSE tests need nothing else. Plugin tests use made-up responses, never the
live sites. The job tests run against a real Postgres and are skipped
unless `TEST_DATABASE_URL` is set. Each one creates a throwaway schema, applies `db/migrations` into
it, and drops it afterwards, so existing data is never touched:

```sh
docker compose up -d db
TEST_DATABASE_URL='postgres://jobapps:jobapps@localhost:5432/jobapps?sslmode=disable' go test ./...
```
