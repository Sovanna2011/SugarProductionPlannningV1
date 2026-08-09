// Package controller holds the HTTP handlers. Per §A2 rule 1 nothing here
// imports gorm or touches persistence, and per rule 4 handlers speak DTO only.
package controller

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/dto"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/middleware"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
)

// OK writes a 200 with the success envelope of §D1.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, dto.Envelope{Success: true, Data: data})
}

// Created writes a 201.
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, dto.Envelope{Success: true, Data: data})
}

// NoContent writes a 204 for an operation with nothing to return.
func NoContent(c *gin.Context) { c.Status(http.StatusNoContent) }

// Paged writes a list response with the meta block the API contract requires.
func Paged[T any](c *gin.Context, rows []T, opts interfaces.ListOptions, total int64) {
	c.JSON(http.StatusOK, dto.Envelope{
		Success: true,
		Data:    rows,
		Meta:    &dto.Meta{Page: opts.Page, Size: opts.Size, Total: total},
	})
}

// Fail maps any error to its documented status through the error catalogue.
func Fail(c *gin.Context, err error) { middleware.Abort(c, err) }

// Bind parses and validates a JSON body, turning a binding failure into the
// standard 400 rather than gin's default shape.
func Bind[T any](c *gin.Context) (T, bool) {
	var payload T
	if err := c.ShouldBindJSON(&payload); err != nil {
		// A DTO's own UnmarshalJSON may already have produced an AppError
		// (a malformed date, for instance) — keep its message.
		if appErr, ok := apperrors.As(err); ok {
			Fail(c, appErr)
			return payload, false
		}
		Fail(c, apperrors.ErrBadRequest.
			Msgf("the request body is not valid").
			WithDetails(apperrors.Detail{Field: "body", Message: err.Error()}))
		return payload, false
	}
	return payload, true
}

// PathID reads a positive integer path parameter.
func PathID(c *gin.Context, name string) (int64, bool) {
	raw := c.Param(name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		Fail(c, apperrors.ErrBadRequest.
			Msgf("%s must be a positive integer", name).
			WithDetails(apperrors.Detail{Field: name, Value: raw}))
		return 0, false
	}
	return id, true
}

// QueryID reads an optional integer query parameter.
func QueryID(c *gin.Context, name string) (*int64, bool) {
	raw := c.Query(name)
	if raw == "" {
		return nil, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		Fail(c, apperrors.ErrBadRequest.
			Msgf("%s must be a positive integer", name).
			WithDetails(apperrors.Detail{Field: name, Value: raw}))
		return nil, false
	}
	return &id, true
}

// QueryDate reads an optional YYYY-MM-DD query parameter.
func QueryDate(c *gin.Context, name string) (*time.Time, bool) {
	raw := c.Query(name)
	if raw == "" {
		return nil, true
	}
	parsed, err := dto.ParseDate(raw)
	if err != nil {
		Fail(c, err)
		return nil, false
	}
	return &parsed, true
}

// RequiredQueryDate reads a mandatory YYYY-MM-DD query parameter.
func RequiredQueryDate(c *gin.Context, name string) (time.Time, bool) {
	raw := c.Query(name)
	if raw == "" {
		Fail(c, apperrors.ErrBadRequest.Msgf("%s is required", name).
			WithDetails(apperrors.Detail{Field: name}))
		return time.Time{}, false
	}
	parsed, err := dto.ParseDate(raw)
	if err != nil {
		Fail(c, err)
		return time.Time{}, false
	}
	return parsed, true
}

