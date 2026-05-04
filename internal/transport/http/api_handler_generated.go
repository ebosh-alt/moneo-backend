package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	appaccounting "moneo/internal/app/accounting"
	appcatalog "moneo/internal/app/catalog"
	appidentity "moneo/internal/app/identity"
	domainaccounting "moneo/internal/domain/accounting"
	domaincatalog "moneo/internal/domain/catalog"
	domainidentity "moneo/internal/domain/identity"
	"moneo/internal/domain/shared"
	domaintransactions "moneo/internal/domain/transactions"
	generated "moneo/internal/transport/http/generated"

	"github.com/gin-gonic/gin"
)

type APIHandler struct {
	auth    *AuthHandler
	catalog *CatalogHandler
}

var _ generated.StrictServerInterface = (*APIHandler)(nil)

func NewAPIHandler(auth *AuthHandler, catalog *CatalogHandler) *APIHandler {
	return &APIHandler{
		auth:    auth,
		catalog: catalog,
	}
}

func (h *APIHandler) ListAccounts(ctx context.Context, request generated.ListAccountsRequestObject) (generated.ListAccountsResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.accountsList == nil {
		return generated.ListAccounts500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.ListAccounts401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	limit, offset, details := strictParseLimitOffset(request.Params.Limit, request.Params.Offset)
	if len(details) > 0 {
		return generated.ListAccounts400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}

	includeArchived := false
	if request.Params.IncludeArchived != nil {
		includeArchived = bool(*request.Params.IncludeArchived)
	}

	var accountType *domainaccounting.AccountType
	if request.Params.Type != nil {
		parsedType, err := domainaccounting.ParseAccountType(strings.TrimSpace(*request.Params.Type))
		if err != nil {
			return generated.ListAccounts400JSONResponse{
				ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
					catalogErrorValidation,
					"Validation failed",
					catalogFieldError{Field: "type", Message: "type must be one of: cash, debit_card, savings, brokerage, credit_card, deposit, debt, other"},
				)),
			}, nil
		}
		accountType = &parsedType
	}

	var currency *shared.Currency
	if request.Params.Currency != nil {
		parsedCurrency, err := shared.ParseCurrency(strings.TrimSpace(*request.Params.Currency))
		if err != nil {
			return generated.ListAccounts400JSONResponse{
				ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
					catalogErrorValidation,
					"Validation failed",
					catalogFieldError{Field: "currency", Message: "currency must be one of: RUB, USD, EUR"},
				)),
			}, nil
		}
		currency = &parsedCurrency
	}

	sortMode := appaccounting.AccountsSortCreatedAtDesc
	if request.Params.Sort != nil {
		candidate := appaccounting.AccountsSort(*request.Params.Sort)
		if !slices.Contains([]appaccounting.AccountsSort{
			appaccounting.AccountsSortCreatedAtDesc,
			appaccounting.AccountsSortNameAsc,
			appaccounting.AccountsSortBalanceDesc,
		}, candidate) {
			return generated.ListAccounts400JSONResponse{
				ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
					catalogErrorValidation,
					"Validation failed",
					catalogFieldError{Field: "sort", Message: "sort must be one of: createdAt:desc, name:asc, balance:desc"},
				)),
			}, nil
		}
		sortMode = candidate
	}

	accounts, err := h.catalog.accountsList.ListByUser(strictAppContext(ctx), appaccounting.ListAccountsInput{
		UserID:          userID,
		IncludeArchived: includeArchived,
		Type:            accountType,
		Currency:        currency,
		Sort:            sortMode,
	})
	if err != nil {
		return generated.ListAccounts500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}

	items := make([]generated.Account, 0, len(accounts))
	for _, account := range accounts {
		mapped, mapErr := toGeneratedAccount(account)
		if mapErr != nil {
			return generated.ListAccounts500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}, nil
		}
		items = append(items, mapped)
	}

	pagedItems, total := paginate(items, limit, offset)
	return generated.ListAccounts200JSONResponse{
		Items: pagedItems,
		Pagination: generated.Pagination{
			Limit:  limit,
			Offset: offset,
			Total:  total,
		},
	}, nil
}

func (h *APIHandler) CreateAccount(ctx context.Context, request generated.CreateAccountRequestObject) (generated.CreateAccountResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.accountsCreate == nil {
		return generated.CreateAccount500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.CreateAccount401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}
	if request.Body == nil {
		return generated.CreateAccount400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "body", Message: "request body is invalid"},
			)),
		}, nil
	}

	createReq := createAccountRequest{}
	if request.Body.Name != nil {
		createReq.Name = *request.Body.Name
	}
	if request.Body.Type != nil {
		createReq.Type = *request.Body.Type
	}
	if request.Body.Currency != nil {
		createReq.Currency = *request.Body.Currency
	}
	if request.Body.InitialBalance != nil {
		initial := DecimalString(*request.Body.InitialBalance)
		createReq.InitialBalance = &initial
	}
	createReq.IncludeInNetWorth = request.Body.IncludeInNetWorth
	createReq.IncludeInDailyBudget = request.Body.IncludeInDailyBudget

	input, details := validateCreateAccountRequest(userID, createReq)
	if len(details) > 0 {
		return generated.CreateAccount400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}

	account, err := h.catalog.accountsCreate.Create(strictAppContext(ctx), input)
	if err != nil {
		switch {
		case errors.Is(err, appaccounting.ErrAccountNameAlreadyExists):
			return generated.CreateAccount409JSONResponse{
				ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(
					catalogErrorConflict, "Conflict", catalogFieldError{Field: "name", Message: "account with this name already exists"},
				)),
			}, nil
		case errors.Is(err, appaccounting.ErrNegativeInitialBalance):
			return generated.CreateAccount400JSONResponse{
				ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
					catalogErrorValidation, "Validation failed", catalogFieldError{Field: "initialBalance", Message: "initialBalance must be greater than or equal to 0"},
				)),
			}, nil
		case errors.Is(err, domainaccounting.ErrInvalidAccountName),
			errors.Is(err, domainaccounting.ErrInvalidAccountType),
			errors.Is(err, domainaccounting.ErrAccountCurrencyMismatch):
			return generated.CreateAccount422JSONResponse{
				BusinessRuleErrorJSONResponse: generated.BusinessRuleErrorJSONResponse(accountErrorEnvelope(catalogErrorBusinessRuleViolation, "Business rule violation")),
			}, nil
		default:
			return generated.CreateAccount500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}, nil
		}
	}

	mapped, mapErr := toGeneratedAccount(account)
	if mapErr != nil {
		return generated.CreateAccount500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	return generated.CreateAccount201JSONResponse(mapped), nil
}

func (h *APIHandler) GetAccountsSummary(ctx context.Context, request generated.GetAccountsSummaryRequestObject) (generated.GetAccountsSummaryResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.accountsSummary == nil {
		return generated.GetAccountsSummary500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.GetAccountsSummary401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	currency, err := shared.ParseCurrency(strings.TrimSpace(request.Params.Currency))
	if err != nil {
		return generated.GetAccountsSummary400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation,
				"Validation failed",
				catalogFieldError{Field: "currency", Message: "currency must be one of: RUB, USD, EUR"},
			)),
		}, nil
	}

	summary, err := h.catalog.accountsSummary.GetByUserAndCurrency(strictAppContext(ctx), appaccounting.GetAccountsSummaryInput{
		UserID:   userID,
		Currency: currency,
	})
	if err != nil {
		return generated.GetAccountsSummary500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}

	mapped, mapErr := toGeneratedAccountSummary(summary)
	if mapErr != nil {
		return generated.GetAccountsSummary500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	return generated.GetAccountsSummary200JSONResponse(mapped), nil
}

func (h *APIHandler) GetAccount(ctx context.Context, request generated.GetAccountRequestObject) (generated.GetAccountResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.accountsGet == nil {
		return generated.GetAccount500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.GetAccount401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	accountID := strings.TrimSpace(string(request.AccountId))
	if accountID == "" {
		return generated.GetAccount400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "accountId", Message: "accountId is required"},
			)),
		}, nil
	}

	account, err := h.catalog.accountsGet.GetByID(strictAppContext(ctx), userID, shared.AccountID(accountID))
	if err != nil {
		if errors.Is(err, appaccounting.ErrAccountNotFound) {
			return generated.GetAccount404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found")),
			}, nil
		}
		return generated.GetAccount500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}

	mapped, mapErr := toGeneratedAccount(account)
	if mapErr != nil {
		return generated.GetAccount500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	return generated.GetAccount200JSONResponse(mapped), nil
}

func (h *APIHandler) PatchAccount(ctx context.Context, request generated.PatchAccountRequestObject) (generated.PatchAccountResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.accountsUpdate == nil {
		return generated.PatchAccount500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.PatchAccount401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	accountID := strings.TrimSpace(string(request.AccountId))
	if accountID == "" {
		return generated.PatchAccount400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "accountId", Message: "accountId is required"},
			)),
		}, nil
	}
	if request.Body == nil {
		return generated.PatchAccount400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "body", Message: "request body is invalid"},
			)),
		}, nil
	}

	patchReq := patchAccountRequest{
		Name:                 request.Body.Name,
		Type:                 request.Body.Type,
		IncludeInNetWorth:    request.Body.IncludeInNetWorth,
		IncludeInDailyBudget: request.Body.IncludeInDailyBudget,
		Currency:             request.Body.Currency,
	}
	if request.Body.InitialBalance != nil {
		initial := DecimalString(*request.Body.InitialBalance)
		patchReq.InitialBalance = &initial
	}
	if request.Body.Balance != nil {
		balance := DecimalString(*request.Body.Balance)
		patchReq.Balance = &balance
	}

	input, details := validatePatchAccountRequest(userID, shared.AccountID(accountID), patchReq)
	if len(details) > 0 {
		return generated.PatchAccount400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}

	account, err := h.catalog.accountsUpdate.Update(strictAppContext(ctx), input)
	if err != nil {
		switch {
		case errors.Is(err, appaccounting.ErrAccountNotFound):
			return generated.PatchAccount404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found")),
			}, nil
		case errors.Is(err, appaccounting.ErrAccountNameAlreadyExists):
			return generated.PatchAccount409JSONResponse{
				ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(
					catalogErrorConflict, "Conflict", catalogFieldError{Field: "name", Message: "account with this name already exists"},
				)),
			}, nil
		case errors.Is(err, appaccounting.ErrConcurrentAccountUpdate):
			return generated.PatchAccount409JSONResponse{
				ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(
					catalogErrorConflict, "Conflict", catalogFieldError{Field: "body", Message: "account was modified concurrently, retry with fresh state"},
				)),
			}, nil
		default:
			return generated.PatchAccount500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}, nil
		}
	}

	mapped, mapErr := toGeneratedAccount(account)
	if mapErr != nil {
		return generated.PatchAccount500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	return generated.PatchAccount200JSONResponse(mapped), nil
}

