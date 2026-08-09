package service

import (
	"context"
	"strings"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/audit"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/config"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
)

// maskedValue replaces the content of a PII field in browser output.
const maskedValue = "********"

// BrowserRequest is a data browser selection as it arrives from the client.
type BrowserRequest struct {
	TableName string
	Fields    []string
	Filters   []interfaces.Filter
	Sort      []interfaces.SortSpec
	Page      int
	Size      int
	// CompanyIDs are the companies the caller is authorised for. The service —
	// not the client — decides whether they are applied.
	AuthorisedCompanyIDs []int64
	RequestedCompanyID   *int64
	Export               bool
}

// DataDictionaryService serves the metadata repository and the data browser.
//
// The browser is the highest-risk feature of the system, so every rule of §G2
// is enforced here rather than at the edge: the table must be whitelisted, all
// field names must exist in the dictionary, the company filter is injected
// server-side, PII is masked and every query is audited.
type DataDictionaryService struct {
	dd       interfaces.DataDictionaryRepository
	browser  interfaces.BrowserRepository
	auditSvc *audit.Service
	cfg      config.BusinessConfig
}

func NewDataDictionaryService(dd interfaces.DataDictionaryRepository,
	browser interfaces.BrowserRepository, auditSvc *audit.Service,
	cfg config.BusinessConfig) *DataDictionaryService {
	return &DataDictionaryService{dd: dd, browser: browser, auditSvc: auditSvc, cfg: cfg}
}

func (s *DataDictionaryService) ListTables(ctx context.Context, module, search string, browsableOnly bool) ([]model.DDTable, error) {
	return s.dd.ListTables(ctx, module, search, browsableOnly)
}

func (s *DataDictionaryService) GetTable(ctx context.Context, tableName string) (*model.DDTable, error) {
	return s.dd.FindTable(ctx, tableName)
}

func (s *DataDictionaryService) Fields(ctx context.Context, tableName string) ([]model.DDField, error) {
	table, err := s.dd.FindTable(ctx, tableName)
	if err != nil {
		return nil, err
	}
	return s.dd.Fields(ctx, table.ID)
}

func (s *DataDictionaryService) UpdateTable(ctx context.Context, table *model.DDTable) error {
	return s.dd.UpdateTable(ctx, table)
}

func (s *DataDictionaryService) GetField(ctx context.Context, id int64) (*model.DDField, error) {
	return s.dd.FindField(ctx, id)
}

func (s *DataDictionaryService) UpdateField(ctx context.Context, field *model.DDField) error {
	return s.dd.UpdateField(ctx, field)
}

func (s *DataDictionaryService) ListDomains(ctx context.Context) ([]model.DDDomain, error) {
	return s.dd.ListDomains(ctx)
}

// Sync regenerates the dictionary from the live catalogue. It runs on start-up
// so a newly migrated table is immediately documented and, per §G1, the CI
// check can fail a build whose columns have no dictionary entry.
func (s *DataDictionaryService) Sync(ctx context.Context) (int, error) {
	return s.dd.SyncFromInformationSchema(ctx)
}

