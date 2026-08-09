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

// --- cane varieties ------------------------------------------------------

type caneVarietyRepository struct {
	crossRepo[model.CaneVariety, *model.CaneVariety]
}

func NewCaneVarietyRepository(db *gorm.DB) interfaces.CaneVarietyRepository {
	r := &caneVarietyRepository{}
	r.db = db
	r.sortable = columns("variety_code", "variety_name", "maturity_months", "typical_ccs_pct")
	r.searchable = []string{"variety_code", "variety_name"}
	return r
}

func (r *caneVarietyRepository) FindByCode(ctx context.Context, code string) (*model.CaneVariety, error) {
	return findOne[model.CaneVariety](ctx, r.conn(ctx), "variety_code = ?", code)
}

// --- growers -------------------------------------------------------------

type growerRepository struct {
	companyRepo[model.Grower, *model.Grower]
}

func NewGrowerRepository(db *gorm.DB) interfaces.GrowerRepository {
	r := &growerRepository{}
	r.db = db
	r.companyColumn = "company_id"
	r.sortable = columns("grower_code", "grower_name", "grower_type", "supply_type",
		"zone", "contract_tons", "price_per_ton", "created_at")
	r.searchable = []string{"grower_code", "grower_name", "contact_name", "contract_no"}
	return r
}

func (r *growerRepository) FindByCode(ctx context.Context, companyID int64, code string) (*model.Grower, error) {
	return findOne[model.Grower](ctx, r.conn(ctx), "company_id = ? AND grower_code = ?", companyID, code)
}

// --- cane fields ---------------------------------------------------------

type caneFieldRepository struct {
	companyRepo[model.CaneField, *model.CaneField]
}

func NewCaneFieldRepository(db *gorm.DB) interfaces.CaneFieldRepository {
	r := &caneFieldRepository{}
	r.db = db
	r.companyColumn = "company_id"
	r.sortable = columns("field_code", "field_name", "grower_id", "variety_id",
		"area_ha", "expected_yield_tph", "crop_cycle", "zone")
	r.searchable = []string{"field_code", "field_name", "zone"}
	return r
}

func (r *caneFieldRepository) FindByCode(ctx context.Context, companyID int64, code string) (*model.CaneField, error) {
	return findOne[model.CaneField](ctx, r.conn(ctx), "company_id = ? AND field_code = ?", companyID, code)
}

func (r *caneFieldRepository) ListForGrower(ctx context.Context, companyID, growerID int64) ([]model.CaneField, error) {
	var rows []model.CaneField
	err := r.conn(ctx).
		Where("company_id = ? AND grower_id = ? AND is_active", companyID, growerID).
		Order("field_code").Find(&rows).Error
	return rows, translate(err)
}

// --- harvest plan --------------------------------------------------------

type harvestPlanRepository struct {
	db *gorm.DB
}

func NewHarvestPlanRepository(db *gorm.DB) interfaces.HarvestPlanRepository {
	return &harvestPlanRepository{db: db}
}

func (r *harvestPlanRepository) CreateHeader(ctx context.Context, header *model.HarvestPlanHeader) error {
	if userID, ok := security.UserIDFrom(ctx); ok {
		header.SetAuditUser(userID, true)
	}
	header.IsActive = true
	header.Version = 1
	return translate(database.Conn(ctx, r.db).Create(header).Error)
}

func (r *harvestPlanRepository) UpdateHeader(ctx context.Context, companyID int64, header *model.HarvestPlanHeader) error {
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

func (r *harvestPlanRepository) FindHeader(ctx context.Context, companyID, headerID int64) (*model.HarvestPlanHeader, error) {
	return findOne[model.HarvestPlanHeader](ctx, database.Conn(ctx, r.db),
		"id = ? AND company_id = ?", headerID, companyID)
}

// FindHeaderByVersion returns nil rather than an error when the version has no
// harvest document yet — the matrix save creates one on first use.
func (r *harvestPlanRepository) FindHeaderByVersion(ctx context.Context, companyID, versionID int64) (*model.HarvestPlanHeader, error) {
	var out model.HarvestPlanHeader
	err := database.Conn(ctx, r.db).
		Where("company_id = ? AND planning_version_id = ?", companyID, versionID).
		First(&out).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, translate(err)
	}
	return &out, nil
}

