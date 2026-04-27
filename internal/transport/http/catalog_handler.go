package http

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	appaccounting "moneo/internal/app/accounting"
	appcatalog "moneo/internal/app/catalog"
	domainaccounting "moneo/internal/domain/accounting"
	domaincatalog "moneo/internal/domain/catalog"
	"moneo/internal/domain/shared"

	"github.com/gin-gonic/gin"
)

var categoryColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

type AccountCreateUseCase interface {
	Create(ctx context.Context, input appaccounting.CreateAccountInput) (domainaccounting.Account, error)
}

type AccountGetUseCase interface {
	GetByID(ctx context.Context, userID shared.UserID, accountID shared.AccountID) (domainaccounting.Account, error)
}

type AccountListUseCase interface {
	ListByUser(ctx context.Context, input appaccounting.ListAccountsInput) ([]domainaccounting.Account, error)
}

type AccountSummaryUseCase interface {
	GetByUserAndCurrency(ctx context.Context, input appaccounting.GetAccountsSummaryInput) (appaccounting.AccountSummary, error)
}

type AccountArchiveUseCase interface {
	Archive(ctx context.Context, userID shared.UserID, accountID shared.AccountID) (domainaccounting.Account, error)
}

type AccountRestoreUseCase interface {
	Restore(ctx context.Context, userID shared.UserID, accountID shared.AccountID) (domainaccounting.Account, error)
}

type AccountUpdateUseCase interface {
	Update(ctx context.Context, input appaccounting.UpdateAccountInput) (domainaccounting.Account, error)
}

type CategoryCreateUseCase interface {
	Create(ctx context.Context, input appcatalog.CreateCategoryInput) (domaincatalog.Category, error)
}

type CategoryGetUseCase interface {
	GetByID(ctx context.Context, userID shared.UserID, categoryID shared.CategoryID) (domaincatalog.Category, error)
}

type CategoryListUseCase interface {
	ListByUser(ctx context.Context, input appcatalog.ListCategoriesInput) ([]domaincatalog.Category, error)
}

type CategoryUpdateUseCase interface {
	Update(ctx context.Context, input appcatalog.UpdateCategoryInput) (domaincatalog.Category, error)
}

type CategoryArchiveUseCase interface {
	Archive(ctx context.Context, userID shared.UserID, categoryID shared.CategoryID) (domaincatalog.Category, error)
}

type CategoryRestoreUseCase interface {
	Restore(ctx context.Context, userID shared.UserID, categoryID shared.CategoryID) (domaincatalog.Category, error)
}

type SubcategoryGetUseCase interface {
	GetByID(ctx context.Context, userID shared.UserID, subcategoryID shared.SubcategoryID) (domaincatalog.Subcategory, error)
}

type SubcategoryListUseCase interface {
	ListByUserID(ctx context.Context, userID shared.UserID) ([]domaincatalog.Subcategory, error)
}

type CatalogHandler struct {
	accountsCreate    AccountCreateUseCase
	accountsGet       AccountGetUseCase
	accountsList      AccountListUseCase
	accountsSummary   AccountSummaryUseCase
	accountsArchive   AccountArchiveUseCase
	accountsRestore   AccountRestoreUseCase
	accountsUpdate    AccountUpdateUseCase
	categoriesCreate  CategoryCreateUseCase
	categoriesGet     CategoryGetUseCase
	categoriesList    CategoryListUseCase
	categoriesUpdate  CategoryUpdateUseCase
	categoriesArchive CategoryArchiveUseCase
	categoriesRestore CategoryRestoreUseCase
	subcategoriesGet  SubcategoryGetUseCase
	subcategoriesList SubcategoryListUseCase
}

func NewCatalogHandler(
	accountsCreate AccountCreateUseCase,
	accountsGet AccountGetUseCase,
	accountsList AccountListUseCase,
	accountsSummary AccountSummaryUseCase,
	accountsArchive AccountArchiveUseCase,
	accountsRestore AccountRestoreUseCase,
	accountsUpdate AccountUpdateUseCase,
	categoriesCreate CategoryCreateUseCase,
	categoriesGet CategoryGetUseCase,
	categoriesList CategoryListUseCase,
	categoriesUpdate CategoryUpdateUseCase,
	categoriesArchive CategoryArchiveUseCase,
	categoriesRestore CategoryRestoreUseCase,
	subcategoriesGet SubcategoryGetUseCase,
	subcategoriesList SubcategoryListUseCase,
) *CatalogHandler {
	return &CatalogHandler{
		accountsCreate:    accountsCreate,
		accountsGet:       accountsGet,
		accountsList:      accountsList,
		accountsSummary:   accountsSummary,
		accountsArchive:   accountsArchive,
		accountsRestore:   accountsRestore,
		accountsUpdate:    accountsUpdate,
		categoriesCreate:  categoriesCreate,
		categoriesGet:     categoriesGet,
		categoriesList:    categoriesList,
		categoriesUpdate:  categoriesUpdate,
		categoriesArchive: categoriesArchive,
		categoriesRestore: categoriesRestore,
		subcategoriesGet:  subcategoriesGet,
		subcategoriesList: subcategoriesList,
	}
}

