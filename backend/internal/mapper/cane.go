package mapper

import (
	"time"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/dto"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/service"
)

// --- cane varieties ------------------------------------------------------

func CaneVarietyToDTO(v model.CaneVariety) dto.CaneVarietyResponse {
	return dto.CaneVarietyResponse{
		ID:             v.ID,
		VarietyCode:    v.VarietyCode,
		VarietyName:    v.VarietyName,
		MaturityMonths: v.MaturityMonths,
		TypicalCCSPct:  v.TypicalCCSPct,
		Description:    v.Description,
		AuditFields:    auditOf(v.Base),
	}
}

func CaneVarietyFromDTO(req dto.CaneVarietyRequest, existing *model.CaneVariety) *model.CaneVariety {
	variety := existing
	if variety == nil {
		variety = &model.CaneVariety{}
	}
	variety.VarietyCode = req.VarietyCode
	variety.VarietyName = req.VarietyName
	variety.MaturityMonths = req.MaturityMonths
	variety.TypicalCCSPct = req.TypicalCCSPct
	variety.Description = req.Description
	variety.Version = req.Version
	return variety
}

// --- growers -------------------------------------------------------------

func GrowerToDTO(g model.Grower) dto.GrowerResponse {
	return dto.GrowerResponse{
		ID:           g.ID,
		CompanyID:    g.CompanyID,
		GrowerCode:   g.GrowerCode,
		GrowerName:   g.GrowerName,
		GrowerType:   g.GrowerType,
		SupplyType:   g.SupplyType,
		Zone:         g.Zone,
		ContactName:  g.ContactName,
		ContactPhone: g.ContactPhone,
		ContractTons: g.ContractTons,
		ContractNo:   g.ContractNo,
		PricePerTon:  g.PricePerTon,
		Currency:     g.Currency,
		TransportKm:  g.TransportKm,
		AuditFields:  auditOf(g.Base),
	}
}

func GrowerFromDTO(req dto.GrowerRequest, companyID int64, existing *model.Grower) *model.Grower {
	grower := existing
	if grower == nil {
		grower = &model.Grower{}
	}
	grower.CompanyID = companyID
	grower.GrowerCode = req.GrowerCode
	grower.GrowerName = req.GrowerName
	grower.GrowerType = req.GrowerType
	grower.SupplyType = req.SupplyType
	grower.Zone = req.Zone
	grower.ContactName = req.ContactName
	grower.ContactPhone = req.ContactPhone
	grower.ContractTons = req.ContractTons
	grower.ContractNo = req.ContractNo
	grower.PricePerTon = req.PricePerTon
	grower.Currency = req.Currency
	grower.TransportKm = req.TransportKm
	grower.Version = req.Version
	return grower
}

// --- cane fields ---------------------------------------------------------

func CaneFieldToDTO(f model.CaneField) dto.CaneFieldResponse {
	out := dto.CaneFieldResponse{
		ID:               f.ID,
		CompanyID:        f.CompanyID,
		GrowerID:         f.GrowerID,
		VarietyID:        f.VarietyID,
		FieldCode:        f.FieldCode,
		FieldName:        f.FieldName,
		AreaHa:           f.AreaHa,
		ExpectedYieldTPH: f.ExpectedYieldTPH,
		EstimatedTons:    f.EstimatedTons(),
		CropCycle:        f.CropCycle,
		Zone:             f.Zone,
		IsIrrigated:      f.IsIrrigated,
		AuditFields:      auditOf(f.Base),
	}
	if f.PlantingDate != nil {
		date := dto.NewDate(*f.PlantingDate)
		out.PlantingDate = &date
	}
	if f.ExpectedHarvestFrom != nil {
		date := dto.NewDate(*f.ExpectedHarvestFrom)
		out.ExpectedHarvestFrom = &date
	}
	if f.ExpectedHarvestTo != nil {
		date := dto.NewDate(*f.ExpectedHarvestTo)
		out.ExpectedHarvestTo = &date
	}
	return out
}

