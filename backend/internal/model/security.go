package model

import "time"

type Company struct {
	Base
	CompanyCode       string  `gorm:"column:company_code;size:10;not null;uniqueIndex"`
	CompanyName       string  `gorm:"column:company_name;size:200;not null"`
	LocalCurrency     string  `gorm:"column:local_currency;size:3;not null"`
	GroupCurrency     string  `gorm:"column:group_currency;size:3;not null"`
	Timezone          string  `gorm:"column:timezone;size:64;not null"`
	FiscalYearVariant *string `gorm:"column:fiscal_year_variant;size:10"`
	CountryCode       *string `gorm:"column:country_code;size:2"`
}

func (Company) TableName() string { return "companies" }

// User deliberately has no company_id column (§10): one login serves many
// companies through user_companies.
type User struct {
	Base
	Username           string     `gorm:"column:username;size:60;not null;uniqueIndex"`
	Email              string     `gorm:"column:email;size:200;not null;uniqueIndex"`
	FullName           string     `gorm:"column:full_name;size:200;not null"`
	PasswordHash       string     `gorm:"column:password_hash;size:255;not null"`
	IsLocked           bool       `gorm:"column:is_locked;not null;default:false"`
	FailedLoginCount   int        `gorm:"column:failed_login_count;not null;default:0"`
	MustChangePassword bool       `gorm:"column:must_change_password;not null;default:false"`
	LastLoginAt        *time.Time `gorm:"column:last_login_at"`
	Locale             string     `gorm:"column:locale;size:10;not null;default:en"`
}

func (User) TableName() string { return "users" }

type Role struct {
	Base
	RoleCode    string  `gorm:"column:role_code;size:40;not null;uniqueIndex"`
	RoleName    string  `gorm:"column:role_name;size:120;not null"`
	Description *string `gorm:"column:description"`

	Permissions []Permission `gorm:"many2many:role_permissions;joinForeignKey:role_id;joinReferences:permission_id"`
}

func (Role) TableName() string { return "roles" }

type Permission struct {
	Base
	PermissionCode string  `gorm:"column:permission_code;size:80;not null;uniqueIndex"`
	Module         string  `gorm:"column:module;size:40;not null"`
	Description    *string `gorm:"column:description"`
}

func (Permission) TableName() string { return "permissions" }

type RolePermission struct {
	Base
	RoleID       int64 `gorm:"column:role_id;not null"`
	PermissionID int64 `gorm:"column:permission_id;not null"`
}

func (RolePermission) TableName() string { return "role_permissions" }

// UserCompany is the authorization anchor checked by CompanyAuthMiddleware.
type UserCompany struct {
	Base
	UserID    int64 `gorm:"column:user_id;not null"`
	CompanyID int64 `gorm:"column:company_id;not null"`
	IsDefault bool  `gorm:"column:is_default;not null;default:false"`

	Company *Company `gorm:"foreignKey:CompanyID"`
}

func (UserCompany) TableName() string     { return "user_companies" }
func (u UserCompany) GetCompanyID() int64 { return u.CompanyID }

// UserCompanyRole realises "same user, different role per company" (§13).
type UserCompanyRole struct {
	Base
	UserID    int64 `gorm:"column:user_id;not null"`
	CompanyID int64 `gorm:"column:company_id;not null"`
	RoleID    int64 `gorm:"column:role_id;not null"`

	Role *Role `gorm:"foreignKey:RoleID"`
}

func (UserCompanyRole) TableName() string     { return "user_company_roles" }
func (u UserCompanyRole) GetCompanyID() int64 { return u.CompanyID }

type RefreshToken struct {
	ID         int64      `gorm:"primaryKey;column:id"`
	UserID     int64      `gorm:"column:user_id;not null"`
	TokenHash  string     `gorm:"column:token_hash;size:255;not null;uniqueIndex"`
	JTI        string     `gorm:"column:jti;size:64;not null"`
	ExpiresAt  time.Time  `gorm:"column:expires_at;not null"`
	RevokedAt  *time.Time `gorm:"column:revoked_at"`
	ReplacedBy *int64     `gorm:"column:replaced_by"`
	UserAgent  *string    `gorm:"column:user_agent;size:255"`
	IPAddress  *string    `gorm:"column:ip_address;size:64"`
	CreatedAt  time.Time  `gorm:"column:created_at;not null;autoCreateTime"`
}

func (RefreshToken) TableName() string { return "refresh_tokens" }

// IsUsable reports whether the token may still be exchanged.
func (r RefreshToken) IsUsable(now time.Time) bool {
	return r.RevokedAt == nil && now.Before(r.ExpiresAt)
}