func (r *harvestPlanRepository) ListHeaders(ctx context.Context, opts interfaces.ListOptions,
	versionID *int64) (interfaces.Page[model.HarvestPlanHeader], error) {

	opts.Normalise()
	var page interfaces.Page[model.HarvestPlanHeader]

	if opts.CompanyID == nil {
		return page, apperrors.ErrCompanyRequired
	}
	q := database.Conn(ctx, r.db).Model(&model.HarvestPlanHeader{}).
		Where("company_id = ?", *opts.CompanyID)
	if !opts.IncludeInactive {
		q = q.Where("is_active")
	}
	if versionID != nil {
		q = q.Where("planning_version_id = ?", *versionID)
	}

	if err := q.Count(&page.Total).Error; err != nil {
		return page, translate(err)
	}
	err := q.Order("document_no").Limit(opts.Size).Offset(opts.Offset()).Find(&page.Rows).Error
	return page, translate(err)
}

func (r *harvestPlanRepository) Items(ctx context.Context, headerID int64) ([]model.HarvestPlanItem, error) {
	var rows []model.HarvestPlanItem
	err := database.Conn(ctx, r.db).
		Where("harvest_plan_header_id = ? AND is_active", headerID).
		Order("plan_date, grower_id, cane_field_id").Find(&rows).Error
	return rows, translate(err)
}

// UpsertItems is the idempotent bulk save behind the harvest matrix, keyed on
// the business key of §28 as it applies to cane: (header, date, grower, field).
func (r *harvestPlanRepository) UpsertItems(ctx context.Context, headerID int64, items []model.HarvestPlanItem) error {
	if len(items) == 0 {
		return nil
	}

	var userID *int64
	if id, ok := security.UserIDFrom(ctx); ok {
		userID = &id
	}

	placeholders := make([]string, 0, len(items))
	args := make([]any, 0, len(items)*12)
	for _, it := range items {
		placeholders = append(placeholders,
			"(?::bigint, ?::date, ?::bigint, ?::bigint, ?::bigint, ?::numeric, ?::bigint, ?::numeric, ?::numeric, ?::varchar, TRUE, 1, ?::bigint, ?::bigint)")
		args = append(args, headerID, it.PlanDate, it.GrowerID, it.CaneFieldID, it.VarietyID,
			it.PlannedTons, it.UOMID, it.PlannedAreaHa, it.ExpectedCCSPct, it.Remark, userID, userID)
	}

	sql := fmt.Sprintf(`
INSERT INTO harvest_plan_items
    (harvest_plan_header_id, plan_date, grower_id, cane_field_id, variety_id,
     planned_tons, uom_id, planned_area_ha, expected_ccs_pct, remark,
     is_active, version, created_by, changed_by)
VALUES %s
ON CONFLICT (harvest_plan_header_id, plan_date, grower_id, cane_field_id)
DO UPDATE SET planned_tons     = EXCLUDED.planned_tons,
              uom_id           = EXCLUDED.uom_id,
              variety_id       = EXCLUDED.variety_id,
              planned_area_ha  = EXCLUDED.planned_area_ha,
              expected_ccs_pct = EXCLUDED.expected_ccs_pct,
              remark           = EXCLUDED.remark,
              is_active        = TRUE,
              version          = harvest_plan_items.version + 1,
              changed_by       = EXCLUDED.changed_by,
              changed_at       = now()`, strings.Join(placeholders, ",\n       "))

	return translate(database.Conn(ctx, r.db).Exec(sql, args...).Error)
}

