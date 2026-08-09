// Package postgres holds the concrete GORM repositories. Per §A2 rule 3 these
// contain no business rules: no authorization decisions, no calculations —
// only persistence, company scoping and optimistic locking.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/database"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/security"
)

// entity is the constraint every managed aggregate satisfies through the
// embedded model.Base.
type entity[T any] interface {
	*T
	GetID() int64
	GetVersion() int
	SetVersion(int)
	SetActive(bool)
	SetAuditUser(userID int64, isCreate bool)
}

// base implements the persistence mechanics shared by every repository.
//
// companyColumn is empty for cross-company master data. When it is set, every
// read and write carries a company predicate — the "defence in depth" barrier
// behind the authorization middleware (§C2 step 6).
type base[T any, PT entity[T]] struct {
	db            *gorm.DB
	companyColumn string
	// sortable and searchable whitelist the columns a client may name, so a
	// query string can never reach SQL unchecked.
	sortable   map[string]bool
	searchable []string
}

func (b *base[T, PT]) conn(ctx context.Context) *gorm.DB {
	return database.Conn(ctx, b.db)
}

// scope applies the company predicate. It refuses to run unscoped against a
// company-dependent table.
func (b *base[T, PT]) scope(q *gorm.DB, companyID *int64) (*gorm.DB, error) {
	if b.companyColumn == "" {
		return q, nil
	}
	if companyID == nil || *companyID == 0 {
		return nil, apperrors.ErrCompanyRequired
	}
	return q.Where(b.companyColumn+" = ?", *companyID), nil
}

func (b *base[T, PT]) stampAudit(ctx context.Context, e PT, isCreate bool) {
	if userID, ok := security.UserIDFrom(ctx); ok {
		e.SetAuditUser(userID, isCreate)
	}
}

func (b *base[T, PT]) Create(ctx context.Context, e *T) error {
	pt := PT(e)
	b.stampAudit(ctx, pt, true)
	pt.SetVersion(1)

	if err := b.conn(ctx).Create(e).Error; err != nil {
		return translate(err)
	}
	return nil
}

// Update performs the optimistic-locked write of §D2:
//
//	UPDATE ... SET version = version + 1 WHERE id = ? AND version = ?
//
// Zero rows affected means another user changed the record first.
func (b *base[T, PT]) update(ctx context.Context, companyID *int64, e *T) error {
	pt := PT(e)
	b.stampAudit(ctx, pt, false)

	expected := pt.GetVersion()
	q := b.conn(ctx).Model(e).Where("version = ?", expected)
	q, err := b.scope(q, companyID)
	if err != nil {
		return err
	}

	// The new version is written as part of the same statement, while the
	// WHERE clause above still carries the version the caller read.
	pt.SetVersion(expected + 1)

	// Select("*") makes GORM write zero values too (e.g. clearing a flag);
	// Omit protects the immutable creation audit columns.
	res := q.Select("*").Omit("id", "created_by", "created_at").Updates(e)
	if res.Error != nil {
		pt.SetVersion(expected)
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		pt.SetVersion(expected)
		return b.explainMissedUpdate(ctx, companyID, pt.GetID())
	}
	return nil
}

// explainMissedUpdate distinguishes "someone else changed it" (409) from
// "it does not exist in your company" (404). Without this the client cannot
// tell a stale form from a wrong id.
func (b *base[T, PT]) explainMissedUpdate(ctx context.Context, companyID *int64, id int64) error {
	var count int64
	q := b.conn(ctx).Model(new(T)).Where("id = ?", id)
	q, err := b.scope(q, companyID)
	if err != nil {
		return err
	}
	if err := q.Count(&count).Error; err != nil {
		return translate(err)
	}
	if count == 0 {
		return apperrors.ErrNotFound
	}
	return apperrors.ErrStaleRecord
}

func (b *base[T, PT]) findByID(ctx context.Context, companyID *int64, id int64) (*T, error) {
	var out T
	q := b.conn(ctx).Where("id = ?", id)
	q, err := b.scope(q, companyID)
	if err != nil {
		return nil, err
	}
	if err := q.First(&out).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrNotFound
		}
		return nil, translate(err)
	}
	return &out, nil
}

