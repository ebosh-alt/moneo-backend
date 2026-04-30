package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"reflect"

	generated "moneo/internal/transport/http/generated"

	"github.com/gin-gonic/gin"
)

// APIHandler adapts generated strict server calls to existing transport handlers.
// It keeps business logic in app/domain and reuses existing HTTP mapping behavior.
type APIHandler struct {
	catalog *CatalogHandler
}

func NewAPIHandler(catalog *CatalogHandler) *APIHandler {
	return &APIHandler{catalog: catalog}
}

func (h *APIHandler) invokeCatalog(ctx context.Context, request any, handlerName string, decode func(status int, payload []byte) (any, error)) (any, error) {
	if h == nil || h.catalog == nil {
		return nil, fmt.Errorf("catalog handler is not configured")
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

	method := reflect.ValueOf(h.catalog).MethodByName(handlerName)
	if !method.IsValid() {
		return nil, fmt.Errorf("catalog handler method %s not found", handlerName)
	}
	method.Call([]reflect.Value{reflect.ValueOf(proxy)})

	return decode(recorder.Code, recorder.Body.Bytes())
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

	payload, err := json.Marshal(bodyField.Interface())
	if err != nil {
		return []byte("{}"), true
	}
	return payload, true
}

func (h *APIHandler) ListAccountsLegacy(ctx context.Context, request generated.ListAccountsLegacyRequestObject) (generated.ListAccountsLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.ListAccountsLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.ListAccountsLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.ListAccountsLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.ListAccountsLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "ListAccounts", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.ListAccountsLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for ListAccountsLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) CreateAccountLegacy(ctx context.Context, request generated.CreateAccountLegacyRequestObject) (generated.CreateAccountLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 201:
			var response generated.CreateAccountLegacy201JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.CreateAccountLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.CreateAccountLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.CreateAccountLegacy409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 422:
			var response generated.CreateAccountLegacy422JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.CreateAccountLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "CreateAccount", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.CreateAccountLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for CreateAccountLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) GetAccountsSummaryLegacy(ctx context.Context, request generated.GetAccountsSummaryLegacyRequestObject) (generated.GetAccountsSummaryLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.GetAccountsSummaryLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.GetAccountsSummaryLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.GetAccountsSummaryLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.GetAccountsSummaryLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "GetAccountsSummary", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.GetAccountsSummaryLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for GetAccountsSummaryLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) GetAccountLegacy(ctx context.Context, request generated.GetAccountLegacyRequestObject) (generated.GetAccountLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.GetAccountLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.GetAccountLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.GetAccountLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.GetAccountLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.GetAccountLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "GetAccount", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.GetAccountLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for GetAccountLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) PatchAccountLegacy(ctx context.Context, request generated.PatchAccountLegacyRequestObject) (generated.PatchAccountLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.PatchAccountLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.PatchAccountLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.PatchAccountLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.PatchAccountLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.PatchAccountLegacy409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.PatchAccountLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "PatchAccount", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.PatchAccountLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for PatchAccountLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) ArchiveAccountLegacy(ctx context.Context, request generated.ArchiveAccountLegacyRequestObject) (generated.ArchiveAccountLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.ArchiveAccountLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.ArchiveAccountLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.ArchiveAccountLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.ArchiveAccountLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "ArchiveAccount", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.ArchiveAccountLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for ArchiveAccountLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) RestoreAccountLegacy(ctx context.Context, request generated.RestoreAccountLegacyRequestObject) (generated.RestoreAccountLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.RestoreAccountLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.RestoreAccountLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.RestoreAccountLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.RestoreAccountLegacy409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.RestoreAccountLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "RestoreAccount", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.RestoreAccountLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for RestoreAccountLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) ListAccounts(ctx context.Context, request generated.ListAccountsRequestObject) (generated.ListAccountsResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.ListAccounts200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.ListAccounts400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.ListAccounts401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.ListAccounts500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "ListAccounts", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.ListAccountsResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for ListAccounts", result)
	}
	return typed, nil
}

func (h *APIHandler) CreateAccount(ctx context.Context, request generated.CreateAccountRequestObject) (generated.CreateAccountResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 201:
			var response generated.CreateAccount201JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.CreateAccount400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.CreateAccount401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.CreateAccount409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 422:
			var response generated.CreateAccount422JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.CreateAccount500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "CreateAccount", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.CreateAccountResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for CreateAccount", result)
	}
	return typed, nil
}

