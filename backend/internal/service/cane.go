package service

import (
	"context"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/audit"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/database"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
)

// HarvestCellInput is one editable cell of the Date × Grower grid.
//
// A nil PlannedTons means "leave unchanged" — not the same as a cell the
// client omitted, which means "delete" unless partialUpdate is set (§E3).
type HarvestCellInput struct {
	PlanDate       time.Time
	GrowerID       int64
	CaneFieldID    *int64
	VarietyID      *int64
	PlannedTons    *decimal.Decimal
	PlannedAreaHa  *decimal.Decimal
	ExpectedCCSPct *decimal.Decimal
	Remark         *string
}

// HarvestMatrixRequest is the matrix-shaped bulk save of the harvest plan.
type HarvestMatrixRequest struct {
	CompanyID     int64
	VersionID     int64
	UOMID         int64
	DateFrom      time.Time
	DateTo        time.Time
	Cells         []HarvestCellInput
	PartialUpdate bool
}

type HarvestMatrixResponse struct {
	CompanyID     int64
	VersionID     int64
	VersionStatus string
	DocumentNo    string
	DateFrom      time.Time
	DateTo        time.Time
	Growers       []model.Grower
	Rows          []HarvestMatrixRow
}

type HarvestMatrixRow struct {
	PlanDate time.Time
	Values   []HarvestMatrixValue
}

type HarvestMatrixValue struct {
	GrowerID       int64
	CaneFieldID    *int64
	VarietyID      *int64
	PlannedTons    decimal.Decimal
	PlannedAreaHa  *decimal.Decimal
	ExpectedCCSPct *decimal.Decimal
	UOMID          int64
	Version        int
}

// CaneService owns cane supply: the growers and fields cane comes from, the
// harvest plan inside a planning version, and the weighbridge tickets that are
// its actual.
//
// It deliberately reuses the machinery that already exists rather than
// mirroring it — planning versions carry the harvest plan, and posting a
// delivery goes through the ordinary inventory service, so cane stock obeys
// exactly the same capacity, negative-stock and season rules as everything
// else.
type CaneService struct {
	varieties  interfaces.CaneVarietyRepository
	growers    interfaces.GrowerRepository
	fields     interfaces.CaneFieldRepository
	plans      interfaces.HarvestPlanRepository
	deliveries interfaces.CaneDeliveryRepository
	reports    interfaces.CaneReportRepository
	versions   interfaces.PlanningVersionRepository
	materials  interfaces.MaterialRepository
	movements  interfaces.MovementTypeRepository
	seasons    interfaces.SeasonRepository
	inventory  *InventoryService
	numbering  *NumberRangeService
	authz      *AuthorizationService
	refs       *ReferenceValidator
	auditSvc   *audit.Service
	uow        database.UnitOfWork
}

func NewCaneService(
	varieties interfaces.CaneVarietyRepository,
	growers interfaces.GrowerRepository,
	fields interfaces.CaneFieldRepository,
	plans interfaces.HarvestPlanRepository,
	deliveries interfaces.CaneDeliveryRepository,
	reports interfaces.CaneReportRepository,
	versions interfaces.PlanningVersionRepository,
	materials interfaces.MaterialRepository,
	movements interfaces.MovementTypeRepository,
	seasons interfaces.SeasonRepository,
	inventory *InventoryService,
	numbering *NumberRangeService,
	authz *AuthorizationService,
	refs *ReferenceValidator,
	auditSvc *audit.Service,
	uow database.UnitOfWork,
) *CaneService {
	return &CaneService{
		varieties: varieties, growers: growers, fields: fields, plans: plans,
		deliveries: deliveries, reports: reports, versions: versions,
		materials: materials, movements: movements, seasons: seasons,
		inventory: inventory, numbering: numbering, authz: authz, refs: refs,
		auditSvc: auditSvc, uow: uow,
	}
}

// --- varieties -----------------------------------------------------------

func (s *CaneService) ListVarieties(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.CaneVariety], error) {
	return s.varieties.List(ctx, opts)
}

