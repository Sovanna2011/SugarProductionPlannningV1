// Package interfaces declares the repository contracts the service layer
// depends on. Per §A2 rule 2 no service may import the concrete postgres
// package; per rule 3 nothing in here carries a business rule.
package interfaces

import "time"

// Operator is the set of filter operators exposed by the REST API (§E8).
type Operator string

const (
	OpEq      Operator = "eq"
	OpNe      Operator = "ne"
	OpGt      Operator = "gt"
	OpGe      Operator = "ge"
	OpLt      Operator = "lt"
	OpLe      Operator = "le"
	OpLike    Operator = "like"
	OpIn      Operator = "in"
	OpBetween Operator = "between"
)

// Filter is one parsed `filter[field]=op:value` expression. Field names are
// validated against the data dictionary before they reach SQL (Part G rule 3)
// and values are always bound, never interpolated (rule 4).
type Filter struct {
	Field    string
	Operator Operator
	Values   []any
}

type SortSpec struct {
	Field      string
	Descending bool
}

// ListOptions carries pagination, sorting and filtering.
//
// CompanyID is a pointer so that the compiler cannot silently default it to
// zero: a company-dependent repository rejects a nil company rather than
// returning every company's rows.
type ListOptions struct {
	CompanyID       *int64
	Page            int
	Size            int
	Sort            []SortSpec
	Filters         []Filter
	Search          string
	IncludeInactive bool
}

const (
	DefaultPageSize = 50
	MaxPageSize     = 500
)

// Normalise applies the documented pagination defaults and caps.
func (o *ListOptions) Normalise() {
	if o.Page < 1 {
		o.Page = 1
	}
	if o.Size < 1 {
		o.Size = DefaultPageSize
	}
	if o.Size > MaxPageSize {
		o.Size = MaxPageSize
	}
}

func (o ListOptions) Offset() int { return (o.Page - 1) * o.Size }

// Page is a slice of results plus the total row count for `meta.total`.
type Page[T any] struct {
	Rows  []T
	Total int64
}

// DateRange is the shared from/to filter of the reporting endpoints.
type DateRange struct {
	From time.Time
	To   time.Time
}
