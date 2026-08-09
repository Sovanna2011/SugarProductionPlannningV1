package postgres

import (
	"errors"
	"regexp"
	"strings"

	"gorm.io/gorm"

	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
)

// raisedCode matches the "E-XXX-NNN: message" convention the database triggers
// use, so a rule enforced in SQL reaches the client with the same error code as
// the equivalent rule enforced in Go.
var raisedCode = regexp.MustCompile(`(E-[A-Z]+-\d+):\s*(.*)`)

// translate maps a driver or GORM error onto the application error model.
// Repositories return only AppErrors so that controllers never inspect
// database-specific error text (§A2 rule 1).
func translate(err error) error {
	if err == nil {
		return nil
	}
	if appErr, ok := apperrors.As(err); ok {
		return appErr
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.ErrNotFound
	}

	msg := err.Error()

	// A trigger-raised business rule keeps its declared code.
	if m := raisedCode.FindStringSubmatch(msg); m != nil {
		if mapped := byCode(m[1]); mapped != nil {
			return mapped.WithCause(err)
		}
		return apperrors.ErrValidation.Msgf("%s", strings.TrimSpace(m[2])).WithCause(err)
	}

	switch {
	case strings.Contains(msg, "SQLSTATE 23505"): // unique_violation
		return apperrors.ErrDuplicate.WithDetails(
			apperrors.Detail{Field: constraintName(msg), Message: "must be unique"}).WithCause(err)
	case strings.Contains(msg, "SQLSTATE 23503"): // foreign_key_violation
		return apperrors.ErrValidation.
			Msgf("Referenced record does not exist").
			WithDetails(apperrors.Detail{Field: constraintName(msg)}).WithCause(err)
	case strings.Contains(msg, "SQLSTATE 23514"): // check_violation
		return apperrors.ErrValidation.
			Msgf("Value violates a database constraint").
			WithDetails(apperrors.Detail{Field: constraintName(msg)}).WithCause(err)
	case strings.Contains(msg, "SQLSTATE 23P01"): // exclusion_violation
		return apperrors.ErrSeasonOverlap.WithCause(err)
	case strings.Contains(msg, "SQLSTATE 40001"), // serialization_failure
		strings.Contains(msg, "SQLSTATE 40P01"): // deadlock_detected
		return apperrors.ErrConflict.
			Msgf("Concurrent update detected, please retry").WithCause(err)
	case strings.Contains(msg, "SQLSTATE 57014"): // query_canceled (statement timeout)
		return apperrors.ErrValidation.
			Msgf("The query exceeded the configured time limit — narrow the selection").WithCause(err)
	}

	return apperrors.ErrInternal.WithCause(err)
}

// byCode resolves a trigger-raised code to the catalogued error so that the
// HTTP status and message stay identical to the Go-side check.
func byCode(code string) *apperrors.AppError {
	switch code {
	case "E-PLAN-007":
		return apperrors.ErrVersionNotEditable
	case "E-VAL-010":
		return apperrors.ErrCrossCompanyReference
	case "E-INV-011":
		return apperrors.ErrNegativeStock
	case "E-INV-012":
		return apperrors.ErrCapacityExceeded
	default:
		return nil
	}
}

var constraintPattern = regexp.MustCompile(`constraint "([^"]+)"`)

func constraintName(msg string) string {
	if m := constraintPattern.FindStringSubmatch(msg); m != nil {
		return m[1]
	}
	return ""
}