func (r *harvestPlanRepository) DeleteItems(ctx context.Context, headerID int64, itemIDs []int64) error {
	if len(itemIDs) == 0 {
		return nil
	}
	return translate(database.Conn(ctx, r.db).
		Where("harvest_plan_header_id = ? AND id IN ?", headerID, itemIDs).
		Delete(&model.HarvestPlanItem{}).Error)
}

func (r *harvestPlanRepository) MatrixCells(ctx context.Context, companyID, versionID int64,
	from, to time.Time) ([]interfaces.HarvestMatrixCell, error) {

	var cells []interfaces.HarvestMatrixCell
	err := database.Conn(ctx, r.db).
		Table("harvest_plan_items hi").
		Select(`hi.harvest_plan_header_id, hi.id AS harvest_plan_item_id, hi.plan_date,
		        hi.grower_id, hi.cane_field_id, hi.variety_id, hi.planned_tons,
		        hi.planned_area_ha, hi.expected_ccs_pct, hi.uom_id, hi.version`).
		Joins("JOIN harvest_plan_headers hh ON hh.id = hi.harvest_plan_header_id").
		Where(`hh.company_id = ? AND hh.planning_version_id = ?
		       AND hi.plan_date BETWEEN ? AND ? AND hi.is_active`,
			companyID, versionID, from, to).
		Order("hi.plan_date, hi.grower_id, hi.cane_field_id").
		Scan(&cells).Error
	return cells, translate(err)
}

// --- cane deliveries -----------------------------------------------------

type caneDeliveryRepository struct {
	companyRepo[model.CaneDelivery, *model.CaneDelivery]
}

func NewCaneDeliveryRepository(db *gorm.DB) interfaces.CaneDeliveryRepository {
	r := &caneDeliveryRepository{}
	r.db = db
	r.companyColumn = "company_id"
	r.sortable = columns("document_no", "delivery_date", "grower_id", "cane_field_id",
		"variety_id", "net_tons", "gross_tons", "ccs_pct", "posting_status")
	r.searchable = []string{"document_no", "ticket_no", "vehicle_no"}
	return r
}

func (r *caneDeliveryRepository) FindByIdempotencyKey(ctx context.Context, companyID int64, key string) (*model.CaneDelivery, error) {
	var out model.CaneDelivery
	err := r.conn(ctx).Where("company_id = ? AND idempotency_key = ?", companyID, key).First(&out).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, translate(err)
	}
	return &out, nil
}

