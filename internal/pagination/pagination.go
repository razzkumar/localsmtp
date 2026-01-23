// Package pagination provides utilities for handling pagination parameters in web APIs.
// It supports extracting pagination parameters from URL query strings, validating them,
// and calculating offsets for database queries. The package includes configurable
// defaults and options for customizing pagination behavior.
package pagination

import (
	"net/url"
	"strconv"
)

// Params represents pagination parameters extracted from a request.
// It contains the page number, limit per page, calculated offset, and sort order.
type Params struct {
	Page   int32  // Current page number (1-based)
	Limit  int32  // Number of items per page
	Offset int32  // Calculated offset for database queries
	Sort   string // Sort order: "newest", "oldest", "asc", or "desc"
}

const (
	// MaxLimit is the maximum number of items allowed per page
	MaxLimit int32 = 100
	// DefaultPage is the default page number when not specified
	DefaultPage int32 = 1
	// DefaultLimit is the default number of items per page when not specified
	DefaultLimit int32 = 10
	// DefaultSort is the default sort order when not specified
	DefaultSort = "newest"
)

// calculateOffset computes the database offset for a given page and limit.
// It ensures page is at least 1 to avoid negative offsets.
func calculateOffset(page, limit int32) int32 {
	if page < 1 {
		page = 1
	}
	return (page - 1) * limit
}

// isValidSort checks if the provided sort string is valid.
// Valid sort options are: "newest", "oldest", "asc", "desc".
func isValidSort(sort string) bool {
	switch sort {
	case "newest", "oldest", "asc", "desc":
		return true
	default:
		return false
	}
}

// PaginationOption is a function type for configuring pagination parameters.
// It follows the functional options pattern for flexible configuration.
type PaginationOption func(*Params)

// WithDefaultLimit returns a PaginationOption that sets the default limit.
// The limit is only applied if it's greater than 0.
func WithDefaultLimit(limit int32) PaginationOption {
	return func(p *Params) {
		if limit > 0 {
			p.Limit = limit
		}
	}
}

// WithDefaultSort returns a PaginationOption that sets the default sort order.
// If the sort string is invalid, it returns a no-op option.
func WithDefaultSort(sort string) PaginationOption {
	if !isValidSort(sort) {
		return func(p *Params) {}
	}
	return func(p *Params) {
		p.Sort = sort
	}
}

// GetPaginationParams extracts pagination parameters from URL query values.
// It applies any provided options and validates the parameters, enforcing
// maximum limits and calculating the appropriate offset.
func GetPaginationParams(q url.Values, opts ...PaginationOption) *Params {
	params := &Params{
		Page:   DefaultPage,
		Limit:  DefaultLimit,
		Offset: calculateOffset(1, 10),
		Sort:   DefaultSort,
	}

	for _, opt := range opts {
		opt(params)
	}

	if pageStr := q.Get("page"); pageStr != "" {
		if val, err := strconv.ParseInt(pageStr, 10, 64); err == nil && val > 0 {
			params.Page = int32(val)
		}
	}

	if limitStr := q.Get("limit"); limitStr != "" {
		if val, err := strconv.ParseInt(limitStr, 10, 64); err == nil && val > 0 {
			params.Limit = int32(val)
		}
	}

	// enforce max limit
	if params.Limit > MaxLimit {
		params.Limit = MaxLimit
	}

	params.Offset = calculateOffset(params.Page, params.Limit)

	if sortStr := q.Get("sort"); sortStr != "" && isValidSort(sortStr) {
		params.Sort = sortStr
	}

	return params
}

// GetHasNext determines if there are more items available after the current page.
// It returns true when the offset plus limit is less than the total count.
func GetHasNext(offset, limit, count int32) bool {
	return (offset + limit) < count
}
