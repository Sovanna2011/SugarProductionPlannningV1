package postgres

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/database"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/security"
)

type inventoryRepository struct {
	db *gorm.DB
}

func NewInventoryRepository(db *gorm.DB) interfaces.InventoryRepository {
	return &inventoryRepository{db: db}
}

func (r *inventoryRepository) CreateMovements(ctx context.Context, movements []model.InventoryMovement) error {
	if len(movements) == 0 {
		return nil
	}
	var userID *int64
	if id, ok := security.UserIDFrom(ctx); ok {
		userID = &id
	}
	for i := range movements {
		movements[i].IsActive = true
		movements[i].Version = 1
		movements[i].CreatedBy = userID
		movements[i].ChangedBy = userID
	}
	return translate(database.Conn(ctx, r.db).Create(&movements).Error)
}

func (r *inventoryRepository) FindMovement(ctx context.Context, companyID, id int64) (*model.InventoryMovement, error) {
	var out model.InventoryMovement
	err := database.Conn(ctx, r.db).Preload("MovementType").
		Where("id = ? AND company_id = ?", id, companyID).First(&out).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrNotFound
		}
		return nil, translate(err)
	}
	return &out, nil
}

func (r *inventoryRepository) ListMovements(ctx context.Context, opts interfaces.ListOptions,
	f interfaces.MovementFilter) (interfaces.Page[model.InventoryMovement], error) {

	opts.Normalise()
	var page interfaces.Page[model.InventoryMovement]

	if opts.CompanyID == nil {
		return page, apperrors.ErrCompanyRequired
	}
	q := database.Conn(ctx, r.db).Model(&model.InventoryMovement{}).
		Where("company_id = ?", *opts.CompanyID)
	if f.WarehouseID != nil {
		q = q.Where("warehouse_id = ?", *f.WarehouseID)
	}
	if f.MaterialID != nil {
		q = q.Where("material_id = ?", *f.MaterialID)
	}
	if f.MovementTypeID != nil {
		q = q.Where("movement_type_id = ?", *f.MovementTypeID)
	}
	if f.DocumentNo != "" {
		q = q.Where("document_no = ?", f.DocumentNo)
	}
	if f.From != nil {
		q = q.Where("transaction_date >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("transaction_date <= ?", *f.To)
	}

	if err := q.Count(&page.Total).Error; err != nil {
		return page, translate(err)
	}
	err := q.Preload("MovementType").
		Order("transaction_date DESC, document_no DESC, line_no").
		Limit(opts.Size).Offset(opts.Offset()).Find(&page.Rows).Error
	return page, translate(err)
}

// MarkReversed flags the original. §33 forbids deleting a movement — the
// reversal is a new opposite row and the original stays in the ledger.
func (r *inventoryRepository) MarkReversed(ctx context.Context, companyID, movementID int64) error {
	res := database.Conn(ctx, r.db).Model(&model.InventoryMovement{}).
		Where("id = ? AND company_id = ? AND NOT is_reversed", movementID, companyID).
		Updates(map[string]any{
			"is_reversed": true,
			"version":     gorm.Expr("version + 1"),
			"changed_at":  gorm.Expr("now()"),
		})
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.ErrAlreadyReversed
	}
	return nil
}

func (r *inventoryRepository) NextLineNo(ctx context.Context, companyID int64, documentNo string) (int, error) {
	var next *int
	err := database.Conn(ctx, r.db).Model(&model.InventoryMovement{}).
		Select("MAX(line_no) + 1").
		Where("company_id = ? AND document_no = ?", companyID, documentNo).
		Scan(&next).Error
	if err != nil {
		return 0, translate(err)
	}
	if next == nil {
		return 1, nil
	}
	return *next, nil
}

