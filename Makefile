POSTGRES_URL ?= postgres://video:video@localhost:5433/accounts?sslmode=disable

.PHONY: swag_init create-migration migrate-up migrate-down migrate-status migrate-version db-init

swag_init:
	swag init --parseInternal -g main.go -o docs -d cmd/server,internal/transport/http,internal/app/accounts,internal/domain/accounts,internal/domain/deals,internal/infra/postgres
	@awk '!/LeftDelim:|RightDelim:/' docs/docs.go > docs/docs.go.tmp && mv docs/docs.go.tmp docs/docs.go

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
