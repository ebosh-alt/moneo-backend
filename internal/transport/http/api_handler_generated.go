package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"

	appaccounting "moneo/internal/app/accounting"
	appcatalog "moneo/internal/app/catalog"
	appidentity "moneo/internal/app/identity"
	domainaccounting "moneo/internal/domain/accounting"
	domaincatalog "moneo/internal/domain/catalog"
	domainidentity "moneo/internal/domain/identity"
	"moneo/internal/domain/shared"
	generated "moneo/internal/transport/http/generated"

	"github.com/gin-gonic/gin"
)

// APIHandler adapts generated strict server calls to existing transport handlers.
// It keeps business logic in app/domain and reuses existing HTTP mapping behavior.
type APIHandler struct {
	auth    *AuthHandler
	catalog *CatalogHandler
}

func NewAPIHandler(auth *AuthHandler, catalog *CatalogHandler) *APIHandler {
	return &APIHandler{
		auth:    auth,
		catalog: catalog,
	}
}

func WithAuthStrictHandler(
	auth *AuthHandler,
	strict generated.StrictServerInterface,
) generated.StrictServerInterface {
	if strict == nil {
		authLegacy := NewAPIHandler(auth, nil)
		return NewStrictAPIHandler(StrictAPIHandlerDeps{
			Accounts:      authLegacy,
			Auth:          authLegacy,
			Categories:    authLegacy,
			Subcategories: authLegacy,
			Transactions:  authLegacy,
		})
	}

	if strictHandler, ok := strict.(*StrictAPIHandler); ok {
		authLegacy := NewAPIHandler(auth, nil)
		return NewStrictAPIHandler(StrictAPIHandlerDeps{
			Accounts:      strictHandler.AccountsStrictHandler,
			Auth:          authLegacy,
			Categories:    strictHandler.CategoriesStrictHandler,
			Subcategories: strictHandler.SubcategoriesStrictHandler,
			Transactions:  strictHandler.TransactionsStrictHandler,
		})
	}

	apiHandler, ok := strict.(*APIHandler)
	if !ok {
		return strict
	}
	legacy := NewAPIHandler(auth, apiHandler.catalog)
	return NewStrictAPIHandler(StrictAPIHandlerDeps{
		Accounts:      legacy,
		Auth:          legacy,
		Categories:    legacy,
		Subcategories: legacy,
		Transactions:  legacy,
	})
}

func (h *APIHandler) invokeHandler(
	ctx context.Context,
	request any,
	target any,
	targetName string,
	handlerName string,
	decode func(status int, payload []byte) (any, error),
) (any, error) {
	if h == nil || target == nil {
		return nil, fmt.Errorf("%s handler is not configured", targetName)
	}

	ginCtx, ok := ctx.(*gin.Context)
	if !ok || ginCtx == nil || ginCtx.Request == nil {
		return nil, fmt.Errorf("gin context is required")
	}

	recorder := httptest.NewRecorder()
	proxy, _ := gin.CreateTestContext(recorder)
	proxy.Request = ginCtx.Request.Clone(ginCtx.Request.Context())
	proxy.Params = append(gin.Params(nil), ginCtx.Params...)
	if ginCtx.Keys != nil {
		for k, v := range ginCtx.Keys {
			proxy.Set(k, v)
		}
	}

	if bodyRaw, hasBody := extractRequestBody(request); hasBody {
		if len(bodyRaw) == 0 {
			bodyRaw = []byte("{}")
		}
		proxy.Request.Body = io.NopCloser(bytes.NewReader(bodyRaw))
		proxy.Request.ContentLength = int64(len(bodyRaw))
		proxy.Request.Header = proxy.Request.Header.Clone()
		proxy.Request.Header.Set("Content-Type", "application/json")
	}

	method := reflect.ValueOf(target).MethodByName(handlerName)
	if !method.IsValid() {
		return nil, fmt.Errorf("%s handler method %s not found", targetName, handlerName)
	}
	method.Call([]reflect.Value{reflect.ValueOf(proxy)})
	proxy.Writer.WriteHeaderNow()
	for _, setCookie := range recorder.Header().Values("Set-Cookie") {
		ginCtx.Writer.Header().Add("Set-Cookie", setCookie)
	}

	return decode(recorder.Code, recorder.Body.Bytes())
}

func (h *APIHandler) invokeAuth(
	ctx context.Context,
	request any,
	handlerName string,
	decode func(status int, payload []byte) (any, error),
) (any, error) {
	return h.invokeHandler(ctx, request, h.auth, "auth", handlerName, decode)
}

