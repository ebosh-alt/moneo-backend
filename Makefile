POSTGRES_URL ?= postgres://video:video@localhost:5433/accounts?sslmode=disable
APP_NAME ?= moneo-api
VERIFY_BASELINE_OUT ?= /tmp/moneo-transactions-verify-baseline.json
VERIFY_BASELINE_IN ?= /tmp/moneo-transactions-verify-baseline.json
VERIFY_REPORT_FILE ?= /tmp/moneo-transactions-verify-report.md

OPENAPI_SRC ?= api/openapi.yaml
OPENAPI_BUNDLE ?= api/bundled/openapi.yaml
OPENAPI_GEN_CONFIG ?= oapi-codegen.yaml
OPENAPI_GEN_FILE ?= internal/transport/http/generated/api.gen.go

.PHONY: create-migration migrate-up migrate-down migrate-status migrate-version db-init
.PHONY: ops-repair-transactions-dry-run ops-repair-transactions
.PHONY: ops-verify-transactions-baseline ops-verify-transactions
.PHONY: fmt vet test build check verify
.PHONY: openapi-lint openapi-bundle openapi-generate openapi openapi-guard openapi-check openapi-docs

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
	npx --yes @redocly/cli lint $(OPENAPI_SRC)

openapi-bundle:
	mkdir -p api/bundled
	npx --yes @redocly/cli bundle $(OPENAPI_SRC) -o $(OPENAPI_BUNDLE)

openapi-generate: openapi-bundle
	mkdir -p internal/transport/http/generated
	oapi-codegen -config $(OPENAPI_GEN_CONFIG) $(OPENAPI_BUNDLE)

openapi: openapi-lint openapi-generate

openapi-guard:
	@test -f $(OPENAPI_GEN_FILE)
	@rg -n "DO NOT EDIT" $(OPENAPI_GEN_FILE) >/dev/null
	@! rg -n "httptest\\.NewRecorder\\(" internal/transport/http --glob '!**/*_test.go'
	@! rg -n "gin\\.CreateTestContext\\(" internal/transport/http --glob '!**/*_test.go'
	@! rg -n "MethodByName\\(" internal/transport/http --glob '!**/*_test.go'

openapi-check: openapi openapi-guard
	git diff --exit-code $(OPENAPI_GEN_FILE) $(OPENAPI_BUNDLE)

openapi-docs:
	mkdir -p docs
	npx --yes @redocly/cli build-docs $(OPENAPI_SRC) -o docs/api.html

verify: openapi-check fmt vet test build

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

ops-verify-transactions-baseline:
	POSTGRES_URL='$(POSTGRES_URL)' go run ./cmd/ops repair transactions-verify --baseline-out='$(VERIFY_BASELINE_OUT)'

ops-verify-transactions:
	POSTGRES_URL='$(POSTGRES_URL)' go run ./cmd/ops repair transactions-verify --baseline-in='$(VERIFY_BASELINE_IN)' --report-file='$(VERIFY_REPORT_FILE)'