func (h *APIHandler) ArchiveAccount(ctx context.Context, request generated.ArchiveAccountRequestObject) (generated.ArchiveAccountResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.accountsArchive == nil {
		return generated.ArchiveAccount500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.ArchiveAccount401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	accountID := strings.TrimSpace(string(request.AccountId))
	if accountID == "" {
		return generated.ArchiveAccount500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}

	account, err := h.catalog.accountsArchive.Archive(strictAppContext(ctx), userID, shared.AccountID(accountID))
	if err != nil {
		if errors.Is(err, appaccounting.ErrAccountNotFound) {
			return generated.ArchiveAccount404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found")),
			}, nil
		}
		return generated.ArchiveAccount500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}

	mapped, mapErr := toGeneratedAccount(account)
	if mapErr != nil {
		return generated.ArchiveAccount500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	return generated.ArchiveAccount200JSONResponse(mapped), nil
}

func (h *APIHandler) RestoreAccount(ctx context.Context, request generated.RestoreAccountRequestObject) (generated.RestoreAccountResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.accountsRestore == nil {
		return generated.RestoreAccount500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.RestoreAccount401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	accountID := strings.TrimSpace(string(request.AccountId))
	if accountID == "" {
		return generated.RestoreAccount500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}

	account, err := h.catalog.accountsRestore.Restore(strictAppContext(ctx), userID, shared.AccountID(accountID))
	if err != nil {
		switch {
		case errors.Is(err, appaccounting.ErrAccountNotFound):
			return generated.RestoreAccount404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found")),
			}, nil
		case errors.Is(err, appaccounting.ErrAccountNameAlreadyExists):
			return generated.RestoreAccount409JSONResponse{
				ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(
					catalogErrorConflict, "Account with this name already exists", catalogFieldError{Field: "name", Message: "account name must be unique per user"},
				)),
			}, nil
		default:
			return generated.RestoreAccount500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}, nil
		}
	}

	mapped, mapErr := toGeneratedAccount(account)
	if mapErr != nil {
		return generated.RestoreAccount500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	return generated.RestoreAccount200JSONResponse(mapped), nil
}

func strictParseLimitOffset(limitRaw *generated.Limit, offsetRaw *generated.Offset) (limit int, offset int, details []catalogFieldError) {
	limit = defaultPageLimit
	offset = 0
	if limitRaw != nil {
		limit = int(*limitRaw)
		if limit <= 0 {
			details = append(details, catalogFieldError{Field: "limit", Message: "limit must be a positive integer"})
		}
	}
	if offsetRaw != nil {
		offset = int(*offsetRaw)
		if offset < 0 {
			details = append(details, catalogFieldError{Field: "offset", Message: "offset must be a non-negative integer"})
		}
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}
	return limit, offset, details
}

func accountErrorEnvelope(code catalogErrorCode, message string, details ...catalogFieldError) generated.ErrorEnvelope {
	errorDetails := make([]generated.ErrorDetail, 0, len(details))
	for _, detail := range details {
		errorDetails = append(errorDetails, generated.ErrorDetail{
			Field:   detail.Field,
			Message: detail.Message,
		})
	}

	errorBody := generated.ErrorBody{
		Code:    generated.ErrorBodyCode(code),
		Message: message,
	}
	if len(errorDetails) > 0 {
		errorBody.Details = &errorDetails
	}
	return generated.ErrorEnvelope{Error: errorBody}
}

func toGeneratedAccount(account domainaccounting.Account) (generated.Account, error) {
	response, err := toAccountResponse(account)
	if err != nil {
		return generated.Account{}, err
	}
	return generated.Account{
		Id:                   response.ID,
		Name:                 response.Name,
		Type:                 response.Type,
		Currency:             response.Currency,
		Balance:              response.Balance,
		InitialBalance:       response.InitialBalance,
		IncludeInNetWorth:    response.IncludeInNetWorth,
		IncludeInDailyBudget: response.IncludeInDailyBudget,
		IsArchived:           response.IsArchived,
		ArchivedAt:           response.ArchivedAt,
		CreatedAt:            response.CreatedAt,
		UpdatedAt:            response.UpdatedAt,
	}, nil
}

func toGeneratedAccountSummary(summary appaccounting.AccountSummary) (generated.AccountSummary, error) {
	response, err := toAccountSummaryResponse(summary)
	if err != nil {
		return generated.AccountSummary{}, err
	}
	accounts := make([]generated.AccountSummaryAccount, 0, len(response.Accounts))
	for _, account := range response.Accounts {
		accounts = append(accounts, generated.AccountSummaryAccount{
			Id:                   account.ID,
			Name:                 account.Name,
			Type:                 account.Type,
			Currency:             account.Currency,
			Balance:              account.Balance,
			IncludeInNetWorth:    account.IncludeInNetWorth,
			IncludeInDailyBudget: account.IncludeInDailyBudget,
		})
	}
	return generated.AccountSummary{
		Currency:                response.Currency,
		NetWorth:                response.NetWorth,
		CashBalance:             response.CashBalance,
		AvailableForDailyBudget: response.AvailableForDailyBudget,
		CreditLiabilities:       response.CreditLiabilities,
		Accounts:                accounts,
	}, nil
}

func (h *APIHandler) ListCategories(ctx context.Context, request generated.ListCategoriesRequestObject) (generated.ListCategoriesResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.categoriesList == nil {
		return generated.ListCategories500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.ListCategories401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	limit, offset, details := strictParseLimitOffset(request.Params.Limit, request.Params.Offset)
	if len(details) > 0 {
		return generated.ListCategories400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}

	includeArchived := false
	if request.Params.IncludeArchived != nil {
		includeArchived = bool(*request.Params.IncludeArchived)
	}
	includeSubcategories := true
	if request.Params.IncludeSubcategories != nil {
		includeSubcategories = bool(*request.Params.IncludeSubcategories)
	}

	var categoryType *domaincatalog.CategoryType
	if request.Params.Type != nil {
		parsedType, err := domaincatalog.ParseCategoryType(strings.TrimSpace(*request.Params.Type))
		if err != nil {
			return generated.ListCategories400JSONResponse{
				ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
					catalogErrorValidation, "Validation failed",
					catalogFieldError{Field: "type", Message: "type must be one of: required, flexible, saving, investment, debt, income"},
				)),
			}, nil
		}
		categoryType = &parsedType
	}

	sortMode := appcatalog.CategorySortSortOrderAsc
	if request.Params.Sort != nil {
		candidate := appcatalog.CategorySort(*request.Params.Sort)
		if !slices.Contains([]appcatalog.CategorySort{
			appcatalog.CategorySortSortOrderAsc,
			appcatalog.CategorySortNameAsc,
			appcatalog.CategorySortCreatedAtDesc,
		}, candidate) {
			return generated.ListCategories400JSONResponse{
				ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
					catalogErrorValidation, "Validation failed",
					catalogFieldError{Field: "sort", Message: "sort must be one of: sortOrder:asc, name:asc, createdAt:desc"},
				)),
			}, nil
		}
		sortMode = candidate
	}

	categories, err := h.catalog.categoriesList.ListByUser(strictAppContext(ctx), appcatalog.ListCategoriesInput{
		UserID:          userID,
		IncludeArchived: includeArchived,
		Type:            categoryType,
		Sort:            sortMode,
	})
	if err != nil {
		return generated.ListCategories500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}

	pagedCategories, total := paginate(categories, limit, offset)
	items := make([]generated.Category, 0, len(pagedCategories))

	if !includeSubcategories {
		for _, category := range pagedCategories {
			items = append(items, toGeneratedCategory(category, nil))
		}
		return generated.ListCategories200JSONResponse{
			Items: items,
			Pagination: generated.Pagination{
				Limit:  limit,
				Offset: offset,
				Total:  total,
			},
		}, nil
	}

	if h.catalog.subcategoriesListByCategory == nil {
		return generated.ListCategories500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}

	for _, category := range pagedCategories {
		subcategories, subcategoriesErr := h.catalog.subcategoriesListByCategory.List(
			ctx,
			appcatalog.ListSubcategoriesByCategoryInput{
				UserID:          userID,
				CategoryID:      category.ID(),
				IncludeArchived: false,
			},
		)
		if subcategoriesErr != nil {
			return generated.ListCategories500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}, nil
		}
		items = append(items, toGeneratedCategory(category, subcategories))
	}

	return generated.ListCategories200JSONResponse{
		Items: items,
		Pagination: generated.Pagination{
			Limit:  limit,
			Offset: offset,
			Total:  total,
		},
	}, nil
}

func (h *APIHandler) CreateCategory(ctx context.Context, request generated.CreateCategoryRequestObject) (generated.CreateCategoryResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.categoriesCreate == nil {
		return generated.CreateCategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.CreateCategory401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}
	if request.Body == nil {
		return generated.CreateCategory400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "body", Message: "request body is invalid"},
			)),
		}, nil
	}

	req := createCategoryRequest{}
	if request.Body.Name != nil {
		req.Name = *request.Body.Name
	}
	if request.Body.Type != nil {
		req.Type = *request.Body.Type
	}
	req.Color = request.Body.Color
	req.SortOrder = request.Body.SortOrder

	input, details := validateCreateCategoryRequest(userID, req)
	if len(details) > 0 {
		return generated.CreateCategory400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}

	category, err := h.catalog.categoriesCreate.Create(strictAppContext(ctx), input)
	if err != nil {
		switch {
		case errors.Is(err, appcatalog.ErrCategoryNameAlreadyExists):
			return generated.CreateCategory409JSONResponse{
				ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(
					catalogErrorConflict, "Category with this name already exists", catalogFieldError{Field: "name", Message: "category name must be unique per user"},
				)),
			}, nil
		case errors.Is(err, domaincatalog.ErrInvalidCategoryName),
			errors.Is(err, domaincatalog.ErrInvalidCategoryType),
			errors.Is(err, domaincatalog.ErrInvalidCategoryColor):
			return generated.CreateCategory400JSONResponse{
				ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
					catalogErrorValidation, "Validation failed", catalogFieldError{Field: "body", Message: "request body is invalid"},
				)),
			}, nil
		default:
			return generated.CreateCategory500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}, nil
		}
	}

	return generated.CreateCategory201JSONResponse(toGeneratedCategory(category, nil)), nil
}

func (h *APIHandler) DeleteCategory(ctx context.Context, request generated.DeleteCategoryRequestObject) (generated.DeleteCategoryResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.categoriesArchive == nil {
		return generated.DeleteCategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.DeleteCategory401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	categoryID := strings.TrimSpace(string(request.CategoryId))
	if categoryID == "" {
		return generated.DeleteCategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}

	category, err := h.catalog.categoriesArchive.Archive(strictAppContext(ctx), userID, shared.CategoryID(categoryID))
	if err != nil {
		if errors.Is(err, appcatalog.ErrCategoryNotFound) {
			return generated.DeleteCategory404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found")),
			}, nil
		}
		return generated.DeleteCategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	return generated.DeleteCategory200JSONResponse(toGeneratedCategory(category, nil)), nil
}

func (h *APIHandler) GetCategory(ctx context.Context, request generated.GetCategoryRequestObject) (generated.GetCategoryResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.categoriesGet == nil {
		return generated.GetCategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.GetCategory401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	categoryID := strings.TrimSpace(string(request.CategoryId))
	if categoryID == "" {
		return generated.GetCategory400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "categoryId", Message: "categoryId is required"},
			)),
		}, nil
	}

	includeSubcategories := true
	if request.Params.IncludeSubcategories != nil {
		includeSubcategories = bool(*request.Params.IncludeSubcategories)
	}

	category, err := h.catalog.categoriesGet.GetByID(strictAppContext(ctx), userID, shared.CategoryID(categoryID))
	if err != nil {
		if errors.Is(err, appcatalog.ErrCategoryNotFound) {
			return generated.GetCategory404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found")),
			}, nil
		}
		return generated.GetCategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}

	var subcategories []domaincatalog.Subcategory
	if includeSubcategories {
		if h.catalog.subcategoriesListByCategory == nil {
			return generated.GetCategory500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}, nil
		}
		listed, subErr := h.catalog.subcategoriesListByCategory.List(strictAppContext(ctx), appcatalog.ListSubcategoriesByCategoryInput{
			UserID:          userID,
			CategoryID:      category.ID(),
			IncludeArchived: false,
		})
		if subErr != nil {
			return generated.GetCategory500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}, nil
		}
		subcategories = listed
	}

	return generated.GetCategory200JSONResponse(toGeneratedCategory(category, subcategories)), nil
}

