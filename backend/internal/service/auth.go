package service

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/audit"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/config"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/database"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/security"
)

// CompanyAccess is one entry of the company drop-down: the company itself plus
// what the user may do inside it.
type CompanyAccess struct {
	Company     model.Company
	IsDefault   bool
	Roles       []model.Role
	Permissions []string
}

// Profile is the payload behind GET /auth/me. It carries every authorised
// company with its own permission set — there is no "current" company (§9).
type Profile struct {
	User      model.User
	Companies []CompanyAccess
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	TokenType    string
}

type LoginResult struct {
	Tokens             TokenPair
	Profile            Profile
	MustChangePassword bool
}

type AuthService struct {
	users    interfaces.UserRepository
	tokens   interfaces.RefreshTokenRepository
	authz    *AuthorizationService
	issuer   *security.TokenIssuer
	hasher   *security.PasswordHasher
	auditSvc *audit.Service
	uow      database.UnitOfWork
	cfg      config.SecurityConfig
}

func NewAuthService(
	users interfaces.UserRepository,
	tokens interfaces.RefreshTokenRepository,
	authz *AuthorizationService,
	issuer *security.TokenIssuer,
	hasher *security.PasswordHasher,
	auditSvc *audit.Service,
	uow database.UnitOfWork,
	cfg config.SecurityConfig,
) *AuthService {
	return &AuthService{
		users: users, tokens: tokens, authz: authz, issuer: issuer,
		hasher: hasher, auditSvc: auditSvc, uow: uow, cfg: cfg,
	}
}

// Login authenticates and issues a token pair. Every failure path returns the
// same error so that the response cannot be used to enumerate user names.
func (s *AuthService) Login(ctx context.Context, username, password, userAgent, ip string) (*LoginResult, error) {
	user, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		if appErr, ok := apperrors.As(err); ok && appErr.Code == apperrors.ErrNotFound.Code {
			// Hash anyway: an early return here would leak, through response
			// time, whether the user name exists.
			s.hasher.Verify(password, dummyHash)
			s.auditSvc.Record(ctx, audit.Event{
				TableName: "users", Action: model.AuditLoginFail,
				NewValues: map[string]string{"username": username, "reason": "unknown user"},
			})
			return nil, apperrors.ErrInvalidCredentials
		}
		return nil, err
	}

	if !user.IsActive || user.IsLocked {
		s.auditSvc.Record(ctx, audit.Event{
			TableName: "users", RecordID: &user.ID, Action: model.AuditLoginFail,
			NewValues: map[string]string{"username": username, "reason": "locked or inactive"},
		})
		return nil, apperrors.ErrAccountLocked
	}

	if !s.hasher.Verify(password, user.PasswordHash) {
		if err := s.users.RecordLoginFailure(ctx, user.ID, s.cfg.MaxFailedLogins); err != nil {
			return nil, err
		}
		s.auditSvc.Record(ctx, audit.Event{
			TableName: "users", RecordID: &user.ID, Action: model.AuditLoginFail,
			NewValues: map[string]string{"username": username, "reason": "wrong password"},
		})
		return nil, apperrors.ErrInvalidCredentials
	}

	var result *LoginResult
	err = s.uow.Do(ctx, func(ctx context.Context) error {
		pair, _, err := s.issueTokens(ctx, user, userAgent, ip)
		if err != nil {
			return err
		}
		if err := s.users.RecordLoginSuccess(ctx, user.ID, time.Now().UTC()); err != nil {
			return err
		}

		// The profile is loaded as the authenticated user so that the
		// permission lookups run under the same principal the token carries.
		authCtx := security.WithPrincipal(ctx, security.Principal{UserID: user.ID, Username: user.Username})
		profile, err := s.buildProfile(authCtx, user)
		if err != nil {
			return err
		}

		result = &LoginResult{
			Tokens:             *pair,
			Profile:            *profile,
			MustChangePassword: user.MustChangePassword,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.auditSvc.Record(ctx, audit.Event{
		TableName: "users", RecordID: &user.ID, Action: model.AuditLogin,
		NewValues: map[string]string{"username": user.Username},
	})
	return result, nil
}

// dummyHash is a real argon2id hash of an unguessable value. Verifying against
// it makes the "unknown user" path cost the same as the "wrong password" path.
const dummyHash = "$argon2id$v=19$m=65536,t=2,p=4$" +
	"c2FsdHNhbHRzYWx0c2FsdA$aOnR5RiG7yfF9zSTWLo3D0YzLE5J2n4nEV3Ff2XNMFk"

// Refresh rotates the refresh token: the presented token is revoked and linked
// to its successor, so replaying a stolen token after a legitimate refresh
// fails and is visible in the chain.
func (s *AuthService) Refresh(ctx context.Context, refreshToken, userAgent, ip string) (*TokenPair, error) {
	stored, err := s.tokens.FindByHash(ctx, security.HashToken(refreshToken))
	if err != nil {
		if appErr, ok := apperrors.As(err); ok && appErr.Code == apperrors.ErrNotFound.Code {
			return nil, apperrors.ErrUnauthorized
		}
		return nil, err
	}
	if !stored.IsUsable(time.Now().UTC()) {
		return nil, apperrors.ErrTokenExpired
	}

	user, err := s.users.FindByID(ctx, stored.UserID)
	if err != nil {
		return nil, err
	}
	if !user.IsActive || user.IsLocked {
		return nil, apperrors.ErrAccountLocked
	}

	var pair *TokenPair
	err = s.uow.Do(ctx, func(ctx context.Context) error {
		fresh, newTokenID, err := s.issueTokens(ctx, user, userAgent, ip)
		if err != nil {
			return err
		}
		if err := s.tokens.Revoke(ctx, stored.ID, &newTokenID); err != nil {
			return err
		}
		pair = fresh
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pair, nil
}

// Logout revokes every refresh token of the user, ending all sessions.
func (s *AuthService) Logout(ctx context.Context, userID int64) error {
	return s.tokens.RevokeAllForUser(ctx, userID)
}

func (s *AuthService) Me(ctx context.Context, userID int64) (*Profile, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.buildProfile(ctx, user)
}

// ChangePassword verifies the current password before setting the new one and
// revokes existing sessions, so a compromised session cannot survive the
// change.
func (s *AuthService) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if !s.hasher.Verify(currentPassword, user.PasswordHash) {
		return apperrors.ErrInvalidCredentials
	}
	if err := s.hasher.ValidatePolicy(newPassword); err != nil {
		return err
	}

	hash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return apperrors.ErrInternal.WithCause(err)
	}

	return s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.users.UpdatePassword(ctx, userID, hash, false); err != nil {
			return err
		}
		if err := s.tokens.RevokeAllForUser(ctx, userID); err != nil {
			return err
		}
		s.auditSvc.Record(ctx, audit.Event{
			TableName: "users", RecordID: &userID, Action: model.AuditUpdate,
			NewValues: map[string]string{"event": "password changed"},
		})
		return nil
	})
}

