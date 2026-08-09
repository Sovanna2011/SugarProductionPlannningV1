package service

import (
	"context"
	"time"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/audit"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
)

// MasterDataService owns the master data of Part B3. Authorization is enforced
// by the route middleware (§C2 steps 3–4); what lives here are the business
// rules the middleware cannot express.
type MasterDataService struct {
	companies  interfaces.CompanyRepositoryIface
	materials  interfaces.MaterialRepository
	warehouses interfaces.WarehouseRepository
	lines      interfaces.ProductionLineRepository
	seasons    interfaces.SeasonRepository
	processes  interfaces.ProcessRepository
	movements  interfaces.MovementTypeRepository
	uoms       interfaces.UOMRepository
	packaging  interfaces.PackagingTypeRepository
	auditSvc   *audit.Service
}

func NewMasterDataService(
	companies interfaces.CompanyRepositoryIface,
	materials interfaces.MaterialRepository,
	warehouses interfaces.WarehouseRepository,
	lines interfaces.ProductionLineRepository,
	seasons interfaces.SeasonRepository,
	processes interfaces.ProcessRepository,
	movements interfaces.MovementTypeRepository,
	uoms interfaces.UOMRepository,
	packaging interfaces.PackagingTypeRepository,
	auditSvc *audit.Service,
) *MasterDataService {
	return &MasterDataService{
		companies: companies, materials: materials, warehouses: warehouses,
		lines: lines, seasons: seasons, processes: processes,
		movements: movements, uoms: uoms, packaging: packaging, auditSvc: auditSvc,
	}
}

// --- companies -----------------------------------------------------------

func (s *MasterDataService) ListCompanies(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.Company], error) {
	return s.companies.List(ctx, opts)
}

func (s *MasterDataService) GetCompany(ctx context.Context, id int64) (*model.Company, error) {
	return s.companies.FindByID(ctx, id)
}

func (s *MasterDataService) CreateCompany(ctx context.Context, company *model.Company) error {
	if err := s.companies.Create(ctx, company); err != nil {
		return err
	}
	s.auditSvc.Record(ctx, audit.Event{
		TableName: "companies", RecordID: &company.ID, CompanyID: &company.ID,
		Action: model.AuditInsert, NewValues: company,
	})
	return nil
}

func (s *MasterDataService) UpdateCompany(ctx context.Context, company *model.Company) error {
	before, err := s.companies.FindByID(ctx, company.ID)
	if err != nil {
		return err
	}
	if err := s.companies.Update(ctx, company); err != nil {
		return err
	}
	s.auditSvc.Record(ctx, audit.Event{
		TableName: "companies", RecordID: &company.ID, CompanyID: &company.ID,
		Action: model.AuditUpdate, OldValues: before, NewValues: company,
	})
	return nil
}

// --- materials -----------------------------------------------------------

func (s *MasterDataService) ListMaterials(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.Material], error) {
	return s.materials.List(ctx, opts)
}

func (s *MasterDataService) GetMaterial(ctx context.Context, id int64) (*model.Material, error) {
	return s.materials.FindByID(ctx, id)
}

// CreateMaterial keeps the conditioning flag consistent with the material
// type: only a finished sugar grade can be routed through the condition silo
// (§F7).
func (s *MasterDataService) CreateMaterial(ctx context.Context, material *model.Material) error {
	if err := s.validateMaterial(ctx, material); err != nil {
		return err
	}
	if err := s.materials.Create(ctx, material); err != nil {
		return err
	}
	s.auditSvc.Record(ctx, audit.Event{
		TableName: "materials", RecordID: &material.ID,
		Action: model.AuditInsert, NewValues: material,
	})
	return nil
}

func (s *MasterDataService) UpdateMaterial(ctx context.Context, material *model.Material) error {
	before, err := s.materials.FindByID(ctx, material.ID)
	if err != nil {
		return err
	}
	if err := s.validateMaterial(ctx, material); err != nil {
		return err
	}
	if err := s.materials.Update(ctx, material); err != nil {
		return err
	}
	s.auditSvc.Record(ctx, audit.Event{
		TableName: "materials", RecordID: &material.ID,
		Action: model.AuditUpdate, OldValues: before, NewValues: material,
	})
	return nil
}

func (s *MasterDataService) validateMaterial(ctx context.Context, material *model.Material) error {
	if _, err := s.uoms.FindByID(ctx, material.BaseUOMID); err != nil {
		return apperrors.ErrValidation.Msgf("base unit of measure does not exist").
			WithDetails(apperrors.Detail{Field: "baseUomId"})
	}
	if material.ConditioningRequired && material.MaterialType != model.MaterialTypeFinished {
		return apperrors.ErrValidation.
			Msgf("only a finished material can require conditioning").
			WithDetails(apperrors.Detail{Field: "conditioningRequired"})
	}
	return nil
}

