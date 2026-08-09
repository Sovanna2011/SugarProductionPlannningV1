package middleware

import (
	"runtime"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/audit"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/security"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/service"
)

// Auth is step 1 of the pipeline in §C2: validate the token and establish the
// principal. A missing, malformed or expired token is 401 — never 403.
func Auth(issuer *security.TokenIssuer) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			Abort(c, apperrors.ErrUnauthorized)
			return
		}

		principal, err := issuer.ParseAccessToken(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			Abort(c, err)
			return
		}

		c.Set(CtxUserID, principal.UserID)
		c.Request = c.Request.WithContext(security.WithPrincipal(c.Request.Context(), principal))
		c.Next()
	}
}

// CompanyAuth is steps 2 and 3: take the company the caller named and check it
// against user_companies.
//
// The company travels per request — path parameter, query parameter or the
// X-Company-Id header — and is never read from a session or from the token
// (§9). It is treated purely as a request parameter here: the entitlement
// comes from the database check below, never from the fact that the client
// sent it (§12).
func CompanyAuth(authz *service.AuthorizationService, auditSvc *audit.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		companyID, err := extractCompanyID(c)
		if err != nil {
			Abort(c, err)
			return
		}

		userID := c.GetInt64(CtxUserID)
		if err := authz.EnsureCompanyAccess(c.Request.Context(), userID, companyID); err != nil {
			if appErr, ok := apperrors.As(err); ok && appErr.Code == apperrors.ErrCompanyNotAuthorized.Code {
				auditSvc.Record(c.Request.Context(), audit.Event{
					TableName: "user_companies",
					CompanyID: &companyID,
					Action:    model.AuditPermissionDenied,
					NewValues: map[string]any{"route": c.FullPath(), "reason": "company not authorised"},
				})
			}
			Abort(c, err)
			return
		}

		c.Set(CtxCompanyID, companyID)
		c.Next()
	}
}

// RequirePermission is step 4: the permission is resolved for this user in
// this company. The same user may hold it in one company and not in another,
// which is why the check can never be cached per user alone (§13, §C3).
func RequirePermission(authz *service.AuthorizationService, auditSvc *audit.Service, permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetInt64(CtxUserID)
		companyID := c.GetInt64(CtxCompanyID)
		if companyID == 0 {
			Abort(c, apperrors.ErrCompanyRequired)
			return
		}

		if err := authz.EnsurePermission(c.Request.Context(), userID, companyID, permission); err != nil {
			if appErr, ok := apperrors.As(err); ok && appErr.Code == apperrors.ErrPermissionDenied.Code {
				auditSvc.Record(c.Request.Context(), audit.Event{
					TableName: "permissions",
					CompanyID: &companyID,
					Action:    model.AuditPermissionDenied,
					NewValues: map[string]any{"route": c.FullPath(), "permission": permission},
				})
			}
			Abort(c, err)
			return
		}
		c.Next()
	}
}

// RequireAnyCompanyPermission guards a cross-company endpoint — the data
// dictionary, the browser, administration — where the caller must hold the
// permission in at least one authorised company.
func RequireAnyCompanyPermission(authz *service.AuthorizationService, auditSvc *audit.Service, permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetInt64(CtxUserID)

		companies, err := authz.Companies(c.Request.Context(), userID)
		if err != nil {
			Abort(c, err)
			return
		}

		for _, assignment := range companies {
			granted, err := authz.HasPermission(c.Request.Context(), userID, assignment.CompanyID, permission)
			if err != nil {
				Abort(c, err)
				return
			}
			if granted {
				c.Next()
				return
			}
		}

		auditSvc.Record(c.Request.Context(), audit.Event{
			TableName: "permissions", Action: model.AuditPermissionDenied,
			NewValues: map[string]any{"route": c.FullPath(), "permission": permission},
		})
		Abort(c, apperrors.ErrPermissionDenied.WithDetails(
			apperrors.Detail{Field: "permission", Value: permission}))
	}
}

// extractCompanyID reads the company from, in order, the path, the query
// string and the X-Company-Id header. A request body is deliberately not a
// source: the body is read once by the controller's binder, and re-reading it
// here would make the authorization check depend on parsing order.
func extractCompanyID(c *gin.Context) (int64, error) {
	candidates := []string{
		c.Param("companyId"),
		c.Query("companyId"),
		c.GetHeader("X-Company-Id"),
	}
	for _, raw := range candidates {
		if raw == "" {
			continue
		}
		companyID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || companyID <= 0 {
			return 0, apperrors.ErrBadRequest.
				Msgf("companyId must be a positive integer").
				WithDetails(apperrors.Detail{Field: "companyId", Value: raw})
		}
		return companyID, nil
	}
	return 0, apperrors.ErrCompanyRequired
}

// CompanyID returns the validated company of the current request.
func CompanyID(c *gin.Context) int64 { return c.GetInt64(CtxCompanyID) }

// UserID returns the authenticated user of the current request.
func UserID(c *gin.Context) int64 { return c.GetInt64(CtxUserID) }

func stack() []byte {
	buf := make([]byte, 8192)
	n := runtime.Stack(buf, false)
	return buf[:n]
}