func (h *APIHandler) PatchCategory(ctx context.Context, request generated.PatchCategoryRequestObject) (generated.PatchCategoryResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.categoriesUpdate == nil {
		return generated.PatchCategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.PatchCategory401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	categoryID := strings.TrimSpace(string(request.CategoryId))
	if categoryID == "" {
		return generated.PatchCategory400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "categoryId", Message: "categoryId is required"},
			)),
		}, nil
	}
	if request.Body == nil {
		return generated.PatchCategory400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "body", Message: "request body is invalid"},
			)),
		}, nil
	}

	req := patchCategoryRequest{
		Name:      request.Body.Name,
		Type:      request.Body.Type,
		Color:     request.Body.Color,
		SortOrder: request.Body.SortOrder,
	}
	input, details := validatePatchCategoryRequest(userID, shared.CategoryID(categoryID), req)
	if len(details) > 0 {
		return generated.PatchCategory400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}

	category, err := h.catalog.categoriesUpdate.Update(strictAppContext(ctx), input)
	if err != nil {
		switch {
		case errors.Is(err, appcatalog.ErrCategoryNotFound):
			return generated.PatchCategory404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found")),
			}, nil
		case errors.Is(err, appcatalog.ErrCategoryNameAlreadyExists):
			return generated.PatchCategory409JSONResponse{
				ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(
					catalogErrorConflict, "Category with this name already exists", catalogFieldError{Field: "name", Message: "category name must be unique per user"},
				)),
			}, nil
		case errors.Is(err, appcatalog.ErrConcurrentCategoryUpdate):
			return generated.PatchCategory409JSONResponse{
				ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(
					catalogErrorConflict, "Conflict", catalogFieldError{Field: "body", Message: "category was modified concurrently, retry with fresh state"},
				)),
			}, nil
		case errors.Is(err, domaincatalog.ErrInvalidCategoryName),
			errors.Is(err, domaincatalog.ErrInvalidCategoryType),
			errors.Is(err, domaincatalog.ErrInvalidCategoryColor):
			return generated.PatchCategory400JSONResponse{
				ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
					catalogErrorValidation, "Validation failed", catalogFieldError{Field: "body", Message: "request body is invalid"},
				)),
			}, nil
		default:
			return generated.PatchCategory500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}, nil
		}
	}

	return generated.PatchCategory200JSONResponse(toGeneratedCategory(category, nil)), nil
}

func (h *APIHandler) RestoreCategory(ctx context.Context, request generated.RestoreCategoryRequestObject) (generated.RestoreCategoryResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.categoriesRestore == nil {
		return generated.RestoreCategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.RestoreCategory401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	categoryID := strings.TrimSpace(string(request.CategoryId))
	if categoryID == "" {
		return generated.RestoreCategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}

	category, err := h.catalog.categoriesRestore.Restore(strictAppContext(ctx), userID, shared.CategoryID(categoryID))
	if err != nil {
		switch {
		case errors.Is(err, appcatalog.ErrCategoryNotFound):
			return generated.RestoreCategory404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found")),
			}, nil
		case errors.Is(err, appcatalog.ErrCategoryNameAlreadyExists):
			return generated.RestoreCategory409JSONResponse{
				ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(
					catalogErrorConflict, "Category with this name already exists", catalogFieldError{Field: "name", Message: "category name must be unique per user"},
				)),
			}, nil
		default:
			return generated.RestoreCategory500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}, nil
		}
	}

	return generated.RestoreCategory200JSONResponse(toGeneratedCategory(category, nil)), nil
}

func (h *APIHandler) ListCategorySubcategories(ctx context.Context, request generated.ListCategorySubcategoriesRequestObject) (generated.ListCategorySubcategoriesResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.subcategoriesListByCategory == nil {
		return generated.ListCategorySubcategories500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.ListCategorySubcategories401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	categoryID := strings.TrimSpace(string(request.CategoryId))
	if categoryID == "" {
		return generated.ListCategorySubcategories400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "categoryId", Message: "categoryId is required"},
			)),
		}, nil
	}

	limit, offset, details := strictParseLimitOffset(request.Params.Limit, request.Params.Offset)
	if len(details) > 0 {
		return generated.ListCategorySubcategories400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}
	includeArchived := false
	if request.Params.IncludeArchived != nil {
		includeArchived = bool(*request.Params.IncludeArchived)
	}

	subcategories, err := h.catalog.subcategoriesListByCategory.List(strictAppContext(ctx), appcatalog.ListSubcategoriesByCategoryInput{
		UserID:          userID,
		CategoryID:      shared.CategoryID(categoryID),
		IncludeArchived: includeArchived,
	})
	if err != nil {
		if errors.Is(err, appcatalog.ErrCategoryNotFound) {
			return generated.ListCategorySubcategories404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found")),
			}, nil
		}
		return generated.ListCategorySubcategories500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}

	items := make([]generated.Subcategory, 0, len(subcategories))
	for _, subcategory := range subcategories {
		items = append(items, toGeneratedSubcategory(subcategory))
	}

	pagedItems, total := paginate(items, limit, offset)
	return generated.ListCategorySubcategories200JSONResponse{
		Items: pagedItems,
		Pagination: generated.Pagination{
			Limit:  limit,
			Offset: offset,
			Total:  total,
		},
	}, nil
}

func (h *APIHandler) CreateSubcategory(ctx context.Context, request generated.CreateSubcategoryRequestObject) (generated.CreateSubcategoryResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.subcategoriesCreate == nil {
		return generated.CreateSubcategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.CreateSubcategory401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	categoryID := strings.TrimSpace(string(request.CategoryId))
	if categoryID == "" {
		return generated.CreateSubcategory400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "categoryId", Message: "categoryId is required"},
			)),
		}, nil
	}
	if request.Body == nil {
		return generated.CreateSubcategory400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "body", Message: "request body is invalid"},
			)),
		}, nil
	}

	req := createSubcategoryRequest{SortOrder: request.Body.SortOrder}
	if request.Body.Name != nil {
		req.Name = *request.Body.Name
	}
	input, details := validateCreateSubcategoryRequest(userID, shared.CategoryID(categoryID), req)
	if len(details) > 0 {
		return generated.CreateSubcategory400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}

	subcategory, err := h.catalog.subcategoriesCreate.Create(strictAppContext(ctx), input)
	if err != nil {
		switch {
		case errors.Is(err, appcatalog.ErrCategoryNotFound):
			return generated.CreateSubcategory404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found")),
			}, nil
		case errors.Is(err, appcatalog.ErrSubcategoryNameAlreadyExists):
			return generated.CreateSubcategory409JSONResponse{
				ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(
					catalogErrorConflict, "Subcategory with this name already exists", catalogFieldError{Field: "name", Message: "subcategory name must be unique per category"},
				)),
			}, nil
		case errors.Is(err, appcatalog.ErrParentCategoryArchived):
			return generated.CreateSubcategory422JSONResponse{
				BusinessRuleErrorJSONResponse: generated.BusinessRuleErrorJSONResponse(accountErrorEnvelope(
					catalogErrorBusinessRuleViolation, "Business rule violation", catalogFieldError{Field: "categoryId", Message: "parent category is archived"},
				)),
			}, nil
		case errors.Is(err, domaincatalog.ErrInvalidSubcategoryName):
			return generated.CreateSubcategory400JSONResponse{
				ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
					catalogErrorValidation, "Validation failed", catalogFieldError{Field: "name", Message: "name must be between 1 and 100 characters"},
				)),
			}, nil
		default:
			return generated.CreateSubcategory500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}, nil
		}
	}

	return generated.CreateSubcategory201JSONResponse(toGeneratedSubcategory(subcategory)), nil
}

func (h *APIHandler) ListSubcategories(ctx context.Context, request generated.ListSubcategoriesRequestObject) (generated.ListSubcategoriesResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.subcategoriesList == nil {
		return generated.ListSubcategories500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.ListSubcategories401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	limit, offset, details := strictParseLimitOffset(request.Params.Limit, request.Params.Offset)
	if len(details) > 0 {
		return generated.ListSubcategories400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}

	subcategories, err := h.catalog.subcategoriesList.ListByUserID(strictAppContext(ctx), userID)
	if err != nil {
		return generated.ListSubcategories500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}

	items := make([]generated.Subcategory, 0, len(subcategories))
	for _, subcategory := range subcategories {
		items = append(items, toGeneratedSubcategory(subcategory))
	}
	pagedItems, total := paginate(items, limit, offset)

	return generated.ListSubcategories200JSONResponse{
		Items: pagedItems,
		Pagination: generated.Pagination{
			Limit:  limit,
			Offset: offset,
			Total:  total,
		},
	}, nil
}

func (h *APIHandler) DeleteSubcategory(ctx context.Context, request generated.DeleteSubcategoryRequestObject) (generated.DeleteSubcategoryResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.subcategoriesArchive == nil {
		return generated.DeleteSubcategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.DeleteSubcategory401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	subcategoryID := strings.TrimSpace(string(request.SubcategoryId))
	if subcategoryID == "" {
		return generated.DeleteSubcategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}

	subcategory, err := h.catalog.subcategoriesArchive.Archive(strictAppContext(ctx), userID, shared.SubcategoryID(subcategoryID))
	if err != nil {
		if errors.Is(err, appcatalog.ErrSubcategoryNotFound) {
			return generated.DeleteSubcategory404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found")),
			}, nil
		}
		return generated.DeleteSubcategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	return generated.DeleteSubcategory200JSONResponse(toGeneratedSubcategory(subcategory)), nil
}

func (h *APIHandler) GetSubcategory(ctx context.Context, request generated.GetSubcategoryRequestObject) (generated.GetSubcategoryResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.subcategoriesGet == nil {
		return generated.GetSubcategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.GetSubcategory401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	subcategoryID := strings.TrimSpace(string(request.SubcategoryId))
	if subcategoryID == "" {
		return generated.GetSubcategory400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "subcategoryId", Message: "subcategoryId is required"},
			)),
		}, nil
	}

	subcategory, err := h.catalog.subcategoriesGet.GetByID(strictAppContext(ctx), userID, shared.SubcategoryID(subcategoryID))
	if err != nil {
		if errors.Is(err, appcatalog.ErrSubcategoryNotFound) {
			return generated.GetSubcategory404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found")),
			}, nil
		}
		return generated.GetSubcategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	return generated.GetSubcategory200JSONResponse(toGeneratedSubcategory(subcategory)), nil
}

