package service

import (
	"context"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/audit"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/database"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/security"
)

// CompanyRoleAssignment is one line of a user's authorisation matrix: which
// role the user holds in which company (§13).
type CompanyRoleAssignment struct {
	CompanyID int64
	RoleIDs   []int64
	IsDefault bool
}

type UserService struct {
	users    interfaces.UserRepository
	authz    *AuthorizationService
	hasher   *security.PasswordHasher
	auditSvc *audit.Service
	uow      database.UnitOfWork
}

func NewUserService(users interfaces.UserRepository, authz *AuthorizationService,
	hasher *security.PasswordHasher, auditSvc *audit.Service,
	uow database.UnitOfWork) *UserService {
	return &UserService{users: users, authz: authz, hasher: hasher, auditSvc: auditSvc, uow: uow}
}

func (s *UserService) List(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.User], error) {
	return s.users.List(ctx, opts)
}

func (s *UserService) Get(ctx context.Context, id int64) (*model.User, error) {
	return s.users.FindByID(ctx, id)
}

// Create registers a user and its per-company roles in one transaction. The
// user carries no company of its own (§10) — the assignments are the link.
func (s *UserService) Create(ctx context.Context, user *model.User, password string,
	assignments []CompanyRoleAssignment) error {

	if err := s.hasher.ValidatePolicy(password); err != nil {
		return err
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return apperrors.ErrInternal.WithCause(err)
	}
	user.PasswordHash = hash

	return s.uow.Do(ctx, func(ctx context.Context) error {
		if err := s.users.Create(ctx, user); err != nil {
			return err
		}
		if err := s.applyAssignments(ctx, user.ID, assignments); err != nil {
			return err
		}
		s.auditSvc.Record(ctx, audit.Event{
			TableName: "users", RecordID: &user.ID, Action: model.AuditInsert,
			NewValues: map[string]any{"username": user.Username, "companies": len(assignments)},
		})
		return nil
	})
}

func (s *UserService) Update(ctx context.Context, user *model.User, assignments []CompanyRoleAssignment) error {
	return s.uow.Do(ctx, func(ctx context.Context) error {
		current, err := s.users.FindByID(ctx, user.ID)
		if err != nil {
			return err
		}
		current.FullName = user.FullName
		current.Email = user.Email
		current.Locale = user.Locale
		current.IsActive = user.IsActive
		current.IsLocked = user.IsLocked
		current.Version = user.Version

		if err := s.users.Update(ctx, current); err != nil {
			return err
		}
		if assignments != nil {
			if err := s.applyAssignments(ctx, current.ID, assignments); err != nil {
				return err
			}
		}
		s.authz.Invalidate(current.ID)
		return nil
	})
}

// ResetPassword sets a new password and forces a change at next login (§C4).
func (s *UserService) ResetPassword(ctx context.Context, userID int64, newPassword string) error {
	if err := s.hasher.ValidatePolicy(newPassword); err != nil {
		return err
	}
	hash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return apperrors.ErrInternal.WithCause(err)
	}
	if err := s.users.UpdatePassword(ctx, userID, hash, true); err != nil {
		return err
	}
	s.auditSvc.Record(ctx, audit.Event{
		TableName: "users", RecordID: &userID, Action: model.AuditUpdate,
		NewValues: map[string]string{"event": "password reset by administrator"},
	})
	return nil
}

// applyAssignments reconciles the authorisation matrix with what the caller
// sent: roles present in the payload are granted, and roles the user currently
// holds in that company but which the payload omits are revoked.
//
// Reconciling rather than only adding is what makes it possible to take an
// authorisation away — an add-only implementation would let a role be granted
// but never withdrawn.
func (s *UserService) applyAssignments(ctx context.Context, userID int64, assignments []CompanyRoleAssignment) error {
	for _, assignment := range assignments {
		uc := &model.UserCompany{
			UserID:    userID,
			CompanyID: assignment.CompanyID,
			IsDefault: assignment.IsDefault,
		}
		if err := s.authz.AssignCompany(ctx, uc); err != nil {
			return err
		}

		wanted := make(map[int64]bool, len(assignment.RoleIDs))
		for _, roleID := range assignment.RoleIDs {
			wanted[roleID] = true

			ucr := &model.UserCompanyRole{
				UserID:    userID,
				CompanyID: assignment.CompanyID,
				RoleID:    roleID,
			}
			if err := s.authz.AssignRole(ctx, ucr); err != nil {
				return err
			}
		}

		current, err := s.authz.Roles(ctx, userID, assignment.CompanyID)
		if err != nil {
			return err
		}
		for _, role := range current {
			if wanted[role.ID] {
				continue
			}
			if err := s.authz.RemoveRole(ctx, userID, assignment.CompanyID, role.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// Assignments returns the authorisation matrix of a user.
func (s *UserService) Assignments(ctx context.Context, userID int64) ([]CompanyRoleAssignment, error) {
	companies, err := s.authz.Companies(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]CompanyRoleAssignment, 0, len(companies))
	for _, company := range companies {
		roles, err := s.authz.Roles(ctx, userID, company.CompanyID)
		if err != nil {
			return nil, err
		}
		ids := make([]int64, 0, len(roles))
		for _, role := range roles {
			ids = append(ids, role.ID)
		}
		out = append(out, CompanyRoleAssignment{
			CompanyID: company.CompanyID,
			RoleIDs:   ids,
			IsDefault: company.IsDefault,
		})
	}
	return out, nil
}
