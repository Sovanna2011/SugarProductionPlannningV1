package mapper

import (
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/dto"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/service"
)

// --- planning ------------------------------------------------------------

func PlanningVersionToDTO(version model.PlanningVersion) dto.PlanningVersionResponse {
	return dto.PlanningVersionResponse{
		ID:                  version.ID,
		CompanyID:           version.CompanyID,
		SeasonID:            version.SeasonID,
		VersionNo:           version.VersionNo,
		VersionName:         version.VersionName,
		Description:         version.Description,
		Status:              version.Status,
		CopiedFromVersionID: version.CopiedFromVersionID,
		SubmittedBy:         version.SubmittedBy,
		SubmittedAt:         version.SubmittedAt,
		ApprovedBy:          version.ApprovedBy,
		ApprovedAt:          version.ApprovedAt,
		LockedBy:            version.LockedBy,
		LockedAt:            version.LockedAt,
		AuditFields:         auditOf(version.Base),
	}
}

func PlanningVersionFromDTO(req dto.PlanningVersionRequest, companyID int64,
	existing *model.PlanningVersion) *model.PlanningVersion {

	version := existing
	if version == nil {
		version = &model.PlanningVersion{}
	}
	version.CompanyID = companyID
	version.SeasonID = req.SeasonID
	version.VersionNo = req.VersionNo
	version.VersionName = req.VersionName
	version.Description = req.Description
	version.Version = req.Version
	version.IsActive = true
	return version
}

func PlanHeaderToDTO(header model.PlanHeader) dto.PlanHeaderResponse {
	return dto.PlanHeaderResponse{
		ID:                header.ID,
		CompanyID:         header.CompanyID,
		SeasonID:          header.SeasonID,
		PlanningVersionID: header.PlanningVersionID,
		MovementTypeID:    header.MovementTypeID,
		ProductionLineID:  header.ProductionLineID,
		DocumentNo:        header.DocumentNo,
		Description:       header.Description,
		DateFrom:          dto.NewDate(header.DateFrom),
		DateTo:            dto.NewDate(header.DateTo),
		AuditFields:       auditOf(header.Base),
	}
}

func PlanItemToDTO(item model.PlanItem) dto.PlanItemResponse {
	return dto.PlanItemResponse{
		ID:               item.ID,
		PlanHeaderID:     item.PlanHeaderID,
		PlanDate:         dto.NewDate(item.PlanDate),
		ProductionLineID: item.ProductionLineID,
		MaterialID:       item.MaterialID,
		ProcessID:        item.ProcessID,
		WarehouseID:      item.WarehouseID,
		PackagingTypeID:  item.PackagingTypeID,
		Quantity:         item.Quantity,
		UOMID:            item.UOMID,
		Remark:           item.Remark,
		AuditFields:      auditOf(item.Base),
	}
}

func PlanItemFromDTO(req dto.PlanItemRequest, headerID int64) model.PlanItem {
	return model.PlanItem{
		PlanHeaderID:     headerID,
		PlanDate:         req.PlanDate.Time,
		ProductionLineID: req.ProductionLineID,
		MaterialID:       req.MaterialID,
		ProcessID:        req.ProcessID,
		WarehouseID:      req.WarehouseID,
		PackagingTypeID:  req.PackagingTypeID,
		Quantity:         req.Quantity,
		UOMID:            req.UOMID,
		Remark:           req.Remark,
	}
}

