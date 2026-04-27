# Catalog MVP API Contract

## 0. Общие правила

### Base URL

```http
/api/v1
```

### Auth

Все endpoints ниже защищены:

```http
Authorization: Bearer <access_token>
```

Пользователь может работать только со своими счетами, категориями и подкатегориями.

---

## 1. Money / Currency Contract

### REST format

Все денежные поля в API передаются строкой:

```json
{
  "amount": "12345.67",
  "currency": "RUB"
}
```

### Backend / DB format

Внутри backend:

```text
"12345.67" -> 1234567 minor units
```

В БД:

```text
BIGINT / int64
```

### Правила валидации money

```text
amount должен быть string
amount не должен быть float / number
amount должен быть больше или равен 0, если поле balance/initialBalance
amount должен иметь максимум 2 знака после точки для RUB
amount не должен содержать пробелы, запятые, символ валюты
```

Валидные значения:

```json
"0"
"0.00"
"100"
"100.50"
"120000.99"
```

Невалидные значения:

```json
100.50
"100,50"
"100.555"
"-100.00"
"10 000.00"
"₽100.00"
```

### Currency

Для MVP1 лучше зафиксировать только:

```text
RUB
```

Но контракт можно оставить расширяемым:

```text
RUB
USD
EUR
```

---

# 2. Общий формат ошибок

```json
{
  "error": {
    "code": "validation_error",
    "message": "Validation failed",
    "details": [
      {
        "field": "balance",
        "message": "balance must be a valid decimal string"
      }
    ]
  }
}
```

Коды:

```text
validation_error
unauthorized
forbidden
not_found
conflict
business_rule_violation
internal_error
```

Формат ошибок и пагинации уже заложен в REST API дизайн Moneo

---

# 3. Accounts API

Счета нужны для карт, наличных, вкладов, накопительных счетов, брокерских счетов и кредиток. В продуктовой логике уже заложено, что счет может учитываться в общем капитале, но не участвовать в дневном бюджете

## 3.1 Account type enum

```text
cash
debit_card
savings
brokerage
credit_card
deposit
debt
other
```

Эти типы уже зафиксированы в текущем REST-дизайне

---

## 3.2 Account object

```json
{
  "id": "acc_01HYXK7Q9N5M8V4A2Z1B3C4D5E",
  "name": "Main card",
  "type": "debit_card",
  "currency": "RUB",
  "balance": "120000.00",
  "initialBalance": "100000.00",
  "includeInNetWorth": true,
  "includeInDailyBudget": true,
  "isArchived": false,
  "archivedAt": null,
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T12:00:00Z"
}
```

---

## 3.3 GET /accounts

Получить список счетов пользователя.

```http
GET /api/v1/accounts?includeArchived=false&limit=50&offset=0&sort=createdAt:desc
```

### Query params

```text
includeArchived: boolean, default false
type: optional account type
currency: optional currency
limit: default 50
offset: default 0
sort: createdAt:desc | name:asc | balance:desc
```

### Response 200

```json
{
  "items": [
    {
      "id": "acc_main",
      "name": "Main card",
      "type": "debit_card",
      "currency": "RUB",
      "balance": "120000.00",
      "initialBalance": "100000.00",
      "includeInNetWorth": true,
      "includeInDailyBudget": true,
      "isArchived": false,
      "archivedAt": null,
      "createdAt": "2026-04-27T12:00:00Z",
      "updatedAt": "2026-04-27T12:00:00Z"
    }
  ],
  "pagination": {
    "limit": 50,
    "offset": 0,
    "total": 1
  }
}
```

---

## 3.4 POST /accounts

Создать счет.

```http
POST /api/v1/accounts
```

### Request

```json
{
  "name": "Main card",
  "type": "debit_card",
  "currency": "RUB",
  "initialBalance": "100000.00",
  "includeInNetWorth": true,
  "includeInDailyBudget": true
}
```

### Backend behavior

```text
balance_minor = initial_balance_minor
initial_balance_minor = parsed initialBalance
user_id = current user id
archived_at = null
```

### Response 201

```json
{
  "id": "acc_main",
  "name": "Main card",
  "type": "debit_card",
  "currency": "RUB",
  "balance": "100000.00",
  "initialBalance": "100000.00",
  "includeInNetWorth": true,
  "includeInDailyBudget": true,
  "isArchived": false,
  "archivedAt": null,
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T12:00:00Z"
}
```

### Validation