func (h *APIHandler) GetAccountsSummary(ctx context.Context, request generated.GetAccountsSummaryRequestObject) (generated.GetAccountsSummaryResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.GetAccountsSummary200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.GetAccountsSummary400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.GetAccountsSummary401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.GetAccountsSummary500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "GetAccountsSummary", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.GetAccountsSummaryResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for GetAccountsSummary", result)
	}
	return typed, nil
}

func (h *APIHandler) GetAccount(ctx context.Context, request generated.GetAccountRequestObject) (generated.GetAccountResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.GetAccount200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.GetAccount400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.GetAccount401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.GetAccount404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.GetAccount500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "GetAccount", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.GetAccountResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for GetAccount", result)
	}
	return typed, nil
}

func (h *APIHandler) PatchAccount(ctx context.Context, request generated.PatchAccountRequestObject) (generated.PatchAccountResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.PatchAccount200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.PatchAccount400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.PatchAccount401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.PatchAccount404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.PatchAccount409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.PatchAccount500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "PatchAccount", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.PatchAccountResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for PatchAccount", result)
	}
	return typed, nil
}

func (h *APIHandler) ArchiveAccount(ctx context.Context, request generated.ArchiveAccountRequestObject) (generated.ArchiveAccountResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.ArchiveAccount200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.ArchiveAccount401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.ArchiveAccount404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.ArchiveAccount500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "ArchiveAccount", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.ArchiveAccountResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for ArchiveAccount", result)
	}
	return typed, nil
}

func (h *APIHandler) RestoreAccount(ctx context.Context, request generated.RestoreAccountRequestObject) (generated.RestoreAccountResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.RestoreAccount200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.RestoreAccount401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.RestoreAccount404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.RestoreAccount409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.RestoreAccount500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "RestoreAccount", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.RestoreAccountResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for RestoreAccount", result)
	}
	return typed, nil
}

func (h *APIHandler) ListCategories(ctx context.Context, request generated.ListCategoriesRequestObject) (generated.ListCategoriesResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.ListCategories200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.ListCategories400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.ListCategories401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.ListCategories500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "ListCategories", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.ListCategoriesResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for ListCategories", result)
	}
	return typed, nil
}

func (h *APIHandler) CreateCategory(ctx context.Context, request generated.CreateCategoryRequestObject) (generated.CreateCategoryResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 201:
			var response generated.CreateCategory201JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.CreateCategory400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.CreateCategory401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.CreateCategory409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.CreateCategory500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "CreateCategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.CreateCategoryResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for CreateCategory", result)
	}
	return typed, nil
}

func (h *APIHandler) DeleteCategory(ctx context.Context, request generated.DeleteCategoryRequestObject) (generated.DeleteCategoryResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.DeleteCategory200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.DeleteCategory401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.DeleteCategory404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.DeleteCategory500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "DeleteCategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.DeleteCategoryResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for DeleteCategory", result)
	}
	return typed, nil
}

func (h *APIHandler) GetCategory(ctx context.Context, request generated.GetCategoryRequestObject) (generated.GetCategoryResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.GetCategory200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.GetCategory400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.GetCategory401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.GetCategory404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.GetCategory500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "GetCategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.GetCategoryResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for GetCategory", result)
	}
	return typed, nil
}

func (h *APIHandler) PatchCategory(ctx context.Context, request generated.PatchCategoryRequestObject) (generated.PatchCategoryResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.PatchCategory200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.PatchCategory400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.PatchCategory401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.PatchCategory404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.PatchCategory409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.PatchCategory500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "PatchCategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.PatchCategoryResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for PatchCategory", result)
	}
	return typed, nil
}

func (h *APIHandler) RestoreCategory(ctx context.Context, request generated.RestoreCategoryRequestObject) (generated.RestoreCategoryResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.RestoreCategory200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.RestoreCategory401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.RestoreCategory404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.RestoreCategory409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.RestoreCategory500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "RestoreCategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.RestoreCategoryResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for RestoreCategory", result)
	}
	return typed, nil
}