func (h *APIHandler) PatchSubcategory(ctx context.Context, request generated.PatchSubcategoryRequestObject) (generated.PatchSubcategoryResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.subcategoriesUpdate == nil {
		return generated.PatchSubcategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.PatchSubcategory401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	subcategoryID := strings.TrimSpace(string(request.SubcategoryId))
	if subcategoryID == "" {
		return generated.PatchSubcategory400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "subcategoryId", Message: "subcategoryId is required"},
			)),
		}, nil
	}
	if request.Body == nil {
		return generated.PatchSubcategory400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "body", Message: "request body is invalid"},
			)),
		}, nil
	}

	req := patchSubcategoryRequest{
		Name:      request.Body.Name,
		SortOrder: request.Body.SortOrder,
	}
	input, details := validatePatchSubcategoryRequest(userID, shared.SubcategoryID(subcategoryID), req)
	if len(details) > 0 {
		return generated.PatchSubcategory400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}

	subcategory, err := h.catalog.subcategoriesUpdate.Update(strictAppContext(ctx), input)
	if err != nil {
		switch {
		case errors.Is(err, appcatalog.ErrSubcategoryNotFound):
			return generated.PatchSubcategory404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found")),
			}, nil
		case errors.Is(err, appcatalog.ErrSubcategoryNameAlreadyExists):
			return generated.PatchSubcategory409JSONResponse{
				ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(
					catalogErrorConflict, "Subcategory with this name already exists", catalogFieldError{Field: "name", Message: "subcategory name must be unique per category"},
				)),
			}, nil
		case errors.Is(err, appcatalog.ErrConcurrentSubcategoryUpdate):
			return generated.PatchSubcategory409JSONResponse{
				ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(
					catalogErrorConflict, "Conflict", catalogFieldError{Field: "body", Message: "subcategory was modified concurrently, retry with fresh state"},
				)),
			}, nil
		case errors.Is(err, domaincatalog.ErrInvalidSubcategoryName):
			return generated.PatchSubcategory400JSONResponse{
				ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
					catalogErrorValidation, "Validation failed", catalogFieldError{Field: "name", Message: "name must be between 1 and 100 characters"},
				)),
			}, nil
		default:
			return generated.PatchSubcategory500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}, nil
		}
	}

	return generated.PatchSubcategory200JSONResponse(toGeneratedSubcategory(subcategory)), nil
}

func (h *APIHandler) RestoreSubcategory(ctx context.Context, request generated.RestoreSubcategoryRequestObject) (generated.RestoreSubcategoryResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.subcategoriesRestore == nil {
		return generated.RestoreSubcategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.RestoreSubcategory401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	subcategoryID := strings.TrimSpace(string(request.SubcategoryId))
	if subcategoryID == "" {
		return generated.RestoreSubcategory500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}

	subcategory, err := h.catalog.subcategoriesRestore.Restore(strictAppContext(ctx), userID, shared.SubcategoryID(subcategoryID))
	if err != nil {
		switch {
		case errors.Is(err, appcatalog.ErrSubcategoryNotFound):
			return generated.RestoreSubcategory404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found")),
			}, nil
		case errors.Is(err, appcatalog.ErrSubcategoryNameAlreadyExists):
			return generated.RestoreSubcategory422JSONResponse{
				BusinessRuleErrorJSONResponse: generated.BusinessRuleErrorJSONResponse(accountErrorEnvelope(
					catalogErrorBusinessRuleViolation, "Business rule violation", catalogFieldError{Field: "name", Message: "subcategory name must be unique per category"},
				)),
			}, nil
		case errors.Is(err, appcatalog.ErrParentCategoryArchived):
			return generated.RestoreSubcategory422JSONResponse{
				BusinessRuleErrorJSONResponse: generated.BusinessRuleErrorJSONResponse(accountErrorEnvelope(
					catalogErrorBusinessRuleViolation, "Business rule violation", catalogFieldError{Field: "categoryId", Message: "parent category is archived"},
				)),
			}, nil
		default:
			return generated.RestoreSubcategory500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}, nil
		}
	}
	return generated.RestoreSubcategory200JSONResponse(toGeneratedSubcategory(subcategory)), nil
}

func toGeneratedCategory(category domaincatalog.Category, subcategories []domaincatalog.Subcategory) generated.Category {
	items := make([]generated.Subcategory, 0, len(subcategories))
	for _, item := range subcategories {
		items = append(items, toGeneratedSubcategory(item))
	}
	return generated.Category{
		Id:            string(category.ID()),
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

func toGeneratedSubcategory(subcategory domaincatalog.Subcategory) generated.Subcategory {
	return generated.Subcategory{
		Id:         string(subcategory.ID()),
		CategoryId: string(subcategory.CategoryID()),
		Name:       subcategory.Name(),
		SortOrder:  subcategory.SortOrder(),
		IsArchived: subcategory.ArchivedAt() != nil,
		ArchivedAt: subcategory.ArchivedAt(),
		CreatedAt:  subcategory.CreatedAt(),
		UpdatedAt:  subcategory.UpdatedAt(),
	}
}

func (h *APIHandler) ListTransactions(ctx context.Context, request generated.ListTransactionsRequestObject) (generated.ListTransactionsResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.transactionsList == nil {
		return generated.ListTransactions500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.ListTransactions401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}

	limit, offset, details := strictParseLimitOffset(request.Params.Limit, request.Params.Offset)
	if len(details) > 0 {
		return generated.ListTransactions400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}

	query, details := strictParseListTransactionsQuery(userID, request.Params)
	if len(details) > 0 {
		return generated.ListTransactions400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}
	query.Limit = limit
	query.Offset = offset

	transactions, err := h.catalog.transactionsList.ListByUser(strictAppContext(ctx), query)
	if err != nil {
		return generated.ListTransactions500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	total, err := h.catalog.transactionsList.CountByUser(strictAppContext(ctx), query)
	if err != nil {
		return generated.ListTransactions500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}

	items := make([]generated.Transaction, 0, len(transactions))
	for _, transaction := range transactions {
		item, mapErr := toGeneratedTransaction(transaction)
		if mapErr != nil {
			return generated.ListTransactions500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}, nil
		}
		items = append(items, item)
	}

	return generated.ListTransactions200JSONResponse{
		Items: items,
		Pagination: generated.Pagination{
			Limit:  limit,
			Offset: offset,
			Total:  total,
		},
	}, nil
}

func (h *APIHandler) CreateTransaction(ctx context.Context, request generated.CreateTransactionRequestObject) (generated.CreateTransactionResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.transactionsCreate == nil {
		return generated.CreateTransaction500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.CreateTransaction401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}
	if request.Body == nil {
		return generated.CreateTransaction400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "body", Message: "request body is invalid"},
			)),
		}, nil
	}

	req := createTransactionRequest{
		Amount:             decimalPtrFromStringPtr(request.Body.Amount),
		Type:               valueOrEmpty(request.Body.Type),
		Status:             valueOrEmpty(request.Body.Status),
		Currency:           valueOrEmpty(request.Body.Currency),
		OccurredAt:         request.Body.OccurredAt,
		PlannedAt:          request.Body.PlannedAt,
		AccountFromID:      request.Body.AccountFromId,
		AccountToID:        request.Body.AccountToId,
		CategoryID:         request.Body.CategoryId,
		SubcategoryID:      request.Body.SubcategoryId,
		Comment:            request.Body.Comment,
		BudgetMemberID:     rawJSONPtrFromStringPtr(request.Body.BudgetMemberId),
		IncomeSourceID:     rawJSONPtrFromStringPtr(request.Body.IncomeSourceId),
		DebtID:             rawJSONPtrFromStringPtr(request.Body.DebtId),
		GoalID:             rawJSONPtrFromStringPtr(request.Body.GoalId),
		InvestmentID:       rawJSONPtrFromStringPtr(request.Body.InvestmentId),
		RecurringPaymentID: rawJSONPtrFromStringPtr(request.Body.RecurringPaymentId),
	}

	input, details := validateCreateTransactionRequest(userID, req)
	if len(details) > 0 {
		return generated.CreateTransaction400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}

	transaction, err := h.catalog.transactionsCreate.Create(strictAppContext(ctx), input)
	if err != nil {
		return mapTransactionAppErrorToCreate(err), nil
	}

	response, mapErr := toGeneratedTransaction(transaction)
	if mapErr != nil {
		return generated.CreateTransaction500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	return generated.CreateTransaction201JSONResponse(response), nil
}

func (h *APIHandler) PatchTransactionsBulk(ctx context.Context, request generated.PatchTransactionsBulkRequestObject) (generated.PatchTransactionsBulkResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.transactionsBulkPatch == nil {
		return generated.PatchTransactionsBulk500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.PatchTransactionsBulk401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}
	if details := strictDetectBulkPatchNullFields(ctx); len(details) > 0 {
		return generated.PatchTransactionsBulk400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}
	req := patchTransactionsBulkRequest{}
	if request.Body != nil && request.Body.Items != nil {
		req.Items = make([]patchTransactionsBulkItemRequest, 0, len(*request.Body.Items))
		for _, item := range *request.Body.Items {
			req.Items = append(req.Items, patchTransactionsBulkItemRequest{
				ID:                 valueOrEmpty(item.Id),
				Type:               optionalStringFromPtr(item.Type),
				Status:             optionalStringFromPtr(item.Status),
				Amount:             optionalDecimalFromPtr(item.Amount),
				Currency:           optionalStringFromPtr(item.Currency),
				OccurredAt:         optionalStringFromPtr(item.OccurredAt),
				PlannedAt:          optionalStringFromPtr(item.PlannedAt),
				AccountFromID:      optionalStringFromPtr(item.AccountFromId),
				AccountToID:        optionalStringFromPtr(item.AccountToId),
				CategoryID:         optionalStringFromPtr(item.CategoryId),
				SubcategoryID:      optionalStringFromPtr(item.SubcategoryId),
				Comment:            optionalStringFromPtr(item.Comment),
				BudgetMemberID:     optionalRawFromPtr(item.BudgetMemberId),
				IncomeSourceID:     optionalRawFromPtr(item.IncomeSourceId),
				DebtID:             optionalRawFromPtr(item.DebtId),
				GoalID:             optionalRawFromPtr(item.GoalId),
				InvestmentID:       optionalRawFromPtr(item.InvestmentId),
				RecurringPaymentID: optionalRawFromPtr(item.RecurringPaymentId),
			})
		}
	}

	input, details := validatePatchTransactionsBulkRequest(userID, req)
	if len(details) > 0 {
		return generated.PatchTransactionsBulk400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}

	transactions, err := h.catalog.transactionsBulkPatch.PatchBulk(strictAppContext(ctx), input)
	if err != nil {
		return mapTransactionBulkAppErrorToPatch(err), nil
	}

	items := make([]generated.Transaction, 0, len(transactions))
	for _, transaction := range transactions {
		item, mapErr := toGeneratedTransaction(transaction)
		if mapErr != nil {
			return generated.PatchTransactionsBulk500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}, nil
		}
		items = append(items, item)
	}
	return generated.PatchTransactionsBulk200JSONResponse{
		Items: items,
	}, nil
}

func (h *APIHandler) CreateTransactionsBulk(ctx context.Context, request generated.CreateTransactionsBulkRequestObject) (generated.CreateTransactionsBulkResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.transactionsBulkCreate == nil {
		return generated.CreateTransactionsBulk500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.CreateTransactionsBulk401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}
	req := createTransactionsBulkRequest{}
	if request.Body != nil && request.Body.Items != nil {
		req.Items = make([]createTransactionRequest, 0, len(*request.Body.Items))
		for _, item := range *request.Body.Items {
			req.Items = append(req.Items, createTransactionRequest{
				Type:               valueOrEmpty(item.Type),
				Status:             valueOrEmpty(item.Status),
				Amount:             decimalPtrFromStringPtr(item.Amount),
				Currency:           valueOrEmpty(item.Currency),
				OccurredAt:         item.OccurredAt,
				PlannedAt:          item.PlannedAt,
				AccountFromID:      item.AccountFromId,
				AccountToID:        item.AccountToId,
				CategoryID:         item.CategoryId,
				SubcategoryID:      item.SubcategoryId,
				Comment:            item.Comment,
				BudgetMemberID:     rawJSONPtrFromStringPtr(item.BudgetMemberId),
				IncomeSourceID:     rawJSONPtrFromStringPtr(item.IncomeSourceId),
				DebtID:             rawJSONPtrFromStringPtr(item.DebtId),
				GoalID:             rawJSONPtrFromStringPtr(item.GoalId),
				InvestmentID:       rawJSONPtrFromStringPtr(item.InvestmentId),
				RecurringPaymentID: rawJSONPtrFromStringPtr(item.RecurringPaymentId),
			})
		}
	}

	input, details := validateCreateTransactionsBulkRequest(userID, req)
	if len(details) > 0 {
		return generated.CreateTransactionsBulk400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}

	transactions, err := h.catalog.transactionsBulkCreate.CreateBulk(strictAppContext(ctx), input)
	if err != nil {
		return mapTransactionBulkAppErrorToCreate(err), nil
	}

	items := make([]generated.Transaction, 0, len(transactions))
	for _, transaction := range transactions {
		item, mapErr := toGeneratedTransaction(transaction)
		if mapErr != nil {
			return generated.CreateTransactionsBulk500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}, nil
		}
		items = append(items, item)
	}
	return generated.CreateTransactionsBulk201JSONResponse{
		Items: items,
	}, nil
}

