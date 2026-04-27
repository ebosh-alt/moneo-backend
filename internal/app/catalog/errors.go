package catalog

import "errors"

var (
	ErrCategoryNotFound            = errors.New("category not found")
	ErrSubcategoryNotFound         = errors.New("subcategory not found")
	ErrDuplicateActiveCategoryName = errors.New("duplicate active category name")
	ErrCategoryNameAlreadyExists   = errors.New("category name already exists")
)
