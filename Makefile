GO      ?= go
NPM     ?= npm
BINARY  ?= bullpen

.PHONY: all build build-go build-web dev-web test test-go test-ts run run-plane fmt vet clean

all: build

build: build-web build-go

build-go:
	$(GO) build -o $(BINARY) .

# The dashboard bundle is embedded by internal/web, so it must exist before the
# Go binary is built.
build-web:
	$(NPM) --prefix web install --no-audit --no-fund
	$(NPM) --prefix web run build

# Vite dev server with HMR, proxying /v1 to a `bullpen server` on :7070.
dev-web:
	$(NPM) --prefix web run dev

run: build
	./$(BINARY) server

test: test-go test-web

test-go:
	$(GO) test ./...

test-web:
	$(NPM) --prefix web run typecheck

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

clean:
	rm -f $(BINARY)
	$(NPM) --prefix ts run clean
	rm -rf internal/web/dist/assets internal/web/dist/index.html
