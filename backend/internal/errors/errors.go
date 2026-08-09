// Package errors defines the single application error type (§D1) and the
// catalogue of business error codes. Controllers translate an AppError into
// the documented HTTP status; nothing else in the stack decides status codes.
package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// Detail carries the field-level context of a validation failure.
type Detail struct {
	Field   string `json:"field"`
	Value   string `json:"value,omitempty"`
	Message string `json:"message,omitempty"`
}

// AppError is the only error type crossing a layer boundary.
type AppError struct {
	Code       string   `json:"code"`
	Message    string   `json:"message"`
	Details    []Detail `json:"details,omitempty"`
	HTTPStatus int      `json:"-"`
	Cause      error    `json:"-"`
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error { return e.Cause }

// WithDetails returns a copy carrying field-level detail.
func (e *AppError) WithDetails(details ...Detail) *AppError {
	clone := *e
	clone.Details = append(append([]Detail{}, e.Details...), details...)
	return &clone
}

// WithCause attaches the underlying technical error. The cause is logged but
// never serialised to the client.
func (e *AppError) WithCause(cause error) *AppError {
	clone := *e
	clone.Cause = cause
	return &clone
}

// Msgf overrides the default message, keeping the code and status.
func (e *AppError) Msgf(format string, args ...any) *AppError {
	clone := *e
	clone.Message = fmt.Sprintf(format, args...)
	return &clone
}

func define(code, message string, status int) *AppError {
	return &AppError{Code: code, Message: message, HTTPStatus: status}
}

// --- generic -------------------------------------------------------------

var (
	ErrBadRequest   = define("E-GEN-400", "The request could not be processed", http.StatusBadRequest)
	ErrUnauthorized = define("E-AUTH-001", "Authentication required", http.StatusUnauthorized)
	ErrNotFound     = define("E-GEN-404", "Resource not found", http.StatusNotFound)
	ErrConflict     = define("E-GEN-409", "The request conflicts with the current state", http.StatusConflict)
	ErrStaleRecord  = define("E-GEN-409", "Record was changed by another user, please reload", http.StatusConflict)
	ErrDuplicate    = define("E-GEN-410", "A record with the same business key already exists", http.StatusConflict)
	ErrValidation   = define("E-VAL-001", "Validation failed", http.StatusUnprocessableEntity)
	ErrInternal     = define("E-GEN-500", "Unexpected error", http.StatusInternalServerError)
)

// --- authentication / authorization (§C2) --------------------------------

var (
	ErrInvalidCredentials   = define("E-AUTH-002", "Invalid user name or password", http.StatusUnauthorized)
	ErrCompanyNotAuthorized = define("E-AUTH-003", "User is not authorised for this company", http.StatusForbidden)
	ErrPermissionDenied     = define("E-AUTH-004", "Permission denied", http.StatusForbidden)
	ErrTokenExpired         = define("E-AUTH-005", "Token has expired", http.StatusUnauthorized)
	ErrAccountLocked        = define("E-AUTH-006", "Account is locked", http.StatusUnauthorized)
	ErrCompanyRequired      = define("E-AUTH-007", "companyId is required for this operation", http.StatusBadRequest)
	ErrPasswordPolicy       = define("E-AUTH-008", "Password does not meet the password policy", http.StatusUnprocessableEntity)
)

// --- validation ----------------------------------------------------------

var (
	// §C2: a document may never reference master data of another company.
	ErrCrossCompanyReference = define("E-VAL-010", "Cross-company reference is not allowed", http.StatusUnprocessableEntity)
	ErrInvalidDateRange      = define("E-VAL-011", "dateTo must not be before dateFrom", http.StatusUnprocessableEntity)
	ErrUOMMismatch           = define("E-VAL-012", "Unit of measure cannot be converted to the material base unit", http.StatusUnprocessableEntity)
	ErrSeasonOverlap         = define("E-VAL-013", "Seasons of one company must not overlap", http.StatusUnprocessableEntity)
	ErrDateOutsideSeason     = define("E-VAL-014", "Date is outside the season period", http.StatusUnprocessableEntity)
)

// --- planning (§F3) ------------------------------------------------------

var (
	ErrVersionNotEditable = define("E-PLAN-007", "Planning version is not in an editable status", http.StatusConflict)
	ErrVersionExists      = define("E-PLAN-008", "A planning version with this number already exists for the season", http.StatusConflict)
	ErrInvalidTransition  = define("E-PLAN-009", "Status transition is not allowed", http.StatusConflict)
	ErrFourEyesViolation  = define("E-PLAN-010", "The approver must be different from the creator", http.StatusForbidden)
	ErrNoApprovedVersion  = define("E-PLAN-011", "No approved planning version exists for this season", http.StatusNotFound)
)

// --- actual --------------------------------------------------------------

var (
	ErrNotPostable      = define("E-ACT-001", "Only a DRAFT document can be posted", http.StatusConflict)
	ErrNotReversible    = define("E-ACT-002", "Only a POSTED document can be reversed", http.StatusConflict)
	ErrDocumentEmpty    = define("E-ACT-003", "A document must contain at least one item", http.StatusUnprocessableEntity)
	ErrDocumentNotDraft = define("E-ACT-004", "Only a DRAFT document can be changed", http.StatusConflict)
)

// --- inventory (§F4, §F5) ------------------------------------------------

var (
	ErrNegativeStock         = define("E-INV-011", "Resulting stock would be negative", http.StatusUnprocessableEntity)
	ErrCapacityExceeded      = define("E-INV-012", "Warehouse capacity would be exceeded", http.StatusUnprocessableEntity)
	ErrSameWarehouse         = define("E-INV-013", "Source and target warehouse must be different", http.StatusUnprocessableEntity)
	ErrMaterialNotInvEnabled = define("E-INV-014", "Material is not inventory-enabled for this company", http.StatusUnprocessableEntity)
	ErrMaterialNotAllowed    = define("E-INV-015", "Material is not allowed in this storage location", http.StatusUnprocessableEntity)
	ErrAlreadyReversed       = define("E-INV-016", "Movement has already been reversed", http.StatusConflict)
)

// --- production routing (§F7) --------------------------------------------

var (
	ErrMaterialNotValidForProcess = define("E-PROD-020", "Material is not a declared input or output of this process", http.StatusUnprocessableEntity)
	ErrMaterialNotSiloManaged     = define("E-PROD-021", "Material does not require conditioning and must not be received into a silo", http.StatusUnprocessableEntity)
	ErrConditioningStepMissing    = define("E-PROD-022", "Conditioned material must reach the finished-goods warehouse via the silo", http.StatusUnprocessableEntity)
)

// --- data browser (Part G) -----------------------------------------------

var (
	ErrTableNotBrowsable = define("E-DD-001", "Table is not available in the data browser", http.StatusForbidden)
	ErrUnknownField      = define("E-DD-002", "Unknown field for this table", http.StatusUnprocessableEntity)
	ErrUnknownOperator   = define("E-DD-003", "Unsupported filter operator", http.StatusUnprocessableEntity)
)

// As extracts an *AppError from an error chain.
func As(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

// Wrap turns any error into an AppError, keeping an existing one untouched.
func Wrap(err error) *AppError {
	if err == nil {
		return nil
	}
	if appErr, ok := As(err); ok {
		return appErr
	}
	return ErrInternal.WithCause(err)
}
