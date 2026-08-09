package postgres

import (
	"context"
	"errors"

	"gorm.io/gorm"

	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
)

// crossRepo adapts base to interfaces.CrossCompanyRepository — master data
// shared by every company.
type crossRepo[T any, PT entity[T]] struct {
	base[T, PT]
}

func (r *crossRepo[T, PT]) Update(ctx context.Context, e *T) error {
	return r.update(ctx, nil, e)
}

func (r *crossRepo[T, PT]) FindByID(ctx context.Context, id int64) (*T, error) {
	return r.findByID(ctx, nil, id)
}

func (r *crossRepo[T, PT]) List(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[T], error) {
	return r.list(ctx, opts)
}

func (r *crossRepo[T, PT]) Deactivate(ctx context.Context, id int64, version int) error {
	return r.deactivate(ctx, nil, id, version)
}

// companyRepo adapts base to interfaces.CompanyRepository. Every method takes
// the company explicitly so the predicate cannot be forgotten.
type companyRepo[T any, PT entity[T]] struct {
	base[T, PT]
}

func (r *companyRepo[T, PT]) Update(ctx context.Context, companyID int64, e *T) error {
	return r.update(ctx, &companyID, e)
}

func (r *companyRepo[T, PT]) FindByID(ctx context.Context, companyID, id int64) (*T, error) {
	return r.findByID(ctx, &companyID, id)
}

func (r *companyRepo[T, PT]) List(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[T], error) {
	return r.list(ctx, opts)
}

func (r *companyRepo[T, PT]) Deactivate(ctx context.Context, companyID, id int64, version int) error {
	return r.deactivate(ctx, &companyID, id, version)
}

// findOne is the shared "look up by business key" helper. conditions are
// literal SQL written in this package — never caller-supplied text.
func findOne[T any](ctx context.Context, db *gorm.DB, query string, args ...any) (*T, error) {
	var out T
	if err := db.Where(query, args...).First(&out).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrNotFound
		}
		return nil, translate(err)
	}
	return &out, nil
}
