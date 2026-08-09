package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/database"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
)

type reportRepository struct {
	db *gorm.DB
}

func NewReportRepository(db *gorm.DB) interfaces.ReportRepository {
	return &reportRepository{db: db}
}

// groupExpression maps the API's groupBy value to the SQL projection. The map
// is the whitelist: an unknown value never reaches the statement.
type groupExpression struct {
	keyExpr   string
	labelExpr string
	joins     string
	groupBy   string
}

var planVsActualGroups = map[string]groupExpression{
	"material": {
		keyExpr:   "m.material_code",
		labelExpr: "m.material_name",
		joins:     "JOIN materials m ON m.id = c.material_id",
		groupBy:   "c.company_id, co.company_code, c.material_id, m.material_code, m.material_name, bu.uom_code",
	},
	"line": {
		keyExpr:   "COALESCE(pl.line_code, '(unassigned)')",
		labelExpr: "COALESCE(pl.line_name, 'Not assigned to a line')",
		joins:     "LEFT JOIN production_lines pl ON pl.id = c.production_line_id",
		groupBy:   "c.company_id, co.company_code, c.production_line_id, pl.line_code, pl.line_name",
	},
	"process": {
		keyExpr:   "COALESCE(pr.process_code, '(none)')",
		labelExpr: "COALESCE(pr.process_name, 'No process')",
		joins:     "LEFT JOIN processes pr ON pr.id = c.process_id",
		groupBy:   "c.company_id, co.company_code, c.process_id, pr.process_code, pr.process_name",
	},
	"date": {
		keyExpr:   "to_char(c.entry_date, 'YYYY-MM-DD')",
		labelExpr: "to_char(c.entry_date, 'YYYY-MM-DD')",
		joins:     "",
		groupBy:   "c.company_id, co.company_code, c.entry_date",
	},
}