func (s *CaneService) GetVariety(ctx context.Context, id int64) (*model.CaneVariety, error) {
	return s.varieties.FindByID(ctx, id)
}

func (s *CaneService) CreateVariety(ctx context.Context, variety *model.CaneVariety) error {
	return s.varieties.Create(ctx, variety)
}

func (s *CaneService) UpdateVariety(ctx context.Context, variety *model.CaneVariety) error {
	return s.varieties.Update(ctx, variety)
}

func (s *CaneService) DeactivateVariety(ctx context.Context, id int64, version int) error {
	return s.varieties.Deactivate(ctx, id, version)
}

// --- growers -------------------------------------------------------------

func (s *CaneService) ListGrowers(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.Grower], error) {
	return s.growers.List(ctx, opts)
}

func (s *CaneService) GetGrower(ctx context.Context, companyID, id int64) (*model.Grower, error) {
	return s.growers.FindByID(ctx, companyID, id)
}

func (s *CaneService) CreateGrower(ctx context.Context, grower *model.Grower) error {
	if err := s.validateGrower(grower); err != nil {
		return err
	}
	if err := s.growers.Create(ctx, grower); err != nil {
		return err
	}
	s.auditSvc.Record(ctx, audit.Event{
		TableName: "growers", RecordID: &grower.ID, CompanyID: &grower.CompanyID,
		Action: model.AuditInsert, NewValues: grower,
	})
	return nil
}

func (s *CaneService) UpdateGrower(ctx context.Context, grower *model.Grower) error {
	if err := s.validateGrower(grower); err != nil {
		return err
	}
	return s.growers.Update(ctx, grower.CompanyID, grower)
}

func (s *CaneService) DeactivateGrower(ctx context.Context, companyID, id int64, version int) error {
	return s.growers.Deactivate(ctx, companyID, id, version)
}

// validateGrower keeps the purchased-cane fields coherent: only purchased cane
// carries a contract and a price, and a price without a currency cannot be
// added up later.
func (s *CaneService) validateGrower(grower *model.Grower) error {
	if grower.SupplyType == model.SupplyTypeOwnEstate {
		if grower.PricePerTon != nil || grower.ContractNo != nil {
			return apperrors.ErrValidation.
				Msgf("own-estate cane is not purchased, so it carries no contract or price").
				WithDetails(apperrors.Detail{Field: "supplyType", Value: grower.SupplyType})
		}
	}
	if grower.PricePerTon != nil && (grower.Currency == nil || *grower.Currency == "") {
		return apperrors.ErrValidation.
			Msgf("a contract price needs the currency it is agreed in").
			WithDetails(apperrors.Detail{Field: "currency"})
	}
	return nil
}

// --- cane fields ---------------------------------------------------------

func (s *CaneService) ListFields(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.CaneField], error) {
	return s.fields.List(ctx, opts)
}

func (s *CaneService) GetField(ctx context.Context, companyID, id int64) (*model.CaneField, error) {
	return s.fields.FindByID(ctx, companyID, id)
}

func (s *CaneService) FieldsForGrower(ctx context.Context, companyID, growerID int64) ([]model.CaneField, error) {
	if _, err := s.growers.FindByID(ctx, companyID, growerID); err != nil {
		return nil, err
	}
	return s.fields.ListForGrower(ctx, companyID, growerID)
}

func (s *CaneService) CreateField(ctx context.Context, field *model.CaneField) error {
	if _, err := s.growers.FindByID(ctx, field.CompanyID, field.GrowerID); err != nil {
		return crossCompany(err, "growerId")
	}
	if err := s.fields.Create(ctx, field); err != nil {
		return err
	}
	s.auditSvc.Record(ctx, audit.Event{
		TableName: "cane_fields", RecordID: &field.ID, CompanyID: &field.CompanyID,
		Action: model.AuditInsert, NewValues: field,
	})
	return nil
}

func (s *CaneService) UpdateField(ctx context.Context, field *model.CaneField) error {
	if _, err := s.growers.FindByID(ctx, field.CompanyID, field.GrowerID); err != nil {
		return crossCompany(err, "growerId")
	}
	return s.fields.Update(ctx, field.CompanyID, field)
}

