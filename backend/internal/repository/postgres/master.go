package postgres

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/database"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
)

func columns(names ...string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}

// --- companies -----------------------------------------------------------

type companyRepository struct {
	crossRepo[model.Company, *model.Company]
}

func NewCompanyRepository(db *gorm.DB) interfaces.CompanyRepositoryIface {
	r := &companyRepository{}
	r.db = db
	r.sortable = columns("company_code", "company_name", "local_currency", "country_code", "created_at")
	r.searchable = []string{"company_code", "company_name"}
	return r
}

func (r *companyRepository) FindByCode(ctx context.Context, code string) (*model.Company, error) {
	return findOne[model.Company](ctx, r.conn(ctx), "company_code = ?", code)
}

// --- units of measure ----------------------------------------------------

type uomRepository struct {
	crossRepo[model.UOM, *model.UOM]
}

func NewUOMRepository(db *gorm.DB) interfaces.UOMRepository {
	r := &uomRepository{}
	r.db = db
	r.sortable = columns("uom_code", "uom_name", "dimension")
	r.searchable = []string{"uom_code", "uom_name"}
	return r
}

func (r *uomRepository) FindByCode(ctx context.Context, code string) (*model.UOM, error) {
	return findOne[model.UOM](ctx, r.conn(ctx), "uom_code = ?", code)
}

func (r *uomRepository) FindConversion(ctx context.Context, fromUOMID, toUOMID int64) (*model.UOMConversion, error) {
	var out model.UOMConversion
	err := r.conn(ctx).
		Where("from_uom_id = ? AND to_uom_id = ? AND is_active", fromUOMID, toUOMID).
		First(&out).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // "not maintained" is a valid answer, not an error
		}
		return nil, translate(err)
	}
	return &out, nil
}

// --- materials -----------------------------------------------------------

type materialRepository struct {
	crossRepo[model.Material, *model.Material]
}

func NewMaterialRepository(db *gorm.DB) interfaces.MaterialRepository {
	r := &materialRepository{}
	r.db = db
	r.sortable = columns("material_code", "material_name", "material_type", "material_group", "created_at")
	r.searchable = []string{"material_code", "material_name"}
	return r
}

func (r *materialRepository) FindByCode(ctx context.Context, code string) (*model.Material, error) {
	return findOne[model.Material](ctx, r.conn(ctx), "material_code = ?", code)
}

func (r *materialRepository) ListForCompany(ctx context.Context, companyID int64,
	opts interfaces.ListOptions) (interfaces.Page[model.CompanyMaterial], error) {

	opts.Normalise()
	var page interfaces.Page[model.CompanyMaterial]

	q := r.conn(ctx).Model(&model.CompanyMaterial{}).
		Joins("JOIN materials m ON m.id = company_materials.material_id").
		Where("company_materials.company_id = ?", companyID)
	if !opts.IncludeInactive {
		q = q.Where("company_materials.is_active AND m.is_active")
	}
	if opts.Search != "" {
		q = q.Where("(m.material_code ILIKE ? OR m.material_name ILIKE ?)",
			"%"+opts.Search+"%", "%"+opts.Search+"%")
	}

	if err := q.Count(&page.Total).Error; err != nil {
		return page, translate(err)
	}

	err := q.Preload("Material").Preload("Material.BaseUOM").
		Order("m.material_code").
		Limit(opts.Size).Offset(opts.Offset()).
		Find(&page.Rows).Error
	if err != nil {
		return page, translate(err)
	}
	return page, nil
}

func (r *materialRepository) FindCompanyMaterial(ctx context.Context, companyID, materialID int64) (*model.CompanyMaterial, error) {
	var out model.CompanyMaterial
	err := r.conn(ctx).Preload("Material").
		Where("company_id = ? AND material_id = ?", companyID, materialID).
		First(&out).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // material simply not assigned to this company
		}
		return nil, translate(err)
	}
	return &out, nil
}

func (r *materialRepository) UpsertCompanyMaterial(ctx context.Context, cm *model.CompanyMaterial) error {
	err := r.conn(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "company_id"}, {Name: "material_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"planning_enabled", "production_enabled", "inventory_enabled",
			"sales_enabled", "is_active", "changed_by", "changed_at",
		}),
	}).Create(cm).Error
	return translate(err)
}

// --- packaging -----------------------------------------------------------

type packagingRepository struct {
	crossRepo[model.PackagingType, *model.PackagingType]
}

