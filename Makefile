# brickmesh — a Go engine, and a little Python that tests it.
#
# The engine was Python once and was ported; docs/findings.md records what each
# port was checked against. What is left in tests/ runs the LDCad animation
# script the engine writes, under a real Lua runtime, because nothing on the Go
# side can tell whether the text it emits works.

.PHONY: all build test lint fmt clean \
        go-build go-test go-vet go-fmt go-staticcheck go-lint-complexity \
        py-sync py-test py-lint py-fmt py-lock \
        web serve assets help

GO      ?= go
UV      ?= uv
BINDIR  ?= bin
BINARY  ?= $(BINDIR)/brickmesh
# Which parts go into the browser assets: 1 common, 2 all Technic, 3 everything.
ASSET_TIER ?= 2

all: build

help:
	@echo "build   - build the Go binary"
	@echo "test    - run Go and Python tests"
	@echo "lint    - vet, gofmt check, staticcheck, complexity lens, ruff"
	@echo "fmt     - format Go and Python sources in place"
	@echo "web     - build the browser calculator into web/"
	@echo "assets  - generate catalog.bin and meshes.bin into web/data"
	@echo "serve   - build it and serve web/ on http://localhost:8080"
	@echo "clean   - remove build output and caches"

build: go-build

# The browser calculator: the functional layer, compiled to WebAssembly.
#
# Only the functional layer, which is why it needs no parts library and no
# network beyond its own directory. wasm_exec.js is vendored from the Go
# distribution and has to match the compiler that built the module, so it is
# copied on every build rather than committed once and forgotten.
web: web/brickmesh.wasm web/wasm_exec.js web/examples

web/brickmesh.wasm: $(shell find internal cmd -name '*.go' 2>/dev/null)
	GOOS=js GOARCH=wasm $(GO) build -trimpath -ldflags="-s -w" \
		-o $@ ./cmd/brickmesh-wasm

web/wasm_exec.js: FORCE
	@cp "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" $@ 2>/dev/null || \
		cp "$$($(GO) env GOROOT)/misc/wasm/wasm_exec.js" $@

# Symlinked rather than copied so the page always shows the examples the tests
# and the command line use.
web/examples: FORCE
	@test -e $@ || ln -s ../examples $@

FORCE:

# The two files a browser build downloads. Minutes of work and tens of
# megabytes, so it is never a dependency of anything: run it when the libraries
# change, not on every build.
#
# What it writes is derived from the LDCad shadow library and the LDraw parts
# library, so publishing it carries their terms. See ATTRIBUTION.md.
assets:
	$(GO) run ./cmd/brickmesh-assets --out web/data --tier $(ASSET_TIER)

serve: web
	@echo "http://localhost:8080 — WebAssembly needs http, not file://"
	@cd web && python3 -m http.server 8080
test: go-test py-test
lint: go-vet go-fmt go-staticcheck go-lint-complexity py-lint
fmt: py-fmt
	$(GO) fmt ./...

# ---------------------------------------------------------------------------
# Go
# ---------------------------------------------------------------------------

go-build:
	$(GO) build -o $(BINARY) ./cmd/brickmesh
	$(GO) build -o $(BINDIR)/brickmesh-extract ./cmd/brickmesh-extract

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

# Runs the emitted Lua under lupa. See tests/README.md.
py-test:
	$(UV) run --frozen pytest -rs

py-lint:
	$(UV) run --frozen ruff check .

py-fmt:
	$(UV) run --frozen ruff check --fix .

clean:
	rm -rf $(BINDIR) .pytest_cache web/brickmesh.wasm web/examples
	find . -name __pycache__ -type d -prune -exec rm -rf {} +
