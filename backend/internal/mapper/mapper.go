// Package mapper converts between GORM models and DTOs. Per §A2 rule 5 this is
// the only place mapping happens — controllers and services call into it and
// never hand-roll a conversion.
package mapper

import (
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/dto"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
)

func auditOf(base model.Base) dto.AuditFields {
	return dto.AuditFields{
		IsActive:  base.IsActive,
		Version:   base.Version,
		CreatedBy: base.CreatedBy,
		CreatedAt: base.CreatedAt,
		ChangedBy: base.ChangedBy,
		ChangedAt: base.ChangedAt,
	}
}

// Slice maps a list with a per-element function, which keeps every list
// endpoint a one-liner.
func Slice[T any, R any](rows []T, fn func(T) R) []R {
	out := make([]R, 0, len(rows))
	for _, row := range rows {
		out = append(out, fn(row))
	}
	return out
}

// --- companies -----------------------------------------------------------

func CompanyToDTO(company model.Company) dto.CompanyResponse {
	return dto.CompanyResponse{
		ID:                company.ID,
		CompanyCode:       company.CompanyCode,
		CompanyName:       company.CompanyName,
		LocalCurrency:     company.LocalCurrency,
		GroupCurrency:     company.GroupCurrency,
		Timezone:          company.Timezone,
		FiscalYearVariant: company.FiscalYearVariant,
		CountryCode:       company.CountryCode,
		AuditFields:       auditOf(company.Base),
	}
}

func CompanyFromDTO(req dto.CompanyRequest, existing *model.Company) *model.Company {
	company := existing
	if company == nil {
		company = &model.Company{}
	}
	company.CompanyCode = req.CompanyCode
	company.CompanyName = req.CompanyName
	company.LocalCurrency = req.LocalCurrency
	company.GroupCurrency = req.GroupCurrency
	company.Timezone = req.Timezone
	company.FiscalYearVariant = req.FiscalYearVariant
	company.CountryCode = req.CountryCode
	company.Version = req.Version
	company.IsActive = true
	return company
}

// --- users, roles, permissions -------------------------------------------

func UserToDTO(user model.User) dto.UserResponse {
	return dto.UserResponse{
		ID:                 user.ID,
		Username:           user.Username,
		Email:              user.Email,
		FullName:           user.FullName,
		Locale:             user.Locale,
		IsLocked:           user.IsLocked,
		MustChangePassword: user.MustChangePassword,
		LastLoginAt:        user.LastLoginAt,
		AuditFields:        auditOf(user.Base),
	}
}

func RoleToDTO(role model.Role) dto.RoleResponse {
	permissions := make([]string, 0, len(role.Permissions))
	for _, permission := range role.Permissions {
		permissions = append(permissions, permission.PermissionCode)
	}
	return dto.RoleResponse{
		ID:          role.ID,
		RoleCode:    role.RoleCode,
		RoleName:    role.RoleName,
		Description: role.Description,
		Permissions: permissions,
		AuditFields: auditOf(role.Base),
	}
}

func PermissionToDTO(permission model.Permission) dto.PermissionResponse {
	return dto.PermissionResponse{
		ID:             permission.ID,
		PermissionCode: permission.PermissionCode,
		Module:         permission.Module,
		Description:    permission.Description,
	}
}

// --- units, materials, packaging -----------------------------------------

func UOMToDTO(uom model.UOM) dto.UOMResponse {
	return dto.UOMResponse{
		ID:          uom.ID,
		UOMCode:     uom.UOMCode,
		UOMName:     uom.UOMName,
		Dimension:   uom.Dimension,
		Decimals:    uom.Decimals,
		AuditFields: auditOf(uom.Base),
	}
}

func UOMFromDTO(req dto.UOMRequest, existing *model.UOM) *model.UOM {
	uom := existing
	if uom == nil {
		uom = &model.UOM{}
	}
	uom.UOMCode = req.UOMCode
	uom.UOMName = req.UOMName
	uom.Dimension = req.Dimension
	uom.Decimals = req.Decimals
	uom.Version = req.Version
	uom.IsActive = true
	return uom
}

func MaterialToDTO(material model.Material) dto.MaterialResponse {
	response := dto.MaterialResponse{
		ID:                   material.ID,
		MaterialCode:         material.MaterialCode,
		MaterialName:         material.MaterialName,
		MaterialType:         material.MaterialType,
		MaterialGroup:        material.MaterialGroup,
		BaseUOMID:            material.BaseUOMID,
		ConditioningRequired: material.ConditioningRequired,
		IsStockManaged:       material.IsStockManaged,
		AuditFields:          auditOf(material.Base),
	}
	if material.BaseUOM != nil {
		response.BaseUOMCode = material.BaseUOM.UOMCode
	}
	return response
}