func (s *MasterDataService) DeactivateMaterial(ctx context.Context, id int64, version int) error {
	return s.materials.Deactivate(ctx, id, version)
}

func (s *MasterDataService) ListCompanyMaterials(ctx context.Context, companyID int64,
	opts interfaces.ListOptions) (interfaces.Page[model.CompanyMaterial], error) {
	return s.materials.ListForCompany(ctx, companyID, opts)
}

func (s *MasterDataService) UpsertCompanyMaterial(ctx context.Context, cm *model.CompanyMaterial) error {
	if _, err := s.materials.FindByID(ctx, cm.MaterialID); err != nil {
		return err
	}
	return s.materials.UpsertCompanyMaterial(ctx, cm)
}

// --- warehouses ----------------------------------------------------------

func (s *MasterDataService) ListWarehouses(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.Warehouse], error) {
	return s.warehouses.List(ctx, opts)
}

func (s *MasterDataService) GetWarehouse(ctx context.Context, companyID, id int64) (*model.Warehouse, error) {
	return s.warehouses.FindByID(ctx, companyID, id)
}

func (s *MasterDataService) CreateWarehouse(ctx context.Context, warehouse *model.Warehouse, allowedMaterials []int64) error {
	if err := s.validateWarehouse(warehouse); err != nil {
		return err
	}
	if err := s.warehouses.Create(ctx, warehouse); err != nil {
		return err
	}
	if len(allowedMaterials) > 0 {
		if err := s.warehouses.SetAllowedMaterials(ctx, warehouse.ID, allowedMaterials); err != nil {
			return err
		}
	}
	s.auditSvc.Record(ctx, audit.Event{
		TableName: "warehouses", RecordID: &warehouse.ID, CompanyID: &warehouse.CompanyID,
		Action: model.AuditInsert, NewValues: warehouse,
	})
	return nil
}

func (s *MasterDataService) UpdateWarehouse(ctx context.Context, warehouse *model.Warehouse, allowedMaterials []int64) error {
	before, err := s.warehouses.FindByID(ctx, warehouse.CompanyID, warehouse.ID)
	if err != nil {
		return err
	}
	if err := s.validateWarehouse(warehouse); err != nil {
		return err
	}
	if err := s.warehouses.Update(ctx, warehouse.CompanyID, warehouse); err != nil {
		return err
	}
	if allowedMaterials != nil {
		if err := s.warehouses.SetAllowedMaterials(ctx, warehouse.ID, allowedMaterials); err != nil {
			return err
		}
	}
	s.auditSvc.Record(ctx, audit.Event{
		TableName: "warehouses", RecordID: &warehouse.ID, CompanyID: &warehouse.CompanyID,
		Action: model.AuditUpdate, OldValues: before, NewValues: warehouse,
	})
	return nil
}

// validateWarehouse enforces that a capacity is always accompanied by its
// unit. A capacity without a unit cannot be checked against stock (§17).
func (s *MasterDataService) validateWarehouse(warehouse *model.Warehouse) error {
	if warehouse.Capacity != nil && warehouse.CapacityUOMID == nil {
		return apperrors.ErrValidation.
			Msgf("a capacity requires a capacity unit of measure").
			WithDetails(apperrors.Detail{Field: "capacityUomId"})
	}
	if warehouse.Capacity != nil && warehouse.Capacity.IsNegative() {
		return apperrors.ErrValidation.Msgf("capacity must not be negative").
			WithDetails(apperrors.Detail{Field: "capacity"})
	}
	return nil
}

func (s *MasterDataService) WarehouseAllowedMaterials(ctx context.Context, warehouseID int64) ([]int64, error) {
	return s.warehouses.AllowedMaterials(ctx, warehouseID)
}

func (s *MasterDataService) DeactivateWarehouse(ctx context.Context, companyID, id int64, version int) error {
	return s.warehouses.Deactivate(ctx, companyID, id, version)
}

// --- production lines ----------------------------------------------------

func (s *MasterDataService) ListProductionLines(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.ProductionLine], error) {
	return s.lines.List(ctx, opts)
}

func (s *MasterDataService) CreateProductionLine(ctx context.Context, line *model.ProductionLine) error {
	return s.lines.Create(ctx, line)
}