func (h *APIHandler) ListCategorySubcategories(ctx context.Context, request generated.ListCategorySubcategoriesRequestObject) (generated.ListCategorySubcategoriesResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.ListCategorySubcategories200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.ListCategorySubcategories400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.ListCategorySubcategories401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.ListCategorySubcategories404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.ListCategorySubcategories500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "ListCategorySubcategories", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.ListCategorySubcategoriesResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for ListCategorySubcategories", result)
	}
	return typed, nil
}

func (h *APIHandler) CreateSubcategory(ctx context.Context, request generated.CreateSubcategoryRequestObject) (generated.CreateSubcategoryResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 201:
			var response generated.CreateSubcategory201JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.CreateSubcategory400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.CreateSubcategory401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.CreateSubcategory404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.CreateSubcategory409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 422:
			var response generated.CreateSubcategory422JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.CreateSubcategory500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "CreateSubcategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.CreateSubcategoryResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for CreateSubcategory", result)
	}
	return typed, nil
}

func (h *APIHandler) ListSubcategories(ctx context.Context, request generated.ListSubcategoriesRequestObject) (generated.ListSubcategoriesResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.ListSubcategories200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.ListSubcategories400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.ListSubcategories401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.ListSubcategories500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "ListSubcategories", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.ListSubcategoriesResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for ListSubcategories", result)
	}
	return typed, nil
}

func (h *APIHandler) DeleteSubcategory(ctx context.Context, request generated.DeleteSubcategoryRequestObject) (generated.DeleteSubcategoryResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.DeleteSubcategory200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.DeleteSubcategory401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.DeleteSubcategory404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.DeleteSubcategory500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "DeleteSubcategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.DeleteSubcategoryResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for DeleteSubcategory", result)
	}
	return typed, nil
}

func (h *APIHandler) GetSubcategory(ctx context.Context, request generated.GetSubcategoryRequestObject) (generated.GetSubcategoryResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.GetSubcategory200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.GetSubcategory400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.GetSubcategory401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.GetSubcategory404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.GetSubcategory500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "GetSubcategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.GetSubcategoryResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for GetSubcategory", result)
	}
	return typed, nil
}

func (h *APIHandler) PatchSubcategory(ctx context.Context, request generated.PatchSubcategoryRequestObject) (generated.PatchSubcategoryResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.PatchSubcategory200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.PatchSubcategory400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.PatchSubcategory401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.PatchSubcategory404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.PatchSubcategory409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.PatchSubcategory500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "PatchSubcategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.PatchSubcategoryResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for PatchSubcategory", result)
	}
	return typed, nil
}

func (h *APIHandler) RestoreSubcategory(ctx context.Context, request generated.RestoreSubcategoryRequestObject) (generated.RestoreSubcategoryResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.RestoreSubcategory200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.RestoreSubcategory401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.RestoreSubcategory404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 422:
			var response generated.RestoreSubcategory422JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.RestoreSubcategory500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "RestoreSubcategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.RestoreSubcategoryResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for RestoreSubcategory", result)
	}
	return typed, nil
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