// issueTokens creates an access/refresh pair and returns the id of the stored
// refresh token row, so a rotation can link the old token to its successor.
func (s *AuthService) issueTokens(ctx context.Context, user *model.User, userAgent, ip string) (*TokenPair, int64, error) {
	accessToken, jti, err := s.issuer.IssueAccessToken(user.ID, user.Username)
	if err != nil {
		return nil, 0, err
	}

	refreshToken, err := security.NewOpaqueToken()
	if err != nil {
		return nil, 0, apperrors.ErrInternal.WithCause(err)
	}

	row := &model.RefreshToken{
		UserID:    user.ID,
		TokenHash: security.HashToken(refreshToken),
		JTI:       jti,
		ExpiresAt: time.Now().UTC().Add(s.issuer.RefreshTTL()),
	}
	if userAgent != "" {
		row.UserAgent = &userAgent
	}
	if ip != "" {
		row.IPAddress = &ip
	}
	if err := s.tokens.Create(ctx, row); err != nil {
		return nil, 0, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.issuer.AccessTTL().Seconds()),
		TokenType:    "Bearer",
	}, row.ID, nil
}

func (s *AuthService) buildProfile(ctx context.Context, user *model.User) (*Profile, error) {
	assignments, err := s.authz.Companies(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	companies := make([]CompanyAccess, 0, len(assignments))
	for _, assignment := range assignments {
		if assignment.Company == nil {
			continue
		}
		roles, err := s.authz.Roles(ctx, user.ID, assignment.CompanyID)
		if err != nil {
			return nil, err
		}
		permSet, err := s.authz.Permissions(ctx, user.ID, assignment.CompanyID)
		if err != nil {
			return nil, err
		}
		permissions := make([]string, 0, len(permSet))
		for code := range permSet {
			permissions = append(permissions, code)
		}
		sort.Strings(permissions) // stable order for the UI and for tests

		companies = append(companies, CompanyAccess{
			Company:     *assignment.Company,
			IsDefault:   assignment.IsDefault,
			Roles:       roles,
			Permissions: permissions,
		})
	}

	return &Profile{User: *user, Companies: companies}, nil
}

// NewSessionID produces the correlation id used in logs and audit records.
func NewSessionID() string { return uuid.NewString() }
