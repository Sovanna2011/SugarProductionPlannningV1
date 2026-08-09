package dto

import "time"

// --- cane varieties ------------------------------------------------------

type CaneVarietyRequest struct {
	VarietyCode    string    `json:"varietyCode" binding:"required,max=30"`
	VarietyName    string    `json:"varietyName" binding:"required,max=200"`
	MaturityMonths *int      `json:"maturityMonths" binding:"omitempty,min=1,max=36"`
	TypicalCCSPct  *Quantity `json:"typicalCcsPct"`
	Description    *string   `json:"description"`
	Version        int       `json:"version"`
}

type CaneVarietyResponse struct {
	ID             int64     `json:"id"`
	VarietyCode    string    `json:"varietyCode"`
	VarietyName    string    `json:"varietyName"`
	MaturityMonths *int      `json:"maturityMonths,omitempty"`
	TypicalCCSPct  *Quantity `json:"typicalCcsPct,omitempty"`
	Description    *string   `json:"description,omitempty"`
	AuditFields
}

// --- growers -------------------------------------------------------------

type GrowerRequest struct {
	GrowerCode string `json:"growerCode" binding:"required,max=20"`
	GrowerName string `json:"growerName" binding:"required,max=200"`
	GrowerType string `json:"growerType" binding:"required,oneof=ESTATE OUT_GROWER CONTRACTOR"`
	// SupplyType separates purchased cane, which carries a contract and a
	// price, from the company's own estate cane, which does not.
	SupplyType   string    `json:"supplyType" binding:"required,oneof=OWN_ESTATE PURCHASED"`
	Zone         *string   `json:"zone" binding:"omitempty,max=60"`
	ContactName  *string   `json:"contactName" binding:"omitempty,max=200"`
	ContactPhone *string   `json:"contactPhone" binding:"omitempty,max=40"`
	ContractTons *Quantity `json:"contractTons"`
	ContractNo   *string   `json:"contractNo" binding:"omitempty,max=40"`
	PricePerTon  *Quantity `json:"pricePerTon"`
	Currency     *string   `json:"currency" binding:"omitempty,len=3"`
	TransportKm  *Quantity `json:"transportKm"`
	Version      int       `json:"version"`
}

type GrowerResponse struct {
	ID           int64     `json:"id"`
	CompanyID    int64     `json:"companyId"`
	GrowerCode   string    `json:"growerCode"`
	GrowerName   string    `json:"growerName"`
	GrowerType   string    `json:"growerType"`
	SupplyType   string    `json:"supplyType"`
	Zone         *string   `json:"zone,omitempty"`
	ContactName  *string   `json:"contactName,omitempty"`
	ContactPhone *string   `json:"contactPhone,omitempty"`
	ContractTons *Quantity `json:"contractTons,omitempty"`
	ContractNo   *string   `json:"contractNo,omitempty"`
	PricePerTon  *Quantity `json:"pricePerTon,omitempty"`
	Currency     *string   `json:"currency,omitempty"`
	TransportKm  *Quantity `json:"transportKm,omitempty"`
	AuditFields
}

// --- cane fields ---------------------------------------------------------

type CaneFieldRequest struct {
	GrowerID            int64     `json:"growerId" binding:"required"`
	VarietyID           *int64    `json:"varietyId"`
	FieldCode           string    `json:"fieldCode" binding:"required,max=20"`
	FieldName           string    `json:"fieldName" binding:"required,max=200"`
	AreaHa              Quantity  `json:"areaHa" binding:"required"`
	ExpectedYieldTPH    *Quantity `json:"expectedYieldTph"`
	CropCycle           string    `json:"cropCycle" binding:"required,oneof=PLANT RATOON_1 RATOON_2 RATOON_3 RATOON_4_PLUS"`
	PlantingDate        *Date     `json:"plantingDate"`
	ExpectedHarvestFrom *Date     `json:"expectedHarvestFrom"`
	ExpectedHarvestTo   *Date     `json:"expectedHarvestTo"`
	Zone                *string   `json:"zone" binding:"omitempty,max=60"`
	IsIrrigated         bool      `json:"isIrrigated"`
	Version             int       `json:"version"`
}

