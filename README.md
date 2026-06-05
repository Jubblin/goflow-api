# goflow-api

A small REST API (Go) that ingests [goflow2](https://github.com/netsampler/goflow2) flow
exports and serves them as JSON with filtering and pagination.

It discovers files named `goflow2*.json` (prefix `goflow2`, suffix `.json`) in a data
directory, loads their newline-delimited JSON (NDJSON) records into SQLite, and exposes
query endpoints over HTTP.

## Requirements

- Go 1.22+ (uses `net/http` method-based routing)
- No CGO — SQLite is provided by the pure-Go `modernc.org/sqlite` driver

## Layout

```text
goflow-api/
  cmd/server/main.go        # flags, startup ingest, HTTP server
  internal/
    model/flow.go           # Flow struct (typed indexed fields)
    ingest/loader.go        # discover goflow2*.json, NDJSON -> SQLite
    store/store.go          # schema, dynamic WHERE filters, COUNT + LIMIT/OFFSET
    api/router.go           # routes + logging middleware
    api/handlers.go         # JSON responses, query validation
  data/                     # put your goflow2*.json files here
```

## Data

Place capture files in `data/`. Any filename matching `goflow2*.json` is picked up,
for example:

- `goflow2.json`
- `goflow2-2026-06-04.json`
- `goflow2 (1).json`

Files are expected to be NDJSON (one flow object per line), which is goflow2's default
JSON transport output. The full original record for each line is preserved and returned
verbatim by the API; a subset of fields is additionally indexed for fast filtering.

On startup (and on `POST /api/v1/reload`) each file's modification time and size are
recorded, so unchanged files are skipped on subsequent runs and only new/changed files
are re-ingested. The first ingest of a very large capture (e.g. a 1.5 GB file) may take
a few minutes; afterwards it is skipped.

## Run

```bash
# from the project root
go run ./cmd/server
# or build a binary
go build -o server ./cmd/server && ./server
```

### Configuration

| Flag | Env | Default | Purpose |
|------|-----|---------|---------|
| `-data-dir` | `DATA_DIR` | `./data` | Directory scanned for `goflow2*.json` |
| `-listen` | `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `-db` | `DB_PATH` | `./goflow.db` | SQLite path (use `:memory:` for ephemeral) |

## Docker

A multi-stage [`Dockerfile`](Dockerfile) builds a static binary (no CGO) and runs it on a
minimal Alpine image as an unprivileged user.

```bash
# build
docker build -t goflow-api:latest .

# run: mount your goflow2*.json captures into /data and publish the port
docker run --rm -p 8080:8080 -v "$PWD/data:/data" goflow-api:latest
```

Inside the container the defaults are `DATA_DIR=/data`, `DB_PATH=/data/goflow.db`, and
`LISTEN_ADDR=:8080`; override any of them with `-e`. `/data` is declared as a volume, so
mount the directory holding your capture files there. To keep `/data` read-only, point the
database elsewhere, e.g. `-v "$PWD/data:/data:ro" -e DB_PATH=/tmp/goflow.db`.

### docker compose

A [`docker-compose.yml`](docker-compose.yml) wires up the build, the `./data` volume, and
port `8080`:

```bash
docker compose up --build      # build and start
docker compose up -d           # detached
docker compose logs -f         # follow logs
docker compose down            # stop
```

## API documentation (Swagger UI)

Interactive docs are served by the app itself:

- `GET /docs` — Swagger UI (the root path `/` redirects here)
- `GET /openapi.json` — the OpenAPI 3 specification

Open `http://localhost:8080/docs` (or `http://<host>:8080/docs` for a remote deployment)
to browse endpoints and use "Try it out". Both the OpenAPI spec and the Swagger UI assets
(pinned `swagger-ui-dist@5.17.14`, vendored under `internal/api/swaggerui/`) are embedded
in the binary, so the docs page works fully offline with no CDN or internet dependency.

## Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET`  | `/health` | Liveness probe |
| `GET`  | `/api/v1/files` | List ingested files with row counts |
| `POST` | `/api/v1/reload` | Re-scan `data/` and ingest new/changed files |
| `GET`  | `/api/v1/flows` | Query flows with filters + pagination |
| `GET`  | `/docs` | Swagger UI |
| `GET`  | `/openapi.json` | OpenAPI 3 spec |

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

Invalid integer parameters return `400` with a JSON error body. A page beyond the result
range returns `200` with an empty `data` array.

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

## Notes

- Lines that fail to parse as JSON are skipped (logged) so a single malformed record does
  not abort ingestion of the rest of a file.
- The SQLite database file (`goflow.db`) is regenerated from `data/` and is gitignored.
