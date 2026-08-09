package interfaces

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
)

// CrossCompanyRepository is the CRUD contract for master data that is shared
// by all companies (materials, UOMs, processes, movement types, packaging).
type CrossCompanyRepository[T any] interface {
	Create(ctx context.Context, entity *T) error
	Update(ctx context.Context, entity *T) error
	FindByID(ctx context.Context, id int64) (*T, error)
	List(ctx context.Context, opts ListOptions) (Page[T], error)
	Deactivate(ctx context.Context, id int64, version int) error
}

// CompanyRepository is the CRUD contract for company-dependent master data.
// Every method takes companyID explicitly (§C2) — an implementation that
// ignores it fails code review.
type CompanyRepository[T any] interface {
	Create(ctx context.Context, entity *T) error
	Update(ctx context.Context, companyID int64, entity *T) error
	FindByID(ctx context.Context, companyID, id int64) (*T, error)
	List(ctx context.Context, opts ListOptions) (Page[T], error)
	Deactivate(ctx context.Context, companyID, id int64, version int) error
}

// --- security ------------------------------------------------------------

type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	Update(ctx context.Context, user *model.User) error
	FindByID(ctx context.Context, id int64) (*model.User, error)
	FindByUsername(ctx context.Context, username string) (*model.User, error)
	List(ctx context.Context, opts ListOptions) (Page[model.User], error)
	RecordLoginSuccess(ctx context.Context, userID int64, at time.Time) error
	RecordLoginFailure(ctx context.Context, userID int64, maxFailures int) error
	UpdatePassword(ctx context.Context, userID int64, passwordHash string, mustChange bool) error
}

type CompanyRepositoryIface interface {
	CrossCompanyRepository[model.Company]
	FindByCode(ctx context.Context, code string) (*model.Company, error)
}

// AuthorizationRepository backs the pipeline of §C2.
type AuthorizationRepository interface {
	// IsUserAuthorisedForCompany is step 3 of the pipeline.
	IsUserAuthorisedForCompany(ctx context.Context, userID, companyID int64) (bool, error)
	// PermissionsFor resolves roles → permissions for exactly one company.
	PermissionsFor(ctx context.Context, userID, companyID int64) ([]string, error)
	// CompaniesFor drives GET /auth/me and the UI5 company drop-down.
	CompaniesFor(ctx context.Context, userID int64) ([]model.UserCompany, error)
	RolesFor(ctx context.Context, userID, companyID int64) ([]model.Role, error)
	AssignCompany(ctx context.Context, uc *model.UserCompany) error
	AssignRole(ctx context.Context, ucr *model.UserCompanyRole) error
	RemoveRole(ctx context.Context, userID, companyID, roleID int64) error
	FindRoleByCode(ctx context.Context, code string) (*model.Role, error)
	ListRoles(ctx context.Context, opts ListOptions) (Page[model.Role], error)
	ListPermissions(ctx context.Context, opts ListOptions) (Page[model.Permission], error)
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, token *model.RefreshToken) error
	FindByHash(ctx context.Context, hash string) (*model.RefreshToken, error)
	Revoke(ctx context.Context, id int64, replacedBy *int64) error
	RevokeAllForUser(ctx context.Context, userID int64) error
	DeleteExpired(ctx context.Context, before time.Time) (int64, error)
}

// --- master data ---------------------------------------------------------

type MaterialRepository interface {
	CrossCompanyRepository[model.Material]
	FindByCode(ctx context.Context, code string) (*model.Material, error)
	// ListForCompany returns materials joined with their company relevance
	// flags, which is what the planning and actual value helps need.
	ListForCompany(ctx context.Context, companyID int64, opts ListOptions) (Page[model.CompanyMaterial], error)
	FindCompanyMaterial(ctx context.Context, companyID, materialID int64) (*model.CompanyMaterial, error)
	UpsertCompanyMaterial(ctx context.Context, cm *model.CompanyMaterial) error
}

type WarehouseRepository interface {
	CompanyRepository[model.Warehouse]
	FindByCode(ctx context.Context, companyID int64, code string) (*model.Warehouse, error)
	// AllowedMaterials is empty when the location accepts every material.
	AllowedMaterials(ctx context.Context, warehouseID int64) ([]int64, error)
	SetAllowedMaterials(ctx context.Context, warehouseID int64, materialIDs []int64) error
}

