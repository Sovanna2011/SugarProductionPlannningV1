package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/dto"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/mapper"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/middleware"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/service"
)

type AuthController struct {
	auth    *service.AuthService
	limiter *middleware.RateLimiter
}

func NewAuthController(auth *service.AuthService, limiter *middleware.RateLimiter) *AuthController {
	return &AuthController{auth: auth, limiter: limiter}
}

// Login returns the token pair together with the authorised companies, which
// is what fills the company drop-down of every view (§11).
func (ctl *AuthController) Login(c *gin.Context) {
	req, ok := Bind[dto.LoginRequest](c)
	if !ok {
		return
	}

	// The per-user limit is applied once the user name is known, so guessing
	// one account from many addresses is throttled too (§C4).
	if !ctl.limiter.AllowLogin(req.Username) {
		Fail(c, apperrors.ErrUnauthorized.
			Msgf("too many attempts for this user, please wait before trying again"))
		return
	}

	result, err := ctl.auth.Login(c.Request.Context(), req.Username, req.Password,
		c.GetHeader("User-Agent"), c.ClientIP())
	if err != nil {
		Fail(c, err)
		return
	}

	OK(c, dto.LoginResponse{
		Tokens: dto.TokenResponse{
			AccessToken:  result.Tokens.AccessToken,
			RefreshToken: result.Tokens.RefreshToken,
			TokenType:    result.Tokens.TokenType,
			ExpiresIn:    result.Tokens.ExpiresIn,
		},
		User:               mapper.UserToDTO(result.Profile.User),
		Companies:          companyAccessToDTO(result.Profile.Companies),
		MustChangePassword: result.MustChangePassword,
	})
}

func (ctl *AuthController) Refresh(c *gin.Context) {
	req, ok := Bind[dto.RefreshRequest](c)
	if !ok {
		return
	}
	pair, err := ctl.auth.Refresh(c.Request.Context(), req.RefreshToken,
		c.GetHeader("User-Agent"), c.ClientIP())
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, dto.TokenResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		TokenType:    pair.TokenType,
		ExpiresIn:    pair.ExpiresIn,
	})
}

func (ctl *AuthController) Logout(c *gin.Context) {
	if err := ctl.auth.Logout(c.Request.Context(), UserID(c)); err != nil {
		Fail(c, err)
		return
	}
	NoContent(c)
}

// Me drives the UI5 shell: the user, the companies they may work in, and the
// permissions that apply in each. There is no "current company" in the
// response because there is no global company context (§9).
func (ctl *AuthController) Me(c *gin.Context) {
	profile, err := ctl.auth.Me(c.Request.Context(), UserID(c))
	if err != nil {
		Fail(c, err)
		return
	}
	OK(c, dto.ProfileResponse{
		User:      mapper.UserToDTO(profile.User),
		Companies: companyAccessToDTO(profile.Companies),
	})
}

func (ctl *AuthController) ChangePassword(c *gin.Context) {
	req, ok := Bind[dto.ChangePasswordRequest](c)
	if !ok {
		return
	}
	err := ctl.auth.ChangePassword(c.Request.Context(), UserID(c),
		req.CurrentPassword, req.NewPassword)
	if err != nil {
		Fail(c, err)
		return
	}
	NoContent(c)
}

func companyAccessToDTO(access []service.CompanyAccess) []dto.CompanyAccess {
	out := make([]dto.CompanyAccess, 0, len(access))
	for _, entry := range access {
		roles := make([]string, 0, len(entry.Roles))
		for _, role := range entry.Roles {
			roles = append(roles, role.RoleCode)
		}
		out = append(out, dto.CompanyAccess{
			CompanyID:     entry.Company.ID,
			CompanyCode:   entry.Company.CompanyCode,
			CompanyName:   entry.Company.CompanyName,
			LocalCurrency: entry.Company.LocalCurrency,
			Timezone:      entry.Company.Timezone,
			IsDefault:     entry.IsDefault,
			Roles:         roles,
			Permissions:   entry.Permissions,
		})
	}
	return out
}
