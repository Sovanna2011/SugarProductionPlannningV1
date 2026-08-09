package dto

import "time"

type LoginRequest struct {
	Username string `json:"username" binding:"required,max=60"`
	Password string `json:"password" binding:"required,max=200"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" binding:"required"`
	NewPassword     string `json:"newPassword" binding:"required,min=12,max=200"`
}

type TokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	TokenType    string `json:"tokenType"`
	ExpiresIn    int64  `json:"expiresIn"`
}

type LoginResponse struct {
	Tokens             TokenResponse   `json:"tokens"`
	User               UserResponse    `json:"user"`
	Companies          []CompanyAccess `json:"companies"`
	MustChangePassword bool            `json:"mustChangePassword"`
}

// CompanyAccess is one entry of the company drop-down of §11: a company the
// user is authorised for, with the roles and permissions that apply there.
// Two entries can carry completely different permissions for the same user.
type CompanyAccess struct {
	CompanyID     int64    `json:"companyId"`
	CompanyCode   string   `json:"companyCode"`
	CompanyName   string   `json:"companyName"`
	LocalCurrency string   `json:"localCurrency"`
	Timezone      string   `json:"timezone"`
	IsDefault     bool     `json:"isDefault"`
	Roles         []string `json:"roles"`
	Permissions   []string `json:"permissions"`
}

type UserResponse struct {
	ID                 int64      `json:"id"`
	Username           string     `json:"username"`
	Email              string     `json:"email"`
	FullName           string     `json:"fullName"`
	Locale             string     `json:"locale"`
	IsLocked           bool       `json:"isLocked"`
	MustChangePassword bool       `json:"mustChangePassword"`
	LastLoginAt        *time.Time `json:"lastLoginAt,omitempty"`
	AuditFields
}

// ProfileResponse is the payload of GET /auth/me. It carries no "current
// company": the client picks one per view (§9, §H2).
type ProfileResponse struct {
	User      UserResponse    `json:"user"`
	Companies []CompanyAccess `json:"companies"`
}

// --- administration ------------------------------------------------------

type CompanyRoleAssignmentRequest struct {
	CompanyID int64   `json:"companyId" binding:"required"`
	RoleIDs   []int64 `json:"roleIds" binding:"required,min=1"`
	IsDefault bool    `json:"isDefault"`
}

type CreateUserRequest struct {
	Username    string                         `json:"username" binding:"required,max=60"`
	Email       string                         `json:"email" binding:"required,email,max=200"`
	FullName    string                         `json:"fullName" binding:"required,max=200"`
	Password    string                         `json:"password" binding:"required,min=12,max=200"`
	Locale      string                         `json:"locale" binding:"omitempty,max=10"`
	Assignments []CompanyRoleAssignmentRequest `json:"assignments" binding:"required,min=1"`
}

type UpdateUserRequest struct {
	Email       string                         `json:"email" binding:"required,email,max=200"`
	FullName    string                         `json:"fullName" binding:"required,max=200"`
	Locale      string                         `json:"locale" binding:"omitempty,max=10"`
	IsActive    bool                           `json:"isActive"`
	IsLocked    bool                           `json:"isLocked"`
	Assignments []CompanyRoleAssignmentRequest `json:"assignments"`
	VersionedRequest
}

type ResetPasswordRequest struct {
	NewPassword string `json:"newPassword" binding:"required,min=12,max=200"`
}

type RoleResponse struct {
	ID          int64    `json:"id"`
	RoleCode    string   `json:"roleCode"`
	RoleName    string   `json:"roleName"`
	Description *string  `json:"description,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	AuditFields
}

type PermissionResponse struct {
	ID             int64   `json:"id"`
	PermissionCode string  `json:"permissionCode"`
	Module         string  `json:"module"`
	Description    *string `json:"description,omitempty"`
}

type UserAssignmentsResponse struct {
	UserID      int64                          `json:"userId"`
	Assignments []CompanyRoleAssignmentRequest `json:"assignments"`
}