func (b *base[T, PT]) list(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[T], error) {
	opts.Normalise()
	var page interfaces.Page[T]

	q := b.conn(ctx).Model(new(T))
	q, err := b.scope(q, opts.CompanyID)
	if err != nil {
		return page, err
	}
	if !opts.IncludeInactive {
		q = q.Where("is_active = ?", true)
	}
	if q, err = b.applyFilters(q, opts.Filters); err != nil {
		return page, err
	}
	if opts.Search != "" && len(b.searchable) > 0 {
		clauses := make([]string, 0, len(b.searchable))
		args := make([]any, 0, len(b.searchable))
		for _, col := range b.searchable {
			clauses = append(clauses, col+" ILIKE ?")
			args = append(args, "%"+opts.Search+"%")
		}
		q = q.Where("("+strings.Join(clauses, " OR ")+")", args...)
	}

	if err := q.Count(&page.Total).Error; err != nil {
		return page, translate(err)
	}

	q, err = b.applySort(q, opts.Sort)
	if err != nil {
		return page, err
	}

	if err := q.Limit(opts.Size).Offset(opts.Offset()).Find(&page.Rows).Error; err != nil {
		return page, translate(err)
	}
	return page, nil
}

func (b *base[T, PT]) applyFilters(q *gorm.DB, filters []interfaces.Filter) (*gorm.DB, error) {
	for _, f := range filters {
		if !b.sortable[f.Field] {
			return nil, apperrors.ErrUnknownField.WithDetails(
				apperrors.Detail{Field: "filter", Value: f.Field})
		}
		clause, args, err := buildPredicate(f)
		if err != nil {
			return nil, err
		}
		q = q.Where(clause, args...)
	}
	return q, nil
}

func (b *base[T, PT]) applySort(q *gorm.DB, sorts []interfaces.SortSpec) (*gorm.DB, error) {
	if len(sorts) == 0 {
		return q.Order("id"), nil
	}
	for _, s := range sorts {
		if !b.sortable[s.Field] {
			return nil, apperrors.ErrUnknownField.WithDetails(
				apperrors.Detail{Field: "sort", Value: s.Field})
		}
		direction := " ASC"
		if s.Descending {
			direction = " DESC"
		}
		// s.Field is whitelisted above, so this concatenation cannot carry
		// caller-controlled text into SQL.
		q = q.Order(s.Field + direction)
	}
	return q, nil
}

// deactivate is the logical delete of §B1 — master data is never removed.
func (b *base[T, PT]) deactivate(ctx context.Context, companyID *int64, id int64, version int) error {
	q := b.conn(ctx).Model(new(T)).Where("id = ? AND version = ?", id, version)
	q, err := b.scope(q, companyID)
	if err != nil {
		return err
	}

	updates := map[string]any{"is_active": false, "version": gorm.Expr("version + 1")}
	if userID, ok := security.UserIDFrom(ctx); ok {
		updates["changed_by"] = userID
	}
	updates["changed_at"] = gorm.Expr("now()")

	res := q.Updates(updates)
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return b.explainMissedUpdate(ctx, companyID, id)
	}
	return nil
}

// buildPredicate turns one filter into a parameterised clause. Every value is
// bound — the field name has already been whitelisted by the caller.
func buildPredicate(f interfaces.Filter) (string, []any, error) {
	if len(f.Values) == 0 {
		return "", nil, apperrors.ErrValidation.Msgf("filter on %q has no value", f.Field)
	}
	switch f.Operator {
	case interfaces.OpEq:
		return f.Field + " = ?", f.Values[:1], nil
	case interfaces.OpNe:
		return f.Field + " <> ?", f.Values[:1], nil
	case interfaces.OpGt:
		return f.Field + " > ?", f.Values[:1], nil
	case interfaces.OpGe:
		return f.Field + " >= ?", f.Values[:1], nil
	case interfaces.OpLt:
		return f.Field + " < ?", f.Values[:1], nil
	case interfaces.OpLe:
		return f.Field + " <= ?", f.Values[:1], nil
	case interfaces.OpLike:
		return f.Field + " ILIKE ?", []any{fmt.Sprintf("%%%v%%", f.Values[0])}, nil
	case interfaces.OpIn:
		return f.Field + " IN ?", []any{f.Values}, nil
	case interfaces.OpBetween:
		if len(f.Values) != 2 {
			return "", nil, apperrors.ErrValidation.Msgf("filter %q with operator between needs two values", f.Field)
		}
		return f.Field + " BETWEEN ? AND ?", f.Values, nil
	default:
		return "", nil, apperrors.ErrUnknownOperator.WithDetails(
			apperrors.Detail{Field: "operator", Value: string(f.Operator)})
	}
}
