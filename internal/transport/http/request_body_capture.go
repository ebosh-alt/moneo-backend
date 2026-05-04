package http

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const rawRequestBodyContextKey = "http.raw_request_body"
const maxCapturedRequestBodyBytes int64 = 1 << 20 // 1 MiB

func captureRawRequestBody() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil || c.Request.Body == nil || !shouldCaptureRawBody(c) {
			c.Next()
			return
		}
		if c.Request.ContentLength > maxCapturedRequestBodyBytes {
			c.Next()
			return
		}

		limited := io.LimitReader(c.Request.Body, maxCapturedRequestBodyBytes+1)
		body, err := io.ReadAll(limited)
		if err != nil {
			c.Request.Body = io.NopCloser(bytes.NewReader(nil))
			c.Next()
			return
		}
		if int64(len(body)) > maxCapturedRequestBodyBytes {
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
		c.Set(rawRequestBodyContextKey, body)
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Next()
	}
}

func shouldCaptureRawBody(c *gin.Context) bool {
	if c.Request == nil {
		return false
	}

	path := c.FullPath()
	method := c.Request.Method
	switch {
	case method == http.MethodPatch && path == "/api/v1/transactions/:transactionId":
		return true
	case method == http.MethodPatch && path == "/api/v1/transactions/bulk":
		return true
	case method == http.MethodPost && strings.HasSuffix(path, "/transactions/:transactionId/duplicate"):
		return true
	default:
		return false
	}
}