func (h *APIHandler) invokeCatalog(
	ctx context.Context,
	request any,
	handlerName string,
	decode func(status int, payload []byte) (any, error),
) (any, error) {
	return h.invokeHandler(ctx, request, h.catalog, "catalog", handlerName, decode)
}

func extractRequestBody(request any) ([]byte, bool) {
	rv := reflect.ValueOf(request)
	if rv.Kind() != reflect.Struct {
		return nil, false
	}
	bodyField := rv.FieldByName("Body")
	if !bodyField.IsValid() || bodyField.IsNil() {
		return nil, false
	}

	sparsePayload, ok := toSparseJSONValue(bodyField)
	if !ok {
		return nil, false
	}

	payload, err := json.Marshal(sparsePayload)
	if err != nil {
		return []byte("{}"), true
	}
	return payload, true
}

func toSparseJSONValue(value reflect.Value) (any, bool) {
	if !value.IsValid() {
		return nil, false
	}

	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return nil, false
		}
		return toSparseJSONValue(value.Elem())
	case reflect.Struct:
		result := make(map[string]any)
		valueType := value.Type()
		for i := 0; i < value.NumField(); i++ {
			fieldType := valueType.Field(i)
			if fieldType.PkgPath != "" {
				continue
			}

			jsonTag := fieldType.Tag.Get("json")
			fieldName := fieldType.Name
			if jsonTag != "" {
				tagName := strings.Split(jsonTag, ",")[0]
				if tagName == "-" {
					continue
				}
				if tagName != "" {
					fieldName = tagName
				}
			}

			fieldValue := value.Field(i)
			if fieldValue.Kind() == reflect.Pointer && fieldValue.IsNil() {
				continue
			}

			encodedField, ok := toSparseJSONValue(fieldValue)
			if !ok {
				continue
			}
			result[fieldName] = encodedField
		}
		return result, true
	case reflect.Slice, reflect.Array:
		items := make([]any, 0, value.Len())
		for i := 0; i < value.Len(); i++ {
			item, ok := toSparseJSONValue(value.Index(i))
			if !ok {
				items = append(items, nil)
				continue
			}
			items = append(items, item)
		}
		return items, true
	case reflect.Map:
		if value.IsNil() {
			return nil, false
		}
		return value.Interface(), true
	case reflect.Interface:
		if value.IsNil() {
			return nil, false
		}
		return toSparseJSONValue(value.Elem())
	default:
		return value.Interface(), true
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

	accounts, err := h.catalog.accountsList.ListByUser(ctx, appaccounting.ListAccountsInput{
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

	account, err := h.catalog.accountsCreate.Create(ctx, input)
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

	summary, err := h.catalog.accountsSummary.GetByUserAndCurrency(ctx, appaccounting.GetAccountsSummaryInput{
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

	account, err := h.catalog.accountsGet.GetByID(ctx, userID, shared.AccountID(accountID))
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

	account, err := h.catalog.accountsUpdate.Update(ctx, input)
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

	account, err := h.catalog.accountsArchive.Archive(ctx, userID, shared.AccountID(accountID))
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

	account, err := h.catalog.accountsRestore.Restore(ctx, userID, shared.AccountID(accountID))
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

	categories, err := h.catalog.categoriesList.ListByUser(ctx, appcatalog.ListCategoriesInput{
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

	category, err := h.catalog.categoriesCreate.Create(ctx, input)
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

	category, err := h.catalog.categoriesArchive.Archive(ctx, userID, shared.CategoryID(categoryID))
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

	category, err := h.catalog.categoriesGet.GetByID(ctx, userID, shared.CategoryID(categoryID))
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
		listed, subErr := h.catalog.subcategoriesListByCategory.List(ctx, appcatalog.ListSubcategoriesByCategoryInput{
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

	category, err := h.catalog.categoriesUpdate.Update(ctx, input)
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

	category, err := h.catalog.categoriesRestore.Restore(ctx, userID, shared.CategoryID(categoryID))
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

	subcategories, err := h.catalog.subcategoriesListByCategory.List(ctx, appcatalog.ListSubcategoriesByCategoryInput{
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

	subcategory, err := h.catalog.subcategoriesCreate.Create(ctx, input)
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

	subcategories, err := h.catalog.subcategoriesList.ListByUserID(ctx, userID)
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

	subcategory, err := h.catalog.subcategoriesArchive.Archive(ctx, userID, shared.SubcategoryID(subcategoryID))
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

	subcategory, err := h.catalog.subcategoriesGet.GetByID(ctx, userID, shared.SubcategoryID(subcategoryID))
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

	subcategory, err := h.catalog.subcategoriesUpdate.Update(ctx, input)
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

	subcategory, err := h.catalog.subcategoriesRestore.Restore(ctx, userID, shared.SubcategoryID(subcategoryID))
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
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.ListTransactions200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.ListTransactions400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.ListTransactions401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.ListTransactions500JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		default:
			return nil, fmt.Errorf("unexpected status %d", status)
		}
	}
	result, err := h.invokeCatalog(ctx, request, "ListTransactions", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.ListTransactionsResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for ListTransactions", result)
	}
	return typed, nil
}

func (h *APIHandler) CreateTransaction(ctx context.Context, request generated.CreateTransactionRequestObject) (generated.CreateTransactionResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 201:
			var response generated.CreateTransaction201JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.CreateTransaction400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.CreateTransaction401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.CreateTransaction404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.CreateTransaction409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 422:
			var response generated.CreateTransaction422JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.CreateTransaction500JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		default:
			return nil, fmt.Errorf("unexpected status %d", status)
		}
	}
	result, err := h.invokeCatalog(ctx, request, "CreateTransaction", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.CreateTransactionResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for CreateTransaction", result)
	}
	return typed, nil
}

func (h *APIHandler) PatchTransactionsBulk(ctx context.Context, request generated.PatchTransactionsBulkRequestObject) (generated.PatchTransactionsBulkResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.PatchTransactionsBulk200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.PatchTransactionsBulk400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.PatchTransactionsBulk401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.PatchTransactionsBulk404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.PatchTransactionsBulk409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 422:
			var response generated.PatchTransactionsBulk422JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.PatchTransactionsBulk500JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		default:
			return nil, fmt.Errorf("unexpected status %d", status)
		}
	}
	result, err := h.invokeCatalog(ctx, request, "PatchTransactionsBulk", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.PatchTransactionsBulkResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for PatchTransactionsBulk", result)
	}
	return typed, nil
}

func (h *APIHandler) CreateTransactionsBulk(ctx context.Context, request generated.CreateTransactionsBulkRequestObject) (generated.CreateTransactionsBulkResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 201:
			var response generated.CreateTransactionsBulk201JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.CreateTransactionsBulk400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.CreateTransactionsBulk401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.CreateTransactionsBulk404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.CreateTransactionsBulk409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 422:
			var response generated.CreateTransactionsBulk422JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.CreateTransactionsBulk500JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		default:
			return nil, fmt.Errorf("unexpected status %d", status)
		}
	}
	result, err := h.invokeCatalog(ctx, request, "CreateTransactionsBulk", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.CreateTransactionsBulkResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for CreateTransactionsBulk", result)
	}
	return typed, nil
}

func (h *APIHandler) DeleteTransaction(ctx context.Context, request generated.DeleteTransactionRequestObject) (generated.DeleteTransactionResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 204:
			return generated.DeleteTransaction204Response{}, nil
		case 400:
			var response generated.DeleteTransaction400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.DeleteTransaction401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.DeleteTransaction404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.DeleteTransaction409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.DeleteTransaction500JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		default:
			return nil, fmt.Errorf("unexpected status %d", status)
		}
	}
	result, err := h.invokeCatalog(ctx, request, "DeleteTransaction", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.DeleteTransactionResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for DeleteTransaction", result)
	}
	return typed, nil
}

