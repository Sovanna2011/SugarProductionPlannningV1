package service

import (
	"context"
	"time"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/audit"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/database"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
)

// ActualDocument is a header together with its items.
type ActualDocument struct {
	Header model.ActualHeader
	Items  []model.ActualItem
}

// ActualService records what was really produced. §32 makes this deliberately
// independent of planning: an actual document has no reference to a planning
// version and can be posted whether or not a plan exists.
type ActualService struct {
	actuals   interfaces.ActualRepository
	inventory *InventoryService
	numbering *NumberRangeService
	refs      *ReferenceValidator
	seasons   interfaces.SeasonRepository
	auditSvc  *audit.Service
	uow       database.UnitOfWork
}

func NewActualService(
	actuals interfaces.ActualRepository,
	inventory *InventoryService,
	numbering *NumberRangeService,
	refs *ReferenceValidator,
	seasons interfaces.SeasonRepository,
	auditSvc *audit.Service,
	uow database.UnitOfWork,
) *ActualService {
	return &ActualService{
		actuals: actuals, inventory: inventory, numbering: numbering,
		refs: refs, seasons: seasons, auditSvc: auditSvc, uow: uow,
	}
}

func (s *ActualService) List(ctx context.Context, opts interfaces.ListOptions,
	from, to *time.Time, movementTypeID, lineID *int64) (interfaces.Page[model.ActualHeader], error) {
	return s.actuals.ListHeaders(ctx, opts, from, to, movementTypeID, lineID)
}

func (s *ActualService) Get(ctx context.Context, companyID, headerID int64) (*ActualDocument, error) {
	header, err := s.actuals.FindHeader(ctx, companyID, headerID)
	if err != nil {
		return nil, err
	}
	items, err := s.actuals.Items(ctx, headerID)
	if err != nil {
		return nil, err
	}
	return &ActualDocument{Header: *header, Items: items}, nil
}

// Create records a draft document. An Idempotency-Key makes a retried POST
// return the document the first attempt created instead of a duplicate (§E8).
func (s *ActualService) Create(ctx context.Context, header *model.ActualHeader,
	items []model.ActualItem, idempotencyKey string) (*ActualDocument, error) {

	if idempotencyKey != "" {
		existing, err := s.actuals.FindByIdempotencyKey(ctx, header.CompanyID, idempotencyKey)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return s.Get(ctx, header.CompanyID, existing.ID)
		}
		header.IdempotencyKey = &idempotencyKey
	}

	var document *ActualDocument

	err := s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.refs.ProductionLine(ctx, header.CompanyID, header.ProductionLineID); err != nil {
			return err
		}

		// The season is resolved from the posting date when the caller did not
		// name one, so reports can group actuals by season without the user
		// having to know which season a date belongs to.
		if header.SeasonID == nil {
			season, err := s.seasons.FindContaining(ctx, header.CompanyID, header.PostingDate)
			if err != nil {
				return err
			}
			if season != nil {
				header.SeasonID = &season.ID
			}
		} else if _, err := s.refs.Season(ctx, header.CompanyID, *header.SeasonID); err != nil {
			return err
		}

		documentNo, err := s.numbering.Next(ctx, header.CompanyID, model.NumberObjectActual, header.PostingDate)
		if err != nil {
			return err
		}
		header.DocumentNo = documentNo
		header.PostingStatus = model.PostingStatusDraft

		if err := s.actuals.CreateHeader(ctx, header); err != nil {
			return err
		}
		if err := s.validateItems(ctx, header, items); err != nil {
			return err
		}
		if err := s.actuals.ReplaceItems(ctx, header.ID, items); err != nil {
			return err
		}

		s.auditSvc.Record(ctx, audit.Event{
			TableName: "actual_headers", RecordID: &header.ID, CompanyID: &header.CompanyID,
			Action: model.AuditInsert, NewValues: header,
		})

		document = &ActualDocument{Header: *header, Items: items}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return document, nil
}