func CaneFieldFromDTO(req dto.CaneFieldRequest, companyID int64, existing *model.CaneField) *model.CaneField {
	field := existing
	if field == nil {
		field = &model.CaneField{}
	}
	field.CompanyID = companyID
	field.GrowerID = req.GrowerID
	field.VarietyID = req.VarietyID
	field.FieldCode = req.FieldCode
	field.FieldName = req.FieldName
	field.AreaHa = req.AreaHa
	field.ExpectedYieldTPH = req.ExpectedYieldTPH
	field.CropCycle = req.CropCycle
	field.Zone = req.Zone
	field.IsIrrigated = req.IsIrrigated
	field.Version = req.Version
	field.PlantingDate = dateValue(req.PlantingDate)
	field.ExpectedHarvestFrom = dateValue(req.ExpectedHarvestFrom)
	field.ExpectedHarvestTo = dateValue(req.ExpectedHarvestTo)
	return field
}

// --- the harvest matrix --------------------------------------------------

func HarvestMatrixRequestFromDTO(req dto.HarvestMatrixSaveRequest, companyID int64) service.HarvestMatrixRequest {
	out := service.HarvestMatrixRequest{
		CompanyID:     companyID,
		VersionID:     req.VersionID,
		UOMID:         req.UOMID,
		DateFrom:      req.DateFrom.Time,
		DateTo:        req.DateTo.Time,
		PartialUpdate: req.PartialUpdate,
	}
	for _, row := range req.Rows {
		for _, value := range row.Values {
			out.Cells = append(out.Cells, service.HarvestCellInput{
				PlanDate:       row.PlanDate.Time,
				GrowerID:       value.GrowerID,
				CaneFieldID:    value.CaneFieldID,
				VarietyID:      value.VarietyID,
				PlannedTons:    value.PlannedTons,
				PlannedAreaHa:  value.PlannedAreaHa,
				ExpectedCCSPct: value.ExpectedCCSPct,
				Remark:         value.Remark,
			})
		}
	}
	return out
}

func HarvestMatrixToDTO(matrix *service.HarvestMatrixResponse) dto.HarvestMatrixResponse {
	out := dto.HarvestMatrixResponse{
		CompanyID:     matrix.CompanyID,
		VersionID:     matrix.VersionID,
		VersionStatus: matrix.VersionStatus,
		DocumentNo:    matrix.DocumentNo,
		DateFrom:      dto.NewDate(matrix.DateFrom),
		DateTo:        dto.NewDate(matrix.DateTo),
		Growers:       make([]dto.HarvestGrowerResponse, 0, len(matrix.Growers)),
		Rows:          make([]dto.HarvestRowResponse, 0, len(matrix.Rows)),
	}
	for _, grower := range matrix.Growers {
		out.Growers = append(out.Growers, dto.HarvestGrowerResponse{
			GrowerID:   grower.ID,
			GrowerCode: grower.GrowerCode,
			GrowerName: grower.GrowerName,
			SupplyType: grower.SupplyType,
			Zone:       grower.Zone,
		})
	}
	for _, row := range matrix.Rows {
		values := make([]dto.HarvestValueResponse, 0, len(row.Values))
		for _, value := range row.Values {
			values = append(values, dto.HarvestValueResponse{
				GrowerID:       value.GrowerID,
				CaneFieldID:    value.CaneFieldID,
				VarietyID:      value.VarietyID,
				PlannedTons:    value.PlannedTons,
				PlannedAreaHa:  value.PlannedAreaHa,
				ExpectedCCSPct: value.ExpectedCCSPct,
				UOMID:          value.UOMID,
				Version:        value.Version,
			})
		}
		out.Rows = append(out.Rows, dto.HarvestRowResponse{
			PlanDate: dto.NewDate(row.PlanDate),
			Values:   values,
		})
	}
	return out
}

func HarvestPlanHeaderToDTO(h model.HarvestPlanHeader) dto.HarvestPlanHeaderResponse {
	return dto.HarvestPlanHeaderResponse{
		ID:                h.ID,
		CompanyID:         h.CompanyID,
		SeasonID:          h.SeasonID,
		PlanningVersionID: h.PlanningVersionID,
		DocumentNo:        h.DocumentNo,
		Description:       h.Description,
		DateFrom:          dto.NewDate(h.DateFrom),
		DateTo:            dto.NewDate(h.DateTo),
		AuditFields:       auditOf(h.Base),
	}
}

