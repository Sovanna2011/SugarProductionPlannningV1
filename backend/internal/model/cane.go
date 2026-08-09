package model

import (
	"time"

	"github.com/shopspring/decimal"
)

// Cane supply: the agricultural half of the plan. The mill plan says what
// each line will produce; this says where the cane comes from, on which day,
// and what actually arrived at the weighbridge.

const (
	GrowerTypeEstate     = "ESTATE"
	GrowerTypeOutGrower  = "OUT_GROWER"
	GrowerTypeContractor = "CONTRACTOR"
)

const (
	// SupplyTypeOwnEstate is cane off the company's own land: no cane bill.
	SupplyTypeOwnEstate = "OWN_ESTATE"
	// SupplyTypePurchased is cane bought in under a supply contract, which is
	// what carries a contracted tonnage and a price per tonne.
	SupplyTypePurchased = "PURCHASED"
)

const (
	CropCyclePlant   = "PLANT"
	CropCycleRatoon1 = "RATOON_1"
	CropCycleRatoon2 = "RATOON_2"
	CropCycleRatoon3 = "RATOON_3"
	CropCycleRatoon4 = "RATOON_4_PLUS"
)

// CaneVariety is cross-company master data, like a material (§14).
type CaneVariety struct {
	Base
	VarietyCode    string           `gorm:"column:variety_code;size:30;not null;uniqueIndex"`
	VarietyName    string           `gorm:"column:variety_name;size:200;not null"`
	MaturityMonths *int             `gorm:"column:maturity_months"`
	TypicalCCSPct  *decimal.Decimal `gorm:"column:typical_ccs_pct;type:numeric(9,4)"`
	Description    *string          `gorm:"column:description"`
}

func (CaneVariety) TableName() string { return "cane_varieties" }

// Grower is a cane supplier of one company: an own estate, an out-grower or a
// harvesting contractor.
type Grower struct {
	Base
	CompanyID    int64            `gorm:"column:company_id;not null"`
	GrowerCode   string           `gorm:"column:grower_code;size:20;not null"`
	GrowerName   string           `gorm:"column:grower_name;size:200;not null"`
	GrowerType   string           `gorm:"column:grower_type;size:20;not null;default:OUT_GROWER"`
	SupplyType   string           `gorm:"column:supply_type;size:20;not null;default:PURCHASED"`
	Zone         *string          `gorm:"column:zone;size:60"`
	ContactName  *string          `gorm:"column:contact_name;size:200"`
	ContactPhone *string          `gorm:"column:contact_phone;size:40"`
	ContractTons *decimal.Decimal `gorm:"column:contract_tons;type:numeric(18,3)"`
	ContractNo   *string          `gorm:"column:contract_no;size:40"`
	PricePerTon  *decimal.Decimal `gorm:"column:price_per_ton;type:numeric(18,4)"`
	Currency     *string          `gorm:"column:currency;size:3"`
	TransportKm  *decimal.Decimal `gorm:"column:transport_km;type:numeric(9,2)"`
}

func (Grower) TableName() string     { return "growers" }
func (g Grower) GetCompanyID() int64 { return g.CompanyID }

// IsPurchased reports whether cane from this grower is bought in rather than
// harvested off the company's own estate.
func (g Grower) IsPurchased() bool { return g.SupplyType == SupplyTypePurchased }

// CaneField is a plot. Area and expected yield give the tonnage a harvest
// plan starts from; both may be absent, in which case the planner enters
// tonnes directly.
type CaneField struct {
	Base
	CompanyID           int64            `gorm:"column:company_id;not null"`
	GrowerID            int64            `gorm:"column:grower_id;not null"`
	VarietyID           *int64           `gorm:"column:variety_id"`
	FieldCode           string           `gorm:"column:field_code;size:20;not null"`
	FieldName           string           `gorm:"column:field_name;size:200;not null"`
	AreaHa              decimal.Decimal  `gorm:"column:area_ha;type:numeric(12,3);not null"`
	ExpectedYieldTPH    *decimal.Decimal `gorm:"column:expected_yield_tph;type:numeric(12,3)"`
	CropCycle           string           `gorm:"column:crop_cycle;size:20;not null;default:PLANT"`
	PlantingDate        *time.Time       `gorm:"column:planting_date;type:date"`
	ExpectedHarvestFrom *time.Time       `gorm:"column:expected_harvest_from;type:date"`
	ExpectedHarvestTo   *time.Time       `gorm:"column:expected_harvest_to;type:date"`
	Zone                *string          `gorm:"column:zone;size:60"`
	IsIrrigated         bool             `gorm:"column:is_irrigated;not null;default:false"`
}

func (CaneField) TableName() string     { return "cane_fields" }
func (f CaneField) GetCompanyID() int64 { return f.CompanyID }

// EstimatedTons is area × expected yield. It returns nil rather than zero
// when no yield is maintained: an unknown estimate is not an estimate of
// nothing (§F5's null rule, applied to agronomy).
func (f CaneField) EstimatedTons() *decimal.Decimal {
	if f.ExpectedYieldTPH == nil {
		return nil
	}
	tons := f.AreaHa.Mul(*f.ExpectedYieldTPH).Round(3)
	return &tons
}