func (s *CaneService) DeactivateField(ctx context.Context, companyID, id int64, version int) error {
	return s.fields.Deactivate(ctx, companyID, id, version)
}

// --- the harvest matrix --------------------------------------------------

// SaveHarvestMatrix upserts the Date × Grower grid. Like the production
// matrix it is idempotent, and like the production matrix it is refused
// outright once the version has been approved — enforced here and again by a
// database trigger.
func (s *CaneService) SaveHarvestMatrix(ctx context.Context, req HarvestMatrixRequest, actorID int64) (*HarvestMatrixResponse, error) {
	if req.DateTo.Before(req.DateFrom) {
		return nil, apperrors.ErrInvalidDateRange
	}

	var response *HarvestMatrixResponse

	err := s.uow.Do(ctx, func(ctx context.Context) error {
		version, err := s.versions.FindByID(ctx, req.CompanyID, req.VersionID)
		if err != nil {
			return err
		}
		if err := s.assertVersionEditable(ctx, version, actorID); err != nil {
			return err
		}
		season, err := s.refs.Season(ctx, req.CompanyID, version.SeasonID)
		if err != nil {
			return err
		}

		uomID := req.UOMID
		if uomID == 0 {
			material, err := s.defaultCaneMaterial(ctx, req.CompanyID)
			if err != nil {
				return err
			}
			uomID = material.BaseUOMID
		}

		header, err := s.resolveHarvestHeader(ctx, req, version, season)
		if err != nil {
			return err
		}

		items := make([]model.HarvestPlanItem, 0, len(req.Cells))
		provided := make(map[string]bool, len(req.Cells))

		for _, cell := range req.Cells {
			if !season.Contains(cell.PlanDate) {
				return apperrors.ErrDateOutsideSeason.WithDetails(apperrors.Detail{
					Field: "planDate", Value: cell.PlanDate.Format("2006-01-02"),
				})
			}
			grower, err := s.growers.FindByID(ctx, req.CompanyID, cell.GrowerID)
			if err != nil {
				return crossCompany(err, "growerId")
			}
			if err := s.assertFieldBelongsToGrower(ctx, req.CompanyID, grower.ID, cell.CaneFieldID); err != nil {
				return err
			}
			provided[harvestKey(cell.PlanDate, cell.GrowerID, cell.CaneFieldID)] = true

			if cell.PlannedTons == nil {
				continue // explicit null: leave the stored value untouched
			}
			if cell.PlannedTons.IsNegative() {
				return apperrors.ErrValidation.Msgf("a planned tonnage must not be negative").
					WithDetails(apperrors.Detail{Field: "plannedTons"})
			}

			items = append(items, model.HarvestPlanItem{
				HarvestPlanHeaderID: header.ID,
				PlanDate:            cell.PlanDate,
				GrowerID:            cell.GrowerID,
				CaneFieldID:         cell.CaneFieldID,
				VarietyID:           cell.VarietyID,
				PlannedTons:         *cell.PlannedTons,
				UOMID:               uomID,
				PlannedAreaHa:       cell.PlannedAreaHa,
				ExpectedCCSPct:      cell.ExpectedCCSPct,
				Remark:              cell.Remark,
			})
		}

		if err := s.plans.UpsertItems(ctx, header.ID, items); err != nil {
			return err
		}

		if !req.PartialUpdate {
			existing, err := s.plans.MatrixCells(ctx, req.CompanyID, req.VersionID, req.DateFrom, req.DateTo)
			if err != nil {
				return err
			}
			var obsolete []int64
			for _, cell := range existing {
				if !provided[harvestKey(cell.PlanDate, cell.GrowerID, cell.CaneFieldID)] {
					obsolete = append(obsolete, cell.HarvestPlanItemID)
				}
			}
			if err := s.plans.DeleteItems(ctx, header.ID, obsolete); err != nil {
				return err
			}
		}

		s.auditSvc.Record(ctx, audit.Event{
			TableName: "harvest_plan_items", RecordID: &header.ID, CompanyID: &req.CompanyID,
			Action: model.AuditUpdate,
			NewValues: map[string]any{
				"versionId": req.VersionID, "cells": len(items), "partialUpdate": req.PartialUpdate,
			},
		})

		response, err = s.loadHarvestMatrix(ctx, req.CompanyID, version, header, req.DateFrom, req.DateTo)
		return err
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetHarvestMatrix reads the grid back in the shape the save accepts.
func (s *CaneService) GetHarvestMatrix(ctx context.Context, companyID, versionID int64,
	from, to time.Time) (*HarvestMatrixResponse, error) {

	if to.Before(from) {
		return nil, apperrors.ErrInvalidDateRange
	}
	version, err := s.versions.FindByID(ctx, companyID, versionID)
	if err != nil {
		return nil, err
	}
	header, err := s.plans.FindHeaderByVersion(ctx, companyID, versionID)
	if err != nil {
		return nil, err
	}
	return s.loadHarvestMatrix(ctx, companyID, version, header, from, to)
}

func (s *CaneService) loadHarvestMatrix(ctx context.Context, companyID int64,
	version *model.PlanningVersion, header *model.HarvestPlanHeader,
	from, to time.Time) (*HarvestMatrixResponse, error) {

	cells, err := s.plans.MatrixCells(ctx, companyID, version.ID, from, to)
	if err != nil {
		return nil, err
	}
	growers, err := s.growers.List(ctx, interfaces.ListOptions{
		CompanyID: &companyID, Size: interfaces.MaxPageSize,
	})
	if err != nil {
		return nil, err
	}

	rows := make([]HarvestMatrixRow, 0)
	byDate := make(map[string]*HarvestMatrixRow)
	// Every day of the range gets a row, so the client renders a complete grid
	// rather than only the days that happen to carry a value.
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		rows = append(rows, HarvestMatrixRow{PlanDate: day})
	}
	for i := range rows {
		byDate[rows[i].PlanDate.Format("2006-01-02")] = &rows[i]
	}

	for _, cell := range cells {
		row, ok := byDate[cell.PlanDate.Format("2006-01-02")]
		if !ok {
			continue
		}
		row.Values = append(row.Values, HarvestMatrixValue{
			GrowerID:       cell.GrowerID,
			CaneFieldID:    cell.CaneFieldID,
			VarietyID:      cell.VarietyID,
			PlannedTons:    cell.PlannedTons,
			PlannedAreaHa:  cell.PlannedAreaHa,
			ExpectedCCSPct: cell.ExpectedCCSPct,
			UOMID:          cell.UOMID,
			Version:        cell.Version,
		})
	}

	response := &HarvestMatrixResponse{
		CompanyID:     companyID,
		VersionID:     version.ID,
		VersionStatus: version.Status,
		DateFrom:      from,
		DateTo:        to,
		Growers:       growers.Rows,
		Rows:          rows,
	}
	if header != nil {
		response.DocumentNo = header.DocumentNo
	}
	return response, nil
}

func (s *CaneService) resolveHarvestHeader(ctx context.Context, req HarvestMatrixRequest,
	version *model.PlanningVersion, season *model.Season) (*model.HarvestPlanHeader, error) {

	header, err := s.plans.FindHeaderByVersion(ctx, req.CompanyID, req.VersionID)
	if err != nil {
		return nil, err
	}
	if header != nil {
		// Widen the document period when the user plans outside it.
		changed := false
		if req.DateFrom.Before(header.DateFrom) {
			header.DateFrom = req.DateFrom
			changed = true
		}
		if req.DateTo.After(header.DateTo) {
			header.DateTo = req.DateTo
			changed = true
		}
		if changed {
			if err := s.plans.UpdateHeader(ctx, req.CompanyID, header); err != nil {
				return nil, err
			}
		}
		return header, nil
	}

	documentNo, err := s.numbering.Next(ctx, req.CompanyID, model.NumberObjectHarvest, req.DateFrom)
	if err != nil {
		return nil, err
	}
	header = &model.HarvestPlanHeader{
		CompanyID:         req.CompanyID,
		SeasonID:          season.ID,
		PlanningVersionID: version.ID,
		DocumentNo:        documentNo,
		DateFrom:          req.DateFrom,
		DateTo:            req.DateTo,
	}
	if err := s.plans.CreateHeader(ctx, header); err != nil {
		return nil, err
	}
	return header, nil
}

func (s *CaneService) HarvestPlans(ctx context.Context, opts interfaces.ListOptions,
	versionID *int64) (interfaces.Page[model.HarvestPlanHeader], error) {
	return s.plans.ListHeaders(ctx, opts, versionID)
}

func (s *CaneService) HarvestPlanItems(ctx context.Context, companyID, headerID int64) ([]model.HarvestPlanItem, error) {
	if _, err := s.plans.FindHeader(ctx, companyID, headerID); err != nil {
		return nil, err
	}
	return s.plans.Items(ctx, headerID)
}

// assertVersionEditable is the cane-side half of the §F3 guard, identical to
// the production plan's: DRAFT is open, SUBMITTED needs the extra permission,
// anything else is refused. The database trigger is the other half.
func (s *CaneService) assertVersionEditable(ctx context.Context, version *model.PlanningVersion, actorID int64) error {
	switch version.Status {
	case model.VersionStatusDraft:
		return nil
	case model.VersionStatusSubmitted:
		granted, err := s.authz.HasPermission(ctx, actorID, version.CompanyID, PermPlanEditSubmitted)
		if err != nil {
			return err
		}
		if !granted {
			return apperrors.ErrPermissionDenied.
				Msgf("changing a SUBMITTED version requires %s", PermPlanEditSubmitted)
		}
		return nil
	default:
		return apperrors.ErrVersionNotEditable.Msgf(
			"planning version is %s and cannot be modified", version.Status)
	}
}

func (s *CaneService) assertFieldBelongsToGrower(ctx context.Context, companyID, growerID int64, fieldID *int64) error {
	if fieldID == nil {
		return nil
	}
	field, err := s.fields.FindByID(ctx, companyID, *fieldID)
	if err != nil {
		return crossCompany(err, "caneFieldId")
	}
	if field.GrowerID != growerID {
		return apperrors.ErrValidation.
			Msgf("field %s belongs to another grower", field.FieldCode).
			WithDetails(apperrors.Detail{Field: "caneFieldId"})
	}
	return nil
}

// --- deliveries ----------------------------------------------------------

func (s *CaneService) ListDeliveries(ctx context.Context, opts interfaces.ListOptions,
	f interfaces.DeliveryFilter) (interfaces.Page[model.CaneDelivery], error) {
	return s.deliveries.List(ctx, opts, f)
}

func (s *CaneService) GetDelivery(ctx context.Context, companyID, id int64) (*model.CaneDelivery, error) {
	return s.deliveries.FindByID(ctx, companyID, id)
}

// CreateDelivery records a weighbridge ticket as a draft. An Idempotency-Key
// makes a retried POST return the ticket the first attempt created rather than
// weighing the same lorry twice (§E8).
func (s *CaneService) CreateDelivery(ctx context.Context, delivery *model.CaneDelivery, idempotencyKey string) (*model.CaneDelivery, error) {
	if idempotencyKey != "" {
		existing, err := s.deliveries.FindByIdempotencyKey(ctx, delivery.CompanyID, idempotencyKey)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
		delivery.IdempotencyKey = &idempotencyKey
	}

	err := s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.prepareDelivery(ctx, delivery); err != nil {
			return err
		}

		documentNo, err := s.numbering.Next(ctx, delivery.CompanyID, model.NumberObjectDelivery, delivery.DeliveryDate)
		if err != nil {
			return err
		}
		delivery.DocumentNo = documentNo
		delivery.PostingStatus = model.PostingStatusDraft

		if err := s.deliveries.Create(ctx, delivery); err != nil {
			return err
		}
		s.auditSvc.Record(ctx, audit.Event{
			TableName: "cane_deliveries", RecordID: &delivery.ID, CompanyID: &delivery.CompanyID,
			Action: model.AuditInsert, NewValues: delivery,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Net weight is computed by the database, so the ticket is read back rather
	// than returned from the in-memory struct.
	return s.deliveries.FindByID(ctx, delivery.CompanyID, delivery.ID)
}

// UpdateDelivery changes a draft ticket. A posted one is immutable: it is
// corrected by a reversal, never by an edit.
func (s *CaneService) UpdateDelivery(ctx context.Context, delivery *model.CaneDelivery) (*model.CaneDelivery, error) {
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		current, err := s.deliveries.FindByID(ctx, delivery.CompanyID, delivery.ID)
		if err != nil {
			return err
		}
		if current.PostingStatus != model.PostingStatusDraft {
			return apperrors.ErrDocumentNotDraft.Msgf(
				"delivery %s is %s and can no longer be changed", current.DocumentNo, current.PostingStatus)
		}
		if err := s.prepareDelivery(ctx, delivery); err != nil {
			return err
		}

		current.DeliveryDate = delivery.DeliveryDate
		current.GrowerID = delivery.GrowerID
		current.CaneFieldID = delivery.CaneFieldID
		current.VarietyID = delivery.VarietyID
		current.MaterialID = delivery.MaterialID
		current.MovementTypeID = delivery.MovementTypeID
		current.WarehouseID = delivery.WarehouseID
		current.UOMID = delivery.UOMID
		current.SeasonID = delivery.SeasonID
		current.TicketNo = delivery.TicketNo
		current.VehicleNo = delivery.VehicleNo
		current.GrossTons = delivery.GrossTons
		current.TareTons = delivery.TareTons
		current.PricePerTon = delivery.PricePerTon
		current.Currency = delivery.Currency
		current.CCSPct = delivery.CCSPct
		current.TrashPct = delivery.TrashPct
		current.IsBurnt = delivery.IsBurnt
		current.Remark = delivery.Remark
		current.Version = delivery.Version

		return s.deliveries.Update(ctx, current.CompanyID, current)
	})
	if err != nil {
		return nil, err
	}
	return s.deliveries.FindByID(ctx, delivery.CompanyID, delivery.ID)
}

// PostDelivery turns a draft ticket into stock: one inventory movement for the
// net weight, written in the same transaction as the status change. Every
// §F4 rule — negative stock, capacity, the open season, the location's allowed
// materials — is applied by the inventory service, so cane is not a special
// case anywhere.
func (s *CaneService) PostDelivery(ctx context.Context, companyID, id, actorID int64) (*model.CaneDelivery, error) {
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		delivery, err := s.deliveries.FindByID(ctx, companyID, id)
		if err != nil {
			return err
		}
		if delivery.PostingStatus != model.PostingStatusDraft {
			return apperrors.ErrNotPostable.Msgf(
				"delivery %s is %s", delivery.DocumentNo, delivery.PostingStatus)
		}

		if delivery.WarehouseID != nil {
			_, err = s.inventory.Apply(ctx, []MovementRequest{{
				CompanyID:         companyID,
				DocumentNo:        delivery.DocumentNo,
				TransactionDate:   delivery.DeliveryDate,
				MaterialID:        delivery.MaterialID,
				WarehouseID:       *delivery.WarehouseID,
				MovementTypeID:    delivery.MovementTypeID,
				Quantity:          delivery.NetTons,
				UOMID:             delivery.UOMID,
				SourceModule:      model.SourceModuleActual,
				ReferenceDocument: &delivery.DocumentNo,
				ReferenceID:       &delivery.ID,
				Remark:            delivery.Remark,
			}})
			if err != nil {
				return err
			}
		}

		now := time.Now().UTC()
		delivery.PostingStatus = model.PostingStatusPosted
		delivery.PostedBy = &actorID
		delivery.PostedAt = &now
		if err := s.deliveries.SetStatus(ctx, delivery); err != nil {
			return err
		}

		s.auditSvc.Record(ctx, audit.Event{
			TableName: "cane_deliveries", RecordID: &delivery.ID, CompanyID: &companyID,
			Action: model.AuditPost,
			NewValues: map[string]any{
				"documentNo": delivery.DocumentNo, "netTons": delivery.NetTons.String(),
			},
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.deliveries.FindByID(ctx, companyID, id)
}

// ReverseDelivery cancels a posted ticket: the movement it wrote gets an
// opposite entry and the ticket is flagged REVERSED. Nothing is deleted.
func (s *CaneService) ReverseDelivery(ctx context.Context, companyID, id, actorID int64, remark *string) (*model.CaneDelivery, error) {
	err := s.uow.Do(ctx, func(ctx context.Context) error {
		delivery, err := s.deliveries.FindByID(ctx, companyID, id)
		if err != nil {
			return err
		}
		if delivery.PostingStatus != model.PostingStatusPosted {
			return apperrors.ErrNotReversible.Msgf(
				"delivery %s is %s", delivery.DocumentNo, delivery.PostingStatus)
		}

		movements, err := s.inventory.ListMovements(ctx,
			interfaces.ListOptions{CompanyID: &companyID, Size: interfaces.MaxPageSize},
			interfaces.MovementFilter{DocumentNo: delivery.DocumentNo})
		if err != nil {
			return err
		}
		for _, movement := range movements.Rows {
			if movement.IsReversed || movement.SourceModule == model.SourceModuleReversal {
				continue
			}
			if _, err := s.inventory.Reverse(ctx, companyID, movement.ID, remark); err != nil {
				return err
			}
		}

		now := time.Now().UTC()
		delivery.PostingStatus = model.PostingStatusReversed
		delivery.ReversedBy = &actorID
		delivery.ReversedAt = &now
		if err := s.deliveries.SetStatus(ctx, delivery); err != nil {
			return err
		}

		s.auditSvc.Record(ctx, audit.Event{
			TableName: "cane_deliveries", RecordID: &delivery.ID, CompanyID: &companyID,
			Action: model.AuditReverse, NewValues: map[string]any{"documentNo": delivery.DocumentNo},
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.deliveries.FindByID(ctx, companyID, id)
}

// prepareDelivery fills in what the caller may legitimately omit and validates
// every reference against the delivery's own company.
func (s *CaneService) prepareDelivery(ctx context.Context, delivery *model.CaneDelivery) error {
	if delivery.GrossTons.LessThanOrEqual(delivery.TareTons) {
		return apperrors.ErrValidation.
			Msgf("the gross weight must exceed the tare weight").
			WithDetails(apperrors.Detail{Field: "grossTons"})
	}

	grower, err := s.growers.FindByID(ctx, delivery.CompanyID, delivery.GrowerID)
	if err != nil {
		return crossCompany(err, "growerId")
	}
	if err := s.assertFieldBelongsToGrower(ctx, delivery.CompanyID, grower.ID, delivery.CaneFieldID); err != nil {
		return err
	}

	// The contract price is copied from the grower once, at ticket time, so
	// that re-negotiating a contract cannot rewrite what an already-delivered
	// load was worth. Own-estate cane has no price and gets none.
	if delivery.PricePerTon == nil && grower.IsPurchased() {
		delivery.PricePerTon = grower.PricePerTon
		if delivery.Currency == nil {
			delivery.Currency = grower.Currency
		}
	}

	if delivery.MaterialID == 0 {
		material, err := s.defaultCaneMaterial(ctx, delivery.CompanyID)
		if err != nil {
			return err
		}
		delivery.MaterialID = material.ID
		if delivery.UOMID == 0 {
			delivery.UOMID = material.BaseUOMID
		}
	}
	if _, _, err := s.refs.MaterialForCompany(ctx, delivery.CompanyID, delivery.MaterialID); err != nil {
		return err
	}
	if delivery.UOMID == 0 {
		material, err := s.materials.FindByID(ctx, delivery.MaterialID)
		if err != nil {
			return err
		}
		delivery.UOMID = material.BaseUOMID
	}

	if delivery.MovementTypeID == 0 {
		// Only the default is resolved by code; nothing branches on it. The
		// sign of the movement still comes from movement_types.direction (§33).
		movementType, err := s.movements.FindByCode(ctx, defaultCaneIntakeMovement)
		if err != nil {
			return apperrors.ErrValidation.
				Msgf("no movement type is configured for cane intake — name one explicitly").
				WithDetails(apperrors.Detail{Field: "movementTypeId"})
		}
		delivery.MovementTypeID = movementType.ID
	}

	if delivery.WarehouseID != nil {
		if _, err := s.refs.Warehouse(ctx, delivery.CompanyID, *delivery.WarehouseID); err != nil {
			return err
		}
	}
	if delivery.VarietyID == nil && delivery.CaneFieldID != nil {
		field, err := s.fields.FindByID(ctx, delivery.CompanyID, *delivery.CaneFieldID)
		if err != nil {
			return err
		}
		delivery.VarietyID = field.VarietyID
	}

	// The season is resolved from the delivery date when the caller did not
	// name one, so cane reports can group by season without the user having to
	// know which season a date falls in.
	if delivery.SeasonID == nil {
		season, err := s.seasons.FindContaining(ctx, delivery.CompanyID, delivery.DeliveryDate)
		if err != nil {
			return err
		}
		if season != nil {
			delivery.SeasonID = &season.ID
		}
	} else if _, err := s.refs.Season(ctx, delivery.CompanyID, *delivery.SeasonID); err != nil {
		return err
	}
	return nil
}

// defaultCaneIntakeMovement is the movement type a delivery uses when the
// caller does not name one.
const defaultCaneIntakeMovement = "CANE_INTAKE"

// defaultCaneMaterial resolves the material a delivery is booked as. It is
// found through the material group rather than a hard-coded code, and an
// ambiguous configuration is reported instead of guessed.
func (s *CaneService) defaultCaneMaterial(ctx context.Context, companyID int64) (*model.Material, error) {
	candidates, err := s.materials.ListByGroup(ctx, model.MaterialGroupCane)
	if err != nil {
		return nil, err
	}

	relevant := make([]model.Material, 0, len(candidates))
	for _, material := range candidates {
		assignment, err := s.materials.FindCompanyMaterial(ctx, companyID, material.ID)
		if err != nil {
			return nil, err
		}
		if assignment != nil {
			relevant = append(relevant, material)
		}
	}

	if len(relevant) != 1 {
		return nil, apperrors.ErrValidation.
			Msgf("this company has %d materials in group %s, so the delivery must name one",
				len(relevant), model.MaterialGroupCane).
			WithDetails(apperrors.Detail{Field: "materialId"})
	}
	return &relevant[0], nil
}

// --- reporting -----------------------------------------------------------

// CanePlanVsActualRow is the report row with the variance already applied,
// keeping the null rule of §F6: no plan means no percentage, not zero.
type CanePlanVsActualRow struct {
	interfaces.CanePlanVsActualRow
	VariancePct *decimal.Decimal
}

func (s *CaneService) CanePlanVsActual(ctx context.Context, q interfaces.CanePlanVsActualQuery) ([]CanePlanVsActualRow, error) {
	if q.Range.To.Before(q.Range.From) {
		return nil, apperrors.ErrInvalidDateRange
	}
	rows, err := s.reports.CanePlanVsActual(ctx, q)
	if err != nil {
		return nil, err
	}

	out := make([]CanePlanVsActualRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, CanePlanVsActualRow{
			CanePlanVsActualRow: row,
			VariancePct:         VariancePercent(row.PlannedTons, row.ActualTons),
		})
	}
	return out, nil
}

func harvestKey(date time.Time, growerID int64, fieldID *int64) string {
	key := date.Format("2006-01-02") + "|" + strconv.FormatInt(growerID, 10)
	if fieldID == nil {
		return key + "|-"
	}
	return key + "|" + strconv.FormatInt(*fieldID, 10)
}

// crossCompany turns "not found in this company" into the cross-company
// reference error, which is what the caller actually did wrong (§C2).
func crossCompany(err error, field string) error {
	if appErr, ok := apperrors.As(err); ok && appErr.Code == apperrors.ErrNotFound.Code {
		return apperrors.ErrCrossCompanyReference.
			WithDetails(apperrors.Detail{Field: field, Message: "unknown in this company"})
	}
	return err
}
