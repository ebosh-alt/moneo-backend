package http

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

var quotedFieldPattern = regexp.MustCompile(`"([^"]+)"`)

func writeOpenAPIValidationError(c *gin.Context, message string, statusCode int) {
	if statusCode == http.StatusNotFound {
		writeCatalogError(c, http.StatusNotFound, catalogErrorNotFound, "Resource not found")
		c.Abort()
		return
	}

	detailMessage := normalizeValidationMessage(message)
	detailField := inferValidationField(message)
	writeCatalogValidationError(c, catalogFieldError{
		Field:   detailField,
		Message: detailMessage,
	})
	c.Abort()
}

func recoverEmptyBadRequestBody() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if c.Writer.Status() != http.StatusBadRequest {
			return
		}
		if c.Writer.Size() > 0 {
			return
		}
		if len(c.Errors) == 0 {
			return
		}

		lastErr := c.Errors.Last()
		message := "request is invalid"
		if lastErr != nil && lastErr.Err != nil {
			message = lastErr.Err.Error()
		}

		writeCatalogValidationError(c, catalogFieldError{
			Field:   inferValidationField(message),
			Message: normalizeValidationMessage(message),
		})
	}
}

func inferValidationField(message string) string {
	lowerMessage := strings.ToLower(message)
	switch {
	case strings.Contains(lowerMessage, "query argument"):
		if field := quotedValueAfter(message, "Query argument "); field != "" {
			return field
		}
	case strings.Contains(lowerMessage, "parameter "):
		if field := firstQuotedValue(message); field != "" {
			return field
		}
		if field := parameterNameFromMessage(message); field != "" {
			return field
		}
	case strings.Contains(lowerMessage, "request body"):
		return "body"
	}

	if field := firstQuotedValue(message); field != "" {
		return field
	}

	return "request"
}

func normalizeValidationMessage(message string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return "request is invalid"
	}

	const requestErrorPrefix = "error in openapi3filter.RequestError: "
	if strings.HasPrefix(trimmed, requestErrorPrefix) {
		return strings.TrimPrefix(trimmed, requestErrorPrefix)
	}
	return trimmed
}

func firstQuotedValue(message string) string {
	match := quotedFieldPattern.FindStringSubmatch(message)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func quotedValueAfter(message string, prefix string) string {
	start := strings.Index(message, prefix)
	if start == -1 {
		return ""
	}
	rest := strings.TrimSpace(message[start+len(prefix):])
	field := firstQuotedValue(rest)
	if field != "" {
		return field
	}

	end := strings.Index(rest, " ")
	if end == -1 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}

func parameterNameFromMessage(message string) string {
	segment := message
	marker := "parameter "
	idx := strings.Index(strings.ToLower(segment), marker)
	if idx == -1 {
		return ""
	}
	rest := strings.TrimSpace(segment[idx+len(marker):])
	if rest == "" {
		return ""
	}

	end := strings.IndexAny(rest, " :")
	if end == -1 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}