func (h *APIHandler) GetTransaction(ctx context.Context, request generated.GetTransactionRequestObject) (generated.GetTransactionResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.GetTransaction200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.GetTransaction400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.GetTransaction401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.GetTransaction404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.GetTransaction500JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		default:
			return nil, fmt.Errorf("unexpected status %d", status)
		}
	}
	result, err := h.invokeCatalog(ctx, request, "GetTransaction", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.GetTransactionResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for GetTransaction", result)
	}
	return typed, nil
}

func (h *APIHandler) PatchTransaction(ctx context.Context, request generated.PatchTransactionRequestObject) (generated.PatchTransactionResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.PatchTransaction200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.PatchTransaction400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.PatchTransaction401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.PatchTransaction404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.PatchTransaction409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 422:
			var response generated.PatchTransaction422JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.PatchTransaction500JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		default:
			return nil, fmt.Errorf("unexpected status %d", status)
		}
	}
	result, err := h.invokeCatalog(ctx, request, "PatchTransaction", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.PatchTransactionResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for PatchTransaction", result)
	}
	return typed, nil
}

func (h *APIHandler) CancelTransaction(ctx context.Context, request generated.CancelTransactionRequestObject) (generated.CancelTransactionResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.CancelTransaction200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.CancelTransaction400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.CancelTransaction401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.CancelTransaction404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.CancelTransaction409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.CancelTransaction500JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		default:
			return nil, fmt.Errorf("unexpected status %d", status)
		}
	}
	result, err := h.invokeCatalog(ctx, request, "CancelTransaction", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.CancelTransactionResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for CancelTransaction", result)
	}
	return typed, nil
}

