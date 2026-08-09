package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/dto"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/mapper"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/service"
)

type PlanningController struct {
	planning *service.PlanningService
}

func NewPlanningController(planning *service.PlanningService) *PlanningController {
	return &PlanningController{planning: planning}
}

// --- versions ------------------------------------------------------------

func (ctl *PlanningController) ListVersions(c *gin.Context) {
	companyID := CompanyID(c)
	seasonID, ok := PathID(c, "seasonId")
	if !ok {
		return
	}
	versions, err := ctl.planning.VersionsForSeason(c.Request.Context(), companyID, seasonID)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.Slice(versions, mapper.PlanningVersionToDTO))
}

func (ctl *PlanningController) CreateVersion(c *gin.Context) {
	seasonID, ok := PathID(c, "seasonId")
	if !ok {
		return
	}
	req, ok := Bind[dto.PlanningVersionRequest](c)
	if !ok {
		return
	}
	req.SeasonID = seasonID

	version := mapper.PlanningVersionFromDTO(req, CompanyID(c), nil)
	if err := ctl.planning.CreateVersion(c.Request.Context(), version); err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.PlanningVersionToDTO(*version))
}

func (ctl *PlanningController) GetVersion(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	version, err := ctl.planning.GetVersion(c.Request.Context(), CompanyID(c), id)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.PlanningVersionToDTO(*version))
}

func (ctl *PlanningController) UpdateVersion(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	req, ok := Bind[dto.PlanningVersionRequest](c)
	if !ok {
		return
	}
	version := mapper.PlanningVersionFromDTO(req, CompanyID(c), &model.PlanningVersion{})
	version.ID = id
	if err := ctl.planning.UpdateVersion(c.Request.Context(), version); err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.PlanningVersionToDTO(*version))
}

// Copy produces a deep, independent snapshot of a version (§31, §F2).
func (ctl *PlanningController) Copy(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	req, ok := Bind[dto.CopyVersionRequest](c)
	if !ok {
		return
	}
	created, err := ctl.planning.CopyVersion(c.Request.Context(), CompanyID(c), id,
		req.NewVersionNo, req.VersionName, req.Description)
	if err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.PlanningVersionToDTO(*created))
}

// transition is shared by submit / approve / lock / cancel — the state machine
// itself lives in the service, so each route only names its target status.
func (ctl *PlanningController) transition(target string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := PathID(c, "id")
		if !ok {
			return
		}
		version, err := ctl.planning.Transition(c.Request.Context(), CompanyID(c), id, target, UserID(c))
		if err != nil {
			Fail(c, err)
			return
		}
		OK(c, mapper.PlanningVersionToDTO(*version))
	}
}

func (ctl *PlanningController) Submit() gin.HandlerFunc {
	return ctl.transition(model.VersionStatusSubmitted)
}
func (ctl *PlanningController) Approve() gin.HandlerFunc {
	return ctl.transition(model.VersionStatusApproved)
}
func (ctl *PlanningController) Lock() gin.HandlerFunc {
	return ctl.transition(model.VersionStatusLocked)
}
func (ctl *PlanningController) Cancel() gin.HandlerFunc {
	return ctl.transition(model.VersionStatusCancelled)
}

// --- plan documents ------------------------------------------------------

func (ctl *PlanningController) ListPlans(c *gin.Context) {
	companyID := CompanyID(c)
	opts, ok := ListOptions(c, &companyID)
	if !ok {
		return
	}
	versionID, ok := QueryID(c, "versionId")
	if !ok {
		return
	}
	movementTypeID, ok := QueryID(c, "movementTypeId")
	if !ok {
		return
	}

	page, err := ctl.planning.ListPlans(c.Request.Context(), opts, versionID, movementTypeID)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.PlanHeaderToDTO), opts, page.Total)
}

func (ctl *PlanningController) GetPlan(c *gin.Context) {
	id, ok := PathID(c, "headerId")
	if !ok {
		return
	}
	header, err := ctl.planning.GetPlan(c.Request.Context(), CompanyID(c), id)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.PlanHeaderToDTO(*header))
}

func (ctl *PlanningController) PlanItems(c *gin.Context) {
	id, ok := PathID(c, "headerId")
	if !ok {
		return
	}
	items, err := ctl.planning.PlanItems(c.Request.Context(), CompanyID(c), id)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.Slice(items, mapper.PlanItemToDTO))
}

// --- the planning matrix (§28) -------------------------------------------

// GetMatrix returns the Date × Line grid the planning screen renders.
func (ctl *PlanningController) GetMatrix(c *gin.Context) {
	companyID := CompanyID(c)

	versionID, ok := QueryID(c, "versionId")
	if !ok {
		return
	}
	movementTypeID, ok := QueryID(c, "movementTypeId")
	if !ok {
		return
	}
	materialID, ok := QueryID(c, "materialId")
	if !ok {
		return
	}
	processID, ok := QueryID(c, "processId")
	if !ok {
		return
	}
	if versionID == nil || movementTypeID == nil || materialID == nil {
		Fail(c, missingMatrixSelection())
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

	matrix, err := ctl.planning.GetMatrix(c.Request.Context(), companyID, *versionID,
		*movementTypeID, *materialID, processID, from, to)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.MatrixToDTO(matrix))
}

// GetMatrixSet returns every product the version plans in the window, each
// with its own grid — the whole plan over several days rather than one product
// at a time.
func (ctl *PlanningController) GetMatrixSet(c *gin.Context) {
	versionID, ok := QueryID(c, "versionId")
	if !ok {
		return
	}
	if versionID == nil {
		Fail(c, missingMatrixSelection())
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

	companyID := CompanyID(c)
	series, err := ctl.planning.GetMatrixSet(c.Request.Context(), companyID, *versionID, from, to)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.MatrixSetToDTO(companyID, *versionID, from, to, series))
}

// SaveMatrix is the matrix-shaped bulk save. It takes one product or several
// in the same payload, and is idempotent either way: replaying it produces the
// same grid rather than duplicate rows.
func (ctl *PlanningController) SaveMatrix(c *gin.Context) {
	req, ok := Bind[dto.MatrixSaveRequest](c)
	if !ok {
		return
	}
	set := mapper.MatrixSetRequestFromDTO(req, CompanyID(c))
	saved, err := ctl.planning.SaveMatrixSet(c.Request.Context(), set, UserID(c))
	if err != nil {
		Fail(c, err)
		return
	}

	// A single-product save answers with that product's grid, so a client
	// written against the original contract sees exactly what it did before.
	if len(req.Series) == 0 && len(saved) == 1 {
		OK(c, mapper.MatrixToDTO(&saved[0]))
		return
	}
	OK(c, mapper.MatrixSetToDTO(set.CompanyID, set.VersionID, set.DateFrom, set.DateTo, saved))
}