// LockBalance takes the row lock that serialises concurrent postings for one
// stock key (§F4). The row is created on first use with the opening quantity
// carried forward from the most recent earlier balance, so a back-dated or
// first-ever posting starts from the correct base.
func (r *inventoryRepository) LockBalance(ctx context.Context, key interfaces.BalanceKey) (*model.InventoryBalance, error) {
	conn := database.Conn(ctx, r.db)

	var balance model.InventoryBalance
	err := conn.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(`company_id = ? AND warehouse_id = ? AND material_id = ?
		       AND packaging_type_id IS NOT DISTINCT FROM ? AND balance_date = ?`,
			key.CompanyID, key.WarehouseID, key.MaterialID, key.PackagingTypeID, key.BalanceDate).
		First(&balance).Error
	if err == nil {
		return &balance, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, translate(err)
	}

	opening, err := r.CurrentStock(ctx, key.CompanyID, key.WarehouseID, key.MaterialID,
		key.PackagingTypeID, key.BalanceDate.AddDate(0, 0, -1))
	if err != nil {
		return nil, err
	}

	fresh := model.InventoryBalance{
		CompanyID:       key.CompanyID,
		WarehouseID:     key.WarehouseID,
		MaterialID:      key.MaterialID,
		PackagingTypeID: key.PackagingTypeID,
		BalanceDate:     key.BalanceDate,
		OpeningQty:      opening,
		ClosingQty:      opening,
		Version:         1,
	}
	if userID, ok := security.UserIDFrom(ctx); ok {
		fresh.CreatedBy = &userID
		fresh.ChangedBy = &userID
	}

	// A concurrent poster may have inserted the same key in the meantime;
	// DoNothing plus a re-read keeps this race benign.
	if err := conn.Clauses(clause.OnConflict{DoNothing: true}).Create(&fresh).Error; err != nil {
		return nil, translate(err)
	}
	if fresh.ID != 0 {
		return &fresh, nil
	}
	return r.LockBalance(ctx, key)
}

// SaveBalance writes the recomputed row and cascades the delta to every later
// balance of the same key, so a back-dated posting cannot leave the forward
// history stale.
func (r *inventoryRepository) SaveBalance(ctx context.Context, balance *model.InventoryBalance) error {
	conn := database.Conn(ctx, r.db)

	var previousClosing decimal.Decimal
	if err := conn.Model(&model.InventoryBalance{}).
		Select("closing_qty").Where("id = ?", balance.ID).
		Scan(&previousClosing).Error; err != nil {
		return translate(err)
	}

	balance.Recompute()
	if userID, ok := security.UserIDFrom(ctx); ok {
		balance.ChangedBy = &userID
	}

	err := conn.Model(&model.InventoryBalance{}).Where("id = ?", balance.ID).
		Updates(map[string]any{
			"opening_qty": balance.OpeningQty,
			"in_qty":      balance.InQty,
			"out_qty":     balance.OutQty,
			"closing_qty": balance.ClosingQty,
			"uom_id":      balance.UOMID,
			"changed_by":  balance.ChangedBy,
			"version":     gorm.Expr("version + 1"),
			"changed_at":  gorm.Expr("now()"),
		}).Error
	if err != nil {
		return translate(err)
	}

	delta := balance.ClosingQty.Sub(previousClosing)
	if delta.IsZero() {
		return nil
	}
	return translate(conn.Exec(`
		UPDATE inventory_balances
		   SET opening_qty = opening_qty + ?,
		       closing_qty = closing_qty + ?,
		       version     = version + 1,
		       changed_at  = now()
		 WHERE company_id = ? AND warehouse_id = ? AND material_id = ?
		   AND packaging_type_id IS NOT DISTINCT FROM ? AND balance_date > ?`,
		delta, delta, balance.CompanyID, balance.WarehouseID, balance.MaterialID,
		balance.PackagingTypeID, balance.BalanceDate).Error)
}

// CurrentStock is the closing quantity of the most recent balance on or before
// asOf — zero when the key has never moved.
func (r *inventoryRepository) CurrentStock(ctx context.Context, companyID, warehouseID,
	materialID int64, packagingTypeID *int64, asOf time.Time) (decimal.Decimal, error) {

	var qty *decimal.Decimal
	err := database.Conn(ctx, r.db).Model(&model.InventoryBalance{}).
		Select("closing_qty").
		Where(`company_id = ? AND warehouse_id = ? AND material_id = ?
		       AND packaging_type_id IS NOT DISTINCT FROM ? AND balance_date <= ?`,
			companyID, warehouseID, materialID, packagingTypeID, asOf).
		Order("balance_date DESC").Limit(1).Scan(&qty).Error
	if err != nil {
		return decimal.Zero, translate(err)
	}
	if qty == nil {
		return decimal.Zero, nil
	}
	return *qty, nil
}

