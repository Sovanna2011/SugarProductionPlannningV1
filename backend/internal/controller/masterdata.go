package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/dto"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/mapper"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/service"
)

// MasterDataController serves §E2. Company-dependent routes carry the company
// in the path, so the authorization middleware has already validated it by the
// time a handler runs.
type MasterDataController struct {
	master *service.MasterDataService
}

func NewMasterDataController(master *service.MasterDataService) *MasterDataController {
	return &MasterDataController{master: master}
}

// --- companies -----------------------------------------------------------

func (ctl *MasterDataController) ListCompanies(c *gin.Context) {
	opts, ok := ListOptions(c, nil)
	if !ok {
		return
	}
	page, err := ctl.master.ListCompanies(c.Request.Context(), opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.CompanyToDTO), opts, page.Total)
}

func (ctl *MasterDataController) GetCompany(c *gin.Context) {
	id, ok := PathID(c, "companyId")
	if !ok {
		return
	}
	company, err := ctl.master.GetCompany(c.Request.Context(), id)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.CompanyToDTO(*company))
}

func (ctl *MasterDataController) CreateCompany(c *gin.Context) {
	req, ok := Bind[dto.CompanyRequest](c)
	if !ok {
		return
	}
	company := mapper.CompanyFromDTO(req, nil)
	if err := ctl.master.CreateCompany(c.Request.Context(), company); err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.CompanyToDTO(*company))
}

func (ctl *MasterDataController) UpdateCompany(c *gin.Context) {
	id, ok := PathID(c, "companyId")
	if !ok {
		return
	}
	req, ok := Bind[dto.CompanyRequest](c)
	if !ok {
		return
	}
	company := mapper.CompanyFromDTO(req, &model.Company{})
	company.ID = id
	if err := ctl.master.UpdateCompany(c.Request.Context(), company); err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.CompanyToDTO(*company))
}

// --- materials -----------------------------------------------------------

func (ctl *MasterDataController) ListMaterials(c *gin.Context) {
	opts, ok := ListOptions(c, nil)
	if !ok {
		return
	}
	page, err := ctl.master.ListMaterials(c.Request.Context(), opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.MaterialToDTO), opts, page.Total)
}

func (ctl *MasterDataController) GetMaterial(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	material, err := ctl.master.GetMaterial(c.Request.Context(), id)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.MaterialToDTO(*material))
}

func (ctl *MasterDataController) CreateMaterial(c *gin.Context) {
	req, ok := Bind[dto.MaterialRequest](c)
	if !ok {
		return
	}
	material := mapper.MaterialFromDTO(req, nil)
	if err := ctl.master.CreateMaterial(c.Request.Context(), material); err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.MaterialToDTO(*material))
}

func (ctl *MasterDataController) UpdateMaterial(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	req, ok := Bind[dto.MaterialRequest](c)
	if !ok {
		return
	}
	material := mapper.MaterialFromDTO(req, &model.Material{})
	material.ID = id
	if err := ctl.master.UpdateMaterial(c.Request.Context(), material); err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.MaterialToDTO(*material))
}

// DeleteMaterial deactivates rather than deletes: master data is never removed
// physically (§B1).
func (ctl *MasterDataController) DeleteMaterial(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	version, ok := QueryID(c, "version")
	if !ok {
		return
	}
	if version == nil {
		Fail(c, versionRequired())
		return
	}
	if err := ctl.master.DeactivateMaterial(c.Request.Context(), id, int(*version)); err != nil {
		Fail(c, err)
		return
	}
	NoContent(c)
}

func (ctl *MasterDataController) ListCompanyMaterials(c *gin.Context) {
	companyID := CompanyID(c)
	opts, ok := ListOptions(c, &companyID)
	if !ok {
		return
	}
	page, err := ctl.master.ListCompanyMaterials(c.Request.Context(), companyID, opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.CompanyMaterialToDTO), opts, page.Total)
}

func (ctl *MasterDataController) UpsertCompanyMaterial(c *gin.Context) {
	req, ok := Bind[dto.CompanyMaterialRequest](c)
	if !ok {
		return
	}
	cm := &model.CompanyMaterial{
		CompanyID:         CompanyID(c),
		MaterialID:        req.MaterialID,
		PlanningEnabled:   req.PlanningEnabled,
		ProductionEnabled: req.ProductionEnabled,
		InventoryEnabled:  req.InventoryEnabled,
		SalesEnabled:      req.SalesEnabled,
	}
	cm.IsActive = true
	cm.Version = 1
	if err := ctl.master.UpsertCompanyMaterial(c.Request.Context(), cm); err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.CompanyMaterialToDTO(*cm))
}