func (h *APIHandler) DeleteTransaction(ctx context.Context, request generated.DeleteTransactionRequestObject) (generated.DeleteTransactionResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.transactionsDelete == nil {
		return generated.DeleteTransaction500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.DeleteTransaction401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}
	transactionID := strings.TrimSpace(string(request.TransactionId))
	if transactionID == "" {
		return generated.DeleteTransaction400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "transactionId", Message: "transactionId is required"},
			)),
		}, nil
	}

	if _, err := h.catalog.transactionsDelete.DeleteByID(strictAppContext(ctx), userID, shared.TransactionID(transactionID)); err != nil {
		return mapTransactionAppErrorToDelete(err), nil
	}
	return generated.DeleteTransaction204Response{}, nil
}

func (h *APIHandler) GetTransaction(ctx context.Context, request generated.GetTransactionRequestObject) (generated.GetTransactionResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.transactionsGet == nil {
		return generated.GetTransaction500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.GetTransaction401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}
	transactionID := strings.TrimSpace(string(request.TransactionId))
	if transactionID == "" {
		return generated.GetTransaction400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "transactionId", Message: "transactionId is required"},
			)),
		}, nil
	}
	transaction, err := h.catalog.transactionsGet.GetByID(strictAppContext(ctx), userID, shared.TransactionID(transactionID))
	if err != nil {
		if errors.Is(err, appaccounting.ErrTransactionNotFound) {
			return generated.GetTransaction404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found")),
			}, nil
		}
		return generated.GetTransaction500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	response, mapErr := toGeneratedTransaction(transaction)
	if mapErr != nil {
		return generated.GetTransaction500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	return generated.GetTransaction200JSONResponse(response), nil
}

func (h *APIHandler) PatchTransaction(ctx context.Context, request generated.PatchTransactionRequestObject) (generated.PatchTransactionResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.transactionsPatch == nil {
		return generated.PatchTransaction500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.PatchTransaction401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}
	transactionID := strings.TrimSpace(string(request.TransactionId))
	if transactionID == "" {
		return generated.PatchTransaction400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "transactionId", Message: "transactionId is required"},
			)),
		}, nil
	}
	if request.Body == nil {
		return generated.PatchTransaction400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "body", Message: "request body is invalid"},
			)),
		}, nil
	}
	if details := strictDetectPatchNullFields(ctx); len(details) > 0 {
		return generated.PatchTransaction400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}

	req := patchTransactionRequest{
		Type:               optionalStringFromPtr(request.Body.Type),
		Status:             optionalStringFromPtr(request.Body.Status),
		Amount:             optionalDecimalFromPtr(request.Body.Amount),
		Currency:           optionalStringFromPtr(request.Body.Currency),
		OccurredAt:         optionalStringFromPtr(request.Body.OccurredAt),
		PlannedAt:          optionalStringFromPtr(request.Body.PlannedAt),
		AccountFromID:      optionalStringFromPtr(request.Body.AccountFromId),
		AccountToID:        optionalStringFromPtr(request.Body.AccountToId),
		CategoryID:         optionalStringFromPtr(request.Body.CategoryId),
		SubcategoryID:      optionalStringFromPtr(request.Body.SubcategoryId),
		Comment:            optionalStringFromPtr(request.Body.Comment),
		BudgetMemberID:     optionalRawFromPtr(request.Body.BudgetMemberId),
		IncomeSourceID:     optionalRawFromPtr(request.Body.IncomeSourceId),
		DebtID:             optionalRawFromPtr(request.Body.DebtId),
		GoalID:             optionalRawFromPtr(request.Body.GoalId),
		InvestmentID:       optionalRawFromPtr(request.Body.InvestmentId),
		RecurringPaymentID: optionalRawFromPtr(request.Body.RecurringPaymentId),
	}

	input, details := validatePatchTransactionRequest(userID, shared.TransactionID(transactionID), req)
	if len(details) > 0 {
		return generated.PatchTransaction400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}

	transaction, err := h.catalog.transactionsPatch.Patch(strictAppContext(ctx), input)
	if err != nil {
		return mapTransactionAppErrorToPatch(err), nil
	}
	response, mapErr := toGeneratedTransaction(transaction)
	if mapErr != nil {
		return generated.PatchTransaction500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	return generated.PatchTransaction200JSONResponse(response), nil
}

func (h *APIHandler) CancelTransaction(ctx context.Context, request generated.CancelTransactionRequestObject) (generated.CancelTransactionResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.transactionsCancel == nil {
		return generated.CancelTransaction500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.CancelTransaction401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}
	if request.Body != nil && len(*request.Body) > 0 {
		return generated.CancelTransaction400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "body", Message: "request body is invalid"},
			)),
		}, nil
	}
	transactionID := strings.TrimSpace(string(request.TransactionId))
	if transactionID == "" {
		return generated.CancelTransaction400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "transactionId", Message: "transactionId is required"},
			)),
		}, nil
	}
	transaction, err := h.catalog.transactionsCancel.CancelByID(strictAppContext(ctx), userID, shared.TransactionID(transactionID))
	if err != nil {
		return mapTransactionAppErrorToCancel(err), nil
	}
	response, mapErr := toGeneratedTransaction(transaction)
	if mapErr != nil {
		return generated.CancelTransaction500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	return generated.CancelTransaction200JSONResponse(response), nil
}

func (h *APIHandler) DuplicateTransaction(ctx context.Context, request generated.DuplicateTransactionRequestObject) (generated.DuplicateTransactionResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.transactionsDuplicate == nil {
		return generated.DuplicateTransaction500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.DuplicateTransaction401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}
	transactionID := strings.TrimSpace(string(request.TransactionId))
	if transactionID == "" {
		return generated.DuplicateTransaction400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "transactionId", Message: "transactionId is required"},
			)),
		}, nil
	}
	if details := strictDetectDuplicateNullFields(ctx); len(details) > 0 {
		return generated.DuplicateTransaction400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}
	req := duplicateTransactionRequest{}
	if request.Body != nil {
		req = duplicateTransactionRequest{
			Status:             optionalStringFromPtr(request.Body.Status),
			OccurredAt:         optionalStringFromPtr(request.Body.OccurredAt),
			PlannedAt:          optionalStringFromPtr(request.Body.PlannedAt),
			Comment:            optionalStringFromPtr(request.Body.Comment),
			BudgetMemberID:     optionalRawFromPtr(request.Body.BudgetMemberId),
			IncomeSourceID:     optionalRawFromPtr(request.Body.IncomeSourceId),
			DebtID:             optionalRawFromPtr(request.Body.DebtId),
			GoalID:             optionalRawFromPtr(request.Body.GoalId),
			InvestmentID:       optionalRawFromPtr(request.Body.InvestmentId),
			RecurringPaymentID: optionalRawFromPtr(request.Body.RecurringPaymentId),
		}
	}

	input, details := validateDuplicateTransactionRequest(userID, shared.TransactionID(transactionID), req)
	if len(details) > 0 {
		return generated.DuplicateTransaction400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(catalogErrorValidation, "Validation failed", details...)),
		}, nil
	}
	transaction, err := h.catalog.transactionsDuplicate.DuplicateByID(strictAppContext(ctx), input)
	if err != nil {
		return mapTransactionAppErrorToDuplicate(err), nil
	}
	response, mapErr := toGeneratedTransaction(transaction)
	if mapErr != nil {
		return generated.DuplicateTransaction500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	return generated.DuplicateTransaction201JSONResponse(response), nil
}

func (h *APIHandler) PostTransaction(ctx context.Context, request generated.PostTransactionRequestObject) (generated.PostTransactionResponseObject, error) {
	if h == nil || h.catalog == nil || h.catalog.transactionsPost == nil {
		return generated.PostTransaction500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.PostTransaction401JSONResponse{
			UnauthorizedErrorJSONResponse: generated.UnauthorizedErrorJSONResponse(accountErrorEnvelope(catalogErrorUnauthorized, "Unauthorized")),
		}, nil
	}
	if request.Body != nil && len(*request.Body) > 0 {
		return generated.PostTransaction400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "body", Message: "request body is invalid"},
			)),
		}, nil
	}
	transactionID := strings.TrimSpace(string(request.TransactionId))
	if transactionID == "" {
		return generated.PostTransaction400JSONResponse{
			ValidationErrorJSONResponse: generated.ValidationErrorJSONResponse(accountErrorEnvelope(
				catalogErrorValidation, "Validation failed", catalogFieldError{Field: "transactionId", Message: "transactionId is required"},
			)),
		}, nil
	}
	transaction, err := h.catalog.transactionsPost.PostByID(strictAppContext(ctx), userID, shared.TransactionID(transactionID))
	if err != nil {
		return mapTransactionAppErrorToPost(err), nil
	}
	response, mapErr := toGeneratedTransaction(transaction)
	if mapErr != nil {
		return generated.PostTransaction500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}, nil
	}
	return generated.PostTransaction200JSONResponse(response), nil
}

func toGeneratedTransaction(transaction domaintransactions.Transaction) (generated.Transaction, error) {
	response, err := toTransactionResponse(transaction)
	if err != nil {
		return generated.Transaction{}, err
	}
	return generated.Transaction{
		Id:                 response.ID,
		Type:               response.Type,
		Status:             response.Status,
		Amount:             response.Amount,
		Currency:           response.Currency,
		OccurredAt:         response.OccurredAt,
		PlannedAt:          response.PlannedAt,
		AccountFromId:      response.AccountFromID,
		AccountToId:        response.AccountToID,
		CategoryId:         response.CategoryID,
		SubcategoryId:      response.SubcategoryID,
		BudgetMemberId:     response.BudgetMemberID,
		IncomeSourceId:     response.IncomeSourceID,
		DebtId:             response.DebtID,
		GoalId:             response.GoalID,
		InvestmentId:       response.InvestmentID,
		RecurringPaymentId: response.RecurringPaymentID,
		Comment:            response.Comment,
		CreatedAt:          response.CreatedAt,
		UpdatedAt:          response.UpdatedAt,
	}, nil
}

