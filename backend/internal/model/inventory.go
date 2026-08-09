package model

import (
	"time"

	"github.com/shopspring/decimal"
)

const (
	SourceModuleActual   = "ACTUAL"
	SourceModuleManual   = "MANUAL"
	SourceModuleTransfer = "TRANSFER"
	SourceModuleOpening  = "OPENING"
	SourceModuleReversal = "REVERSAL"
)

// InventoryMovement is an append-only ledger row (§33). Quantity is ALWAYS
// positive — the direction comes from the movement type, so reporting sums
// quantity * direction sign.
type InventoryMovement struct {
	Base
	CompanyID          int64           `gorm:"column:company_id;not null"`
	DocumentNo         string          `gorm:"column:document_no;size:40;not null"`
	LineNo             int             `gorm:"column:line_no;not null;default:1"`
	TransactionDate    time.Time       `gorm:"column:transaction_date;type:date;not null"`
	MaterialID         int64           `gorm:"column:material_id;not null"`
	WarehouseID        int64           `gorm:"column:warehouse_id;not null"`
	MovementTypeID     int64           `gorm:"column:movement_type_id;not null"`
	PackagingTypeID    *int64          `gorm:"column:packaging_type_id"`
	ProcessID          *int64          `gorm:"column:process_id"`
	Quantity           decimal.Decimal `gorm:"column:quantity;type:numeric(18,3);not null"`
	UOMID              int64           `gorm:"column:uom_id;not null"`
	ReferenceDocument  *string         `gorm:"column:reference_document;size:40"`
	ReferenceID        *int64          `gorm:"column:reference_id"`
	SourceModule       string          `gorm:"column:source_module;size:20;not null"`
	TransferGroupID    *string         `gorm:"column:transfer_group_id;size:40"`
	ReversedMovementID *int64          `gorm:"column:reversed_movement_id"`
	IsReversed         bool            `gorm:"column:is_reversed;not null;default:false"`
	Remark             *string         `gorm:"column:remark;size:500"`

	MovementType *MovementType `gorm:"foreignKey:MovementTypeID"`
}

func (InventoryMovement) TableName() string     { return "inventory_movements" }
func (m InventoryMovement) GetCompanyID() int64 { return m.CompanyID }

// InventoryBalance is the materialised daily balance maintained incrementally
// on posting and recomputed by the nightly reconciliation job.
//
//	Closing = Opening + IN − OUT
type InventoryBalance struct {
	ID              int64           `gorm:"primaryKey;column:id"`
	CompanyID       int64           `gorm:"column:company_id;not null"`
	WarehouseID     int64           `gorm:"column:warehouse_id;not null"`
	MaterialID      int64           `gorm:"column:material_id;not null"`
	PackagingTypeID *int64          `gorm:"column:packaging_type_id"`
	BalanceDate     time.Time       `gorm:"column:balance_date;type:date;not null"`
	OpeningQty      decimal.Decimal `gorm:"column:opening_qty;type:numeric(18,3);not null"`
	InQty           decimal.Decimal `gorm:"column:in_qty;type:numeric(18,3);not null"`
	OutQty          decimal.Decimal `gorm:"column:out_qty;type:numeric(18,3);not null"`
	ClosingQty      decimal.Decimal `gorm:"column:closing_qty;type:numeric(18,3);not null"`
	UOMID           *int64          `gorm:"column:uom_id"`
	Version         int             `gorm:"column:version;not null;default:1"`
	CreatedBy       *int64          `gorm:"column:created_by"`
	CreatedAt       time.Time       `gorm:"column:created_at;not null;autoCreateTime"`
	ChangedBy       *int64          `gorm:"column:changed_by"`
	ChangedAt       time.Time       `gorm:"column:changed_at;not null;autoCreateTime;autoUpdateTime"`
}

func (InventoryBalance) TableName() string     { return "inventory_balances" }
func (b InventoryBalance) GetCompanyID() int64 { return b.CompanyID }

// Recompute keeps closing consistent with the movement columns.
func (b *InventoryBalance) Recompute() {
	b.ClosingQty = b.OpeningQty.Add(b.InQty).Sub(b.OutQty)
}
