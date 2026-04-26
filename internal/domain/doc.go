// Package domain contains the write-side business model for Moneo.
//
// Bounded contexts must not import each other directly. Cross-context
// coordination belongs in internal/app, and cross-context references use IDs or
// value objects from internal/domain/shared.
package domain
