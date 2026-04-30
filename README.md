# Moneo 
## помогает заранее планировать д оходы, расходы, долги, накопления и инвестиции, а затем показывает дневной лимит, чтобы месяц закончился в плюсе.

## OpenAPI workflow

- `make openapi-lint` — lint OpenAPI contract via Redocly.
- `make openapi` — lint + bundle + generate `oapi-codegen` output.
- `make check` — full local gate in order: `fmt -> vet -> openapi-lint -> openapi-generate -> test -> build`.
- `make openapi-check` — same as `openapi`, then verifies no generation drift via `git diff --exit-code`.