func MaterialFromDTO(req dto.MaterialRequest, existing *model.Material) *model.Material {
	material := existing
	if material == nil {
		material = &model.Material{}
	}
	material.MaterialCode = req.MaterialCode
	material.MaterialName = req.MaterialName
	material.MaterialType = req.MaterialType
	material.MaterialGroup = req.MaterialGroup
	material.BaseUOMID = req.BaseUOMID
	material.ConditioningRequired = req.ConditioningRequired
	material.IsStockManaged = req.IsStockManaged
	material.Version = req.Version
	material.IsActive = true
	return material
}

func CompanyMaterialToDTO(cm model.CompanyMaterial) dto.CompanyMaterialResponse {
	response := dto.CompanyMaterialResponse{
		ID:                cm.ID,
		CompanyID:         cm.CompanyID,
		MaterialID:        cm.MaterialID,
		PlanningEnabled:   cm.PlanningEnabled,
		ProductionEnabled: cm.ProductionEnabled,
		InventoryEnabled:  cm.InventoryEnabled,
		SalesEnabled:      cm.SalesEnabled,
		AuditFields:       auditOf(cm.Base),
	}
	if cm.Material != nil {
		material := MaterialToDTO(*cm.Material)
		response.Material = &material
	}
	return response
}

func PackagingTypeToDTO(pt model.PackagingType) dto.PackagingTypeResponse {
	return dto.PackagingTypeResponse{
		ID:              pt.ID,
		PackagingCode:   pt.PackagingCode,
		PackagingName:   pt.PackagingName,
		NominalQuantity: pt.NominalQuantity,
		NominalUOMID:    pt.NominalUOMID,
		IsBulk:          pt.IsBulk,
		AuditFields:     auditOf(pt.Base),
	}
}

func PackagingTypeFromDTO(req dto.PackagingTypeRequest, existing *model.PackagingType) *model.PackagingType {
	pt := existing
	if pt == nil {
		pt = &model.PackagingType{}
	}
	pt.PackagingCode = req.PackagingCode
	pt.PackagingName = req.PackagingName
	pt.NominalQuantity = req.NominalQuantity
	pt.NominalUOMID = req.NominalUOMID
	pt.IsBulk = req.IsBulk
	pt.Version = req.Version
	pt.IsActive = true
	return pt
}

// --- storage and lines ---------------------------------------------------

func WarehouseToDTO(warehouse model.Warehouse, allowedMaterials []int64) dto.WarehouseResponse {
	return dto.WarehouseResponse{
		ID:                 warehouse.ID,
		CompanyID:          warehouse.CompanyID,
		WarehouseCode:      warehouse.WarehouseCode,
		WarehouseName:      warehouse.WarehouseName,
		WarehouseType:      warehouse.WarehouseType,
		Capacity:           warehouse.Capacity,
		CapacityUOMID:      warehouse.CapacityUOMID,
		AllowNegativeStock: warehouse.AllowNegativeStock,
		Location:           warehouse.Location,
		AllowedMaterialIDs: allowedMaterials,
		AuditFields:        auditOf(warehouse.Base),
	}
}

func WarehouseFromDTO(req dto.WarehouseRequest, companyID int64, existing *model.Warehouse) *model.Warehouse {
	warehouse := existing
	if warehouse == nil {
		warehouse = &model.Warehouse{}
	}
	warehouse.CompanyID = companyID
	warehouse.WarehouseCode = req.WarehouseCode
	warehouse.WarehouseName = req.WarehouseName
	warehouse.WarehouseType = req.WarehouseType
	warehouse.Capacity = req.Capacity
	warehouse.CapacityUOMID = req.CapacityUOMID
	warehouse.AllowNegativeStock = req.AllowNegativeStock
	warehouse.Location = req.Location
	warehouse.Version = req.Version
	warehouse.IsActive = true
	return warehouse
}

func ProductionLineToDTO(line model.ProductionLine) dto.ProductionLineResponse {
	return dto.ProductionLineResponse{
		ID:             line.ID,
		CompanyID:      line.CompanyID,
		LineCode:       line.LineCode,
		LineName:       line.LineName,
		CapacityPerDay: line.CapacityPerDay,
		CapacityUOMID:  line.CapacityUOMID,
		AuditFields:    auditOf(line.Base),
	}
}

