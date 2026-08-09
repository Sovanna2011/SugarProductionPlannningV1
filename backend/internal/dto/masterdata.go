package dto

// --- companies -----------------------------------------------------------

type CompanyRequest struct {
	CompanyCode       string  `json:"companyCode" binding:"required,max=10"`
	CompanyName       string  `json:"companyName" binding:"required,max=200"`
	LocalCurrency     string  `json:"localCurrency" binding:"required,len=3"`
	GroupCurrency     string  `json:"groupCurrency" binding:"required,len=3"`
	Timezone          string  `json:"timezone" binding:"required,max=64"`
	FiscalYearVariant *string `json:"fiscalYearVariant" binding:"omitempty,max=10"`
	CountryCode       *string `json:"countryCode" binding:"omitempty,len=2"`
	Version           int     `json:"version"`
}

type CompanyResponse struct {
	ID                int64   `json:"id"`
	CompanyCode       string  `json:"companyCode"`
	CompanyName       string  `json:"companyName"`
	LocalCurrency     string  `json:"localCurrency"`
	GroupCurrency     string  `json:"groupCurrency"`
	Timezone          string  `json:"timezone"`
	FiscalYearVariant *string `json:"fiscalYearVariant,omitempty"`
	CountryCode       *string `json:"countryCode,omitempty"`
	AuditFields
}

// --- units of measure ----------------------------------------------------

type UOMRequest struct {
	UOMCode   string `json:"uomCode" binding:"required,max=10"`
	UOMName   string `json:"uomName" binding:"required,max=60"`
	Dimension string `json:"dimension" binding:"required,oneof=MASS VOLUME ENERGY COUNT"`
	Decimals  int    `json:"decimals" binding:"min=0,max=6"`
	Version   int    `json:"version"`
}

type UOMResponse struct {
	ID        int64  `json:"id"`
	UOMCode   string `json:"uomCode"`
	UOMName   string `json:"uomName"`
	Dimension string `json:"dimension"`
	Decimals  int    `json:"decimals"`
	AuditFields
}

// --- materials -----------------------------------------------------------

type MaterialRequest struct {
	MaterialCode  string  `json:"materialCode" binding:"required,max=40"`
	MaterialName  string  `json:"materialName" binding:"required,max=200"`
	MaterialType  string  `json:"materialType" binding:"required,oneof=RAW SEMI_FINISHED FINISHED BY_PRODUCT UTILITY"`
	MaterialGroup *string `json:"materialGroup" binding:"omitempty,max=40"`
	BaseUOMID     int64   `json:"baseUomId" binding:"required"`
	// ConditioningRequired routes a finished grade through the condition silo
	// (§F7). White Sugar keeps it false and goes direct.
	ConditioningRequired bool `json:"conditioningRequired"`
	IsStockManaged       bool `json:"isStockManaged"`
	Version              int  `json:"version"`
}

type MaterialResponse struct {
	ID                   int64   `json:"id"`
	MaterialCode         string  `json:"materialCode"`
	MaterialName         string  `json:"materialName"`
	MaterialType         string  `json:"materialType"`
	MaterialGroup        *string `json:"materialGroup,omitempty"`
	BaseUOMID            int64   `json:"baseUomId"`
	BaseUOMCode          string  `json:"baseUomCode,omitempty"`
	ConditioningRequired bool    `json:"conditioningRequired"`
	IsStockManaged       bool    `json:"isStockManaged"`
	AuditFields
}

type CompanyMaterialRequest struct {
	MaterialID        int64 `json:"materialId" binding:"required"`
	PlanningEnabled   bool  `json:"planningEnabled"`
	ProductionEnabled bool  `json:"productionEnabled"`
	InventoryEnabled  bool  `json:"inventoryEnabled"`
	SalesEnabled      bool  `json:"salesEnabled"`
	IsActive          bool  `json:"isActive"`
}