// Update changes a draft. A posted or reversed document is immutable — it is
// corrected by a reversal, never by an edit.
func (s *ActualService) Update(ctx context.Context, header *model.ActualHeader, items []model.ActualItem) (*ActualDocument, error) {
	var document *ActualDocument

	err := s.uow.Do(ctx, func(ctx context.Context) error {
		current, err := s.actuals.FindHeader(ctx, header.CompanyID, header.ID)
		if err != nil {
			return err
		}
		if current.PostingStatus != model.PostingStatusDraft {
			return apperrors.ErrDocumentNotDraft.Msgf(
				"document %s is %s and can no longer be changed", current.DocumentNo, current.PostingStatus)
		}

		if err := s.refs.ProductionLine(ctx, header.CompanyID, header.ProductionLineID); err != nil {
			return err
		}

		current.Description = header.Description
		current.PostingDate = header.PostingDate
		current.MovementTypeID = header.MovementTypeID
		current.ProductionLineID = header.ProductionLineID
		current.Version = header.Version

		if err := s.actuals.UpdateHeader(ctx, current.CompanyID, current); err != nil {
			return err
		}
		if items != nil {
			if err := s.validateItems(ctx, current, items); err != nil {
				return err
			}
			if err := s.actuals.ReplaceItems(ctx, current.ID, items); err != nil {
				return err
			}
		}

		stored, err := s.actuals.Items(ctx, current.ID)
		if err != nil {
			return err
		}
		document = &ActualDocument{Header: *current, Items: stored}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return document, nil
}

// Post turns a draft into a posted document and writes the corresponding
// inventory movements in the same transaction (§D3, §32). Every §F4 and §F7
// validation runs as part of that write, so a document that would break stock
// or the production routing is rejected before anything is committed.
func (s *ActualService) Post(ctx context.Context, companyID, headerID, actorID int64) (*ActualDocument, error) {
	var document *ActualDocument

	err := s.uow.Do(ctx, func(ctx context.Context) error {
		header, err := s.actuals.FindHeader(ctx, companyID, headerID)
		if err != nil {
			return err
		}
		if header.PostingStatus != model.PostingStatusDraft {
			return apperrors.ErrNotPostable.Msgf(
				"document %s is %s", header.DocumentNo, header.PostingStatus)
		}

		items, err := s.actuals.Items(ctx, headerID)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return apperrors.ErrDocumentEmpty
		}

		requests := make([]MovementRequest, 0, len(items))
		for _, item := range items {
			if item.WarehouseID == nil {
				// A production quantity that is not stock managed — electricity,
				// for instance — is recorded on the document but never becomes
				// warehouse stock (OQ-8).
				continue
			}
			requests = append(requests, MovementRequest{
				CompanyID: companyID,
				// The movements of a posted document carry the document's own
				// number, which is what makes the ledger traceable back to it.
				DocumentNo:        header.DocumentNo,
				TransactionDate:   item.ActualDate,
				MaterialID:        item.MaterialID,
				WarehouseID:       *item.WarehouseID,
				MovementTypeID:    header.MovementTypeID,
				PackagingTypeID:   item.PackagingTypeID,
				ProcessID:         item.ProcessID,
				Quantity:          item.Quantity,
				UOMID:             item.UOMID,
				SourceModule:      model.SourceModuleActual,
				ReferenceDocument: &header.DocumentNo,
				ReferenceID:       &header.ID,
				Remark:            item.Remark,
			})
		}

		if _, err := s.inventory.Apply(ctx, requests); err != nil {
			return err
		}

		now := time.Now().UTC()
		header.PostingStatus = model.PostingStatusPosted
		header.PostedBy = &actorID
		header.PostedAt = &now
		if err := s.actuals.SetStatus(ctx, header); err != nil {
			return err
		}

		s.auditSvc.Record(ctx, audit.Event{
			TableName: "actual_headers", RecordID: &header.ID, CompanyID: &companyID,
			Action:    model.AuditPost,
			NewValues: map[string]any{"documentNo": header.DocumentNo, "movements": len(requests)},
		})

		document = &ActualDocument{Header: *header, Items: items}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return document, nil
}

// Reverse cancels a posted document: every movement it generated gets an
// opposite entry and the document is flagged REVERSED. Nothing is deleted.
func (s *ActualService) Reverse(ctx context.Context, companyID, headerID, actorID int64, remark *string) (*ActualDocument, error) {
	var document *ActualDocument

	err := s.uow.Do(ctx, func(ctx context.Context) error {
		header, err := s.actuals.FindHeader(ctx, companyID, headerID)
		if err != nil {
			return err
		}
		if header.PostingStatus != model.PostingStatusPosted {
			return apperrors.ErrNotReversible.Msgf(
				"document %s is %s", header.DocumentNo, header.PostingStatus)
		}

		movements, err := s.inventory.ListMovements(ctx,
			interfaces.ListOptions{CompanyID: &companyID, Size: interfaces.MaxPageSize},
			interfaces.MovementFilter{DocumentNo: header.DocumentNo})
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
		header.PostingStatus = model.PostingStatusReversed
		header.ReversedBy = &actorID
		header.ReversedAt = &now
		if err := s.actuals.SetStatus(ctx, header); err != nil {
			return err
		}

		s.auditSvc.Record(ctx, audit.Event{
			TableName: "actual_headers", RecordID: &header.ID, CompanyID: &companyID,
			Action:    model.AuditReverse,
			NewValues: map[string]any{"documentNo": header.DocumentNo, "reversedMovements": len(movements.Rows)},
		})

		items, err := s.actuals.Items(ctx, headerID)
		if err != nil {
			return err
		}
		document = &ActualDocument{Header: *header, Items: items}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return document, nil
}

// validateItems checks what can be checked before posting, so a draft cannot
// be saved with references that would certainly fail later.
func (s *ActualService) validateItems(ctx context.Context, header *model.ActualHeader, items []model.ActualItem) error {
	if len(items) == 0 {
		return nil // a draft may legitimately be empty; posting rejects it
	}
	for i, item := range items {
		if item.Quantity.IsNegative() {
			return apperrors.ErrValidation.Msgf("an actual quantity must not be negative").
				WithDetails(apperrors.Detail{Field: "items.quantity"})
		}
		if _, _, err := s.refs.MaterialForCompany(ctx, header.CompanyID, item.MaterialID); err != nil {
			return err
		}
		if err := s.refs.ProductionLine(ctx, header.CompanyID, item.ProductionLineID); err != nil {
			return err
		}
		if item.WarehouseID != nil {
			if _, err := s.refs.Warehouse(ctx, header.CompanyID, *item.WarehouseID); err != nil {
				return err
			}
		}
		items[i].ActualHeaderID = header.ID
	}
	return nil
}
