package interfaces

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
)

// --- cane master data ----------------------------------------------------

type CaneVarietyRepository interface {
	CrossCompanyRepository[model.CaneVariety]
	FindByCode(ctx context.Context, code string) (*model.CaneVariety, error)
}

type GrowerRepository interface {
	CompanyRepository[model.Grower]
	FindByCode(ctx context.Context, companyID int64, code string) (*model.Grower, error)
}

type CaneFieldRepository interface {
	CompanyRepository[model.CaneField]
	FindByCode(ctx context.Context, companyID int64, code string) (*model.CaneField, error)
	// ListForGrower backs the value help of the harvest matrix, which offers a
	// grower's fields once a column has been picked.
	ListForGrower(ctx context.Context, companyID, growerID int64) ([]model.CaneField, error)
}

// --- harvest plan --------------------------------------------------------

type HarvestPlanRepository interface {
	CreateHeader(ctx context.Context, header *model.HarvestPlanHeader) error
	UpdateHeader(ctx context.Context, companyID int64, header *model.HarvestPlanHeader) error
	FindHeader(ctx context.Context, companyID, headerID int64) (*model.HarvestPlanHeader, error)
	FindHeaderByVersion(ctx context.Context, companyID, versionID int64) (*model.HarvestPlanHeader, error)
	ListHeaders(ctx context.Context, opts ListOptions, versionID *int64) (Page[model.HarvestPlanHeader], error)
	Items(ctx context.Context, headerID int64) ([]model.HarvestPlanItem, error)
	// UpsertItems is keyed on (header, date, grower, field), so replaying a
	// payload reproduces the grid instead of duplicating it.
	UpsertItems(ctx context.Context, headerID int64, items []model.HarvestPlanItem) error
	DeleteItems(ctx context.Context, headerID int64, itemIDs []int64) error
	// MatrixCells reads the Date × Grower grid in one query.
	MatrixCells(ctx context.Context, companyID, versionID int64, from, to time.Time) ([]HarvestMatrixCell, error)
}

// HarvestMatrixCell is one editable cell of the harvest matrix.
type HarvestMatrixCell struct {
	HarvestPlanHeaderID int64
	HarvestPlanItemID   int64
	PlanDate            time.Time
	GrowerID            int64
	CaneFieldID         *int64
	VarietyID           *int64
	PlannedTons         decimal.Decimal
	PlannedAreaHa       *decimal.Decimal
	ExpectedCCSPct      *decimal.Decimal
	UOMID               int64
	Version             int
}

// --- cane deliveries -----------------------------------------------------

type CaneDeliveryRepository interface {
	Create(ctx context.Context, delivery *model.CaneDelivery) error
	Update(ctx context.Context, companyID int64, delivery *model.CaneDelivery) error
	FindByID(ctx context.Context, companyID, id int64) (*model.CaneDelivery, error)
	FindByIdempotencyKey(ctx context.Context, companyID int64, key string) (*model.CaneDelivery, error)
	List(ctx context.Context, opts ListOptions, f DeliveryFilter) (Page[model.CaneDelivery], error)
	SetStatus(ctx context.Context, delivery *model.CaneDelivery) error
}

type DeliveryFilter struct {
	GrowerID    *int64
	CaneFieldID *int64
	VarietyID   *int64
	Status      string
	From        *time.Time
	To          *time.Time
}

// --- cane reporting ------------------------------------------------------

// CaneReportRepository answers the cane half of §F6: planned harvest against
// what the weighbridge actually received.
type CaneReportRepository interface {
	CanePlanVsActual(ctx context.Context, q CanePlanVsActualQuery) ([]CanePlanVsActualRow, error)
}

type CanePlanVsActualQuery struct {
	CompanyID int64
	VersionID *int64
	Range     DateRange
	// GroupBy is grower | variety | date | zone | supplyType.
	GroupBy string
	// SupplyType narrows the report to purchased or own-estate cane. Empty
	// means both.
	SupplyType string
}

type CanePlanVsActualRow struct {
	GroupKey   string
	GroupLabel string
	SupplyType string
	GrowerID   *int64
	GrowerCode *string
	VarietyID  *int64
	VarietyCode *string
	PlanDate   *time.Time
	PlannedTons decimal.Decimal
	ActualTons  decimal.Decimal
	Deliveries  int64
	// AverageCCSPct is nil when no delivery carried a cane analysis — an
	// unmeasured quality is not a quality of zero.
	AverageCCSPct *decimal.Decimal
	// PurchaseValue is nil when any contributing delivery had no agreed price.
	PurchaseValue *decimal.Decimal
	Currency      *string
}