// MatrixSetRequestFromDTO flattens the row/value grids into the cell lists the
// service works with. A payload that names one product at the top level and a
// payload carrying a series array both arrive here as a set — the single
// product is simply a set of one.
func MatrixSetRequestFromDTO(req dto.MatrixSaveRequest, companyID int64) service.MatrixSetRequest {
	set := service.MatrixSetRequest{
		CompanyID:     companyID,
		SeasonID:      req.SeasonID,
		VersionID:     req.VersionID,
		DateFrom:      req.DateFrom.Time,
		DateTo:        req.DateTo.Time,
		PartialUpdate: req.PartialUpdate,
	}

	if req.MaterialID != 0 {
		set.Series = append(set.Series, service.MatrixSeriesInput{
			MovementTypeID: req.MovementTypeID,
			MaterialID:     req.MaterialID,
			ProcessID:      req.ProcessID,
			WarehouseID:    req.WarehouseID,
			UOMID:          req.UOMID,
			Cells:          matrixCells(req.Rows),
		})
	}
	for _, series := range req.Series {
		set.Series = append(set.Series, service.MatrixSeriesInput{
			MovementTypeID: series.MovementTypeID,
			MaterialID:     series.MaterialID,
			ProcessID:      series.ProcessID,
			WarehouseID:    series.WarehouseID,
			UOMID:          series.UOMID,
			Cells:          matrixCells(series.Rows),
		})
	}
	return set
}

func matrixCells(rows []dto.MatrixRowRequest) []service.MatrixCellInput {
	cells := make([]service.MatrixCellInput, 0)
	for _, row := range rows {
		for _, value := range row.Values {
			cells = append(cells, service.MatrixCellInput{
				PlanDate:         row.PlanDate.Time,
				ProductionLineID: value.ProductionLineID,
				Quantity:         value.Quantity,
			})
		}
	}
	return cells
}

// MatrixSetToDTO renders every product's grid under one set of dates and
// lines, which is what a screen showing the whole plan needs.
func MatrixSetToDTO(companyID, versionID int64, from, to time.Time,
	series []service.MatrixResponse) dto.MatrixSetResponse {

	out := dto.MatrixSetResponse{
		CompanyID: companyID,
		VersionID: versionID,
		DateFrom:  dto.NewDate(from),
		DateTo:    dto.NewDate(to),
		Series:    make([]dto.MatrixResponse, 0, len(series)),
	}
	for i := range series {
		grid := MatrixToDTO(&series[i])
		if i == 0 {
			out.VersionStatus = grid.VersionStatus
			out.Lines = grid.Lines
		}
		out.Series = append(out.Series, grid)
	}
	return out
}

func MatrixToDTO(matrix *service.MatrixResponse) dto.MatrixResponse {
	lines := make([]dto.MatrixLineResponse, 0, len(matrix.Lines))
	for _, line := range matrix.Lines {
		lines = append(lines, dto.MatrixLineResponse{
			ProductionLineID: line.ID,
			LineCode:         line.LineCode,
			LineName:         line.LineName,
		})
	}

	rows := make([]dto.MatrixRowResponse, 0, len(matrix.Rows))
	for _, row := range matrix.Rows {
		values := make([]dto.MatrixValueResponse, 0, len(row.Values))
		for _, value := range row.Values {
			values = append(values, dto.MatrixValueResponse{
				ProductionLineID: value.ProductionLineID,
				Quantity:         value.Quantity,
				UOMID:            value.UOMID,
				Version:          value.Version,
			})
		}
		rows = append(rows, dto.MatrixRowResponse{
			PlanDate: dto.NewDate(row.PlanDate),
			Values:   values,
		})
	}

	return dto.MatrixResponse{
		CompanyID:      matrix.CompanyID,
		VersionID:      matrix.VersionID,
		VersionStatus:  matrix.VersionStatus,
		MovementTypeID: matrix.MovementTypeID,
		MaterialID:     matrix.MaterialID,
		MaterialCode:   matrix.MaterialCode,
		MaterialName:   matrix.MaterialName,
		ProcessID:      matrix.ProcessID,
		ProcessCode:    matrix.ProcessCode,
		UOMID:          matrix.UOMID,
		UOMCode:        matrix.UOMCode,
		DateFrom:       dto.NewDate(matrix.DateFrom),
		DateTo:         dto.NewDate(matrix.DateTo),
		Lines:          lines,
		Rows:           rows,
	}
}

// --- actual --------------------------------------------------------------

