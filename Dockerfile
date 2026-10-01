# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

# Distroless runtime image. Expects GoReleaser linux binaries in dist/:
#   dist/goflow-api-linux-amd64
#   dist/goflow-api-linux-arm64
# CI and release workflows stage the amd64 binary; release builds multi-arch.

FROM gcr.io/distroless/static-debian12:nonroot

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
