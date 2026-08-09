package dto

import "time"

// --- planning versions ---------------------------------------------------

type PlanningVersionRequest struct {
	SeasonID int64 `json:"seasonId" binding:"required"`
	// VersionNo may be omitted; the server then allocates the next free number.
	// Numbering is open-ended — nothing caps it at three (§29).
	VersionNo   int     `json:"versionNo" binding:"omitempty,min=1"`
	VersionName string  `json:"versionName" binding:"required,max=200"`
	Description *string `json:"description"`
	Version     int     `json:"version"`
}

type CopyVersionRequest struct {
	NewVersionNo int     `json:"newVersionNo" binding:"omitempty,min=1"`
	VersionName  string  `json:"versionName" binding:"required,max=200"`
	Description  *string `json:"description"`
}

type PlanningVersionResponse struct {
	ID          int64   `json:"id"`
	CompanyID   int64   `json:"companyId"`
	SeasonID    int64   `json:"seasonId"`
	VersionNo   int     `json:"versionNo"`
	VersionName string  `json:"versionName"`
	Description *string `json:"description,omitempty"`
	Status      string  `json:"status"`
	// CopiedFromVersionID records lineage only. A copy shares no row with its
	// source, so editing it can never affect the original (§31).
	CopiedFromVersionID *int64     `json:"copiedFromVersionId,omitempty"`
	SubmittedBy         *int64     `json:"submittedBy,omitempty"`
	SubmittedAt         *time.Time `json:"submittedAt,omitempty"`
	ApprovedBy          *int64     `json:"approvedBy,omitempty"`
	ApprovedAt          *time.Time `json:"approvedAt,omitempty"`
	LockedBy            *int64     `json:"lockedBy,omitempty"`
	LockedAt            *time.Time `json:"lockedAt,omitempty"`
	AuditFields
}

// --- plan documents ------------------------------------------------------

type PlanHeaderRequest struct {
	SeasonID          int64   `json:"seasonId" binding:"required"`
	PlanningVersionID int64   `json:"planningVersionId" binding:"required"`
	MovementTypeID    int64   `json:"movementTypeId" binding:"required"`
	ProductionLineID  *int64  `json:"productionLineId"`
	Description       *string `json:"description" binding:"omitempty,max=255"`
	DateFrom          Date    `json:"dateFrom" binding:"required"`
	DateTo            Date    `json:"dateTo" binding:"required"`
	Version           int     `json:"version"`
}

type PlanHeaderResponse struct {
	ID                int64   `json:"id"`
	CompanyID         int64   `json:"companyId"`
	SeasonID          int64   `json:"seasonId"`
	PlanningVersionID int64   `json:"planningVersionId"`
	MovementTypeID    int64   `json:"movementTypeId"`
	ProductionLineID  *int64  `json:"productionLineId,omitempty"`
	DocumentNo        string  `json:"documentNo"`
	Description       *string `json:"description,omitempty"`
	DateFrom          Date    `json:"dateFrom"`
	DateTo            Date    `json:"dateTo"`
	AuditFields
}

type PlanItemRequest struct {
	PlanDate         Date     `json:"planDate" binding:"required"`
	ProductionLineID *int64   `json:"productionLineId"`
	MaterialID       int64    `json:"materialId" binding:"required"`
	ProcessID        *int64   `json:"processId"`
	WarehouseID      *int64   `json:"warehouseId"`
	PackagingTypeID  *int64   `json:"packagingTypeId"`
	Quantity         Quantity `json:"quantity" binding:"required"`
	UOMID            int64    `json:"uomId" binding:"required"`
	Remark           *string  `json:"remark" binding:"omitempty,max=500"`
}

type PlanItemResponse struct {
	ID               int64    `json:"id"`
	PlanHeaderID     int64    `json:"planHeaderId"`
	PlanDate         Date     `json:"planDate"`
	ProductionLineID *int64   `json:"productionLineId,omitempty"`
	MaterialID       int64    `json:"materialId"`
	ProcessID        *int64   `json:"processId,omitempty"`
	WarehouseID      *int64   `json:"warehouseId,omitempty"`
	PackagingTypeID  *int64   `json:"packagingTypeId,omitempty"`
	Quantity         Quantity `json:"quantity"`
	UOMID            int64    `json:"uomId"`
	Remark           *string  `json:"remark,omitempty"`
	AuditFields
}

// --- the planning matrix (§28) -------------------------------------------

// MatrixValueRequest is one cell. A null quantity means "leave unchanged";
// omitting the cell entirely means "delete" unless partialUpdate is set.
type MatrixValueRequest struct {
	ProductionLineID *int64    `json:"productionLineId"`
	Quantity         *Quantity `json:"quantity"`
}

type MatrixRowRequest struct {
	PlanDate Date                 `json:"planDate" binding:"required"`
	Values   []MatrixValueRequest `json:"values" binding:"required"`
}

// MatrixSaveRequest saves the planning grid. It accepts either shape:
//
//   - one product, with movementTypeId/materialId/uomId and rows at the top
//     level;
//   - several products at once, each in its own entry of series.
//
// Both cover the same span of days and are written in one transaction.
type MatrixSaveRequest struct {
	SeasonID  int64 `json:"seasonId" binding:"required"`
	VersionID int64 `json:"versionId" binding:"required"`
	// The single-product fields. Leave them out when using series.
	MovementTypeID int64              `json:"movementTypeId"`
	MaterialID     int64              `json:"materialId"`
	ProcessID      *int64             `json:"processId"`
	WarehouseID    *int64             `json:"warehouseId"`
	UOMID          int64              `json:"uomId"`
	Rows           []MatrixRowRequest `json:"rows"`
	// Series plans several products over the same days in one call.
	Series        []MatrixSeriesRequest `json:"series"`
	DateFrom      Date                  `json:"dateFrom" binding:"required"`
	DateTo        Date                  `json:"dateTo" binding:"required"`
	PartialUpdate bool                  `json:"partialUpdate"`
}

