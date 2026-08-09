package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/audit"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/config"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/database"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
)

// MovementRequest is one line to be written to the ledger. Quantity is always
// positive; the direction comes from the movement type (§33).
type MovementRequest struct {
	CompanyID         int64
	DocumentNo        string // allocated when empty
	TransactionDate   time.Time
	MaterialID        int64
	WarehouseID       int64
	MovementTypeID    int64
	PackagingTypeID   *int64
	ProcessID         *int64
	Quantity          decimal.Decimal
	UOMID             int64
	SourceModule      string
	ReferenceDocument *string
	ReferenceID       *int64
	TransferGroupID   *string
	Remark            *string
}

// TransferRequest moves stock between two locations of the same company (§34).
type TransferRequest struct {
	CompanyID       int64
	FromWarehouseID int64
	ToWarehouseID   int64
	MaterialID      int64
	PackagingTypeID *int64
	Quantity        decimal.Decimal
	UOMID           int64
	TransactionDate time.Time
	Remark          *string
}

// CapacityStatus is the §F5 view of one storage location.
type CapacityStatus struct {
	Warehouse    model.Warehouse
	CapacityUOM  *string
	CurrentStock decimal.Decimal
	Available    *decimal.Decimal
	// Utilisation is nil for an unlimited location — not zero, not infinity.
	Utilisation  *decimal.Decimal
	TrafficLight string // GREEN | AMBER | RED | NONE
}

type InventoryService struct {
	inventory  interfaces.InventoryRepository
	warehouses interfaces.WarehouseRepository
	materials  interfaces.MaterialRepository
	movements  interfaces.MovementTypeRepository
	processes  interfaces.ProcessRepository
	seasons    interfaces.SeasonRepository
	uoms       interfaces.UOMRepository
	numbering  *NumberRangeService
	refs       *ReferenceValidator
	auditSvc   *audit.Service
	uow        database.UnitOfWork
	cfg        config.BusinessConfig
}

func NewInventoryService(
	inventory interfaces.InventoryRepository,
	warehouses interfaces.WarehouseRepository,
	materials interfaces.MaterialRepository,
	movements interfaces.MovementTypeRepository,
	processes interfaces.ProcessRepository,
	seasons interfaces.SeasonRepository,
	uoms interfaces.UOMRepository,
	numbering *NumberRangeService,
	refs *ReferenceValidator,
	auditSvc *audit.Service,
	uow database.UnitOfWork,
	cfg config.BusinessConfig,
) *InventoryService {
	return &InventoryService{
		inventory: inventory, warehouses: warehouses, materials: materials,
		movements: movements, processes: processes, seasons: seasons, uoms: uoms,
		numbering: numbering, refs: refs, auditSvc: auditSvc, uow: uow, cfg: cfg,
	}
}