```text
name: required, 1..100 chars
type: required, enum
currency: required, enum
initialBalance: required, valid money string
includeInNetWorth: required boolean
includeInDailyBudget: required boolean
```

---

## 3.5 GET /accounts/{accountId}

```http
GET /api/v1/accounts/acc_main
```

### Response 200

```json
{
  "id": "acc_main",
  "name": "Main card",
  "type": "debit_card",
  "currency": "RUB",
  "balance": "100000.00",
  "initialBalance": "100000.00",
  "includeInNetWorth": true,
  "includeInDailyBudget": true,
  "isArchived": false,
  "archivedAt": null,
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T12:00:00Z"
}
```

### Errors

```text
404 not_found - счет не найден или принадлежит другому пользователю
```

---

## 3.6 PATCH /accounts/{accountId}

Обновить счет.

```http
PATCH /api/v1/accounts/acc_main
```

### Request

Все поля опциональны:

```json
{
  "name": "Main debit card",
  "type": "debit_card",
  "includeInNetWorth": true,
  "includeInDailyBudget": false
}
```

### Что можно менять

```text
name
type
includeInNetWorth
includeInDailyBudget
```

### Что лучше не менять после создания

```text
currency
initialBalance
balance
```

Баланс позже должен меняться через операции, а не прямым PATCH счета. Иначе после Transactions API будет сложно держать консистентность.

### Response 200

```json
{
  "id": "acc_main",
  "name": "Main debit card",
  "type": "debit_card",
  "currency": "RUB",
  "balance": "100000.00",
  "initialBalance": "100000.00",
  "includeInNetWorth": true,
  "includeInDailyBudget": false,
  "isArchived": false,
  "archivedAt": null,
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T12:30:00Z"
}
```

---

## 3.7 POST /accounts/{accountId}/archive

Архивировать счет.

```http
POST /api/v1/accounts/acc_main/archive
```

### Response 200

```json
{
  "id": "acc_main",
  "name": "Main card",
  "type": "debit_card",
  "currency": "RUB",
  "balance": "100000.00",
  "initialBalance": "100000.00",
  "includeInNetWorth": true,
  "includeInDailyBudget": true,
  "isArchived": true,
  "archivedAt": "2026-04-27T13:00:00Z",
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T13:00:00Z"
}
```

### Business rules

```text
нельзя архивировать чужой счет
повторный archive идемпотентный
архивный счет не должен использоваться при создании новых операций
```

---

## 3.8 POST /accounts/{accountId}/restore

Восстановить счет.

```http
POST /api/v1/accounts/acc_main/restore
```

### Response 200

```json
{
  "id": "acc_main",
  "name": "Main card",
  "type": "debit_card",
  "currency": "RUB",
  "balance": "100000.00",
  "initialBalance": "100000.00",
  "includeInNetWorth": true,
  "includeInDailyBudget": true,
  "isArchived": false,
  "archivedAt": null,
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T13:10:00Z"
}
```

---

## 3.9 GET /accounts/summary

Сводка по счетам.

```http
GET /api/v1/accounts/summary?currency=RUB
```

### Response 200

```json
{
  "currency": "RUB",
  "netWorth": "350000.00",
  "cashBalance": "180000.00",
  "availableForDailyBudget": "120000.00",
  "creditLiabilities": "50000.00",
  "accounts": [
    {
      "id": "acc_main",
      "name": "Main card",
      "type": "debit_card",
      "currency": "RUB",
      "balance": "120000.00",
      "includeInNetWorth": true,
      "includeInDailyBudget": true
    }
  ]
}
```

### Calculation rules

```text
netWorth =
  сумма balance активных счетов, где includeInNetWorth = true
  минус creditLiabilities

cashBalance =
  сумма активных cash/debit_card/savings/deposit счетов

availableForDailyBudget =
  сумма активных счетов, где includeInDailyBudget = true

creditLiabilities =
  сумма задолженности по credit_card/debt счетам
```

Сводка уже предусмотрена в текущем REST-дизайне как отдельный endpoint `/accounts/summary` с `netWorth`, `cashBalance`, `availableForDailyBudget`, `creditLiabilities`

---

# 4. Categories API

Категории нужны не только для аналитики, но и для будущего расчета бюджета и дневного лимита. В продуктовой модели категории делятся на обязательные, гибкие, накопительные, инвестиционные, долговые и доходные

## 4.1 Category type enum

```text
required
flexible
saving
investment
debt
income
```

---

## 4.2 Category object