type ProductionLineRepository interface {
	CompanyRepository[model.ProductionLine]
	FindByCode(ctx context.Context, companyID int64, code string) (*model.ProductionLine, error)
}

type SeasonRepository interface {
	CompanyRepository[model.Season]
	FindByCode(ctx context.Context, companyID int64, code string) (*model.Season, error)
	// FindContaining resolves the season a business date falls into.
	FindContaining(ctx context.Context, companyID int64, date time.Time) (*model.Season, error)
}

type ProcessRepository interface {
	CrossCompanyRepository[model.Process]
	FindByCode(ctx context.Context, code string) (*model.Process, error)
	// Materials backs the routing validation of §F7.
	Materials(ctx context.Context, processID int64) ([]model.ProcessMaterial, error)
	SetMaterials(ctx context.Context, processID int64, materials []model.ProcessMaterial) error
}

type MovementTypeRepository interface {
	CrossCompanyRepository[model.MovementType]
	FindByCode(ctx context.Context, code string) (*model.MovementType, error)
}

type UOMRepository interface {
	CrossCompanyRepository[model.UOM]
	FindByCode(ctx context.Context, code string) (*model.UOM, error)
	// FindConversion returns the factor between two units, or nil when the
	// pair is not maintained.
	FindConversion(ctx context.Context, fromUOMID, toUOMID int64) (*model.UOMConversion, error)
}

type PackagingTypeRepository interface {
	CrossCompanyRepository[model.PackagingType]
	FindByCode(ctx context.Context, code string) (*model.PackagingType, error)
}

// --- planning ------------------------------------------------------------

type PlanningVersionRepository interface {
	CompanyRepository[model.PlanningVersion]
	FindBySeason(ctx context.Context, companyID, seasonID int64) ([]model.PlanningVersion, error)
	ExistsVersionNo(ctx context.Context, companyID, seasonID int64, versionNo int) (bool, error)
	NextVersionNo(ctx context.Context, companyID, seasonID int64) (int, error)
	// LatestApproved resolves "latest approved version" of §F6.
	LatestApproved(ctx context.Context, companyID, seasonID int64) (*model.PlanningVersion, error)
	UpdateStatus(ctx context.Context, version *model.PlanningVersion) error
	// DeepCopy performs the set-based copy of §F2 inside the caller's
	// transaction: headers and items are duplicated in two statements using a
	// CTE that maps old header ids to new ones.
	DeepCopy(ctx context.Context, sourceVersionID, targetVersionID int64, docNos map[int64]string) error
}

type PlanRepository interface {
	CreateHeader(ctx context.Context, header *model.PlanHeader) error
	UpdateHeader(ctx context.Context, companyID int64, header *model.PlanHeader) error
	FindHeader(ctx context.Context, companyID, headerID int64) (*model.PlanHeader, error)
	FindHeaderByKey(ctx context.Context, companyID, versionID, movementTypeID int64, lineID *int64) (*model.PlanHeader, error)
	ListHeaders(ctx context.Context, opts ListOptions, versionID, movementTypeID *int64) (Page[model.PlanHeader], error)
	Items(ctx context.Context, headerID int64) ([]model.PlanItem, error)
	// UpsertItems is the idempotent bulk save behind PUT /plans/{id}/items and
	// the matrix endpoint. It is keyed on
	// (header, date, line, material, process).
	UpsertItems(ctx context.Context, headerID int64, items []model.PlanItem) error
	DeleteItems(ctx context.Context, headerID int64, itemIDs []int64) error
	// MatrixCells reads the Date × Line grid of §28 in one query.
	MatrixCells(ctx context.Context, companyID, versionID, movementTypeID, materialID int64,
		processID *int64, from, to time.Time) ([]MatrixCell, error)
}

// MatrixCell is one editable cell of the planning matrix.
type MatrixCell struct {
	PlanHeaderID     int64
	PlanItemID       int64
	PlanDate         time.Time
	ProductionLineID *int64
	Quantity         decimal.Decimal
	UOMID            int64
	Version          int
}