// PlanVsActual implements §F6. A FULL OUTER JOIN is essential: a material that
// was planned but never produced, and one that was produced without a plan,
// must both appear.
//
// Quantities are converted to the material's base unit before they are
// compared; a row whose conversion is not maintained is flagged rather than
// silently added up.
func (r *reportRepository) PlanVsActual(ctx context.Context, q interfaces.PlanVsActualQuery) ([]interfaces.PlanVsActualRow, error) {
	group, ok := planVsActualGroups[q.GroupBy]
	if !ok {
		return nil, apperrors.ErrValidation.Msgf("unsupported groupBy %q", q.GroupBy)
	}
	if len(q.CompanyIDs) == 0 {
		return nil, apperrors.ErrCompanyRequired
	}

	// Only the version predicate varies; both branches stay parameterised.
	versionFilter := ""
	args := []any{q.CompanyIDs, q.Range.From, q.Range.To}
	if q.VersionID != nil {
		versionFilter = "AND ph.planning_version_id = ?"
		args = append(args, *q.VersionID)
	} else if q.SeasonID != nil {
		versionFilter = `AND ph.planning_version_id IN (
		        SELECT DISTINCT ON (company_id) id FROM planning_versions
		         WHERE company_id IN ? AND season_id = ? AND status IN ('APPROVED','LOCKED')
		         ORDER BY company_id, version_no DESC)`
		args = append(args, q.CompanyIDs, *q.SeasonID)
	}
	args = append(args, q.CompanyIDs, q.Range.From, q.Range.To)

	sql := fmt.Sprintf(`
WITH plan AS (
    SELECT ph.company_id, pi.material_id, pi.production_line_id, pi.process_id,
           pi.plan_date AS entry_date,
           SUM(pi.quantity * conv.factor) AS qty,
           BOOL_OR(conv.factor IS NULL)   AS conversion_missing
      FROM plan_items pi
      JOIN plan_headers ph ON ph.id = pi.plan_header_id
      JOIN materials     m ON m.id  = pi.material_id
      CROSS JOIN LATERAL (
            SELECT CASE WHEN pi.uom_id = m.base_uom_id THEN 1::numeric
                        ELSE (SELECT uc.numerator / uc.denominator
                                FROM uom_conversions uc
                               WHERE uc.from_uom_id = pi.uom_id
                                 AND uc.to_uom_id   = m.base_uom_id
                                 AND uc.is_active)
                   END AS factor) conv
     WHERE ph.company_id IN ? AND pi.is_active
       AND pi.plan_date BETWEEN ? AND ? %s
     GROUP BY 1,2,3,4,5
),
act AS (
    SELECT ah.company_id, ai.material_id, ai.production_line_id, ai.process_id,
           ai.actual_date AS entry_date,
           SUM(ai.quantity * conv.factor) AS qty,
           BOOL_OR(conv.factor IS NULL)   AS conversion_missing
      FROM actual_items ai
      JOIN actual_headers ah ON ah.id = ai.actual_header_id
      JOIN materials       m ON m.id  = ai.material_id
      CROSS JOIN LATERAL (
            SELECT CASE WHEN ai.uom_id = m.base_uom_id THEN 1::numeric
                        ELSE (SELECT uc.numerator / uc.denominator
                                FROM uom_conversions uc
                               WHERE uc.from_uom_id = ai.uom_id
                                 AND uc.to_uom_id   = m.base_uom_id
                                 AND uc.is_active)
                   END AS factor) conv
     WHERE ah.company_id IN ? AND ai.is_active
       AND ah.posting_status = 'POSTED'
       AND ai.actual_date BETWEEN ? AND ?
     GROUP BY 1,2,3,4,5
),
c AS (
    SELECT COALESCE(p.company_id, a.company_id)                 AS company_id,
           COALESCE(p.material_id, a.material_id)               AS material_id,
           COALESCE(p.production_line_id, a.production_line_id) AS production_line_id,
           COALESCE(p.process_id, a.process_id)                 AS process_id,
           COALESCE(p.entry_date, a.entry_date)                 AS entry_date,
           COALESCE(p.qty, 0)                                   AS plan_qty,
           COALESCE(a.qty, 0)                                   AS actual_qty,
           COALESCE(p.conversion_missing, FALSE)
             OR COALESCE(a.conversion_missing, FALSE)           AS conversion_missing
      FROM plan p
      FULL OUTER JOIN act a
        ON  a.company_id = p.company_id
        AND a.material_id = p.material_id
        AND a.production_line_id IS NOT DISTINCT FROM p.production_line_id
        AND a.process_id         IS NOT DISTINCT FROM p.process_id
        AND a.entry_date = p.entry_date
)
SELECT c.company_id,
       co.company_code,
       %s AS group_key,
       %s AS group_label,
       COALESCE(bu.uom_code, '')   AS uom_code,
       SUM(c.plan_qty)             AS plan_qty,
       SUM(c.actual_qty)           AS actual_qty,
       BOOL_OR(c.conversion_missing) AS conversion_missing
  FROM c
  JOIN companies co ON co.id = c.company_id
  LEFT JOIN materials mm ON mm.id = c.material_id
  LEFT JOIN uoms bu ON bu.id = mm.base_uom_id
  %s
 GROUP BY %s
 ORDER BY co.company_code, group_key`,
		versionFilter, group.keyExpr, group.labelExpr, group.joins, group.groupBy)

	// The material grouping already lists bu.uom_code in its GROUP BY; the
	// other groupings aggregate across materials, so the unit is not a
	// meaningful column there and is grouped away.
	if q.GroupBy != "material" {
		sql = strings.Replace(sql, "COALESCE(bu.uom_code, '')   AS uom_code", "''::text AS uom_code", 1)
	}

	var rows []interfaces.PlanVsActualRow
	err := database.Conn(ctx, r.db).Raw(sql, args...).Scan(&rows).Error
	return rows, translate(err)
}

