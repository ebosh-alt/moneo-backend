package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	appaccounting "moneo/internal/app/accounting"
	"moneo/internal/domain/shared"
	domaintransactions "moneo/internal/domain/transactions"

	"github.com/gin-gonic/gin"
)

const (
	dateLayout  = "2006-01-02"
	monthLayout = "2006-01"
)

type createTransactionRequest struct {
	Type               string           `json:"type"`
	Status             string           `json:"status"`
	Amount             *DecimalString   `json:"amount"`
	Currency           string           `json:"currency"`
	OccurredAt         *string          `json:"occurredAt"`
	PlannedAt          *string          `json:"plannedAt"`
	AccountFromID      *string          `json:"accountFromId"`
	AccountToID        *string          `json:"accountToId"`
	CategoryID         *string          `json:"categoryId"`
	SubcategoryID      *string          `json:"subcategoryId"`
	Comment            *string          `json:"comment"`
	BudgetMemberID     *json.RawMessage `json:"budgetMemberId"`
	IncomeSourceID     *json.RawMessage `json:"incomeSourceId"`
	DebtID             *json.RawMessage `json:"debtId"`
	GoalID             *json.RawMessage `json:"goalId"`
	InvestmentID       *json.RawMessage `json:"investmentId"`
	RecurringPaymentID *json.RawMessage `json:"recurringPaymentId"`
}

type patchTransactionRequest struct {
	Type          optionalString  `json:"type"`
	Status        optionalString  `json:"status"`
	Amount        optionalDecimal `json:"amount"`
	Currency      optionalString  `json:"currency"`
	OccurredAt    optionalString  `json:"occurredAt"`
	PlannedAt     optionalString  `json:"plannedAt"`
	AccountFromID optionalString  `json:"accountFromId"`
	AccountToID   optionalString  `json:"accountToId"`
	CategoryID    optionalString  `json:"categoryId"`
	SubcategoryID optionalString  `json:"subcategoryId"`
	Comment       optionalString  `json:"comment"`

	BudgetMemberID     optionalRawValue `json:"budgetMemberId"`
	IncomeSourceID     optionalRawValue `json:"incomeSourceId"`
	DebtID             optionalRawValue `json:"debtId"`
	GoalID             optionalRawValue `json:"goalId"`
	InvestmentID       optionalRawValue `json:"investmentId"`
	RecurringPaymentID optionalRawValue `json:"recurringPaymentId"`
}

type duplicateTransactionRequest struct {
	Status     optionalString `json:"status"`
	OccurredAt optionalString `json:"occurredAt"`
	PlannedAt  optionalString `json:"plannedAt"`
	Comment    optionalString `json:"comment"`

	BudgetMemberID     optionalRawValue `json:"budgetMemberId"`
	IncomeSourceID     optionalRawValue `json:"incomeSourceId"`
	DebtID             optionalRawValue `json:"debtId"`
	GoalID             optionalRawValue `json:"goalId"`
	InvestmentID       optionalRawValue `json:"investmentId"`
	RecurringPaymentID optionalRawValue `json:"recurringPaymentId"`
}