// WarehouseStock totals every material held in a location, converted into the
// capacity unit. Balances are stored in each material's base unit, so summing
// them raw would add tonnes to litres; the conversion factor comes from
// uom_conversions and a missing one is reported instead of assumed to be 1.
func (r *inventoryRepository) WarehouseStock(ctx context.Context, companyID,
	warehouseID, targetUOMID int64, asOf time.Time) (interfaces.WarehouseStock, error) {

	var out struct {
		Total             *decimal.Decimal
		ConversionMissing *bool
	}

	err := database.Conn(ctx, r.db).Raw(`
		SELECT COALESCE(SUM(latest.closing_qty * conv.factor), 0) AS total,
		       BOOL_OR(conv.factor IS NULL)                       AS conversion_missing
		  FROM (
		        SELECT DISTINCT ON (material_id, packaging_type_id) material_id, closing_qty
		          FROM inventory_balances
		         WHERE company_id = ? AND warehouse_id = ? AND balance_date <= ?
		         ORDER BY material_id, packaging_type_id, balance_date DESC
		       ) AS latest
		  JOIN materials m ON m.id = latest.material_id
		  CROSS JOIN LATERAL (
		        SELECT CASE WHEN m.base_uom_id = ? THEN 1::numeric
		                    ELSE (SELECT uc.numerator / uc.denominator
		                            FROM uom_conversions uc
		                           WHERE uc.from_uom_id = m.base_uom_id
		                             AND uc.to_uom_id   = ?
		                             AND uc.is_active)
		               END AS factor) conv`,
		companyID, warehouseID, asOf, targetUOMID, targetUOMID).Scan(&out).Error
	if err != nil {
		return interfaces.WarehouseStock{}, translate(err)
	}

	result := interfaces.WarehouseStock{}
	if out.Total != nil {
		result.Total = *out.Total
	}
	if out.ConversionMissing != nil {
		result.ConversionMissing = *out.ConversionMissing
	}
	return result, nil
}

func (r *inventoryRepository) ListBalances(ctx context.Context, companyID int64,
	warehouseID, materialID *int64, asOf time.Time) ([]interfaces.BalanceRow, error) {

	args := []any{companyID, asOf}
	filter := ""
	if warehouseID != nil {
		filter += " AND b.warehouse_id = ?"
		args = append(args, *warehouseID)
	}
	if materialID != nil {
		filter += " AND b.material_id = ?"
		args = append(args, *materialID)
	}

	var rows []interfaces.BalanceRow
	err := database.Conn(ctx, r.db).Raw(`
		SELECT b.company_id, b.warehouse_id, w.warehouse_code, w.warehouse_name,
		       b.material_id, m.material_code, m.material_name, b.packaging_type_id,
		       u.uom_code, b.opening_qty, b.in_qty, b.out_qty, b.closing_qty
		  FROM (
		        -- The inner alias must be b as well: the optional filters above
		        -- are written with a b. prefix and are spliced in here.
		        SELECT DISTINCT ON (b.company_id, b.warehouse_id, b.material_id, b.packaging_type_id) b.*
		          FROM inventory_balances b
		         WHERE b.company_id = ? AND b.balance_date <= ?`+filter+`
		         ORDER BY b.company_id, b.warehouse_id, b.material_id, b.packaging_type_id, b.balance_date DESC
		       ) b
		  JOIN warehouses w ON w.id = b.warehouse_id
		  JOIN materials  m ON m.id = b.material_id
		  LEFT JOIN uoms  u ON u.id = COALESCE(b.uom_id, m.base_uom_id)
		 ORDER BY w.warehouse_code, m.material_code`, args...).Scan(&rows).Error
	return rows, translate(err)
}

