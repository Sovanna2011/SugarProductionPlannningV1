package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/database"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/security"
)

// --- planning versions ---------------------------------------------------

type planningVersionRepository struct {
	companyRepo[model.PlanningVersion, *model.PlanningVersion]
}

func NewPlanningVersionRepository(db *gorm.DB) interfaces.PlanningVersionRepository {
	r := &planningVersionRepository{}
	r.db = db
	r.companyColumn = "company_id"
	r.sortable = columns("version_no", "version_name", "status", "created_at", "approved_at", "season_id")
	r.searchable = []string{"version_name", "description"}
	return r
}

func (r *planningVersionRepository) FindBySeason(ctx context.Context, companyID, seasonID int64) ([]model.PlanningVersion, error) {
	var rows []model.PlanningVersion
	err := r.conn(ctx).
		Where("company_id = ? AND season_id = ? AND is_active", companyID, seasonID).
		Order("version_no").Find(&rows).Error
	return rows, translate(err)
}

func (r *planningVersionRepository) ExistsVersionNo(ctx context.Context, companyID, seasonID int64, versionNo int) (bool, error) {
	var count int64
	err := r.conn(ctx).Model(&model.PlanningVersion{}).
		Where("company_id = ? AND season_id = ? AND version_no = ?", companyID, seasonID, versionNo).
		Count(&count).Error
	return count > 0, translate(err)
}

// NextVersionNo keeps version numbering open-ended (§29) — nothing caps it
// at three.
func (r *planningVersionRepository) NextVersionNo(ctx context.Context, companyID, seasonID int64) (int, error) {
	var next *int
	err := r.conn(ctx).Model(&model.PlanningVersion{}).
		Select("MAX(version_no) + 1").
		Where("company_id = ? AND season_id = ?", companyID, seasonID).
		Scan(&next).Error
	if err != nil {
		return 0, translate(err)
	}
	if next == nil {
		return 1, nil
	}
	return *next, nil
}

// LatestApproved resolves the "latest approved version" of §F6. LOCKED counts
// as approved — it is an approved version that was frozen.
func (r *planningVersionRepository) LatestApproved(ctx context.Context, companyID, seasonID int64) (*model.PlanningVersion, error) {
	var out model.PlanningVersion
	err := r.conn(ctx).
		Where("company_id = ? AND season_id = ? AND status IN ?",
			companyID, seasonID, []string{model.VersionStatusApproved, model.VersionStatusLocked}).
		Order("version_no DESC").First(&out).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrNoApprovedVersion
		}
		return nil, translate(err)
	}
	return &out, nil
}

// UpdateStatus writes the status transition together with its audit stamps
// under the same optimistic lock as any other change.
func (r *planningVersionRepository) UpdateStatus(ctx context.Context, version *model.PlanningVersion) error {
	return r.update(ctx, &version.CompanyID, version)
}

// DeepCopy duplicates every header and item of a version in two statements
// (§F2). A CTE maps old header ids to new ones through the freshly allocated
// document numbers, so items can be inserted in a single set-based statement
// instead of one round-trip per row.
//
// The result is an independent snapshot: no row is shared with the source and
// the only link back is the informational copied_from_version_id on the
// version itself.
func (r *planningVersionRepository) DeepCopy(ctx context.Context,
	sourceVersionID, targetVersionID int64, docNos map[int64]string) error {

	if len(docNos) == 0 {
		return nil // source version has no plan documents — nothing to copy
	}

	var userID *int64
	if id, ok := security.UserIDFrom(ctx); ok {
		userID = &id
	}

	rows := make([]string, 0, len(docNos))
	args := make([]any, 0, len(docNos)*2+6)
	for oldID, docNo := range docNos {
		rows = append(rows, "(?::bigint, ?::varchar)")
		args = append(args, oldID, docNo)
	}
	// Placeholders below appear in this order: the VALUES pairs, the source
	// version, the target version, the header audit users, the item audit users.
	args = append(args, sourceVersionID, targetVersionID, userID, userID, userID, userID)

	sql := fmt.Sprintf(`
WITH src AS (
    SELECT ph.*, d.new_doc
      FROM plan_headers ph
      JOIN (VALUES %s) AS d(old_id, new_doc) ON d.old_id = ph.id
     WHERE ph.planning_version_id = ?
),
ins AS (
    INSERT INTO plan_headers
        (company_id, season_id, planning_version_id, movement_type_id, production_line_id,
         document_no, description, date_from, date_to, is_active, version, created_by, changed_by)
    SELECT src.company_id, src.season_id, ?, src.movement_type_id, src.production_line_id,
           src.new_doc, src.description, src.date_from, src.date_to, TRUE, 1, ?, ?
      FROM src
    RETURNING id, document_no
),
header_map AS (
    SELECT src.id AS old_id, ins.id AS new_id
      FROM src JOIN ins ON ins.document_no = src.new_doc
)
INSERT INTO plan_items
    (plan_header_id, plan_date, production_line_id, material_id, process_id, warehouse_id,
     packaging_type_id, quantity, uom_id, remark, is_active, version, created_by, changed_by)
SELECT hm.new_id, pi.plan_date, pi.production_line_id, pi.material_id, pi.process_id,
       pi.warehouse_id, pi.packaging_type_id, pi.quantity, pi.uom_id, pi.remark,
       TRUE, 1, ?::bigint, ?::bigint
  FROM plan_items pi
  JOIN header_map hm ON hm.old_id = pi.plan_header_id
 WHERE pi.is_active`, strings.Join(rows, ", "))

	return translate(database.Conn(ctx, r.db).Exec(sql, args...).Error)
}

