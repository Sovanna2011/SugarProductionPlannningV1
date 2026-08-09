package model

import (
	"time"

	"gorm.io/datatypes"
)

// --- number ranges (§35) -------------------------------------------------

const (
	NumberObjectPlan      = "PLAN"
	NumberObjectActual    = "ACTUAL"
	NumberObjectInventory = "INVENTORY"
	NumberObjectTransfer  = "TRANSFER"
	NumberObjectHarvest   = "HARVEST"
	NumberObjectDelivery  = "DELIVERY"
)

type NumberRange struct {
	Base
	CompanyID  int64  `gorm:"column:company_id;not null"`
	ObjectType string `gorm:"column:object_type;size:30;not null"`
	FiscalYear int    `gorm:"column:fiscal_year;not null"`
	Prefix     string `gorm:"column:prefix;size:10;not null"`
	CurrentNo  int64  `gorm:"column:current_no;not null;default:0"`
	Length     int    `gorm:"column:length;not null;default:6"`
}

func (NumberRange) TableName() string     { return "number_ranges" }
func (n NumberRange) GetCompanyID() int64 { return n.CompanyID }

// --- audit log (§7) ------------------------------------------------------

const (
	AuditInsert           = "INSERT"
	AuditUpdate           = "UPDATE"
	AuditDelete           = "DELETE"
	AuditApprove          = "APPROVE"
	AuditPost             = "POST"
	AuditReverse          = "REVERSE"
	AuditLogin            = "LOGIN"
	AuditLoginFail        = "LOGIN_FAIL"
	AuditPermissionDenied = "PERMISSION_DENIED"
	AuditBrowse           = "BROWSE"
	AuditExport           = "EXPORT"
)

type AuditLog struct {
	ID         int64          `gorm:"primaryKey;column:id"`
	TableName_ string         `gorm:"column:table_name;size:80;not null"`
	RecordID   *int64         `gorm:"column:record_id"`
	CompanyID  *int64         `gorm:"column:company_id"`
	Action     string         `gorm:"column:action;size:20;not null"`
	ChangedBy  *int64         `gorm:"column:changed_by"`
	ChangedAt  time.Time      `gorm:"column:changed_at;not null;autoCreateTime"`
	OldValues  datatypes.JSON `gorm:"column:old_values;type:jsonb"`
	NewValues  datatypes.JSON `gorm:"column:new_values;type:jsonb"`
	RequestID  *string        `gorm:"column:request_id;size:64"`
	IPAddress  *string        `gorm:"column:ip_address;size:64"`
}

func (AuditLog) TableName() string { return "audit_log" }

// --- data dictionary (Part G) --------------------------------------------

const (
	DDTableTypeMaster      = "MASTER"
	DDTableTypeTransaction = "TRANSACTION"
	DDTableTypeCustomizing = "CUSTOMIZING"
	DDTableTypeTechnical   = "TECHNICAL"
)

type DDDomain struct {
	Base
	DomainName  string         `gorm:"column:domain_name;size:60;not null;uniqueIndex"`
	DataType    string         `gorm:"column:data_type;size:30;not null"`
	Length      *int           `gorm:"column:length"`
	Decimals    *int           `gorm:"column:decimals"`
	FixedValues datatypes.JSON `gorm:"column:fixed_values;type:jsonb"`
	Description *string        `gorm:"column:description"`
}

func (DDDomain) TableName() string { return "dd_domains" }

type DDValueHelp struct {
	Base
	ValueHelpName string         `gorm:"column:value_help_name;size:60;not null;uniqueIndex"`
	HelpType      string         `gorm:"column:help_type;size:20;not null"`
	FixedValues   datatypes.JSON `gorm:"column:fixed_values;type:jsonb"`
	SourceTable   *string        `gorm:"column:source_table;size:80"`
	KeyField      *string        `gorm:"column:key_field;size:80"`
	TextField     *string        `gorm:"column:text_field;size:80"`
}

func (DDValueHelp) TableName() string { return "dd_value_helps" }

type DDTable struct {
	Base
	TableNameCol        string  `gorm:"column:table_name;size:80;not null;uniqueIndex"`
	Module              string  `gorm:"column:module;size:40;not null"`
	TableType           string  `gorm:"column:table_type;size:20;not null"`
	DescriptionEN       *string `gorm:"column:description_en;size:255"`
	DescriptionLocal    *string `gorm:"column:description_local;size:255"`
	BusinessDescription *string `gorm:"column:business_description"`
	IsCompanyDependent  bool    `gorm:"column:is_company_dependent;not null;default:false"`
	IsBrowsable         bool    `gorm:"column:is_browsable;not null;default:false"`

	Fields []DDField `gorm:"foreignKey:TableID"`
}

func (DDTable) TableName() string { return "dd_tables" }

type DDField struct {
	Base
	TableID             int64   `gorm:"column:table_id;not null"`
	FieldName           string  `gorm:"column:field_name;size:80;not null"`
	Position            int     `gorm:"column:position;not null;default:0"`
	LabelShort          *string `gorm:"column:label_short;size:20"`
	LabelMedium         *string `gorm:"column:label_medium;size:40"`
	LabelLong           *string `gorm:"column:label_long;size:100"`
	DomainID            *int64  `gorm:"column:domain_id"`
	ValueHelpID         *int64  `gorm:"column:value_help_id"`
	DataType            string  `gorm:"column:data_type;size:30;not null"`
	Length              *int    `gorm:"column:length"`
	Decimals            *int    `gorm:"column:decimals"`
	IsKey               bool    `gorm:"column:is_key;not null;default:false"`
	IsRequired          bool    `gorm:"column:is_required;not null;default:false"`
	IsPII               bool    `gorm:"column:is_pii;not null;default:false"`
	IsBrowsable         bool    `gorm:"column:is_browsable;not null;default:true"`
	BusinessDescription *string `gorm:"column:business_description"`
}

func (DDField) TableName() string { return "dd_fields" }