func ActualHeaderToDTO(header model.ActualHeader, items []model.ActualItem) dto.ActualHeaderResponse {
	return dto.ActualHeaderResponse{
		ID:               header.ID,
		CompanyID:        header.CompanyID,
		SeasonID:         header.SeasonID,
		MovementTypeID:   header.MovementTypeID,
		ProductionLineID: header.ProductionLineID,
		DocumentNo:       header.DocumentNo,
		Description:      header.Description,
		PostingDate:      dto.NewDate(header.PostingDate),
		PostingStatus:    header.PostingStatus,
		PostedBy:         header.PostedBy,
		PostedAt:         header.PostedAt,
		ReversedBy:       header.ReversedBy,
		ReversedAt:       header.ReversedAt,
		Items:            Slice(items, ActualItemToDTO),
		AuditFields:      auditOf(header.Base),
	}
}

func ActualItemToDTO(item model.ActualItem) dto.ActualItemResponse {
	return dto.ActualItemResponse{
		ID:               item.ID,
		ActualHeaderID:   item.ActualHeaderID,
		ActualDate:       dto.NewDate(item.ActualDate),
		ProductionLineID: item.ProductionLineID,
		MaterialID:       item.MaterialID,
		ProcessID:        item.ProcessID,
		WarehouseID:      item.WarehouseID,
		PackagingTypeID:  item.PackagingTypeID,
		Quantity:         item.Quantity,
		UOMID:            item.UOMID,
		Shift:            item.Shift,
		Remark:           item.Remark,
		AuditFields:      auditOf(item.Base),
	}
}

func ActualHeaderFromDTO(req dto.ActualHeaderRequest, companyID int64,
	existing *model.ActualHeader) *model.ActualHeader {

	header := existing
	if header == nil {
		header = &model.ActualHeader{}
	}
	header.CompanyID = companyID
	header.SeasonID = req.SeasonID
	header.MovementTypeID = req.MovementTypeID
	header.ProductionLineID = req.ProductionLineID
	header.Description = req.Description
	header.PostingDate = req.PostingDate.Time
	header.Version = req.Version
	header.IsActive = true
	return header
}

func ActualItemFromDTO(req dto.ActualItemRequest) model.ActualItem {
	return model.ActualItem{
		ActualDate:       req.ActualDate.Time,
		ProductionLineID: req.ProductionLineID,
		MaterialID:       req.MaterialID,
		ProcessID:        req.ProcessID,
		WarehouseID:      req.WarehouseID,
		PackagingTypeID:  req.PackagingTypeID,
		Quantity:         req.Quantity,
		UOMID:            req.UOMID,
		Shift:            req.Shift,
		Remark:           req.Remark,
	}
}

// --- inventory -----------------------------------------------------------

func MovementToDTO(movement model.InventoryMovement) dto.MovementResponse {
	response := dto.MovementResponse{
		ID:                 movement.ID,
		CompanyID:          movement.CompanyID,
		DocumentNo:         movement.DocumentNo,
		LineNo:             movement.LineNo,
		TransactionDate:    dto.NewDate(movement.TransactionDate),
		MaterialID:         movement.MaterialID,
		WarehouseID:        movement.WarehouseID,
		MovementTypeID:     movement.MovementTypeID,
		PackagingTypeID:    movement.PackagingTypeID,
		ProcessID:          movement.ProcessID,
		Quantity:           movement.Quantity,
		UOMID:              movement.UOMID,
		ReferenceDocument:  movement.ReferenceDocument,
		ReferenceID:        movement.ReferenceID,
		SourceModule:       movement.SourceModule,
		TransferGroupID:    movement.TransferGroupID,
		ReversedMovementID: movement.ReversedMovementID,
		IsReversed:         movement.IsReversed,
		Remark:             movement.Remark,
		AuditFields:        auditOf(movement.Base),
	}
	if movement.MovementType != nil {
		response.MovementCode = movement.MovementType.MovementCode
		response.Direction = movement.MovementType.Direction
	}
	return response
}

