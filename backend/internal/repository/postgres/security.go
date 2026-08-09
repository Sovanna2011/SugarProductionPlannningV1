package postgres

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/database"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
)

// --- users ---------------------------------------------------------------

type userRepository struct {
	crossRepo[model.User, *model.User]
}

func NewUserRepository(db *gorm.DB) interfaces.UserRepository {
	r := &userRepository{}
	r.db = db
	r.sortable = columns("username", "email", "full_name", "is_locked", "last_login_at", "created_at")
	r.searchable = []string{"username", "email", "full_name"}
	return r
}

func (r *userRepository) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	return findOne[model.User](ctx, r.conn(ctx), "lower(username) = lower(?)", username)
}

func (r *userRepository) RecordLoginSuccess(ctx context.Context, userID int64, at time.Time) error {
	return translate(r.conn(ctx).Model(&model.User{}).Where("id = ?", userID).
		Updates(map[string]any{
			"last_login_at":      at,
			"failed_login_count": 0,
			"changed_at":         at,
		}).Error)
}

// RecordLoginFailure increments the counter and locks the account once the
// configured threshold is reached (§C4).
func (r *userRepository) RecordLoginFailure(ctx context.Context, userID int64, maxFailures int) error {
	return translate(r.conn(ctx).Exec(`
		UPDATE users
		   SET failed_login_count = failed_login_count + 1,
		       is_locked = (failed_login_count + 1) >= ?,
		       changed_at = now()
		 WHERE id = ?`, maxFailures, userID).Error)
}

func (r *userRepository) UpdatePassword(ctx context.Context, userID int64, passwordHash string, mustChange bool) error {
	res := r.conn(ctx).Model(&model.User{}).Where("id = ?", userID).
		Updates(map[string]any{
			"password_hash":        passwordHash,
			"must_change_password": mustChange,
			"failed_login_count":   0,
			"is_locked":            false,
			"version":              gorm.Expr("version + 1"),
			"changed_at":           gorm.Expr("now()"),
		})
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

// --- authorization (§C2) -------------------------------------------------

type authorizationRepository struct {
	db *gorm.DB
}

func NewAuthorizationRepository(db *gorm.DB) interfaces.AuthorizationRepository {
	return &authorizationRepository{db: db}
}

// IsUserAuthorisedForCompany is step 3 of the authorization pipeline. The
// client-supplied company id is only ever a request parameter; this query is
// what turns it into an entitlement.
func (r *authorizationRepository) IsUserAuthorisedForCompany(ctx context.Context, userID, companyID int64) (bool, error) {
	var count int64
	err := database.Conn(ctx, r.db).Model(&model.UserCompany{}).
		Joins("JOIN companies c ON c.id = user_companies.company_id").
		Where("user_companies.user_id = ? AND user_companies.company_id = ? AND user_companies.is_active AND c.is_active",
			userID, companyID).
		Count(&count).Error
	if err != nil {
		return false, translate(err)
	}
	return count > 0, nil
}

// PermissionsFor resolves roles → permissions for exactly one company. The
// company predicate is mandatory: a per-user-only permission set would leak
// authorisations across companies (§C3).
func (r *authorizationRepository) PermissionsFor(ctx context.Context, userID, companyID int64) ([]string, error) {
	var codes []string
	err := database.Conn(ctx, r.db).
		Table("user_company_roles ucr").
		Select("DISTINCT p.permission_code").
		Joins("JOIN roles r ON r.id = ucr.role_id AND r.is_active").
		Joins("JOIN role_permissions rp ON rp.role_id = r.id AND rp.is_active").
		Joins("JOIN permissions p ON p.id = rp.permission_id AND p.is_active").
		Where("ucr.user_id = ? AND ucr.company_id = ? AND ucr.is_active", userID, companyID).
		Pluck("p.permission_code", &codes).Error
	if err != nil {
		return nil, translate(err)
	}
	return codes, nil
}

func (r *authorizationRepository) CompaniesFor(ctx context.Context, userID int64) ([]model.UserCompany, error) {
	var rows []model.UserCompany
	err := database.Conn(ctx, r.db).
		Preload("Company").
		Joins("JOIN companies c ON c.id = user_companies.company_id AND c.is_active").
		Where("user_companies.user_id = ? AND user_companies.is_active", userID).
		Order("c.company_code").
		Find(&rows).Error
	if err != nil {
		return nil, translate(err)
	}
	return rows, nil
}

func (r *authorizationRepository) RolesFor(ctx context.Context, userID, companyID int64) ([]model.Role, error) {
	var roles []model.Role
	err := database.Conn(ctx, r.db).
		Table("roles").
		Joins("JOIN user_company_roles ucr ON ucr.role_id = roles.id AND ucr.is_active").
		Where("ucr.user_id = ? AND ucr.company_id = ? AND roles.is_active", userID, companyID).
		Order("roles.role_code").
		Find(&roles).Error
	if err != nil {
		return nil, translate(err)
	}
	return roles, nil
}

func (r *authorizationRepository) AssignCompany(ctx context.Context, uc *model.UserCompany) error {
	conn := database.Conn(ctx, r.db)
	// The partial unique index allows only one default per user, so an
	// incoming default has to clear the previous one first.
	if uc.IsDefault {
		if err := conn.Model(&model.UserCompany{}).
			Where("user_id = ? AND is_default", uc.UserID).
			Update("is_default", false).Error; err != nil {
			return translate(err)
		}
	}
	uc.IsActive = true
	uc.Version = 1
	return translate(conn.Exec(`
		INSERT INTO user_companies (user_id, company_id, is_default, is_active, version, created_by, changed_by)
		VALUES (?, ?, ?, TRUE, 1, ?, ?)
		ON CONFLICT (user_id, company_id)
		DO UPDATE SET is_default = EXCLUDED.is_default, is_active = TRUE,
		              version = user_companies.version + 1, changed_at = now()`,
		uc.UserID, uc.CompanyID, uc.IsDefault, uc.CreatedBy, uc.ChangedBy).Error)
}

func (r *authorizationRepository) AssignRole(ctx context.Context, ucr *model.UserCompanyRole) error {
	return translate(database.Conn(ctx, r.db).Exec(`
		INSERT INTO user_company_roles (user_id, company_id, role_id, is_active, version, created_by, changed_by)
		VALUES (?, ?, ?, TRUE, 1, ?, ?)
		ON CONFLICT (user_id, company_id, role_id)
		DO UPDATE SET is_active = TRUE, version = user_company_roles.version + 1, changed_at = now()`,
		ucr.UserID, ucr.CompanyID, ucr.RoleID, ucr.CreatedBy, ucr.ChangedBy).Error)
}

func (r *authorizationRepository) RemoveRole(ctx context.Context, userID, companyID, roleID int64) error {
	return translate(database.Conn(ctx, r.db).Model(&model.UserCompanyRole{}).
		Where("user_id = ? AND company_id = ? AND role_id = ?", userID, companyID, roleID).
		Updates(map[string]any{
			"is_active":  false,
			"version":    gorm.Expr("version + 1"),
			"changed_at": gorm.Expr("now()"),
		}).Error)
}

func (r *authorizationRepository) FindRoleByCode(ctx context.Context, code string) (*model.Role, error) {
	return findOne[model.Role](ctx, database.Conn(ctx, r.db), "role_code = ?", code)
}

func (r *authorizationRepository) ListRoles(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.Role], error) {
	opts.Normalise()
	var page interfaces.Page[model.Role]
	q := database.Conn(ctx, r.db).Model(&model.Role{})
	if !opts.IncludeInactive {
		q = q.Where("is_active")
	}
	if err := q.Count(&page.Total).Error; err != nil {
		return page, translate(err)
	}
	err := q.Preload("Permissions").Order("role_code").
		Limit(opts.Size).Offset(opts.Offset()).Find(&page.Rows).Error
	return page, translate(err)
}