// --- warehouses ----------------------------------------------------------

func (ctl *MasterDataController) ListWarehouses(c *gin.Context) {
	companyID := CompanyID(c)
	opts, ok := ListOptions(c, &companyID)
	if !ok {
		return
	}
	page, err := ctl.master.ListWarehouses(c.Request.Context(), opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, func(w model.Warehouse) dto.WarehouseResponse {
		return mapper.WarehouseToDTO(w, nil)
	}), opts, page.Total)
}

func (ctl *MasterDataController) GetWarehouse(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	companyID := CompanyID(c)
	warehouse, err := ctl.master.GetWarehouse(c.Request.Context(), companyID, id)
	if err != nil {
		Fail(c, err)
		return
	}
	allowed, err := ctl.master.WarehouseAllowedMaterials(c.Request.Context(), id)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.WarehouseToDTO(*warehouse, allowed))
}

func (ctl *MasterDataController) CreateWarehouse(c *gin.Context) {
	req, ok := Bind[dto.WarehouseRequest](c)
	if !ok {
		return
	}
	warehouse := mapper.WarehouseFromDTO(req, CompanyID(c), nil)
	err := ctl.master.CreateWarehouse(c.Request.Context(), warehouse, req.AllowedMaterialIDs)
	if err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.WarehouseToDTO(*warehouse, req.AllowedMaterialIDs))
}

func (ctl *MasterDataController) UpdateWarehouse(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	req, ok := Bind[dto.WarehouseRequest](c)
	if !ok {
		return
	}
	warehouse := mapper.WarehouseFromDTO(req, CompanyID(c), &model.Warehouse{})
	warehouse.ID = id
	err := ctl.master.UpdateWarehouse(c.Request.Context(), warehouse, req.AllowedMaterialIDs)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.WarehouseToDTO(*warehouse, req.AllowedMaterialIDs))
}

func (ctl *MasterDataController) DeleteWarehouse(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	version, ok := QueryID(c, "version")
	if !ok {
		return
	}
	if version == nil {
		Fail(c, versionRequired())
		return
	}
	err := ctl.master.DeactivateWarehouse(c.Request.Context(), CompanyID(c), id, int(*version))
	if err != nil {
		Fail(c, err)
		return
	}
	NoContent(c)
}

// --- production lines ----------------------------------------------------

func (ctl *MasterDataController) ListProductionLines(c *gin.Context) {
	companyID := CompanyID(c)
	opts, ok := ListOptions(c, &companyID)
	if !ok {
		return
	}
	page, err := ctl.master.ListProductionLines(c.Request.Context(), opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.ProductionLineToDTO), opts, page.Total)
}

func (ctl *MasterDataController) CreateProductionLine(c *gin.Context) {
	req, ok := Bind[dto.ProductionLineRequest](c)
	if !ok {
		return
	}
	line := mapper.ProductionLineFromDTO(req, CompanyID(c), nil)
	if err := ctl.master.CreateProductionLine(c.Request.Context(), line); err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.ProductionLineToDTO(*line))
}

func (ctl *MasterDataController) UpdateProductionLine(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	req, ok := Bind[dto.ProductionLineRequest](c)
	if !ok {
		return
	}
	line := mapper.ProductionLineFromDTO(req, CompanyID(c), &model.ProductionLine{})
	line.ID = id
	if err := ctl.master.UpdateProductionLine(c.Request.Context(), line); err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.ProductionLineToDTO(*line))
}

// --- seasons -------------------------------------------------------------

func (ctl *MasterDataController) ListSeasons(c *gin.Context) {
	companyID := CompanyID(c)
	opts, ok := ListOptions(c, &companyID)
	if !ok {
		return
	}
	page, err := ctl.master.ListSeasons(c.Request.Context(), opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.SeasonToDTO), opts, page.Total)
}

func (ctl *MasterDataController) CreateSeason(c *gin.Context) {
	req, ok := Bind[dto.SeasonRequest](c)
	if !ok {
		return
	}
	season := mapper.SeasonFromDTO(req, CompanyID(c), nil)
	if err := ctl.master.CreateSeason(c.Request.Context(), season); err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.SeasonToDTO(*season))
}

