package postgres

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/database"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/security"
)

type actualRepository struct {
	db *gorm.DB
}

func NewActualRepository(db *gorm.DB) interfaces.ActualRepository {
	return &actualRepository{db: db}
}

func (r *actualRepository) CreateHeader(ctx context.Context, header *model.ActualHeader) error {
	if userID, ok := security.UserIDFrom(ctx); ok {
		header.SetAuditUser(userID, true)
	}
	header.IsActive = true
	header.Version = 1
	return translate(database.Conn(ctx, r.db).Create(header).Error)
}

func (r *actualRepository) UpdateHeader(ctx context.Context, companyID int64, header *model.ActualHeader) error {
	if userID, ok := security.UserIDFrom(ctx); ok {
		header.SetAuditUser(userID, false)
	}
	expected := header.Version
	header.Version = expected + 1

	res := database.Conn(ctx, r.db).Model(header).
		Where("company_id = ? AND version = ?", companyID, expected).
		Select("*").Omit("id", "created_by", "created_at").Updates(header)
	if res.Error != nil {
		header.Version = expected
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		header.Version = expected
		return apperrors.ErrStaleRecord
	}
	return nil
}

// SetStatus is the posting / reversal transition. It is a distinct method from
// UpdateHeader so the status machine cannot be moved by an ordinary field edit.
func (r *actualRepository) SetStatus(ctx context.Context, header *model.ActualHeader) error {
	return r.UpdateHeader(ctx, header.CompanyID, header)
}

func (r *actualRepository) FindHeader(ctx context.Context, companyID, headerID int64) (*model.ActualHeader, error) {
	return findOne[model.ActualHeader](ctx, database.Conn(ctx, r.db),
		"id = ? AND company_id = ?", headerID, companyID)
}

// FindByIdempotencyKey supports the Idempotency-Key header of §E8: a retried
// POST returns the document created by the first attempt instead of posting
// twice.
func (r *actualRepository) FindByIdempotencyKey(ctx context.Context, companyID int64, key string) (*model.ActualHeader, error) {
	var out model.ActualHeader
	err := database.Conn(ctx, r.db).
		Where("company_id = ? AND idempotency_key = ?", companyID, key).
		First(&out).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, translate(err)
	}
	return &out, nil
}

func (r *actualRepository) ListHeaders(ctx context.Context, opts interfaces.ListOptions,
	from, to *time.Time, movementTypeID, lineID *int64) (interfaces.Page[model.ActualHeader], error) {

	opts.Normalise()
	var page interfaces.Page[model.ActualHeader]

	if opts.CompanyID == nil {
		return page, apperrors.ErrCompanyRequired
	}
	q := database.Conn(ctx, r.db).Model(&model.ActualHeader{}).
		Where("company_id = ?", *opts.CompanyID)
	if !opts.IncludeInactive {
		q = q.Where("is_active")
	}
	if from != nil {
		q = q.Where("posting_date >= ?", *from)
	}
	if to != nil {
		q = q.Where("posting_date <= ?", *to)
	}
	if movementTypeID != nil {
		q = q.Where("movement_type_id = ?", *movementTypeID)
	}
	if lineID != nil {
		q = q.Where("production_line_id = ?", *lineID)
	}

	if err := q.Count(&page.Total).Error; err != nil {
		return page, translate(err)
	}
	err := q.Order("posting_date DESC, document_no DESC").
		Limit(opts.Size).Offset(opts.Offset()).Find(&page.Rows).Error
	return page, translate(err)
}

func (r *actualRepository) Items(ctx context.Context, headerID int64) ([]model.ActualItem, error) {
	var rows []model.ActualItem
	err := database.Conn(ctx, r.db).
		Where("actual_header_id = ? AND is_active", headerID).
		Order("actual_date, id").Find(&rows).Error
	return rows, translate(err)
}

// ReplaceItems overwrites the item set of a draft document. Unlike plan items
// there is no partial matrix merge here: an actual document is entered and
// saved as a whole.
func (r *actualRepository) ReplaceItems(ctx context.Context, headerID int64, items []model.ActualItem) error {
	conn := database.Conn(ctx, r.db)
	if err := conn.Where("actual_header_id = ?", headerID).
		Delete(&model.ActualItem{}).Error; err != nil {
		return translate(err)
	}
	if len(items) == 0 {
		return nil
	}

	var userID *int64
	if id, ok := security.UserIDFrom(ctx); ok {
		userID = &id
	}
	for i := range items {
		items[i].ID = 0
		items[i].ActualHeaderID = headerID
		items[i].IsActive = true
		items[i].Version = 1
		items[i].CreatedBy = userID
		items[i].ChangedBy = userID
	}
	return translate(conn.Create(&items).Error)
}