func HarvestPlanItemToDTO(i model.HarvestPlanItem) dto.HarvestPlanItemResponse {
	return dto.HarvestPlanItemResponse{
		ID:             i.ID,
		PlanDate:       dto.NewDate(i.PlanDate),
		GrowerID:       i.GrowerID,
		CaneFieldID:    i.CaneFieldID,
		VarietyID:      i.VarietyID,
		PlannedTons:    i.PlannedTons,
		UOMID:          i.UOMID,
		PlannedAreaHa:  i.PlannedAreaHa,
		ExpectedCCSPct: i.ExpectedCCSPct,
		Remark:         i.Remark,
		AuditFields:    auditOf(i.Base),
	}
}

// --- cane deliveries -----------------------------------------------------

func CaneDeliveryToDTO(d model.CaneDelivery) dto.CaneDeliveryResponse {
	return dto.CaneDeliveryResponse{
		ID:             d.ID,
		CompanyID:      d.CompanyID,
		SeasonID:       d.SeasonID,
		DocumentNo:     d.DocumentNo,
		DeliveryDate:   dto.NewDate(d.DeliveryDate),
		GrowerID:       d.GrowerID,
		CaneFieldID:    d.CaneFieldID,
		VarietyID:      d.VarietyID,
		MaterialID:     d.MaterialID,
		MovementTypeID: d.MovementTypeID,
		WarehouseID:    d.WarehouseID,
		UOMID:          d.UOMID,
		TicketNo:       d.TicketNo,
		VehicleNo:      d.VehicleNo,
		GrossTons:      d.GrossTons,
		TareTons:       d.TareTons,
		NetTons:        d.NetTons,
		PricePerTon:    d.PricePerTon,
		Currency:       d.Currency,
		Value:          d.Value(),
		CCSPct:         d.CCSPct,
		TrashPct:       d.TrashPct,
		IsBurnt:        d.IsBurnt,
		PostingStatus:  d.PostingStatus,
		PostedBy:       d.PostedBy,
		PostedAt:       d.PostedAt,
		ReversedBy:     d.ReversedBy,
		ReversedAt:     d.ReversedAt,
		Remark:         d.Remark,
		AuditFields:    auditOf(d.Base),
	}
}

func CaneDeliveryFromDTO(req dto.CaneDeliveryRequest, companyID int64, existing *model.CaneDelivery) *model.CaneDelivery {
	delivery := existing
	if delivery == nil {
		delivery = &model.CaneDelivery{}
	}
	delivery.CompanyID = companyID
	delivery.SeasonID = req.SeasonID
	delivery.DeliveryDate = req.DeliveryDate.Time
	delivery.GrowerID = req.GrowerID
	delivery.CaneFieldID = req.CaneFieldID
	delivery.VarietyID = req.VarietyID
	delivery.MaterialID = req.MaterialID
	delivery.MovementTypeID = req.MovementTypeID
	delivery.WarehouseID = req.WarehouseID
	delivery.UOMID = req.UOMID
	delivery.TicketNo = req.TicketNo
	delivery.VehicleNo = req.VehicleNo
	delivery.GrossTons = req.GrossTons
	delivery.TareTons = req.TareTons
	delivery.PricePerTon = req.PricePerTon
	delivery.Currency = req.Currency
	delivery.CCSPct = req.CCSPct
	delivery.TrashPct = req.TrashPct
	delivery.IsBurnt = req.IsBurnt
	delivery.Remark = req.Remark
	delivery.Version = req.Version
	return delivery
}

// --- cane plan vs actual -------------------------------------------------

func CanePlanVsActualToDTO(row service.CanePlanVsActualRow) dto.CanePlanVsActualRow {
	out := dto.CanePlanVsActualRow{
		GroupKey:      row.GroupKey,
		GroupLabel:    row.GroupLabel,
		SupplyType:    row.SupplyType,
		GrowerID:      row.GrowerID,
		GrowerCode:    row.GrowerCode,
		VarietyID:     row.VarietyID,
		VarietyCode:   row.VarietyCode,
		PlannedTons:   row.PlannedTons,
		ActualTons:    row.ActualTons,
		Deliveries:    row.Deliveries,
		VariancePct:   row.VariancePct,
		AverageCCSPct: row.AverageCCSPct,
		PurchaseValue: row.PurchaseValue,
		Currency:      row.Currency,
	}
	if row.PlanDate != nil {
		date := dto.NewDate(*row.PlanDate)
		out.PlanDate = &date
	}
	return out
}

// dateValue converts an optional request date to the optional business date
// the model holds.
func dateValue(d *dto.Date) *time.Time {
	if d == nil || d.Time.IsZero() {
		return nil
	}
	value := d.Time
	return &value
}