// --- actual --------------------------------------------------------------

type ActualRepository interface {
	CreateHeader(ctx context.Context, header *model.ActualHeader) error
	UpdateHeader(ctx context.Context, companyID int64, header *model.ActualHeader) error
	FindHeader(ctx context.Context, companyID, headerID int64) (*model.ActualHeader, error)
	FindByIdempotencyKey(ctx context.Context, companyID int64, key string) (*model.ActualHeader, error)
	ListHeaders(ctx context.Context, opts ListOptions, from, to *time.Time, movementTypeID, lineID *int64) (Page[model.ActualHeader], error)
	Items(ctx context.Context, headerID int64) ([]model.ActualItem, error)
	ReplaceItems(ctx context.Context, headerID int64, items []model.ActualItem) error
	SetStatus(ctx context.Context, header *model.ActualHeader) error
}

// --- inventory -----------------------------------------------------------

type InventoryRepository interface {
	CreateMovements(ctx context.Context, movements []model.InventoryMovement) error
	FindMovement(ctx context.Context, companyID, id int64) (*model.InventoryMovement, error)
	ListMovements(ctx context.Context, opts ListOptions, f MovementFilter) (Page[model.InventoryMovement], error)
	MarkReversed(ctx context.Context, companyID, movementID int64) error
	NextLineNo(ctx context.Context, companyID int64, documentNo string) (int, error)

	// LockBalance selects the balance row FOR UPDATE, serialising concurrent
	// postings for one stock key without a table-level lock (§F4).
	LockBalance(ctx context.Context, key BalanceKey) (*model.InventoryBalance, error)
	SaveBalance(ctx context.Context, balance *model.InventoryBalance) error
	// CurrentStock is the running closing quantity as of a date.
	CurrentStock(ctx context.Context, companyID, warehouseID, materialID int64, packagingTypeID *int64, asOf time.Time) (decimal.Decimal, error)
	// WarehouseStock aggregates every material of a location, expressed in the
	// warehouse's capacity unit, which is what the capacity check compares
	// against warehouses.capacity. A material whose base unit cannot be
	// converted to that unit is reported rather than silently added up.
	WarehouseStock(ctx context.Context, companyID, warehouseID, targetUOMID int64, asOf time.Time) (WarehouseStock, error)
	ListBalances(ctx context.Context, companyID int64, warehouseID, materialID *int64, asOf time.Time) ([]BalanceRow, error)
	// RecomputeBalances rebuilds the materialised balances from the ledger and
	// reports the rows that disagreed — the nightly reconciliation job.
	RecomputeBalances(ctx context.Context, companyID int64, from, to time.Time) ([]BalanceDiscrepancy, error)
}

// WarehouseStock is the total held in a location, expressed in one unit.
type WarehouseStock struct {
	Total             decimal.Decimal
	ConversionMissing bool
}

type BalanceKey struct {
	CompanyID       int64
	WarehouseID     int64
	MaterialID      int64
	PackagingTypeID *int64
	BalanceDate     time.Time
}

type BalanceRow struct {
	CompanyID       int64
	WarehouseID     int64
	WarehouseCode   string
	WarehouseName   string
	MaterialID      int64
	MaterialCode    string
	MaterialName    string
	PackagingTypeID *int64
	UOMCode         string
	OpeningQty      decimal.Decimal
	InQty           decimal.Decimal
	OutQty          decimal.Decimal
	ClosingQty      decimal.Decimal
}

type BalanceDiscrepancy struct {
	Key      BalanceKey
	Stored   decimal.Decimal
	Computed decimal.Decimal
}

type MovementFilter struct {
	WarehouseID    *int64
	MaterialID     *int64
	MovementTypeID *int64
	DocumentNo     string
	From           *time.Time
	To             *time.Time
}

// --- number ranges (§35) -------------------------------------------------

type NumberRangeRepository interface {
	// Allocate atomically increments and returns the next sequence number.
	// It must run inside the caller's transaction and must use
	// UPDATE ... RETURNING, never SELECT-then-UPDATE.
	Allocate(ctx context.Context, companyID int64, objectType string, fiscalYear int) (*model.NumberRange, int64, error)
	EnsureRange(ctx context.Context, companyID int64, objectType string, fiscalYear int, prefix string) error
	List(ctx context.Context, opts ListOptions) (Page[model.NumberRange], error)
}

