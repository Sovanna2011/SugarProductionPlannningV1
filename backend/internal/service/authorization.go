// Package service holds the business logic. It depends on repository
// interfaces only (§A2 rule 2) and owns every transaction boundary (rule 6).
package service

import (
	"context"
	"sync"
	"time"

	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
)

// Permission codes referenced from Go. Keeping them as constants means a typo
// is a compile error rather than a silently ungranted authorisation.
const (
	PermMaterialEdit     = "MASTER.MATERIAL.EDIT"
	PermWarehouseEdit    = "MASTER.WAREHOUSE.EDIT"
	PermLineEdit         = "MASTER.PRODUCTIONLINE.EDIT"
	PermSeasonEdit       = "MASTER.SEASON.EDIT"
	PermProcessEdit      = "MASTER.PROCESS.EDIT"
	PermPackagingEdit    = "MASTER.PACKAGING.EDIT"
	PermMovementTypeEdit = "MASTER.MOVEMENTTYPE.EDIT"
	PermUOMEdit          = "MASTER.UOM.EDIT"
	PermCompanyEdit      = "MASTER.COMPANY.EDIT"

	PermVersionView       = "PLAN.VERSION.VIEW"
	PermVersionCreate     = "PLAN.VERSION.CREATE"
	PermVersionEdit       = "PLAN.VERSION.EDIT"
	PermVersionCopy       = "PLAN.VERSION.COPY"
	PermVersionSubmit     = "PLAN.VERSION.SUBMIT"
	PermVersionApprove    = "PLAN.VERSION.APPROVE"
	PermVersionLock       = "PLAN.VERSION.LOCK"
	PermVersionCancel     = "PLAN.VERSION.CANCEL"
	PermPlanItemView      = "PLAN.ITEM.VIEW"
	PermPlanItemEdit      = "PLAN.ITEM.EDIT"
	PermPlanEditSubmitted = "PLAN.EDIT.SUBMITTED"

	PermCaneVarietyView   = "CANE.VARIETY.VIEW"
	PermCaneVarietyEdit   = "CANE.VARIETY.EDIT"
	PermGrowerView        = "CANE.GROWER.VIEW"
	PermGrowerEdit        = "CANE.GROWER.EDIT"
	PermCaneFieldView     = "CANE.FIELD.VIEW"
	PermCaneFieldEdit     = "CANE.FIELD.EDIT"
	PermHarvestPlanView   = "CANE.PLAN.VIEW"
	PermHarvestPlanEdit   = "CANE.PLAN.EDIT"
	PermDeliveryView      = "CANE.DELIVERY.VIEW"
	PermDeliveryCreate    = "CANE.DELIVERY.CREATE"
	PermDeliveryEdit      = "CANE.DELIVERY.EDIT"
	PermDeliveryPost      = "CANE.DELIVERY.POST"
	PermDeliveryReverse   = "CANE.DELIVERY.REVERSE"
	PermReportCane        = "REPORT.CANE.VIEW"

	PermActualView    = "ACTUAL.VIEW"
	PermActualCreate  = "ACTUAL.CREATE"
	PermActualEdit    = "ACTUAL.EDIT"
	PermActualPost    = "ACTUAL.POST"
	PermActualReverse = "ACTUAL.REVERSE"

	PermMovementView    = "INV.MOVEMENT.VIEW"
	PermMovementCreate  = "INV.MOVEMENT.CREATE"
	PermMovementReverse = "INV.MOVEMENT.REVERSE"
	PermTransferCreate  = "INV.TRANSFER.CREATE"
	PermAdjustCreate    = "INV.ADJUSTMENT.CREATE"
	PermBalanceView     = "INV.BALANCE.VIEW"

	PermReportPlanActual = "REPORT.PLANACTUAL.VIEW"
	PermReportProduction = "REPORT.PRODUCTION.VIEW"
	PermReportInventory  = "REPORT.INVENTORY.VIEW"
	PermReportCapacity   = "REPORT.CAPACITY.VIEW"
	PermReportExport     = "REPORT.EXPORT"

	PermDDView        = "DD.VIEW"
	PermDDMaintain    = "DD.MAINTAIN"
	PermBrowserView   = "BROWSER.VIEW"
	PermBrowserExport = "BROWSER.EXPORT"

	PermUserManage = "ADMIN.USER.MANAGE"
	PermRoleManage = "ADMIN.ROLE.MANAGE"
	PermAuditView  = "ADMIN.AUDIT.VIEW"
)

// AuthorizationService implements steps 3 and 4 of the pipeline in §C2.
type AuthorizationService struct {
	repo  interfaces.AuthorizationRepository
	cache *permissionCache
}

func NewAuthorizationService(repo interfaces.AuthorizationRepository, ttl time.Duration) *AuthorizationService {
	return &AuthorizationService{repo: repo, cache: newPermissionCache(ttl)}
}

// EnsureCompanyAccess re-validates the company the caller named. The
// client-supplied company id is a request parameter, never an assertion of
// rights (§C2).
func (s *AuthorizationService) EnsureCompanyAccess(ctx context.Context, userID, companyID int64) error {
	if companyID == 0 {
		return apperrors.ErrCompanyRequired
	}
	allowed, err := s.repo.IsUserAuthorisedForCompany(ctx, userID, companyID)
	if err != nil {
		return err
	}
	if !allowed {
		return apperrors.ErrCompanyNotAuthorized
	}
	return nil
}

