# OpenAPI Migration Report

## Existing Routes

Runtime registration now happens in `internal/transport/http/router.go` via `generated.RegisterHandlersWithOptions`. The legacy `CatalogHandler` and transaction methods remain as transport implementation/mapping helpers, but duplicate manual Gin route registration for catalogs/transactions was not found.

| Group | Method | Path | Current Handler | Request DTO | Response DTO | Auth | Notes |
|---|---|---|---|---|---|---|---|
| Catalogs / Accounts | GET | `/api/v1/accounts` | `APIHandler.ListAccounts` | query: `ListAccountsParams` | `PaginatedAccounts` | Bearer | Supports `limit`, `offset`, `includeArchived`, `type`, `currency`, `sort`. |
| Catalogs / Accounts | POST | `/api/v1/accounts` | `APIHandler.CreateAccount` | `CreateAccountRequest` | `Account` | Bearer | Returns `201`; money fields are strings. |
| Catalogs / Accounts | GET | `/api/v1/accounts/summary` | `APIHandler.GetAccountsSummary` | query: `GetAccountsSummaryParams` | `AccountSummary` | Bearer | `currency` is required. |
| Catalogs / Accounts | GET | `/api/v1/accounts/{accountId}` | `APIHandler.GetAccount` | path: `accountId` | `Account` | Bearer | ID is a string. |
| Catalogs / Accounts | PATCH | `/api/v1/accounts/{accountId}` | `APIHandler.PatchAccount` | `PatchAccountRequest` | `Account` | Bearer | Partial update. |
| Catalogs / Accounts | POST | `/api/v1/accounts/{accountId}/archive` | `APIHandler.ArchiveAccount` | path: `accountId` | `Account` | Bearer | Soft archive. |
| Catalogs / Accounts | POST | `/api/v1/accounts/{accountId}/restore` | `APIHandler.RestoreAccount` | path: `accountId` | `Account` | Bearer | Can return conflict on duplicate restored name. |
| Catalogs / Categories | GET | `/api/v1/categories` | `APIHandler.ListCategories` | query: `ListCategoriesParams` | `PaginatedCategories` | Bearer | Supports `includeSubcategories`. |
| Catalogs / Categories | POST | `/api/v1/categories` | `APIHandler.CreateCategory` | `CreateCategoryRequest` | `Category` | Bearer | Returns `201`. |
| Catalogs / Categories | GET | `/api/v1/categories/{categoryId}` | `APIHandler.GetCategory` | path: `categoryId`, query: `includeSubcategories` | `Category` | Bearer | ID is a string. |
| Catalogs / Categories | PATCH | `/api/v1/categories/{categoryId}` | `APIHandler.PatchCategory` | `PatchCategoryRequest` | `Category` | Bearer | Partial update. |
| Catalogs / Categories | DELETE | `/api/v1/categories/{categoryId}` | `APIHandler.DeleteCategory` | path: `categoryId` | `Category` | Bearer | Current behavior is soft delete returning `200` and archived category. |
| Catalogs / Categories | POST | `/api/v1/categories/{categoryId}/restore` | `APIHandler.RestoreCategory` | path: `categoryId` | `Category` | Bearer | Can return conflict. |
| Catalogs / Subcategories | GET | `/api/v1/categories/{categoryId}/subcategories` | `APIHandler.ListCategorySubcategories` | path: `categoryId`, query: `limit`, `offset`, `includeArchived` | `PaginatedSubcategories` | Bearer | Category-scoped list. |
| Catalogs / Subcategories | POST | `/api/v1/categories/{categoryId}/subcategories` | `APIHandler.CreateSubcategory` | `CreateSubcategoryRequest` | `Subcategory` | Bearer | Returns `201`; can return `422` when parent category is archived. |
| Catalogs / Subcategories | GET | `/api/v1/subcategories` | `APIHandler.ListSubcategories` | query: `ListSubcategoriesParams` | `PaginatedSubcategories` | Bearer | Supports optional `categoryId` and `sort` in contract. |
| Catalogs / Subcategories | GET | `/api/v1/subcategories/{subcategoryId}` | `APIHandler.GetSubcategory` | path: `subcategoryId` | `Subcategory` | Bearer | ID is a string. |
| Catalogs / Subcategories | PATCH | `/api/v1/subcategories/{subcategoryId}` | `APIHandler.PatchSubcategory` | `PatchSubcategoryRequest` | `Subcategory` | Bearer | Partial update. |
| Catalogs / Subcategories | DELETE | `/api/v1/subcategories/{subcategoryId}` | `APIHandler.DeleteSubcategory` | path: `subcategoryId` | `Subcategory` | Bearer | Current behavior is soft delete returning `200` and archived subcategory. |
| Catalogs / Subcategories | POST | `/api/v1/subcategories/{subcategoryId}/restore` | `APIHandler.RestoreSubcategory` | path: `subcategoryId` | `Subcategory` | Bearer | Can return `422` when parent category is archived. |
| Transactions | GET | `/api/v1/transactions` | `APIHandler.ListTransactions` | query: `ListTransactionsParams` | `PaginatedTransactions` | Bearer | Supports month/date/account/category/subcategory/status/type/search/sort filters. `budgetMemberId` is documented as currently unsupported. |
| Transactions | POST | `/api/v1/transactions` | `APIHandler.CreateTransaction` | `CreateTransactionRequest` | `Transaction` | Bearer | Returns `201`; amount is a string. |
| Transactions | POST | `/api/v1/transactions/bulk` | `APIHandler.CreateTransactionsBulk` | `CreateTransactionsBulkRequest` | `TransactionItemsResponse` | Bearer | Returns `201`. |
| Transactions | PATCH | `/api/v1/transactions/bulk` | `APIHandler.PatchTransactionsBulk` | `PatchTransactionsBulkRequest` | `TransactionItemsResponse` | Bearer | Raw body capture preserves current null-field behavior for selected fields. |
| Transactions | GET | `/api/v1/transactions/{transactionId}` | `APIHandler.GetTransaction` | path: `transactionId` | `Transaction` | Bearer | ID is a string. |
| Transactions | PATCH | `/api/v1/transactions/{transactionId}` | `APIHandler.PatchTransaction` | `PatchTransactionRequest` | `Transaction` | Bearer | Raw body capture rejects null for non-nullable patch fields while allowing nullable clears. |
| Transactions | DELETE | `/api/v1/transactions/{transactionId}` | `APIHandler.DeleteTransaction` | path: `transactionId` | no body | Bearer | Returns `204` on success. |
| Transactions | POST | `/api/v1/transactions/{transactionId}/post` | `APIHandler.PostTransaction` | optional empty JSON object | `Transaction` | Bearer | State transition endpoint. |
| Transactions | POST | `/api/v1/transactions/{transactionId}/cancel` | `APIHandler.CancelTransaction` | optional empty JSON object | `Transaction` | Bearer | State transition endpoint. |
| Transactions | POST | `/api/v1/transactions/{transactionId}/duplicate` | `APIHandler.DuplicateTransaction` | `DuplicateTransactionRequest` | `Transaction` | Bearer | Returns `201`; raw body capture rejects null `status` while allowing nullable clears. |