func (ctl *MasterDataController) UpdateSeason(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	req, ok := Bind[dto.SeasonRequest](c)
	if !ok {
		return
	}
	season := mapper.SeasonFromDTO(req, CompanyID(c), &model.Season{})
	season.ID = id
	if err := ctl.master.UpdateSeason(c.Request.Context(), season); err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.SeasonToDTO(*season))
}

// --- processes -----------------------------------------------------------

func (ctl *MasterDataController) ListProcesses(c *gin.Context) {
	opts, ok := ListOptions(c, nil)
	if !ok {
		return
	}
	page, err := ctl.master.ListProcesses(c.Request.Context(), opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, func(p model.Process) dto.ProcessResponse {
		return mapper.ProcessToDTO(p, nil)
	}), opts, page.Total)
}

// GetProcess returns the process with its declared input and output materials,
// which is the data the routing validation of §F7 is driven from.
func (ctl *MasterDataController) GetProcess(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	process, err := ctl.master.GetProcess(c.Request.Context(), id)
	if err != nil {
		Fail(c, err)
		return
	}
	materials, err := ctl.master.ProcessMaterials(c.Request.Context(), id)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.ProcessToDTO(*process, materials))
}

func (ctl *MasterDataController) CreateProcess(c *gin.Context) {
	req, ok := Bind[dto.ProcessRequest](c)
	if !ok {
		return
	}
	process := mapper.ProcessFromDTO(req, nil)
	if err := ctl.master.CreateProcess(c.Request.Context(), process); err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.ProcessToDTO(*process, nil))
}

func (ctl *MasterDataController) SetProcessMaterials(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	req, ok := Bind[[]dto.ProcessMaterialRequest](c)
	if !ok {
		return
	}
	materials := make([]model.ProcessMaterial, 0, len(req))
	for _, item := range req {
		materials = append(materials, mapper.ProcessMaterialFromDTO(item, id))
	}
	if err := ctl.master.SetProcessMaterials(c.Request.Context(), id, materials); err != nil {
		Fail(c, err)
		return
	}
	stored, err := ctl.master.ProcessMaterials(c.Request.Context(), id)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.Slice(stored, mapper.ProcessMaterialToDTO))
}

// --- movement types, units, packaging ------------------------------------

func (ctl *MasterDataController) ListMovementTypes(c *gin.Context) {
	opts, ok := ListOptions(c, nil)
	if !ok {
		return
	}
	page, err := ctl.master.ListMovementTypes(c.Request.Context(), opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.MovementTypeToDTO), opts, page.Total)
}

func (ctl *MasterDataController) CreateMovementType(c *gin.Context) {
	req, ok := Bind[dto.MovementTypeRequest](c)
	if !ok {
		return
	}
	mt := mapper.MovementTypeFromDTO(req, nil)
	if err := ctl.master.CreateMovementType(c.Request.Context(), mt); err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.MovementTypeToDTO(*mt))
}

func (ctl *MasterDataController) ListUOMs(c *gin.Context) {
	opts, ok := ListOptions(c, nil)
	if !ok {
		return
	}
	page, err := ctl.master.ListUOMs(c.Request.Context(), opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.UOMToDTO), opts, page.Total)
}

func (ctl *MasterDataController) CreateUOM(c *gin.Context) {
	req, ok := Bind[dto.UOMRequest](c)
	if !ok {
		return
	}
	uom := mapper.UOMFromDTO(req, nil)
	if err := ctl.master.CreateUOM(c.Request.Context(), uom); err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.UOMToDTO(*uom))
}

func (ctl *MasterDataController) ListPackagingTypes(c *gin.Context) {
	opts, ok := ListOptions(c, nil)
	if !ok {
		return
	}
	page, err := ctl.master.ListPackagingTypes(c.Request.Context(), opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.PackagingTypeToDTO), opts, page.Total)
}

func (ctl *MasterDataController) CreatePackagingType(c *gin.Context) {
	req, ok := Bind[dto.PackagingTypeRequest](c)
	if !ok {
		return
	}
	pt := mapper.PackagingTypeFromDTO(req, nil)
	if err := ctl.master.CreatePackagingType(c.Request.Context(), pt); err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.PackagingTypeToDTO(*pt))
}