func strictParseListTransactionsQuery(userID shared.UserID, params generated.ListTransactionsParams) (appaccounting.ListTransactionsQuery, []catalogFieldError) {
	query := appaccounting.ListTransactionsQuery{UserID: userID}
	details := make([]catalogFieldError, 0, 4)

	if params.Month != nil {
		rawMonth := strings.TrimSpace(*params.Month)
		if rawMonth != "" {
			monthStart, err := time.Parse(monthLayout, rawMonth)
			if err != nil {
				details = append(details, catalogFieldError{Field: "month", Message: "month must be YYYY-MM"})
			} else {
				from := monthStart.UTC()
				to := from.AddDate(0, 1, 0).Add(-time.Nanosecond)
				query.EffectiveFrom = &from
				query.EffectiveTo = &to
			}
		}
	}
	if params.From != nil {
		from := params.From.Time.UTC()
		query.EffectiveFrom = &from
	}
	if params.To != nil {
		to := params.To.Time.UTC().Add(24*time.Hour - time.Nanosecond)
		query.EffectiveTo = &to
	}
	if params.Type != nil {
		parsed, err := domaintransactions.ParseTransactionType(strings.TrimSpace(*params.Type))
		if err != nil {
			details = append(details, catalogFieldError{Field: "type", Message: "type must be one of: income, expense, transfer, investment, saving"})
		} else {
			query.Type = &parsed
		}
	}
	if params.Status != nil {
		parsed, err := domaintransactions.ParseTransactionStatus(strings.TrimSpace(*params.Status))
		if err != nil {
			details = append(details, catalogFieldError{Field: "status", Message: "status must be one of: planned, posted, cancelled"})
		} else {
			query.Status = &parsed
		}
	}
	if params.AccountId != nil && strings.TrimSpace(*params.AccountId) != "" {
		accountID := shared.AccountID(strings.TrimSpace(*params.AccountId))
		query.AccountID = &accountID
	}
	if params.CategoryId != nil && strings.TrimSpace(*params.CategoryId) != "" {
		categoryID := shared.CategoryID(strings.TrimSpace(*params.CategoryId))
		query.CategoryID = &categoryID
	}
	if params.SubcategoryId != nil && strings.TrimSpace(*params.SubcategoryId) != "" {
		subcategoryID := shared.SubcategoryID(strings.TrimSpace(*params.SubcategoryId))
		query.SubcategoryID = &subcategoryID
	}
	if params.Sort != nil {
		switch *params.Sort {
		case generated.ListTransactionsParamsSortOccurredAtDesc:
			query.Sort = appaccounting.TransactionsSortEffectiveAtDesc
		case generated.ListTransactionsParamsSortOccurredAtAsc:
			query.Sort = appaccounting.TransactionsSortEffectiveAtAsc
		case generated.ListTransactionsParamsSortCreatedAtDesc:
			query.Sort = appaccounting.TransactionsSortCreatedAtDesc
		case generated.ListTransactionsParamsSortCreatedAtAsc:
			query.Sort = appaccounting.TransactionsSortCreatedAtAsc
		case generated.ListTransactionsParamsSortAmountDesc:
			query.Sort = appaccounting.TransactionsSortAmountDesc
		case generated.ListTransactionsParamsSortAmountAsc:
			query.Sort = appaccounting.TransactionsSortAmountAsc
		default:
			details = append(details, catalogFieldError{Field: "sort", Message: "sort must be one of: occurredAt:desc, occurredAt:asc, createdAt:desc, createdAt:asc, amount:desc, amount:asc"})
		}
	}
	if params.Q != nil {
		raw := strings.TrimSpace(*params.Q)
		if raw != "" {
			query.Search = &raw
		}
	}
	if params.BudgetMemberId != nil && strings.TrimSpace(*params.BudgetMemberId) != "" {
		details = append(details, catalogFieldError{Field: "budgetMemberId", Message: "budgetMemberId is not supported in MVP1"})
	}
	return query, details
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func decimalPtrFromStringPtr(v *string) *DecimalString {
	if v == nil {
		return nil
	}
	d := DecimalString(*v)
	return &d
}

func rawJSONPtrFromStringPtr(v *string) *json.RawMessage {
	if v == nil {
		return nil
	}
	raw := json.RawMessage([]byte(*v))
	return &raw
}

func optionalStringFromPtr(v *string) optionalString {
	return optionalString{Set: v != nil, Value: v}
}

func optionalDecimalFromPtr(v *string) optionalDecimal {
	if v == nil {
		return optionalDecimal{Set: false, Value: nil}
	}
	d := DecimalString(*v)
	return optionalDecimal{Set: true, Value: &d}
}

func optionalRawFromPtr(v *string) optionalRawValue {
	if v == nil {
		return optionalRawValue{Set: false, IsNil: false}
	}
	return optionalRawValue{Set: true, IsNil: false}
}

func mapTransactionAppErrorToCreate(err error) generated.CreateTransactionResponseObject {
	switch {
	case errors.Is(err, appaccounting.ErrTransactionNotFound):
		return generated.CreateTransaction404JSONResponse{NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found"))}
	case isTransactionConflictError(err):
		return generated.CreateTransaction409JSONResponse{ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(catalogErrorConflict, "Conflict"))}
	case isTransactionBusinessRuleError(err):
		return generated.CreateTransaction422JSONResponse{BusinessRuleErrorJSONResponse: generated.BusinessRuleErrorJSONResponse(accountErrorEnvelope(catalogErrorBusinessRuleViolation, "Business rule violation"))}
	default:
		return generated.CreateTransaction500JSONResponse{InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error"))}
	}
}

func mapTransactionAppErrorToPatch(err error) generated.PatchTransactionResponseObject {
	switch {
	case errors.Is(err, appaccounting.ErrTransactionNotFound):
		return generated.PatchTransaction404JSONResponse{NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found"))}
	case isTransactionConflictError(err):
		return generated.PatchTransaction409JSONResponse{ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(catalogErrorConflict, "Conflict"))}
	case isTransactionBusinessRuleError(err):
		return generated.PatchTransaction422JSONResponse{BusinessRuleErrorJSONResponse: generated.BusinessRuleErrorJSONResponse(accountErrorEnvelope(catalogErrorBusinessRuleViolation, "Business rule violation"))}
	default:
		return generated.PatchTransaction500JSONResponse{InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error"))}
	}
}

func mapTransactionAppErrorToDelete(err error) generated.DeleteTransactionResponseObject {
	switch {
	case errors.Is(err, appaccounting.ErrTransactionNotFound):
		return generated.DeleteTransaction404JSONResponse{NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found"))}
	case isTransactionConflictError(err):
		return generated.DeleteTransaction409JSONResponse{ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(catalogErrorConflict, "Conflict"))}
	default:
		return generated.DeleteTransaction500JSONResponse{InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error"))}
	}
}

func mapTransactionAppErrorToPost(err error) generated.PostTransactionResponseObject {
	switch {
	case errors.Is(err, appaccounting.ErrTransactionNotFound):
		return generated.PostTransaction404JSONResponse{NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found"))}
	case isTransactionConflictError(err):
		return generated.PostTransaction409JSONResponse{ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(catalogErrorConflict, "Conflict"))}
	default:
		return generated.PostTransaction500JSONResponse{InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error"))}
	}
}

func mapTransactionAppErrorToCancel(err error) generated.CancelTransactionResponseObject {
	switch {
	case errors.Is(err, appaccounting.ErrTransactionNotFound):
		return generated.CancelTransaction404JSONResponse{NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found"))}
	case isTransactionConflictError(err):
		return generated.CancelTransaction409JSONResponse{ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(catalogErrorConflict, "Conflict"))}
	default:
		return generated.CancelTransaction500JSONResponse{InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error"))}
	}
}

func mapTransactionAppErrorToDuplicate(err error) generated.DuplicateTransactionResponseObject {
	switch {
	case errors.Is(err, appaccounting.ErrTransactionNotFound):
		return generated.DuplicateTransaction404JSONResponse{NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(catalogErrorNotFound, "Resource not found"))}
	case isTransactionConflictError(err):
		return generated.DuplicateTransaction409JSONResponse{ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(catalogErrorConflict, "Conflict"))}
	case isTransactionBusinessRuleError(err):
		return generated.DuplicateTransaction422JSONResponse{BusinessRuleErrorJSONResponse: generated.BusinessRuleErrorJSONResponse(accountErrorEnvelope(catalogErrorBusinessRuleViolation, "Business rule violation"))}
	default:
		return generated.DuplicateTransaction500JSONResponse{InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error"))}
	}
}

func mapTransactionBulkAppErrorToCreate(err error) generated.CreateTransactionsBulkResponseObject {
	var itemErr *appaccounting.BulkItemError
	if errors.As(err, &itemErr) {
		field := indexedItemField(itemErr.Index, itemErr.Field)
		switch {
		case errors.Is(itemErr.Err, appaccounting.ErrTransactionNotFound):
			return generated.CreateTransactionsBulk404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(
					catalogErrorNotFound, "Resource not found", catalogFieldError{Field: field, Message: "transaction not found"},
				)),
			}
		case isTransactionConflictError(itemErr.Err):
			msg := itemErr.Err.Error()
			if itemErr.Field == "amount" {
				msg = "posted transaction amount cannot be changed; cancel and duplicate instead"
			}
			return generated.CreateTransactionsBulk409JSONResponse{
				ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(
					catalogErrorConflict, "Transaction cannot be updated", catalogFieldError{Field: field, Message: msg},
				)),
			}
		case isTransactionBusinessRuleError(itemErr.Err):
			return generated.CreateTransactionsBulk422JSONResponse{
				BusinessRuleErrorJSONResponse: generated.BusinessRuleErrorJSONResponse(accountErrorEnvelope(
					catalogErrorBusinessRuleViolation, "Business rule violation", catalogFieldError{Field: field, Message: itemErr.Err.Error()},
				)),
			}
		default:
			return generated.CreateTransactionsBulk500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}
		}
	}

	switch {
	case isTransactionConflictError(err):
		return generated.CreateTransactionsBulk409JSONResponse{
			ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(catalogErrorConflict, "Conflict")),
		}
	case isTransactionBusinessRuleError(err):
		return generated.CreateTransactionsBulk422JSONResponse{
			BusinessRuleErrorJSONResponse: generated.BusinessRuleErrorJSONResponse(accountErrorEnvelope(catalogErrorBusinessRuleViolation, "Business rule violation")),
		}
	default:
		return generated.CreateTransactionsBulk500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}
	}
}

func mapTransactionBulkAppErrorToPatch(err error) generated.PatchTransactionsBulkResponseObject {
	var itemErr *appaccounting.BulkItemError
	if errors.As(err, &itemErr) {
		field := indexedItemField(itemErr.Index, itemErr.Field)
		switch {
		case errors.Is(itemErr.Err, appaccounting.ErrTransactionNotFound):
			return generated.PatchTransactionsBulk404JSONResponse{
				NotFoundErrorJSONResponse: generated.NotFoundErrorJSONResponse(accountErrorEnvelope(
					catalogErrorNotFound, "Resource not found", catalogFieldError{Field: field, Message: "transaction not found"},
				)),
			}
		case isTransactionConflictError(itemErr.Err):
			msg := itemErr.Err.Error()
			if itemErr.Field == "amount" {
				msg = "posted transaction amount cannot be changed; cancel and duplicate instead"
			}
			return generated.PatchTransactionsBulk409JSONResponse{
				ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(
					catalogErrorConflict, "Transaction cannot be updated", catalogFieldError{Field: field, Message: msg},
				)),
			}
		case isTransactionBusinessRuleError(itemErr.Err):
			return generated.PatchTransactionsBulk422JSONResponse{
				BusinessRuleErrorJSONResponse: generated.BusinessRuleErrorJSONResponse(accountErrorEnvelope(
					catalogErrorBusinessRuleViolation, "Business rule violation", catalogFieldError{Field: field, Message: itemErr.Err.Error()},
				)),
			}
		default:
			return generated.PatchTransactionsBulk500JSONResponse{
				InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
			}
		}
	}

	switch {
	case isTransactionConflictError(err):
		return generated.PatchTransactionsBulk409JSONResponse{
			ConflictErrorJSONResponse: generated.ConflictErrorJSONResponse(accountErrorEnvelope(catalogErrorConflict, "Conflict")),
		}
	case isTransactionBusinessRuleError(err):
		return generated.PatchTransactionsBulk422JSONResponse{
			BusinessRuleErrorJSONResponse: generated.BusinessRuleErrorJSONResponse(accountErrorEnvelope(catalogErrorBusinessRuleViolation, "Business rule violation")),
		}
	default:
		return generated.PatchTransactionsBulk500JSONResponse{
			InternalErrorJSONResponse: generated.InternalErrorJSONResponse(accountErrorEnvelope(catalogErrorInternal, "Internal error")),
		}
	}
}