type createAccountRequest struct {
	Name                 string         `json:"name"`
	Type                 string         `json:"type"`
	Currency             string         `json:"currency"`
	InitialBalance       *DecimalString `json:"initialBalance"`
	IncludeInNetWorth    *bool          `json:"includeInNetWorth"`
	IncludeInDailyBudget *bool          `json:"includeInDailyBudget"`
}

type patchAccountRequest struct {
	Name                 *string `json:"name"`
	Type                 *string `json:"type"`
	IncludeInNetWorth    *bool   `json:"includeInNetWorth"`
	IncludeInDailyBudget *bool   `json:"includeInDailyBudget"`

	Currency       *string        `json:"currency"`
	InitialBalance *DecimalString `json:"initialBalance"`
	Balance        *DecimalString `json:"balance"`
}

type createCategoryRequest struct {
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Color     *string `json:"color"`
	SortOrder *int    `json:"sortOrder"`
}

type patchCategoryRequest struct {
	Name      *string `json:"name"`
	Type      *string `json:"type"`
	Color     *string `json:"color"`
	SortOrder *int    `json:"sortOrder"`
}

type accountResponse struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Type                 string     `json:"type"`
	Currency             string     `json:"currency"`
	Balance              string     `json:"balance"`
	InitialBalance       string     `json:"initialBalance"`
	IncludeInNetWorth    bool       `json:"includeInNetWorth"`
	IncludeInDailyBudget bool       `json:"includeInDailyBudget"`
	IsArchived           bool       `json:"isArchived"`
	ArchivedAt           *time.Time `json:"archivedAt"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
}

type accountSummaryAccountResponse struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Type                 string `json:"type"`
	Currency             string `json:"currency"`
	Balance              string `json:"balance"`
	IncludeInNetWorth    bool   `json:"includeInNetWorth"`
	IncludeInDailyBudget bool   `json:"includeInDailyBudget"`
}

type accountSummaryResponse struct {
	Currency                string                          `json:"currency"`
	NetWorth                string                          `json:"netWorth"`
	CashBalance             string                          `json:"cashBalance"`
	AvailableForDailyBudget string                          `json:"availableForDailyBudget"`
	CreditLiabilities       string                          `json:"creditLiabilities"`
	Accounts                []accountSummaryAccountResponse `json:"accounts"`
}

type categoryResponse struct {
	ID            string                        `json:"id"`
	Name          string                        `json:"name"`
	Type          string                        `json:"type"`
	Color         *string                       `json:"color"`
	SortOrder     int                           `json:"sortOrder"`
	IsArchived    bool                          `json:"isArchived"`
	ArchivedAt    *time.Time                    `json:"archivedAt"`
	CreatedAt     time.Time                     `json:"createdAt"`
	UpdatedAt     time.Time                     `json:"updatedAt"`
	Subcategories []categorySubcategoryResponse `json:"subcategories"`
}

type categorySubcategoryResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type subcategoryResponse struct {
	ID         string    `json:"id"`
	CategoryID string    `json:"categoryId"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (h *CatalogHandler) CreateAccount(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.accountsCreate == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	var request createAccountRequest
	if err := decodeStrictJSONBody(c, &request); err != nil {
		if errors.Is(err, ErrMoneyAmountMustBeString) || strings.Contains(err.Error(), ErrMoneyAmountMustBeString.Error()) {
			writeCatalogValidationError(c, catalogFieldError{
				Field:   "initialBalance",
				Message: "initialBalance must be a valid decimal string",
			})
			return
		}

		writeCatalogValidationError(c, catalogFieldError{
			Field:   "body",
			Message: "request body is invalid",
		})
		return
	}

	input, details := validateCreateAccountRequest(user.ID, request)
	if len(details) > 0 {
		writeCatalogValidationError(c, details...)
		return
	}

	account, err := h.accountsCreate.Create(c.Request.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, appaccounting.ErrAccountNameAlreadyExists):
			writeCatalogError(c, http.StatusConflict, catalogErrorConflict, "Conflict", catalogFieldError{
				Field:   "name",
				Message: "account with this name already exists",
			})
		case errors.Is(err, appaccounting.ErrNegativeInitialBalance):
			writeCatalogValidationError(c, catalogFieldError{
				Field:   "initialBalance",
				Message: "initialBalance must be greater than or equal to 0",
			})
		case errors.Is(err, domainaccounting.ErrInvalidAccountName),
			errors.Is(err, domainaccounting.ErrInvalidAccountType),
			errors.Is(err, domainaccounting.ErrAccountCurrencyMismatch):
			writeCatalogError(c, http.StatusUnprocessableEntity, catalogErrorBusinessRuleViolation, "Business rule violation")
		default:
			writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		}
		return
	}

	response, mapErr := toAccountResponse(account)
	if mapErr != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	c.JSON(http.StatusCreated, response)
}

