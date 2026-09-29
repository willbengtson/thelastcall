BINARY := bin/thelastcall
GO     ?= go
NPM    ?= npm

.DEFAULT_GOAL := help

## help: list the targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | awk -F: '{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

## deps: install frontend dependencies
deps:
	cd web && $(NPM) install

## web: build the frontend into internal/webui/dist
web:
	cd web && $(NPM) run build

## build: build the frontend, then the server binary that embeds it
build: web
	$(GO) build -o $(BINARY) ./cmd/server

## run: run the server against the in-memory store
run:
	$(GO) run ./cmd/server

## run-dynamo: run the server against DynamoDB Local (needs `make dynamo`)
run-dynamo:
	TLC_STORE=dynamo \
	TLC_DYNAMO_ENDPOINT=http://localhost:8042 \
	TLC_DYNAMO_AUTOCREATE=true \
	AWS_REGION=us-west-2 \
	AWS_ACCESS_KEY_ID=local \
	AWS_SECRET_ACCESS_KEY=local \
	$(GO) run ./cmd/server

## dev: run the API and the Vite dev server together, with hot reload
dev:
	TLC_ALLOWED_ORIGINS=http://localhost:5173 $(GO) run ./cmd/server & \
	cd web && $(NPM) run dev; \
	kill %1 2>/dev/null || true

## dynamo: start DynamoDB Local on :8042
dynamo:
	docker compose up -d

## dynamo-down: stop DynamoDB Local
dynamo-down:
	docker compose down

## test: run the Go tests under the race detector
test:
	$(GO) test -race ./...

## check: vet, test, and typecheck everything
check: test
	$(GO) vet ./...
	gofmt -l cmd internal
	cd web && $(NPM) run typecheck

## clean: remove build output
clean:
	rm -rf $(BINARY) internal/webui/dist/assets internal/webui/dist/index.html

.PHONY: help deps web build run run-dynamo dev dynamo dynamo-down test check clean