func NewPackagingTypeRepository(db *gorm.DB) interfaces.PackagingTypeRepository {
	r := &packagingRepository{}
	r.db = db
	r.sortable = columns("packaging_code", "packaging_name", "nominal_quantity")
	r.searchable = []string{"packaging_code", "packaging_name"}
	return r
}

func (r *packagingRepository) FindByCode(ctx context.Context, code string) (*model.PackagingType, error) {
	return findOne[model.PackagingType](ctx, r.conn(ctx), "packaging_code = ?", code)
}

// --- warehouses ----------------------------------------------------------

type warehouseRepository struct {
	companyRepo[model.Warehouse, *model.Warehouse]
}

func NewWarehouseRepository(db *gorm.DB) interfaces.WarehouseRepository {
	r := &warehouseRepository{}
	r.db = db
	r.companyColumn = "company_id"
	r.sortable = columns("warehouse_code", "warehouse_name", "warehouse_type", "capacity", "location")
	r.searchable = []string{"warehouse_code", "warehouse_name"}
	return r
}

func (r *warehouseRepository) FindByCode(ctx context.Context, companyID int64, code string) (*model.Warehouse, error) {
	return findOne[model.Warehouse](ctx, r.conn(ctx), "company_id = ? AND warehouse_code = ?", companyID, code)
}

func (r *warehouseRepository) AllowedMaterials(ctx context.Context, warehouseID int64) ([]int64, error) {
	var ids []int64
	err := r.conn(ctx).Model(&model.WarehouseMaterial{}).
		Where("warehouse_id = ? AND is_active", warehouseID).
		Pluck("material_id", &ids).Error
	if err != nil {
		return nil, translate(err)
	}
	return ids, nil
}

func (r *warehouseRepository) SetAllowedMaterials(ctx context.Context, warehouseID int64, materialIDs []int64) error {
	conn := r.conn(ctx)
	if err := conn.Where("warehouse_id = ?", warehouseID).
		Delete(&model.WarehouseMaterial{}).Error; err != nil {
		return translate(err)
	}
	if len(materialIDs) == 0 {
		return nil
	}
	rows := make([]model.WarehouseMaterial, 0, len(materialIDs))
	for _, id := range materialIDs {
		row := model.WarehouseMaterial{WarehouseID: warehouseID, MaterialID: id}
		row.IsActive = true
		row.Version = 1
		rows = append(rows, row)
	}
	return translate(conn.Create(&rows).Error)
}

// --- production lines ----------------------------------------------------

type productionLineRepository struct {
	companyRepo[model.ProductionLine, *model.ProductionLine]
}

func NewProductionLineRepository(db *gorm.DB) interfaces.ProductionLineRepository {
	r := &productionLineRepository{}
	r.db = db
	r.companyColumn = "company_id"
	r.sortable = columns("line_code", "line_name", "capacity_per_day")
	r.searchable = []string{"line_code", "line_name"}
	return r
}

func (r *productionLineRepository) FindByCode(ctx context.Context, companyID int64, code string) (*model.ProductionLine, error) {
	return findOne[model.ProductionLine](ctx, r.conn(ctx), "company_id = ? AND line_code = ?", companyID, code)
}

// --- seasons -------------------------------------------------------------

type seasonRepository struct {
	companyRepo[model.Season, *model.Season]
}

func NewSeasonRepository(db *gorm.DB) interfaces.SeasonRepository {
	r := &seasonRepository{}
	r.db = db
	r.companyColumn = "company_id"
	r.sortable = columns("season_code", "season_name", "start_date", "end_date", "status")
	r.searchable = []string{"season_code", "season_name"}
	return r
}

func (r *seasonRepository) FindByCode(ctx context.Context, companyID int64, code string) (*model.Season, error) {
	return findOne[model.Season](ctx, r.conn(ctx), "company_id = ? AND season_code = ?", companyID, code)
}

func (r *seasonRepository) FindContaining(ctx context.Context, companyID int64, date time.Time) (*model.Season, error) {
	var out model.Season
	err := r.conn(ctx).
		Where("company_id = ? AND is_active AND ? BETWEEN start_date AND end_date",
			companyID, date.Format("2006-01-02")).
		First(&out).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, translate(err)
	}
	return &out, nil
}

// --- processes -----------------------------------------------------------

type processRepository struct {
	crossRepo[model.Process, *model.Process]
}

