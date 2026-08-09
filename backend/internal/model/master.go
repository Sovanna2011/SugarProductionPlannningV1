package model

import (
	"time"

	"github.com/shopspring/decimal"
)

// --- units of measure ----------------------------------------------------

type UOM struct {
	Base
	UOMCode   string `gorm:"column:uom_code;size:10;not null;uniqueIndex"`
	UOMName   string `gorm:"column:uom_name;size:60;not null"`
	Dimension string `gorm:"column:dimension;size:20;not null"`
	Decimals  int    `gorm:"column:decimals;not null;default:3"`
}

func (UOM) TableName() string { return "uoms" }

// UOMConversion converts a quantity as qty * numerator / denominator.
type UOMConversion struct {
	Base
	FromUOMID   int64           `gorm:"column:from_uom_id;not null"`
	ToUOMID     int64           `gorm:"column:to_uom_id;not null"`
	Numerator   decimal.Decimal `gorm:"column:numerator;type:numeric(18,6);not null"`
	Denominator decimal.Decimal `gorm:"column:denominator;type:numeric(18,6);not null"`
}

func (UOMConversion) TableName() string { return "uom_conversions" }

// Convert applies the factor to a quantity.
func (c UOMConversion) Convert(qty decimal.Decimal) decimal.Decimal {
	return qty.Mul(c.Numerator).Div(c.Denominator)
}

// --- materials -----------------------------------------------------------

const (
	MaterialTypeRaw          = "RAW"
	MaterialTypeSemiFinished = "SEMI_FINISHED"
	MaterialTypeFinished     = "FINISHED"
	MaterialTypeByProduct    = "BY_PRODUCT"
	MaterialTypeUtility      = "UTILITY"
)

// Material is cross-company master data (§14). Company relevance lives in
// CompanyMaterial.
type Material struct {
	Base
	MaterialCode         string  `gorm:"column:material_code;size:40;not null;uniqueIndex"`
	MaterialName         string  `gorm:"column:material_name;size:200;not null"`
	MaterialType         string  `gorm:"column:material_type;size:20;not null"`
	MaterialGroup        *string `gorm:"column:material_group;size:40"`
	BaseUOMID            int64   `gorm:"column:base_uom_id;not null"`
	ConditioningRequired bool    `gorm:"column:conditioning_required;not null;default:false"`
	IsStockManaged       bool    `gorm:"column:is_stock_managed;not null;default:true"`

	BaseUOM *UOM `gorm:"foreignKey:BaseUOMID"`
}

func (Material) TableName() string { return "materials" }

type CompanyMaterial struct {
	Base
	CompanyID         int64 `gorm:"column:company_id;not null"`
	MaterialID        int64 `gorm:"column:material_id;not null"`
	PlanningEnabled   bool  `gorm:"column:planning_enabled;not null;default:true"`
	ProductionEnabled bool  `gorm:"column:production_enabled;not null;default:true"`
	InventoryEnabled  bool  `gorm:"column:inventory_enabled;not null;default:true"`
	SalesEnabled      bool  `gorm:"column:sales_enabled;not null;default:false"`

	Material *Material `gorm:"foreignKey:MaterialID"`
}

func (CompanyMaterial) TableName() string     { return "company_materials" }
func (c CompanyMaterial) GetCompanyID() int64 { return c.CompanyID }

// --- packaging -----------------------------------------------------------

type PackagingType struct {
	Base
	PackagingCode   string           `gorm:"column:packaging_code;size:20;not null;uniqueIndex"`
	PackagingName   string           `gorm:"column:packaging_name;size:100;not null"`
	NominalQuantity *decimal.Decimal `gorm:"column:nominal_quantity;type:numeric(18,3)"`
	NominalUOMID    *int64           `gorm:"column:nominal_uom_id"`
	IsBulk          bool             `gorm:"column:is_bulk;not null;default:false"`
}

func (PackagingType) TableName() string { return "packaging_types" }

// --- storage locations ---------------------------------------------------

const (
	WarehouseTypeWarehouse         = "WAREHOUSE"
	WarehouseTypeTank              = "TANK"
	WarehouseTypeSilo              = "SILO"
	WarehouseTypeProductionStorage = "PRODUCTION_STORAGE"
)

type Warehouse struct {
	Base
	CompanyID          int64            `gorm:"column:company_id;not null"`
	WarehouseCode      string           `gorm:"column:warehouse_code;size:20;not null"`
	WarehouseName      string           `gorm:"column:warehouse_name;size:200;not null"`
	WarehouseType      string           `gorm:"column:warehouse_type;size:30;not null"`
	Capacity           *decimal.Decimal `gorm:"column:capacity;type:numeric(18,3)"`
	CapacityUOMID      *int64           `gorm:"column:capacity_uom_id"`
	AllowNegativeStock bool             `gorm:"column:allow_negative_stock;not null;default:false"`
	Location           *string          `gorm:"column:location;size:200"`
}