func (s *MasterDataService) UpdateProductionLine(ctx context.Context, line *model.ProductionLine) error {
	return s.lines.Update(ctx, line.CompanyID, line)
}

func (s *MasterDataService) DeactivateProductionLine(ctx context.Context, companyID, id int64, version int) error {
	return s.lines.Deactivate(ctx, companyID, id, version)
}

// --- seasons -------------------------------------------------------------

func (s *MasterDataService) ListSeasons(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.Season], error) {
	return s.seasons.List(ctx, opts)
}

func (s *MasterDataService) GetSeason(ctx context.Context, companyID, id int64) (*model.Season, error) {
	return s.seasons.FindByID(ctx, companyID, id)
}

// CreateSeason relies on the database exclusion constraint for the overlap
// rule of §27, but checks the date order here so the caller gets a field-level
// message instead of a constraint name.
func (s *MasterDataService) CreateSeason(ctx context.Context, season *model.Season) error {
	if err := validateSeasonDates(season); err != nil {
		return err
	}
	if err := s.seasons.Create(ctx, season); err != nil {
		return err
	}
	s.auditSvc.Record(ctx, audit.Event{
		TableName: "seasons", RecordID: &season.ID, CompanyID: &season.CompanyID,
		Action: model.AuditInsert, NewValues: season,
	})
	return nil
}

func (s *MasterDataService) UpdateSeason(ctx context.Context, season *model.Season) error {
	if err := validateSeasonDates(season); err != nil {
		return err
	}
	return s.seasons.Update(ctx, season.CompanyID, season)
}

func validateSeasonDates(season *model.Season) error {
	if !season.EndDate.After(season.StartDate) {
		return apperrors.ErrInvalidDateRange.
			Msgf("the season end date must be after its start date").
			WithDetails(apperrors.Detail{Field: "endDate"})
	}
	return nil
}

func (s *MasterDataService) DeactivateSeason(ctx context.Context, companyID, id int64, version int) error {
	return s.seasons.Deactivate(ctx, companyID, id, version)
}

// --- processes -----------------------------------------------------------

func (s *MasterDataService) ListProcesses(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.Process], error) {
	return s.processes.List(ctx, opts)
}

func (s *MasterDataService) GetProcess(ctx context.Context, id int64) (*model.Process, error) {
	return s.processes.FindByID(ctx, id)
}

func (s *MasterDataService) ProcessMaterials(ctx context.Context, processID int64) ([]model.ProcessMaterial, error) {
	return s.processes.Materials(ctx, processID)
}

func (s *MasterDataService) CreateProcess(ctx context.Context, process *model.Process) error {
	return s.processes.Create(ctx, process)
}

func (s *MasterDataService) UpdateProcess(ctx context.Context, process *model.Process) error {
	return s.processes.Update(ctx, process)
}

// SetProcessMaterials rewrites the input/output network of a process. This is
// how a new production route is introduced — as master data, not as code.
func (s *MasterDataService) SetProcessMaterials(ctx context.Context, processID int64, materials []model.ProcessMaterial) error {
	if _, err := s.processes.FindByID(ctx, processID); err != nil {
		return err
	}
	for _, m := range materials {
		if m.IOType != model.IOTypeInput && m.IOType != model.IOTypeOutput {
			return apperrors.ErrValidation.Msgf("ioType must be INPUT or OUTPUT").
				WithDetails(apperrors.Detail{Field: "ioType", Value: m.IOType})
		}
		if _, err := s.materials.FindByID(ctx, m.MaterialID); err != nil {
			return err
		}
	}
	if err := s.processes.SetMaterials(ctx, processID, materials); err != nil {
		return err
	}
	s.auditSvc.Record(ctx, audit.Event{
		TableName: "process_materials", RecordID: &processID,
		Action: model.AuditUpdate, NewValues: materials,
	})
	return nil
}

// --- movement types, units, packaging ------------------------------------

func (s *MasterDataService) ListMovementTypes(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.MovementType], error) {
	return s.movements.List(ctx, opts)
}

func (s *MasterDataService) GetMovementType(ctx context.Context, id int64) (*model.MovementType, error) {
	return s.movements.FindByID(ctx, id)
}

func (s *MasterDataService) CreateMovementType(ctx context.Context, mt *model.MovementType) error {
	if mt.Direction != model.DirectionIn && mt.Direction != model.DirectionOut {
		return apperrors.ErrValidation.Msgf("direction must be IN or OUT").
			WithDetails(apperrors.Detail{Field: "direction", Value: mt.Direction})
	}
	return s.movements.Create(ctx, mt)
}

