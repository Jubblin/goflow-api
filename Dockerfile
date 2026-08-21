# syntax=docker/dockerfile:1

# Distroless runtime image. Expects GoReleaser linux binaries in dist/:
#   dist/goflow-api-linux-amd64
#   dist/goflow-api-linux-arm64
# CI and release workflows stage the amd64 binary; release builds multi-arch.

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

ARG TARGETARCH
COPY dist/goflow-api-linux-${TARGETARCH} /usr/local/bin/goflow-api

USER nonroot:nonroot
WORKDIR /home/nonroot

ENV DATA_DIR=/data \
	DB_PATH=/data/goflow.db \
	LISTEN_ADDR=:8080

EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["/usr/local/bin/goflow-api"]

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
	CMD ["/usr/local/bin/goflow-api", "-version"]