// --- plan documents ------------------------------------------------------

type planRepository struct {
	db *gorm.DB
}

func NewPlanRepository(db *gorm.DB) interfaces.PlanRepository {
	return &planRepository{db: db}
}

func (r *planRepository) CreateHeader(ctx context.Context, header *model.PlanHeader) error {
	if userID, ok := security.UserIDFrom(ctx); ok {
		header.SetAuditUser(userID, true)
	}
	header.IsActive = true
	header.Version = 1
	return translate(database.Conn(ctx, r.db).Create(header).Error)
}

func (r *planRepository) UpdateHeader(ctx context.Context, companyID int64, header *model.PlanHeader) error {
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

func (r *planRepository) FindHeader(ctx context.Context, companyID, headerID int64) (*model.PlanHeader, error) {
	return findOne[model.PlanHeader](ctx, database.Conn(ctx, r.db),
		"id = ? AND company_id = ?", headerID, companyID)
}

// FindHeaderByKey resolves the document a matrix save belongs to. NULL and a
// concrete line are distinguished explicitly because a header for "all lines"
// is a different document from a header for line 1.
func (r *planRepository) FindHeaderByKey(ctx context.Context, companyID, versionID,
	movementTypeID int64, lineID *int64) (*model.PlanHeader, error) {

	q := database.Conn(ctx, r.db).
		Where("company_id = ? AND planning_version_id = ? AND movement_type_id = ?",
			companyID, versionID, movementTypeID)
	if lineID == nil {
		q = q.Where("production_line_id IS NULL")
	} else {
		q = q.Where("production_line_id = ?", *lineID)
	}

	var out model.PlanHeader
	if err := q.First(&out).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, translate(err)
	}
	return &out, nil
}

func (r *planRepository) ListHeaders(ctx context.Context, opts interfaces.ListOptions,
	versionID, movementTypeID *int64) (interfaces.Page[model.PlanHeader], error) {

	opts.Normalise()
	var page interfaces.Page[model.PlanHeader]

	if opts.CompanyID == nil {
		return page, apperrors.ErrCompanyRequired
	}
	q := database.Conn(ctx, r.db).Model(&model.PlanHeader{}).
		Where("company_id = ?", *opts.CompanyID)
	if !opts.IncludeInactive {
		q = q.Where("is_active")
	}
	if versionID != nil {
		q = q.Where("planning_version_id = ?", *versionID)
	}
	if movementTypeID != nil {
		q = q.Where("movement_type_id = ?", *movementTypeID)
	}

	if err := q.Count(&page.Total).Error; err != nil {
		return page, translate(err)
	}
	err := q.Order("document_no").Limit(opts.Size).Offset(opts.Offset()).
		Find(&page.Rows).Error
	return page, translate(err)
}

func (r *planRepository) Items(ctx context.Context, headerID int64) ([]model.PlanItem, error) {
	var rows []model.PlanItem
	err := database.Conn(ctx, r.db).
		Where("plan_header_id = ? AND is_active", headerID).
		Order("plan_date, production_line_id, material_id").
		Find(&rows).Error
	return rows, translate(err)
}