// ListOptions parses the pagination, sorting and filtering conventions of §E8:
//
//	?page=1&size=50&sort=field,-other&filter[field]=op:value
//
// Field names are not validated here — the repository checks them against its
// own whitelist, which is what keeps a query string from reaching SQL.
func ListOptions(c *gin.Context, companyID *int64) (interfaces.ListOptions, bool) {
	opts := interfaces.ListOptions{CompanyID: companyID}

	if raw := c.Query("page"); raw != "" {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 {
			Fail(c, apperrors.ErrBadRequest.Msgf("page must be a positive integer"))
			return opts, false
		}
		opts.Page = page
	}
	if raw := c.Query("size"); raw != "" {
		size, err := strconv.Atoi(raw)
		if err != nil || size < 1 {
			Fail(c, apperrors.ErrBadRequest.Msgf("size must be a positive integer"))
			return opts, false
		}
		opts.Size = size
	}

	opts.Search = strings.TrimSpace(c.Query("search"))
	opts.IncludeInactive = c.Query("includeInactive") == "true"
	opts.Sort = parseSort(c.Query("sort"))

	filters, err := parseFilters(c)
	if err != nil {
		Fail(c, err)
		return opts, false
	}
	opts.Filters = filters

	opts.Normalise()
	return opts, true
}

func parseSort(raw string) []interfaces.SortSpec {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	specs := make([]interfaces.SortSpec, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "-") {
			specs = append(specs, interfaces.SortSpec{Field: part[1:], Descending: true})
			continue
		}
		specs = append(specs, interfaces.SortSpec{Field: part})
	}
	return specs
}

var supportedOperators = map[string]interfaces.Operator{
	"eq": interfaces.OpEq, "ne": interfaces.OpNe,
	"gt": interfaces.OpGt, "ge": interfaces.OpGe,
	"lt": interfaces.OpLt, "le": interfaces.OpLe,
	"like": interfaces.OpLike, "in": interfaces.OpIn,
	"between": interfaces.OpBetween,
}

// parseFilters turns `filter[field]=op:value` into bound filter expressions.
// An `in` or `between` value is comma-separated.
func parseFilters(c *gin.Context) ([]interfaces.Filter, error) {
	var filters []interfaces.Filter

	for key, values := range c.Request.URL.Query() {
		if !strings.HasPrefix(key, "filter[") || !strings.HasSuffix(key, "]") {
			continue
		}
		field := key[len("filter[") : len(key)-1]
		if field == "" {
			continue
		}
		for _, raw := range values {
			operatorName, value, found := strings.Cut(raw, ":")
			if !found {
				operatorName, value = "eq", raw
			}
			operator, ok := supportedOperators[strings.ToLower(operatorName)]
			if !ok {
				return nil, apperrors.ErrUnknownOperator.WithDetails(
					apperrors.Detail{Field: field, Value: operatorName})
			}

			filter := interfaces.Filter{Field: field, Operator: operator}
			switch operator {
			case interfaces.OpIn, interfaces.OpBetween:
				for _, part := range strings.Split(value, ",") {
					filter.Values = append(filter.Values, strings.TrimSpace(part))
				}
			default:
				filter.Values = []any{value}
			}
			filters = append(filters, filter)
		}
	}
	return filters, nil
}

// CompanyID and UserID re-export the middleware accessors so handlers do not
// reach into the middleware package for context keys.
func CompanyID(c *gin.Context) int64 { return middleware.CompanyID(c) }
func UserID(c *gin.Context) int64    { return middleware.UserID(c) }

// ParseIDList parses `?companyIds=1000,2000,3000` for the consolidated report.
func ParseIDList(raw string) ([]int64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 {
			return nil, apperrors.ErrBadRequest.
				Msgf("companyIds must be a comma-separated list of positive integers").
				WithDetails(apperrors.Detail{Field: "companyIds", Value: part})
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// versionRequired is the error for a delete that omits the optimistic-lock
// version, which the API takes as a query parameter (§D2).
func versionRequired() error {
	return apperrors.ErrBadRequest.
		Msgf("the current record version must be supplied as ?version=").
		WithDetails(apperrors.Detail{Field: "version"})
}

// missingMatrixSelection is returned when the matrix read is missing one of
// the three coordinates that identify a grid.
func missingMatrixSelection() error {
	return apperrors.ErrBadRequest.
		Msgf("versionId, movementTypeId and materialId are required to read a planning matrix").
		WithDetails(apperrors.Detail{Field: "versionId"})
}
