# AI Content Studio — developer entry points. Run `make help`.
SHELL := /bin/bash
API   := api
WEB   := web
SQLC_VERSION := v1.30.0
GOLANGCI_VERSION := v2.15.0
COMPOSE := docker compose
GO_TEST_PKGS := ./... ./internal/modules/_template ./internal/modules/_template/domain ./internal/modules/_template/usecase ./internal/modules/_template/adapter/http

-include .env
export

.PHONY: help up up-app down down-v logs migrate migrate-down migrate-status admin seed \
        run-api run-worker build test test-unit test-integration lint lint-go lint-web \
        gen sqlc openapi openapi-check gen-web fmt tidy

help: ## Show targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

up: ## Start postgres, redis, minio (+bucket), mailpit
	$(COMPOSE) up -d postgres redis minio minio-init mailpit
	@echo "Postgres :5432 · Redis :6379 · MinIO :9000 (console :9001) · Mailpit :8025"

up-app: ## Start infrastructure + api + worker containers
	$(COMPOSE) --profile app up -d --build

down: ## Stop the stack (keep data)
	$(COMPOSE) --profile app down

down-v: ## Stop the stack and delete volumes
	$(COMPOSE) --profile app down -v

logs: ## Tail container logs
	$(COMPOSE) logs -f --tail=100

migrate: ## Apply migrations
	cd $(API) && go run ./cmd/acs migrate up

migrate-down: ## Roll back the latest migration
	cd $(API) && go run ./cmd/acs migrate down

migrate-status: ## Show migration status
	cd $(API) && go run ./cmd/acs migrate status

admin: ## Create an admin: make admin EMAIL=a@b.c (password prompted or ACS_ADMIN_PASSWORD)
	cd $(API) && go run ./cmd/acs admin create --email "$(EMAIL)" --name "$(or $(NAME),Administrator)"

seed: ## Seed a local demo admin (admin@acs.local / admin12345!)
	cd $(API) && go run ./cmd/acs seed demo

run-api: ## Run the API on :8080
	cd $(API) && go run ./cmd/server

run-worker: ## Run a worker (QUEUES=text,image to restrict)
	cd $(API) && go run ./cmd/worker $(if $(QUEUES),--queues=$(QUEUES),)

build: ## Build Go binaries into api/bin
	cd $(API) && go build -o bin/ ./cmd/...

test: test-unit ## Run Go + web unit tests (no Docker needed)
	@if [ -f $(WEB)/package.json ]; then cd $(WEB) && pnpm test; fi

test-unit: ## Go tests; integration tests auto-skip without Docker
	cd $(API) && go test -race -count=1 $(GO_TEST_PKGS)

test-integration: ## Go tests incl. testcontainers integration tests (requires Docker)
	@docker info >/dev/null 2>&1 || { echo "Docker is not running"; exit 1; }
	cd $(API) && go test -count=1 $(GO_TEST_PKGS)

lint: lint-go lint-web ## Lint everything

lint-go: ## golangci-lint (depguard architecture rules) + vet + arch test
	cd $(API) && go vet ./... && go test ./internal/archtest/ && \
		go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run ./...

lint-web: ## ESLint (boundaries) + typecheck
	@if [ -f $(WEB)/package.json ]; then cd $(WEB) && pnpm lint && pnpm typecheck; fi

gen: sqlc openapi gen-web ## Regenerate sqlc code, openapi.yaml and web API types

sqlc: ## Generate sqlc code (pinned version)
	cd $(API) && go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

openapi: ## Bundle api/openapi/*.yaml → api/openapi.yaml
	cd $(API) && go run ./tools/openapi-bundle

openapi-check: ## Fail if api/openapi.yaml is stale
	cd $(API) && go run ./tools/openapi-bundle -check

gen-web: ## Generate web/src/shared/api/schema.d.ts from api/openapi.yaml
	@if [ -f $(WEB)/package.json ]; then cd $(WEB) && pnpm gen:api; fi

fmt: ## gofmt
	cd $(API) && gofmt -w cmd internal migrations tools

tidy: ## go mod tidy
	cd $(API) && go mod tidy