// RecomputeBalances rebuilds the materialised balances from the append-only
// ledger and reports every row that disagreed. This is the nightly
// reconciliation job of B3: the ledger is the source of truth, the balance
// table is only a performance projection of it.
func (r *inventoryRepository) RecomputeBalances(ctx context.Context, companyID int64,
	from, to time.Time) ([]interfaces.BalanceDiscrepancy, error) {

	conn := database.Conn(ctx, r.db)

	const computed = `
	WITH ledger AS (
	    SELECT im.company_id, im.warehouse_id, im.material_id, im.packaging_type_id,
	           im.transaction_date AS balance_date,
	           SUM(CASE WHEN mt.direction = 'IN'  THEN im.quantity ELSE 0 END) AS in_qty,
	           SUM(CASE WHEN mt.direction = 'OUT' THEN im.quantity ELSE 0 END) AS out_qty
	      FROM inventory_movements im
	      JOIN movement_types mt ON mt.id = im.movement_type_id
	     WHERE im.company_id = ? AND im.is_active AND mt.affects_stock
	     GROUP BY 1,2,3,4,5
	),
	running AS (
	    SELECT l.*,
	           SUM(l.in_qty - l.out_qty) OVER (
	               PARTITION BY l.company_id, l.warehouse_id, l.material_id, l.packaging_type_id
	               ORDER BY l.balance_date
	               ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS closing_qty
	      FROM ledger l
	)
	SELECT company_id, warehouse_id, material_id, packaging_type_id, balance_date,
	       closing_qty - (in_qty - out_qty) AS opening_qty, in_qty, out_qty, closing_qty
	  FROM running
	 WHERE balance_date BETWEEN ? AND ?`

	// A flat row is needed because BalanceDiscrepancy nests the key struct.
	type discrepancyRow struct {
		CompanyID       int64
		WarehouseID     int64
		MaterialID      int64
		PackagingTypeID *int64
		BalanceDate     time.Time
		Stored          decimal.Decimal
		Computed        decimal.Decimal
	}

	// Report first, so the caller sees what was wrong before it is corrected.
	var scanned []discrepancyRow
	err := conn.Raw(`
		SELECT c.company_id, c.warehouse_id, c.material_id, c.packaging_type_id, c.balance_date,
		       COALESCE(b.closing_qty, 0) AS stored, c.closing_qty AS computed
		  FROM (`+computed+`) c
		  LEFT JOIN inventory_balances b
		         ON b.company_id = c.company_id AND b.warehouse_id = c.warehouse_id
		        AND b.material_id = c.material_id
		        AND b.packaging_type_id IS NOT DISTINCT FROM c.packaging_type_id
		        AND b.balance_date = c.balance_date
		 WHERE COALESCE(b.closing_qty, 0) <> c.closing_qty`,
		companyID, from, to).Scan(&scanned).Error
	if err != nil {
		return nil, translate(err)
	}

	discrepancies := make([]interfaces.BalanceDiscrepancy, 0, len(scanned))
	for _, row := range scanned {
		discrepancies = append(discrepancies, interfaces.BalanceDiscrepancy{
			Key: interfaces.BalanceKey{
				CompanyID:       row.CompanyID,
				WarehouseID:     row.WarehouseID,
				MaterialID:      row.MaterialID,
				PackagingTypeID: row.PackagingTypeID,
				BalanceDate:     row.BalanceDate,
			},
			Stored:   row.Stored,
			Computed: row.Computed,
		})
	}

	err = conn.Exec(`
		INSERT INTO inventory_balances
		    (company_id, warehouse_id, material_id, packaging_type_id, balance_date,
		     opening_qty, in_qty, out_qty, closing_qty)
		`+computed+`
		ON CONFLICT (company_id, warehouse_id, material_id, packaging_type_id, balance_date)
		DO UPDATE SET opening_qty = EXCLUDED.opening_qty,
		              in_qty      = EXCLUDED.in_qty,
		              out_qty     = EXCLUDED.out_qty,
		              closing_qty = EXCLUDED.closing_qty,
		              version     = inventory_balances.version + 1,
		              changed_at  = now()`,
		companyID, from, to).Error
	if err != nil {
		return nil, translate(err)
	}

	return discrepancies, nil
}