type CompanyMaterialResponse struct {
	ID                int64             `json:"id"`
	CompanyID         int64             `json:"companyId"`
	MaterialID        int64             `json:"materialId"`
	Material          *MaterialResponse `json:"material,omitempty"`
	PlanningEnabled   bool              `json:"planningEnabled"`
	ProductionEnabled bool              `json:"productionEnabled"`
	InventoryEnabled  bool              `json:"inventoryEnabled"`
	SalesEnabled      bool              `json:"salesEnabled"`
	AuditFields
}

// --- packaging -----------------------------------------------------------

type PackagingTypeRequest struct {
	PackagingCode   string    `json:"packagingCode" binding:"required,max=20"`
	PackagingName   string    `json:"packagingName" binding:"required,max=100"`
	NominalQuantity *Quantity `json:"nominalQuantity"`
	NominalUOMID    *int64    `json:"nominalUomId"`
	IsBulk          bool      `json:"isBulk"`
	Version         int       `json:"version"`
}

type PackagingTypeResponse struct {
	ID              int64     `json:"id"`
	PackagingCode   string    `json:"packagingCode"`
	PackagingName   string    `json:"packagingName"`
	NominalQuantity *Quantity `json:"nominalQuantity,omitempty"`
	NominalUOMID    *int64    `json:"nominalUomId,omitempty"`
	IsBulk          bool      `json:"isBulk"`
	AuditFields
}

// --- warehouses ----------------------------------------------------------

type WarehouseRequest struct {
	WarehouseCode string `json:"warehouseCode" binding:"required,max=20"`
	WarehouseName string `json:"warehouseName" binding:"required,max=200"`
	WarehouseType string `json:"warehouseType" binding:"required,oneof=WAREHOUSE TANK SILO PRODUCTION_STORAGE"`
	// A null capacity means unlimited — not zero (§F5).
	Capacity           *Quantity `json:"capacity"`
	CapacityUOMID      *int64    `json:"capacityUomId"`
	AllowNegativeStock bool      `json:"allowNegativeStock"`
	Location           *string   `json:"location" binding:"omitempty,max=200"`
	// AllowedMaterialIDs restricts a tank or silo to specific materials (OQ-4).
	AllowedMaterialIDs []int64 `json:"allowedMaterialIds"`
	Version            int     `json:"version"`
}

// WarehouseMaterialsRequest maintains the material restriction of a tank or
// silo on its own, without rewriting the location itself. An empty list means
// the location accepts every material.
type WarehouseMaterialsRequest struct {
	MaterialIDs []int64 `json:"materialIds"`
}

type WarehouseResponse struct {
	ID                 int64     `json:"id"`
	CompanyID          int64     `json:"companyId"`
	WarehouseCode      string    `json:"warehouseCode"`
	WarehouseName      string    `json:"warehouseName"`
	WarehouseType      string    `json:"warehouseType"`
	Capacity           *Quantity `json:"capacity,omitempty"`
	CapacityUOMID      *int64    `json:"capacityUomId,omitempty"`
	AllowNegativeStock bool      `json:"allowNegativeStock"`
	Location           *string   `json:"location,omitempty"`
	AllowedMaterialIDs []int64   `json:"allowedMaterialIds,omitempty"`
	AuditFields
}

// --- production lines ----------------------------------------------------

type ProductionLineRequest struct {
	LineCode       string    `json:"lineCode" binding:"required,max=20"`
	LineName       string    `json:"lineName" binding:"required,max=200"`
	CapacityPerDay *Quantity `json:"capacityPerDay"`
	CapacityUOMID  *int64    `json:"capacityUomId"`
	Version        int       `json:"version"`
}

type ProductionLineResponse struct {
	ID             int64     `json:"id"`
	CompanyID      int64     `json:"companyId"`
	LineCode       string    `json:"lineCode"`
	LineName       string    `json:"lineName"`
	CapacityPerDay *Quantity `json:"capacityPerDay,omitempty"`
	CapacityUOMID  *int64    `json:"capacityUomId,omitempty"`
	AuditFields
}