func (r *authorizationRepository) ListPermissions(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.Permission], error) {
	opts.Normalise()
	var page interfaces.Page[model.Permission]
	q := database.Conn(ctx, r.db).Model(&model.Permission{}).Where("is_active")
	if err := q.Count(&page.Total).Error; err != nil {
		return page, translate(err)
	}
	err := q.Order("module, permission_code").
		Limit(opts.Size).Offset(opts.Offset()).Find(&page.Rows).Error
	return page, translate(err)
}

// --- refresh tokens ------------------------------------------------------

type refreshTokenRepository struct {
	db *gorm.DB
}

func NewRefreshTokenRepository(db *gorm.DB) interfaces.RefreshTokenRepository {
	return &refreshTokenRepository{db: db}
}

func (r *refreshTokenRepository) Create(ctx context.Context, token *model.RefreshToken) error {
	return translate(database.Conn(ctx, r.db).Create(token).Error)
}

func (r *refreshTokenRepository) FindByHash(ctx context.Context, hash string) (*model.RefreshToken, error) {
	return findOne[model.RefreshToken](ctx, database.Conn(ctx, r.db), "token_hash = ?", hash)
}

func (r *refreshTokenRepository) Revoke(ctx context.Context, id int64, replacedBy *int64) error {
	return translate(database.Conn(ctx, r.db).Model(&model.RefreshToken{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Updates(map[string]any{"revoked_at": gorm.Expr("now()"), "replaced_by": replacedBy}).Error)
}

func (r *refreshTokenRepository) RevokeAllForUser(ctx context.Context, userID int64) error {
	return translate(database.Conn(ctx, r.db).Model(&model.RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", gorm.Expr("now()")).Error)
}

func (r *refreshTokenRepository) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	res := database.Conn(ctx, r.db).Where("expires_at < ?", before).Delete(&model.RefreshToken{})
	if res.Error != nil {
		return 0, translate(res.Error)
	}
	return res.RowsAffected, nil
}

// --- audit log -----------------------------------------------------------

type auditRepository struct {
	db *gorm.DB
}

func NewAuditRepository(db *gorm.DB) interfaces.AuditRepository {
	return &auditRepository{db: db}
}

// Write uses the pool rather than an ambient transaction on purpose: an audit
// record for a rejected business event (a permission denial, a failed login)
// must survive the rollback of that event.
func (r *auditRepository) Write(ctx context.Context, entry *model.AuditLog) error {
	return translate(r.db.WithContext(ctx).Create(entry).Error)
}

func (r *auditRepository) List(ctx context.Context, opts interfaces.ListOptions,
	tableName string, recordID *int64) (interfaces.Page[model.AuditLog], error) {

	opts.Normalise()
	var page interfaces.Page[model.AuditLog]

	q := database.Conn(ctx, r.db).Model(&model.AuditLog{})
	if opts.CompanyID != nil {
		q = q.Where("company_id = ?", *opts.CompanyID)
	}
	if tableName != "" {
		q = q.Where("table_name = ?", tableName)
	}
	if recordID != nil {
		q = q.Where("record_id = ?", *recordID)
	}
	if err := q.Count(&page.Total).Error; err != nil {
		return page, translate(err)
	}
	err := q.Order("changed_at DESC").
		Limit(opts.Size).Offset(opts.Offset()).Find(&page.Rows).Error
	return page, translate(err)
}