func (r *caneDeliveryRepository) List(ctx context.Context, opts interfaces.ListOptions,
	f interfaces.DeliveryFilter) (interfaces.Page[model.CaneDelivery], error) {

	opts.Normalise()
	var page interfaces.Page[model.CaneDelivery]

	if opts.CompanyID == nil {
		return page, apperrors.ErrCompanyRequired
	}
	q := r.conn(ctx).Model(&model.CaneDelivery{}).Where("company_id = ?", *opts.CompanyID)
	if !opts.IncludeInactive {
		q = q.Where("is_active")
	}
	if f.GrowerID != nil {
		q = q.Where("grower_id = ?", *f.GrowerID)
	}
	if f.CaneFieldID != nil {
		q = q.Where("cane_field_id = ?", *f.CaneFieldID)
	}
	if f.VarietyID != nil {
		q = q.Where("variety_id = ?", *f.VarietyID)
	}
	if f.Status != "" {
		q = q.Where("posting_status = ?", f.Status)
	}
	if f.From != nil {
		q = q.Where("delivery_date >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("delivery_date <= ?", *f.To)
	}

	if err := q.Count(&page.Total).Error; err != nil {
		return page, translate(err)
	}
	err := q.Order("delivery_date DESC, document_no DESC").
		Limit(opts.Size).Offset(opts.Offset()).Find(&page.Rows).Error
	return page, translate(err)
}

// SetStatus writes the posting transition under the same optimistic lock as
// any other change.
func (r *caneDeliveryRepository) SetStatus(ctx context.Context, delivery *model.CaneDelivery) error {
	return r.update(ctx, &delivery.CompanyID, delivery)
}

// --- cane reporting ------------------------------------------------------

type caneReportRepository struct {
	db *gorm.DB
}

func NewCaneReportRepository(db *gorm.DB) interfaces.CaneReportRepository {
	return &caneReportRepository{db: db}
}

// canePlanVsActual joins the planned harvest to the delivered tonnage on the
// grouping key. It is a full outer join, so a grower who delivered without a
// plan appears just as a planned grower who delivered nothing does — the
// variance of the first is "n/a" and of the second is -100 %.
//
// Only the chosen grouping keys the result. Grower, variety and date are
// projected when the grouping is the one that determines them and are NULL
// otherwise: a report grouped by zone must not also split by grower, and it
// must not name an arbitrary grower as if the row belonged to them.
const canePlanVsActualSQL = `
WITH plan AS (
    SELECT %[1]s AS group_key,
           %[3]s,
           SUM(hi.planned_tons) AS planned_tons
      FROM harvest_plan_items hi
      JOIN harvest_plan_headers hh ON hh.id = hi.harvest_plan_header_id
      JOIN growers g               ON g.id = hi.grower_id
 LEFT JOIN cane_varieties v        ON v.id = hi.variety_id
     WHERE hh.company_id = @companyId
       AND hi.plan_date BETWEEN @dateFrom AND @dateTo
       AND hi.is_active
       AND (@versionId = 0 OR hh.planning_version_id = @versionId)
       AND (@supplyType = '' OR g.supply_type = @supplyType)
     GROUP BY 1, 2, 3, 4
),
actual AS (
    SELECT %[2]s AS group_key,
           %[4]s,
           SUM(d.net_tons) AS actual_tons,
           COUNT(*)        AS deliveries,
           -- tonnage-weighted, because a 40 t load says more about the day's
           -- cane quality than a 4 t one
           CASE WHEN SUM(d.net_tons) FILTER (WHERE d.ccs_pct IS NOT NULL) > 0
                THEN SUM(d.net_tons * d.ccs_pct) FILTER (WHERE d.ccs_pct IS NOT NULL)
                     / SUM(d.net_tons) FILTER (WHERE d.ccs_pct IS NOT NULL)
           END AS avg_ccs_pct,
           -- A single unpriced load makes the whole group's value unknown
           -- rather than understated.
           CASE WHEN COUNT(*) FILTER (WHERE d.price_per_ton IS NULL) = 0
                THEN SUM(d.net_tons * d.price_per_ton)
           END AS purchase_value,
           MIN(d.currency) AS currency
      FROM cane_deliveries d
      JOIN growers g        ON g.id = d.grower_id
 LEFT JOIN cane_varieties v ON v.id = d.variety_id
     WHERE d.company_id = @companyId
       AND d.delivery_date BETWEEN @dateFrom AND @dateTo
       AND d.posting_status = 'POSTED'
       AND d.is_active
       AND (@supplyType = '' OR g.supply_type = @supplyType)
     GROUP BY 1, 2, 3, 4
)
SELECT COALESCE(p.group_key, a.group_key)   AS group_key,
       COALESCE(p.group_key, a.group_key)   AS group_label,
       COALESCE(gp.supply_type, ga.supply_type, '') AS supply_type,
       COALESCE(p.grower_id, a.grower_id)   AS grower_id,
       COALESCE(gp.grower_code, ga.grower_code)     AS grower_code,
       COALESCE(p.variety_id, a.variety_id) AS variety_id,
       COALESCE(vp.variety_code, va.variety_code)   AS variety_code,
       COALESCE(p.plan_date, a.plan_date)   AS plan_date,
       COALESCE(p.planned_tons, 0)          AS planned_tons,
       COALESCE(a.actual_tons, 0)           AS actual_tons,
       COALESCE(a.deliveries, 0)            AS deliveries,
       a.avg_ccs_pct                        AS average_ccs_pct,
       a.purchase_value                     AS purchase_value,
       a.currency                           AS currency
  FROM plan p
  FULL OUTER JOIN actual a ON a.group_key = p.group_key
  LEFT JOIN growers gp        ON gp.id = p.grower_id
  LEFT JOIN growers ga        ON ga.id = a.grower_id
  LEFT JOIN cane_varieties vp ON vp.id = p.variety_id
  LEFT JOIN cane_varieties va ON va.id = a.variety_id
 ORDER BY 1`

// caneGrouping describes one whitelisted grouping: how the key is built on
// each side, and which attributes that grouping legitimately determines. The
// client names a grouping, never an expression — nothing caller-supplied
// reaches the statement.
type caneGrouping struct {
	planKey     string
	actualKey   string
	planAttrs   string
	actualAttrs string
}

const (
	noGrowerAttr  = "NULL::bigint AS grower_id"
	noVarietyAttr = "NULL::bigint AS variety_id"
	noDateAttr    = "NULL::date AS plan_date"
)

var caneGroupings = map[string]caneGrouping{
	"grower": {
		planKey:     "g.grower_code || ' — ' || g.grower_name",
		actualKey:   "g.grower_code || ' — ' || g.grower_name",
		planAttrs:   "hi.grower_id, " + noVarietyAttr + ", " + noDateAttr,
		actualAttrs: "d.grower_id, " + noVarietyAttr + ", " + noDateAttr,
	},
	"variety": {
		planKey:     "COALESCE(v.variety_code, '(none)')",
		actualKey:   "COALESCE(v.variety_code, '(none)')",
		planAttrs:   noGrowerAttr + ", hi.variety_id, " + noDateAttr,
		actualAttrs: noGrowerAttr + ", d.variety_id, " + noDateAttr,
	},
	"date": {
		planKey:     "to_char(hi.plan_date, 'YYYY-MM-DD')",
		actualKey:   "to_char(d.delivery_date, 'YYYY-MM-DD')",
		planAttrs:   noGrowerAttr + ", " + noVarietyAttr + ", hi.plan_date",
		actualAttrs: noGrowerAttr + ", " + noVarietyAttr + ", d.delivery_date AS plan_date",
	},
	"zone": {
		planKey:     "COALESCE(g.zone, '(unzoned)')",
		actualKey:   "COALESCE(g.zone, '(unzoned)')",
		planAttrs:   noGrowerAttr + ", " + noVarietyAttr + ", " + noDateAttr,
		actualAttrs: noGrowerAttr + ", " + noVarietyAttr + ", " + noDateAttr,
	},
	"supplytype": {
		planKey:     "g.supply_type",
		actualKey:   "g.supply_type",
		planAttrs:   noGrowerAttr + ", " + noVarietyAttr + ", " + noDateAttr,
		actualAttrs: noGrowerAttr + ", " + noVarietyAttr + ", " + noDateAttr,
	},
}

func (r *caneReportRepository) CanePlanVsActual(ctx context.Context,
	q interfaces.CanePlanVsActualQuery) ([]interfaces.CanePlanVsActualRow, error) {

	grouping, ok := caneGroupings[strings.ToLower(q.GroupBy)]
	if !ok {
		return nil, apperrors.ErrValidation.
			Msgf("groupBy must be one of grower, variety, date, zone or supplyType").
			WithDetails(apperrors.Detail{Field: "groupBy", Value: q.GroupBy})
	}

	var versionID int64
	if q.VersionID != nil {
		versionID = *q.VersionID
	}

	var rows []interfaces.CanePlanVsActualRow
	err := database.Conn(ctx, r.db).Raw(
		fmt.Sprintf(canePlanVsActualSQL,
			grouping.planKey, grouping.actualKey, grouping.planAttrs, grouping.actualAttrs),
		map[string]any{
			"companyId":  q.CompanyID,
			"versionId":  versionID,
			"dateFrom":   q.Range.From,
			"dateTo":     q.Range.To,
			"supplyType": q.SupplyType,
		}).Scan(&rows).Error
	return rows, translate(err)
}
