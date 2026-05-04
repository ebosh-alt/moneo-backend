package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"reflect"
	"strings"

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

func (h *APIHandler) RegisterAuth(ctx context.Context, request generated.RegisterAuthRequestObject) (generated.RegisterAuthResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 201:
			var response generated.RegisterAuth201JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.RegisterAuth400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 409:
			var response generated.RegisterAuth409JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.RegisterAuth500JSONResponse
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
	result, err := h.invokeAuth(ctx, request, "Register", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.RegisterAuthResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for RegisterAuth", result)
	}
	return typed, nil
}

func (h *APIHandler) LoginAuth(ctx context.Context, request generated.LoginAuthRequestObject) (generated.LoginAuthResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.LoginAuth200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.LoginAuth400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.LoginAuth401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.LoginAuth500JSONResponse
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
	result, err := h.invokeAuth(ctx, request, "Login", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.LoginAuthResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for LoginAuth", result)
	}
	return typed, nil
}

func (h *APIHandler) RefreshAuth(ctx context.Context, request generated.RefreshAuthRequestObject) (generated.RefreshAuthResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.RefreshAuth200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.RefreshAuth400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.RefreshAuth401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.RefreshAuth500JSONResponse
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
	result, err := h.invokeAuth(ctx, request, "Refresh", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.RefreshAuthResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for RefreshAuth", result)
	}
	return typed, nil
}

func (h *APIHandler) LogoutAuth(ctx context.Context, request generated.LogoutAuthRequestObject) (generated.LogoutAuthResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.LogoutAuth200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.LogoutAuth400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.LogoutAuth500JSONResponse
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
	result, err := h.invokeAuth(ctx, request, "Logout", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.LogoutAuthResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for LogoutAuth", result)
	}
	return typed, nil
}

func (h *APIHandler) LogoutAllAuth(ctx context.Context, request generated.LogoutAllAuthRequestObject) (generated.LogoutAllAuthResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.LogoutAllAuth200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.LogoutAllAuth401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.LogoutAllAuth500JSONResponse
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
	result, err := h.invokeAuth(ctx, request, "LogoutAll", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.LogoutAllAuthResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for LogoutAllAuth", result)
	}
	return typed, nil
}

func (h *APIHandler) MeAuth(ctx context.Context, request generated.MeAuthRequestObject) (generated.MeAuthResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.MeAuth200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.MeAuth401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.MeAuth500JSONResponse
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
	result, err := h.invokeAuth(ctx, request, "Me", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.MeAuthResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for MeAuth", result)
	}
	return typed, nil
}

func (h *APIHandler) SessionsAuth(ctx context.Context, request generated.SessionsAuthRequestObject) (generated.SessionsAuthResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.SessionsAuth200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.SessionsAuth401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.SessionsAuth500JSONResponse
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
	result, err := h.invokeAuth(ctx, request, "Sessions", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.SessionsAuthResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for SessionsAuth", result)
	}
	return typed, nil
}

func (h *APIHandler) RevokeSessionAuth(ctx context.Context, request generated.RevokeSessionAuthRequestObject) (generated.RevokeSessionAuthResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 204:
			return generated.RevokeSessionAuth204Response{}, nil
		case 400:
			var response generated.RevokeSessionAuth400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.RevokeSessionAuth401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 404:
			var response generated.RevokeSessionAuth404JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.RevokeSessionAuth500JSONResponse
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
	result, err := h.invokeAuth(ctx, request, "RevokeSession", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.RevokeSessionAuthResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for RevokeSessionAuth", result)
	}
	return typed, nil
}

func (h *APIHandler) ForgotPasswordAuth(ctx context.Context, request generated.ForgotPasswordAuthRequestObject) (generated.ForgotPasswordAuthResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.ForgotPasswordAuth200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.ForgotPasswordAuth400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.ForgotPasswordAuth500JSONResponse
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
	result, err := h.invokeAuth(ctx, request, "ForgotPassword", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.ForgotPasswordAuthResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for ForgotPasswordAuth", result)
	}
	return typed, nil
}

func (h *APIHandler) ResetPasswordAuth(ctx context.Context, request generated.ResetPasswordAuthRequestObject) (generated.ResetPasswordAuthResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.ResetPasswordAuth200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.ResetPasswordAuth400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.ResetPasswordAuth401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.ResetPasswordAuth500JSONResponse
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
	result, err := h.invokeAuth(ctx, request, "ResetPassword", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.ResetPasswordAuthResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for ResetPasswordAuth", result)
	}
	return typed, nil
}

func (h *APIHandler) SendVerificationEmailAuth(ctx context.Context, request generated.SendVerificationEmailAuthRequestObject) (generated.SendVerificationEmailAuthResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.SendVerificationEmailAuth200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.SendVerificationEmailAuth401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.SendVerificationEmailAuth500JSONResponse
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
	result, err := h.invokeAuth(ctx, request, "SendVerificationEmail", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.SendVerificationEmailAuthResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for SendVerificationEmailAuth", result)
	}
	return typed, nil
}

func (h *APIHandler) VerifyEmailAuth(ctx context.Context, request generated.VerifyEmailAuthRequestObject) (generated.VerifyEmailAuthResponseObject, error) {
	decode := func(status int, payload []byte) (any, error) {
		switch status {
		case 200:
			var response generated.VerifyEmailAuth200JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 400:
			var response generated.VerifyEmailAuth400JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 401:
			var response generated.VerifyEmailAuth401JSONResponse
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &response); err != nil {
					return nil, err
				}
			}
			return response, nil
		case 500:
			var response generated.VerifyEmailAuth500JSONResponse
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
	result, err := h.invokeAuth(ctx, request, "VerifyEmail", decode)
	if err != nil {
		return nil, err
	}
	typed, ok := result.(generated.VerifyEmailAuthResponseObject)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T for VerifyEmailAuth", result)
	}
	return typed, nil
}
