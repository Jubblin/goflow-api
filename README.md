# goflow-api

A REST API (Go) that ingests [goflow2](https://github.com/netsampler/goflow2) flow exports and
serves them as JSON with filtering and pagination.

**Repository:** https://github.com/Jubblin/goflow-api

It discovers files named `goflow2*.json` in a data directory, loads their newline-delimited JSON
(NDJSON) records into SQLite, and exposes query endpoints over HTTP. An embedded Swagger UI is
served at `/docs/` (fully offline — vendored assets, no CDN).

## Requirements

- Go **1.26+** (see `go.mod`; uses Go 1.22+ `net/http` method-based routing)
- No CGO — SQLite via the pure-Go [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) driver

## Quick start

```bash
# place captures in data/ (see Data below), then:
go run ./cmd/server

# or build a binary
go build -buildvcs=false -o server ./cmd/server && ./server
```

Open http://localhost:8080/docs/ for interactive API documentation.

## Configuration

| Flag | Env | Default | Purpose |
|------|-----|---------|---------|
| `-data-dir` | `DATA_DIR` | `./data` | Directory scanned for `goflow2*.json` |
| `-listen` | `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `-db` | `DB_PATH` | `./goflow.db` | SQLite path (use `:memory:` for ephemeral) |

## Project layout

```text
goflow-api/
  cmd/server/main.go              # flags, startup ingest, HTTP server
  internal/
    model/flow.go                 # Flow struct (typed indexed fields)
    ingest/loader.go              # discover goflow2*.json, NDJSON → SQLite
    store/store.go                # schema, filters, COUNT + LIMIT/OFFSET
    api/
      handlers.go                 # JSON responses, query validation
      router.go                   # routes + request logging
      docs.go                     # embed OpenAPI spec + Swagger UI assets
      openapi.json                # OpenAPI 3 specification
      swaggerui/                  # vendored swagger-ui-dist@5.17.14
  data/                           # goflow2*.json captures (gitignored)
  Dockerfile                      # multi-stage build (main branch)
  docker-compose.yml              # local container deployment
  .github/workflows/              # CI, Docker, release (see Development)
```

## Data

Place capture files in `data/`. Any filename matching `goflow2*.json` is picked up, for example:

- `goflow2.json`
- `goflow2-2026-06-04.json`
- `goflow2 (1).json`

Files are expected to be **NDJSON** (one flow object per line), which is goflow2's default JSON
transport output. The full original record for each line is preserved in `raw_json` and returned
verbatim by the API; a subset of fields is additionally indexed for fast filtering.

On startup (and on `POST /api/v1/reload`) each file's modification time and size are recorded,
so unchanged files are skipped on subsequent runs. The first ingest of a very large capture
(e.g. a 1.5 GB file) may take a few minutes; afterwards it is skipped.

Malformed JSON lines are logged and skipped so a single bad record does not abort ingestion.

## API documentation (Swagger UI)

| Path | Purpose |
|------|---------|
| `GET /` | Redirects to `/docs/` |
| `GET /docs` | Redirects to `/docs/` (301) |
| `GET /docs/` | Swagger UI (vendored, offline) |
| `GET /openapi.json` | OpenAPI 3 specification |

The trailing slash on `/docs/` matters — relative asset URLs resolve correctly only there.

## Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/health` | Liveness probe |
| `GET` | `/api/v1/files` | List ingested files with row counts |
| `POST` | `/api/v1/reload` | Re-scan `data/` and ingest new/changed files |
| `GET` | `/api/v1/flows` | Query flows with filters + pagination |

### `GET /api/v1/flows` query parameters

| Param | Behavior |
|-------|----------|
| `page` | 1-based page number (default `1`) |
| `page_size` | Records per page (default `50`, capped at `500`) |
| `file` | Restrict to one source filename |
| `src_addr`, `dst_addr` | Exact IP match |
| `proto` | Exact protocol match (`TCP`, `UDP`, `ICMP`, …) |
| `src_port`, `dst_port` | Exact integer port match |
| `type` | Record type (e.g. `IPFIX`) |
| `bytes_min`, `bytes_max` | Inclusive numeric range on `bytes` |
| `q` | Substring match across `src_addr`, `dst_addr`, `proto` |

Invalid integer parameters return `400` with a JSON error body. A page beyond the result range
returns `200` with an empty `data` array.

### Response envelope

```json
{
  "data": [ { "...full goflow2 record..." } ],
  "pagination": {
    "page": 1,
    "page_size": 50,
    "total": 1054,
    "total_pages": 22
  }
}
```

## Examples

```bash
# list ingested files
curl 'http://localhost:8080/api/v1/files'

# TCP flows to port 443 from a specific host, page 2
curl 'http://localhost:8080/api/v1/flows?proto=TCP&dst_port=443&src_addr=10.2.254.48&page=2&page_size=25'

# DNS (UDP/53) flows
curl 'http://localhost:8080/api/v1/flows?proto=UDP&dst_port=53'

# large TCP transfers
curl 'http://localhost:8080/api/v1/flows?proto=TCP&bytes_min=5000'

# substring search for an address
curl 'http://localhost:8080/api/v1/flows?q=1.1.1.1'

# flows from one capture file
curl 'http://localhost:8080/api/v1/flows?file=goflow2.json&page=1&page_size=50'

# re-ingest after dropping new files into data/
curl -X POST 'http://localhost:8080/api/v1/reload'
```

## Docker

### docker compose (local development)

The [`Dockerfile`](Dockerfile) on `main` is a multi-stage build: compile a static Go binary, then
run it on Alpine as an unprivileged `app` user with `/data` as a volume.

```bash
docker compose up --build      # build and start
docker compose up -d           # detached
docker compose logs -f         # follow logs
docker compose down            # stop
```

Inside the container the defaults are `DATA_DIR=/data`, `DB_PATH=/data/goflow.db`, and
`LISTEN_ADDR=:8080`. Mount your capture directory at `/data`. To keep `/data` read-only, point
the database elsewhere, e.g. `-e DB_PATH=/tmp/goflow.db`.

### Manual image build

```bash
docker build -t goflow-api:latest .
docker run --rm -p 8080:8080 -v "$PWD/data:/data" goflow-api:latest
```

## Development

### Build and verify

```bash
go build -buildvcs=false ./...
go vet ./...
go test -buildvcs=false ./...
```

### Pre-commit

[`.pre-commit-config.yaml`](.pre-commit-config.yaml) runs trailing-whitespace, YAML checks,
`golangci-lint`, `go vet`, and Hadolint (on `Dockerfile` changes):

```bash
pre-commit install
pre-commit run --all-files
```

A [`.golangci.yml`](.golangci.yml) config is expected by pre-commit but may not yet be present —
add one before enabling full lint locally.

### Editor

[`.vscode/extensions.json`](.vscode/extensions.json) recommends the Go, golangci-lint, and
EditorConfig extensions.

## CI/CD and releases

GitHub Actions workflows under [`.github/workflows/`](.github/workflows/) are scaffolded for:

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| [`ci.yml`](.github/workflows/ci.yml) | Push/PR to `main` | Test, lint, cross-compile, SBOM, Trivy; snapshot release on push |
| [`docker.yml`](.github/workflows/docker.yml) | Push/PR to `main` | Dockerfile lint, image build, container scan |
| [`release.yml`](.github/workflows/release.yml) | Tag `v*` or CI call | GoReleaser binaries, GHCR image, Cosign signing |

[`.goreleaser.yaml`](.goreleaser.yaml) and [Renovate](renovate.json5) keep dependencies current.

The [`Makefile`](Makefile) provides `build`, `build-platform`, `test`, and `sbom` targets.
Binaries are named `goflow-api` and built from [`cmd/server`](cmd/server/main.go).
Container images publish to `ghcr.io/jubblin/goflow-api`.

### Repository controls

On GitHub, `main` is protected:

- Pull requests required (1 approval), conversation resolution, linear history
- Squash-only merges; head branches deleted after merge
- Force-push and branch deletion blocked

## Contributing

1. Branch from `main` (e.g. `feature/my-change`).
2. Make changes; run `go build -buildvcs=false ./...` and `go vet ./...`.
3. Open a pull request using the [PR template](.github/pull_request_template.md).

## Notes

- The SQLite database (`goflow.db`) is regenerated from `data/` and is gitignored.
- Capture files (`data/*.json`) are gitignored; only `data/.gitkeep` is tracked.
