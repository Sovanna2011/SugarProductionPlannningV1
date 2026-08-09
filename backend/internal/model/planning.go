package model

import (
	"time"

	"github.com/shopspring/decimal"
)

// Planning version status machine (§F3):
//
//	DRAFT ──submit──► SUBMITTED ──approve──► APPROVED ──lock──► LOCKED
//	  │                   │                      │
//	  └────cancel─────────┴──────cancel──────────┘ ──► CANCELLED
const (
	VersionStatusDraft     = "DRAFT"
	VersionStatusSubmitted = "SUBMITTED"
	VersionStatusApproved  = "APPROVED"
	VersionStatusLocked    = "LOCKED"
	VersionStatusCancelled = "CANCELLED"
)

type PlanningVersion struct {
	Base
	CompanyID           int64      `gorm:"column:company_id;not null"`
	SeasonID            int64      `gorm:"column:season_id;not null"`
	VersionNo           int        `gorm:"column:version_no;not null"`
	VersionName         string     `gorm:"column:version_name;size:200;not null"`
	Description         *string    `gorm:"column:description"`
	Status              string     `gorm:"column:status;size:20;not null;default:DRAFT"`
	CopiedFromVersionID *int64     `gorm:"column:copied_from_version_id"`
	SubmittedBy         *int64     `gorm:"column:submitted_by"`
	SubmittedAt         *time.Time `gorm:"column:submitted_at"`
	ApprovedBy          *int64     `gorm:"column:approved_by"`
	ApprovedAt          *time.Time `gorm:"column:approved_at"`
	LockedBy            *int64     `gorm:"column:locked_by"`
	LockedAt            *time.Time `gorm:"column:locked_at"`

	Season *Season `gorm:"foreignKey:SeasonID"`
}

func (PlanningVersion) TableName() string     { return "planning_versions" }
func (v PlanningVersion) GetCompanyID() int64 { return v.CompanyID }

// IsEditable reports whether plan data of this version may be changed at all.
// SUBMITTED is editable only for a user holding PLAN.EDIT.SUBMITTED, which the
// service checks separately.
func (v PlanningVersion) IsEditable() bool {
	return v.Status == VersionStatusDraft || v.Status == VersionStatusSubmitted
}

// AllowedTransitions is the single source of truth for the status machine.
var AllowedTransitions = map[string][]string{
	VersionStatusDraft:     {VersionStatusSubmitted, VersionStatusCancelled},
	VersionStatusSubmitted: {VersionStatusApproved, VersionStatusDraft, VersionStatusCancelled},
	VersionStatusApproved:  {VersionStatusLocked, VersionStatusCancelled},
	VersionStatusLocked:    {},
	VersionStatusCancelled: {},
}

// CanTransitionTo reports whether the transition is part of the state machine.
func (v PlanningVersion) CanTransitionTo(target string) bool {
	for _, allowed := range AllowedTransitions[v.Status] {
		if allowed == target {
			return true
		}
	}
	return false
}

// PlanHeader is one planning document per company / season / version /
// movement type, optionally narrowed to one production line (§28).
type PlanHeader struct {
	Base
	CompanyID         int64     `gorm:"column:company_id;not null"`
	SeasonID          int64     `gorm:"column:season_id;not null"`
	PlanningVersionID int64     `gorm:"column:planning_version_id;not null"`
	MovementTypeID    int64     `gorm:"column:movement_type_id;not null"`
	ProductionLineID  *int64    `gorm:"column:production_line_id"`
	DocumentNo        string    `gorm:"column:document_no;size:40;not null"`
	Description       *string   `gorm:"column:description;size:255"`
	DateFrom          time.Time `gorm:"column:date_from;type:date;not null"`
	DateTo            time.Time `gorm:"column:date_to;type:date;not null"`

	Items []PlanItem `gorm:"foreignKey:PlanHeaderID"`
}

func (PlanHeader) TableName() string     { return "plan_headers" }
func (p PlanHeader) GetCompanyID() int64 { return p.CompanyID }

// PlanItem is the daily granular row. Its business key
// (header, date, line, material, process) is what makes the Date × Line
// matrix of §28 round-trippable.
type PlanItem struct {
	Base
	PlanHeaderID     int64           `gorm:"column:plan_header_id;not null"`
	PlanDate         time.Time       `gorm:"column:plan_date;type:date;not null"`
	ProductionLineID *int64          `gorm:"column:production_line_id"`
	MaterialID       int64           `gorm:"column:material_id;not null"`
	ProcessID        *int64          `gorm:"column:process_id"`
	WarehouseID      *int64          `gorm:"column:warehouse_id"`
	PackagingTypeID  *int64          `gorm:"column:packaging_type_id"`
	Quantity         decimal.Decimal `gorm:"column:quantity;type:numeric(18,3);not null"`
	UOMID            int64           `gorm:"column:uom_id;not null"`
	Remark           *string         `gorm:"column:remark;size:500"`
}

func (PlanItem) TableName() string { return "plan_items" }
