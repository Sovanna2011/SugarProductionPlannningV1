package service

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/audit"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/config"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/database"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
)

// MatrixCellInput is one editable cell of the Date × Line grid.
//
// A nil Quantity means "leave unchanged" — it is not the same as a cell that
// the client omitted, which means "delete" when partialUpdate is off (§E3).
type MatrixCellInput struct {
	PlanDate         time.Time
	ProductionLineID *int64
	Quantity         *decimal.Decimal
}

// MatrixRequest is the matrix-shaped bulk save of §28.
type MatrixRequest struct {
	CompanyID      int64
	SeasonID       int64
	VersionID      int64
	MovementTypeID int64
	MaterialID     int64
	ProcessID      *int64
	UOMID          int64
	WarehouseID    *int64
	DateFrom       time.Time
	DateTo         time.Time
	Cells          []MatrixCellInput
	PartialUpdate  bool
}

// MatrixResponse is the round-trippable view of the same grid.
type MatrixResponse struct {
	CompanyID      int64
	VersionID      int64
	VersionStatus  string
	MovementTypeID int64
	MaterialID     int64
	ProcessID      *int64
	DateFrom       time.Time
	DateTo         time.Time
	Lines          []model.ProductionLine
	Rows           []MatrixRow
}

type MatrixRow struct {
	PlanDate time.Time
	Values   []MatrixValue
}

type MatrixValue struct {
	ProductionLineID *int64
	Quantity         decimal.Decimal
	UOMID            int64
	Version          int
}

type PlanningService struct {
	versions  interfaces.PlanningVersionRepository
	plans     interfaces.PlanRepository
	lines     interfaces.ProductionLineRepository
	numbering *NumberRangeService
	authz     *AuthorizationService
	refs      *ReferenceValidator
	auditSvc  *audit.Service
	uow       database.UnitOfWork
	cfg       config.BusinessConfig
}

func NewPlanningService(
	versions interfaces.PlanningVersionRepository,
	plans interfaces.PlanRepository,
	lines interfaces.ProductionLineRepository,
	numbering *NumberRangeService,
	authz *AuthorizationService,
	refs *ReferenceValidator,
	auditSvc *audit.Service,
	uow database.UnitOfWork,
	cfg config.BusinessConfig,
) *PlanningService {
	return &PlanningService{
		versions: versions, plans: plans, lines: lines, numbering: numbering,
		authz: authz, refs: refs, auditSvc: auditSvc, uow: uow, cfg: cfg,
	}
}

// --- versions ------------------------------------------------------------

func (s *PlanningService) VersionsForSeason(ctx context.Context, companyID, seasonID int64) ([]model.PlanningVersion, error) {
	if _, err := s.refs.Season(ctx, companyID, seasonID); err != nil {
		return nil, err
	}
	return s.versions.FindBySeason(ctx, companyID, seasonID)
}

func (s *PlanningService) GetVersion(ctx context.Context, companyID, id int64) (*model.PlanningVersion, error) {
	return s.versions.FindByID(ctx, companyID, id)
}

// CreateVersion allocates the next free version number when the caller does
// not supply one. Numbering is open-ended: nothing caps it at three (§29).
func (s *PlanningService) CreateVersion(ctx context.Context, version *model.PlanningVersion) error {
	if _, err := s.refs.Season(ctx, version.CompanyID, version.SeasonID); err != nil {
		return err
	}

	return s.uow.Do(ctx, func(ctx context.Context) error {
		if version.VersionNo <= 0 {
			next, err := s.versions.NextVersionNo(ctx, version.CompanyID, version.SeasonID)
			if err != nil {
				return err
			}
			version.VersionNo = next
		} else {
			exists, err := s.versions.ExistsVersionNo(ctx, version.CompanyID, version.SeasonID, version.VersionNo)
			if err != nil {
				return err
			}
			if exists {
				return apperrors.ErrVersionExists.WithDetails(
					apperrors.Detail{Field: "versionNo"})
			}
		}

		version.Status = model.VersionStatusDraft
		if err := s.versions.Create(ctx, version); err != nil {
			return err
		}

		s.auditSvc.Record(ctx, audit.Event{
			TableName: "planning_versions", RecordID: &version.ID, CompanyID: &version.CompanyID,
			Action: model.AuditInsert, NewValues: version,
		})
		return nil
	})
}