## Request DTOs

Generated request DTOs live in `internal/transport/http/generated/api.gen.go` and are consumed only by transport adapter code. Legacy transport DTOs remain in `internal/transport/http/catalog_handler.go` and `internal/transport/http/transactions_handler.go` to reuse current parsing/validation/mapping behavior without moving generated types into app/domain/infra.

- Accounts: `CreateAccountRequest`, `PatchAccountRequest`, `ListAccountsParams`, `GetAccountsSummaryParams`.
- Categories: `CreateCategoryRequest`, `PatchCategoryRequest`, `ListCategoriesParams`, `GetCategoryParams`.
- Subcategories: `CreateSubcategoryRequest`, `PatchSubcategoryRequest`, `ListCategorySubcategoriesParams`, `ListSubcategoriesParams`.
- Transactions: `CreateTransactionRequest`, `CreateTransactionsBulkRequest`, `PatchTransactionRequest`, `PatchTransactionsBulkRequest`, `PatchTransactionsBulkItemRequest`, `DuplicateTransactionRequest`, `ListTransactionsParams`.

## Response DTOs

- Catalogs: `Account`, `AccountSummary`, `Category`, `Subcategory`, `PaginatedAccounts`, `PaginatedCategories`, `PaginatedSubcategories`.
- Transactions: `Transaction`, `TransactionItemsResponse`, `PaginatedTransactions`.
- Errors for catalogs/transactions: `ErrorEnvelope` with nested `ErrorBody`.

## Current Error Formats

Catalogs, transactions, and OpenAPI validation errors use the current transport envelope:

```json
{
  "error": {
    "code": "validation_error",
    "message": "Validation failed",
    "details": [
      { "field": "body", "message": "request body is invalid" }
    ]
  }
}
```

Known error codes are `validation_error`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `business_rule_violation`, and `internal_error`. This differs from the initially proposed top-level `ErrorResponse` shape (`code`, `message`, `details` at the root). The contract keeps the existing envelope as the minimal safe adaptation so clients are not broken.

Auth middleware and auth endpoints still use a separate auth format:

