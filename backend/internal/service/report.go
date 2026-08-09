package service

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
)

// PlanVsActualLine is one reported row with its variance already computed.
type PlanVsActualLine struct {
	CompanyID   int64
	CompanyCode string
	GroupKey    string
	GroupLabel  string
	UOMCode     string
	PlanQty     decimal.Decimal
	ActualQty   decimal.Decimal
	Variance    decimal.Decimal
	// VariancePct is nil where a percentage has no meaning — an actual against
	// a zero plan. It is never rendered as 0 or 100 by accident (§F6).
	VariancePct       *decimal.Decimal
	ConversionMissing bool
}

type PlanVsActualReport struct {
	GroupBy     string
	DateFrom    time.Time
	DateTo      time.Time
	VersionID   *int64
	Lines       []PlanVsActualLine
	TotalPlan   decimal.Decimal
	TotalActual decimal.Decimal
}

var allowedGroupBy = map[string]bool{
	"material": true, "line": true, "process": true, "date": true,
}

type ReportService struct {
	reports  interfaces.ReportRepository
	versions interfaces.PlanningVersionRepository
}

func NewReportService(reports interfaces.ReportRepository,
	versions interfaces.PlanningVersionRepository) *ReportService {
	return &ReportService{reports: reports, versions: versions}
}

// PlanVsActual implements §36 / §F6. When no version is named it falls back to
// the latest approved version of the season, which is what the business means
// by "the plan".
func (s *ReportService) PlanVsActual(ctx context.Context, companyIDs []int64,
	seasonID, versionID *int64, from, to time.Time, groupBy string, latestApproved bool) (*PlanVsActualReport, error) {

	if groupBy == "" {
		groupBy = "material"
	}
	if !allowedGroupBy[groupBy] {
		return nil, apperrors.ErrValidation.
			Msgf("groupBy must be one of material, line, process or date").
			WithDetails(apperrors.Detail{Field: "groupBy", Value: groupBy})
	}
	if to.Before(from) {
		return nil, apperrors.ErrInvalidDateRange
	}

	// For a single company the caller may ask for "the latest approved
	// version" instead of naming one. Across companies the resolution happens
	// per company inside the query, because each has its own version numbering.
	if versionID == nil && latestApproved && seasonID != nil && len(companyIDs) == 1 {
		version, err := s.versions.LatestApproved(ctx, companyIDs[0], *seasonID)
		if err != nil {
			return nil, err
		}
		versionID = &version.ID
	}

	rows, err := s.reports.PlanVsActual(ctx, interfaces.PlanVsActualQuery{
		CompanyIDs: companyIDs,
		SeasonID:   seasonID,
		VersionID:  versionID,
		Range:      interfaces.DateRange{From: from, To: to},
		GroupBy:    groupBy,
	})
	if err != nil {
		return nil, err
	}

	report := &PlanVsActualReport{
		GroupBy: groupBy, DateFrom: from, DateTo: to, VersionID: versionID,
		Lines: make([]PlanVsActualLine, 0, len(rows)),
	}

	for _, row := range rows {
		line := PlanVsActualLine{
			CompanyID:         row.CompanyID,
			CompanyCode:       row.CompanyCode,
			GroupKey:          row.GroupKey,
			GroupLabel:        row.GroupLabel,
			UOMCode:           row.UOMCode,
			PlanQty:           row.PlanQty,
			ActualQty:         row.ActualQty,
			Variance:          row.ActualQty.Sub(row.PlanQty),
			VariancePct:       VariancePercent(row.PlanQty, row.ActualQty),
			ConversionMissing: row.ConversionMissing,
		}
		report.Lines = append(report.Lines, line)
		report.TotalPlan = report.TotalPlan.Add(row.PlanQty)
		report.TotalActual = report.TotalActual.Add(row.ActualQty)
	}

	return report, nil
}

// VariancePercent implements the §F6 rule exactly:
//
//	plan ≠ 0  → (actual − plan) / |plan| × 100
//	plan = 0 and actual = 0 → 0
//	plan = 0 and actual ≠ 0 → nil, rendered as "n/a" or "new"
//
// abs(plan) in the denominator keeps the sign of the variance meaningful even
// if a planned value is ever negative.
func VariancePercent(plan, actual decimal.Decimal) *decimal.Decimal {
	if !plan.IsZero() {
		pct := actual.Sub(plan).Div(plan.Abs()).Mul(decimal.NewFromInt(100)).Round(2)
		return &pct
	}
	if actual.IsZero() {
		zero := decimal.Zero
		return &zero
	}
	return nil
}

func (s *ReportService) ProductionSummary(ctx context.Context, companyID int64,
	from, to time.Time) ([]interfaces.ProductionSummaryRow, error) {
	if to.Before(from) {
		return nil, apperrors.ErrInvalidDateRange
	}
	return s.reports.ProductionSummary(ctx, companyID, interfaces.DateRange{From: from, To: to})
}

func (s *ReportService) InventoryMovements(ctx context.Context, companyID int64,
	from, to time.Time, warehouseID, materialID *int64) ([]interfaces.MovementReportRow, error) {
	if to.Before(from) {
		return nil, apperrors.ErrInvalidDateRange
	}
	return s.reports.InventoryMovementReport(ctx, companyID,
		interfaces.DateRange{From: from, To: to}, warehouseID, materialID)
}

func (s *ReportService) CapacityUtilisation(ctx context.Context, companyID int64,
	asOf time.Time) ([]interfaces.CapacityRow, error) {
	return s.reports.CapacityUtilisation(ctx, companyID, asOf)
}