// UpdateVersion changes descriptive attributes only. The status is moved by
// Transition, never by an ordinary field edit.
func (s *PlanningService) UpdateVersion(ctx context.Context, version *model.PlanningVersion) error {
	current, err := s.versions.FindByID(ctx, version.CompanyID, version.ID)
	if err != nil {
		return err
	}
	if current.Status == model.VersionStatusLocked || current.Status == model.VersionStatusCancelled {
		return apperrors.ErrVersionNotEditable.Msgf(
			"planning version is %s and cannot be modified", current.Status)
	}

	current.VersionName = version.VersionName
	current.Description = version.Description
	current.Version = version.Version

	return s.versions.Update(ctx, current.CompanyID, current)
}

// CopyVersion implements §F2: a deep, independent snapshot. Editing the copy
// can never touch the source — the only link is the informational
// copied_from_version_id.
func (s *PlanningService) CopyVersion(ctx context.Context, companyID, sourceID int64,
	newVersionNo int, name string, description *string) (*model.PlanningVersion, error) {

	var created *model.PlanningVersion

	err := s.uow.Do(ctx, func(ctx context.Context) error {
		source, err := s.versions.FindByID(ctx, companyID, sourceID)
		if err != nil {
			return err
		}

		if newVersionNo <= 0 {
			newVersionNo, err = s.versions.NextVersionNo(ctx, companyID, source.SeasonID)
			if err != nil {
				return err
			}
		} else {
			exists, err := s.versions.ExistsVersionNo(ctx, companyID, source.SeasonID, newVersionNo)
			if err != nil {
				return err
			}
			if exists {
				return apperrors.ErrVersionExists.WithDetails(
					apperrors.Detail{Field: "newVersionNo"})
			}
		}

		target := &model.PlanningVersion{
			CompanyID:           companyID,
			SeasonID:            source.SeasonID,
			VersionNo:           newVersionNo,
			VersionName:         name,
			Description:         description,
			Status:              model.VersionStatusDraft,
			CopiedFromVersionID: &source.ID,
		}
		if target.VersionName == "" {
			target.VersionName = source.VersionName
		}
		if err := s.versions.Create(ctx, target); err != nil {
			return err
		}

		// Every copied header needs its own document number, allocated from
		// the same range as an originally created document.
		headers, err := s.plans.ListHeaders(ctx,
			interfaces.ListOptions{CompanyID: &companyID, Size: interfaces.MaxPageSize},
			&sourceID, nil)
		if err != nil {
			return err
		}

		docNos := make(map[int64]string, len(headers.Rows))
		for _, header := range headers.Rows {
			no, err := s.numbering.Next(ctx, companyID, model.NumberObjectPlan, header.DateFrom)
			if err != nil {
				return err
			}
			docNos[header.ID] = no
		}

		if err := s.versions.DeepCopy(ctx, sourceID, target.ID, docNos); err != nil {
			return err
		}

		created = target
		s.auditSvc.Record(ctx, audit.Event{
			TableName: "planning_versions", RecordID: &target.ID, CompanyID: &companyID,
			Action: model.AuditInsert,
			NewValues: map[string]any{
				"copiedFrom": sourceID, "versionNo": newVersionNo, "headers": len(docNos),
			},
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// Transition moves the version through the state machine of §F3. The allowed
// edges live on the model, so the machine has exactly one definition.
func (s *PlanningService) Transition(ctx context.Context, companyID, versionID int64,
	target string, actorID int64) (*model.PlanningVersion, error) {

	var updated *model.PlanningVersion

	err := s.uow.Do(ctx, func(ctx context.Context) error {
		version, err := s.versions.FindByID(ctx, companyID, versionID)
		if err != nil {
			return err
		}
		if !version.CanTransitionTo(target) {
			return apperrors.ErrInvalidTransition.Msgf(
				"a %s version cannot become %s", version.Status, target)
		}

		now := time.Now().UTC()
		switch target {
		case model.VersionStatusSubmitted:
			version.SubmittedBy = &actorID
			version.SubmittedAt = &now
		case model.VersionStatusApproved:
			// OQ-5: four eyes — the approver must not be the creator.
			if s.cfg.EnforceFourEyes && version.CreatedBy != nil && *version.CreatedBy == actorID {
				return apperrors.ErrFourEyesViolation
			}
			version.ApprovedBy = &actorID
			version.ApprovedAt = &now
		case model.VersionStatusLocked:
			version.LockedBy = &actorID
			version.LockedAt = &now
		}

		previous := version.Status
		version.Status = target
		if err := s.versions.UpdateStatus(ctx, version); err != nil {
			return err
		}

		action := model.AuditUpdate
		if target == model.VersionStatusApproved {
			action = model.AuditApprove
		}
		s.auditSvc.Record(ctx, audit.Event{
			TableName: "planning_versions", RecordID: &version.ID, CompanyID: &companyID,
			Action:    action,
			OldValues: map[string]string{"status": previous},
			NewValues: map[string]string{"status": target},
		})

		updated = version
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// assertEditable is the Go-side half of the §F3 guard. The database trigger is
// the other half, so no code path — including a future batch job — can write
// to an approved or locked version.
func (s *PlanningService) assertEditable(ctx context.Context, version *model.PlanningVersion, actorID int64) error {
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

// --- plan documents ------------------------------------------------------

func (s *PlanningService) ListPlans(ctx context.Context, opts interfaces.ListOptions,
	versionID, movementTypeID *int64) (interfaces.Page[model.PlanHeader], error) {
	return s.plans.ListHeaders(ctx, opts, versionID, movementTypeID)
}

func (s *PlanningService) GetPlan(ctx context.Context, companyID, headerID int64) (*model.PlanHeader, error) {
	return s.plans.FindHeader(ctx, companyID, headerID)
}

func (s *PlanningService) PlanItems(ctx context.Context, companyID, headerID int64) ([]model.PlanItem, error) {
	if _, err := s.plans.FindHeader(ctx, companyID, headerID); err != nil {
		return nil, err
	}
	return s.plans.Items(ctx, headerID)
}

// --- the planning matrix (§28) -------------------------------------------

// SaveMatrix upserts the Date × Line grid. The operation is idempotent: the
// same payload can be replayed without creating duplicate rows, because the
// upsert is keyed on the item business key.
func (s *PlanningService) SaveMatrix(ctx context.Context, req MatrixRequest, actorID int64) (*MatrixResponse, error) {
	if req.DateTo.Before(req.DateFrom) {
		return nil, apperrors.ErrInvalidDateRange
	}

	var response *MatrixResponse

	err := s.uow.Do(ctx, func(ctx context.Context) error {
		version, err := s.versions.FindByID(ctx, req.CompanyID, req.VersionID)
		if err != nil {
			return err
		}
		if err := s.assertEditable(ctx, version, actorID); err != nil {
			return err
		}

		season, err := s.refs.Season(ctx, req.CompanyID, version.SeasonID)
		if err != nil {
			return err
		}
		if _, _, err := s.refs.MaterialForCompany(ctx, req.CompanyID, req.MaterialID); err != nil {
			return err
		}

		header, err := s.resolveMatrixHeader(ctx, req, version, season)
		if err != nil {
			return err
		}

		items := make([]model.PlanItem, 0, len(req.Cells))
		provided := make(map[string]bool, len(req.Cells))

		for _, cell := range req.Cells {
			if !season.Contains(cell.PlanDate) {
				return apperrors.ErrDateOutsideSeason.WithDetails(apperrors.Detail{
					Field: "planDate", Value: cell.PlanDate.Format("2006-01-02"),
				})
			}
			if err := s.refs.ProductionLine(ctx, req.CompanyID, cell.ProductionLineID); err != nil {
				return err
			}
			provided[matrixKey(cell.PlanDate, cell.ProductionLineID)] = true

			if cell.Quantity == nil {
				continue // explicit null: leave the stored value untouched
			}
			if cell.Quantity.IsNegative() {
				return apperrors.ErrValidation.Msgf("a planned quantity must not be negative").
					WithDetails(apperrors.Detail{Field: "quantity"})
			}

			items = append(items, model.PlanItem{
				PlanHeaderID:     header.ID,
				PlanDate:         cell.PlanDate,
				ProductionLineID: cell.ProductionLineID,
				MaterialID:       req.MaterialID,
				ProcessID:        req.ProcessID,
				WarehouseID:      req.WarehouseID,
				Quantity:         *cell.Quantity,
				UOMID:            req.UOMID,
			})
		}

		if err := s.plans.UpsertItems(ctx, header.ID, items); err != nil {
			return err
		}

		// A full save treats an omitted cell as a deletion; a partial save
		// leaves everything it did not mention alone.
		if !req.PartialUpdate {
			existing, err := s.plans.MatrixCells(ctx, req.CompanyID, req.VersionID,
				req.MovementTypeID, req.MaterialID, req.ProcessID, req.DateFrom, req.DateTo)
			if err != nil {
				return err
			}
			var obsolete []int64
			for _, cell := range existing {
				if !provided[matrixKey(cell.PlanDate, cell.ProductionLineID)] {
					obsolete = append(obsolete, cell.PlanItemID)
				}
			}
			if err := s.plans.DeleteItems(ctx, header.ID, obsolete); err != nil {
				return err
			}
		}

		s.auditSvc.Record(ctx, audit.Event{
			TableName: "plan_items", RecordID: &header.ID, CompanyID: &req.CompanyID,
			Action: model.AuditUpdate,
			NewValues: map[string]any{
				"versionId": req.VersionID, "materialId": req.MaterialID,
				"cells": len(items), "partialUpdate": req.PartialUpdate,
			},
		})

		response, err = s.loadMatrix(ctx, req.CompanyID, version, req.MovementTypeID,
			req.MaterialID, req.ProcessID, req.DateFrom, req.DateTo)
		return err
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetMatrix reads the grid back in the same shape the save accepts.
func (s *PlanningService) GetMatrix(ctx context.Context, companyID, versionID,
	movementTypeID, materialID int64, processID *int64, from, to time.Time) (*MatrixResponse, error) {

	if to.Before(from) {
		return nil, apperrors.ErrInvalidDateRange
	}
	version, err := s.versions.FindByID(ctx, companyID, versionID)
	if err != nil {
		return nil, err
	}
	return s.loadMatrix(ctx, companyID, version, movementTypeID, materialID, processID, from, to)
}

func (s *PlanningService) loadMatrix(ctx context.Context, companyID int64,
	version *model.PlanningVersion, movementTypeID, materialID int64, processID *int64,
	from, to time.Time) (*MatrixResponse, error) {

	cells, err := s.plans.MatrixCells(ctx, companyID, version.ID, movementTypeID,
		materialID, processID, from, to)
	if err != nil {
		return nil, err
	}

	lines, err := s.lines.List(ctx, interfaces.ListOptions{
		CompanyID: &companyID, Size: interfaces.MaxPageSize,
	})
	if err != nil {
		return nil, err
	}

	byDate := make(map[string]*MatrixRow)
	rows := make([]MatrixRow, 0)

	// Every day of the range gets a row, so the client renders a complete grid
	// rather than only the days that happen to carry a value.
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		rows = append(rows, MatrixRow{PlanDate: day})
	}
	for i := range rows {
		byDate[rows[i].PlanDate.Format("2006-01-02")] = &rows[i]
	}

	for _, cell := range cells {
		row, ok := byDate[cell.PlanDate.Format("2006-01-02")]
		if !ok {
			continue
		}
		row.Values = append(row.Values, MatrixValue{
			ProductionLineID: cell.ProductionLineID,
			Quantity:         cell.Quantity,
			UOMID:            cell.UOMID,
			Version:          cell.Version,
		})
	}

	return &MatrixResponse{
		CompanyID:      companyID,
		VersionID:      version.ID,
		VersionStatus:  version.Status,
		MovementTypeID: movementTypeID,
		MaterialID:     materialID,
		ProcessID:      processID,
		DateFrom:       from,
		DateTo:         to,
		Lines:          lines.Rows,
		Rows:           rows,
	}, nil
}

// resolveMatrixHeader finds or creates the document the matrix writes into.
// A matrix spans several production lines, so its header carries no line and
// the line lives on each item instead.
func (s *PlanningService) resolveMatrixHeader(ctx context.Context, req MatrixRequest,
	version *model.PlanningVersion, season *model.Season) (*model.PlanHeader, error) {

	header, err := s.plans.FindHeaderByKey(ctx, req.CompanyID, req.VersionID, req.MovementTypeID, nil)
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

	documentNo, err := s.numbering.Next(ctx, req.CompanyID, model.NumberObjectPlan, req.DateFrom)
	if err != nil {
		return nil, err
	}

	header = &model.PlanHeader{
		CompanyID:         req.CompanyID,
		SeasonID:          season.ID,
		PlanningVersionID: version.ID,
		MovementTypeID:    req.MovementTypeID,
		DocumentNo:        documentNo,
		DateFrom:          req.DateFrom,
		DateTo:            req.DateTo,
	}
	if err := s.plans.CreateHeader(ctx, header); err != nil {
		return nil, err
	}
	return header, nil
}

func matrixKey(date time.Time, lineID *int64) string {
	key := date.Format("2006-01-02")
	if lineID == nil {
		return key + "|-"
	}
	return key + "|" + decimal.NewFromInt(*lineID).String()
}
