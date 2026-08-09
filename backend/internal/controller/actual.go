package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/dto"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/mapper"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/service"
)

type ActualController struct {
	actuals *service.ActualService
}

func NewActualController(actuals *service.ActualService) *ActualController {
	return &ActualController{actuals: actuals}
}

func (ctl *ActualController) List(c *gin.Context) {
	companyID := CompanyID(c)
	opts, ok := ListOptions(c, &companyID)
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
	movementTypeID, ok := QueryID(c, "movementTypeId")
	if !ok {
		return
	}
	lineID, ok := QueryID(c, "lineId")
	if !ok {
		return
	}

	page, err := ctl.actuals.List(c.Request.Context(), opts, from, to, movementTypeID, lineID)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, func(h model.ActualHeader) dto.ActualHeaderResponse {
		return mapper.ActualHeaderToDTO(h, nil)
	}), opts, page.Total)
}

func (ctl *ActualController) Get(c *gin.Context) {
	id, ok := PathID(c, "headerId")
	if !ok {
		return
	}
	document, err := ctl.actuals.Get(c.Request.Context(), CompanyID(c), id)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.ActualHeaderToDTO(document.Header, document.Items))
}

// Create honours the Idempotency-Key header of §E8, so a client that retries
// after a timeout gets the original document instead of a second one.
func (ctl *ActualController) Create(c *gin.Context) {
	req, ok := Bind[dto.ActualHeaderRequest](c)
	if !ok {
		return
	}
	header := mapper.ActualHeaderFromDTO(req, CompanyID(c), nil)
	items := mapper.Slice(req.Items, mapper.ActualItemFromDTO)

	document, err := ctl.actuals.Create(c.Request.Context(), header, items,
		c.GetHeader("Idempotency-Key"))
	if err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.ActualHeaderToDTO(document.Header, document.Items))
}

func (ctl *ActualController) Update(c *gin.Context) {
	id, ok := PathID(c, "headerId")
	if !ok {
		return
	}
	req, ok := Bind[dto.ActualHeaderRequest](c)
	if !ok {
		return
	}
	header := mapper.ActualHeaderFromDTO(req, CompanyID(c), &model.ActualHeader{})
	header.ID = id

	var items []model.ActualItem
	if req.Items != nil {
		items = mapper.Slice(req.Items, mapper.ActualItemFromDTO)
	}

	document, err := ctl.actuals.Update(c.Request.Context(), header, items)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.ActualHeaderToDTO(document.Header, document.Items))
}

// Post writes the document and its inventory movements in one transaction.
func (ctl *ActualController) Post(c *gin.Context) {
	id, ok := PathID(c, "headerId")
	if !ok {
		return
	}
	document, err := ctl.actuals.Post(c.Request.Context(), CompanyID(c), id, UserID(c))
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.ActualHeaderToDTO(document.Header, document.Items))
}

// Reverse cancels a posted document with opposite movements — nothing is
// deleted (§33).
func (ctl *ActualController) Reverse(c *gin.Context) {
	id, ok := PathID(c, "headerId")
	if !ok {
		return
	}
	req, ok := Bind[dto.ReverseRequest](c)
	if !ok {
		return
	}
	document, err := ctl.actuals.Reverse(c.Request.Context(), CompanyID(c), id, UserID(c), req.Remark)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.ActualHeaderToDTO(document.Header, document.Items))
}
