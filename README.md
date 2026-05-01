# Moneo 
## помогает заранее планировать д оходы, расходы, долги, накопления и инвестиции, а затем показывает дневной лимит, чтобы месяц закончился в плюсе.

## OpenAPI workflow

- `make openapi-lint` — lint OpenAPI contract via Redocly.
- `make openapi` — lint + bundle + generate `oapi-codegen` output.
- `make openapi-generate` — regenerate `internal/transport/http/generated/api.gen.go` from bundled OpenAPI.
- `make check` — full local gate in order: `fmt -> vet -> openapi-lint -> openapi-generate -> test -> build`.
- `make openapi-check` — same as `openapi`, then verifies no generation drift via `git diff --exit-code`.
- `internal/transport/http/generated/api.gen.go` is generated code and must not be edited manually.
- `docs/openapi-format-compatibility-matrix.md` — migration compatibility rules for field formats (money/date/type/status).
- `docs/openapi-dual-compatibility-rollout.md` — phased dual-read/dual-write rollout and rollback strategy.

## Ops repair jobs

- `make ops-repair-transactions-dry-run` — scan historical transactions and print repair report without DB changes.
- `make ops-repair-transactions` — run idempotent historical transactions repair.
- Direct CLI: `go run ./cmd/ops repair transactions-format --dry-run --batch-size=500 --limit=0`.