func (h *APIHandler) DuplicateTransaction(ctx context.Context, request generated.DuplicateTransactionRequestObject) (generated.DuplicateTransactionResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 201:
			var response generated.DuplicateTransaction201JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.DuplicateTransaction400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.DuplicateTransaction401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.DuplicateTransaction404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.DuplicateTransaction409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 422:
			var response generated.DuplicateTransaction422JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.DuplicateTransaction500JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		default:
			return nil, fmt.Errorf("unexpected status %d", status)
		}
	}
	result, err := h.invokeCatalog(ctx, request, "DuplicateTransaction", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.DuplicateTransactionResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for DuplicateTransaction", result)
	}
	return typed, nil
}

func (h *APIHandler) PostTransaction(ctx context.Context, request generated.PostTransactionRequestObject) (generated.PostTransactionResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.PostTransaction200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.PostTransaction400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.PostTransaction401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.PostTransaction404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.PostTransaction409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.PostTransaction500JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		default:
			return nil, fmt.Errorf("unexpected status %d", status)
		}
	}
	result, err := h.invokeCatalog(ctx, request, "PostTransaction", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.PostTransactionResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for PostTransaction", result)
	}
	return typed, nil
}

func (h *APIHandler) RegisterAuth(ctx context.Context, request generated.RegisterAuthRequestObject) (generated.RegisterAuthResponseObject, error) {
	if h == nil || h.auth == nil {
		return generated.RegisterAuth500JSONResponse{Error: "internal_error"}, nil
	}
	if request.Body == nil || request.Body.Email == nil || request.Body.Password == nil || request.Body.PasswordConfirm == nil {
		return generated.RegisterAuth400JSONResponse{AuthErrorJSONResponse: generated.AuthErrorJSONResponse{Error: "invalid_request"}}, nil
	}

	tokens, err := h.auth.auth.Register(ctx, appidentity.RegisterInput{
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

	tokens, err := h.auth.auth.Login(ctx, appidentity.LoginInput{
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

	tokens, refreshErr := h.auth.auth.Refresh(ctx, appidentity.RefreshInput{RefreshToken: refreshToken})
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
		if logoutErr := h.auth.auth.Logout(ctx, appidentity.LogoutInput{RefreshToken: refreshToken}); logoutErr != nil && !errors.Is(logoutErr, appidentity.ErrInvalidRefreshToken) {
			return generated.LogoutAuth500JSONResponse{Error: "internal_error"}, nil
		}
	} else {
		accessToken := parseBearerToken(ginCtx.GetHeader("Authorization"))
		if strings.TrimSpace(accessToken) != "" {
			if logoutCurrentErr := h.auth.auth.LogoutCurrent(ctx, appidentity.LogoutCurrentInput{AccessToken: accessToken}); logoutCurrentErr != nil && !errors.Is(logoutCurrentErr, appidentity.ErrInvalidAccessToken) {
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

	if logoutErr := h.auth.auth.LogoutAll(ctx, appidentity.LogoutAllInput{AccessToken: accessToken}); logoutErr != nil {
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

	sessions, listErr := h.auth.auth.ListActiveSessions(ctx, appidentity.ListSessionsInput{UserID: user.ID})
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

	revokeErr := h.auth.auth.RevokeSession(ctx, appidentity.RevokeSessionInput{
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

	forgotErr := h.auth.postMVP.ForgotPassword(ctx, appidentity.ForgotPasswordInput{Email: *request.Body.Email})
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

	resetErr := h.auth.postMVP.ResetPassword(ctx, appidentity.ResetPasswordInput{
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

	sendErr := h.auth.postMVP.SendVerificationEmail(ctx, appidentity.SendVerificationEmailInput{UserID: userID})
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

	verifyErr := h.auth.postMVP.VerifyEmail(ctx, appidentity.VerifyEmailInput{Token: *request.Body.Token})
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