func strictDetectPatchNullFields(ctx context.Context) []catalogFieldError {
	raw := strictRawRequestBody(ctx)
	if len(raw) == 0 {
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	fields := []string{"type", "status", "amount", "currency"}
	details := make([]catalogFieldError, 0, len(fields))
	for _, field := range fields {
		if value, ok := obj[field]; ok && isJSONNullValue(value) {
			details = append(details, catalogFieldError{Field: field, Message: field + " must not be null"})
		}
	}
	return details
}

func strictDetectBulkPatchNullFields(ctx context.Context) []catalogFieldError {
	raw := strictRawRequestBody(ctx)
	if len(raw) == 0 {
		return nil
	}
	var payload struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	if len(payload.Items) == 0 {
		return nil
	}
	fields := []string{"type", "status", "amount", "currency"}
	details := make([]catalogFieldError, 0)
	for idx, itemRaw := range payload.Items {
		var item map[string]json.RawMessage
		if err := json.Unmarshal(itemRaw, &item); err != nil {
			continue
		}
		for _, field := range fields {
			if value, ok := item[field]; ok && isJSONNullValue(value) {
				details = append(details, catalogFieldError{
					Field:   "items[" + strconv.Itoa(idx) + "]." + field,
					Message: field + " must not be null",
				})
			}
		}
	}
	return details
}

func strictDetectDuplicateNullFields(ctx context.Context) []catalogFieldError {
	raw := strictRawRequestBody(ctx)
	if len(raw) == 0 {
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	fields := []string{"status"}
	details := make([]catalogFieldError, 0, len(fields))
	for _, field := range fields {
		if value, ok := obj[field]; ok && isJSONNullValue(value) {
			details = append(details, catalogFieldError{Field: field, Message: field + " must not be null"})
		}
	}
	return details
}

func strictRawRequestBody(ctx context.Context) []byte {
	carrier, ok := ctx.(interface {
		Get(string) (any, bool)
	})
	if !ok {
		return nil
	}
	value, exists := carrier.Get(rawRequestBodyContextKey)
	if !exists {
		return nil
	}
	body, ok := value.([]byte)
	if !ok {
		return nil
	}
	return body
}

func isJSONNullValue(value []byte) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func isTransactionConflictError(err error) bool {
	return errors.Is(err, appaccounting.ErrConcurrentTransactionUpdate) ||
		errors.Is(err, appaccounting.ErrTransactionAlreadyPosted) ||
		errors.Is(err, appaccounting.ErrTransactionAlreadyCancelled) ||
		errors.Is(err, appaccounting.ErrPostedTransactionPatchConflict) ||
		errors.Is(err, appaccounting.ErrCancelledTransactionPatchConflict) ||
		errors.Is(err, appaccounting.ErrPostedTransactionDeleteConflict)
}

func isTransactionBusinessRuleError(err error) bool {
	return errors.Is(err, appaccounting.ErrAccountNotFound) ||
		errors.Is(err, domaintransactions.ErrInvalidTransactionType) ||
		errors.Is(err, domaintransactions.ErrInvalidTransactionStatus) ||
		errors.Is(err, domaintransactions.ErrTransactionAmountMustBeNonNegative) ||
		errors.Is(err, domaintransactions.ErrTransactionAccountFromRequired) ||
		errors.Is(err, domaintransactions.ErrTransactionAccountToRequired) ||
		errors.Is(err, domaintransactions.ErrTransactionAccountFromMustBeEmpty) ||
		errors.Is(err, domaintransactions.ErrTransactionAccountToMustBeEmpty) ||
		errors.Is(err, domaintransactions.ErrTransactionCategoryMustBeEmpty) ||
		errors.Is(err, domaintransactions.ErrTransactionSubcategoryMustBeEmpty) ||
		errors.Is(err, domaintransactions.ErrTransactionCategoryRequired) ||
		errors.Is(err, domaintransactions.ErrTransactionTransferAccountsMustDiffer)
}

func (h *APIHandler) RegisterAuth(ctx context.Context, request generated.RegisterAuthRequestObject) (generated.RegisterAuthResponseObject, error) {
	if h == nil || h.auth == nil {
		return generated.RegisterAuth500JSONResponse{Error: "internal_error"}, nil
	}
	if request.Body == nil || request.Body.Email == nil || request.Body.Password == nil || request.Body.PasswordConfirm == nil {
		return generated.RegisterAuth400JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_request"}}, nil
	}

	tokens, err := h.auth.auth.Register(strictAppContext(ctx), appidentity.RegisterInput{
		Email:           *request.Body.Email,
		Password:        *request.Body.Password,
		PasswordConfirm: *request.Body.PasswordConfirm,
	})
	if err != nil {
		return mapRegisterAuthError(err), nil
	}

	if ginCtx, ctxErr := ginContextFromContext(ctx); ctxErr == nil {
		setRefreshCookie(ginCtx, tokens.RefreshToken)
	}

	return generated.RegisterAuth201JSONResponse{
		AccessToken: tokens.AccessToken,
		ExpiresIn:   int(tokens.ExpiresIn),
	}, nil
}

func (h *APIHandler) LoginAuth(ctx context.Context, request generated.LoginAuthRequestObject) (generated.LoginAuthResponseObject, error) {
	if h == nil || h.auth == nil {
		return generated.LoginAuth500JSONResponse{Error: "internal_error"}, nil
	}
	if request.Body == nil || request.Body.Email == nil || request.Body.Password == nil {
		return generated.LoginAuth400JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_request"}}, nil
	}

	tokens, err := h.auth.auth.Login(strictAppContext(ctx), appidentity.LoginInput{
		Email:    *request.Body.Email,
		Password: *request.Body.Password,
	})
	if err != nil {
		return mapLoginAuthError(err), nil
	}

	if ginCtx, ctxErr := ginContextFromContext(ctx); ctxErr == nil {
		setRefreshCookie(ginCtx, tokens.RefreshToken)
	}

	return generated.LoginAuth200JSONResponse{
		AccessToken: tokens.AccessToken,
		ExpiresIn:   int(tokens.ExpiresIn),
	}, nil
}

func (h *APIHandler) RefreshAuth(ctx context.Context, _ generated.RefreshAuthRequestObject) (generated.RefreshAuthResponseObject, error) {
	if h == nil || h.auth == nil {
		return generated.RefreshAuth500JSONResponse{Error: "internal_error"}, nil
	}
	ginCtx, err := ginContextFromContext(ctx)
	if err != nil {
		return generated.RefreshAuth500JSONResponse{Error: "internal_error"}, nil
	}

	refreshToken, fromCookie, extractErr := extractRefreshToken(ginCtx)
	if extractErr != nil {
		return generated.RefreshAuth400JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_request"}}, nil
	}
	if strings.TrimSpace(refreshToken) == "" {
		return generated.RefreshAuth401JSONResponse{Error: "invalid_refresh_token"}, nil
	}

	tokens, refreshErr := h.auth.auth.Refresh(strictAppContext(ctx), appidentity.RefreshInput{RefreshToken: refreshToken})
	if refreshErr != nil {
		return mapRefreshAuthError(refreshErr), nil
	}
	if fromCookie {
		setRefreshCookie(ginCtx, refreshToken)
	}

	return generated.RefreshAuth200JSONResponse{
		AccessToken: tokens.AccessToken,
		ExpiresIn:   int(tokens.ExpiresIn),
	}, nil
}

func (h *APIHandler) LogoutAuth(ctx context.Context, _ generated.LogoutAuthRequestObject) (generated.LogoutAuthResponseObject, error) {
	if h == nil || h.auth == nil {
		return generated.LogoutAuth500JSONResponse{Error: "internal_error"}, nil
	}
	ginCtx, err := ginContextFromContext(ctx)
	if err != nil {
		return generated.LogoutAuth500JSONResponse{Error: "internal_error"}, nil
	}

	refreshToken, _, extractErr := extractRefreshToken(ginCtx)
	if extractErr != nil {
		clearRefreshCookie(ginCtx)
		return generated.LogoutAuth400JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_request"}}, nil
	}

	if strings.TrimSpace(refreshToken) != "" {
		if logoutErr := h.auth.auth.Logout(strictAppContext(ctx), appidentity.LogoutInput{RefreshToken: refreshToken}); logoutErr != nil && !errors.Is(logoutErr, appidentity.ErrInvalidRefreshToken) {
			return generated.LogoutAuth500JSONResponse{Error: "internal_error"}, nil
		}
	} else {
		accessToken := parseBearerToken(ginCtx.GetHeader("Authorization"))
		if strings.TrimSpace(accessToken) != "" {
			if logoutCurrentErr := h.auth.auth.LogoutCurrent(strictAppContext(ctx), appidentity.LogoutCurrentInput{AccessToken: accessToken}); logoutCurrentErr != nil && !errors.Is(logoutCurrentErr, appidentity.ErrInvalidAccessToken) {
				return generated.LogoutAuth500JSONResponse{Error: "internal_error"}, nil
			}
		}
	}

	clearRefreshCookie(ginCtx)
	return generated.LogoutAuth200JSONResponse{Ok: true}, nil
}

func (h *APIHandler) LogoutAllAuth(ctx context.Context, _ generated.LogoutAllAuthRequestObject) (generated.LogoutAllAuthResponseObject, error) {
	if h == nil || h.auth == nil {
		return generated.LogoutAllAuth500JSONResponse{Error: "internal_error"}, nil
	}
	ginCtx, err := ginContextFromContext(ctx)
	if err != nil {
		return generated.LogoutAllAuth500JSONResponse{Error: "internal_error"}, nil
	}

	accessToken := parseBearerToken(ginCtx.GetHeader("Authorization"))
	if strings.TrimSpace(accessToken) == "" {
		clearRefreshCookie(ginCtx)
		return generated.LogoutAllAuth401JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_access_token"}}, nil
	}

	if logoutErr := h.auth.auth.LogoutAll(strictAppContext(ctx), appidentity.LogoutAllInput{AccessToken: accessToken}); logoutErr != nil {
		return mapLogoutAllAuthError(logoutErr), nil
	}

	clearRefreshCookie(ginCtx)
	return generated.LogoutAllAuth200JSONResponse{Ok: true}, nil
}

func (h *APIHandler) MeAuth(ctx context.Context, _ generated.MeAuthRequestObject) (generated.MeAuthResponseObject, error) {
	ginCtx, err := ginContextFromContext(ctx)
	if err != nil {
		return generated.MeAuth500JSONResponse{Error: "internal_error"}, nil
	}
	user, userOK := UserFromContext(ginCtx)
	_, sessionOK := SessionFromContext(ginCtx)
	if !userOK || !sessionOK {
		return generated.MeAuth401JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_access_token"}}, nil
	}

	return generated.MeAuth200JSONResponse{
		Id:            string(user.ID),
		Email:         user.Email,
		EmailVerified: user.EmailVerified,
		CreatedAt:     user.CreatedAt,
	}, nil
}

func (h *APIHandler) SessionsAuth(ctx context.Context, _ generated.SessionsAuthRequestObject) (generated.SessionsAuthResponseObject, error) {
	if h == nil || h.auth == nil {
		return generated.SessionsAuth500JSONResponse{Error: "internal_error"}, nil
	}
	ginCtx, err := ginContextFromContext(ctx)
	if err != nil {
		return generated.SessionsAuth500JSONResponse{Error: "internal_error"}, nil
	}
	user, userOK := UserFromContext(ginCtx)
	_, sessionOK := SessionFromContext(ginCtx)
	if !userOK || !sessionOK {
		return generated.SessionsAuth401JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_access_token"}}, nil
	}

	sessions, listErr := h.auth.auth.ListActiveSessions(strictAppContext(ctx), appidentity.ListSessionsInput{UserID: user.ID})
	if listErr != nil {
		return generated.SessionsAuth500JSONResponse{Error: "internal_error"}, nil
	}

	response := make([]generated.AuthSession, 0, len(sessions))
	for _, session := range sessions {
		response = append(response, generated.AuthSession{
			Id:         string(session.ID),
			UserAgent:  session.UserAgent,
			Ip:         session.IP,
			DeviceName: session.DeviceName,
			CreatedAt:  session.CreatedAt,
			LastUsedAt: session.LastUsedAt,
			ExpiresAt:  session.ExpiresAt,
		})
	}

	return generated.SessionsAuth200JSONResponse{Sessions: response}, nil
}

func (h *APIHandler) RevokeSessionAuth(ctx context.Context, request generated.RevokeSessionAuthRequestObject) (generated.RevokeSessionAuthResponseObject, error) {
	if h == nil || h.auth == nil {
		return generated.RevokeSessionAuth500JSONResponse{Error: "internal_error"}, nil
	}
	ginCtx, err := ginContextFromContext(ctx)
	if err != nil {
		return generated.RevokeSessionAuth500JSONResponse{Error: "internal_error"}, nil
	}
	user, userOK := UserFromContext(ginCtx)
	_, sessionOK := SessionFromContext(ginCtx)
	if !userOK || !sessionOK {
		return generated.RevokeSessionAuth401JSONResponse{Error: "invalid_access_token"}, nil
	}

	sessionID := strings.TrimSpace(string(request.SessionId))
	if sessionID == "" || !isUUID(sessionID) {
		return generated.RevokeSessionAuth400JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_request"}}, nil
	}

	revokeErr := h.auth.auth.RevokeSession(strictAppContext(ctx), appidentity.RevokeSessionInput{
		UserID:    user.ID,
		SessionID: shared.SessionID(sessionID),
	})
	if revokeErr != nil {
		if errors.Is(revokeErr, appidentity.ErrSessionNotFound) {
			return generated.RevokeSessionAuth404JSONResponse{Error: "session_not_found"}, nil
		}
		return generated.RevokeSessionAuth500JSONResponse{Error: "internal_error"}, nil
	}

	return generated.RevokeSessionAuth204Response{}, nil
}

func (h *APIHandler) ForgotPasswordAuth(ctx context.Context, request generated.ForgotPasswordAuthRequestObject) (generated.ForgotPasswordAuthResponseObject, error) {
	if h == nil || h.auth == nil || h.auth.postMVP == nil {
		return generated.ForgotPasswordAuth500JSONResponse{Error: "internal_error"}, nil
	}
	if request.Body == nil || request.Body.Email == nil {
		return generated.ForgotPasswordAuth400JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_request"}}, nil
	}

	forgotErr := h.auth.postMVP.ForgotPassword(strictAppContext(ctx), appidentity.ForgotPasswordInput{Email: *request.Body.Email})
	if forgotErr != nil {
		return mapPostMVPForgotPasswordError(forgotErr), nil
	}
	return generated.ForgotPasswordAuth200JSONResponse{Ok: true}, nil
}

func (h *APIHandler) ResetPasswordAuth(ctx context.Context, request generated.ResetPasswordAuthRequestObject) (generated.ResetPasswordAuthResponseObject, error) {
	if h == nil || h.auth == nil || h.auth.postMVP == nil {
		return generated.ResetPasswordAuth500JSONResponse{Error: "internal_error"}, nil
	}
	if request.Body == nil || request.Body.Token == nil || request.Body.Password == nil || request.Body.PasswordConfirm == nil {
		return generated.ResetPasswordAuth400JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_request"}}, nil
	}

	resetErr := h.auth.postMVP.ResetPassword(strictAppContext(ctx), appidentity.ResetPasswordInput{
		Token:           *request.Body.Token,
		Password:        *request.Body.Password,
		PasswordConfirm: *request.Body.PasswordConfirm,
	})
	if resetErr != nil {
		return mapPostMVPResetPasswordError(resetErr), nil
	}

	if ginCtx, ctxErr := ginContextFromContext(ctx); ctxErr == nil {
		clearRefreshCookie(ginCtx)
	}
	return generated.ResetPasswordAuth200JSONResponse{Ok: true}, nil
}

func (h *APIHandler) SendVerificationEmailAuth(ctx context.Context, _ generated.SendVerificationEmailAuthRequestObject) (generated.SendVerificationEmailAuthResponseObject, error) {
	if h == nil || h.auth == nil || h.auth.postMVP == nil {
		return generated.SendVerificationEmailAuth500JSONResponse{Error: "internal_error"}, nil
	}
	userID, ok := userIDFromStrictContext(ctx)
	if !ok {
		return generated.SendVerificationEmailAuth401JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_access_token"}}, nil
	}

	sendErr := h.auth.postMVP.SendVerificationEmail(strictAppContext(ctx), appidentity.SendVerificationEmailInput{UserID: userID})
	if sendErr != nil {
		return mapPostMVPSendVerificationEmailError(sendErr), nil
	}
	return generated.SendVerificationEmailAuth200JSONResponse{Ok: true}, nil
}

func (h *APIHandler) VerifyEmailAuth(ctx context.Context, request generated.VerifyEmailAuthRequestObject) (generated.VerifyEmailAuthResponseObject, error) {
	if h == nil || h.auth == nil || h.auth.postMVP == nil {
		return generated.VerifyEmailAuth500JSONResponse{Error: "internal_error"}, nil
	}
	if request.Body == nil || request.Body.Token == nil {
		return generated.VerifyEmailAuth400JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_request"}}, nil
	}

	verifyErr := h.auth.postMVP.VerifyEmail(strictAppContext(ctx), appidentity.VerifyEmailInput{Token: *request.Body.Token})
	if verifyErr != nil {
		return mapPostMVPVerifyEmailError(verifyErr), nil
	}
	return generated.VerifyEmailAuth200JSONResponse{Ok: true}, nil
}

func mapRegisterAuthError(err error) generated.RegisterAuthResponseObject {
	switch {
	case errors.Is(err, appidentity.ErrEmailAlreadyRegistered):
		return generated.RegisterAuth409JSONResponse{Error: "duplicate_email"}
	case errors.Is(err, domainidentity.ErrInvalidEmail),
		errors.Is(err, domainidentity.ErrInvalidPassword),
		errors.Is(err, domainidentity.ErrPasswordConfirmMismatch):
		return generated.RegisterAuth400JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_request"}}
	default:
		return generated.RegisterAuth500JSONResponse{Error: "internal_error"}
	}
}

func mapLoginAuthError(err error) generated.LoginAuthResponseObject {
	switch {
	case errors.Is(err, appidentity.ErrInvalidCredentials):
		return generated.LoginAuth401JSONResponse{Error: "invalid_credentials"}
	case errors.Is(err, domainidentity.ErrInvalidEmail),
		errors.Is(err, domainidentity.ErrInvalidPassword),
		errors.Is(err, domainidentity.ErrPasswordConfirmMismatch):
		return generated.LoginAuth400JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_request"}}
	default:
		return generated.LoginAuth500JSONResponse{Error: "internal_error"}
	}
}

func mapRefreshAuthError(err error) generated.RefreshAuthResponseObject {
	switch {
	case errors.Is(err, appidentity.ErrInvalidRefreshToken):
		return generated.RefreshAuth401JSONResponse{Error: "invalid_refresh_token"}
	case errors.Is(err, domainidentity.ErrInvalidEmail),
		errors.Is(err, domainidentity.ErrInvalidPassword),
		errors.Is(err, domainidentity.ErrPasswordConfirmMismatch):
		return generated.RefreshAuth400JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_request"}}
	default:
		return generated.RefreshAuth500JSONResponse{Error: "internal_error"}
	}
}

func mapLogoutAllAuthError(err error) generated.LogoutAllAuthResponseObject {
	switch {
	case errors.Is(err, appidentity.ErrInvalidAccessToken):
		return generated.LogoutAllAuth401JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_access_token"}}
	default:
		return generated.LogoutAllAuth500JSONResponse{Error: "internal_error"}
	}
}

func mapPostMVPForgotPasswordError(err error) generated.ForgotPasswordAuthResponseObject {
	switch {
	case errors.Is(err, domainidentity.ErrInvalidEmail),
		errors.Is(err, domainidentity.ErrInvalidPassword),
		errors.Is(err, domainidentity.ErrPasswordConfirmMismatch):
		return generated.ForgotPasswordAuth400JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_request"}}
	default:
		return generated.ForgotPasswordAuth500JSONResponse{Error: "internal_error"}
	}
}

func mapPostMVPResetPasswordError(err error) generated.ResetPasswordAuthResponseObject {
	switch {
	case errors.Is(err, appidentity.ErrInvalidPasswordResetToken):
		return generated.ResetPasswordAuth401JSONResponse{Error: "invalid_reset_token"}
	case errors.Is(err, appidentity.ErrInvalidAccessToken):
		return generated.ResetPasswordAuth401JSONResponse{Error: "invalid_access_token"}
	case errors.Is(err, domainidentity.ErrInvalidEmail),
		errors.Is(err, domainidentity.ErrInvalidPassword),
		errors.Is(err, domainidentity.ErrPasswordConfirmMismatch):
		return generated.ResetPasswordAuth400JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_request"}}
	default:
		return generated.ResetPasswordAuth500JSONResponse{Error: "internal_error"}
	}
}

func mapPostMVPSendVerificationEmailError(err error) generated.SendVerificationEmailAuthResponseObject {
	switch {
	case errors.Is(err, appidentity.ErrInvalidAccessToken):
		return generated.SendVerificationEmailAuth401JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_access_token"}}
	default:
		return generated.SendVerificationEmailAuth500JSONResponse{Error: "internal_error"}
	}
}

func mapPostMVPVerifyEmailError(err error) generated.VerifyEmailAuthResponseObject {
	switch {
	case errors.Is(err, appidentity.ErrInvalidEmailVerificationToken):
		return generated.VerifyEmailAuth401JSONResponse{Error: "invalid_verification_token"}
	case errors.Is(err, appidentity.ErrInvalidAccessToken):
		return generated.VerifyEmailAuth401JSONResponse{Error: "invalid_access_token"}
	case errors.Is(err, domainidentity.ErrInvalidEmail),
		errors.Is(err, domainidentity.ErrInvalidPassword),
		errors.Is(err, domainidentity.ErrPasswordConfirmMismatch):
		return generated.VerifyEmailAuth400JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_request"}}
	default:
		return generated.VerifyEmailAuth500JSONResponse{Error: "internal_error"}
	}
}

func strictAppContext(ctx context.Context) context.Context {
	ginCtx, ok := ctx.(*gin.Context)
	if !ok || ginCtx == nil || ginCtx.Request == nil {
		return ctx
	}
	if reqCtx := ginCtx.Request.Context(); reqCtx != nil {
		return reqCtx
	}
	return ctx
}