type CaneFieldResponse struct {
	ID                  int64     `json:"id"`
	CompanyID           int64     `json:"companyId"`
	GrowerID            int64     `json:"growerId"`
	VarietyID           *int64    `json:"varietyId,omitempty"`
	FieldCode           string    `json:"fieldCode"`
	FieldName           string    `json:"fieldName"`
	AreaHa              Quantity  `json:"areaHa"`
	ExpectedYieldTPH    *Quantity `json:"expectedYieldTph,omitempty"`
	// EstimatedTons is area × expected yield. It is absent, not zero, when the
	// field has no yield maintained yet.
	EstimatedTons       *Quantity `json:"estimatedTons,omitempty"`
	CropCycle           string    `json:"cropCycle"`
	PlantingDate        *Date     `json:"plantingDate,omitempty"`
	ExpectedHarvestFrom *Date     `json:"expectedHarvestFrom,omitempty"`
	ExpectedHarvestTo   *Date     `json:"expectedHarvestTo,omitempty"`
	Zone                *string   `json:"zone,omitempty"`
	IsIrrigated         bool      `json:"isIrrigated"`
	AuditFields
}

// --- the harvest matrix --------------------------------------------------

// HarvestValueRequest is one cell. A null plannedTons means "leave
// unchanged"; omitting the cell means "delete" unless partialUpdate is set.
type HarvestValueRequest struct {
	GrowerID       int64     `json:"growerId" binding:"required"`
	CaneFieldID    *int64    `json:"caneFieldId"`
	VarietyID      *int64    `json:"varietyId"`
	PlannedTons    *Quantity `json:"plannedTons"`
	PlannedAreaHa  *Quantity `json:"plannedAreaHa"`
	ExpectedCCSPct *Quantity `json:"expectedCcsPct"`
	Remark         *string   `json:"remark" binding:"omitempty,max=500"`
}

type HarvestRowRequest struct {
	PlanDate Date                  `json:"planDate" binding:"required"`
	Values   []HarvestValueRequest `json:"values" binding:"required"`
}

type HarvestMatrixSaveRequest struct {
	VersionID     int64               `json:"versionId" binding:"required"`
	UOMID         int64               `json:"uomId"`
	DateFrom      Date                `json:"dateFrom" binding:"required"`
	DateTo        Date                `json:"dateTo" binding:"required"`
	Rows          []HarvestRowRequest `json:"rows" binding:"required"`
	PartialUpdate bool                `json:"partialUpdate"`
}

type HarvestGrowerResponse struct {
	GrowerID   int64  `json:"growerId"`
	GrowerCode string `json:"growerCode"`
	GrowerName string `json:"growerName"`
	SupplyType string `json:"supplyType"`
	Zone       *string `json:"zone,omitempty"`
}

type HarvestValueResponse struct {
	GrowerID       int64     `json:"growerId"`
	CaneFieldID    *int64    `json:"caneFieldId,omitempty"`
	VarietyID      *int64    `json:"varietyId,omitempty"`
	PlannedTons    Quantity  `json:"plannedTons"`
	PlannedAreaHa  *Quantity `json:"plannedAreaHa,omitempty"`
	ExpectedCCSPct *Quantity `json:"expectedCcsPct,omitempty"`
	UOMID          int64     `json:"uomId"`
	Version        int       `json:"version"`
}

type HarvestRowResponse struct {
	PlanDate Date                   `json:"planDate"`
	Values   []HarvestValueResponse `json:"values"`
}

type HarvestMatrixResponse struct {
	CompanyID     int64                   `json:"companyId"`
	VersionID     int64                   `json:"versionId"`
	VersionStatus string                  `json:"versionStatus"`
	DocumentNo    string                  `json:"documentNo,omitempty"`
	DateFrom      Date                    `json:"dateFrom"`
	DateTo        Date                    `json:"dateTo"`
	Growers       []HarvestGrowerResponse `json:"growers"`
	Rows          []HarvestRowResponse    `json:"rows"`
}

type HarvestPlanHeaderResponse struct {
	ID                int64   `json:"id"`
	CompanyID         int64   `json:"companyId"`
	SeasonID          int64   `json:"seasonId"`
	PlanningVersionID int64   `json:"planningVersionId"`
	DocumentNo        string  `json:"documentNo"`
	Description       *string `json:"description,omitempty"`
	DateFrom          Date    `json:"dateFrom"`
	DateTo            Date    `json:"dateTo"`
	AuditFields
}

type HarvestPlanItemResponse struct {
	ID             int64     `json:"id"`
	PlanDate       Date      `json:"planDate"`
	GrowerID       int64     `json:"growerId"`
	CaneFieldID    *int64    `json:"caneFieldId,omitempty"`
	VarietyID      *int64    `json:"varietyId,omitempty"`
	PlannedTons    Quantity  `json:"plannedTons"`
	UOMID          int64     `json:"uomId"`
	PlannedAreaHa  *Quantity `json:"plannedAreaHa,omitempty"`
	ExpectedCCSPct *Quantity `json:"expectedCcsPct,omitempty"`
	Remark         *string   `json:"remark,omitempty"`
	AuditFields
}

