# syntax=docker/dockerfile:1

# --- Build stage -------------------------------------------------------------
# Pure-Go build (modernc.org/sqlite needs no CGO), so we can produce a fully
# static binary and run it on a minimal image.
FROM golang:1.26-alpine AS build

WORKDIR /src

# Cache module downloads separately from the source for faster rebuilds.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0 GOOS=linux
RUN go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# --- Runtime stage -----------------------------------------------------------
FROM alpine:3.20

# Run as an unprivileged user that owns the data directory.
RUN addgroup -S app \
    && adduser -S -G app app \
    && mkdir -p /data \
    && chown app:app /data

COPY --from=build /out/server /usr/local/bin/server

USER app

# Defaults read by cmd/server (overridable at run time).
ENV DATA_DIR=/data \
    DB_PATH=/data/goflow.db \
    LISTEN_ADDR=:8080

EXPOSE 8080

# Mount goflow2*.json captures here; the SQLite db is written here too.
VOLUME ["/data"]

ENTRYPOINT ["server"]
