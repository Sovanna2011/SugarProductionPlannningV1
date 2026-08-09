package dto

// --- movements -----------------------------------------------------------

type MovementRequest struct {
	TransactionDate Date   `json:"transactionDate" binding:"required"`
	MaterialID      int64  `json:"materialId" binding:"required"`
	WarehouseID     int64  `json:"warehouseId" binding:"required"`
	MovementTypeID  int64  `json:"movementTypeId" binding:"required"`
	PackagingTypeID *int64 `json:"packagingTypeId"`
	ProcessID       *int64 `json:"processId"`
	// Quantity is always positive; the movement type supplies the sign (§33).
	Quantity Quantity `json:"quantity" binding:"required"`
	UOMID    int64    `json:"uomId" binding:"required"`
	Remark   *string  `json:"remark" binding:"omitempty,max=500"`
}

type TransferRequest struct {
	FromWarehouseID int64    `json:"fromWarehouseId" binding:"required"`
	ToWarehouseID   int64    `json:"toWarehouseId" binding:"required"`
	MaterialID      int64    `json:"materialId" binding:"required"`
	PackagingTypeID *int64   `json:"packagingTypeId"`
	Quantity        Quantity `json:"quantity" binding:"required"`
	UOMID           int64    `json:"uomId" binding:"required"`
	TransactionDate Date     `json:"transactionDate" binding:"required"`
	Remark          *string  `json:"remark" binding:"omitempty,max=500"`
}

type MovementResponse struct {
	ID                 int64    `json:"id"`
	CompanyID          int64    `json:"companyId"`
	DocumentNo         string   `json:"documentNo"`
	LineNo             int      `json:"lineNo"`
	TransactionDate    Date     `json:"transactionDate"`
	MaterialID         int64    `json:"materialId"`
	WarehouseID        int64    `json:"warehouseId"`
	MovementTypeID     int64    `json:"movementTypeId"`
	MovementCode       string   `json:"movementCode,omitempty"`
	Direction          string   `json:"direction,omitempty"`
	PackagingTypeID    *int64   `json:"packagingTypeId,omitempty"`
	ProcessID          *int64   `json:"processId,omitempty"`
	Quantity           Quantity `json:"quantity"`
	UOMID              int64    `json:"uomId"`
	ReferenceDocument  *string  `json:"referenceDocument,omitempty"`
	ReferenceID        *int64   `json:"referenceId,omitempty"`
	SourceModule       string   `json:"sourceModule"`
	TransferGroupID    *string  `json:"transferGroupId,omitempty"`
	ReversedMovementID *int64   `json:"reversedMovementId,omitempty"`
	IsReversed         bool     `json:"isReversed"`
	Remark             *string  `json:"remark,omitempty"`
	AuditFields
}

// --- balances and capacity -----------------------------------------------

type BalanceResponse struct {
	CompanyID       int64    `json:"companyId"`
	WarehouseID     int64    `json:"warehouseId"`
	WarehouseCode   string   `json:"warehouseCode"`
	WarehouseName   string   `json:"warehouseName"`
	MaterialID      int64    `json:"materialId"`
	MaterialCode    string   `json:"materialCode"`
	MaterialName    string   `json:"materialName"`
	PackagingTypeID *int64   `json:"packagingTypeId,omitempty"`
	UOMCode         string   `json:"uomCode"`
	OpeningQty      Quantity `json:"openingQty"`
	InQty           Quantity `json:"inQty"`
	OutQty          Quantity `json:"outQty"`
	ClosingQty      Quantity `json:"closingQty"`
}

// CapacityResponse implements §F5. Utilisation is null for an unlimited
// location — never 0 and never infinite.
type CapacityResponse struct {
	WarehouseID    int64     `json:"warehouseId"`
	WarehouseCode  string    `json:"warehouseCode"`
	WarehouseName  string    `json:"warehouseName"`
	WarehouseType  string    `json:"warehouseType"`
	Capacity       *Quantity `json:"capacity"`
	CapacityUOMID  *int64    `json:"capacityUomId,omitempty"`
	CurrentStock   Quantity  `json:"currentStock"`
	Available      *Quantity `json:"available"`
	UtilisationPct *Quantity `json:"utilisationPct"`
	TrafficLight   string    `json:"trafficLight"`
}

type ReconcileResponse struct {
	CompanyID     int64                 `json:"companyId"`
	DateFrom      Date                  `json:"dateFrom"`
	DateTo        Date                  `json:"dateTo"`
	Discrepancies []DiscrepancyResponse `json:"discrepancies"`
}

type DiscrepancyResponse struct {
	WarehouseID     int64    `json:"warehouseId"`
	MaterialID      int64    `json:"materialId"`
	PackagingTypeID *int64   `json:"packagingTypeId,omitempty"`
	BalanceDate     Date     `json:"balanceDate"`
	StoredQty       Quantity `json:"storedQty"`
	ComputedQty     Quantity `json:"computedQty"`
}