// HarvestPlanHeader is the cane half of a planning version. There is exactly
// one per version: the matrix spans every grower, so there is nothing to
// split it by.
type HarvestPlanHeader struct {
	Base
	CompanyID         int64     `gorm:"column:company_id;not null"`
	SeasonID          int64     `gorm:"column:season_id;not null"`
	PlanningVersionID int64     `gorm:"column:planning_version_id;not null"`
	DocumentNo        string    `gorm:"column:document_no;size:40;not null"`
	Description       *string   `gorm:"column:description;size:255"`
	DateFrom          time.Time `gorm:"column:date_from;type:date;not null"`
	DateTo            time.Time `gorm:"column:date_to;type:date;not null"`

	Items []HarvestPlanItem `gorm:"foreignKey:HarvestPlanHeaderID"`
}

func (HarvestPlanHeader) TableName() string     { return "harvest_plan_headers" }
func (h HarvestPlanHeader) GetCompanyID() int64 { return h.CompanyID }

// HarvestPlanItem is one cell of the Date × Grower grid. Its business key
// (header, date, grower, field) is what makes the grid round-trippable, the
// same contract the production matrix has (§28).
type HarvestPlanItem struct {
	Base
	HarvestPlanHeaderID int64            `gorm:"column:harvest_plan_header_id;not null"`
	PlanDate            time.Time        `gorm:"column:plan_date;type:date;not null"`
	GrowerID            int64            `gorm:"column:grower_id;not null"`
	CaneFieldID         *int64           `gorm:"column:cane_field_id"`
	VarietyID           *int64           `gorm:"column:variety_id"`
	PlannedTons         decimal.Decimal  `gorm:"column:planned_tons;type:numeric(18,3);not null"`
	UOMID               int64            `gorm:"column:uom_id;not null"`
	PlannedAreaHa       *decimal.Decimal `gorm:"column:planned_area_ha;type:numeric(12,3)"`
	ExpectedCCSPct      *decimal.Decimal `gorm:"column:expected_ccs_pct;type:numeric(9,4)"`
	Remark              *string          `gorm:"column:remark;size:500"`
}

func (HarvestPlanItem) TableName() string { return "harvest_plan_items" }

// CaneDelivery is a weighbridge ticket — the actual against the harvest plan.
// Posting one writes an ordinary inventory movement for the net weight, so
// cane needs no special case anywhere in the stock ledger.
type CaneDelivery struct {
	Base
	CompanyID      int64            `gorm:"column:company_id;not null"`
	SeasonID       *int64           `gorm:"column:season_id"`
	DocumentNo     string           `gorm:"column:document_no;size:40;not null"`
	DeliveryDate   time.Time        `gorm:"column:delivery_date;type:date;not null"`
	GrowerID       int64            `gorm:"column:grower_id;not null"`
	CaneFieldID    *int64           `gorm:"column:cane_field_id"`
	VarietyID      *int64           `gorm:"column:variety_id"`
	MaterialID     int64            `gorm:"column:material_id;not null"`
	MovementTypeID int64            `gorm:"column:movement_type_id;not null"`
	WarehouseID    *int64           `gorm:"column:warehouse_id"`
	UOMID          int64            `gorm:"column:uom_id;not null"`
	TicketNo       *string          `gorm:"column:ticket_no;size:40"`
	VehicleNo      *string          `gorm:"column:vehicle_no;size:30"`
	GrossTons      decimal.Decimal  `gorm:"column:gross_tons;type:numeric(18,3);not null"`
	TareTons       decimal.Decimal  `gorm:"column:tare_tons;type:numeric(18,3);not null"`
	// NetTons is a generated column: the database derives it from gross and
	// tare, so no code path can leave the three disagreeing.
	NetTons        decimal.Decimal  `gorm:"column:net_tons;type:numeric(18,3);->"`
	PricePerTon    *decimal.Decimal `gorm:"column:price_per_ton;type:numeric(18,4)"`
	Currency       *string          `gorm:"column:currency;size:3"`
	CCSPct         *decimal.Decimal `gorm:"column:ccs_pct;type:numeric(9,4)"`
	TrashPct       *decimal.Decimal `gorm:"column:trash_pct;type:numeric(9,4)"`
	IsBurnt        bool             `gorm:"column:is_burnt;not null;default:false"`
	PostingStatus  string           `gorm:"column:posting_status;size:20;not null;default:DRAFT"`
	PostedBy       *int64           `gorm:"column:posted_by"`
	PostedAt       *time.Time       `gorm:"column:posted_at"`
	ReversedBy     *int64           `gorm:"column:reversed_by"`
	ReversedAt     *time.Time       `gorm:"column:reversed_at"`
	IdempotencyKey *string          `gorm:"column:idempotency_key;size:80"`
	Remark         *string          `gorm:"column:remark;size:500"`
}

func (CaneDelivery) TableName() string     { return "cane_deliveries" }
func (d CaneDelivery) GetCompanyID() int64 { return d.CompanyID }

// Value is net tonnage × contracted price. It is nil when no price was
// agreed — an unpriced load has an unknown value, not a value of zero.
func (d CaneDelivery) Value() *decimal.Decimal {
	if d.PricePerTon == nil {
		return nil
	}
	amount := d.NetTons.Mul(*d.PricePerTon).Round(2)
	return &amount
}
