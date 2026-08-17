# brickmesh — Go engine + Python extractor.
#
# The two halves meet at exactly one place: the extractor writes
# data/catalog.json, the engine reads it. So they build and test independently,
# and this Makefile is the thin layer that runs both.

.PHONY: all build test lint fmt clean \
        go-build go-test go-vet go-fmt go-staticcheck go-lint-complexity \
        py-sync py-test py-lint py-fmt py-lock help

GO      ?= go
UV      ?= uv
BINDIR  ?= bin
BINARY  ?= $(BINDIR)/brickmesh

all: build

help:
	@echo "build   - build the Go binary"
	@echo "test    - run Go and Python tests"
	@echo "lint    - vet, gofmt check, staticcheck, complexity lens, ruff"
	@echo "fmt     - format Go and Python sources in place"
	@echo "clean   - remove build output and caches"

build: go-build
test: go-test py-test
lint: go-vet go-fmt go-staticcheck go-lint-complexity py-lint
fmt: py-fmt
	$(GO) fmt ./...

# ---------------------------------------------------------------------------
# Go
# ---------------------------------------------------------------------------

go-build:
	$(GO) build -o $(BINARY) ./cmd/brickmesh

go-test:
	$(GO) test ./...

go-vet:
	$(GO) vet ./...

# `go fmt` rewrites; this only reports, so CI fails instead of silently fixing.
go-fmt:
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then \
		echo "gofmt needed on:"; echo "$$out"; exit 1; \
	fi

go-staticcheck:
	$(GO) run honnef.co/go/tools/cmd/staticcheck@2025.1.1 ./...

go-lint-complexity:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.2 run ./...

# ---------------------------------------------------------------------------
# Python
# ---------------------------------------------------------------------------

py-sync:
	$(UV) sync --frozen

py-lock:
	$(UV) lock

# Tests cover the pure-logic modules, which need numpy and nothing heavier, so
# the geometry extra stays out of the default test install.
py-test:
	$(UV) run --frozen pytest -rs

py-lint:
	$(UV) run --frozen ruff check .

py-fmt:
	$(UV) run --frozen ruff check --fix .

clean:
	rm -rf $(BINDIR) .pytest_cache
	find . -name __pycache__ -type d -prune -exec rm -rf {} +
