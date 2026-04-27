package catalog

import "errors"

var (
	ErrCategoryNotFound    = errors.New("category not found")
	ErrSubcategoryNotFound = errors.New("subcategory not found")
)