func ProductionLineFromDTO(req dto.ProductionLineRequest, companyID int64, existing *model.ProductionLine) *model.ProductionLine {
	line := existing
	if line == nil {
		line = &model.ProductionLine{}
	}
	line.CompanyID = companyID
	line.LineCode = req.LineCode
	line.LineName = req.LineName
	line.CapacityPerDay = req.CapacityPerDay
	line.CapacityUOMID = req.CapacityUOMID
	line.Version = req.Version
	line.IsActive = true
	return line
}

func SeasonToDTO(season model.Season) dto.SeasonResponse {
	return dto.SeasonResponse{
		ID:          season.ID,
		CompanyID:   season.CompanyID,
		SeasonCode:  season.SeasonCode,
		SeasonName:  season.SeasonName,
		StartDate:   dto.NewDate(season.StartDate),
		EndDate:     dto.NewDate(season.EndDate),
		Status:      season.Status,
		AuditFields: auditOf(season.Base),
	}
}

func SeasonFromDTO(req dto.SeasonRequest, companyID int64, existing *model.Season) *model.Season {
	season := existing
	if season == nil {
		season = &model.Season{}
	}
	season.CompanyID = companyID
	season.SeasonCode = req.SeasonCode
	season.SeasonName = req.SeasonName
	season.StartDate = req.StartDate.Time
	season.EndDate = req.EndDate.Time
	if req.Status != "" {
		season.Status = req.Status
	} else if season.Status == "" {
		season.Status = model.SeasonStatusPlanning
	}
	season.Version = req.Version
	season.IsActive = true
	return season
}

// --- processes and movement types ----------------------------------------

func ProcessToDTO(process model.Process, materials []model.ProcessMaterial) dto.ProcessResponse {
	return dto.ProcessResponse{
		ID:          process.ID,
		ProcessCode: process.ProcessCode,
		ProcessName: process.ProcessName,
		SequenceNo:  process.SequenceNo,
		Description: process.Description,
		Materials:   Slice(materials, ProcessMaterialToDTO),
		AuditFields: auditOf(process.Base),
	}
}

func ProcessFromDTO(req dto.ProcessRequest, existing *model.Process) *model.Process {
	process := existing
	if process == nil {
		process = &model.Process{}
	}
	process.ProcessCode = req.ProcessCode
	process.ProcessName = req.ProcessName
	process.SequenceNo = req.SequenceNo
	process.Description = req.Description
	process.Version = req.Version
	process.IsActive = true
	return process
}

func ProcessMaterialToDTO(pm model.ProcessMaterial) dto.ProcessMaterialResponse {
	response := dto.ProcessMaterialResponse{
		ID:                   pm.ID,
		ProcessID:            pm.ProcessID,
		MaterialID:           pm.MaterialID,
		IOType:               pm.IOType,
		RequiresConditioning: pm.RequiresConditioning,
		ExpectedYieldPct:     pm.ExpectedYieldPct,
	}
	if pm.Material != nil {
		response.MaterialCode = pm.Material.MaterialCode
		response.MaterialName = pm.Material.MaterialName
	}
	return response
}

func ProcessMaterialFromDTO(req dto.ProcessMaterialRequest, processID int64) model.ProcessMaterial {
	return model.ProcessMaterial{
		ProcessID:            processID,
		MaterialID:           req.MaterialID,
		IOType:               req.IOType,
		RequiresConditioning: req.RequiresConditioning,
		ExpectedYieldPct:     req.ExpectedYieldPct,
	}
}

func MovementTypeToDTO(mt model.MovementType) dto.MovementTypeResponse {
	return dto.MovementTypeResponse{
		ID:                 mt.ID,
		MovementCode:       mt.MovementCode,
		MovementName:       mt.MovementName,
		Direction:          mt.Direction,
		AffectsStock:       mt.AffectsStock,
		IsPlanningRelevant: mt.IsPlanningRelevant,
		IsTransfer:         mt.IsTransfer,
		IsAdjustment:       mt.IsAdjustment,
		CounterpartID:      mt.CounterpartID,
		AuditFields:        auditOf(mt.Base),
	}
}

func MovementTypeFromDTO(req dto.MovementTypeRequest, existing *model.MovementType) *model.MovementType {
	mt := existing
	if mt == nil {
		mt = &model.MovementType{}
	}
	mt.MovementCode = req.MovementCode
	mt.MovementName = req.MovementName
	mt.Direction = req.Direction
	mt.AffectsStock = req.AffectsStock
	mt.IsPlanningRelevant = req.IsPlanningRelevant
	mt.IsTransfer = req.IsTransfer
	mt.IsAdjustment = req.IsAdjustment
	mt.CounterpartID = req.CounterpartID
	mt.Version = req.Version
	mt.IsActive = true
	return mt
}
