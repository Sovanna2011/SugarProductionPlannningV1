package service

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/config"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
)

// BootstrapService creates the first administrator and, in DEV/SIT, a set of
// demo users. Users are created here rather than in a migration because
// password hashing belongs to the application, not to SQL.
type BootstrapService struct {
	users     *UserService
	userRepo  interfaces.UserRepository
	companies interfaces.CompanyRepositoryIface
	authz     *AuthorizationService
	cfg       *config.Config
	log       zerolog.Logger
}

func NewBootstrapService(users *UserService, userRepo interfaces.UserRepository,
	companies interfaces.CompanyRepositoryIface, authz *AuthorizationService,
	cfg *config.Config, log zerolog.Logger) *BootstrapService {
	return &BootstrapService{
		users: users, userRepo: userRepo, companies: companies,
		authz: authz, cfg: cfg, log: log.With().Str("component", "bootstrap").Logger(),
	}
}

// demoUser describes one seeded user and the roles it holds per company.
// The matrix deliberately gives one user different roles in different
// companies, which is the §13 requirement made visible in the demo data.
type demoUser struct {
	Username string
	FullName string
	Email    string
	// roles maps a company code to the role codes held there.
	Roles map[string][]string
	// defaultCompany is the company pre-selected in the UI drop-down.
	DefaultCompany string
}

var demoUsers = []demoUser{
	{
		Username: "planner", FullName: "Sophea Planner", Email: "planner@example.com",
		DefaultCompany: "1000",
		Roles: map[string][]string{
			"1000": {"PLANNER"},
			"2000": {"VIEWER"}, // same user, a different role per company
		},
	},
	{
		Username: "approver", FullName: "Dara Approver", Email: "approver@example.com",
		DefaultCompany: "1000",
		Roles: map[string][]string{
			"1000": {"APPROVER"},
			"2000": {"APPROVER"},
			"3000": {"VIEWER"},
		},
	},
	{
		Username: "operator", FullName: "Vichea Operator", Email: "operator@example.com",
		DefaultCompany: "1000",
		Roles: map[string][]string{
			"1000": {"OPERATOR", "INVENTORY"},
		},
	},
	{
		Username: "viewer", FullName: "Bopha Viewer", Email: "viewer@example.com",
		DefaultCompany: "3000",
		Roles: map[string][]string{
			"1000": {"VIEWER"},
			"2000": {"VIEWER"},
			"3000": {"VIEWER"},
		},
	},
}

// Run is idempotent: an existing user is left untouched, so restarting the
// service never resets a password that an operator has already changed.
func (s *BootstrapService) Run(ctx context.Context) error {
	if err := s.ensureAdmin(ctx); err != nil {
		return err
	}
	if s.cfg.Security.SeedDemoUsers && !s.cfg.IsProduction() {
		if err := s.ensureDemoUsers(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *BootstrapService) ensureAdmin(ctx context.Context) error {
	username := s.cfg.Security.BootstrapAdminUser
	existing, err := s.findUser(ctx, username)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	password := s.cfg.Security.BootstrapAdminPass
	if password == "" {
		if s.cfg.IsProduction() {
			return apperrors.ErrValidation.
				Msgf("BOOTSTRAP_ADMIN_PASSWORD must be set before the first start in PRD")
		}
		password = s.cfg.Security.DemoUserPassword
		s.log.Warn().Str("username", username).
			Msg("no BOOTSTRAP_ADMIN_PASSWORD set — using the development default; change it before any shared environment")
	}

	admin := &model.User{
		Username:           username,
		Email:              s.cfg.Security.BootstrapAdminEmail,
		FullName:           "System Administrator",
		MustChangePassword: true,
	}
	admin.IsActive = true

	// The administrator is given the ADMIN role in every company that exists,
	// so a fresh installation is administrable from the first login.
	companies, err := s.companies.List(ctx, interfaces.ListOptions{Size: interfaces.MaxPageSize})
	if err != nil {
		return err
	}
	adminRole, err := s.authz.FindRoleByCode(ctx, "ADMIN")
	if err != nil {
		return err
	}

	assignments := make([]CompanyRoleAssignment, 0, len(companies.Rows))
	for i, company := range companies.Rows {
		assignments = append(assignments, CompanyRoleAssignment{
			CompanyID: company.ID,
			RoleIDs:   []int64{adminRole.ID},
			IsDefault: i == 0,
		})
	}

	if err := s.users.Create(ctx, admin, password, assignments); err != nil {
		return err
	}
	s.log.Info().Str("username", username).Int("companies", len(assignments)).
		Msg("bootstrap administrator created")
	return nil
}

func (s *BootstrapService) ensureDemoUsers(ctx context.Context) error {
	for _, seed := range demoUsers {
		existing, err := s.findUser(ctx, seed.Username)
		if err != nil {
			return err
		}
		if existing != nil {
			continue
		}

		assignments := make([]CompanyRoleAssignment, 0, len(seed.Roles))
		for companyCode, roleCodes := range seed.Roles {
			company, err := s.companies.FindByCode(ctx, companyCode)
			if err != nil {
				if appErr, ok := apperrors.As(err); ok && appErr.Code == apperrors.ErrNotFound.Code {
					continue // demo company not present in this environment
				}
				return err
			}
			roleIDs := make([]int64, 0, len(roleCodes))
			for _, roleCode := range roleCodes {
				role, err := s.authz.FindRoleByCode(ctx, roleCode)
				if err != nil {
					return err
				}
				roleIDs = append(roleIDs, role.ID)
			}
			assignments = append(assignments, CompanyRoleAssignment{
				CompanyID: company.ID,
				RoleIDs:   roleIDs,
				IsDefault: companyCode == seed.DefaultCompany,
			})
		}
		if len(assignments) == 0 {
			continue
		}

		user := &model.User{
			Username: seed.Username,
			Email:    seed.Email,
			FullName: seed.FullName,
		}
		user.IsActive = true

		if err := s.users.Create(ctx, user, s.cfg.Security.DemoUserPassword, assignments); err != nil {
			return err
		}
		s.log.Info().Str("username", seed.Username).Msg("demo user created")
	}
	return nil
}

func (s *BootstrapService) findUser(ctx context.Context, username string) (*model.User, error) {
	user, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil {
		if appErr, ok := apperrors.As(err); ok && appErr.Code == apperrors.ErrNotFound.Code {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}
