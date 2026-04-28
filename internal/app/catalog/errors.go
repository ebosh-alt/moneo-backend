package catalog

import "errors"

var (
	ErrCategoryNotFound               = errors.New("category not found")
	ErrSubcategoryNotFound            = errors.New("subcategory not found")
	ErrParentCategoryArchived         = errors.New("parent category is archived")
	ErrDuplicateActiveCategoryName    = errors.New("duplicate active category name")
	ErrCategoryNameAlreadyExists      = errors.New("category name already exists")
	ErrConcurrentCategoryUpdate       = errors.New("concurrent category update")
	ErrDuplicateActiveSubcategoryName = errors.New("duplicate active subcategory name")
	ErrSubcategoryNameAlreadyExists   = errors.New("subcategory name already exists")
	ErrConcurrentSubcategoryUpdate    = errors.New("concurrent subcategory update")
)