type transactionResponse struct {
	ID                 string    `json:"id"`
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Amount             string    `json:"amount"`
	Currency           string    `json:"currency"`
	OccurredAt         *string   `json:"occurredAt"`
	PlannedAt          *string   `json:"plannedAt"`
	AccountFromID      *string   `json:"accountFromId"`
	AccountToID        *string   `json:"accountToId"`
	CategoryID         *string   `json:"categoryId"`
	SubcategoryID      *string   `json:"subcategoryId"`
	BudgetMemberID     *string   `json:"budgetMemberId"`
	IncomeSourceID     *string   `json:"incomeSourceId"`
	DebtID             *string   `json:"debtId"`
	GoalID             *string   `json:"goalId"`
	InvestmentID       *string   `json:"investmentId"`
	RecurringPaymentID *string   `json:"recurringPaymentId"`
	Comment            *string   `json:"comment"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

func (h *CatalogHandler) CreateTransaction(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.transactionsCreate == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	var req createTransactionRequest
	if err := decodeStrictJSONBody(c, &req); err != nil {
		writeCatalogValidationError(c, catalogFieldError{Field: "body", Message: "request body is invalid"})
		return
	}

	input, details := validateCreateTransactionRequest(user.ID, req)
	if len(details) > 0 {
		writeCatalogValidationError(c, details...)
		return
	}

	transaction, err := h.transactionsCreate.Create(c.Request.Context(), input)
	if err != nil {
		writeTransactionAppError(c, err)
		return
	}

	response, mapErr := toTransactionResponse(transaction)
	if mapErr != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}
	c.JSON(http.StatusCreated, response)
}

func (h *CatalogHandler) ListTransactions(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.transactionsList == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	limit, offset, details := parseLimitOffset(c)
	if len(details) > 0 {
		writeCatalogValidationError(c, details...)
		return
	}

	query, details := parseListTransactionsQuery(user.ID, c)
	if len(details) > 0 {
		writeCatalogValidationError(c, details...)
		return
	}

	transactions, err := h.transactionsList.ListByUser(c.Request.Context(), query)
	if err != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	items := make([]transactionResponse, 0, len(transactions))
	for _, transaction := range transactions {
		item, mapErr := toTransactionResponse(transaction)
		if mapErr != nil {
			writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
			return
		}
		items = append(items, item)
	}

	paged, total := paginate(items, limit, offset)
	c.JSON(http.StatusOK, paginatedResponse[transactionResponse]{
		Items: paged,
		Pagination: paginationMeta{
			Limit:  limit,
			Offset: offset,
			Total:  total,
		},
	})
}

func (h *CatalogHandler) GetTransaction(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.transactionsGet == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	transactionID := strings.TrimSpace(c.Param("transactionId"))
	if transactionID == "" {
		writeCatalogValidationError(c, catalogFieldError{Field: "transactionId", Message: "transactionId is required"})
		return
	}

	transaction, err := h.transactionsGet.GetByID(c.Request.Context(), user.ID, shared.TransactionID(transactionID))
	if err != nil {
		if errors.Is(err, appaccounting.ErrTransactionNotFound) {
			writeCatalogError(c, http.StatusNotFound, catalogErrorNotFound, "Resource not found")
			return
		}
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	response, mapErr := toTransactionResponse(transaction)
	if mapErr != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *CatalogHandler) PatchTransaction(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.transactionsPatch == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	transactionID := strings.TrimSpace(c.Param("transactionId"))
	if transactionID == "" {
		writeCatalogValidationError(c, catalogFieldError{Field: "transactionId", Message: "transactionId is required"})
		return
	}

	var req patchTransactionRequest
	if err := decodeStrictJSONBody(c, &req); err != nil {
		writeCatalogValidationError(c, catalogFieldError{Field: "body", Message: "request body is invalid"})
		return
	}

	input, details := validatePatchTransactionRequest(user.ID, shared.TransactionID(transactionID), req)
	if len(details) > 0 {
		writeCatalogValidationError(c, details...)
		return
	}

	transaction, err := h.transactionsPatch.Patch(c.Request.Context(), input)
	if err != nil {
		writeTransactionAppError(c, err)
		return
	}

	response, mapErr := toTransactionResponse(transaction)
	if mapErr != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *CatalogHandler) DeleteTransaction(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.transactionsDelete == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	transactionID := strings.TrimSpace(c.Param("transactionId"))
	if transactionID == "" {
		writeCatalogValidationError(c, catalogFieldError{Field: "transactionId", Message: "transactionId is required"})
		return
	}

	if _, err := h.transactionsDelete.DeleteByID(c.Request.Context(), user.ID, shared.TransactionID(transactionID)); err != nil {
		writeTransactionAppError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *CatalogHandler) PostTransaction(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.transactionsPost == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	if err := validateOptionalEmptyBody(c); err != nil {
		writeCatalogValidationError(c, catalogFieldError{Field: "body", Message: "request body is invalid"})
		return
	}

	transactionID := strings.TrimSpace(c.Param("transactionId"))
	if transactionID == "" {
		writeCatalogValidationError(c, catalogFieldError{Field: "transactionId", Message: "transactionId is required"})
		return
	}

	transaction, err := h.transactionsPost.PostByID(c.Request.Context(), user.ID, shared.TransactionID(transactionID))
	if err != nil {
		writeTransactionAppError(c, err)
		return
	}

	response, mapErr := toTransactionResponse(transaction)
	if mapErr != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *CatalogHandler) CancelTransaction(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.transactionsCancel == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	if err := validateOptionalEmptyBody(c); err != nil {
		writeCatalogValidationError(c, catalogFieldError{Field: "body", Message: "request body is invalid"})
		return
	}

	transactionID := strings.TrimSpace(c.Param("transactionId"))
	if transactionID == "" {
		writeCatalogValidationError(c, catalogFieldError{Field: "transactionId", Message: "transactionId is required"})
		return
	}

	transaction, err := h.transactionsCancel.CancelByID(c.Request.Context(), user.ID, shared.TransactionID(transactionID))
	if err != nil {
		writeTransactionAppError(c, err)
		return
	}

	response, mapErr := toTransactionResponse(transaction)
	if mapErr != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *CatalogHandler) DuplicateTransaction(c *gin.Context) {
	user, ok := UserFromContext(c)
	if !ok {
		writeCatalogError(c, http.StatusUnauthorized, catalogErrorUnauthorized, "Unauthorized")
		return
	}
	if h.transactionsDuplicate == nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}

	transactionID := strings.TrimSpace(c.Param("transactionId"))
	if transactionID == "" {
		writeCatalogValidationError(c, catalogFieldError{Field: "transactionId", Message: "transactionId is required"})
		return
	}

	var req duplicateTransactionRequest
	if c.Request.Body != nil {
		payload, err := readRequestPayload(c.Request.Body, maxAuthJSONBodyBytes)
		if err != nil {
			writeCatalogValidationError(c, catalogFieldError{Field: "body", Message: "request body is invalid"})
			return
		}
		if len(bytes.TrimSpace(payload)) > 0 {
			if err := decodeStrictJSONPayload(payload, &req); err != nil {
				writeCatalogValidationError(c, catalogFieldError{Field: "body", Message: "request body is invalid"})
				return
			}
		}
	}

	input, details := validateDuplicateTransactionRequest(user.ID, shared.TransactionID(transactionID), req)
	if len(details) > 0 {
		writeCatalogValidationError(c, details...)
		return
	}

	transaction, err := h.transactionsDuplicate.DuplicateByID(c.Request.Context(), input)
	if err != nil {
		writeTransactionAppError(c, err)
		return
	}

	response, mapErr := toTransactionResponse(transaction)
	if mapErr != nil {
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
		return
	}
	c.JSON(http.StatusCreated, response)
}

func validateCreateTransactionRequest(userID shared.UserID, req createTransactionRequest) (appaccounting.CreateTransactionInput, []catalogFieldError) {
	details := make([]catalogFieldError, 0, 8)

	if !reservedRefNil(req.BudgetMemberID) {
		details = append(details, catalogFieldError{Field: "budgetMemberId", Message: "budgetMemberId is not supported in MVP1"})
	}
	if !reservedRefNil(req.IncomeSourceID) {
		details = append(details, catalogFieldError{Field: "incomeSourceId", Message: "incomeSourceId is not supported in MVP1"})
	}
	if !reservedRefNil(req.DebtID) {
		details = append(details, catalogFieldError{Field: "debtId", Message: "debtId is not supported in MVP1"})
	}
	if !reservedRefNil(req.GoalID) {
		details = append(details, catalogFieldError{Field: "goalId", Message: "goalId is not supported in MVP1"})
	}
	if !reservedRefNil(req.InvestmentID) {
		details = append(details, catalogFieldError{Field: "investmentId", Message: "investmentId is not supported in MVP1"})
	}
	if !reservedRefNil(req.RecurringPaymentID) {
		details = append(details, catalogFieldError{Field: "recurringPaymentId", Message: "recurringPaymentId is not supported in MVP1"})
	}

	transactionType, typeErr := domaintransactions.ParseTransactionType(strings.TrimSpace(req.Type))
	if strings.TrimSpace(req.Type) == "" || typeErr != nil {
		details = append(details, catalogFieldError{Field: "type", Message: "type must be one of: income, expense, transfer, investment, saving"})
	}

	status := domaintransactions.TransactionStatusPlanned
	if strings.TrimSpace(req.Status) != "" {
		parsed, statusErr := domaintransactions.ParseTransactionStatus(strings.TrimSpace(req.Status))
		if statusErr != nil {
			details = append(details, catalogFieldError{Field: "status", Message: "status must be one of: planned, posted"})
		} else {
			status = parsed
		}
	}
	if status == domaintransactions.TransactionStatusCancelled {
		details = append(details, catalogFieldError{Field: "status", Message: "status=cancelled is not allowed on create"})
	}

	currency := strings.TrimSpace(req.Currency)
	if currency == "" {
		details = append(details, catalogFieldError{Field: "currency", Message: "currency is required"})
	} else if currency != "RUB" {
		details = append(details, catalogFieldError{Field: "currency", Message: "currency must be RUB in MVP1"})
	}

	var amount shared.Money
	if req.Amount == nil {
		details = append(details, catalogFieldError{Field: "amount", Message: "amount is required"})
	} else {
		parsed, parseErr := ParseMoneyFromREST(*req.Amount, currency, true)
		if parseErr != nil || parsed.MinorUnits() <= 0 {
			details = append(details, catalogFieldError{Field: "amount", Message: "amount must be a valid positive decimal string"})
		} else {
			amount = parsed
		}
	}

	occurredAt, occurredErr := parseDateOptional(req.OccurredAt)
	if occurredErr != nil {
		details = append(details, catalogFieldError{Field: "occurredAt", Message: "occurredAt must be YYYY-MM-DD"})
	}
	plannedAt, plannedErr := parseDateOptional(req.PlannedAt)
	if plannedErr != nil {
		details = append(details, catalogFieldError{Field: "plannedAt", Message: "plannedAt must be YYYY-MM-DD"})
	}

	accountFromID := normalizeIDOptional(req.AccountFromID)
	accountToID := normalizeIDOptional(req.AccountToID)
	categoryID := normalizeCategoryIDOptional(req.CategoryID)
	subcategoryID := normalizeSubcategoryIDOptional(req.SubcategoryID)
	comment := normalizeCommentOptional(req.Comment)

	if len(details) > 0 {
		return appaccounting.CreateTransactionInput{}, details
	}

	if transactionType == domaintransactions.TransactionTypeTransfer {
		if categoryID != nil || subcategoryID != nil {
			details = append(details, catalogFieldError{Field: "categoryId", Message: "transfer cannot have categoryId/subcategoryId"})
		}
	}
	if len(details) > 0 {
		return appaccounting.CreateTransactionInput{}, details
	}

	return appaccounting.CreateTransactionInput{
		UserID:         userID,
		Type:           transactionType,
		Status:         status,
		Amount:         amount,
		AccountFromID:  accountFromID,
		AccountToID:    accountToID,
		CategoryID:     categoryID,
		SubcategoryID:  subcategoryID,
		Comment:        comment,
		OccurredAt:     occurredAt,
		PlannedAt:      plannedAt,
		IncomeSourceID: nil,
	}, nil
}

func parseListTransactionsQuery(userID shared.UserID, c *gin.Context) (appaccounting.ListTransactionsQuery, []catalogFieldError) {
	query := appaccounting.ListTransactionsQuery{
		UserID: userID,
	}
	details := make([]catalogFieldError, 0, 4)

	if rawMonth := strings.TrimSpace(c.Query("month")); rawMonth != "" {
		monthStart, err := time.Parse(monthLayout, rawMonth)
		if err != nil {
			details = append(details, catalogFieldError{Field: "month", Message: "month must be YYYY-MM"})
		} else {
			from := monthStart.UTC()
			to := from.AddDate(0, 1, 0).Add(-time.Nanosecond)
			query.OccurredFrom = &from
			query.OccurredTo = &to
		}
	}
	if rawFrom := strings.TrimSpace(c.Query("from")); rawFrom != "" {
		from, err := time.Parse(dateLayout, rawFrom)
		if err != nil {
			details = append(details, catalogFieldError{Field: "from", Message: "from must be YYYY-MM-DD"})
		} else {
			utc := from.UTC()
			query.OccurredFrom = &utc
		}
	}
	if rawTo := strings.TrimSpace(c.Query("to")); rawTo != "" {
		to, err := time.Parse(dateLayout, rawTo)
		if err != nil {
			details = append(details, catalogFieldError{Field: "to", Message: "to must be YYYY-MM-DD"})
		} else {
			utc := to.UTC().Add(24*time.Hour - time.Nanosecond)
			query.OccurredTo = &utc
		}
	}

	if rawType := strings.TrimSpace(c.Query("type")); rawType != "" {
		parsed, err := domaintransactions.ParseTransactionType(rawType)
		if err != nil {
			details = append(details, catalogFieldError{Field: "type", Message: "type must be one of: income, expense, transfer, investment, saving"})
		} else {
			query.Type = &parsed
		}
	}

	if rawStatus := strings.TrimSpace(c.Query("status")); rawStatus != "" {
		parsed, err := domaintransactions.ParseTransactionStatus(rawStatus)
		if err != nil {
			details = append(details, catalogFieldError{Field: "status", Message: "status must be one of: planned, posted, cancelled"})
		} else {
			query.Status = &parsed
		}
	}

	if rawAccountID := strings.TrimSpace(c.Query("accountId")); rawAccountID != "" {
		accountID := shared.AccountID(rawAccountID)
		query.AccountID = &accountID
	}
	if rawCategoryID := strings.TrimSpace(c.Query("categoryId")); rawCategoryID != "" {
		categoryID := shared.CategoryID(rawCategoryID)
		query.CategoryID = &categoryID
	}
	if rawSubcategoryID := strings.TrimSpace(c.Query("subcategoryId")); rawSubcategoryID != "" {
		subcategoryID := shared.SubcategoryID(rawSubcategoryID)
		query.SubcategoryID = &subcategoryID
	}

	if rawSort := strings.TrimSpace(c.Query("sort")); rawSort != "" {
		switch rawSort {
		case "occurredAt:desc":
			query.Sort = appaccounting.TransactionsSortEffectiveAtDesc
		case "occurredAt:asc":
			query.Sort = appaccounting.TransactionsSortEffectiveAtAsc
		case "createdAt:desc":
			query.Sort = appaccounting.TransactionsSortCreatedAtDesc
		case "amount:desc":
			query.Sort = appaccounting.TransactionsSortAmountDesc
		default:
			details = append(details, catalogFieldError{Field: "sort", Message: "sort must be one of: occurredAt:desc, occurredAt:asc, createdAt:desc, amount:desc"})
		}
	}

	if rawQuery := strings.TrimSpace(c.Query("q")); rawQuery != "" {
		query.Search = &rawQuery
	}

	if rawBudgetMemberID := strings.TrimSpace(c.Query("budgetMemberId")); rawBudgetMemberID != "" {
		details = append(details, catalogFieldError{Field: "budgetMemberId", Message: "budgetMemberId is not supported in MVP1"})
	}

	return query, details
}

func validatePatchTransactionRequest(
	userID shared.UserID,
	transactionID shared.TransactionID,
	req patchTransactionRequest,
) (appaccounting.PatchTransactionInput, []catalogFieldError) {
	details := make([]catalogFieldError, 0, 8)

	if !req.BudgetMemberID.isNullIfSet() {
		details = append(details, catalogFieldError{Field: "budgetMemberId", Message: "budgetMemberId is not supported in MVP1"})
	}
	if !req.IncomeSourceID.isNullIfSet() {
		details = append(details, catalogFieldError{Field: "incomeSourceId", Message: "incomeSourceId is not supported in MVP1"})
	}
	if !req.DebtID.isNullIfSet() {
		details = append(details, catalogFieldError{Field: "debtId", Message: "debtId is not supported in MVP1"})
	}
	if !req.GoalID.isNullIfSet() {
		details = append(details, catalogFieldError{Field: "goalId", Message: "goalId is not supported in MVP1"})
	}
	if !req.InvestmentID.isNullIfSet() {
		details = append(details, catalogFieldError{Field: "investmentId", Message: "investmentId is not supported in MVP1"})
	}
	if !req.RecurringPaymentID.isNullIfSet() {
		details = append(details, catalogFieldError{Field: "recurringPaymentId", Message: "recurringPaymentId is not supported in MVP1"})
	}

	input := appaccounting.PatchTransactionInput{
		UserID:        userID,
		TransactionID: transactionID,
	}

	if req.Type.Set {
		input.TypeSet = true
		if req.Type.Value == nil {
			details = append(details, catalogFieldError{Field: "type", Message: "type must not be null"})
		} else {
			parsed, err := domaintransactions.ParseTransactionType(*req.Type.Value)
			if err != nil {
				details = append(details, catalogFieldError{Field: "type", Message: "type must be one of: income, expense, transfer, investment, saving"})
			} else {
				input.Type = &parsed
			}
		}
	}
	if req.Status.Set {
		input.StatusSet = true
		if req.Status.Value == nil {
			details = append(details, catalogFieldError{Field: "status", Message: "status must not be null"})
		} else {
			parsed, err := domaintransactions.ParseTransactionStatus(*req.Status.Value)
			if err != nil {
				details = append(details, catalogFieldError{Field: "status", Message: "status must be one of: planned, posted, cancelled"})
			} else {
				input.Status = &parsed
			}
		}
	}
	if req.Amount.Set {
		input.AmountSet = true
		if req.Amount.Value == nil {
			details = append(details, catalogFieldError{Field: "amount", Message: "amount must not be null"})
		} else {
			parsed, err := ParseMoneyFromREST(*req.Amount.Value, "RUB", true)
			if err != nil || parsed.MinorUnits() <= 0 {
				details = append(details, catalogFieldError{Field: "amount", Message: "amount must be a valid positive decimal string"})
			} else {
				input.Amount = &parsed
			}
		}
	}
	if req.Currency.Set {
		if req.Currency.Value == nil || strings.TrimSpace(*req.Currency.Value) != "RUB" {
			details = append(details, catalogFieldError{Field: "currency", Message: "currency must be RUB"})
		}
	}

	if req.AccountFromID.Set {
		input.AccountFromIDSet = true
		input.AccountFromID = normalizeIDOptional(req.AccountFromID.Value)
	}
	if req.AccountToID.Set {
		input.AccountToIDSet = true
		input.AccountToID = normalizeIDOptional(req.AccountToID.Value)
	}
	if req.CategoryID.Set {
		input.CategoryIDSet = true
		input.CategoryID = normalizeCategoryIDOptional(req.CategoryID.Value)
	}
	if req.SubcategoryID.Set {
		input.SubcategoryIDSet = true
		input.SubcategoryID = normalizeSubcategoryIDOptional(req.SubcategoryID.Value)
	}
	if req.Comment.Set {
		input.CommentSet = true
		input.Comment = normalizeCommentOptional(req.Comment.Value)
	}
	if req.OccurredAt.Set {
		input.OccurredAtSet = true
		parsed, err := parseDateOptional(req.OccurredAt.Value)
		if err != nil {
			details = append(details, catalogFieldError{Field: "occurredAt", Message: "occurredAt must be YYYY-MM-DD"})
		} else {
			input.OccurredAt = parsed
		}
	}
	if req.PlannedAt.Set {
		input.PlannedAtSet = true
		parsed, err := parseDateOptional(req.PlannedAt.Value)
		if err != nil {
			details = append(details, catalogFieldError{Field: "plannedAt", Message: "plannedAt must be YYYY-MM-DD"})
		} else {
			input.PlannedAt = parsed
		}
	}

	if len(details) > 0 {
		return appaccounting.PatchTransactionInput{}, details
	}
	return input, nil
}

func validateDuplicateTransactionRequest(
	userID shared.UserID,
	transactionID shared.TransactionID,
	req duplicateTransactionRequest,
) (appaccounting.DuplicateTransactionInput, []catalogFieldError) {
	details := make([]catalogFieldError, 0, 6)

	if !req.BudgetMemberID.isNullIfSet() {
		details = append(details, catalogFieldError{Field: "budgetMemberId", Message: "budgetMemberId is not supported in MVP1"})
	}
	if !req.IncomeSourceID.isNullIfSet() {
		details = append(details, catalogFieldError{Field: "incomeSourceId", Message: "incomeSourceId is not supported in MVP1"})
	}
	if !req.DebtID.isNullIfSet() {
		details = append(details, catalogFieldError{Field: "debtId", Message: "debtId is not supported in MVP1"})
	}
	if !req.GoalID.isNullIfSet() {
		details = append(details, catalogFieldError{Field: "goalId", Message: "goalId is not supported in MVP1"})
	}
	if !req.InvestmentID.isNullIfSet() {
		details = append(details, catalogFieldError{Field: "investmentId", Message: "investmentId is not supported in MVP1"})
	}
	if !req.RecurringPaymentID.isNullIfSet() {
		details = append(details, catalogFieldError{Field: "recurringPaymentId", Message: "recurringPaymentId is not supported in MVP1"})
	}

	input := appaccounting.DuplicateTransactionInput{
		UserID:        userID,
		TransactionID: transactionID,
	}

	if req.Status.Set {
		if req.Status.Value == nil {
			details = append(details, catalogFieldError{Field: "status", Message: "status must not be null"})
		} else {
			parsed, err := domaintransactions.ParseTransactionStatus(*req.Status.Value)
			if err != nil {
				details = append(details, catalogFieldError{Field: "status", Message: "status must be one of: planned, posted"})
			} else if parsed == domaintransactions.TransactionStatusCancelled {
				details = append(details, catalogFieldError{Field: "status", Message: "status must be one of: planned, posted"})
			} else {
				input.Status = &parsed
			}
		}
	}
	if req.OccurredAt.Set {
		parsed, err := parseDateOptional(req.OccurredAt.Value)
		if err != nil {
			details = append(details, catalogFieldError{Field: "occurredAt", Message: "occurredAt must be YYYY-MM-DD"})
		} else {
			input.OccurredAt = parsed
		}
	}
	if req.PlannedAt.Set {
		parsed, err := parseDateOptional(req.PlannedAt.Value)
		if err != nil {
			details = append(details, catalogFieldError{Field: "plannedAt", Message: "plannedAt must be YYYY-MM-DD"})
		} else {
			input.PlannedAt = parsed
		}
	}
	if req.Comment.Set {
		input.Comment = normalizeCommentOptional(req.Comment.Value)
	}

	if len(details) > 0 {
		return appaccounting.DuplicateTransactionInput{}, details
	}
	return input, nil
}

func toTransactionResponse(transaction domaintransactions.Transaction) (transactionResponse, error) {
	amount, err := FormatMoneyToREST(transaction.Amount())
	if err != nil {
		return transactionResponse{}, err
	}

	return transactionResponse{
		ID:                 string(transaction.ID()),
		Type:               string(transaction.Type()),
		Status:             string(transaction.Status()),
		Amount:             amount,
		Currency:           transaction.Amount().Currency().String(),
		OccurredAt:         formatDatePtr(transaction.OccurredAt()),
		PlannedAt:          formatDatePtr(transaction.PlannedAt()),
		AccountFromID:      stringPtrFromAccountID(transaction.AccountFromID()),
		AccountToID:        stringPtrFromAccountID(transaction.AccountToID()),
		CategoryID:         stringPtrFromCategoryID(transaction.CategoryID()),
		SubcategoryID:      stringPtrFromSubcategoryID(transaction.SubcategoryID()),
		BudgetMemberID:     nil,
		IncomeSourceID:     nil,
		DebtID:             nil,
		GoalID:             nil,
		InvestmentID:       nil,
		RecurringPaymentID: nil,
		Comment:            transaction.Comment(),
		CreatedAt:          transaction.CreatedAt(),
		UpdatedAt:          transaction.UpdatedAt(),
	}, nil
}

func writeTransactionAppError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, appaccounting.ErrTransactionNotFound):
		writeCatalogError(c, http.StatusNotFound, catalogErrorNotFound, "Resource not found")
	case errors.Is(err, appaccounting.ErrConcurrentTransactionUpdate),
		errors.Is(err, appaccounting.ErrTransactionAlreadyPosted),
		errors.Is(err, appaccounting.ErrTransactionAlreadyCancelled),
		errors.Is(err, appaccounting.ErrPostedTransactionPatchConflict),
		errors.Is(err, appaccounting.ErrCancelledTransactionPatchConflict),
		errors.Is(err, appaccounting.ErrPostedTransactionDeleteConflict):
		writeCatalogError(c, http.StatusConflict, catalogErrorConflict, "Conflict")
	case errors.Is(err, domaintransactions.ErrInvalidTransactionType),
		errors.Is(err, domaintransactions.ErrInvalidTransactionStatus),
		errors.Is(err, domaintransactions.ErrTransactionAmountMustBeNonNegative),
		errors.Is(err, domaintransactions.ErrTransactionAccountFromRequired),
		errors.Is(err, domaintransactions.ErrTransactionAccountToRequired),
		errors.Is(err, domaintransactions.ErrTransactionAccountFromMustBeEmpty),
		errors.Is(err, domaintransactions.ErrTransactionAccountToMustBeEmpty),
		errors.Is(err, domaintransactions.ErrTransactionCategoryRequired),
		errors.Is(err, domaintransactions.ErrTransactionTransferAccountsMustDiffer):
		writeCatalogError(c, http.StatusUnprocessableEntity, catalogErrorBusinessRuleViolation, "Business rule violation")
	default:
		writeCatalogError(c, http.StatusInternalServerError, catalogErrorInternal, "Internal error")
	}
}

func validateOptionalEmptyBody(c *gin.Context) error {
	if c.Request.Body == nil {
		return nil
	}
	payload, err := readRequestPayload(c.Request.Body, maxAuthJSONBodyBytes)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return nil
	}
	var placeholder map[string]any
	if err := decodeStrictJSONPayload(payload, &placeholder); err != nil {
		return err
	}
	if len(placeholder) > 0 {
		return errors.New("body must be empty")
	}
	return nil
}

func parseDateOptional(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := time.Parse(dateLayout, trimmed)
	if err != nil {
		return nil, err
	}
	utc := parsed.UTC()
	return &utc, nil
}

func formatDatePtr(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format(dateLayout)
	return &formatted
}

func normalizeIDOptional(value *string) *shared.AccountID {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	accountID := shared.AccountID(trimmed)
	return &accountID
}

func normalizeCategoryIDOptional(value *string) *shared.CategoryID {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	categoryID := shared.CategoryID(trimmed)
	return &categoryID
}

func normalizeSubcategoryIDOptional(value *string) *shared.SubcategoryID {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	subcategoryID := shared.SubcategoryID(trimmed)
	return &subcategoryID
}

func normalizeCommentOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func stringPtrFromAccountID(value *shared.AccountID) *string {
	if value == nil {
		return nil
	}
	converted := string(*value)
	return &converted
}

func stringPtrFromCategoryID(value *shared.CategoryID) *string {
	if value == nil {
		return nil
	}
	converted := string(*value)
	return &converted
}

func stringPtrFromSubcategoryID(value *shared.SubcategoryID) *string {
	if value == nil {
		return nil
	}
	converted := string(*value)
	return &converted
}

func reservedRefNil(value *json.RawMessage) bool {
	if value == nil {
		return true
	}
	return bytes.Equal(bytes.TrimSpace(*value), []byte("null"))
}

type optionalString struct {
	Set   bool
	Value *string
}

func (o *optionalString) UnmarshalJSON(data []byte) error {
	o.Set = true
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) {
		o.Value = nil
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

type optionalDecimal struct {
	Set   bool
	Value *DecimalString
}

func (o *optionalDecimal) UnmarshalJSON(data []byte) error {
	o.Set = true
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) {
		o.Value = nil
		return nil
	}
	var value DecimalString
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

type optionalRawValue struct {
	Set   bool
	IsNil bool
}

func (o *optionalRawValue) UnmarshalJSON(data []byte) error {
	o.Set = true
	o.IsNil = bytes.Equal(bytes.TrimSpace(data), []byte("null"))
	return nil
}

func (o optionalRawValue) isNullIfSet() bool {
	if !o.Set {
		return true
	}
	return o.IsNil
}
