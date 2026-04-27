package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	defaultPageLimit = 50
	maxPageLimit     = 200
)

type catalogErrorCode string

const (
	catalogErrorValidation            catalogErrorCode = "validation_error"
	catalogErrorUnauthorized          catalogErrorCode = "unauthorized"
	catalogErrorForbidden             catalogErrorCode = "forbidden"
	catalogErrorNotFound              catalogErrorCode = "not_found"
	catalogErrorConflict              catalogErrorCode = "conflict"
	catalogErrorBusinessRuleViolation catalogErrorCode = "business_rule_violation"
	catalogErrorInternal              catalogErrorCode = "internal_error"
)

type catalogFieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type catalogErrorBody struct {
	Code    catalogErrorCode    `json:"code"`
	Message string              `json:"message"`
	Details []catalogFieldError `json:"details,omitempty"`
}

type catalogErrorEnvelope struct {
	Error catalogErrorBody `json:"error"`
}

type paginationMeta struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

type paginatedResponse[T any] struct {
	Items      []T            `json:"items"`
	Pagination paginationMeta `json:"pagination"`
}

func writeCatalogError(
	c *gin.Context,
	status int,
	code catalogErrorCode,
	message string,
	details ...catalogFieldError,
) {
	payload := catalogErrorEnvelope{
		Error: catalogErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
	}
	c.JSON(status, payload)
}

func writeCatalogValidationError(c *gin.Context, details ...catalogFieldError) {
	writeCatalogError(c, http.StatusBadRequest, catalogErrorValidation, "Validation failed", details...)
}

func parseLimitOffset(c *gin.Context) (limit int, offset int, details []catalogFieldError) {
	limit = defaultPageLimit
	offset = 0

	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, err := parsePositiveInt(rawLimit)
		if err != nil {
			details = append(details, catalogFieldError{
				Field:   "limit",
				Message: "limit must be a positive integer",
			})
		} else {
			limit = parsed
		}
	}

	if rawOffset := c.Query("offset"); rawOffset != "" {
		parsed, err := parseNonNegativeInt(rawOffset)
		if err != nil {
			details = append(details, catalogFieldError{
				Field:   "offset",
				Message: "offset must be a non-negative integer",
			})
		} else {
			offset = parsed
		}
	}

	if limit > maxPageLimit {
		limit = maxPageLimit
	}

	return limit, offset, details
}

func paginate[T any](items []T, limit int, offset int) ([]T, int) {
	total := len(items)
	if offset >= total {
		return []T{}, total
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return items[offset:end], total
}

func parsePositiveInt(raw string) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, strconv.ErrSyntax
	}

	return value, nil
}

func parseNonNegativeInt(raw string) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, strconv.ErrSyntax
	}

	return value, nil
}
