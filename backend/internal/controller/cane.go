package controller

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/dto"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/mapper"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/service"
)

// CaneController serves the cane supply module: growers and fields, the
// harvest plan, weighbridge tickets and the cane plan vs actual report.
type CaneController struct {
	cane *service.CaneService
}

func NewCaneController(cane *service.CaneService) *CaneController {
	return &CaneController{cane: cane}
}

// --- varieties -----------------------------------------------------------

func (ctl *CaneController) ListVarieties(c *gin.Context) {
	opts, ok := ListOptions(c, nil)
	if !ok {
		return
	}
	page, err := ctl.cane.ListVarieties(c.Request.Context(), opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.CaneVarietyToDTO), opts, page.Total)
}

func (ctl *CaneController) GetVariety(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	variety, err := ctl.cane.GetVariety(c.Request.Context(), id)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.CaneVarietyToDTO(*variety))
}

func (ctl *CaneController) CreateVariety(c *gin.Context) {
	req, ok := Bind[dto.CaneVarietyRequest](c)
	if !ok {
		return
	}
	variety := mapper.CaneVarietyFromDTO(req, nil)
	if err := ctl.cane.CreateVariety(c.Request.Context(), variety); err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.CaneVarietyToDTO(*variety))
}

func (ctl *CaneController) UpdateVariety(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	req, ok := Bind[dto.CaneVarietyRequest](c)
	if !ok {
		return
	}
	variety := mapper.CaneVarietyFromDTO(req, &model.CaneVariety{})
	variety.ID = id
	if err := ctl.cane.UpdateVariety(c.Request.Context(), variety); err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.CaneVarietyToDTO(*variety))
}

func (ctl *CaneController) DeleteVariety(c *gin.Context) {
	id, version, ok := idAndVersion(c, "id")
	if !ok {
		return
	}
	if err := ctl.cane.DeactivateVariety(c.Request.Context(), id, version); err != nil {
		Fail(c, err)
		return
	}
	NoContent(c)
}

// --- growers -------------------------------------------------------------

func (ctl *CaneController) ListGrowers(c *gin.Context) {
	companyID := CompanyID(c)
	opts, ok := ListOptions(c, &companyID)
	if !ok {
		return
	}
	page, err := ctl.cane.ListGrowers(c.Request.Context(), opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.GrowerToDTO), opts, page.Total)
}

func (ctl *CaneController) GetGrower(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	grower, err := ctl.cane.GetGrower(c.Request.Context(), CompanyID(c), id)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.GrowerToDTO(*grower))
}

func (ctl *CaneController) CreateGrower(c *gin.Context) {
	req, ok := Bind[dto.GrowerRequest](c)
	if !ok {
		return
	}
	grower := mapper.GrowerFromDTO(req, CompanyID(c), nil)
	if err := ctl.cane.CreateGrower(c.Request.Context(), grower); err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.GrowerToDTO(*grower))
}

func (ctl *CaneController) UpdateGrower(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	req, ok := Bind[dto.GrowerRequest](c)
	if !ok {
		return
	}
	grower := mapper.GrowerFromDTO(req, CompanyID(c), &model.Grower{})
	grower.ID = id
	if err := ctl.cane.UpdateGrower(c.Request.Context(), grower); err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.GrowerToDTO(*grower))
}

func (ctl *CaneController) DeleteGrower(c *gin.Context) {
	id, version, ok := idAndVersion(c, "id")
	if !ok {
		return
	}
	if err := ctl.cane.DeactivateGrower(c.Request.Context(), CompanyID(c), id, version); err != nil {
		Fail(c, err)
		return
	}
	NoContent(c)
}

// --- cane fields ---------------------------------------------------------