```json
{
  "id": "cat_food",
  "name": "Food",
  "type": "required",
  "color": "#2F80ED",
  "sortOrder": 100,
  "isArchived": false,
  "archivedAt": null,
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T12:00:00Z",
  "subcategories": [
    {
      "id": "sub_groceries",
      "categoryId": "cat_food",
      "name": "Groceries",
      "sortOrder": 100,
      "isArchived": false,
      "archivedAt": null,
      "createdAt": "2026-04-27T12:00:00Z",
      "updatedAt": "2026-04-27T12:00:00Z"
    }
  ]
}
```

---

## 4.3 GET /categories

```http
GET /api/v1/categories?includeArchived=false&includeSubcategories=true&type=required&limit=50&offset=0
```

### Query params

```text
includeArchived: boolean, default false
includeSubcategories: boolean, default true
type: optional category type
limit: default 50
offset: default 0
sort: sortOrder:asc | name:asc | createdAt:desc
```

### Response 200

```json
{
  "items": [
    {
      "id": "cat_food",
      "name": "Food",
      "type": "required",
      "color": "#2F80ED",
      "sortOrder": 100,
      "isArchived": false,
      "archivedAt": null,
      "createdAt": "2026-04-27T12:00:00Z",
      "updatedAt": "2026-04-27T12:00:00Z",
      "subcategories": [
        {
          "id": "sub_groceries",
          "categoryId": "cat_food",
          "name": "Groceries",
          "sortOrder": 100,
          "isArchived": false,
          "archivedAt": null,
          "createdAt": "2026-04-27T12:00:00Z",
          "updatedAt": "2026-04-27T12:00:00Z"
        }
      ]
    }
  ],
  "pagination": {
    "limit": 50,
    "offset": 0,
    "total": 1
  }
}
```

---

## 4.4 POST /categories

```http
POST /api/v1/categories
```

### Request

```json
{
  "name": "Food",
  "type": "required",
  "color": "#2F80ED",
  "sortOrder": 100
}
```

### Response 201

```json
{
  "id": "cat_food",
  "name": "Food",
  "type": "required",
  "color": "#2F80ED",
  "sortOrder": 100,
  "isArchived": false,
  "archivedAt": null,
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T12:00:00Z",
  "subcategories": []
}
```

### Validation

```text
name: required, 1..100 chars
type: required, enum
color: optional, HEX format #RRGGBB
sortOrder: optional int, default 100
```

### Conflict

```json
{
  "error": {
    "code": "conflict",
    "message": "Category with this name already exists",
    "details": [
      {
        "field": "name",
        "message": "category name must be unique per user"
      }
    ]
  }
}
```

---

## 4.5 GET /categories/{categoryId}

```http
GET /api/v1/categories/cat_food?includeSubcategories=true
```

### Response 200

```json
{
  "id": "cat_food",
  "name": "Food",
  "type": "required",
  "color": "#2F80ED",
  "sortOrder": 100,
  "isArchived": false,
  "archivedAt": null,
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T12:00:00Z",
  "subcategories": [
    {
      "id": "sub_groceries",
      "categoryId": "cat_food",
      "name": "Groceries",
      "sortOrder": 100,
      "isArchived": false,
      "archivedAt": null,
      "createdAt": "2026-04-27T12:00:00Z",
      "updatedAt": "2026-04-27T12:00:00Z"
    }
  ]
}
```

---

## 4.6 PATCH /categories/{categoryId}

```http
PATCH /api/v1/categories/cat_food
```

### Request

```json
{
  "name": "Products",
  "type": "required",
  "color": "#27AE60",
  "sortOrder": 110
}
```

### Response 200

```json
{
  "id": "cat_food",
  "name": "Products",
  "type": "required",
  "color": "#27AE60",
  "sortOrder": 110,
  "isArchived": false,
  "archivedAt": null,
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T12:30:00Z",
  "subcategories": []
}
```

---

## 4.7 DELETE /categories/{categoryId}

Для MVP лучше делать **soft archive**, а не физическое удаление.

```http
DELETE /api/v1/categories/cat_food
```

### Response 200

```json
{
  "id": "cat_food",
  "name": "Food",
  "type": "required",
  "color": "#2F80ED",
  "sortOrder": 100,
  "isArchived": true,
  "archivedAt": "2026-04-27T13:00:00Z",
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T13:00:00Z",
  "subcategories": []
}
```

### Business rules

