# Local commands. Run `make help` for the list.
# The test database is a throwaway Postgres in Docker on port 54329.

TEST_DATABASE_URL ?= postgres://postgres:postgres@localhost:54329/talent_bridge?sslmode=disable
VERSION := $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)

.PHONY: help db db-stop generate lint test web-test build run dev e2e docker check

help: ## List the commands
	@grep -E '^[a-z0-9-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  make %-10s %s\n", $$1, $$2}'

db: ## Start the local test Postgres (Docker)
	@docker start tb-pg >/dev/null 2>&1 || docker run -d --name tb-pg -e POSTGRES_PASSWORD=postgres \
		-e POSTGRES_DB=talent_bridge -p 54329:5432 postgres:18-alpine >/dev/null
	@until docker exec tb-pg pg_isready -U postgres >/dev/null 2>&1; do sleep 0.5; done
	@echo "Postgres is ready on localhost:54329"

db-stop: ## Stop the local test Postgres
	docker stop tb-pg

generate: ## Regenerate Go code from db/queries (sqlc)
	sqlc generate

lint: ## Go lint + format check, TypeScript type check
	golangci-lint run ./...
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	cd web && npm run typecheck

test: db ## Go tests, with the database tests
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -race ./...

web-test: ## Vitest unit tests for the page
	cd web && npm test

build: ## Build the page and the server binary
	cd web && npm run build
	go build -ldflags "-X main.version=$(VERSION)" -o bin/server ./cmd/server

run: build ## Run the server on :8080 with DATABASE_URL from .env
	@# .env is read as plain KEY=value lines, not as shell code: a Neon URL contains "&".
	env $$([ -f .env ] && grep -v '^[[:space:]]*#' .env | xargs) ./bin/server

dev: ## Vite dev server on :5173 (run `make run` in another terminal for the API)
	cd web && npm run dev

e2e: db build ## Playwright smoke test against the real server
	cd web && TEST_DATABASE_URL="$(TEST_DATABASE_URL)" npx playwright test

docker: ## Build the production image
	docker build --build-arg VERSION=$(VERSION) -t talent-bridge:local .

check: lint test web-test e2e ## Everything CI runs