// Apply validates and writes a set of movements, then updates the affected
// balances. It joins the caller's transaction, which is what makes posting an
// actual document — header, items, movements, balances, number and audit — a
// single atomic operation (§D3).
func (s *InventoryService) Apply(ctx context.Context, requests []MovementRequest) ([]model.InventoryMovement, error) {
	if len(requests) == 0 {
		return nil, nil
	}

	var created []model.InventoryMovement

	err := s.uow.Do(ctx, func(ctx context.Context) error {
		movements := make([]model.InventoryMovement, 0, len(requests))
		lineNos := make(map[string]int)

		for _, req := range requests {
			movement, err := s.prepare(ctx, req, lineNos)
			if err != nil {
				return err
			}
			movements = append(movements, *movement)
		}

		if err := s.inventory.CreateMovements(ctx, movements); err != nil {
			return err
		}
		for _, movement := range movements {
			if err := s.applyToBalance(ctx, movement, false); err != nil {
				return err
			}
		}

		created = movements
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// prepare runs the full validation chain of §F4 and §F7 for one line.
func (s *InventoryService) prepare(ctx context.Context, req MovementRequest, lineNos map[string]int) (*model.InventoryMovement, error) {
	if !req.Quantity.IsPositive() {
		return nil, apperrors.ErrValidation.
			Msgf("a movement quantity must be greater than zero").
			WithDetails(apperrors.Detail{Field: "quantity"})
	}

	// 1. warehouse and material must belong to the document's company.
	warehouse, err := s.refs.Warehouse(ctx, req.CompanyID, req.WarehouseID)
	if err != nil {
		return nil, err
	}
	material, relevance, err := s.refs.MaterialForCompany(ctx, req.CompanyID, req.MaterialID)
	if err != nil {
		return nil, err
	}

	// 2. the material must be inventory-managed in this company.
	if !relevance.InventoryEnabled || !material.IsStockManaged {
		return nil, apperrors.ErrMaterialNotInvEnabled.
			WithDetails(apperrors.Detail{Field: "materialId", Value: material.MaterialCode})
	}

	movementType, err := s.movements.FindByID(ctx, req.MovementTypeID)
	if err != nil {
		return nil, err
	}

	// 6. the transaction date must sit in a season and must not run ahead of
	//    today by more than the configured tolerance.
	if err := s.validatePostingDate(ctx, req.CompanyID, req.TransactionDate); err != nil {
		return nil, err
	}

	// OQ-4: a tank or silo restricted to certain materials rejects the rest.
	allowed, err := s.warehouses.AllowedMaterials(ctx, warehouse.ID)
	if err != nil {
		return nil, err
	}
	if len(allowed) > 0 && !containsID(allowed, material.ID) {
		return nil, apperrors.ErrMaterialNotAllowed.WithDetails(
			apperrors.Detail{Field: "materialId", Value: material.MaterialCode,
				Message: "not allowed in " + warehouse.WarehouseCode})
	}

	// §F7 — the production routing rules, driven by master data.
	if err := s.validateRouting(ctx, req, material, warehouse, movementType); err != nil {
		return nil, err
	}

	documentNo := req.DocumentNo
	if documentNo == "" {
		documentNo, err = s.numbering.Next(ctx, req.CompanyID, model.NumberObjectInventory, req.TransactionDate)
		if err != nil {
			return nil, err
		}
	}

	// Several lines of one document are numbered consecutively.
	lineNo, ok := lineNos[documentNo]
	if !ok {
		lineNo, err = s.inventory.NextLineNo(ctx, req.CompanyID, documentNo)
		if err != nil {
			return nil, err
		}
	} else {
		lineNo++
	}
	lineNos[documentNo] = lineNo

	return &model.InventoryMovement{
		CompanyID:         req.CompanyID,
		DocumentNo:        documentNo,
		LineNo:            lineNo,
		TransactionDate:   req.TransactionDate,
		MaterialID:        req.MaterialID,
		WarehouseID:       req.WarehouseID,
		MovementTypeID:    req.MovementTypeID,
		PackagingTypeID:   req.PackagingTypeID,
		ProcessID:         req.ProcessID,
		Quantity:          req.Quantity,
		UOMID:             req.UOMID,
		ReferenceDocument: req.ReferenceDocument,
		ReferenceID:       req.ReferenceID,
		SourceModule:      req.SourceModule,
		TransferGroupID:   req.TransferGroupID,
		Remark:            req.Remark,
		MovementType:      movementType,
	}, nil
}

// validateRouting implements §F7. Nothing here reads a hard-coded material or
// warehouse code: the decision comes from materials.conditioning_required,
// process_materials.requires_conditioning and warehouses.warehouse_type, so a
// new grade or a second silo is a master-data change.
func (s *InventoryService) validateRouting(ctx context.Context, req MovementRequest,
	material *model.Material, warehouse *model.Warehouse, movementType *model.MovementType) error {

	if req.ProcessID != nil {
		processMaterials, err := s.processes.Materials(ctx, *req.ProcessID)
		if err != nil {
			return err
		}
		var declared *model.ProcessMaterial
		for i := range processMaterials {
			if processMaterials[i].MaterialID == material.ID {
				declared = &processMaterials[i]
				break
			}
		}
		// Rule 1: the material must be a declared input or output.
		if declared == nil {
			return apperrors.ErrMaterialNotValidForProcess.WithDetails(
				apperrors.Detail{Field: "materialId", Value: material.MaterialCode})
		}

		// Rule 3: a material that this process produces for conditioning may
		// not land straight in a finished-goods warehouse — it goes to the
		// silo first, and reaches its warehouse by an issue from there.
		if movementType.Direction == model.DirectionIn &&
			declared.IOType == model.IOTypeOutput &&
			declared.RequiresConditioning &&
			warehouse.WarehouseType != model.WarehouseTypeSilo {
			return apperrors.ErrConditioningStepMissing.WithDetails(
				apperrors.Detail{Field: "warehouseId", Value: warehouse.WarehouseCode,
					Message: material.MaterialCode + " must be received into a conditioning silo first"})
		}
	}

	// Rule 2: nothing that does not require conditioning may be received into
	// a silo — this is what keeps White Sugar out of the Condition Silo.
	if movementType.Direction == model.DirectionIn &&
		warehouse.WarehouseType == model.WarehouseTypeSilo &&
		!material.ConditioningRequired {
		return apperrors.ErrMaterialNotSiloManaged.WithDetails(
			apperrors.Detail{Field: "materialId", Value: material.MaterialCode,
				Message: "goes directly to its warehouse and never enters the silo"})
	}

	return nil
}

// validatePostingDate implements §F4 rule 6.
func (s *InventoryService) validatePostingDate(ctx context.Context, companyID int64, date time.Time) error {
	limit := time.Now().UTC().AddDate(0, 0, s.cfg.MaxFuturePostingDays)
	if date.After(limit) {
		return apperrors.ErrValidation.
			Msgf("a document may not be posted more than %d day(s) into the future", s.cfg.MaxFuturePostingDays).
			WithDetails(apperrors.Detail{Field: "transactionDate", Value: date.Format("2006-01-02")})
	}

	season, err := s.seasons.FindContaining(ctx, companyID, date)
	if err != nil {
		return err
	}
	if season == nil {
		return apperrors.ErrDateOutsideSeason.
			Msgf("no season is defined for %s", date.Format("2006-01-02")).
			WithDetails(apperrors.Detail{Field: "transactionDate"})
	}
	if season.Status == model.SeasonStatusClosed {
		return apperrors.ErrDateOutsideSeason.
			Msgf("season %s is closed", season.SeasonCode).
			WithDetails(apperrors.Detail{Field: "transactionDate"})
	}
	return nil
}

// applyToBalance locks the stock key, applies the signed delta and enforces
// the negative-stock and capacity rules. The row lock is what serialises
// concurrent posters without locking the whole table (§F4).
func (s *InventoryService) applyToBalance(ctx context.Context, movement model.InventoryMovement, reversal bool) error {
	movementType := movement.MovementType
	if movementType == nil {
		loaded, err := s.movements.FindByID(ctx, movement.MovementTypeID)
		if err != nil {
			return err
		}
		movementType = loaded
	}
	if !movementType.AffectsStock {
		return nil // e.g. electricity export — produced, but never warehouse stock
	}

	material, err := s.materials.FindByID(ctx, movement.MaterialID)
	if err != nil {
		return err
	}

	// Balances are kept in the material's base unit so that quantities entered
	// in different units remain addable.
	quantity, err := s.convert(ctx, movement.Quantity, movement.UOMID, material.BaseUOMID)
	if err != nil {
		return err
	}

	balance, err := s.inventory.LockBalance(ctx, interfaces.BalanceKey{
		CompanyID:       movement.CompanyID,
		WarehouseID:     movement.WarehouseID,
		MaterialID:      movement.MaterialID,
		PackagingTypeID: movement.PackagingTypeID,
		BalanceDate:     movement.TransactionDate,
	})
	if err != nil {
		return err
	}

	direction := movementType.Direction
	if reversal {
		direction = opposite(direction)
	}
	if direction == model.DirectionIn {
		balance.InQty = balance.InQty.Add(quantity)
	} else {
		balance.OutQty = balance.OutQty.Add(quantity)
	}
	balance.UOMID = &material.BaseUOMID
	balance.Recompute()

	warehouse, err := s.warehouses.FindByID(ctx, movement.CompanyID, movement.WarehouseID)
	if err != nil {
		return err
	}

	// 3. stock may not go negative unless the location explicitly allows it.
	if balance.ClosingQty.IsNegative() && !warehouse.AllowNegativeStock {
		return apperrors.ErrNegativeStock.WithDetails(apperrors.Detail{
			Field:   "quantity",
			Value:   quantity.String(),
			Message: "resulting stock would be " + balance.ClosingQty.String(),
		})
	}

	if err := s.inventory.SaveBalance(ctx, balance); err != nil {
		return err
	}

	// 4. the location must be able to hold the result.
	if direction == model.DirectionIn && warehouse.HasCapacityLimit() {
		if err := s.assertCapacity(ctx, warehouse, movement.TransactionDate); err != nil {
			return err
		}
	}
	return nil
}

// assertCapacity compares the location's total stock against its capacity,
// both expressed in the capacity unit (§F5, §17).
func (s *InventoryService) assertCapacity(ctx context.Context, warehouse *model.Warehouse, asOf time.Time) error {
	if warehouse.CapacityUOMID == nil {
		return nil
	}
	stock, err := s.inventory.WarehouseStock(ctx, warehouse.CompanyID, warehouse.ID, *warehouse.CapacityUOMID, asOf)
	if err != nil {
		return err
	}
	if stock.ConversionMissing {
		return apperrors.ErrUOMMismatch.
			Msgf("stock in %s cannot be converted to its capacity unit, so the capacity cannot be checked",
				warehouse.WarehouseCode).
			WithDetails(apperrors.Detail{Field: "capacityUomId"})
	}
	if stock.Total.GreaterThan(*warehouse.Capacity) {
		return apperrors.ErrCapacityExceeded.WithDetails(apperrors.Detail{
			Field:   "warehouseId",
			Value:   warehouse.WarehouseCode,
			Message: "capacity " + warehouse.Capacity.String() + ", resulting stock " + stock.Total.String(),
		})
	}
	return nil
}

// convert brings a quantity into the target unit, failing loudly when the
// conversion is not maintained rather than assuming a factor of one.
func (s *InventoryService) convert(ctx context.Context, qty decimal.Decimal, fromUOMID, toUOMID int64) (decimal.Decimal, error) {
	if fromUOMID == toUOMID {
		return qty, nil
	}
	conversion, err := s.uoms.FindConversion(ctx, fromUOMID, toUOMID)
	if err != nil {
		return decimal.Zero, err
	}
	if conversion == nil {
		return decimal.Zero, apperrors.ErrUOMMismatch.WithDetails(
			apperrors.Detail{Field: "uomId", Message: "no conversion to the material base unit"})
	}
	return conversion.Convert(qty), nil
}

// --- transfers (§34) -----------------------------------------------------

// Transfer writes the two linked movements of a stock transfer. Both rows
// carry the same transfer_group_id and, by construction and by a database
// trigger, the same company: there is no cross-company transfer.
func (s *InventoryService) Transfer(ctx context.Context, req TransferRequest) ([]model.InventoryMovement, error) {
	if req.FromWarehouseID == req.ToWarehouseID {
		return nil, apperrors.ErrSameWarehouse
	}

	out, err := s.movements.FindByCode(ctx, "TRANSFER_OUT")
	if err != nil {
		return nil, err
	}
	in, err := s.movements.FindByCode(ctx, "TRANSFER_IN")
	if err != nil {
		return nil, err
	}

	var movements []model.InventoryMovement
	err = s.uow.Do(ctx, func(ctx context.Context) error {
		documentNo, err := s.numbering.Next(ctx, req.CompanyID, model.NumberObjectTransfer, req.TransactionDate)
		if err != nil {
			return err
		}
		group := uuid.NewString()

		movements, err = s.Apply(ctx, []MovementRequest{
			{
				CompanyID: req.CompanyID, DocumentNo: documentNo,
				TransactionDate: req.TransactionDate, MaterialID: req.MaterialID,
				WarehouseID: req.FromWarehouseID, MovementTypeID: out.ID,
				PackagingTypeID: req.PackagingTypeID, Quantity: req.Quantity, UOMID: req.UOMID,
				SourceModule: model.SourceModuleTransfer, TransferGroupID: &group, Remark: req.Remark,
			},
			{
				CompanyID: req.CompanyID, DocumentNo: documentNo,
				TransactionDate: req.TransactionDate, MaterialID: req.MaterialID,
				WarehouseID: req.ToWarehouseID, MovementTypeID: in.ID,
				PackagingTypeID: req.PackagingTypeID, Quantity: req.Quantity, UOMID: req.UOMID,
				SourceModule: model.SourceModuleTransfer, TransferGroupID: &group, Remark: req.Remark,
			},
		})
		if err != nil {
			return err
		}

		s.auditSvc.Record(ctx, audit.Event{
			TableName: "inventory_movements", CompanyID: &req.CompanyID, Action: model.AuditInsert,
			NewValues: map[string]any{
				"documentNo": documentNo, "transferGroupId": group,
				"from": req.FromWarehouseID, "to": req.ToWarehouseID,
				"quantity": req.Quantity.String(),
			},
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return movements, nil
}

// Adjust writes a single manual correction.
func (s *InventoryService) Adjust(ctx context.Context, req MovementRequest) ([]model.InventoryMovement, error) {
	req.SourceModule = model.SourceModuleManual
	return s.Apply(ctx, []MovementRequest{req})
}

// Reverse cancels a movement by writing an opposite one and flagging the
// original. A ledger row is never deleted (§33).
func (s *InventoryService) Reverse(ctx context.Context, companyID, movementID int64, remark *string) (*model.InventoryMovement, error) {
	var reversal *model.InventoryMovement

	err := s.uow.Do(ctx, func(ctx context.Context) error {
		original, err := s.inventory.FindMovement(ctx, companyID, movementID)
		if err != nil {
			return err
		}
		if original.IsReversed {
			return apperrors.ErrAlreadyReversed
		}

		movementType, err := s.movements.FindByID(ctx, original.MovementTypeID)
		if err != nil {
			return err
		}
		counterpart, err := s.counterpartOf(ctx, movementType)
		if err != nil {
			return err
		}

		documentNo, err := s.numbering.Next(ctx, companyID, model.NumberObjectInventory, original.TransactionDate)
		if err != nil {
			return err
		}
		lineNo, err := s.inventory.NextLineNo(ctx, companyID, documentNo)
		if err != nil {
			return err
		}

		row := model.InventoryMovement{
			CompanyID:          companyID,
			DocumentNo:         documentNo,
			LineNo:             lineNo,
			TransactionDate:    original.TransactionDate,
			MaterialID:         original.MaterialID,
			WarehouseID:        original.WarehouseID,
			MovementTypeID:     counterpart.ID,
			PackagingTypeID:    original.PackagingTypeID,
			ProcessID:          original.ProcessID,
			Quantity:           original.Quantity,
			UOMID:              original.UOMID,
			ReferenceDocument:  &original.DocumentNo,
			ReferenceID:        &original.ID,
			SourceModule:       model.SourceModuleReversal,
			ReversedMovementID: &original.ID,
			Remark:             remark,
			MovementType:       counterpart,
		}

		if err := s.inventory.CreateMovements(ctx, []model.InventoryMovement{row}); err != nil {
			return err
		}
		if err := s.applyToBalance(ctx, row, false); err != nil {
			return err
		}
		if err := s.inventory.MarkReversed(ctx, companyID, original.ID); err != nil {
			return err
		}

		s.auditSvc.Record(ctx, audit.Event{
			TableName: "inventory_movements", RecordID: &original.ID, CompanyID: &companyID,
			Action:    model.AuditReverse,
			NewValues: map[string]any{"reversalDocument": documentNo},
		})

		reversal = &row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return reversal, nil
}

// counterpartOf finds the movement type that undoes another one: the
// maintained counterpart if there is one, otherwise any active type with the
// opposite direction in the same family.
func (s *InventoryService) counterpartOf(ctx context.Context, movementType *model.MovementType) (*model.MovementType, error) {
	if movementType.CounterpartID != nil {
		return s.movements.FindByID(ctx, *movementType.CounterpartID)
	}
	code := "ADJUSTMENT_IN"
	if movementType.Direction == model.DirectionIn {
		code = "ADJUSTMENT_OUT"
	}
	return s.movements.FindByCode(ctx, code)
}

// --- queries -------------------------------------------------------------

func (s *InventoryService) ListMovements(ctx context.Context, opts interfaces.ListOptions,
	f interfaces.MovementFilter) (interfaces.Page[model.InventoryMovement], error) {
	return s.inventory.ListMovements(ctx, opts, f)
}

func (s *InventoryService) Balances(ctx context.Context, companyID int64,
	warehouseID, materialID *int64, asOf time.Time) ([]interfaces.BalanceRow, error) {
	return s.inventory.ListBalances(ctx, companyID, warehouseID, materialID, asOf)
}

// Capacity implements §F5, including the rule that an unmaintained capacity
// means unlimited and reports a nil utilisation.
func (s *InventoryService) Capacity(ctx context.Context, companyID int64,
	warehouseID *int64, asOf time.Time) ([]CapacityStatus, error) {

	opts := interfaces.ListOptions{CompanyID: &companyID, Size: interfaces.MaxPageSize}
	page, err := s.warehouses.List(ctx, opts)
	if err != nil {
		return nil, err
	}

	statuses := make([]CapacityStatus, 0, len(page.Rows))
	for _, warehouse := range page.Rows {
		if warehouseID != nil && warehouse.ID != *warehouseID {
			continue
		}

		status := CapacityStatus{Warehouse: warehouse, TrafficLight: "NONE"}

		targetUOM := warehouse.CapacityUOMID
		if targetUOM == nil {
			// Without a capacity unit there is nothing to compare against, so
			// the location is reported as unlimited.
			statuses = append(statuses, status)
			continue
		}

		stock, err := s.inventory.WarehouseStock(ctx, companyID, warehouse.ID, *targetUOM, asOf)
		if err != nil {
			return nil, err
		}
		status.CurrentStock = stock.Total

		if warehouse.HasCapacityLimit() && !stock.ConversionMissing {
			available := warehouse.Capacity.Sub(stock.Total)
			utilisation := stock.Total.Div(*warehouse.Capacity).Mul(decimal.NewFromInt(100)).
				Round(2)
			status.Available = &available
			status.Utilisation = &utilisation
			status.TrafficLight = s.trafficLight(utilisation)
		}

		statuses = append(statuses, status)
	}
	return statuses, nil
}

func (s *InventoryService) trafficLight(utilisation decimal.Decimal) string {
	value, _ := utilisation.Float64()
	switch {
	case value > s.cfg.CapacityRedPct:
		return "RED"
	case value >= s.cfg.CapacityAmberPct:
		return "AMBER"
	default:
		return "GREEN"
	}
}

// Reconcile is the nightly job: rebuild the balances from the ledger and
// report every row that had drifted.
func (s *InventoryService) Reconcile(ctx context.Context, companyID int64, from, to time.Time) ([]interfaces.BalanceDiscrepancy, error) {
	var discrepancies []interfaces.BalanceDiscrepancy
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		var err error
		discrepancies, err = s.inventory.RecomputeBalances(ctx, companyID, from, to)
		return err
	})
	if err != nil {
		return nil, err
	}
	if len(discrepancies) > 0 {
		s.auditSvc.Record(ctx, audit.Event{
			TableName: "inventory_balances", CompanyID: &companyID, Action: model.AuditUpdate,
			NewValues: map[string]any{"reconciledDiscrepancies": len(discrepancies)},
		})
	}
	return discrepancies, nil
}

func opposite(direction string) string {
	if direction == model.DirectionIn {
		return model.DirectionOut
	}
	return model.DirectionIn
}

func containsID(ids []int64, id int64) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}