```json
{ "error": "invalid_access_token" }
```

Security middleware can also return auth-style errors such as `https_required` and `rate_limited`. These are outside the catalogs/transactions migration scope but are documented because the middleware wraps the same Gin router.

## Middleware

- Recovery: `gin.Recovery()` in `internal/transport/http/router.go`.
- Auth security: `NewAuthSecurityMiddleware` in `internal/transport/http/auth_security.go`; handles auth rate limits, production HTTPS checks, and security event logging.
- Auth: `NewAuthMiddleware` in `internal/transport/http/auth_middleware.go`; protects all non-public paths and stores user/session in Gin context.
- OpenAPI validation: `ginmiddleware.OapiRequestValidatorWithOptions` in `registerStrictHandlers`; configured with custom `writeOpenAPIValidationError` and disabled auth validation because auth is handled by Gin middleware.
- Empty-body recovery: `recoverEmptyBadRequestBody`.
- Raw body capture: `captureRawRequestBody` for transaction patch/duplicate endpoints where null detection must preserve existing behavior.
- Logging: no general request logging middleware found; auth security emits security events through a logger.
- CORS: no CORS middleware found.

## Route Registration

Routes are registered in `internal/transport/http/router.go`.

- `NewRouterWithOptions` creates `gin.New()`.
- The auth/security middleware is attached before API routes.
- `registerStrictHandlers` loads embedded Swagger through `generated.GetSwagger()`, clears `swagger.Servers`, attaches the OpenAPI validator, wraps `APIHandler` with `generated.NewStrictHandler`, then calls `generated.RegisterHandlersWithOptions`.
- Runtime dependencies are assembled in `internal/bootstrap/api.go`.

## Layer Locations

- Handlers/adapters: `internal/transport/http`.
- Generated OpenAPI transport code: `internal/transport/http/generated/api.gen.go`.
- Application use cases: `internal/app/accounting`, `internal/app/catalog`, `internal/app/identity`.
- Domain models and invariants: `internal/domain/accounting`, `internal/domain/catalog`, `internal/domain/transactions`, `internal/domain/shared`.
- Infrastructure adapters: `internal/infra/postgres`, `internal/infra/security`, `internal/infra/clock`, `internal/infra/idgen`.
- Bootstrap/runtime assembly: `internal/bootstrap/api.go`.

## Migration Risks

- Duplicate routes: currently mitigated because catalogs/transactions are registered only through generated Gin routes; legacy handler methods remain but are not manually registered in `router.go`.
- Status-code mismatch: soft delete for categories/subcategories returns `200`, transaction delete returns `204`; this is captured in OpenAPI and should not be normalized without a separate compatibility decision.
- Error format mismatch: catalogs/transactions use nested `error` envelope, while auth middleware uses `{ "error": "..." }`; the migration preserves current behavior and documents the difference.
- Generated type leakage: generated types must remain inside `internal/transport/http`; app/domain/infra imports were checked separately.
- Validator behavior: OpenAPI request validation can reject requests before legacy manual validation. It is attached only to generated API routes and maps validation errors into the existing catalogs/transactions envelope.
- Nullable patch semantics: oapi-codegen pointer/null semantics differ from legacy optional field handling. Raw request body capture is used for transaction patch/bulk patch/duplicate null checks to preserve current behavior.

## Runtime Curl Examples

Replace `$TOKEN` and IDs with values created in the target environment.

```bash
curl -sS -H "Authorization: Bearer $TOKEN" "http://localhost:8080/api/v1/accounts?limit=50&offset=0"
curl -sS -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"Cash","type":"cash","currency":"RUB","initialBalance":"0","includeInNetWorth":true,"includeInDailyBudget":true}' \
  http://localhost:8080/api/v1/accounts
curl -sS -H "Authorization: Bearer $TOKEN" "http://localhost:8080/api/v1/categories?includeSubcategories=true"
curl -sS -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"Food","type":"expense","color":"#FFAA00","sortOrder":10}' \
  http://localhost:8080/api/v1/categories
curl -sS -H "Authorization: Bearer $TOKEN" "http://localhost:8080/api/v1/transactions?month=2026-04&limit=50&offset=0"
curl -sS -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"type":"expense","status":"planned","amount":"100.00","currency":"RUB","plannedAt":"2026-04-10","accountFromId":"acc_main","categoryId":"cat_food"}' \
  http://localhost:8080/api/v1/transactions
curl -sS -X PATCH -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"comment":"updated"}' \
  http://localhost:8080/api/v1/transactions/txn_123
curl -sS -X DELETE -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/transactions/txn_123
curl -sS -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"amount":null}' \
  http://localhost:8080/api/v1/transactions
curl -sS -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/transactions/missing_transaction
```

