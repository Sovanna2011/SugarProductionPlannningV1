// Package audit writes the change history required by §7. CRUD changes are
// captured by a GORM callback; business events (approve, post, reverse, login,
// permission denial) are written explicitly by the services that perform them.
package audit

import (
	"context"
	"encoding/json"

	"github.com/rs/zerolog"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/security"
)

type Service struct {
	repo interfaces.AuditRepository
	log  zerolog.Logger
}

func NewService(repo interfaces.AuditRepository, log zerolog.Logger) *Service {
	return &Service{repo: repo, log: log.With().Str("component", "audit").Logger()}
}

// Event describes one auditable business event.
type Event struct {
	TableName string
	RecordID  *int64
	CompanyID *int64
	Action    string
	OldValues any
	NewValues any
}

// Record writes an audit row. A failure here is logged but never propagated:
// losing the audit trail must not roll back a completed business transaction,
// and the log line is what alerts operations that the trail has a gap.
func (s *Service) Record(ctx context.Context, event Event) {
	entry := &model.AuditLog{
		TableName_: event.TableName,
		RecordID:   event.RecordID,
		CompanyID:  event.CompanyID,
		Action:     event.Action,
		OldValues:  marshal(event.OldValues),
		NewValues:  marshal(event.NewValues),
	}

	if userID, ok := security.UserIDFrom(ctx); ok {
		entry.ChangedBy = &userID
	}
	if requestID := security.RequestIDFrom(ctx); requestID != "" {
		entry.RequestID = &requestID
	}
	if ip := security.IPAddressFrom(ctx); ip != "" {
		entry.IPAddress = &ip
	}

	if err := s.repo.Write(ctx, entry); err != nil {
		s.log.Error().Err(err).
			Str("table", event.TableName).
			Str("action", event.Action).
			Msg("failed to write audit record")
	}
}

// List backs GET of the audit log for administrators.
func (s *Service) List(ctx context.Context, opts interfaces.ListOptions,
	tableName string, recordID *int64) (interfaces.Page[model.AuditLog], error) {
	return s.repo.List(ctx, opts, tableName, recordID)
}

func marshal(v any) []byte {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return raw
}

// Int64Ptr is a small helper for building events from plain ids.
func Int64Ptr(v int64) *int64 { return &v }
