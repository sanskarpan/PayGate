.PHONY: build test test-integration lint migrate migrate-up migrate-down migrate-status infra-up infra-down dev docker

GO ?= go
MIGRATIONS_DIR := migrations
DB_URL ?= postgres://paygate:paygate@localhost:5432/paygate?sslmode=disable
GOCACHE ?= $(CURDIR)/.gocache
GOMODCACHE ?= $(CURDIR)/.gomodcache
GOLANGCI_LINT_CACHE ?= $(CURDIR)/.golangci-cache

build:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) build ./...

test:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) test ./...

test-integration:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) test -tags=integration ./...

lint:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) GOLANGCI_LINT_CACHE=$(GOLANGCI_LINT_CACHE) golangci-lint run --config .golangci.yml ./...

migrate: migrate-up

migrate-up:
	migrate -path $(MIGRATIONS_DIR) -database '$(DB_URL)' up

migrate-down:
	migrate -path $(MIGRATIONS_DIR) -database '$(DB_URL)' down 1

migrate-status:
	migrate -path $(MIGRATIONS_DIR) -database '$(DB_URL)' version

# Bring up all infrastructure containers and wait for them to be healthy.
infra-up:
	docker compose up -d
	@echo "Waiting for postgres to be healthy..."
	@until docker compose exec postgres pg_isready -U paygate -d paygate > /dev/null 2>&1; do sleep 1; done
	@echo "Infrastructure is ready."

# Tear down all infrastructure containers and remove volumes.
infra-down:
	docker compose down -v

dev:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) run ./cmd/api-gateway

docker:
	docker compose up -d --build
