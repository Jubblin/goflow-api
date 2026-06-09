BINARY := goflow-api
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X github.com/richardw/goflow-api/internal/version.Version=$(VERSION) \
	-X github.com/richardw/goflow-api/internal/version.Commit=$(COMMIT) \
	-X github.com/richardw/goflow-api/internal/version.Date=$(DATE)

GOFLAGS := -buildvcs=false -trimpath

.PHONY: build build-platform test sbom docker-build

build:
	mkdir -p bin
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/server

build-platform:
	mkdir -p bin
	ext=""; [ "$(GOOS)" = "windows" ] && ext=".exe"; \
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
		-o bin/$(BINARY)-$(GOOS)-$(GOARCH)$$ext ./cmd/server

test:
	go test $(GOFLAGS) ./...

sbom:
	mkdir -p dist
	go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@v1.10.0
	cyclonedx-gomod mod -licenses -json -output dist/sbom.cyclonedx.json .

docker-build:
	$(MAKE) build-platform GOOS=linux GOARCH=amd64 VERSION=$(VERSION)
	mkdir -p dist
	cp bin/$(BINARY)-linux-amd64 dist/$(BINARY)-linux-amd64
	chmod +x dist/$(BINARY)-linux-amd64
	docker build --platform linux/amd64 -t $(BINARY):local .