func (Warehouse) TableName() string     { return "warehouses" }
func (w Warehouse) GetCompanyID() int64 { return w.CompanyID }

// HasCapacityLimit reports whether a capacity check applies. A NULL capacity
// means unlimited (§F5) — not zero.
func (w Warehouse) HasCapacityLimit() bool {
	return w.Capacity != nil && w.Capacity.IsPositive()
}

// WarehouseMaterial restricts a tank or silo to specific materials (OQ-4).
type WarehouseMaterial struct {
	Base
	WarehouseID int64 `gorm:"column:warehouse_id;not null"`
	MaterialID  int64 `gorm:"column:material_id;not null"`
}

func (WarehouseMaterial) TableName() string { return "warehouse_materials" }

type ProductionLine struct {
	Base
	CompanyID      int64            `gorm:"column:company_id;not null"`
	LineCode       string           `gorm:"column:line_code;size:20;not null"`
	LineName       string           `gorm:"column:line_name;size:200;not null"`
	CapacityPerDay *decimal.Decimal `gorm:"column:capacity_per_day;type:numeric(18,3)"`
	CapacityUOMID  *int64           `gorm:"column:capacity_uom_id"`
}

func (ProductionLine) TableName() string     { return "production_lines" }
func (p ProductionLine) GetCompanyID() int64 { return p.CompanyID }

// --- production processes (§19–§26 as data, §F7) -------------------------

type Process struct {
	Base
	ProcessCode string  `gorm:"column:process_code;size:30;not null;uniqueIndex"`
	ProcessName string  `gorm:"column:process_name;size:200;not null"`
	SequenceNo  int     `gorm:"column:sequence_no;not null;default:0"`
	Description *string `gorm:"column:description"`

	Materials []ProcessMaterial `gorm:"foreignKey:ProcessID"`
}

func (Process) TableName() string { return "processes" }

const (
	IOTypeInput  = "INPUT"
	IOTypeOutput = "OUTPUT"
)

type ProcessMaterial struct {
	Base
	ProcessID            int64            `gorm:"column:process_id;not null"`
	MaterialID           int64            `gorm:"column:material_id;not null"`
	IOType               string           `gorm:"column:io_type;size:10;not null"`
	RequiresConditioning bool             `gorm:"column:requires_conditioning;not null;default:false"`
	ExpectedYieldPct     *decimal.Decimal `gorm:"column:expected_yield_pct;type:numeric(9,4)"`

	Material *Material `gorm:"foreignKey:MaterialID"`
}

func (ProcessMaterial) TableName() string { return "process_materials" }

// --- movement types ------------------------------------------------------

const (
	DirectionIn  = "IN"
	DirectionOut = "OUT"
)

// MovementType drives the sign of every inventory movement. §33 forbids
// hard-coding movement codes in business logic — Direction is the contract.
type MovementType struct {
	Base
	MovementCode       string `gorm:"column:movement_code;size:30;not null;uniqueIndex"`
	MovementName       string `gorm:"column:movement_name;size:200;not null"`
	Direction          string `gorm:"column:direction;size:3;not null"`
	AffectsStock       bool   `gorm:"column:affects_stock;not null;default:true"`
	IsPlanningRelevant bool   `gorm:"column:is_planning_relevant;not null;default:true"`
	IsTransfer         bool   `gorm:"column:is_transfer;not null;default:false"`
	IsAdjustment       bool   `gorm:"column:is_adjustment;not null;default:false"`
	CounterpartID      *int64 `gorm:"column:counterpart_id"`
}

func (MovementType) TableName() string { return "movement_types" }

// Sign returns +1 for a receipt and -1 for an issue.
func (m MovementType) Sign() int {
	if m.Direction == DirectionOut {
		return -1
	}
	return 1
}

// --- seasons -------------------------------------------------------------

const (
	SeasonStatusPlanning = "PLANNING"
	SeasonStatusOpen     = "OPEN"
	SeasonStatusClosed   = "CLOSED"
)

type Season struct {
	Base
	CompanyID  int64     `gorm:"column:company_id;not null"`
	SeasonCode string    `gorm:"column:season_code;size:20;not null"`
	SeasonName string    `gorm:"column:season_name;size:200;not null"`
	StartDate  time.Time `gorm:"column:start_date;type:date;not null"`
	EndDate    time.Time `gorm:"column:end_date;type:date;not null"`
	Status     string    `gorm:"column:status;size:20;not null;default:PLANNING"`
}

func (Season) TableName() string     { return "seasons" }
func (s Season) GetCompanyID() int64 { return s.CompanyID }

// Contains reports whether a business date falls inside the season, bounds
// included.
func (s Season) Contains(d time.Time) bool {
	day := d.Truncate(24 * time.Hour)
	return !day.Before(s.StartDate.Truncate(24*time.Hour)) &&
		!day.After(s.EndDate.Truncate(24*time.Hour))
}