// --- cane deliveries -----------------------------------------------------

type CaneDeliveryRequest struct {
	DeliveryDate Date   `json:"deliveryDate" binding:"required"`
	GrowerID     int64  `json:"growerId" binding:"required"`
	CaneFieldID  *int64 `json:"caneFieldId"`
	VarietyID    *int64 `json:"varietyId"`
	SeasonID     *int64 `json:"seasonId"`
	// MaterialID, MovementTypeID and UOMID may be omitted: the server fills in
	// the company's cane material, the cane intake movement and the material's
	// base unit.
	MaterialID     int64     `json:"materialId"`
	MovementTypeID int64     `json:"movementTypeId"`
	WarehouseID    *int64    `json:"warehouseId"`
	UOMID          int64     `json:"uomId"`
	TicketNo       *string   `json:"ticketNo" binding:"omitempty,max=40"`
	VehicleNo      *string   `json:"vehicleNo" binding:"omitempty,max=30"`
	GrossTons      Quantity  `json:"grossTons" binding:"required"`
	TareTons       Quantity  `json:"tareTons"`
	PricePerTon    *Quantity `json:"pricePerTon"`
	Currency       *string   `json:"currency" binding:"omitempty,len=3"`
	CCSPct         *Quantity `json:"ccsPct"`
	TrashPct       *Quantity `json:"trashPct"`
	IsBurnt        bool      `json:"isBurnt"`
	Remark         *string   `json:"remark" binding:"omitempty,max=500"`
	Version        int       `json:"version"`
}

type CaneDeliveryResponse struct {
	ID             int64     `json:"id"`
	CompanyID      int64     `json:"companyId"`
	SeasonID       *int64    `json:"seasonId,omitempty"`
	DocumentNo     string    `json:"documentNo"`
	DeliveryDate   Date      `json:"deliveryDate"`
	GrowerID       int64     `json:"growerId"`
	CaneFieldID    *int64    `json:"caneFieldId,omitempty"`
	VarietyID      *int64    `json:"varietyId,omitempty"`
	MaterialID     int64     `json:"materialId"`
	MovementTypeID int64     `json:"movementTypeId"`
	WarehouseID    *int64    `json:"warehouseId,omitempty"`
	UOMID          int64     `json:"uomId"`
	TicketNo       *string   `json:"ticketNo,omitempty"`
	VehicleNo      *string   `json:"vehicleNo,omitempty"`
	GrossTons      Quantity  `json:"grossTons"`
	TareTons       Quantity  `json:"tareTons"`
	NetTons        Quantity  `json:"netTons"`
	PricePerTon    *Quantity `json:"pricePerTon,omitempty"`
	Currency       *string   `json:"currency,omitempty"`
	// Value is net tonnage × price. Absent when no price was agreed — an
	// unpriced load has an unknown value, never a value of zero.
	Value         *Quantity  `json:"value,omitempty"`
	CCSPct        *Quantity  `json:"ccsPct,omitempty"`
	TrashPct      *Quantity  `json:"trashPct,omitempty"`
	IsBurnt       bool       `json:"isBurnt"`
	PostingStatus string     `json:"postingStatus"`
	PostedBy      *int64     `json:"postedBy,omitempty"`
	PostedAt      *time.Time `json:"postedAt,omitempty"`
	ReversedBy    *int64     `json:"reversedBy,omitempty"`
	ReversedAt    *time.Time `json:"reversedAt,omitempty"`
	Remark        *string    `json:"remark,omitempty"`
	AuditFields
}

// --- cane plan vs actual -------------------------------------------------

type CanePlanVsActualRow struct {
	GroupKey    string    `json:"groupKey"`
	GroupLabel  string    `json:"groupLabel"`
	SupplyType  string    `json:"supplyType,omitempty"`
	GrowerID    *int64    `json:"growerId,omitempty"`
	GrowerCode  *string   `json:"growerCode,omitempty"`
	VarietyID   *int64    `json:"varietyId,omitempty"`
	VarietyCode *string   `json:"varietyCode,omitempty"`
	PlanDate    *Date     `json:"planDate,omitempty"`
	PlannedTons Quantity  `json:"plannedTons"`
	ActualTons  Quantity  `json:"actualTons"`
	Deliveries  int64     `json:"deliveries"`
	// VariancePct is null when there is no plan to vary from — rendered as
	// "n/a", never as 0 or 100 (§F6).
	VariancePct   *Quantity `json:"variancePct"`
	AverageCCSPct *Quantity `json:"averageCcsPct,omitempty"`
	PurchaseValue *Quantity `json:"purchaseValue,omitempty"`
	Currency      *string   `json:"currency,omitempty"`
}
