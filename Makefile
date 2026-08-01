.PHONY: build web-build build-production dev dev-fake web-dev fmt fmt-check lint lint-go vet test web-check check openapi api api-check run run-fake

build:
	go build ./cmd/log-leopard

web-build:
	pnpm --dir web run build

build-production: web-build
	go build -tags production -o log-leopard ./cmd/log-leopard

dev:
	LOG_LEOPARD_BROWSER="$(BROWSER)" sh ./scripts/dev.sh

dev-fake:
	LOG_LEOPARD_BROWSER="$(BROWSER)" sh ./scripts/dev.sh -fake

web-dev:
	pnpm --dir web exec vite --config "$(CURDIR)/vite.dev.config.mjs"

fmt:
	go fmt ./...
	pnpm --dir web run format

fmt-check:
	test -z "$$(gofmt -l ./cmd ./internal)"
	pnpm --dir web run format:check

lint: lint-go
	pnpm --dir web run lint

lint-go:
	golangci-lint run

vet:
	go vet ./...

test:
	go test ./...

web-check:
	pnpm --dir web run format:check
	pnpm --dir web run lint
	pnpm --dir web run typecheck
	pnpm --dir web run test
	pnpm --dir web run build
	go test -tags production ./...
	go -C web test -tags production .

check: fmt-check lint-go vet test web-check api-check

openapi:
	go run ./cmd/log-leopard -write-openapi openapi.json

api: openapi
	pnpm --dir web run generate:api

api-check:
	@tmp="$$(mktemp)"; trap 'rm -f "$$tmp"' EXIT; \
		go run ./cmd/log-leopard -write-openapi "$$tmp" && \
		diff -u openapi.json "$$tmp" && \
		pnpm --dir web run check:api

run: web-build
	go run -tags production ./cmd/log-leopard

run-fake: web-build
	go run -tags production ./cmd/log-leopard -fake
