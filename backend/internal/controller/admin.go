package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/audit"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/dto"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/mapper"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/service"
)

// AdminController serves user, role and number-range administration plus the
// audit log.
type AdminController struct {
	users     *service.UserService
	authz     *service.AuthorizationService
	numbering *service.NumberRangeService
	auditSvc  *audit.Service
}

func NewAdminController(users *service.UserService, authz *service.AuthorizationService,
	numbering *service.NumberRangeService, auditSvc *audit.Service) *AdminController {
	return &AdminController{users: users, authz: authz, numbering: numbering, auditSvc: auditSvc}
}

func (ctl *AdminController) ListUsers(c *gin.Context) {
	opts, ok := ListOptions(c, nil)
	if !ok {
		return
	}
	page, err := ctl.users.List(c.Request.Context(), opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.UserToDTO), opts, page.Total)
}

func (ctl *AdminController) GetUser(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	user, err := ctl.users.Get(c.Request.Context(), id)
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.UserToDTO(*user))
}

// CreateUser registers a user with its per-company roles. A user has no
// company of its own — the assignments are the only link (§10, §13).
func (ctl *AdminController) CreateUser(c *gin.Context) {
	req, ok := Bind[dto.CreateUserRequest](c)
	if !ok {
		return
	}

	user := &model.User{
		Username: req.Username,
		Email:    req.Email,
		FullName: req.FullName,
		Locale:   req.Locale,
	}
	if user.Locale == "" {
		user.Locale = "en"
	}
	user.IsActive = true

	err := ctl.users.Create(c.Request.Context(), user, req.Password,
		assignmentsFromDTO(req.Assignments))
	if err != nil {
		Fail(c, err)
		return
	}
	Created(c, mapper.UserToDTO(*user))
}

func (ctl *AdminController) UpdateUser(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	req, ok := Bind[dto.UpdateUserRequest](c)
	if !ok {
		return
	}

	user := &model.User{
		Email:    req.Email,
		FullName: req.FullName,
		Locale:   req.Locale,
		IsLocked: req.IsLocked,
	}
	user.ID = id
	user.IsActive = req.IsActive
	user.Version = req.Version

	var assignments []service.CompanyRoleAssignment
	if req.Assignments != nil {
		assignments = assignmentsFromDTO(req.Assignments)
	}

	if err := ctl.users.Update(c.Request.Context(), user, assignments); err != nil {
		Fail(c, err)
		return
	}
	OK(c, mapper.UserToDTO(*user))
}

func (ctl *AdminController) ResetPassword(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	req, ok := Bind[dto.ResetPasswordRequest](c)
	if !ok {
		return
	}
	if err := ctl.users.ResetPassword(c.Request.Context(), id, req.NewPassword); err != nil {
		Fail(c, err)
		return
	}
	NoContent(c)
}

func (ctl *AdminController) UserAssignments(c *gin.Context) {
	id, ok := PathID(c, "id")
	if !ok {
		return
	}
	assignments, err := ctl.users.Assignments(c.Request.Context(), id)
	if err != nil {
		Fail(c, err)
		return
	}

	out := make([]dto.CompanyRoleAssignmentRequest, 0, len(assignments))
	for _, assignment := range assignments {
		out = append(out, dto.CompanyRoleAssignmentRequest{
			CompanyID: assignment.CompanyID,
			RoleIDs:   assignment.RoleIDs,
			IsDefault: assignment.IsDefault,
		})
	}
	OK(c, dto.UserAssignmentsResponse{UserID: id, Assignments: out})
}

func (ctl *AdminController) ListRoles(c *gin.Context) {
	opts, ok := ListOptions(c, nil)
	if !ok {
		return
	}
	page, err := ctl.authz.ListRoles(c.Request.Context(), opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.RoleToDTO), opts, page.Total)
}

func (ctl *AdminController) ListPermissions(c *gin.Context) {
	opts, ok := ListOptions(c, nil)
	if !ok {
		return
	}
	page, err := ctl.authz.ListPermissions(c.Request.Context(), opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.PermissionToDTO), opts, page.Total)
}

func (ctl *AdminController) ListNumberRanges(c *gin.Context) {
	companyID := CompanyID(c)
	opts, ok := ListOptions(c, &companyID)
	if !ok {
		return
	}
	page, err := ctl.numbering.List(c.Request.Context(), opts)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.NumberRangeToDTO), opts, page.Total)
}

// AuditLog is scoped to the company of the request, so an administrator of one
// company cannot read another company's history.
func (ctl *AdminController) AuditLog(c *gin.Context) {
	companyID := CompanyID(c)
	opts, ok := ListOptions(c, &companyID)
	if !ok {
		return
	}
	recordID, ok := QueryID(c, "recordId")
	if !ok {
		return
	}

	page, err := ctl.auditSvc.List(c.Request.Context(), opts, c.Query("tableName"), recordID)
	if err != nil {
		Fail(c, err)
		return
	}
	Paged(c, mapper.Slice(page.Rows, mapper.AuditLogToDTO), opts, page.Total)
}

func assignmentsFromDTO(requests []dto.CompanyRoleAssignmentRequest) []service.CompanyRoleAssignment {
	out := make([]service.CompanyRoleAssignment, 0, len(requests))
	for _, req := range requests {
		out = append(out, service.CompanyRoleAssignment{
			CompanyID: req.CompanyID,
			RoleIDs:   req.RoleIDs,
			IsDefault: req.IsDefault,
		})
	}
	return out
}