func BalanceToDTO(row interfaces.BalanceRow) dto.BalanceResponse {
	return dto.BalanceResponse{
		CompanyID:       row.CompanyID,
		WarehouseID:     row.WarehouseID,
		WarehouseCode:   row.WarehouseCode,
		WarehouseName:   row.WarehouseName,
		MaterialID:      row.MaterialID,
		MaterialCode:    row.MaterialCode,
		MaterialName:    row.MaterialName,
		PackagingTypeID: row.PackagingTypeID,
		UOMCode:         row.UOMCode,
		OpeningQty:      row.OpeningQty,
		InQty:           row.InQty,
		OutQty:          row.OutQty,
		ClosingQty:      row.ClosingQty,
	}
}

func CapacityToDTO(status service.CapacityStatus) dto.CapacityResponse {
	return dto.CapacityResponse{
		WarehouseID:    status.Warehouse.ID,
		WarehouseCode:  status.Warehouse.WarehouseCode,
		WarehouseName:  status.Warehouse.WarehouseName,
		WarehouseType:  status.Warehouse.WarehouseType,
		Capacity:       status.Warehouse.Capacity,
		CapacityUOMID:  status.Warehouse.CapacityUOMID,
		CurrentStock:   status.CurrentStock,
		Available:      status.Available,
		UtilisationPct: status.Utilisation,
		TrafficLight:   status.TrafficLight,
	}
}

func DiscrepancyToDTO(d interfaces.BalanceDiscrepancy) dto.DiscrepancyResponse {
	return dto.DiscrepancyResponse{
		WarehouseID:     d.Key.WarehouseID,
		MaterialID:      d.Key.MaterialID,
		PackagingTypeID: d.Key.PackagingTypeID,
		BalanceDate:     dto.NewDate(d.Key.BalanceDate),
		StoredQty:       d.Stored,
		ComputedQty:     d.Computed,
	}
}

// --- reports -------------------------------------------------------------

func PlanVsActualToDTO(report *service.PlanVsActualReport) dto.PlanVsActualResponse {
	lines := make([]dto.PlanVsActualLineResponse, 0, len(report.Lines))
	for _, line := range report.Lines {
		lines = append(lines, dto.PlanVsActualLineResponse{
			CompanyID:         line.CompanyID,
			CompanyCode:       line.CompanyCode,
			GroupKey:          line.GroupKey,
			GroupLabel:        line.GroupLabel,
			UOMCode:           line.UOMCode,
			PlanQty:           line.PlanQty,
			ActualQty:         line.ActualQty,
			Variance:          line.Variance,
			VariancePct:       line.VariancePct,
			ConversionMissing: line.ConversionMissing,
		})
	}
	return dto.PlanVsActualResponse{
		GroupBy:       report.GroupBy,
		DateFrom:      dto.NewDate(report.DateFrom),
		DateTo:        dto.NewDate(report.DateTo),
		VersionID:     report.VersionID,
		TotalPlan:     report.TotalPlan,
		TotalActual:   report.TotalActual,
		TotalVariance: report.TotalActual.Sub(report.TotalPlan),
		Lines:         lines,
	}
}

func ProductionSummaryToDTO(row interfaces.ProductionSummaryRow) dto.ProductionSummaryResponse {
	return dto.ProductionSummaryResponse{
		MaterialID:   row.MaterialID,
		MaterialCode: row.MaterialCode,
		MaterialName: row.MaterialName,
		ProcessCode:  row.ProcessCode,
		UOMCode:      row.UOMCode,
		Quantity:     row.Quantity,
	}
}

func MovementReportToDTO(row interfaces.MovementReportRow) dto.MovementReportResponse {
	return dto.MovementReportResponse{
		TransactionDate: dto.NewDate(row.TransactionDate),
		DocumentNo:      row.DocumentNo,
		WarehouseCode:   row.WarehouseCode,
		MaterialCode:    row.MaterialCode,
		MovementCode:    row.MovementCode,
		Direction:       row.Direction,
		Quantity:        row.Quantity,
		UOMCode:         row.UOMCode,
		SourceModule:    row.SourceModule,
		IsReversed:      row.IsReversed,
	}
}