// MatrixSeriesRequest is one product's grid inside a multi-product save.
type MatrixSeriesRequest struct {
	MovementTypeID int64              `json:"movementTypeId" binding:"required"`
	MaterialID     int64              `json:"materialId" binding:"required"`
	ProcessID      *int64             `json:"processId"`
	WarehouseID    *int64             `json:"warehouseId"`
	UOMID          int64              `json:"uomId" binding:"required"`
	Rows           []MatrixRowRequest `json:"rows" binding:"required"`
}

type MatrixLineResponse struct {
	ProductionLineID int64  `json:"productionLineId"`
	LineCode         string `json:"lineCode"`
	LineName         string `json:"lineName"`
}

type MatrixValueResponse struct {
	ProductionLineID *int64   `json:"productionLineId,omitempty"`
	Quantity         Quantity `json:"quantity"`
	UOMID            int64    `json:"uomId"`
	Version          int      `json:"version"`
}

type MatrixRowResponse struct {
	PlanDate Date                  `json:"planDate"`
	Values   []MatrixValueResponse `json:"values"`
}

type MatrixResponse struct {
	CompanyID      int64                `json:"companyId"`
	VersionID      int64                `json:"versionId"`
	VersionStatus  string               `json:"versionStatus"`
	MovementTypeID int64                `json:"movementTypeId"`
	MaterialID     int64                `json:"materialId"`
	MaterialCode   string               `json:"materialCode,omitempty"`
	MaterialName   string               `json:"materialName,omitempty"`
	ProcessID      *int64               `json:"processId,omitempty"`
	ProcessCode    *string              `json:"processCode,omitempty"`
	UOMID          int64                `json:"uomId,omitempty"`
	UOMCode        string               `json:"uomCode,omitempty"`
	DateFrom       Date                 `json:"dateFrom"`
	DateTo         Date                 `json:"dateTo"`
	Lines          []MatrixLineResponse `json:"lines"`
	Rows           []MatrixRowResponse  `json:"rows"`
}

// MatrixSetResponse is the whole plan for a window: every product the version
// plans, each with its own grid, sharing one set of dates and lines.
type MatrixSetResponse struct {
	CompanyID     int64                `json:"companyId"`
	VersionID     int64                `json:"versionId"`
	VersionStatus string               `json:"versionStatus"`
	DateFrom      Date                 `json:"dateFrom"`
	DateTo        Date                 `json:"dateTo"`
	Lines         []MatrixLineResponse `json:"lines"`
	Series        []MatrixResponse     `json:"series"`
}

// --- actual documents ----------------------------------------------------

type ActualItemRequest struct {
	ActualDate       Date     `json:"actualDate" binding:"required"`
	ProductionLineID *int64   `json:"productionLineId"`
	MaterialID       int64    `json:"materialId" binding:"required"`
	ProcessID        *int64   `json:"processId"`
	WarehouseID      *int64   `json:"warehouseId"`
	PackagingTypeID  *int64   `json:"packagingTypeId"`
	Quantity         Quantity `json:"quantity" binding:"required"`
	UOMID            int64    `json:"uomId" binding:"required"`
	Shift            *string  `json:"shift" binding:"omitempty,max=10"`
	Remark           *string  `json:"remark" binding:"omitempty,max=500"`
}

type ActualHeaderRequest struct {
	SeasonID         *int64              `json:"seasonId"`
	MovementTypeID   int64               `json:"movementTypeId" binding:"required"`
	ProductionLineID *int64              `json:"productionLineId"`
	Description      *string             `json:"description" binding:"omitempty,max=255"`
	PostingDate      Date                `json:"postingDate" binding:"required"`
	Items            []ActualItemRequest `json:"items"`
	Version          int                 `json:"version"`
}

type ActualItemResponse struct {
	ID               int64    `json:"id"`
	ActualHeaderID   int64    `json:"actualHeaderId"`
	ActualDate       Date     `json:"actualDate"`
	ProductionLineID *int64   `json:"productionLineId,omitempty"`
	MaterialID       int64    `json:"materialId"`
	ProcessID        *int64   `json:"processId,omitempty"`
	WarehouseID      *int64   `json:"warehouseId,omitempty"`
	PackagingTypeID  *int64   `json:"packagingTypeId,omitempty"`
	Quantity         Quantity `json:"quantity"`
	UOMID            int64    `json:"uomId"`
	Shift            *string  `json:"shift,omitempty"`
	Remark           *string  `json:"remark,omitempty"`
	AuditFields
}

type ActualHeaderResponse struct {
	ID               int64                `json:"id"`
	CompanyID        int64                `json:"companyId"`
	SeasonID         *int64               `json:"seasonId,omitempty"`
	MovementTypeID   int64                `json:"movementTypeId"`
	ProductionLineID *int64               `json:"productionLineId,omitempty"`
	DocumentNo       string               `json:"documentNo"`
	Description      *string              `json:"description,omitempty"`
	PostingDate      Date                 `json:"postingDate"`
	PostingStatus    string               `json:"postingStatus"`
	PostedBy         *int64               `json:"postedBy,omitempty"`
	PostedAt         *time.Time           `json:"postedAt,omitempty"`
	ReversedBy       *int64               `json:"reversedBy,omitempty"`
	ReversedAt       *time.Time           `json:"reversedAt,omitempty"`
	Items            []ActualItemResponse `json:"items,omitempty"`
	AuditFields
}

type ReverseRequest struct {
	Remark *string `json:"remark" binding:"omitempty,max=500"`
}
