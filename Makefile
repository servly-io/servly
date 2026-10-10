# servly - build, test, install.

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/servly-io/servly/internal/cli.Version=$(VERSION)
DIST    := dist
GO ?= go
NPM ?= npm
GOLANGCILINT_VERSION ?= v2.13.2
IMAGE ?= servly:$(VERSION)
CONTAINER ?= $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)

# A failing `go test` must fail the pipeline it is tee'd through.
SHELL := bash
.SHELLFLAGS := -o pipefail -c

# Coverage and test-report output. CI uploads these; locally `make cover`
# opens the HTML report. The race detector needs cgo, so it is on wherever a
# C compiler is (CI, most workstations) and skipped where there is none.
RACE      := $(if $(filter 1,$(shell $(GO) env CGO_ENABLED)),-race,)
TESTCOVER ?= $(RACE) -covermode=atomic -coverprofile=coverage.out -coverpkg=./...
GOJUNITREPORT_VERSION ?= v2.1.0

.PHONY: all build build-go web web-deps web-lint web-typecheck run image test test-report test-integration fmt-check \
        cover check vet fmt lint clean install docs docs-serve

all: build

## build: the frontend, then the binary with the frontend embedded, into dist/
build: web build-go

## build-go: the binary only, embedding whatever web/dist holds
build-go:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o $(DIST)/servly ./cmd/servly
	@echo "built $(DIST)/servly ($(VERSION))"

web-deps:
	cd web && $(NPM) ci

## web: build the frontend into web/dist
web: web-deps
	cd web && $(NPM) run build

web-lint: web-deps
	cd web && $(NPM) run lint

web-typecheck: web-deps
	cd web && $(NPM) run typecheck

## run: build and serve locally on :8080 with SQLite in ./data and readable logs
run: build
	SERVLY_LOG_FORMAT=text $(DIST)/servly serve

## image: build the container image (see Containerfile).
image:
	$(CONTAINER) build --build-arg VERSION=$(VERSION) -t $(IMAGE) -f Containerfile .
	@echo "built $(IMAGE)"

## test: unit and SQLite integration tests with the race detector and coverage. No containers.
test:
	$(GO) test $(TESTCOVER) ./...

## test-report: the same tests, with a JUnit file (junit.xml) for CI to publish.
test-report:
	$(GO) test $(TESTCOVER) -v -json ./... 2>&1 | tee test.json | \
		$(GO) run github.com/jstemmer/go-junit-report/v2@$(GOJUNITREPORT_VERSION) -parser gojson -out junit.xml
	@scripts/coverage-summary.sh coverage.out

## test-integration: every test on SQLite and on PostgreSQL. Starts a throwaway
## PostgreSQL container unless SERVLY_TEST_POSTGRES_URL is already set.
test-integration:
	scripts/test-integration.sh

## cover: coverage per package, then the HTML report in the browser.
cover: test
	@scripts/coverage-summary.sh coverage.out
	$(GO) tool cover -html=coverage.out

## check: everything CI checks, except the PostgreSQL run (see test-integration).
check: fmt-check vet lint test web-lint web-typecheck web

fmt-check:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "run: make fmt"; exit 1; }

vet:
	$(GO) vet ./...

fmt:
	gofmt -l -w .

## lint: run golangci-lint using the repo's Go toolchain.
lint:
	GO111MODULE=on $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCILINT_VERSION) run --timeout 10m --default=none --enable=govet

# Docusaurus needs Node 20+. With an older (or no) host Node, the docs
# targets run in a Node container instead.
NODE_MAJOR      := $(shell node -p 'process.versions.node.split(".")[0]' 2>/dev/null || echo 0)
DOCS_NODE_IMAGE ?= docker.io/library/node:22-alpine
DOCS_IN_CONTAINER = $(CONTAINER) run --rm --init -v "$(CURDIR)/docs:/docs:Z" -w /docs
# `npm ci` only when node_modules is missing, older than the lockfile, or was
# installed by the other runtime (host vs container: native binaries differ).
# npm ci wipes node_modules, which drops the other runtime's marker.
docs_deps = m=node_modules/.servly-installed-$(1); \
	if [ -f $$m ] && [ $$m -nt package-lock.json ]; then echo "docs dependencies up to date"; \
	else npm ci && touch $$m; fi

## docs: build the documentation site (docs/, Docusaurus) into docs/build.
docs:
	@if [ "$(NODE_MAJOR)" -ge 20 ]; then \
		cd docs && { $(call docs_deps,host); } && $(NPM) run build; \
	else \
		echo "host Node $(NODE_MAJOR) < 20: building docs in $(DOCS_NODE_IMAGE)"; \
		$(DOCS_IN_CONTAINER) $(DOCS_NODE_IMAGE) sh -c '$(call docs_deps,container) && npm run build'; \
	fi

## docs-serve: run the documentation site locally with live reload (make docs-serve DOCS_PORT=4000).
DOCS_PORT ?= 3000
docs-serve:
	@if [ "$(NODE_MAJOR)" -ge 20 ]; then \
		cd docs && { $(call docs_deps,host); } && DOCS_BASE_URL=/ $(NPM) start -- --port $(DOCS_PORT); \
	else \
		echo "host Node $(NODE_MAJOR) < 20: serving docs from $(DOCS_NODE_IMAGE) on http://localhost:$(DOCS_PORT)/ (ready once compiled)"; \
		$(DOCS_IN_CONTAINER) -e DOCS_BASE_URL=/ -p 127.0.0.1:$(DOCS_PORT):$(DOCS_PORT) $(DOCS_NODE_IMAGE) \
			sh -c '$(call docs_deps,container) && npm start -- --host 0.0.0.0 --port $(DOCS_PORT) --no-open'; \
	fi

## install: put the binary on this machine (needs root).
install: build
	install -m 0755 $(DIST)/servly /usr/local/bin/servly

clean:
	rm -rf $(DIST) coverage.out coverage.html junit.xml test.json
	find web/dist -mindepth 1 ! -name .gitkeep -delete 2>/dev/null || true
