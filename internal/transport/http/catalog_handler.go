package http

import (
	"context"
	"errors"
	"net/http"
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

type AccountCreateUseCase interface {
	Create(ctx context.Context, input appaccounting.CreateAccountInput) (domainaccounting.Account, error)
}

type AccountGetUseCase interface {
	GetByID(ctx context.Context, userID shared.UserID, accountID shared.AccountID) (domainaccounting.Account, error)
}

type AccountListUseCase interface {
	ListByUser(ctx context.Context, input appaccounting.ListAccountsInput) ([]domainaccounting.Account, error)
}

type CategoryGetUseCase interface {
	GetByID(ctx context.Context, userID shared.UserID, categoryID shared.CategoryID) (domaincatalog.Category, error)
}

type CategoryListUseCase interface {
	ListByUserID(ctx context.Context, userID shared.UserID) ([]domaincatalog.Category, error)
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
	categoriesGet     CategoryGetUseCase
	categoriesList    CategoryListUseCase
	subcategoriesGet  SubcategoryGetUseCase
	subcategoriesList SubcategoryListUseCase
}

func NewCatalogHandler(
	accountsCreate AccountCreateUseCase,
	accountsGet AccountGetUseCase,
	accountsList AccountListUseCase,
	categoriesGet CategoryGetUseCase,
	categoriesList CategoryListUseCase,
	subcategoriesGet SubcategoryGetUseCase,
	subcategoriesList SubcategoryListUseCase,
) *CatalogHandler {
	return &CatalogHandler{
		accountsCreate:    accountsCreate,
		accountsGet:       accountsGet,
		accountsList:      accountsList,
		categoriesGet:     categoriesGet,
		categoriesList:    categoriesList,
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

type categoryResponse struct {
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

	accounts, err := h.accountsList.ListByUser(c.Request.Context(), appaccounting.ListAccountsInput{
		UserID:          user.ID,
		IncludeArchived: includeArchived,
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

	category, err := h.categoriesGet.GetByID(c.Request.Context(), user.ID, shared.CategoryID(categoryID))
	if err != nil {
		if errors.Is(err, appcatalog.ErrCategoryNotFound) {
			writeCatalogError(c, http.StatusNotFound, catalogErrorNotFound, "Resource not found")
			return
		}

		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	c.JSON(http.StatusOK, categoryResponse{
		ID:        string(category.ID()),
		Name:      category.Name(),
		CreatedAt: category.CreatedAt(),
		UpdatedAt: category.UpdatedAt(),
	})
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

	categories, err := h.categoriesList.ListByUserID(c.Request.Context(), user.ID)
	if err != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	items := make([]categoryResponse, 0, len(categories))
	for _, category := range categories {
		items = append(items, categoryResponse{
			ID:        string(category.ID()),
			Name:      category.Name(),
			CreatedAt: category.CreatedAt(),
			UpdatedAt: category.UpdatedAt(),
		})
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

	if strings.TrimSpace(request.Name) == "" {
		details = append(details, catalogFieldError{
			Field:   "name",
			Message: "name is required",
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
		Name:                 strings.TrimSpace(request.Name),
		Type:                 accountType,
		InitialBalance:       initialBalance,
		IncludeInNetWorth:    *request.IncludeInNetWorth,
		IncludeInDailyBudget: *request.IncludeInDailyBudget,
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
