# bff

Backend-for-frontend: a small Go GraphQL API over the `jobs` table.

> [!WARNING]
> **Local use only. Not for production.** No auth, hardcoded database credentials, and CORS open to
> any origin.

It serves two clients:

- **frontend**: `job`, `jobs` (filter, sort, paginate), `updateJob`
- **engine**: `submitJobs`. The engine sends every posting it finds in a scan; postings whose
  `(companyName, postingId)` is already saved (or repeated in the batch) are skipped. The engine never
  talks to the database directly.

The schema is in [`graph/schema.graphqls`](graph/schema.graphqls).

## Stack

- Go standard library HTTP server
- [graph-gophers/graphql-go](https://github.com/graph-gophers/graphql-go): schema-first, no code generation
- [lib/pq](https://github.com/lib/pq): Postgres driver for `database/sql`

## Layout

```
main.go              HTTP server, config, CORS
graph/schema.graphqls  GraphQL schema (embedded into the binary)
graph/resolver.go    Query/Mutation resolvers, input validation
graph/scalars.go     Date and DateTime scalars
store/store.go       All SQL
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

Endpoints: `POST /graphql`, `GET /healthz`. CORS allows any origin (local-only app).

Example:

```sh
curl -s localhost:8080/graphql -H 'Content-Type: application/json' \
  -d '{"query":"{ jobs(filter: {excludedStatuses: [USER_REJECTED, COMPANY_REJECTED]}) { totalCount items { id companyName jobTitle status } } }"}'
```

## Tests

Integration tests run against a real Postgres. Each test creates a throwaway schema, applies
`db/migrations` into it, and drops it afterwards, so existing data is never touched.

```sh
docker compose up -d db
TEST_DATABASE_URL='postgres://jobapps:jobapps@localhost:5432/jobapps?sslmode=disable' go test ./...
```

Without `TEST_DATABASE_URL` the tests are skipped.