func (ctl *CaneController) ListFields(c *gin.Context) {
	companyID := CompanyID(c)
	opts, ok := ListOptions(c, &companyID)
	if !ok {
		return
	}
	// ?growerId= narrows the list to one grower's plots, which is what the
	// harvest matrix's value help asks for.
	growerID, ok := QueryID(c, "growerId")
	if !ok {
		return
	}
	if growerID != nil {
		fields, err := ctl.cane.FieldsForGrower(c.Request.Context(), companyID, *growerID)
		if err != nil {
			Fail(c, err)
			return
		}
		Paged(c, mapper.Slice(fields, mapper.CaneFieldToDTO), opts, int64(len(fields)))
		return
	}

	page, err := ctl.cane.ListFields(c.Request.Context(), opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.CaneFieldToDTO), opts, page.Total)
}

func (ctl *CaneController) GetField(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	field, err := ctl.cane.GetField(c.Request.Context(), CompanyID(c), id)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.CaneFieldToDTO(*field))
}

func (ctl *CaneController) CreateField(c *gin.Context) {
	req, ok := Bind[dto.CaneFieldRequest](c)
	if !ok {
		return
	}
	field := mapper.CaneFieldFromDTO(req, CompanyID(c), nil)
	if err := ctl.cane.CreateField(c.Request.Context(), field); err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.CaneFieldToDTO(*field))
}

func (ctl *CaneController) UpdateField(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	req, ok := Bind[dto.CaneFieldRequest](c)
	if !ok {
		return
	}
	field := mapper.CaneFieldFromDTO(req, CompanyID(c), &model.CaneField{})
	field.ID = id
	if err := ctl.cane.UpdateField(c.Request.Context(), field); err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.CaneFieldToDTO(*field))
}

func (ctl *CaneController) DeleteField(c *gin.Context) {
	id, version, ok := idAndVersion(c, "id")
	if !ok {
		return
	}
	if err := ctl.cane.DeactivateField(c.Request.Context(), CompanyID(c), id, version); err != nil {
		Fail(c, err)
		return
	}
	NoContent(c)
}

// --- the harvest matrix --------------------------------------------------

// GetHarvestMatrix returns the Date × Grower grid the cane planning screen
// renders.
func (ctl *CaneController) GetHarvestMatrix(c *gin.Context) {
	versionID, ok := QueryID(c, "versionId")
	if !ok {
		return
	}
	if versionID == nil {
		Fail(c, apperrors.ErrBadRequest.
			Msgf("versionId is required to read a harvest matrix").
			WithDetails(apperrors.Detail{Field: "versionId"}))
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

	matrix, err := ctl.cane.GetHarvestMatrix(c.Request.Context(), CompanyID(c), *versionID, from, to)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.HarvestMatrixToDTO(matrix))
}

// SaveHarvestMatrix is the matrix-shaped bulk save. Replaying the same payload
// produces the same grid rather than duplicate rows.
func (ctl *CaneController) SaveHarvestMatrix(c *gin.Context) {
	req, ok := Bind[dto.HarvestMatrixSaveRequest](c)
	if !ok {
		return
	}
	matrix, err := ctl.cane.SaveHarvestMatrix(c.Request.Context(),
		mapper.HarvestMatrixRequestFromDTO(req, CompanyID(c)), UserID(c))
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.HarvestMatrixToDTO(matrix))
}

func (ctl *CaneController) ListHarvestPlans(c *gin.Context) {
	companyID := CompanyID(c)
	opts, ok := ListOptions(c, &companyID)
	if !ok {
		return
	}
	versionID, ok := QueryID(c, "versionId")
	if !ok {
		return
	}
	page, err := ctl.cane.HarvestPlans(c.Request.Context(), opts, versionID)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.HarvestPlanHeaderToDTO), opts, page.Total)
}

func (ctl *CaneController) HarvestPlanItems(c *gin.Context) {
	headerID, ok := PathID(c, "headerId")
	if !ok {
		return
	}
	items, err := ctl.cane.HarvestPlanItems(c.Request.Context(), CompanyID(c), headerID)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.Slice(items, mapper.HarvestPlanItemToDTO))
}

// --- deliveries ----------------------------------------------------------