func (h *CatalogHandler) GetAccount(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.accountsGet == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	accountID := strings.TrimSpace(c.Param("accountId"))
	if accountID == "" {
		writeCatalogValidationError(c, catalogFieldError{
			Field:   "accountId",
			Message: "accountId is required",
		})
		return
	}

	account, err := h.accountsGet.GetByID(c.Request.Context(), user.ID, shared.AccountID(accountID))
	if err != nil {
		if errors.Is(err, appaccounting.ErrAccountNotFound) {
			writeCatalogError(c, http.StatusNotFound, catalogErrorNotFound, "Resource not found")
			return
		}

		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	response, mapErr := toAccountResponse(account)
	if mapErr != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *CatalogHandler) PatchAccount(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.accountsUpdate == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	accountID := strings.TrimSpace(c.Param("accountId"))
	if accountID == "" {
		writeCatalogValidationError(c, catalogFieldError{
			Field:   "accountId",
			Message: "accountId is required",
		})
		return
	}

	var request patchAccountRequest
	if err := decodeStrictJSONBody(c, &request); err != nil {
		writeCatalogValidationError(c, catalogFieldError{
			Field:   "body",
			Message: "request body is invalid",
		})
		return
	}

	input, details := validatePatchAccountRequest(user.ID, shared.AccountID(accountID), request)
	if len(details) > 0 {
		writeCatalogValidationError(c, details...)
		return
	}

	account, err := h.accountsUpdate.Update(c.Request.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, appaccounting.ErrAccountNotFound):
			writeCatalogError(c, http.StatusNotFound, catalogErrorNotFound, "Resource not found")
		case errors.Is(err, appaccounting.ErrAccountNameAlreadyExists):
			writeCatalogError(c, http.StatusConflict, catalogErrorConflict, "Conflict", catalogFieldError{
				Field:   "name",
				Message: "account with this name already exists",
			})
		default:
			writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		}
		return
	}

	response, mapErr := toAccountResponse(account)
	if mapErr != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *CatalogHandler) ArchiveAccount(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.accountsArchive == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	accountID := strings.TrimSpace(c.Param("accountId"))
	if accountID == "" {
		writeCatalogValidationError(c, catalogFieldError{
			Field:   "accountId",
			Message: "accountId is required",
		})
		return
	}

	account, err := h.accountsArchive.Archive(c.Request.Context(), user.ID, shared.AccountID(accountID))
	if err != nil {
		if errors.Is(err, appaccounting.ErrAccountNotFound) {
			writeCatalogError(c, http.StatusNotFound, catalogErrorNotFound, "Resource not found")
			return
		}

		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	response, mapErr := toAccountResponse(account)
	if mapErr != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *CatalogHandler) RestoreAccount(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.accountsRestore == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	accountID := strings.TrimSpace(c.Param("accountId"))
	if accountID == "" {
		writeCatalogValidationError(c, catalogFieldError{
			Field:   "accountId",
			Message: "accountId is required",
		})
		return
	}

	account, err := h.accountsRestore.Restore(c.Request.Context(), user.ID, shared.AccountID(accountID))
	if err != nil {
		if errors.Is(err, appaccounting.ErrAccountNotFound) {
			writeCatalogError(c, http.StatusNotFound, catalogErrorNotFound, "Resource not found")
			return
		}

		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	response, mapErr := toAccountResponse(account)
	if mapErr != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *CatalogHandler) ListAccounts(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.accountsList == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	limit, offset, details := parseLimitOffset(c)
	if len(details) > 0 {
		writeCatalogValidationError(c, details...)
		return
	}

	includeArchived := false
	if rawIncludeArchived := c.Query("includeArchived"); rawIncludeArchived != "" {
		parsed, err := strconv.ParseBool(rawIncludeArchived)
		if err != nil {
			writeCatalogValidationError(c, catalogFieldError{
				Field:   "includeArchived",
				Message: "includeArchived must be a boolean",
			})
			return
		}
		includeArchived = parsed
	}

	var accountType *domainaccounting.AccountType
	if rawType := strings.TrimSpace(c.Query("type")); rawType != "" {
		parsedType, err := domainaccounting.ParseAccountType(rawType)
		if err != nil {
			writeCatalogValidationError(c, catalogFieldError{
				Field:   "type",
				Message: "type must be one of: cash, debit_card, savings, brokerage, credit_card, deposit, debt, other",
			})
			return
		}
		accountType = &parsedType
	}

	var currency *shared.Currency
	if rawCurrency := strings.TrimSpace(c.Query("currency")); rawCurrency != "" {
		parsedCurrency, err := shared.ParseCurrency(rawCurrency)
		if err != nil {
			writeCatalogValidationError(c, catalogFieldError{
				Field:   "currency",
				Message: "currency must be one of: RUB, USD, EUR",
			})
			return
		}
		currency = &parsedCurrency
	}

	sortMode := appaccounting.AccountsSortCreatedAtDesc
	if rawSort := strings.TrimSpace(c.Query("sort")); rawSort != "" {
		switch appaccounting.AccountsSort(rawSort) {
		case appaccounting.AccountsSortCreatedAtDesc,
			appaccounting.AccountsSortNameAsc,
			appaccounting.AccountsSortBalanceDesc:
			sortMode = appaccounting.AccountsSort(rawSort)
		default:
			writeCatalogValidationError(c, catalogFieldError{
				Field:   "sort",
				Message: "sort must be one of: createdAt:desc, name:asc, balance:desc",
			})
			return
		}
	}

	accounts, err := h.accountsList.ListByUser(c.Request.Context(), appaccounting.ListAccountsInput{
		UserID:          user.ID,
		IncludeArchived: includeArchived,
		Type:            accountType,
		Currency:        currency,
		Sort:            sortMode,
	})
	if err != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	items := make([]accountResponse, 0, len(accounts))
	for _, account := range accounts {
		item, mapErr := toAccountResponse(account)
		if mapErr != nil {
			writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
			return
		}
		items = append(items, item)
	}

	pagedItems, total := paginate(items, limit, offset)
	c.JSON(http.StatusOK, paginatedResponse[accountResponse]{
		Items: pagedItems,
		Pagination: paginationMeta{
			Limit:  limit,
			Offset: offset,
			Total:  total,
		},
	})
}

func (h *CatalogHandler) GetAccountsSummary(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.accountsSummary == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	rawCurrency := strings.TrimSpace(c.Query("currency"))
	if rawCurrency == "" {
		writeCatalogValidationError(c, catalogFieldError{
			Field:   "currency",
			Message: "currency is required",
		})
		return
	}

	currency, err := shared.ParseCurrency(rawCurrency)
	if err != nil {
		writeCatalogValidationError(c, catalogFieldError{
			Field:   "currency",
			Message: "currency must be one of: RUB, USD, EUR",
		})
		return
	}

	summary, err := h.accountsSummary.GetByUserAndCurrency(c.Request.Context(), appaccounting.GetAccountsSummaryInput{
		UserID:   user.ID,
		Currency: currency,
	})
	if err != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	response, mapErr := toAccountSummaryResponse(summary)
	if mapErr != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *CatalogHandler) CreateCategory(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.categoriesCreate == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	var request createCategoryRequest
	if err := decodeStrictJSONBody(c, &request); err != nil {
		writeCatalogValidationError(c, catalogFieldError{
			Field:   "body",
			Message: "request body is invalid",
		})
		return
	}

	input, details := validateCreateCategoryRequest(user.ID, request)
	if len(details) > 0 {
		writeCatalogValidationError(c, details...)
		return
	}

	category, err := h.categoriesCreate.Create(c.Request.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, appcatalog.ErrCategoryNameAlreadyExists):
			writeCatalogError(c, http.StatusConflict, catalogErrorConflict, "Category with this name already exists", catalogFieldError{
				Field:   "name",
				Message: "category name must be unique per user",
			})
		case errors.Is(err, domaincatalog.ErrInvalidCategoryName),
			errors.Is(err, domaincatalog.ErrInvalidCategoryType),
			errors.Is(err, domaincatalog.ErrInvalidCategoryColor):
			writeCatalogValidationError(c, catalogFieldError{
				Field:   "body",
				Message: "request body is invalid",
			})
		default:
			writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		}
		return
	}

	response := toCategoryResponse(category, nil)
	c.JSON(http.StatusCreated, response)
}