// --- audit ---------------------------------------------------------------

type AuditRepository interface {
	Write(ctx context.Context, entry *model.AuditLog) error
	List(ctx context.Context, opts ListOptions, tableName string, recordID *int64) (Page[model.AuditLog], error)
}

// --- reporting (§F6) -----------------------------------------------------

type ReportRepository interface {
	PlanVsActual(ctx context.Context, q PlanVsActualQuery) ([]PlanVsActualRow, error)
	ProductionSummary(ctx context.Context, companyID int64, r DateRange) ([]ProductionSummaryRow, error)
	InventoryMovementReport(ctx context.Context, companyID int64, r DateRange, warehouseID, materialID *int64) ([]MovementReportRow, error)
	CapacityUtilisation(ctx context.Context, companyID int64, asOf time.Time) ([]CapacityRow, error)
}

type PlanVsActualQuery struct {
	CompanyIDs []int64
	SeasonID   *int64
	VersionID  *int64
	Range      DateRange
	GroupBy    string // material | line | process | date
}

type PlanVsActualRow struct {
	CompanyID        int64
	CompanyCode      string
	GroupKey         string
	GroupLabel       string
	MaterialID       *int64
	MaterialCode     *string
	ProductionLineID *int64
	LineCode         *string
	ProcessID        *int64
	ProcessCode      *string
	PlanDate         *time.Time
	UOMCode          string
	PlanQty          decimal.Decimal
	ActualQty        decimal.Decimal
	// ConversionMissing marks a row whose quantities could not all be brought
	// to the material's base unit, so the variance is not trustworthy (§F6).
	ConversionMissing bool
}

type ProductionSummaryRow struct {
	MaterialID   int64
	MaterialCode string
	MaterialName string
	ProcessCode  *string
	UOMCode      string
	Quantity     decimal.Decimal
}

type MovementReportRow struct {
	TransactionDate time.Time
	DocumentNo      string
	WarehouseCode   string
	MaterialCode    string
	MovementCode    string
	Direction       string
	Quantity        decimal.Decimal
	UOMCode         string
	SourceModule    string
	IsReversed      bool
}

type CapacityRow struct {
	WarehouseID   int64
	WarehouseCode string
	WarehouseName string
	WarehouseType string
	Capacity      *decimal.Decimal
	CapacityUOM   *string
	CurrentStock  decimal.Decimal
}

// --- data dictionary & browser (Part G) ----------------------------------

type DataDictionaryRepository interface {
	ListTables(ctx context.Context, module, search string, browsableOnly bool) ([]model.DDTable, error)
	FindTable(ctx context.Context, tableName string) (*model.DDTable, error)
	UpdateTable(ctx context.Context, table *model.DDTable) error
	Fields(ctx context.Context, tableID int64) ([]model.DDField, error)
	FindField(ctx context.Context, id int64) (*model.DDField, error)
	UpdateField(ctx context.Context, field *model.DDField) error
	ListDomains(ctx context.Context) ([]model.DDDomain, error)
	FindValueHelp(ctx context.Context, id int64) (*model.DDValueHelp, error)
	// SyncFromInformationSchema regenerates dictionary entries from the live
	// catalogue, preserving human-maintained descriptions (§G1).
	SyncFromInformationSchema(ctx context.Context) (int, error)
}

// BrowserRepository executes the metadata-driven query of §G2. The table and
// every field name have already been validated against the dictionary; values
// are always bound parameters.
type BrowserRepository interface {
	Query(ctx context.Context, q BrowserQuery) (BrowserResult, error)
}

type BrowserQuery struct {
	TableName string
	Fields    []string
	Filters   []Filter
	Sort      []SortSpec
	Page      int
	Size      int
	// CompanyIDs is injected server-side for a company-dependent table and
	// cannot be influenced by the caller (§G2 rule 5).
	CompanyIDs []int64
	RowCap     int
	Timeout    time.Duration
}

type BrowserResult struct {
	Fields []string
	Rows   []map[string]any
	Total  int64
}