func (s *MasterDataService) UpdateMovementType(ctx context.Context, mt *model.MovementType) error {
	return s.movements.Update(ctx, mt)
}

func (s *MasterDataService) ListUOMs(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.UOM], error) {
	return s.uoms.List(ctx, opts)
}

func (s *MasterDataService) CreateUOM(ctx context.Context, uom *model.UOM) error {
	return s.uoms.Create(ctx, uom)
}

func (s *MasterDataService) UpdateUOM(ctx context.Context, uom *model.UOM) error {
	return s.uoms.Update(ctx, uom)
}

func (s *MasterDataService) ListPackagingTypes(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.PackagingType], error) {
	return s.packaging.List(ctx, opts)
}

func (s *MasterDataService) CreatePackagingType(ctx context.Context, pt *model.PackagingType) error {
	return s.packaging.Create(ctx, pt)
}

func (s *MasterDataService) UpdatePackagingType(ctx context.Context, pt *model.PackagingType) error {
	return s.packaging.Update(ctx, pt)
}

// --- shared validation used by the document services ---------------------

// ReferenceValidator checks that every master data record a document points at
// belongs to the same company. This is the E-VAL-010 rule of §C2 and the
// no-cross-company-transfer rule of §34.
type ReferenceValidator struct {
	warehouses interfaces.WarehouseRepository
	lines      interfaces.ProductionLineRepository
	seasons    interfaces.SeasonRepository
	materials  interfaces.MaterialRepository
}

func NewReferenceValidator(
	warehouses interfaces.WarehouseRepository,
	lines interfaces.ProductionLineRepository,
	seasons interfaces.SeasonRepository,
	materials interfaces.MaterialRepository,
) *ReferenceValidator {
	return &ReferenceValidator{warehouses: warehouses, lines: lines, seasons: seasons, materials: materials}
}

func (v *ReferenceValidator) Warehouse(ctx context.Context, companyID, warehouseID int64) (*model.Warehouse, error) {
	warehouse, err := v.warehouses.FindByID(ctx, companyID, warehouseID)
	if err != nil {
		if appErr, ok := apperrors.As(err); ok && appErr.Code == apperrors.ErrNotFound.Code {
			return nil, apperrors.ErrCrossCompanyReference.
				WithDetails(apperrors.Detail{Field: "warehouseId", Message: "unknown in this company"})
		}
		return nil, err
	}
	return warehouse, nil
}

func (v *ReferenceValidator) ProductionLine(ctx context.Context, companyID int64, lineID *int64) error {
	if lineID == nil {
		return nil
	}
	if _, err := v.lines.FindByID(ctx, companyID, *lineID); err != nil {
		if appErr, ok := apperrors.As(err); ok && appErr.Code == apperrors.ErrNotFound.Code {
			return apperrors.ErrCrossCompanyReference.
				WithDetails(apperrors.Detail{Field: "productionLineId", Message: "unknown in this company"})
		}
		return err
	}
	return nil
}

func (v *ReferenceValidator) Season(ctx context.Context, companyID, seasonID int64) (*model.Season, error) {
	season, err := v.seasons.FindByID(ctx, companyID, seasonID)
	if err != nil {
		if appErr, ok := apperrors.As(err); ok && appErr.Code == apperrors.ErrNotFound.Code {
			return nil, apperrors.ErrCrossCompanyReference.
				WithDetails(apperrors.Detail{Field: "seasonId", Message: "unknown in this company"})
		}
		return nil, err
	}
	return season, nil
}

// MaterialForCompany resolves a material together with its company relevance.
func (v *ReferenceValidator) MaterialForCompany(ctx context.Context, companyID, materialID int64) (*model.Material, *model.CompanyMaterial, error) {
	material, err := v.materials.FindByID(ctx, materialID)
	if err != nil {
		return nil, nil, err
	}
	relevance, err := v.materials.FindCompanyMaterial(ctx, companyID, materialID)
	if err != nil {
		return nil, nil, err
	}
	if relevance == nil {
		return nil, nil, apperrors.ErrCrossCompanyReference.
			WithDetails(apperrors.Detail{Field: "materialId", Message: "not assigned to this company"})
	}
	return material, relevance, nil
}

// DateInSeason enforces §F4 rule 6 for the season side of the check.
func (v *ReferenceValidator) DateInSeason(season *model.Season, date time.Time) error {
	if season == nil {
		return nil
	}
	if !season.Contains(date) {
		return apperrors.ErrDateOutsideSeason.WithDetails(apperrors.Detail{
			Field: "date", Value: date.Format("2006-01-02"),
		})
	}
	return nil
}
