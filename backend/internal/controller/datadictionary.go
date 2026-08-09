package controller

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/dto"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/mapper"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/service"
)

type DataDictionaryController struct {
	dd    *service.DataDictionaryService
	authz *service.AuthorizationService
}

func NewDataDictionaryController(dd *service.DataDictionaryService,
	authz *service.AuthorizationService) *DataDictionaryController {
	return &DataDictionaryController{dd: dd, authz: authz}
}

func (ctl *DataDictionaryController) ListTables(c *gin.Context) {
	tables, err := ctl.dd.ListTables(c.Request.Context(),
		c.Query("module"), c.Query("search"), c.Query("browsableOnly") == "true")
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.Slice(tables, mapper.DDTableToDTO))
}

func (ctl *DataDictionaryController) GetTable(c *gin.Context) {
	table, err := ctl.dd.GetTable(c.Request.Context(), c.Param("tableName"))
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.DDTableToDTO(*table))
}

func (ctl *DataDictionaryController) Fields(c *gin.Context) {
	fields, err := ctl.dd.Fields(c.Request.Context(), c.Param("tableName"))
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.Slice(fields, mapper.DDFieldToDTO))
}

// UpdateTable maintains the business description. The structural attributes
// stay owned by the generator (§G1).
func (ctl *DataDictionaryController) UpdateTable(c *gin.Context) {
	req, ok := Bind[dto.DDTableUpdateRequest](c)
	if !ok {
		return
	}
	table, err := ctl.dd.GetTable(c.Request.Context(), c.Param("tableName"))
	if err != nil {
		Fail(c, err)
		return
	}

	table.DescriptionEN = req.DescriptionEN
	table.DescriptionLocal = req.DescriptionLocal
	table.BusinessDescription = req.BusinessDescription
	table.IsBrowsable = req.IsBrowsable
	table.Version = req.Version
	userID := UserID(c)
	table.ChangedBy = &userID

	if err := ctl.dd.UpdateTable(c.Request.Context(), table); err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.DDTableToDTO(*table))
}

func (ctl *DataDictionaryController) UpdateField(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	req, ok := Bind[dto.DDFieldUpdateRequest](c)
	if !ok {
		return
	}

	field, err := ctl.dd.GetField(c.Request.Context(), id)
	if err != nil {
		Fail(c, err)
		return
	}

	field.LabelShort = req.LabelShort
	field.LabelMedium = req.LabelMedium
	field.LabelLong = req.LabelLong
	field.BusinessDescription = req.BusinessDescription
	field.IsPII = req.IsPII
	field.IsBrowsable = req.IsBrowsable
	field.ValueHelpID = req.ValueHelpID
	field.Version = req.Version
	userID := UserID(c)
	field.ChangedBy = &userID

	if err := ctl.dd.UpdateField(c.Request.Context(), field); err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.DDFieldToDTO(*field))
}

func (ctl *DataDictionaryController) ListDomains(c *gin.Context) {
	domains, err := ctl.dd.ListDomains(c.Request.Context())
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.Slice(domains, mapper.DDDomainToDTO))
}

// Browse is the SE16-like data browser. Everything that makes it safe lives in
// the service: the table whitelist, the field validation, the injected company
// filter, PII masking, the row cap and the audit record.
func (ctl *DataDictionaryController) Browse(c *gin.Context) {
	ctl.browse(c, false)
}

// Export is the same query with the larger row cap and its own permission.
func (ctl *DataDictionaryController) Export(c *gin.Context) {
	ctl.browse(c, true)
}

func (ctl *DataDictionaryController) browse(c *gin.Context, export bool) {
	opts, ok := ListOptions(c, nil)
	if !ok {
		return
	}
	requestedCompanyID, ok := QueryID(c, "companyId")
	if !ok {
		return
	}

	// The caller can only ever narrow the scope to companies it is already
	// authorised for; the list comes from the database, not from the request.
	assignments, err := ctl.authz.Companies(c.Request.Context(), UserID(c))
	if err != nil {
		Fail(c, err)
		return
	}
	authorised := make([]int64, 0, len(assignments))
	for _, assignment := range assignments {
		authorised = append(authorised, assignment.CompanyID)
	}

	var fields []string
	if raw := c.Query("fields"); raw != "" {
		for _, name := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(name); trimmed != "" {
				fields = append(fields, trimmed)
			}
		}
	}

	result, err := ctl.dd.Browse(c.Request.Context(), service.BrowserRequest{
		TableName:            c.Param("tableName"),
		Fields:               fields,
		Filters:              opts.Filters,
		Sort:                 opts.Sort,
		Page:                 opts.Page,
		Size:                 opts.Size,
		AuthorisedCompanyIDs: authorised,
		RequestedCompanyID:   requestedCompanyID,
		Export:               export,
	})
	if err != nil {
		Fail(c, err)
		return
	}

	c.JSON(200, dto.Envelope{
		Success: true,
		Data: dto.BrowserResponse{
			TableName: c.Param("tableName"),
			Fields:    result.Fields,
			Rows:      result.Rows,
		},
		Meta: &dto.Meta{Page: opts.Page, Size: opts.Size, Total: result.Total},
	})
}
