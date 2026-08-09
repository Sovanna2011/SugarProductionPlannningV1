// Package model holds the GORM entities. Per §A2 rule 4 these never cross the
// controller boundary — controllers speak DTO only.
package model

import "time"

// Base carries the audit and locking columns every business table has (§B1).
// created_at / changed_at are filled from the server clock by GORM, never
// from the client (§7, §D4).
type Base struct {
	ID        int64     `gorm:"primaryKey;column:id"`
	IsActive  bool      `gorm:"column:is_active;not null;default:true"`
	Version   int       `gorm:"column:version;not null;default:1"`
	CreatedBy *int64    `gorm:"column:created_by"`
	CreatedAt time.Time `gorm:"column:created_at;not null;autoCreateTime"`
	ChangedBy *int64    `gorm:"column:changed_by"`
	ChangedAt time.Time `gorm:"column:changed_at;not null;autoCreateTime;autoUpdateTime"`
}

// GetID lets generic repository helpers read the surrogate key.
func (b Base) GetID() int64 { return b.ID }

// GetVersion / SetVersion back the optimistic locking of §D2.
func (b *Base) GetVersion() int  { return b.Version }
func (b *Base) SetVersion(v int) { b.Version = v }
func (b *Base) SetActive(a bool) { b.IsActive = a }

// SetAuditUser is called by the audit interceptor.
func (b *Base) SetAuditUser(userID int64, isCreate bool) {
	if isCreate && b.CreatedBy == nil {
		b.CreatedBy = &userID
	}
	b.ChangedBy = &userID
}

// CompanyScoped marks an entity that belongs to exactly one company. The
// repository layer requires it so that a company filter can be applied as a
// second barrier behind the authorization middleware (§C2).
type CompanyScoped interface {
	GetCompanyID() int64
}