// Browse executes a selection. The rules it enforces, in order:
//
//  1. the table must exist in the dictionary and be flagged browsable;
//  2. every requested, filtered and sorted field must exist on that table;
//  3. a company-dependent table is always restricted to authorised companies;
//  4. PII values are masked in the result;
//  5. the row cap and statement timeout bound the cost;
//  6. the query is written to the audit log.
func (s *DataDictionaryService) Browse(ctx context.Context, req BrowserRequest) (interfaces.BrowserResult, error) {
	var empty interfaces.BrowserResult

	table, err := s.dd.FindTable(ctx, req.TableName)
	if err != nil {
		if appErr, ok := apperrors.As(err); ok && appErr.Code == apperrors.ErrNotFound.Code {
			// The same answer for "unknown" and "not browsable": the browser
			// must not reveal which tables exist.
			return empty, apperrors.ErrTableNotBrowsable
		}
		return empty, err
	}
	if !table.IsBrowsable || !table.IsActive {
		return empty, apperrors.ErrTableNotBrowsable.WithDetails(
			apperrors.Detail{Field: "tableName", Value: req.TableName})
	}

	fields, err := s.dd.Fields(ctx, table.ID)
	if err != nil {
		return empty, err
	}

	byName := make(map[string]model.DDField, len(fields))
	readable := make([]string, 0, len(fields))
	for _, field := range fields {
		byName[field.FieldName] = field
		if field.IsBrowsable {
			readable = append(readable, field.FieldName)
		}
	}

	selected, err := s.resolveFields(req.Fields, byName, readable)
	if err != nil {
		return empty, err
	}
	if err := s.validateFieldNames(req.Filters, req.Sort, byName); err != nil {
		return empty, err
	}

	companyIDs, err := s.resolveCompanyScope(table, req)
	if err != nil {
		return empty, err
	}

	rowCap := s.cfg.BrowserRowCap
	if req.Export {
		rowCap = s.cfg.BrowserExportCap
	}
	size := req.Size
	if size <= 0 {
		size = interfaces.DefaultPageSize
	}
	if size > rowCap {
		size = rowCap
	}
	page := req.Page
	if page < 1 {
		page = 1
	}

	result, err := s.browser.Query(ctx, interfaces.BrowserQuery{
		TableName:  table.TableNameCol,
		Fields:     selected,
		Filters:    req.Filters,
		Sort:       req.Sort,
		Page:       page,
		Size:       size,
		CompanyIDs: companyIDs,
		RowCap:     rowCap,
		Timeout:    s.cfg.BrowserTimeout,
	})
	if err != nil {
		return empty, err
	}

	s.maskPII(result.Rows, byName)

	action := model.AuditBrowse
	if req.Export {
		action = model.AuditExport
	}
	s.auditSvc.Record(ctx, audit.Event{
		TableName: table.TableNameCol, Action: action,
		NewValues: map[string]any{
			"fields": selected, "filters": describeFilters(req.Filters),
			"rows": len(result.Rows), "total": result.Total, "companyIds": companyIDs,
		},
	})

	return result, nil
}

// resolveFields defaults to every readable field and rejects anything the
// dictionary does not know.
func (s *DataDictionaryService) resolveFields(requested []string,
	byName map[string]model.DDField, readable []string) ([]string, error) {

	if len(requested) == 0 {
		return readable, nil
	}
	selected := make([]string, 0, len(requested))
	for _, name := range requested {
		name = strings.TrimSpace(name)
		field, ok := byName[name]
		if !ok || !field.IsBrowsable {
			return nil, apperrors.ErrUnknownField.WithDetails(
				apperrors.Detail{Field: "fields", Value: name})
		}
		selected = append(selected, field.FieldName)
	}
	return selected, nil
}

func (s *DataDictionaryService) validateFieldNames(filters []interfaces.Filter,
	sort []interfaces.SortSpec, byName map[string]model.DDField) error {

	for _, filter := range filters {
		if field, ok := byName[filter.Field]; !ok || !field.IsBrowsable {
			return apperrors.ErrUnknownField.WithDetails(
				apperrors.Detail{Field: "filter", Value: filter.Field})
		}
	}
	for _, spec := range sort {
		if field, ok := byName[spec.Field]; !ok || !field.IsBrowsable {
			return apperrors.ErrUnknownField.WithDetails(
				apperrors.Detail{Field: "sort", Value: spec.Field})
		}
	}
	return nil
}

// resolveCompanyScope injects the company predicate for a company-dependent
// table. A caller may narrow the scope to one of its authorised companies but
// can never widen it, and cannot remove it.
func (s *DataDictionaryService) resolveCompanyScope(table *model.DDTable, req BrowserRequest) ([]int64, error) {
	if !table.IsCompanyDependent {
		return nil, nil
	}
	if len(req.AuthorisedCompanyIDs) == 0 {
		return nil, apperrors.ErrCompanyNotAuthorized
	}
	if req.RequestedCompanyID == nil {
		return req.AuthorisedCompanyIDs, nil
	}
	for _, id := range req.AuthorisedCompanyIDs {
		if id == *req.RequestedCompanyID {
			return []int64{id}, nil
		}
	}
	return nil, apperrors.ErrCompanyNotAuthorized
}

func (s *DataDictionaryService) maskPII(rows []map[string]any, byName map[string]model.DDField) {
	for _, row := range rows {
		for name, value := range row {
			if value == nil {
				continue
			}
			if field, ok := byName[name]; ok && field.IsPII {
				row[name] = maskedValue
			}
		}
	}
}

// describeFilters renders the selection for the audit record without copying
// raw values, which may themselves be sensitive.
func describeFilters(filters []interfaces.Filter) []string {
	out := make([]string, 0, len(filters))
	for _, f := range filters {
		out = append(out, f.Field+" "+string(f.Operator))
	}
	return out
}