func (h *CatalogHandler) GetCategory(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.categoriesGet == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	categoryID := strings.TrimSpace(c.Param("categoryId"))
	if categoryID == "" {
		writeCatalogValidationError(c, catalogFieldError{
			Field:   "categoryId",
			Message: "categoryId is required",
		})
		return
	}

	includeSubcategories := true
	if rawIncludeSubcategories := c.Query("includeSubcategories"); rawIncludeSubcategories != "" {
		parsed, parseErr := strconv.ParseBool(rawIncludeSubcategories)
		if parseErr != nil {
			writeCatalogValidationError(c, catalogFieldError{
				Field:   "includeSubcategories",
				Message: "includeSubcategories must be a boolean",
			})
			return
		}
		includeSubcategories = parsed
	}

	category, err := h.categoriesGet.GetByID(c.Request.Context(), user.ID, shared.CategoryID(categoryID))
	if err != nil {
		if errors.Is(err, appcatalog.ErrCategoryNotFound) {
			writeCatalogError(c, http.StatusNotFound, catalogErrorNotFound, "Resource not found")
			return
		}

		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	subcategories := make([]domaincatalog.Subcategory, 0)
	if includeSubcategories {
		if h.subcategoriesList == nil {
			writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
			return
		}

		allSubcategories, subcategoriesErr := h.subcategoriesList.ListByUserID(c.Request.Context(), user.ID)
		if subcategoriesErr != nil {
			writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
			return
		}

		subcategories = selectSubcategoriesByCategoryID(allSubcategories, category.ID())
	}

	c.JSON(http.StatusOK, toCategoryResponse(category, subcategories))
}

func (h *CatalogHandler) PatchCategory(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.categoriesUpdate == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	categoryID := strings.TrimSpace(c.Param("categoryId"))
	if categoryID == "" {
		writeCatalogValidationError(c, catalogFieldError{
			Field:   "categoryId",
			Message: "categoryId is required",
		})
		return
	}

	var request patchCategoryRequest
	if err := decodeStrictJSONBody(c, &request); err != nil {
		writeCatalogValidationError(c, catalogFieldError{
			Field:   "body",
			Message: "request body is invalid",
		})
		return
	}

	input, details := validatePatchCategoryRequest(user.ID, shared.CategoryID(categoryID), request)
	if len(details) > 0 {
		writeCatalogValidationError(c, details...)
		return
	}

	category, err := h.categoriesUpdate.Update(c.Request.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, appcatalog.ErrCategoryNotFound):
			writeCatalogError(c, http.StatusNotFound, catalogErrorNotFound, "Resource not found")
		case errors.Is(err, appcatalog.ErrCategoryNameAlreadyExists):
			writeCatalogError(c, http.StatusConflict, catalogErrorConflict, "Category with this name already exists", catalogFieldError{
				Field:   "name",
				Message: "category name must be unique per user",
			})
		case errors.Is(err, domaincatalog.ErrInvalidCategoryName),
			errors.Is(err, domaincatalog.ErrInvalidCategoryType),
			errors.Is(err, domaincatalog.ErrInvalidCategoryColor):
			writeCatalogValidationError(c, catalogFieldError{
				Field:   "body",
				Message: "request body is invalid",
			})
		default:
			writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		}
		return
	}

	c.JSON(http.StatusOK, toCategoryResponse(category, nil))
}