func NewProcessRepository(db *gorm.DB) interfaces.ProcessRepository {
	r := &processRepository{}
	r.db = db
	r.sortable = columns("process_code", "process_name", "sequence_no")
	r.searchable = []string{"process_code", "process_name"}
	return r
}

func (r *processRepository) FindByCode(ctx context.Context, code string) (*model.Process, error) {
	return findOne[model.Process](ctx, r.conn(ctx), "process_code = ?", code)
}

func (r *processRepository) Materials(ctx context.Context, processID int64) ([]model.ProcessMaterial, error) {
	var rows []model.ProcessMaterial
	err := r.conn(ctx).Preload("Material").
		Where("process_id = ? AND is_active", processID).
		Order("io_type, material_id").
		Find(&rows).Error
	if err != nil {
		return nil, translate(err)
	}
	return rows, nil
}

func (r *processRepository) SetMaterials(ctx context.Context, processID int64, materials []model.ProcessMaterial) error {
	conn := r.conn(ctx)
	if err := conn.Where("process_id = ?", processID).
		Delete(&model.ProcessMaterial{}).Error; err != nil {
		return translate(err)
	}
	if len(materials) == 0 {
		return nil
	}
	for i := range materials {
		materials[i].ProcessID = processID
		materials[i].IsActive = true
		materials[i].Version = 1
	}
	return translate(conn.Create(&materials).Error)
}

// --- movement types ------------------------------------------------------

type movementTypeRepository struct {
	crossRepo[model.MovementType, *model.MovementType]
}

func NewMovementTypeRepository(db *gorm.DB) interfaces.MovementTypeRepository {
	r := &movementTypeRepository{}
	r.db = db
	r.sortable = columns("movement_code", "movement_name", "direction")
	r.searchable = []string{"movement_code", "movement_name"}
	return r
}

func (r *movementTypeRepository) FindByCode(ctx context.Context, code string) (*model.MovementType, error) {
	return findOne[model.MovementType](ctx, r.conn(ctx), "movement_code = ?", code)
}

// --- number ranges (§35 / §F1) -------------------------------------------

type numberRangeRepository struct {
	db *gorm.DB
}

func NewNumberRangeRepository(db *gorm.DB) interfaces.NumberRangeRepository {
	return &numberRangeRepository{db: db}
}

// Allocate uses a single UPDATE ... RETURNING. The row-level lock it takes
// serialises concurrent allocators, which is what guarantees gap-free,
// duplicate-free document numbers under the 20-concurrent-poster requirement
// of Part I. A SELECT-then-UPDATE would race; a PostgreSQL sequence cannot be
// scoped per company and year.
func (r *numberRangeRepository) Allocate(ctx context.Context, companyID int64,
	objectType string, fiscalYear int) (*model.NumberRange, int64, error) {

	var row model.NumberRange
	err := database.Conn(ctx, r.db).Raw(`
		UPDATE number_ranges
		   SET current_no = current_no + 1,
		       changed_at = now()
		 WHERE company_id = ? AND object_type = ? AND fiscal_year = ? AND is_active
		RETURNING *`, companyID, objectType, fiscalYear).Scan(&row).Error
	if err != nil {
		return nil, 0, translate(err)
	}
	if row.ID == 0 {
		return nil, 0, nil // caller creates the range and retries
	}
	return &row, row.CurrentNo, nil
}

func (r *numberRangeRepository) EnsureRange(ctx context.Context, companyID int64,
	objectType string, fiscalYear int, prefix string) error {

	err := database.Conn(ctx, r.db).Exec(`
		INSERT INTO number_ranges (company_id, object_type, fiscal_year, prefix, current_no, length)
		VALUES (?, ?, ?, ?, 0, 6)
		ON CONFLICT (company_id, object_type, fiscal_year) DO NOTHING`,
		companyID, objectType, fiscalYear, prefix).Error
	return translate(err)
}

func (r *numberRangeRepository) List(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.NumberRange], error) {
	opts.Normalise()
	var page interfaces.Page[model.NumberRange]

	q := database.Conn(ctx, r.db).Model(&model.NumberRange{})
	if opts.CompanyID != nil {
		q = q.Where("company_id = ?", *opts.CompanyID)
	}
	if err := q.Count(&page.Total).Error; err != nil {
		return page, translate(err)
	}
	err := q.Order("company_id, object_type, fiscal_year").
		Limit(opts.Size).Offset(opts.Offset()).Find(&page.Rows).Error
	return page, translate(err)
}
