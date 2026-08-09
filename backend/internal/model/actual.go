package model

import (
	"time"

	"github.com/shopspring/decimal"
)

const (
	PostingStatusDraft    = "DRAFT"
	PostingStatusPosted   = "POSTED"
	PostingStatusReversed = "REVERSED"
)

// ActualHeader mirrors PlanHeader structurally but carries no reference to a
// planning version: §32 requires actuals to be recordable independently of
// whether a plan exists.
type ActualHeader struct {
	Base
	CompanyID        int64      `gorm:"column:company_id;not null"`
	SeasonID         *int64     `gorm:"column:season_id"`
	MovementTypeID   int64      `gorm:"column:movement_type_id;not null"`
	ProductionLineID *int64     `gorm:"column:production_line_id"`
	DocumentNo       string     `gorm:"column:document_no;size:40;not null"`
	Description      *string    `gorm:"column:description;size:255"`
	PostingDate      time.Time  `gorm:"column:posting_date;type:date;not null"`
	PostingStatus    string     `gorm:"column:posting_status;size:20;not null;default:DRAFT"`
	PostedBy         *int64     `gorm:"column:posted_by"`
	PostedAt         *time.Time `gorm:"column:posted_at"`
	ReversedBy       *int64     `gorm:"column:reversed_by"`
	ReversedAt       *time.Time `gorm:"column:reversed_at"`
	ReversalOfID     *int64     `gorm:"column:reversal_of_id"`
	IdempotencyKey   *string    `gorm:"column:idempotency_key;size:80"`

	Items []ActualItem `gorm:"foreignKey:ActualHeaderID"`
}

func (ActualHeader) TableName() string     { return "actual_headers" }
func (a ActualHeader) GetCompanyID() int64 { return a.CompanyID }

type ActualItem struct {
	Base
	ActualHeaderID   int64           `gorm:"column:actual_header_id;not null"`
	ActualDate       time.Time       `gorm:"column:actual_date;type:date;not null"`
	ProductionLineID *int64          `gorm:"column:production_line_id"`
	MaterialID       int64           `gorm:"column:material_id;not null"`
	ProcessID        *int64          `gorm:"column:process_id"`
	WarehouseID      *int64          `gorm:"column:warehouse_id"`
	PackagingTypeID  *int64          `gorm:"column:packaging_type_id"`
	Quantity         decimal.Decimal `gorm:"column:quantity;type:numeric(18,3);not null"`
	UOMID            int64           `gorm:"column:uom_id;not null"`
	Shift            *string         `gorm:"column:shift;size:10"`
	Remark           *string         `gorm:"column:remark;size:500"`
}

func (ActualItem) TableName() string { return "actual_items" }
