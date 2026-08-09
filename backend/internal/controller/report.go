package controller

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/config"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/dto"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/mapper"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/service"
)

type ReportController struct {
	reports *service.ReportService
	authz   *service.AuthorizationService
	cfg     config.BusinessConfig
}

func NewReportController(reports *service.ReportService, authz *service.AuthorizationService,
	cfg config.BusinessConfig) *ReportController {
	return &ReportController{reports: reports, authz: authz, cfg: cfg}
}

// PlanVsActual serves the single-company report. The company has already been
// validated by the middleware.
func (ctl *ReportController) PlanVsActual(c *gin.Context) {
	companyID := CompanyID(c)
	ctl.planVsActual(c, []int64{companyID})
}

// PlanVsActualConsolidated serves the multi-company report of §1. Every named
// company is checked individually; an unauthorised id fails the whole request
// rather than being filtered out silently, so the audit trail is unambiguous
// (§E6 / OQ-6).
func (ctl *ReportController) PlanVsActualConsolidated(c *gin.Context) {
	companyIDs, err := ParseIDList(c.Query("companyIds"))
	if err != nil {
		Fail(c, err)
		return
	}
	err = ctl.authz.EnsureCompaniesAccess(c.Request.Context(), UserID(c), companyIDs,
		service.PermReportPlanActual)
	if err != nil {
		Fail(c, err)
		return
	}
	ctl.planVsActual(c, companyIDs)
}

func (ctl *ReportController) planVsActual(c *gin.Context, companyIDs []int64) {
	seasonID, ok := QueryID(c, "seasonId")
	if !ok {
		return
	}
	versionID, ok := QueryID(c, "versionId")
	if !ok {
		return
	}
	from, ok := RequiredQueryDate(c, "dateFrom")
	if !ok {
		return
	}
	to, ok := RequiredQueryDate(c, "dateTo")
	if !ok {
		return
	}

	report, err := ctl.reports.PlanVsActual(c.Request.Context(), companyIDs, seasonID,
		versionID, from, to, c.Query("groupBy"), c.Query("latestApproved") == "true")
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.PlanVsActualToDTO(report))
}

func (ctl *ReportController) ProductionSummary(c *gin.Context) {
	from, ok := RequiredQueryDate(c, "dateFrom")
	if !ok {
		return
	}
	to, ok := RequiredQueryDate(c, "dateTo")
	if !ok {
		return
	}
	rows, err := ctl.reports.ProductionSummary(c.Request.Context(), CompanyID(c), from, to)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.Slice(rows, mapper.ProductionSummaryToDTO))
}

func (ctl *ReportController) InventoryMovements(c *gin.Context) {
	from, ok := RequiredQueryDate(c, "dateFrom")
	if !ok {
		return
	}
	to, ok := RequiredQueryDate(c, "dateTo")
	if !ok {
		return
	}
	warehouseID, ok := QueryID(c, "warehouseId")
	if !ok {
		return
	}
	materialID, ok := QueryID(c, "materialId")
	if !ok {
		return
	}

	rows, err := ctl.reports.InventoryMovements(c.Request.Context(), CompanyID(c),
		from, to, warehouseID, materialID)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.Slice(rows, mapper.MovementReportToDTO))
}

func (ctl *ReportController) CapacityUtilisation(c *gin.Context) {
	asOf := time.Now().UTC().Truncate(24 * time.Hour)
	if raw := c.Query("asOfDate"); raw != "" {
		parsed, err := dto.ParseDate(raw)
		if err != nil {
			Fail(c, err)
			return
		}
		asOf = parsed
	}

	rows, err := ctl.reports.CapacityUtilisation(c.Request.Context(), CompanyID(c), asOf)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.Slice(rows, func(row interfaces.CapacityRow) dto.CapacityResponse {
		return mapper.CapacityRowToDTO(row, ctl.cfg.CapacityAmberPct, ctl.cfg.CapacityRedPct)
	}))
}