// EnsurePermission checks one permission within one company. It never
// consults a per-user-only cache: the same user may be a planner in company
// 1000 and a display user in company 2000.
func (s *AuthorizationService) EnsurePermission(ctx context.Context, userID, companyID int64, permission string) error {
	granted, err := s.HasPermission(ctx, userID, companyID, permission)
	if err != nil {
		return err
	}
	if !granted {
		return apperrors.ErrPermissionDenied.WithDetails(
			apperrors.Detail{Field: "permission", Value: permission})
	}
	return nil
}

func (s *AuthorizationService) HasPermission(ctx context.Context, userID, companyID int64, permission string) (bool, error) {
	perms, err := s.Permissions(ctx, userID, companyID)
	if err != nil {
		return false, err
	}
	_, ok := perms[permission]
	return ok, nil
}

// Permissions resolves the permission set of (user, company), using the
// short-lived cache of §C3.
func (s *AuthorizationService) Permissions(ctx context.Context, userID, companyID int64) (map[string]struct{}, error) {
	if cached, ok := s.cache.get(userID, companyID); ok {
		return cached, nil
	}
	codes, err := s.repo.PermissionsFor(ctx, userID, companyID)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(codes))
	for _, c := range codes {
		set[c] = struct{}{}
	}
	s.cache.put(userID, companyID, set)
	return set, nil
}

// Companies drives GET /auth/me and therefore the UI5 company drop-down (§11).
func (s *AuthorizationService) Companies(ctx context.Context, userID int64) ([]model.UserCompany, error) {
	return s.repo.CompaniesFor(ctx, userID)
}

func (s *AuthorizationService) Roles(ctx context.Context, userID, companyID int64) ([]model.Role, error) {
	return s.repo.RolesFor(ctx, userID, companyID)
}

// EnsureCompaniesAccess validates every company of a consolidated report
// request. An unauthorised id fails the whole call rather than being filtered
// out silently, which keeps the audit trail unambiguous (§E6 / OQ-6).
func (s *AuthorizationService) EnsureCompaniesAccess(ctx context.Context, userID int64, companyIDs []int64, permission string) error {
	if len(companyIDs) == 0 {
		return apperrors.ErrCompanyRequired
	}
	for _, id := range companyIDs {
		if err := s.EnsureCompanyAccess(ctx, userID, id); err != nil {
			return err
		}
		if err := s.EnsurePermission(ctx, userID, id, permission); err != nil {
			return err
		}
	}
	return nil
}

// Invalidate drops cached permissions after a role or assignment change.
func (s *AuthorizationService) Invalidate(userID int64) { s.cache.invalidateUser(userID) }

func (s *AuthorizationService) AssignCompany(ctx context.Context, uc *model.UserCompany) error {
	if err := s.repo.AssignCompany(ctx, uc); err != nil {
		return err
	}
	s.Invalidate(uc.UserID)
	return nil
}

func (s *AuthorizationService) AssignRole(ctx context.Context, ucr *model.UserCompanyRole) error {
	if err := s.repo.AssignRole(ctx, ucr); err != nil {
		return err
	}
	s.Invalidate(ucr.UserID)
	return nil
}

func (s *AuthorizationService) RemoveRole(ctx context.Context, userID, companyID, roleID int64) error {
	if err := s.repo.RemoveRole(ctx, userID, companyID, roleID); err != nil {
		return err
	}
	s.Invalidate(userID)
	return nil
}

func (s *AuthorizationService) ListRoles(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.Role], error) {
	return s.repo.ListRoles(ctx, opts)
}

func (s *AuthorizationService) ListPermissions(ctx context.Context, opts interfaces.ListOptions) (interfaces.Page[model.Permission], error) {
	return s.repo.ListPermissions(ctx, opts)
}

func (s *AuthorizationService) FindRoleByCode(ctx context.Context, code string) (*model.Role, error) {
	return s.repo.FindRoleByCode(ctx, code)
}

// --- cache ---------------------------------------------------------------

type cacheKey struct {
	userID    int64
	companyID int64
}

type cacheEntry struct {
	permissions map[string]struct{}
	expiresAt   time.Time
}

// permissionCache keys on (user, company) — never on user alone, which would
// leak authorisations between companies (§C3).
type permissionCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[cacheKey]cacheEntry
}

func newPermissionCache(ttl time.Duration) *permissionCache {
	if ttl <= 0 {
		ttl = time.Minute
	}
	return &permissionCache{ttl: ttl, entries: make(map[cacheKey]cacheEntry)}
}

func (c *permissionCache) get(userID, companyID int64) (map[string]struct{}, bool) {
	c.mu.RLock()
	entry, ok := c.entries[cacheKey{userID, companyID}]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.permissions, true
}

func (c *permissionCache) put(userID, companyID int64, permissions map[string]struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[cacheKey{userID, companyID}] = cacheEntry{
		permissions: permissions,
		expiresAt:   time.Now().Add(c.ttl),
	}
	c.evictExpiredLocked()
}

func (c *permissionCache) invalidateUser(userID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.entries {
		if key.userID == userID {
			delete(c.entries, key)
		}
	}
}

// evictExpiredLocked keeps the map from growing without bound in a long-lived
// process. It is cheap because the map only ever holds active sessions.
func (c *permissionCache) evictExpiredLocked() {
	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}
