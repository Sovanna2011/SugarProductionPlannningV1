# Sugar Production Planning — developer entry points.
#
# The database connection defaults match docker-compose; override on the
# command line, e.g. `make test DB_HOST=db`.

DB_HOST ?= 127.0.0.1
DB_PORT ?= 5432
DB_USER ?= sugar
DB_PASSWORD ?= sugar
DB_NAME ?= sugar_dev

BACKEND := backend
SCHEMA  := schema.sql

.PHONY: help
help: ## Show the available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: run
run: ## Run the API against a local PostgreSQL
	cd $(BACKEND) && go run ./cmd/api

.PHONY: web
web: ## Serve the UI5 frontend on :8081, proxying /api to :8080
	node frontend/dev-server.js

.PHONY: build
build: ## Compile the API binary
	cd $(BACKEND) && go build -trimpath -o bin/api ./cmd/api

.PHONY: test
test: ## Run every test, including the integration suite
	cd $(BACKEND) && TEST_DB_HOST=$(DB_HOST) TEST_DB_PORT=$(DB_PORT) \
		TEST_DB_USER=$(DB_USER) TEST_DB_PASSWORD=$(DB_PASSWORD) \
		go test ./... -count=1

.PHONY: unit
unit: ## Run only the tests that need no database
	cd $(BACKEND) && go test ./internal/... -count=1

.PHONY: cover
cover: ## Report statement coverage per package
	@TEST_DB_HOST=$(DB_HOST) TEST_DB_PORT=$(DB_PORT) TEST_DB_USER=$(DB_USER) \
		TEST_DB_PASSWORD=$(DB_PASSWORD) $(BACKEND)/scripts/coverage.sh

.PHONY: lint
lint: ## Vet and check formatting
	cd $(BACKEND) && go vet ./...
	@unformatted=$$(gofmt -l $(BACKEND)); \
	if [ -n "$$unformatted" ]; then echo "not gofmt-ed:"; echo "$$unformatted"; exit 1; fi
	@echo "formatting and vet are clean"

.PHONY: schema
schema: ## Regenerate schema.sql from the migrations
	@printf -- '-- GENERATED FILE — do not edit.\n' > $(SCHEMA)
	@printf -- '-- Concatenation of backend/migrations/*.up.sql, produced by `make schema`.\n' >> $(SCHEMA)
	@printf -- '-- The migrations are the source of truth; this file is the readable\n' >> $(SCHEMA)
	@printf -- '-- companion the specification refers to.\n\n' >> $(SCHEMA)
	@for file in $(BACKEND)/migrations/*.up.sql $(BACKEND)/migrations/demo/*.up.sql; do \
		printf -- '\n-- ===== %s =====\n' "$$(basename $$file)" >> $(SCHEMA); \
		cat "$$file" >> $(SCHEMA); \
	done
	@echo "wrote $(SCHEMA)"

.PHONY: up
up: ## Start the whole stack with docker compose
	docker compose up --build

.PHONY: down
down: ## Stop the stack and drop its volumes
	docker compose down -v