```text
если категория уже используется в transactions - только soft archive
если категория не используется нигде - все равно можно оставить soft archive для единого поведения
архивация категории архивирует подкатегории
архивная категория не доступна для новых операций
```

---

## 4.8 POST /categories/{categoryId}/restore

Я бы добавил этот endpoint сразу, хотя в исходном списке был только DELETE.

```http
POST /api/v1/categories/cat_food/restore
```

### Response 200

```json
{
  "id": "cat_food",
  "name": "Food",
  "type": "required",
  "color": "#2F80ED",
  "sortOrder": 100,
  "isArchived": false,
  "archivedAt": null,
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T13:10:00Z",
  "subcategories": []
}
```

---

# 5. Subcategories API

Subcategories принадлежат категории и пользователю через категорию.

## 5.1 Subcategory object

```json
{
  "id": "sub_groceries",
  "categoryId": "cat_food",
  "name": "Groceries",
  "sortOrder": 100,
  "isArchived": false,
  "archivedAt": null,
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T12:00:00Z"
}
```

---

## 5.2 GET /categories/{categoryId}/subcategories

```http
GET /api/v1/categories/cat_food/subcategories?includeArchived=false
```

### Response 200

```json
{
  "items": [
    {
      "id": "sub_groceries",
      "categoryId": "cat_food",
      "name": "Groceries",
      "sortOrder": 100,
      "isArchived": false,
      "archivedAt": null,
      "createdAt": "2026-04-27T12:00:00Z",
      "updatedAt": "2026-04-27T12:00:00Z"
    }
  ],
  "pagination": {
    "limit": 50,
    "offset": 0,
    "total": 1
  }
}
```

---

## 5.3 POST /categories/{categoryId}/subcategories

```http
POST /api/v1/categories/cat_food/subcategories
```

### Request

```json
{
  "name": "Groceries",
  "sortOrder": 100
}
```

### Response 201

```json
{
  "id": "sub_groceries",
  "categoryId": "cat_food",
  "name": "Groceries",
  "sortOrder": 100,
  "isArchived": false,
  "archivedAt": null,
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T12:00:00Z"
}
```

### Validation

```text
categoryId должен существовать
categoryId должен принадлежать текущему user_id
category не должна быть archived
name: required, 1..100 chars
sortOrder: optional int, default 100
```

---

## 5.4 PATCH /subcategories/{subcategoryId}

```http
PATCH /api/v1/subcategories/sub_groceries
```

### Request

```json
{
  "name": "Supermarkets",
  "sortOrder": 120
}
```

### Response 200

```json
{
  "id": "sub_groceries",
  "categoryId": "cat_food",
  "name": "Supermarkets",
  "sortOrder": 120,
  "isArchived": false,
  "archivedAt": null,
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T12:30:00Z"
}
```

---

## 5.5 DELETE /subcategories/{subcategoryId}

Soft archive.

```http
DELETE /api/v1/subcategories/sub_groceries
```

### Response 200

```json
{
  "id": "sub_groceries",
  "categoryId": "cat_food",
  "name": "Groceries",
  "sortOrder": 100,
  "isArchived": true,
  "archivedAt": "2026-04-27T13:00:00Z",
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T13:00:00Z"
}
```

---

## 5.6 POST /subcategories/{subcategoryId}/restore

```http
POST /api/v1/subcategories/sub_groceries/restore
```

### Response 200

```json
{
  "id": "sub_groceries",
  "categoryId": "cat_food",
  "name": "Groceries",
  "sortOrder": 100,
  "isArchived": false,
  "archivedAt": null,
  "createdAt": "2026-04-27T12:00:00Z",
  "updatedAt": "2026-04-27T13:10:00Z"
}
```

### Business rule

```text
нельзя восстановить подкатегорию, если родительская категория archived
```

---

# 6. DB schema contract

## 6.1 accounts

```sql
CREATE TABLE accounts (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    name TEXT NOT NULL,
    type TEXT NOT NULL,
    currency CHAR(3) NOT NULL,

    balance_minor BIGINT NOT NULL DEFAULT 0,
    initial_balance_minor BIGINT NOT NULL DEFAULT 0,

    include_in_net_worth BOOLEAN NOT NULL DEFAULT TRUE,
    include_in_daily_budget BOOLEAN NOT NULL DEFAULT TRUE,

    archived_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT accounts_name_not_empty CHECK (length(trim(name)) > 0),
    CONSTRAINT accounts_type_check CHECK (
        type IN (
            'cash',
            'debit_card',
            'savings',
            'brokerage',
            'credit_card',
            'deposit',
            'debt',
            'other'
        )
    ),
    CONSTRAINT accounts_currency_check CHECK (
        currency IN ('RUB', 'USD', 'EUR')
    )
);

CREATE INDEX idx_accounts_user_id
    ON accounts(user_id);

CREATE INDEX idx_accounts_user_archived
    ON accounts(user_id, archived_at);

CREATE UNIQUE INDEX ux_accounts_user_name_active
    ON accounts(user_id, lower(name))
    WHERE archived_at IS NULL;
```

