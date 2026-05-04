package http

import (
	"context"
	"fmt"

	generated "moneo/internal/transport/http/generated"
)

type AccountsStrictHandler interface {
	ListAccounts(ctx context.Context, request generated.ListAccountsRequestObject) (generated.ListAccountsResponseObject, error)
	CreateAccount(ctx context.Context, request generated.CreateAccountRequestObject) (generated.CreateAccountResponseObject, error)
	GetAccountsSummary(ctx context.Context, request generated.GetAccountsSummaryRequestObject) (generated.GetAccountsSummaryResponseObject, error)
	GetAccount(ctx context.Context, request generated.GetAccountRequestObject) (generated.GetAccountResponseObject, error)
	PatchAccount(ctx context.Context, request generated.PatchAccountRequestObject) (generated.PatchAccountResponseObject, error)
	ArchiveAccount(ctx context.Context, request generated.ArchiveAccountRequestObject) (generated.ArchiveAccountResponseObject, error)
	RestoreAccount(ctx context.Context, request generated.RestoreAccountRequestObject) (generated.RestoreAccountResponseObject, error)
}

type AuthStrictHandler interface {
	ForgotPasswordAuth(ctx context.Context, request generated.ForgotPasswordAuthRequestObject) (generated.ForgotPasswordAuthResponseObject, error)
	LoginAuth(ctx context.Context, request generated.LoginAuthRequestObject) (generated.LoginAuthResponseObject, error)
	LogoutAuth(ctx context.Context, request generated.LogoutAuthRequestObject) (generated.LogoutAuthResponseObject, error)
	LogoutAllAuth(ctx context.Context, request generated.LogoutAllAuthRequestObject) (generated.LogoutAllAuthResponseObject, error)
	MeAuth(ctx context.Context, request generated.MeAuthRequestObject) (generated.MeAuthResponseObject, error)
	RefreshAuth(ctx context.Context, request generated.RefreshAuthRequestObject) (generated.RefreshAuthResponseObject, error)
	RegisterAuth(ctx context.Context, request generated.RegisterAuthRequestObject) (generated.RegisterAuthResponseObject, error)
	ResetPasswordAuth(ctx context.Context, request generated.ResetPasswordAuthRequestObject) (generated.ResetPasswordAuthResponseObject, error)
	SendVerificationEmailAuth(ctx context.Context, request generated.SendVerificationEmailAuthRequestObject) (generated.SendVerificationEmailAuthResponseObject, error)
	SessionsAuth(ctx context.Context, request generated.SessionsAuthRequestObject) (generated.SessionsAuthResponseObject, error)
	RevokeSessionAuth(ctx context.Context, request generated.RevokeSessionAuthRequestObject) (generated.RevokeSessionAuthResponseObject, error)
	VerifyEmailAuth(ctx context.Context, request generated.VerifyEmailAuthRequestObject) (generated.VerifyEmailAuthResponseObject, error)
}

type CategoriesStrictHandler interface {
	ListCategories(ctx context.Context, request generated.ListCategoriesRequestObject) (generated.ListCategoriesResponseObject, error)
	CreateCategory(ctx context.Context, request generated.CreateCategoryRequestObject) (generated.CreateCategoryResponseObject, error)
	DeleteCategory(ctx context.Context, request generated.DeleteCategoryRequestObject) (generated.DeleteCategoryResponseObject, error)
	GetCategory(ctx context.Context, request generated.GetCategoryRequestObject) (generated.GetCategoryResponseObject, error)
	PatchCategory(ctx context.Context, request generated.PatchCategoryRequestObject) (generated.PatchCategoryResponseObject, error)
	RestoreCategory(ctx context.Context, request generated.RestoreCategoryRequestObject) (generated.RestoreCategoryResponseObject, error)
}

type SubcategoriesStrictHandler interface {
	ListCategorySubcategories(ctx context.Context, request generated.ListCategorySubcategoriesRequestObject) (generated.ListCategorySubcategoriesResponseObject, error)
	CreateSubcategory(ctx context.Context, request generated.CreateSubcategoryRequestObject) (generated.CreateSubcategoryResponseObject, error)
	ListSubcategories(ctx context.Context, request generated.ListSubcategoriesRequestObject) (generated.ListSubcategoriesResponseObject, error)
	DeleteSubcategory(ctx context.Context, request generated.DeleteSubcategoryRequestObject) (generated.DeleteSubcategoryResponseObject, error)
	GetSubcategory(ctx context.Context, request generated.GetSubcategoryRequestObject) (generated.GetSubcategoryResponseObject, error)
	PatchSubcategory(ctx context.Context, request generated.PatchSubcategoryRequestObject) (generated.PatchSubcategoryResponseObject, error)
	RestoreSubcategory(ctx context.Context, request generated.RestoreSubcategoryRequestObject) (generated.RestoreSubcategoryResponseObject, error)
}

type TransactionsStrictHandler interface {
	ListTransactions(ctx context.Context, request generated.ListTransactionsRequestObject) (generated.ListTransactionsResponseObject, error)
	CreateTransaction(ctx context.Context, request generated.CreateTransactionRequestObject) (generated.CreateTransactionResponseObject, error)
	PatchTransactionsBulk(ctx context.Context, request generated.PatchTransactionsBulkRequestObject) (generated.PatchTransactionsBulkResponseObject, error)
	CreateTransactionsBulk(ctx context.Context, request generated.CreateTransactionsBulkRequestObject) (generated.CreateTransactionsBulkResponseObject, error)
	DeleteTransaction(ctx context.Context, request generated.DeleteTransactionRequestObject) (generated.DeleteTransactionResponseObject, error)
	GetTransaction(ctx context.Context, request generated.GetTransactionRequestObject) (generated.GetTransactionResponseObject, error)
	PatchTransaction(ctx context.Context, request generated.PatchTransactionRequestObject) (generated.PatchTransactionResponseObject, error)
	CancelTransaction(ctx context.Context, request generated.CancelTransactionRequestObject) (generated.CancelTransactionResponseObject, error)
	DuplicateTransaction(ctx context.Context, request generated.DuplicateTransactionRequestObject) (generated.DuplicateTransactionResponseObject, error)
	PostTransaction(ctx context.Context, request generated.PostTransactionRequestObject) (generated.PostTransactionResponseObject, error)
}

type StrictAPIHandler struct {
	AccountsStrictHandler
	AuthStrictHandler
	CategoriesStrictHandler
	SubcategoriesStrictHandler
	TransactionsStrictHandler
}

type StrictAPIHandlerDeps struct {
	Accounts      AccountsStrictHandler
	Auth          AuthStrictHandler
	Categories    CategoriesStrictHandler
	Subcategories SubcategoriesStrictHandler
	Transactions  TransactionsStrictHandler
}

func NewStrictAPIHandler(deps StrictAPIHandlerDeps) *StrictAPIHandler {
	return &StrictAPIHandler{
		AccountsStrictHandler:      requireStrictGroup("accounts", deps.Accounts),
		AuthStrictHandler:          requireStrictGroup("auth", deps.Auth),
		CategoriesStrictHandler:    requireStrictGroup("categories", deps.Categories),
		SubcategoriesStrictHandler: requireStrictGroup("subcategories", deps.Subcategories),
		TransactionsStrictHandler:  requireStrictGroup("transactions", deps.Transactions),
	}
}

func requireStrictGroup[T any](name string, handler T) T {
	if any(handler) == nil {
		panic(fmt.Errorf("%s strict handler is required", name))
	}
	return handler
}

var _ generated.StrictServerInterface = (*StrictAPIHandler)(nil)
