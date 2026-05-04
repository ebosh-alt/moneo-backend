# Матрица совместимости форматов API (legacy -> strict OpenAPI)

## 1. Контекст и цель

Документ фиксирует правила форматов данных на период миграции OpenAPI-first.

- `legacy` слой: исторические маршруты (`/accounts`, `/transactions`, ...).
- `strict` слой: generated-интерфейс OpenAPI (`/api/v1/...`) + validator.

Цель: одинаковые правила преобразования и одинаковое поведение при ошибках формата.

## 2. Базовые принципы совместимости

- Публичный формат полей для `legacy` и `strict` должен быть эквивалентным.
- Внутренний формат денег в домене: `int64` minor units.
- Ошибки формата возвращаются как `400 validation_error`.
- Канонический формат ответа всегда нормализуется transport-слоем.

## 3. Матрица: деньги и валюта

| Поле | Legacy input | Strict input | Преобразование | Ограничения | Output |
|---|---|---|---|---|---|
| `accounts.initialBalance` (create) | decimal-string | decimal-string | `"123.45"` -> `12345` minor | только JSON string; без пробелов/запятых; <= 2 знаков после точки; `>= 0` | всегда с 2 знаками (`"123.45"`) |
| `accounts.balance` (PATCH) | запрещено | запрещено | не применяется | immutable поле | не меняется через PATCH |
| `transactions.amount` (create/patch/bulk) | decimal-string | decimal-string | `"900.00"` -> `90000` minor | только JSON string; `> 0`; для MVP1 валюта только `RUB` | всегда с 2 знаками (`"900.00"`) |
| `currency` (catalogs) | `RUB|USD|EUR` | `RUB|USD|EUR` | string -> `shared.Currency` | строго enum, регистр чувствителен | как в домене (`RUB/ USD/ EUR`) |
| `currency` (transactions) | `RUB` | `RUB` | string -> `shared.CurrencyRUB` | любое значение кроме `RUB` -> `400` | `RUB` |

## 4. Матрица: даты и время

| Поле | Legacy input | Strict input | Преобразование | Ограничения | Output |
|---|---|---|---|---|---|
| `transactions.occurredAt` | `YYYY-MM-DD` или null/omit | `YYYY-MM-DD` или null/omit | parse date -> UTC day (`00:00:00Z`) | неверный формат -> `400` | строка `YYYY-MM-DD` |
| `transactions.plannedAt` | `YYYY-MM-DD` или null/omit | `YYYY-MM-DD` или null/omit | parse date -> UTC day (`00:00:00Z`) | неверный формат -> `400` | строка `YYYY-MM-DD` |
| query `month` | `YYYY-MM` | `YYYY-MM` (regex в OpenAPI) | `month` -> `[from,to]` за месяц в UTC | неверный формат -> `400` | влияет на фильтр list |
| query `from`/`to` | `YYYY-MM-DD` | `YYYY-MM-DD` | `from` = start-of-day UTC; `to` = end-of-day UTC | неверный формат -> `400` | влияет на фильтр list |
| `createdAt/updatedAt/archivedAt` | не принимаются в input | не принимаются в input | доменные `time.Time` -> JSON | transport-only output fields | RFC3339 в JSON |

## 5. Матрица: типы и статусы

| Поле | Legacy | Strict | Ограничения/правила |
|---|---|---|---|
| `account.type` | `cash,debit_card,savings,brokerage,credit_card,deposit,debt,other` | те же | регистр и значение строго фиксированы |
| `category.type` | `required,flexible,saving,investment,debt,income` | те же | строго enum |
| `transaction.type` | `income,expense,transfer,investment,saving` | те же | строго enum |
| `transaction.status` (create) | `planned|posted`, default `planned` | те же | `cancelled` на create запрещен |
| `transaction.status` (patch/list) | `planned|posted|cancelled` | те же | строго enum |
| `duplicate.status` | `planned|posted` | те же | `cancelled` запрещен |

## 6. Допустимые преобразования (нормализация)

| Сценарий | Поведение |
|---|---|
| `comment`, `accountId`, `categoryId`, `subcategoryId` как пустая строка | нормализуется в `nil` |
| `occurredAt` / `plannedAt` как пустая строка | нормализуется в `nil` |
| PATCH optional-поля | `omit` и `null` различаются; `null` для некоторых полей => валидационная ошибка |
| Денежный input с ведущими нулями (`"001.20"`) | допустим, в output канонизируется (`"1.20"`) |

## 7. Edge-cases и ожидаемое поведение

| Edge-case | Ожидаемое поведение |
|---|---|
| `amount`/`initialBalance` передан числом (`100.5`) | `400 validation_error` (money должен быть JSON string) |
| money со строкой `"100,50"` или пробелами | `400 validation_error` |
| `transactions.amount = "0.00"` | `400 validation_error` (требуется положительная сумма) |
| `accounts.initialBalance = "0.00"` | допустимо |
| query `limit > 200` | clamp до `200` |
| query `limit <= 0` или `offset < 0` | `400 validation_error` |
| reserved refs (`budgetMemberId`, `incomeSourceId`, `debtId`, `goalId`, `investmentId`, `recurringPaymentId`) не `null` | `400 validation_error` (MVP1 not supported) |

## 8. Правила для команды на период миграции

- Все новые изменения формата сначала фиксируются в OpenAPI.
- Любое изменение формата сопровождается обновлением этой матрицы.
- Для money/date/type/status нельзя вводить "мягкие" неявные преобразования без обновления контракта.
- Контрактные тесты должны покрывать happy-path и negative-path по этой матрице.