---

## 6.2 categories

```sql
CREATE TABLE categories (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    name TEXT NOT NULL,
    type TEXT NOT NULL,
    color TEXT,
    sort_order INTEGER NOT NULL DEFAULT 100,

    archived_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT categories_name_not_empty CHECK (length(trim(name)) > 0),
    CONSTRAINT categories_type_check CHECK (
        type IN (
            'required',
            'flexible',
            'saving',
            'investment',
            'debt',
            'income'
        )
    ),
    CONSTRAINT categories_color_check CHECK (
        color IS NULL OR color ~ '^#[0-9A-Fa-f]{6}$'
    )
);

CREATE INDEX idx_categories_user_id
    ON categories(user_id);

CREATE INDEX idx_categories_user_archived
    ON categories(user_id, archived_at);

CREATE INDEX idx_categories_user_type
    ON categories(user_id, type);

CREATE UNIQUE INDEX ux_categories_user_name_active
    ON categories(user_id, lower(name))
    WHERE archived_at IS NULL;
```

---

## 6.3 subcategories

```sql
CREATE TABLE subcategories (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,

    name TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 100,

    archived_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT subcategories_name_not_empty CHECK (length(trim(name)) > 0)
);

CREATE INDEX idx_subcategories_user_id
    ON subcategories(user_id);

CREATE INDEX idx_subcategories_category_id
    ON subcategories(category_id);

CREATE INDEX idx_subcategories_user_archived
    ON subcategories(user_id, archived_at);

CREATE UNIQUE INDEX ux_subcategories_category_name_active
    ON subcategories(category_id, lower(name))
    WHERE archived_at IS NULL;
```

---

# 7. Ownership rules

Для каждого endpoint:

```text
account.user_id == current_user.id
category.user_id == current_user.id
subcategory.user_id == current_user.id
subcategory.category.user_id == current_user.id
```

Если ресурс существует, но принадлежит другому пользователю, лучше возвращать:

```http
404 Not Found
```

А не `403`, чтобы не раскрывать существование чужих ресурсов.

---

# 8. Минимальный список endpoints для реализации

```http
GET    /api/v1/accounts
POST   /api/v1/accounts
GET    /api/v1/accounts/{accountId}
PATCH  /api/v1/accounts/{accountId}
POST   /api/v1/accounts/{accountId}/archive
POST   /api/v1/accounts/{accountId}/restore
GET    /api/v1/accounts/summary

GET    /api/v1/categories
POST   /api/v1/categories
GET    /api/v1/categories/{categoryId}
PATCH  /api/v1/categories/{categoryId}
DELETE /api/v1/categories/{categoryId}
POST   /api/v1/categories/{categoryId}/restore

GET    /api/v1/categories/{categoryId}/subcategories
POST   /api/v1/categories/{categoryId}/subcategories
PATCH  /api/v1/subcategories/{subcategoryId}
DELETE /api/v1/subcategories/{subcategoryId}
POST   /api/v1/subcategories/{subcategoryId}/restore
```

В текущем REST-документе уже есть базовые endpoints для счетов, категорий и подкатегорий; я добавил `restore` для категорий и подкатегорий, чтобы soft archive был симметричным

---

# 9. Что важно зафиксировать перед Transactions API

```text
1. Деньги в REST всегда string.
2. Деньги в БД всегда BIGINT minor units.
3. Баланс счета после MVP Catalog пока можно хранить как balance_minor.
4. После Transactions API balance_minor должен обновляться только через операции.
5. Category и Subcategory удаляются через soft archive.
6. Все выборки всегда scoped by user_id.
7. Account, Category, Subcategory из архива нельзя использовать в новых transactions.
```

Это даст нормальную базу под следующий этап: **Transactions API**, где уже появятся `accountFromId`, `accountToId`, `categoryId`, `subcategoryId`, статусы `planned/posted/cancelled` и пересчет балансов.