func (h *CatalogHandler) DeleteCategory(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.categoriesArchive == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	categoryID := strings.TrimSpace(c.Param("categoryId"))
	if categoryID == "" {
		writeCatalogValidationError(c, catalogFieldError{
			Field:   "categoryId",
			Message: "categoryId is required",
		})
		return
	}

	category, err := h.categoriesArchive.Archive(c.Request.Context(), user.ID, shared.CategoryID(categoryID))
	if err != nil {
		if errors.Is(err, appcatalog.ErrCategoryNotFound) {
			writeCatalogError(c, http.StatusNotFound, catalogErrorNotFound, "Resource not found")
			return
		}
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	c.JSON(http.StatusOK, toCategoryResponse(category, nil))
}

func (h *CatalogHandler) RestoreCategory(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.categoriesRestore == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	categoryID := strings.TrimSpace(c.Param("categoryId"))
	if categoryID == "" {
		writeCatalogValidationError(c, catalogFieldError{
			Field:   "categoryId",
			Message: "categoryId is required",
		})
		return
	}

	category, err := h.categoriesRestore.Restore(c.Request.Context(), user.ID, shared.CategoryID(categoryID))
	if err != nil {
		if errors.Is(err, appcatalog.ErrCategoryNotFound) {
			writeCatalogError(c, http.StatusNotFound, catalogErrorNotFound, "Resource not found")
			return
		}
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	c.JSON(http.StatusOK, toCategoryResponse(category, nil))
}

func (h *CatalogHandler) ListCategories(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.categoriesList == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	limit, offset, details := parseLimitOffset(c)
	if len(details) > 0 {
		writeCatalogValidationError(c, details...)
		return
	}

	includeArchived := false
	if rawIncludeArchived := c.Query("includeArchived"); rawIncludeArchived != "" {
		parsed, parseErr := strconv.ParseBool(rawIncludeArchived)
		if parseErr != nil {
			writeCatalogValidationError(c, catalogFieldError{
				Field:   "includeArchived",
				Message: "includeArchived must be a boolean",
			})
			return
		}
		includeArchived = parsed
	}

	includeSubcategories := true
	if rawIncludeSubcategories := c.Query("includeSubcategories"); rawIncludeSubcategories != "" {
		parsed, parseErr := strconv.ParseBool(rawIncludeSubcategories)
		if parseErr != nil {
			writeCatalogValidationError(c, catalogFieldError{
				Field:   "includeSubcategories",
				Message: "includeSubcategories must be a boolean",
			})
			return
		}
		includeSubcategories = parsed
	}

	var categoryType *domaincatalog.CategoryType
	if rawType := strings.TrimSpace(c.Query("type")); rawType != "" {
		parsedType, parseErr := domaincatalog.ParseCategoryType(rawType)
		if parseErr != nil {
			writeCatalogValidationError(c, catalogFieldError{
				Field:   "type",
				Message: "type must be one of: required, flexible, saving, investment, debt, income",
			})
			return
		}
		categoryType = &parsedType
	}

	sortMode := appcatalog.CategorySortSortOrderAsc
	if rawSort := strings.TrimSpace(c.Query("sort")); rawSort != "" {
		switch appcatalog.CategorySort(rawSort) {
		case appcatalog.CategorySortSortOrderAsc,
			appcatalog.CategorySortNameAsc,
			appcatalog.CategorySortCreatedAtDesc:
			sortMode = appcatalog.CategorySort(rawSort)
		default:
			writeCatalogValidationError(c, catalogFieldError{
				Field:   "sort",
				Message: "sort must be one of: sortOrder:asc, name:asc, createdAt:desc",
			})
			return
		}
	}

	categories, err := h.categoriesList.ListByUser(c.Request.Context(), appcatalog.ListCategoriesInput{
		UserID:          user.ID,
		IncludeArchived: includeArchived,
		Type:            categoryType,
		Sort:            sortMode,
	})
	if err != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	subcategoriesByCategoryID := make(map[shared.CategoryID][]domaincatalog.Subcategory)
	if includeSubcategories {
		if h.subcategoriesList == nil {
			writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
			return
		}

		allSubcategories, subcategoriesErr := h.subcategoriesList.ListByUserID(c.Request.Context(), user.ID)
		if subcategoriesErr != nil {
			writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
			return
		}
		for _, subcategory := range allSubcategories {
			categoryID := subcategory.CategoryID()
			subcategoriesByCategoryID[categoryID] = append(subcategoriesByCategoryID[categoryID], subcategory)
		}
	}

	items := make([]categoryResponse, 0, len(categories))
	for _, category := range categories {
		items = append(items, toCategoryResponse(category, subcategoriesByCategoryID[category.ID()]))
	}

	pagedItems, total := paginate(items, limit, offset)
	c.JSON(http.StatusOK, paginatedResponse[categoryResponse]{
		Items: pagedItems,
		Pagination: paginationMeta{
			Limit:  limit,
			Offset: offset,
			Total:  total,
		},
	})
}

func (h *CatalogHandler) GetSubcategory(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.subcategoriesGet == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	subcategoryID := strings.TrimSpace(c.Param("subcategoryId"))
	if subcategoryID == "" {
		writeCatalogValidationError(c, catalogFieldError{
			Field:   "subcategoryId",
			Message: "subcategoryId is required",
		})
		return
	}

	subcategory, err := h.subcategoriesGet.GetByID(c.Request.Context(), user.ID, shared.SubcategoryID(subcategoryID))
	if err != nil {
		if errors.Is(err, appcatalog.ErrSubcategoryNotFound) {
			writeCatalogError(c, http.StatusNotFound, catalogErrorNotFound, "Resource not found")
			return
		}

		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	c.JSON(http.StatusOK, subcategoryResponse{
		ID:         string(subcategory.ID()),
		CategoryID: string(subcategory.CategoryID()),
		Name:       subcategory.Name(),
		CreatedAt:  subcategory.CreatedAt(),
		UpdatedAt:  subcategory.UpdatedAt(),
	})
}

func (h *CatalogHandler) ListSubcategories(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.subcategoriesList == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	limit, offset, details := parseLimitOffset(c)
	if len(details) > 0 {
		writeCatalogValidationError(c, details...)
		return
	}

	subcategories, err := h.subcategoriesList.ListByUserID(c.Request.Context(), user.ID)
	if err != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	items := make([]subcategoryResponse, 0, len(subcategories))
	for _, subcategory := range subcategories {
		items = append(items, subcategoryResponse{
			ID:         string(subcategory.ID()),
			CategoryID: string(subcategory.CategoryID()),
			Name:       subcategory.Name(),
			CreatedAt:  subcategory.CreatedAt(),
			UpdatedAt:  subcategory.UpdatedAt(),
		})
	}

	pagedItems, total := paginate(items, limit, offset)
	c.JSON(http.StatusOK, paginatedResponse[subcategoryResponse]{
		Items: pagedItems,
		Pagination: paginationMeta{
			Limit:  limit,
			Offset: offset,
			Total:  total,
		},
	})
}

func validateCreateAccountRequest(userID shared.UserID, request createAccountRequest) (appaccounting.CreateAccountInput, []catalogFieldError) {
	details := make([]catalogFieldError, 0, 6)

	trimmedName := strings.TrimSpace(request.Name)
	if trimmedName == "" {
		details = append(details, catalogFieldError{
			Field:   "name",
			Message: "name is required",
		})
	} else if len([]rune(trimmedName)) > 100 {
		details = append(details, catalogFieldError{
			Field:   "name",
			Message: "name must be between 1 and 100 characters",
		})
	}

	accountType, err := domainaccounting.ParseAccountType(strings.TrimSpace(request.Type))
	if strings.TrimSpace(request.Type) == "" {
		details = append(details, catalogFieldError{
			Field:   "type",
			Message: "type is required",
		})
	} else if err != nil {
		details = append(details, catalogFieldError{
			Field:   "type",
			Message: "type must be one of: cash, debit_card, savings, brokerage, credit_card, deposit, debt, other",
		})
	}

	if strings.TrimSpace(request.Currency) == "" {
		details = append(details, catalogFieldError{
			Field:   "currency",
			Message: "currency is required",
		})
	}

	var initialBalance shared.Money
	if request.InitialBalance == nil {
		details = append(details, catalogFieldError{
			Field:   "initialBalance",
			Message: "initialBalance is required",
		})
	} else {
		parsedBalance, parseErr := ParseInitialBalanceMoneyFromREST(*request.InitialBalance, request.Currency)
		if parseErr != nil {
			if errors.Is(parseErr, shared.ErrNegativeMoneyAmount) {
				details = append(details, catalogFieldError{
					Field:   "initialBalance",
					Message: "initialBalance must be greater than or equal to 0",
				})
			} else {
				details = append(details, catalogFieldError{
					Field:   "initialBalance",
					Message: "initialBalance must be a valid decimal string",
				})
			}
		} else {
			initialBalance = parsedBalance
		}
	}

	if _, currencyErr := shared.ParseCurrency(strings.TrimSpace(request.Currency)); strings.TrimSpace(request.Currency) != "" && currencyErr != nil {
		details = append(details, catalogFieldError{
			Field:   "currency",
			Message: "currency must be one of: RUB, USD, EUR",
		})
	}

	if request.IncludeInNetWorth == nil {
		details = append(details, catalogFieldError{
			Field:   "includeInNetWorth",
			Message: "includeInNetWorth is required",
		})
	}
	if request.IncludeInDailyBudget == nil {
		details = append(details, catalogFieldError{
			Field:   "includeInDailyBudget",
			Message: "includeInDailyBudget is required",
		})
	}

	if len(details) > 0 {
		return appaccounting.CreateAccountInput{}, details
	}

	return appaccounting.CreateAccountInput{
		UserID:               userID,
		Name:                 trimmedName,
		Type:                 accountType,
		InitialBalance:       initialBalance,
		IncludeInNetWorth:    *request.IncludeInNetWorth,
		IncludeInDailyBudget: *request.IncludeInDailyBudget,
	}, nil
}

func validatePatchAccountRequest(
	userID shared.UserID,
	accountID shared.AccountID,
	request patchAccountRequest,
) (appaccounting.UpdateAccountInput, []catalogFieldError) {
	details := make([]catalogFieldError, 0, 5)

	if request.Currency != nil {
		details = append(details, catalogFieldError{
			Field:   "currency",
			Message: "currency is immutable and cannot be changed",
		})
	}
	if request.InitialBalance != nil {
		details = append(details, catalogFieldError{
			Field:   "initialBalance",
			Message: "initialBalance is immutable and cannot be changed",
		})
	}
	if request.Balance != nil {
		details = append(details, catalogFieldError{
			Field:   "balance",
			Message: "balance is immutable and cannot be changed directly",
		})
	}

	if request.Name == nil && request.Type == nil && request.IncludeInNetWorth == nil && request.IncludeInDailyBudget == nil {
		details = append(details, catalogFieldError{
			Field:   "body",
			Message: "at least one mutable field is required",
		})
	}

	var name *string
	if request.Name != nil {
		trimmed := strings.TrimSpace(*request.Name)
		if trimmed == "" || len([]rune(trimmed)) > 100 {
			details = append(details, catalogFieldError{
				Field:   "name",
				Message: "name must be between 1 and 100 characters",
			})
		} else {
			name = &trimmed
		}
	}

	var accountType *domainaccounting.AccountType
	if request.Type != nil {
		parsedType, err := domainaccounting.ParseAccountType(strings.TrimSpace(*request.Type))
		if err != nil {
			details = append(details, catalogFieldError{
				Field:   "type",
				Message: "type must be one of: cash, debit_card, savings, brokerage, credit_card, deposit, debt, other",
			})
		} else {
			accountType = &parsedType
		}
	}

	if len(details) > 0 {
		return appaccounting.UpdateAccountInput{}, details
	}

	return appaccounting.UpdateAccountInput{
		UserID:               userID,
		AccountID:            accountID,
		Name:                 name,
		Type:                 accountType,
		IncludeInNetWorth:    request.IncludeInNetWorth,
		IncludeInDailyBudget: request.IncludeInDailyBudget,
	}, nil
}

func validateCreateCategoryRequest(
	userID shared.UserID,
	request createCategoryRequest,
) (appcatalog.CreateCategoryInput, []catalogFieldError) {
	details := make([]catalogFieldError, 0, 4)

	trimmedName := strings.TrimSpace(request.Name)
	if trimmedName == "" {
		details = append(details, catalogFieldError{
			Field:   "name",
			Message: "name is required",
		})
	} else if len([]rune(trimmedName)) > 100 {
		details = append(details, catalogFieldError{
			Field:   "name",
			Message: "name must be between 1 and 100 characters",
		})
	}

	var categoryType domaincatalog.CategoryType
	trimmedType := strings.TrimSpace(request.Type)
	if trimmedType == "" {
		details = append(details, catalogFieldError{
			Field:   "type",
			Message: "type is required",
		})
	} else {
		parsedType, err := domaincatalog.ParseCategoryType(trimmedType)
		if err != nil {
			details = append(details, catalogFieldError{
				Field:   "type",
				Message: "type must be one of: required, flexible, saving, investment, debt, income",
			})
		} else {
			categoryType = parsedType
		}
	}

	var color *string
	if request.Color != nil {
		trimmedColor := strings.TrimSpace(*request.Color)
		if trimmedColor == "" || !categoryColorPattern.MatchString(trimmedColor) {
			details = append(details, catalogFieldError{
				Field:   "color",
				Message: "color must be a valid HEX string #RRGGBB",
			})
		} else {
			color = &trimmedColor
		}
	}

	if len(details) > 0 {
		return appcatalog.CreateCategoryInput{}, details
	}

	input := appcatalog.CreateCategoryInput{
		UserID: userID,
		Name:   trimmedName,
		Type:   categoryType,
		Color:  color,
	}
	if request.SortOrder != nil {
		input.SortOrder = request.SortOrder
	}
	return input, nil
}

func validatePatchCategoryRequest(
	userID shared.UserID,
	categoryID shared.CategoryID,
	request patchCategoryRequest,
) (appcatalog.UpdateCategoryInput, []catalogFieldError) {
	details := make([]catalogFieldError, 0, 4)

	if request.Name == nil && request.Type == nil && request.Color == nil && request.SortOrder == nil {
		details = append(details, catalogFieldError{
			Field:   "body",
			Message: "at least one mutable field is required",
		})
	}

	var name *string
	if request.Name != nil {
		trimmedName := strings.TrimSpace(*request.Name)
		if trimmedName == "" || len([]rune(trimmedName)) > 100 {
			details = append(details, catalogFieldError{
				Field:   "name",
				Message: "name must be between 1 and 100 characters",
			})
		} else {
			name = &trimmedName
		}
	}

	var categoryType *domaincatalog.CategoryType
	if request.Type != nil {
		parsedType, err := domaincatalog.ParseCategoryType(strings.TrimSpace(*request.Type))
		if err != nil {
			details = append(details, catalogFieldError{
				Field:   "type",
				Message: "type must be one of: required, flexible, saving, investment, debt, income",
			})
		} else {
			categoryType = &parsedType
		}
	}

	var color *string
	if request.Color != nil {
		trimmedColor := strings.TrimSpace(*request.Color)
		if trimmedColor == "" || !categoryColorPattern.MatchString(trimmedColor) {
			details = append(details, catalogFieldError{
				Field:   "color",
				Message: "color must be a valid HEX string #RRGGBB",
			})
		} else {
			color = &trimmedColor
		}
	}

	if len(details) > 0 {
		return appcatalog.UpdateCategoryInput{}, details
	}

	return appcatalog.UpdateCategoryInput{
		UserID:     userID,
		CategoryID: categoryID,
		Name:       name,
		Type:       categoryType,
		Color:      color,
		SortOrder:  request.SortOrder,
	}, nil
}

func toAccountResponse(account domainaccounting.Account) (accountResponse, error) {
	balance, err := FormatMoneyToREST(account.Balance())
	if err != nil {
		return accountResponse{}, err
	}
	initialBalance, err := FormatMoneyToREST(account.InitialBalance())
	if err != nil {
		return accountResponse{}, err
	}

	return accountResponse{
		ID:                   string(account.ID()),
		Name:                 account.Name(),
		Type:                 string(account.Type()),
		Currency:             account.Balance().Currency().String(),
		Balance:              balance,
		InitialBalance:       initialBalance,
		IncludeInNetWorth:    account.IncludeInNetWorth(),
		IncludeInDailyBudget: account.IncludeInDailyBudget(),
		IsArchived:           account.ArchivedAt() != nil,
		ArchivedAt:           account.ArchivedAt(),
		CreatedAt:            account.CreatedAt(),
		UpdatedAt:            account.UpdatedAt(),
	}, nil
}

func toCategoryResponse(
	category domaincatalog.Category,
	subcategories []domaincatalog.Subcategory,
) categoryResponse {
	items := make([]categorySubcategoryResponse, 0, len(subcategories))
	for _, subcategory := range subcategories {
		items = append(items, categorySubcategoryResponse{
			ID:        string(subcategory.ID()),
			Name:      subcategory.Name(),
			CreatedAt: subcategory.CreatedAt(),
			UpdatedAt: subcategory.UpdatedAt(),
		})
	}

	return categoryResponse{
		ID:            string(category.ID()),
		Name:          category.Name(),
		Type:          string(category.Type()),
		Color:         category.Color(),
		SortOrder:     category.SortOrder(),
		IsArchived:    category.ArchivedAt() != nil,
		ArchivedAt:    category.ArchivedAt(),
		CreatedAt:     category.CreatedAt(),
		UpdatedAt:     category.UpdatedAt(),
		Subcategories: items,
	}
}

func selectSubcategoriesByCategoryID(
	subcategories []domaincatalog.Subcategory,
	categoryID shared.CategoryID,
) []domaincatalog.Subcategory {
	result := make([]domaincatalog.Subcategory, 0, 4)
	for _, subcategory := range subcategories {
		if subcategory.CategoryID() == categoryID {
			result = append(result, subcategory)
		}
	}
	return result
}

func toAccountSummaryResponse(summary appaccounting.AccountSummary) (accountSummaryResponse, error) {
	netWorth, err := FormatMoneyToREST(summary.NetWorth)
	if err != nil {
		return accountSummaryResponse{}, err
	}
	cashBalance, err := FormatMoneyToREST(summary.CashBalance)
	if err != nil {
		return accountSummaryResponse{}, err
	}
	availableForDailyBudget, err := FormatMoneyToREST(summary.AvailableForDailyBudget)
	if err != nil {
		return accountSummaryResponse{}, err
	}
	creditLiabilities, err := FormatMoneyToREST(summary.CreditLiabilities)
	if err != nil {
		return accountSummaryResponse{}, err
	}

	accounts := make([]accountSummaryAccountResponse, 0, len(summary.Accounts))
	for _, account := range summary.Accounts {
		balance, formatErr := FormatMoneyToREST(account.Balance)
		if formatErr != nil {
			return accountSummaryResponse{}, formatErr
		}

		accounts = append(accounts, accountSummaryAccountResponse{
			ID:                   string(account.ID),
			Name:                 account.Name,
			Type:                 string(account.Type),
			Currency:             account.Balance.Currency().String(),
			Balance:              balance,
			IncludeInNetWorth:    account.IncludeInNetWorth,
			IncludeInDailyBudget: account.IncludeInDailyBudget,
		})
	}

	return accountSummaryResponse{
		Currency:                summary.Currency.String(),
		NetWorth:                netWorth,
		CashBalance:             cashBalance,
		AvailableForDailyBudget: availableForDailyBudget,
		CreditLiabilities:       creditLiabilities,
		Accounts:                accounts,
	}, nil
}
