// Package dto holds the request and response shapes of the REST API. Per §A2
// rule 4 these are the only types a controller speaks — GORM models never
// cross the controller boundary.
package dto

import (
	"time"

	"github.com/shopspring/decimal"

	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
)

// Envelope is the success shape of §D1.
type Envelope struct {
	Success bool  `json:"success"`
	Data    any   `json:"data,omitempty"`
	Meta    *Meta `json:"meta,omitempty"`
}

type Meta struct {
	Page  int   `json:"page"`
	Size  int   `json:"size"`
	Total int64 `json:"total"`
}

// ErrorEnvelope is the failure shape of §D1.
type ErrorEnvelope struct {
	Success   bool         `json:"success"`
	Error     ErrorPayload `json:"error"`
	RequestID string       `json:"requestId,omitempty"`
}

type ErrorPayload struct {
	Code    string             `json:"code"`
	Message string             `json:"message"`
	Details []apperrors.Detail `json:"details,omitempty"`
}

// AuditFields are the server-maintained columns echoed back to the client.
// They are response-only: a request DTO never carries them, which is how §D4
// keeps the client from supplying its own timestamps or user ids.
type AuditFields struct {
	IsActive  bool      `json:"isActive"`
	Version   int       `json:"version"`
	CreatedBy *int64    `json:"createdBy,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	ChangedBy *int64    `json:"changedBy,omitempty"`
	ChangedAt time.Time `json:"changedAt"`
}

// VersionedRequest is embedded by every update DTO so the optimistic lock of
// §D2 is part of the contract rather than an afterthought.
type VersionedRequest struct {
	Version int `json:"version" binding:"required,min=1"`
}

// Date is a business date (no time, no timezone) serialised as YYYY-MM-DD.
// §B1 keeps these apart from timestamps so a date can never drift across a
// timezone boundary.
type Date struct {
	time.Time
}

const dateLayout = "2006-01-02"

func (d Date) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.Format(dateLayout) + `"`), nil
}

func (d *Date) UnmarshalJSON(data []byte) error {
	raw := string(data)
	if raw == "null" || raw == `""` {
		return nil
	}
	if len(raw) >= 2 && raw[0] == '"' {
		raw = raw[1 : len(raw)-1]
	}
	parsed, err := time.Parse(dateLayout, raw)
	if err != nil {
		return apperrors.ErrBadRequest.Msgf("expected a date as YYYY-MM-DD, got %q", raw)
	}
	d.Time = parsed
	return nil
}

func NewDate(t time.Time) Date { return Date{Time: t} }

// ParseDate converts a query-string date.
func ParseDate(value string) (time.Time, error) {
	parsed, err := time.Parse(dateLayout, value)
	if err != nil {
		return time.Time{}, apperrors.ErrBadRequest.
			Msgf("expected a date as YYYY-MM-DD, got %q", value)
	}
	return parsed, nil
}

// Quantity is serialised as a JSON string so that a NUMERIC(18,3) never loses
// precision by passing through a float64 in the browser.
type Quantity = decimal.Decimal
