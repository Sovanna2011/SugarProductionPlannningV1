package dto

// PlanVsActualLineResponse is one row of the §36 report.
type PlanVsActualLineResponse struct {
	CompanyID   int64    `json:"companyId"`
	CompanyCode string   `json:"companyCode"`
	GroupKey    string   `json:"groupKey"`
	GroupLabel  string   `json:"groupLabel"`
	UOMCode     string   `json:"uomCode,omitempty"`
	PlanQty     Quantity `json:"planQty"`
	ActualQty   Quantity `json:"actualQty"`
	Variance    Quantity `json:"variance"`
	// VariancePct is null where a percentage is undefined — an actual against
	// a zero plan. The client renders that as "n/a", never as 0 or 100 (§F6).
	VariancePct *Quantity `json:"variancePct"`
	// ConversionMissing warns that some quantities could not be brought to the
	// material base unit, so the variance on this row is not comparable.
	ConversionMissing bool `json:"conversionMissing,omitempty"`
}

type PlanVsActualResponse struct {
	GroupBy       string                     `json:"groupBy"`
	DateFrom      Date                       `json:"dateFrom"`
	DateTo        Date                       `json:"dateTo"`
	VersionID     *int64                     `json:"versionId,omitempty"`
	TotalPlan     Quantity                   `json:"totalPlan"`
	TotalActual   Quantity                   `json:"totalActual"`
	TotalVariance Quantity                   `json:"totalVariance"`
	Lines         []PlanVsActualLineResponse `json:"lines"`
}

type ProductionSummaryResponse struct {
	MaterialID   int64    `json:"materialId"`
	MaterialCode string   `json:"materialCode"`
	MaterialName string   `json:"materialName"`
	ProcessCode  *string  `json:"processCode,omitempty"`
	UOMCode      string   `json:"uomCode"`
	Quantity     Quantity `json:"quantity"`
}

type MovementReportResponse struct {
	TransactionDate Date     `json:"transactionDate"`
	DocumentNo      string   `json:"documentNo"`
	WarehouseCode   string   `json:"warehouseCode"`
	MaterialCode    string   `json:"materialCode"`
	MovementCode    string   `json:"movementCode"`
	Direction       string   `json:"direction"`
	Quantity        Quantity `json:"quantity"`
	UOMCode         string   `json:"uomCode"`
	SourceModule    string   `json:"sourceModule"`
	IsReversed      bool     `json:"isReversed"`
}

// --- data dictionary & browser (Part G) ----------------------------------

type DDTableResponse struct {
	ID                  int64   `json:"id"`
	TableName           string  `json:"tableName"`
	Module              string  `json:"module"`
	TableType           string  `json:"tableType"`
	DescriptionEN       *string `json:"descriptionEn,omitempty"`
	DescriptionLocal    *string `json:"descriptionLocal,omitempty"`
	BusinessDescription *string `json:"businessDescription,omitempty"`
	IsCompanyDependent  bool    `json:"isCompanyDependent"`
	IsBrowsable         bool    `json:"isBrowsable"`
	AuditFields
}

type DDTableUpdateRequest struct {
	DescriptionEN       *string `json:"descriptionEn" binding:"omitempty,max=255"`
	DescriptionLocal    *string `json:"descriptionLocal" binding:"omitempty,max=255"`
	BusinessDescription *string `json:"businessDescription"`
	IsBrowsable         bool    `json:"isBrowsable"`
	VersionedRequest
}

type DDFieldResponse struct {
	ID                  int64   `json:"id"`
	TableID             int64   `json:"tableId"`
	FieldName           string  `json:"fieldName"`
	Position            int     `json:"position"`
	LabelShort          *string `json:"labelShort,omitempty"`
	LabelMedium         *string `json:"labelMedium,omitempty"`
	LabelLong           *string `json:"labelLong,omitempty"`
	DataType            string  `json:"dataType"`
	Length              *int    `json:"length,omitempty"`
	Decimals            *int    `json:"decimals,omitempty"`
	IsKey               bool    `json:"isKey"`
	IsRequired          bool    `json:"isRequired"`
	IsPII               bool    `json:"isPii"`
	IsBrowsable         bool    `json:"isBrowsable"`
	ValueHelpID         *int64  `json:"valueHelpId,omitempty"`
	BusinessDescription *string `json:"businessDescription,omitempty"`
	AuditFields
}

type DDFieldUpdateRequest struct {
	LabelShort          *string `json:"labelShort" binding:"omitempty,max=20"`
	LabelMedium         *string `json:"labelMedium" binding:"omitempty,max=40"`
	LabelLong           *string `json:"labelLong" binding:"omitempty,max=100"`
	BusinessDescription *string `json:"businessDescription"`
	IsPII               bool    `json:"isPii"`
	IsBrowsable         bool    `json:"isBrowsable"`
	ValueHelpID         *int64  `json:"valueHelpId"`
	VersionedRequest
}

type DDDomainResponse struct {
	ID          int64   `json:"id"`
	DomainName  string  `json:"domainName"`
	DataType    string  `json:"dataType"`
	Length      *int    `json:"length,omitempty"`
	Decimals    *int    `json:"decimals,omitempty"`
	Description *string `json:"description,omitempty"`
}

type BrowserResponse struct {
	TableName string           `json:"tableName"`
	Fields    []string         `json:"fields"`
	Rows      []map[string]any `json:"rows"`
}

type AuditLogResponse struct {
	ID        int64   `json:"id"`
	TableName string  `json:"tableName"`
	RecordID  *int64  `json:"recordId,omitempty"`
	CompanyID *int64  `json:"companyId,omitempty"`
	Action    string  `json:"action"`
	ChangedBy *int64  `json:"changedBy,omitempty"`
	ChangedAt string  `json:"changedAt"`
	OldValues any     `json:"oldValues,omitempty"`
	NewValues any     `json:"newValues,omitempty"`
	RequestID *string `json:"requestId,omitempty"`
	IPAddress *string `json:"ipAddress,omitempty"`
}

type NumberRangeResponse struct {
	ID         int64  `json:"id"`
	CompanyID  int64  `json:"companyId"`
	ObjectType string `json:"objectType"`
	FiscalYear int    `json:"fiscalYear"`
	Prefix     string `json:"prefix"`
	CurrentNo  int64  `json:"currentNo"`
	Length     int    `json:"length"`
	AuditFields
}