// CapacityRowToDTO converts the report row, computing the utilisation with the
// same rule as the inventory service: a location without a capacity reports a
// null utilisation, never zero.
func CapacityRowToDTO(row interfaces.CapacityRow, amber, red float64) dto.CapacityResponse {
	response := dto.CapacityResponse{
		WarehouseID:   row.WarehouseID,
		WarehouseCode: row.WarehouseCode,
		WarehouseName: row.WarehouseName,
		WarehouseType: row.WarehouseType,
		Capacity:      row.Capacity,
		CurrentStock:  row.CurrentStock,
		TrafficLight:  "NONE",
	}
	if row.Capacity != nil && row.Capacity.IsPositive() {
		available := row.Capacity.Sub(row.CurrentStock)
		utilisation := row.CurrentStock.Div(*row.Capacity).Mul(decimal.NewFromInt(100)).Round(2)
		response.Available = &available
		response.UtilisationPct = &utilisation

		value, _ := utilisation.Float64()
		switch {
		case value > red:
			response.TrafficLight = "RED"
		case value >= amber:
			response.TrafficLight = "AMBER"
		default:
			response.TrafficLight = "GREEN"
		}
	}
	return response
}

// --- data dictionary -----------------------------------------------------

func DDTableToDTO(table model.DDTable) dto.DDTableResponse {
	return dto.DDTableResponse{
		ID:                  table.ID,
		TableName:           table.TableNameCol,
		Module:              table.Module,
		TableType:           table.TableType,
		DescriptionEN:       table.DescriptionEN,
		DescriptionLocal:    table.DescriptionLocal,
		BusinessDescription: table.BusinessDescription,
		IsCompanyDependent:  table.IsCompanyDependent,
		IsBrowsable:         table.IsBrowsable,
		AuditFields:         auditOf(table.Base),
	}
}

func DDFieldToDTO(field model.DDField) dto.DDFieldResponse {
	return dto.DDFieldResponse{
		ID:                  field.ID,
		TableID:             field.TableID,
		FieldName:           field.FieldName,
		Position:            field.Position,
		LabelShort:          field.LabelShort,
		LabelMedium:         field.LabelMedium,
		LabelLong:           field.LabelLong,
		DataType:            field.DataType,
		Length:              field.Length,
		Decimals:            field.Decimals,
		IsKey:               field.IsKey,
		IsRequired:          field.IsRequired,
		IsPII:               field.IsPII,
		IsBrowsable:         field.IsBrowsable,
		ValueHelpID:         field.ValueHelpID,
		BusinessDescription: field.BusinessDescription,
		AuditFields:         auditOf(field.Base),
	}
}

func DDDomainToDTO(domain model.DDDomain) dto.DDDomainResponse {
	return dto.DDDomainResponse{
		ID:          domain.ID,
		DomainName:  domain.DomainName,
		DataType:    domain.DataType,
		Length:      domain.Length,
		Decimals:    domain.Decimals,
		Description: domain.Description,
	}
}

func NumberRangeToDTO(rng model.NumberRange) dto.NumberRangeResponse {
	return dto.NumberRangeResponse{
		ID:          rng.ID,
		CompanyID:   rng.CompanyID,
		ObjectType:  rng.ObjectType,
		FiscalYear:  rng.FiscalYear,
		Prefix:      rng.Prefix,
		CurrentNo:   rng.CurrentNo,
		Length:      rng.Length,
		AuditFields: auditOf(rng.Base),
	}
}

func AuditLogToDTO(entry model.AuditLog) dto.AuditLogResponse {
	return dto.AuditLogResponse{
		ID:        entry.ID,
		TableName: entry.TableName_,
		RecordID:  entry.RecordID,
		CompanyID: entry.CompanyID,
		Action:    entry.Action,
		ChangedBy: entry.ChangedBy,
		ChangedAt: entry.ChangedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		OldValues: rawJSON(entry.OldValues),
		NewValues: rawJSON(entry.NewValues),
		RequestID: entry.RequestID,
		IPAddress: entry.IPAddress,
	}
}

// rawJSON passes stored JSONB through untouched instead of re-encoding it as
// a quoted string.
func rawJSON(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	return json.RawMessage(raw)
}