func (ctl *CaneController) ListDeliveries(c *gin.Context) {
	companyID := CompanyID(c)
	opts, ok := ListOptions(c, &companyID)
	if !ok {
		return
	}

	growerID, ok := QueryID(c, "growerId")
	if !ok {
		return
	}
	fieldID, ok := QueryID(c, "caneFieldId")
	if !ok {
		return
	}
	varietyID, ok := QueryID(c, "varietyId")
	if !ok {
		return
	}
	from, ok := QueryDate(c, "dateFrom")
	if !ok {
		return
	}
	to, ok := QueryDate(c, "dateTo")
	if !ok {
		return
	}

	filter := interfaces.DeliveryFilter{
		GrowerID: growerID, CaneFieldID: fieldID, VarietyID: varietyID,
		Status: strings.ToUpper(c.Query("status")), From: from, To: to,
	}
	page, err := ctl.cane.ListDeliveries(c.Request.Context(), opts, filter)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.CaneDeliveryToDTO), opts, page.Total)
}

func (ctl *CaneController) GetDelivery(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	delivery, err := ctl.cane.GetDelivery(c.Request.Context(), CompanyID(c), id)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.CaneDeliveryToDTO(*delivery))
}

func (ctl *CaneController) CreateDelivery(c *gin.Context) {
	req, ok := Bind[dto.CaneDeliveryRequest](c)
	if !ok {
		return
	}
	delivery := mapper.CaneDeliveryFromDTO(req, CompanyID(c), nil)
	stored, err := ctl.cane.CreateDelivery(c.Request.Context(), delivery,
		c.GetHeader("Idempotency-Key"))
	if err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.CaneDeliveryToDTO(*stored))
}

func (ctl *CaneController) UpdateDelivery(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	req, ok := Bind[dto.CaneDeliveryRequest](c)
	if !ok {
		return
	}
	delivery := mapper.CaneDeliveryFromDTO(req, CompanyID(c), &model.CaneDelivery{})
	delivery.ID = id
	stored, err := ctl.cane.UpdateDelivery(c.Request.Context(), delivery)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.CaneDeliveryToDTO(*stored))
}

func (ctl *CaneController) PostDelivery(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	delivery, err := ctl.cane.PostDelivery(c.Request.Context(), CompanyID(c), id, UserID(c))
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.CaneDeliveryToDTO(*delivery))
}

func (ctl *CaneController) ReverseDelivery(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	var body struct {
		Remark *string `json:"remark"`
	}
	_ = c.ShouldBindJSON(&body) // a reversal reason is welcome but not required

	delivery, err := ctl.cane.ReverseDelivery(c.Request.Context(), CompanyID(c), id, UserID(c), body.Remark)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.CaneDeliveryToDTO(*delivery))
}

// --- reporting -----------------------------------------------------------

func (ctl *CaneController) CanePlanVsActual(c *gin.Context) {
	from, ok := RequiredQueryDate(c, "dateFrom")
	if !ok {
		return
	}
	to, ok := RequiredQueryDate(c, "dateTo")
	if !ok {
		return
	}
	versionID, ok := QueryID(c, "versionId")
	if !ok {
		return
	}

	groupBy := c.DefaultQuery("groupBy", "grower")
	rows, err := ctl.cane.CanePlanVsActual(c.Request.Context(), interfaces.CanePlanVsActualQuery{
		CompanyID:  CompanyID(c),
		VersionID:  versionID,
		Range:      interfaces.DateRange{From: from, To: to},
		GroupBy:    groupBy,
		SupplyType: strings.ToUpper(c.Query("supplyType")),
	})
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.Slice(rows, mapper.CanePlanVsActualToDTO))
}

// idAndVersion reads the path id together with the optimistic-lock version the
// API takes as ?version= on a delete (§D2).
func idAndVersion(c *gin.Context, name string) (int64, int, bool) {
	id, ok := PathID(c, name)
	if !ok {
		return 0, 0, false
	}
	version, ok := QueryID(c, "version")
	if !ok {
		return 0, 0, false
	}
	if version == nil {
		Fail(c, versionRequired())
		return 0, 0, false
	}
	return id, int(*version), true
}