## Completed

- Audited current catalogs and transactions routes, DTOs, middleware, registration, and layer locations.
- Confirmed catalogs/transactions route registration goes through generated Gin handlers.
- Confirmed `APIHandler` implements generated strict handler methods and maps generated DTOs to existing app/usecase inputs.
- Confirmed generated transport code is located under `internal/transport/http/generated`.
- Added Makefile targets for OpenAPI docs and `verify`.
- Updated `oapi-codegen.yaml` so the generated output path lives in config.
- Ran OpenAPI generation/check and full Go verification successfully.
- Checked generated type leakage into `internal/app`, `internal/domain`, and `internal/infra`; result was empty.

## Runtime Checks

Automated runtime-style endpoint checks are covered by `go test ./...`, including:

- catalog strict GET endpoints;
- catalog strict write endpoints;
- transaction strict GET endpoints;
- transaction strict write endpoints for create, patch, delete, post, cancel, duplicate, and bulk operations;
- invalid OpenAPI request validation returning `400`;
- not-found application paths returning `404`;
- transaction null-field compatibility checks for patch/bulk patch/duplicate.

Manual curl checks against a running local server are still listed above because the verification run used Go tests, not a live external process.

## Changed Files

- `internal/transport/http/api_handler_generated.go` (pre-existing dirty adapter work; verified by tests)
- `internal/transport/http/router.go` (pre-existing dirty router middleware change; verified by tests)
- `internal/transport/http/request_body_capture.go` (pre-existing untracked raw body capture helper; verified by tests)
- `internal/transport/http/transactions_strict_write_endpoints_test.go` (pre-existing dirty tests; verified by tests)
- `internal/transport/http/catalog_strict_get_endpoints_test.go` (formatted by `go fmt`)
- `internal/transport/http/catalog_strict_write_endpoints_test.go` (formatted by `go fmt`)
- `internal/transport/http/transactions_strict_get_endpoints_test.go` (formatted by `go fmt`)
- `Makefile`
- `oapi-codegen.yaml`
- `docs/openapi-migration-report.md`

## Commands Run

- `git status --short` - showed pre-existing dirty files in transport code.
- `rg`/`sed` audit commands - inspected routes, handlers, DTOs, OpenAPI spec, middleware, and bootstrap wiring.
- `command -v oapi-codegen` and `oapi-codegen --version` - found `/Users/eboshit/go/bin/oapi-codegen`, version `v2.5.1`.
- `npx @redocly/cli --version` - attempted in sandbox and did not return promptly because the environment had no registry DNS access.
- `make openapi-generate` - first sandbox run failed with `ENOTFOUND registry.npmjs.org`; rerun with network escalation succeeded.
- `make openapi-lint` - first sandbox run failed with `ENOTFOUND registry.npmjs.org`; rerun with network escalation succeeded.
- `make openapi-check` - first sandbox run failed with `ENOTFOUND registry.npmjs.org`; rerun with network escalation succeeded.
- `grep`/`rg` leakage checks for generated types in `internal/app`, `internal/domain`, `internal/infra` - empty result.
- `make verify` - succeeded. This ran OpenAPI lint/bundle/generate/check, `go fmt ./...`, `go vet ./...`, `go test ./...`, and `go build -o bin/moneo-api ./cmd/api`.
- `go build ./...` - succeeded.
- `rm -rf bin` - removed the local build artifact produced by `make verify`.

## Remaining Issues

- Redocly CLI is executed through `npx --yes @redocly/cli`; it requires network access unless the npm package is already cached/installed.
- Auth endpoints and auth middleware still use `{ "error": "..." }`, while catalogs/transactions use the nested `ErrorEnvelope`. This is preserved deliberately to avoid breaking current behavior.

## Manual Checks Required

- Exercise the curl examples against a running API with a real database and valid bearer token.
- Confirm external clients tolerate OpenAPI validator errors matching the existing nested `error` envelope.
- Confirm auth middleware behavior remains acceptable for catalogs/transactions even though auth errors use a separate `{ "error": "..." }` shape.

## Final Verification

- `APIHandler` has compile-time assertion: `var _ generated.StrictServerInterface = (*APIHandler)(nil)`.
- Generated transport types are not imported from `internal/app`, `internal/domain`, or `internal/infra`.
- `package.json` pins `@redocly/cli` as a dev dependency.
- `api/bundled/openapi.yaml` is tracked by Git, so `openapi-check` continues to verify both generated Go and bundled OpenAPI artifacts.
- `make openapi-check` passed.
- `go test ./...` passed.
- `go build ./...` passed.