// UpsertItems is the idempotent bulk save behind the matrix. It is keyed on
// (header, date, line, material, process) — the business key of §28 — so the
// same payload can be replayed without creating duplicates.
func (r *planRepository) UpsertItems(ctx context.Context, headerID int64, items []model.PlanItem) error {
	if len(items) == 0 {
		return nil
	}

	var userID *int64
	if id, ok := security.UserIDFrom(ctx); ok {
		userID = &id
	}

	placeholders := make([]string, 0, len(items))
	args := make([]any, 0, len(items)*13)
	for _, it := range items {
		placeholders = append(placeholders,
			"(?::bigint, ?::date, ?::bigint, ?::bigint, ?::bigint, ?::bigint, ?::bigint, ?::numeric, ?::bigint, ?::varchar, TRUE, 1, ?::bigint, ?::bigint)")
		args = append(args, headerID, it.PlanDate, it.ProductionLineID, it.MaterialID,
			it.ProcessID, it.WarehouseID, it.PackagingTypeID, it.Quantity, it.UOMID,
			it.Remark, userID, userID)
	}

	sql := fmt.Sprintf(`
INSERT INTO plan_items
    (plan_header_id, plan_date, production_line_id, material_id, process_id, warehouse_id,
     packaging_type_id, quantity, uom_id, remark, is_active, version, created_by, changed_by)
VALUES %s
ON CONFLICT (plan_header_id, plan_date, production_line_id, material_id, process_id)
DO UPDATE SET quantity          = EXCLUDED.quantity,
              uom_id            = EXCLUDED.uom_id,
              warehouse_id      = EXCLUDED.warehouse_id,
              packaging_type_id = EXCLUDED.packaging_type_id,
              remark            = EXCLUDED.remark,
              is_active         = TRUE,
              version           = plan_items.version + 1,
              changed_by        = EXCLUDED.changed_by,
              changed_at        = now()`, strings.Join(placeholders, ",\n       "))

	return translate(database.Conn(ctx, r.db).Exec(sql, args...).Error)
}

// DeleteItems removes cells the user cleared. Plan items are working data of
// an unapproved version, so a physical delete is correct here — the audit
// trail of the version itself is what carries the history.
func (r *planRepository) DeleteItems(ctx context.Context, headerID int64, itemIDs []int64) error {
	if len(itemIDs) == 0 {
		return nil
	}
	return translate(database.Conn(ctx, r.db).
		Where("plan_header_id = ? AND id IN ?", headerID, itemIDs).
		Delete(&model.PlanItem{}).Error)
}

// MatrixCells reads the Date × Line grid in one query.
func (r *planRepository) MatrixCells(ctx context.Context, companyID, versionID,
	movementTypeID, materialID int64, processID *int64,
	from, to time.Time) ([]interfaces.MatrixCell, error) {

	q := database.Conn(ctx, r.db).
		Table("plan_items pi").
		Select(`pi.plan_header_id, pi.id AS plan_item_id, pi.plan_date,
		        pi.production_line_id, pi.quantity, pi.uom_id, pi.version`).
		Joins("JOIN plan_headers ph ON ph.id = pi.plan_header_id").
		Where(`ph.company_id = ? AND ph.planning_version_id = ? AND ph.movement_type_id = ?
		       AND pi.material_id = ? AND pi.plan_date BETWEEN ? AND ? AND pi.is_active`,
			companyID, versionID, movementTypeID, materialID, from, to)
	if processID == nil {
		q = q.Where("pi.process_id IS NULL")
	} else {
		q = q.Where("pi.process_id = ?", *processID)
	}

	var cells []interfaces.MatrixCell
	err := q.Order("pi.plan_date, pi.production_line_id").Scan(&cells).Error
	return cells, translate(err)
}

// MatrixSeries discovers which grids a version holds in the window, so the
// planning screen can render every product it plans rather than asking the
// user to pick one at a time.
func (r *planRepository) MatrixSeries(ctx context.Context, companyID, versionID int64,
	from, to time.Time) ([]interfaces.MatrixSeriesKey, error) {

	var series []interfaces.MatrixSeriesKey
	err := database.Conn(ctx, r.db).
		Table("plan_items pi").
		Select(`DISTINCT ph.movement_type_id, mt.movement_code, pi.material_id,
		        m.material_code, m.material_name, pi.process_id, pr.process_code,
		        pi.uom_id, u.uom_code`).
		Joins("JOIN plan_headers ph ON ph.id = pi.plan_header_id").
		Joins("JOIN movement_types mt ON mt.id = ph.movement_type_id").
		Joins("JOIN materials m ON m.id = pi.material_id").
		Joins("LEFT JOIN processes pr ON pr.id = pi.process_id").
		Joins("JOIN uoms u ON u.id = pi.uom_id").
		Where(`ph.company_id = ? AND ph.planning_version_id = ?
		       AND pi.plan_date BETWEEN ? AND ? AND pi.is_active`,
			companyID, versionID, from, to).
		Order("m.material_code, pr.process_code").
		Scan(&series).Error
	return series, translate(err)
}
