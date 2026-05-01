POSTGRES_URL ?= postgres://video:video@localhost:5433/accounts?sslmode=disable
APP_NAME ?= moneo-api

OPENAPI_SRC ?= api/openapi.yaml
OPENAPI_BUNDLE ?= api/bundled/openapi.yaml
OPENAPI_GEN_CONFIG ?= oapi-codegen.yaml
OPENAPI_GEN_FILE ?= internal/transport/http/generated/api.gen.go

.PHONY: swag_init create-migration migrate-up migrate-down migrate-status migrate-version db-init
.PHONY: ops-repair-transactions-dry-run ops-repair-transactions
.PHONY: fmt vet test build check
.PHONY: openapi-lint openapi-bundle openapi-generate openapi openapi-check

swag_init:
	swag init --parseInternal -g main.go -o docs -d cmd/server,internal/transport/http,internal/app/accounts,internal/domain/accounts,internal/domain/deals,internal/infra/postgres
	@awk '!/LeftDelim:|RightDelim:/' docs/docs.go > docs/docs.go.tmp && mv docs/docs.go.tmp docs/docs.go

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

build:
	mkdir -p bin
	go build -o bin/$(APP_NAME) ./cmd/api

check: fmt vet openapi-lint openapi-generate test build

openapi-lint:
	npx @redocly/cli lint $(OPENAPI_SRC)

openapi-bundle:
	mkdir -p api/bundled
	npx @redocly/cli bundle $(OPENAPI_SRC) -o $(OPENAPI_BUNDLE)

openapi-generate: openapi-bundle
	mkdir -p internal/transport/http/generated
	oapi-codegen -config $(OPENAPI_GEN_CONFIG) $(OPENAPI_BUNDLE) > $(OPENAPI_GEN_FILE)

openapi: openapi-lint openapi-generate

openapi-check: openapi
	git diff --exit-code $(OPENAPI_GEN_FILE) $(OPENAPI_BUNDLE)

create-migration:
	@if [ -z "$(NAME)" ]; then echo "NAME is required: make create-migration NAME=add_new_table"; exit 1; fi
	go run ./cmd/migrate create $(NAME)

migrate-up:
	POSTGRES_URL='$(POSTGRES_URL)' go run ./cmd/migrate up

migrate-down:
	POSTGRES_URL='$(POSTGRES_URL)' go run ./cmd/migrate down

migrate-status:
	POSTGRES_URL='$(POSTGRES_URL)' go run ./cmd/migrate status

migrate-version:
	POSTGRES_URL='$(POSTGRES_URL)' go run ./cmd/migrate version

db-init:
	POSTGRES_URL='$(POSTGRES_URL)' go run ./cmd/dbinit

ops-repair-transactions-dry-run:
	POSTGRES_URL='$(POSTGRES_URL)' go run ./cmd/ops repair transactions-format --dry-run

ops-repair-transactions:
	POSTGRES_URL='$(POSTGRES_URL)' go run ./cmd/ops repair transactions-format