// --- seasons -------------------------------------------------------------

type SeasonRequest struct {
	SeasonCode string `json:"seasonCode" binding:"required,max=20"`
	SeasonName string `json:"seasonName" binding:"required,max=200"`
	StartDate  Date   `json:"startDate" binding:"required"`
	EndDate    Date   `json:"endDate" binding:"required"`
	Status     string `json:"status" binding:"omitempty,oneof=PLANNING OPEN CLOSED"`
	Version    int    `json:"version"`
}

type SeasonResponse struct {
	ID         int64  `json:"id"`
	CompanyID  int64  `json:"companyId"`
	SeasonCode string `json:"seasonCode"`
	SeasonName string `json:"seasonName"`
	StartDate  Date   `json:"startDate"`
	EndDate    Date   `json:"endDate"`
	Status     string `json:"status"`
	AuditFields
}

// --- processes -----------------------------------------------------------

type ProcessRequest struct {
	ProcessCode string  `json:"processCode" binding:"required,max=30"`
	ProcessName string  `json:"processName" binding:"required,max=200"`
	SequenceNo  int     `json:"sequenceNo"`
	Description *string `json:"description"`
	Version     int     `json:"version"`
}

type ProcessMaterialRequest struct {
	MaterialID int64  `json:"materialId" binding:"required"`
	IOType     string `json:"ioType" binding:"required,oneof=INPUT OUTPUT"`
	// RequiresConditioning marks an output that must pass through the silo
	// before it can reach a finished-goods warehouse (§F7).
	RequiresConditioning bool      `json:"requiresConditioning"`
	ExpectedYieldPct     *Quantity `json:"expectedYieldPct"`
}

type ProcessMaterialResponse struct {
	ID                   int64     `json:"id"`
	ProcessID            int64     `json:"processId"`
	MaterialID           int64     `json:"materialId"`
	MaterialCode         string    `json:"materialCode,omitempty"`
	MaterialName         string    `json:"materialName,omitempty"`
	IOType               string    `json:"ioType"`
	RequiresConditioning bool      `json:"requiresConditioning"`
	ExpectedYieldPct     *Quantity `json:"expectedYieldPct,omitempty"`
}

type ProcessResponse struct {
	ID          int64                     `json:"id"`
	ProcessCode string                    `json:"processCode"`
	ProcessName string                    `json:"processName"`
	SequenceNo  int                       `json:"sequenceNo"`
	Description *string                   `json:"description,omitempty"`
	Materials   []ProcessMaterialResponse `json:"materials,omitempty"`
	AuditFields
}

// --- movement types ------------------------------------------------------

type MovementTypeRequest struct {
	MovementCode string `json:"movementCode" binding:"required,max=30"`
	MovementName string `json:"movementName" binding:"required,max=200"`
	// Direction is what gives a movement its sign — quantities themselves are
	// always positive (§33).
	Direction          string `json:"direction" binding:"required,oneof=IN OUT"`
	AffectsStock       bool   `json:"affectsStock"`
	IsPlanningRelevant bool   `json:"isPlanningRelevant"`
	IsTransfer         bool   `json:"isTransfer"`
	IsAdjustment       bool   `json:"isAdjustment"`
	CounterpartID      *int64 `json:"counterpartId"`
	Version            int    `json:"version"`
}

type MovementTypeResponse struct {
	ID                 int64  `json:"id"`
	MovementCode       string `json:"movementCode"`
	MovementName       string `json:"movementName"`
	Direction          string `json:"direction"`
	AffectsStock       bool   `json:"affectsStock"`
	IsPlanningRelevant bool   `json:"isPlanningRelevant"`
	IsTransfer         bool   `json:"isTransfer"`
	IsAdjustment       bool   `json:"isAdjustment"`
	CounterpartID      *int64 `json:"counterpartId,omitempty"`
	AuditFields
}