func (h *APIHandler) ListCategoriesLegacy(ctx context.Context, request generated.ListCategoriesLegacyRequestObject) (generated.ListCategoriesLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.ListCategoriesLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.ListCategoriesLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.ListCategoriesLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.ListCategoriesLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "ListCategories", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.ListCategoriesLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for ListCategoriesLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) CreateCategoryLegacy(ctx context.Context, request generated.CreateCategoryLegacyRequestObject) (generated.CreateCategoryLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 201:
			var response generated.CreateCategoryLegacy201JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.CreateCategoryLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.CreateCategoryLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.CreateCategoryLegacy409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.CreateCategoryLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "CreateCategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.CreateCategoryLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for CreateCategoryLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) DeleteCategoryLegacy(ctx context.Context, request generated.DeleteCategoryLegacyRequestObject) (generated.DeleteCategoryLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.DeleteCategoryLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.DeleteCategoryLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.DeleteCategoryLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.DeleteCategoryLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "DeleteCategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.DeleteCategoryLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for DeleteCategoryLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) GetCategoryLegacy(ctx context.Context, request generated.GetCategoryLegacyRequestObject) (generated.GetCategoryLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.GetCategoryLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.GetCategoryLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.GetCategoryLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.GetCategoryLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.GetCategoryLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "GetCategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.GetCategoryLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for GetCategoryLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) PatchCategoryLegacy(ctx context.Context, request generated.PatchCategoryLegacyRequestObject) (generated.PatchCategoryLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.PatchCategoryLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.PatchCategoryLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.PatchCategoryLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.PatchCategoryLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.PatchCategoryLegacy409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.PatchCategoryLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "PatchCategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.PatchCategoryLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for PatchCategoryLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) RestoreCategoryLegacy(ctx context.Context, request generated.RestoreCategoryLegacyRequestObject) (generated.RestoreCategoryLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.RestoreCategoryLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.RestoreCategoryLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.RestoreCategoryLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.RestoreCategoryLegacy409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.RestoreCategoryLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "RestoreCategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.RestoreCategoryLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for RestoreCategoryLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) ListCategorySubcategoriesLegacy(ctx context.Context, request generated.ListCategorySubcategoriesLegacyRequestObject) (generated.ListCategorySubcategoriesLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.ListCategorySubcategoriesLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.ListCategorySubcategoriesLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.ListCategorySubcategoriesLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.ListCategorySubcategoriesLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.ListCategorySubcategoriesLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "ListCategorySubcategories", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.ListCategorySubcategoriesLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for ListCategorySubcategoriesLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) CreateSubcategoryLegacy(ctx context.Context, request generated.CreateSubcategoryLegacyRequestObject) (generated.CreateSubcategoryLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 201:
			var response generated.CreateSubcategoryLegacy201JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.CreateSubcategoryLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.CreateSubcategoryLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.CreateSubcategoryLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.CreateSubcategoryLegacy409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 422:
			var response generated.CreateSubcategoryLegacy422JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.CreateSubcategoryLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "CreateSubcategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.CreateSubcategoryLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for CreateSubcategoryLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) ListSubcategoriesLegacy(ctx context.Context, request generated.ListSubcategoriesLegacyRequestObject) (generated.ListSubcategoriesLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.ListSubcategoriesLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.ListSubcategoriesLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.ListSubcategoriesLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.ListSubcategoriesLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "ListSubcategories", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.ListSubcategoriesLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for ListSubcategoriesLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) DeleteSubcategoryLegacy(ctx context.Context, request generated.DeleteSubcategoryLegacyRequestObject) (generated.DeleteSubcategoryLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.DeleteSubcategoryLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.DeleteSubcategoryLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.DeleteSubcategoryLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.DeleteSubcategoryLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "DeleteSubcategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.DeleteSubcategoryLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for DeleteSubcategoryLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) GetSubcategoryLegacy(ctx context.Context, request generated.GetSubcategoryLegacyRequestObject) (generated.GetSubcategoryLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.GetSubcategoryLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.GetSubcategoryLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.GetSubcategoryLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.GetSubcategoryLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.GetSubcategoryLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "GetSubcategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.GetSubcategoryLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for GetSubcategoryLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) PatchSubcategoryLegacy(ctx context.Context, request generated.PatchSubcategoryLegacyRequestObject) (generated.PatchSubcategoryLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.PatchSubcategoryLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.PatchSubcategoryLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.PatchSubcategoryLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.PatchSubcategoryLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.PatchSubcategoryLegacy409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.PatchSubcategoryLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "PatchSubcategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.PatchSubcategoryLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for PatchSubcategoryLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) RestoreSubcategoryLegacy(ctx context.Context, request generated.RestoreSubcategoryLegacyRequestObject) (generated.RestoreSubcategoryLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.RestoreSubcategoryLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.RestoreSubcategoryLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.RestoreSubcategoryLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 422:
			var response generated.RestoreSubcategoryLegacy422JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.RestoreSubcategoryLegacy500JSONResponse
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
	result, err := h.invokeCatalog(ctx, request, "RestoreSubcategory", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.RestoreSubcategoryLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for RestoreSubcategoryLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) ListTransactionsLegacy(ctx context.Context, request generated.ListTransactionsLegacyRequestObject) (generated.ListTransactionsLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.ListTransactionsLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.ListTransactionsLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.ListTransactionsLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.ListTransactionsLegacy500JSONResponse
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
	typed, ok := result.(generated.ListTransactionsLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for ListTransactionsLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) CreateTransactionLegacy(ctx context.Context, request generated.CreateTransactionLegacyRequestObject) (generated.CreateTransactionLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 201:
			var response generated.CreateTransactionLegacy201JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.CreateTransactionLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.CreateTransactionLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.CreateTransactionLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.CreateTransactionLegacy409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 422:
			var response generated.CreateTransactionLegacy422JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.CreateTransactionLegacy500JSONResponse
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
	typed, ok := result.(generated.CreateTransactionLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for CreateTransactionLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) PatchTransactionsBulkLegacy(ctx context.Context, request generated.PatchTransactionsBulkLegacyRequestObject) (generated.PatchTransactionsBulkLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.PatchTransactionsBulkLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.PatchTransactionsBulkLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.PatchTransactionsBulkLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.PatchTransactionsBulkLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.PatchTransactionsBulkLegacy409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 422:
			var response generated.PatchTransactionsBulkLegacy422JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.PatchTransactionsBulkLegacy500JSONResponse
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
	typed, ok := result.(generated.PatchTransactionsBulkLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for PatchTransactionsBulkLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) CreateTransactionsBulkLegacy(ctx context.Context, request generated.CreateTransactionsBulkLegacyRequestObject) (generated.CreateTransactionsBulkLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 201:
			var response generated.CreateTransactionsBulkLegacy201JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.CreateTransactionsBulkLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.CreateTransactionsBulkLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.CreateTransactionsBulkLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.CreateTransactionsBulkLegacy409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 422:
			var response generated.CreateTransactionsBulkLegacy422JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.CreateTransactionsBulkLegacy500JSONResponse
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
	typed, ok := result.(generated.CreateTransactionsBulkLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for CreateTransactionsBulkLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) DeleteTransactionLegacy(ctx context.Context, request generated.DeleteTransactionLegacyRequestObject) (generated.DeleteTransactionLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 204:
			return generated.DeleteTransactionLegacy204Response{}, nil
		case 400:
			var response generated.DeleteTransactionLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.DeleteTransactionLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.DeleteTransactionLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.DeleteTransactionLegacy409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.DeleteTransactionLegacy500JSONResponse
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
	typed, ok := result.(generated.DeleteTransactionLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for DeleteTransactionLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) GetTransactionLegacy(ctx context.Context, request generated.GetTransactionLegacyRequestObject) (generated.GetTransactionLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.GetTransactionLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.GetTransactionLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.GetTransactionLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.GetTransactionLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.GetTransactionLegacy500JSONResponse
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
	typed, ok := result.(generated.GetTransactionLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for GetTransactionLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) PatchTransactionLegacy(ctx context.Context, request generated.PatchTransactionLegacyRequestObject) (generated.PatchTransactionLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.PatchTransactionLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.PatchTransactionLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.PatchTransactionLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.PatchTransactionLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.PatchTransactionLegacy409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 422:
			var response generated.PatchTransactionLegacy422JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.PatchTransactionLegacy500JSONResponse
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
	typed, ok := result.(generated.PatchTransactionLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for PatchTransactionLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) CancelTransactionLegacy(ctx context.Context, request generated.CancelTransactionLegacyRequestObject) (generated.CancelTransactionLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.CancelTransactionLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.CancelTransactionLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.CancelTransactionLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.CancelTransactionLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.CancelTransactionLegacy409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.CancelTransactionLegacy500JSONResponse
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
	typed, ok := result.(generated.CancelTransactionLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for CancelTransactionLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) DuplicateTransactionLegacy(ctx context.Context, request generated.DuplicateTransactionLegacyRequestObject) (generated.DuplicateTransactionLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 201:
			var response generated.DuplicateTransactionLegacy201JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.DuplicateTransactionLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.DuplicateTransactionLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.DuplicateTransactionLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.DuplicateTransactionLegacy409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 422:
			var response generated.DuplicateTransactionLegacy422JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.DuplicateTransactionLegacy500JSONResponse
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
	typed, ok := result.(generated.DuplicateTransactionLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for DuplicateTransactionLegacy", result)
	}
	return typed, nil
}

func (h *APIHandler) PostTransactionLegacy(ctx context.Context, request generated.PostTransactionLegacyRequestObject) (generated.PostTransactionLegacyResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.PostTransactionLegacy200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.PostTransactionLegacy400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.PostTransactionLegacy401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.PostTransactionLegacy404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.PostTransactionLegacy409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.PostTransactionLegacy500JSONResponse
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
	typed, ok := result.(generated.PostTransactionLegacyResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for PostTransactionLegacy", result)
	}
	return typed, nil
}