func (r *reportRepository) ProductionSummary(ctx context.Context, companyID int64,
	rng interfaces.DateRange) ([]interfaces.ProductionSummaryRow, error) {

	var rows []interfaces.ProductionSummaryRow
	err := database.Conn(ctx, r.db).Raw(`
		SELECT ai.material_id, m.material_code, m.material_name,
		       pr.process_code, u.uom_code, SUM(ai.quantity) AS quantity
		  FROM actual_items ai
		  JOIN actual_headers ah ON ah.id = ai.actual_header_id
		  JOIN materials m  ON m.id = ai.material_id
		  JOIN uoms u       ON u.id = ai.uom_id
		  LEFT JOIN processes pr ON pr.id = ai.process_id
		 WHERE ah.company_id = ? AND ah.posting_status = 'POSTED' AND ai.is_active
		   AND ai.actual_date BETWEEN ? AND ?
		 GROUP BY ai.material_id, m.material_code, m.material_name, pr.process_code, u.uom_code
		 ORDER BY m.material_code`,
		companyID, rng.From, rng.To).Scan(&rows).Error
	return rows, translate(err)
}

func (r *reportRepository) InventoryMovementReport(ctx context.Context, companyID int64,
	rng interfaces.DateRange, warehouseID, materialID *int64) ([]interfaces.MovementReportRow, error) {

	args := []any{companyID, rng.From, rng.To}
	filter := ""
	if warehouseID != nil {
		filter += " AND im.warehouse_id = ?"
		args = append(args, *warehouseID)
	}
	if materialID != nil {
		filter += " AND im.material_id = ?"
		args = append(args, *materialID)
	}

	var rows []interfaces.MovementReportRow
	err := database.Conn(ctx, r.db).Raw(`
		SELECT im.transaction_date, im.document_no, w.warehouse_code, m.material_code,
		       mt.movement_code, mt.direction, im.quantity, u.uom_code,
		       im.source_module, im.is_reversed
		  FROM inventory_movements im
		  JOIN warehouses     w  ON w.id  = im.warehouse_id
		  JOIN materials      m  ON m.id  = im.material_id
		  JOIN movement_types mt ON mt.id = im.movement_type_id
		  JOIN uoms           u  ON u.id  = im.uom_id
		 WHERE im.company_id = ? AND im.is_active
		   AND im.transaction_date BETWEEN ? AND ?`+filter+`
		 ORDER BY im.transaction_date, im.document_no, im.line_no`,
		args...).Scan(&rows).Error
	return rows, translate(err)
}

// CapacityUtilisation returns the raw figures; the percentage and its
// traffic-light band are computed in the service so the thresholds stay
// configurable (§F5).
func (r *reportRepository) CapacityUtilisation(ctx context.Context, companyID int64,
	asOf time.Time) ([]interfaces.CapacityRow, error) {

	var rows []interfaces.CapacityRow
	err := database.Conn(ctx, r.db).Raw(`
		SELECT w.id AS warehouse_id, w.warehouse_code, w.warehouse_name, w.warehouse_type,
		       w.capacity, cu.uom_code AS capacity_uom,
		       COALESCE(stock.total, 0) AS current_stock
		  FROM warehouses w
		  LEFT JOIN uoms cu ON cu.id = w.capacity_uom_id
		  LEFT JOIN LATERAL (
		        SELECT SUM(latest.closing_qty) AS total
		          FROM (
		                SELECT DISTINCT ON (material_id, packaging_type_id) closing_qty
		                  FROM inventory_balances b
		                 WHERE b.company_id = w.company_id AND b.warehouse_id = w.id
		                   AND b.balance_date <= ?
		                 ORDER BY material_id, packaging_type_id, balance_date DESC
		               ) latest
		       ) stock ON TRUE
		 WHERE w.company_id = ? AND w.is_active
		 ORDER BY w.warehouse_code`,
		asOf, companyID).Scan(&rows).Error
	return rows, translate(err)
}
