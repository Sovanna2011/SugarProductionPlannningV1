package postgres

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/database"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/model"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/interfaces"
)

type dataDictionaryRepository struct {
	db *gorm.DB
}

func NewDataDictionaryRepository(db *gorm.DB) interfaces.DataDictionaryRepository {
	return &dataDictionaryRepository{db: db}
}

// tableProfile is the classification the generator applies to a physical
// table. It mirrors the entity groups of §B2 and decides what the data browser
// is allowed to show.
type tableProfile struct {
	Module           string
	TableType        string
	CompanyDependent bool
	Browsable        bool
	Description      string
}

// Sensitive tables are deliberately not browsable (Part G rule 6): users holds
// password hashes, refresh_tokens holds credentials, audit_log holds the
// before/after payload of every other table.
var tableProfiles = map[string]tableProfile{
	"companies":           {"SECURITY", model.DDTableTypeMaster, false, true, "Companies of the group"},
	"users":               {"SECURITY", model.DDTableTypeMaster, false, false, "Application users"},
	"roles":               {"SECURITY", model.DDTableTypeCustomizing, false, true, "Authorisation roles"},
	"permissions":         {"SECURITY", model.DDTableTypeCustomizing, false, true, "Authorisation objects"},
	"role_permissions":    {"SECURITY", model.DDTableTypeCustomizing, false, true, "Role to permission assignment"},
	"user_companies":      {"SECURITY", model.DDTableTypeMaster, true, true, "User to company authorisation"},
	"user_company_roles":  {"SECURITY", model.DDTableTypeMaster, true, true, "User role per company"},
	"refresh_tokens":      {"SECURITY", model.DDTableTypeTechnical, false, false, "Issued refresh tokens"},
	"uoms":                {"MASTER", model.DDTableTypeCustomizing, false, true, "Units of measure"},
	"uom_conversions":     {"MASTER", model.DDTableTypeCustomizing, false, true, "Unit of measure conversion factors"},
	"materials":           {"MASTER", model.DDTableTypeMaster, false, true, "Materials (cross-company)"},
	"company_materials":   {"MASTER", model.DDTableTypeMaster, true, true, "Material relevance per company"},
	"packaging_types":     {"MASTER", model.DDTableTypeCustomizing, false, true, "Packaging types"},
	"warehouses":          {"MASTER", model.DDTableTypeMaster, true, true, "Warehouses, tanks and silos"},
	"warehouse_materials": {"MASTER", model.DDTableTypeMaster, false, true, "Materials allowed per storage location"},
	"production_lines":    {"MASTER", model.DDTableTypeMaster, true, true, "Production lines"},
	"processes":           {"MASTER", model.DDTableTypeCustomizing, false, true, "Production processes"},
	"process_materials":   {"MASTER", model.DDTableTypeCustomizing, false, true, "Process input and output materials"},
	"movement_types":      {"MASTER", model.DDTableTypeCustomizing, false, true, "Inventory movement types"},
	"seasons":             {"MASTER", model.DDTableTypeMaster, true, true, "Crushing seasons"},
	"planning_versions":   {"PLANNING", model.DDTableTypeTransaction, true, true, "Planning versions"},
	"plan_headers":        {"PLANNING", model.DDTableTypeTransaction, true, true, "Planning documents"},
	"plan_items":          {"PLANNING", model.DDTableTypeTransaction, false, true, "Planning document items"},
	"actual_headers":      {"ACTUAL", model.DDTableTypeTransaction, true, true, "Actual production documents"},
	"actual_items":        {"ACTUAL", model.DDTableTypeTransaction, false, true, "Actual production document items"},
	"inventory_movements": {"INVENTORY", model.DDTableTypeTransaction, true, true, "Inventory movement ledger"},
	"inventory_balances":  {"INVENTORY", model.DDTableTypeTransaction, true, true, "Materialised stock balances"},
	"number_ranges":       {"TECHNICAL", model.DDTableTypeCustomizing, true, true, "Document number ranges"},
	"audit_log":           {"TECHNICAL", model.DDTableTypeTechnical, true, false, "Change history"},
	"dd_domains":          {"TECHNICAL", model.DDTableTypeTechnical, false, true, "Data dictionary domains"},
	"dd_tables":           {"TECHNICAL", model.DDTableTypeTechnical, false, true, "Data dictionary tables"},
	"dd_fields":           {"TECHNICAL", model.DDTableTypeTechnical, false, true, "Data dictionary fields"},
	"dd_value_helps":      {"TECHNICAL", model.DDTableTypeTechnical, false, true, "Data dictionary value helps"},
}

// piiFields are masked for browser users even on a browsable table.
var piiFields = map[string]bool{
	"password_hash": true,
	"email":         true,
	"token_hash":    true,
	"ip_address":    true,
}

func (r *dataDictionaryRepository) ListTables(ctx context.Context, module, search string, browsableOnly bool) ([]model.DDTable, error) {
	q := database.Conn(ctx, r.db).Model(&model.DDTable{}).Where("is_active")
	if module != "" {
		q = q.Where("module = ?", module)
	}
	if browsableOnly {
		q = q.Where("is_browsable")
	}
	if search != "" {
		q = q.Where("(table_name ILIKE ? OR description_en ILIKE ?)", "%"+search+"%", "%"+search+"%")
	}
	var rows []model.DDTable
	err := q.Order("module, table_name").Find(&rows).Error
	return rows, translate(err)
}

func (r *dataDictionaryRepository) FindTable(ctx context.Context, tableName string) (*model.DDTable, error) {
	return findOne[model.DDTable](ctx, database.Conn(ctx, r.db), "table_name = ?", tableName)
}

// UpdateTable maintains the human-written description only. The structural
// columns are owned by the generator and are not writable through the API.
func (r *dataDictionaryRepository) UpdateTable(ctx context.Context, table *model.DDTable) error {
	res := database.Conn(ctx, r.db).Model(&model.DDTable{}).
		Where("id = ? AND version = ?", table.ID, table.Version).
		Updates(map[string]any{
			"description_en":       table.DescriptionEN,
			"description_local":    table.DescriptionLocal,
			"business_description": table.BusinessDescription,
			"is_browsable":         table.IsBrowsable,
			"version":              gorm.Expr("version + 1"),
			"changed_by":           table.ChangedBy,
			"changed_at":           gorm.Expr("now()"),
		})
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.ErrStaleRecord
	}
	table.Version++
	return nil
}

func (r *dataDictionaryRepository) Fields(ctx context.Context, tableID int64) ([]model.DDField, error) {
	var rows []model.DDField
	err := database.Conn(ctx, r.db).
		Where("table_id = ? AND is_active", tableID).
		Order("position").Find(&rows).Error
	return rows, translate(err)
}

func (r *dataDictionaryRepository) FindField(ctx context.Context, id int64) (*model.DDField, error) {
	return findOne[model.DDField](ctx, database.Conn(ctx, r.db), "id = ?", id)
}

func (r *dataDictionaryRepository) UpdateField(ctx context.Context, field *model.DDField) error {
	res := database.Conn(ctx, r.db).Model(&model.DDField{}).
		Where("id = ? AND version = ?", field.ID, field.Version).
		Updates(map[string]any{
			"label_short":          field.LabelShort,
			"label_medium":         field.LabelMedium,
			"label_long":           field.LabelLong,
			"business_description": field.BusinessDescription,
			"is_pii":               field.IsPII,
			"is_browsable":         field.IsBrowsable,
			"value_help_id":        field.ValueHelpID,
			"version":              gorm.Expr("version + 1"),
			"changed_by":           field.ChangedBy,
			"changed_at":           gorm.Expr("now()"),
		})
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.ErrStaleRecord
	}
	field.Version++
	return nil
}

func (r *dataDictionaryRepository) ListDomains(ctx context.Context) ([]model.DDDomain, error) {
	var rows []model.DDDomain
	err := database.Conn(ctx, r.db).Where("is_active").Order("domain_name").Find(&rows).Error
	return rows, translate(err)
}

func (r *dataDictionaryRepository) FindValueHelp(ctx context.Context, id int64) (*model.DDValueHelp, error) {
	return findOne[model.DDValueHelp](ctx, database.Conn(ctx, r.db), "id = ?", id)
}

// SyncFromInformationSchema regenerates the dictionary from the live catalogue
// (§G1). Human-maintained text — labels and business descriptions — is never
// overwritten, so running the generator on every deploy is safe.
func (r *dataDictionaryRepository) SyncFromInformationSchema(ctx context.Context) (int, error) {
	conn := database.Conn(ctx, r.db)

	type columnInfo struct {
		TableName  string
		ColumnName string
		Position   int
		DataType   string
		MaxLength  *int
		Precision  *int
		Scale      *int
		IsNullable string
		IsKey      bool
	}

	var cols []columnInfo
	err := conn.Raw(`
		SELECT c.table_name, c.column_name, c.ordinal_position AS position,
		       c.data_type, c.character_maximum_length AS max_length,
		       c.numeric_precision AS precision, c.numeric_scale AS scale,
		       c.is_nullable,
		       COALESCE(k.is_key, FALSE) AS is_key
		  FROM information_schema.columns c
		  LEFT JOIN (
		        SELECT kcu.table_name, kcu.column_name, TRUE AS is_key
		          FROM information_schema.table_constraints tc
		          JOIN information_schema.key_column_usage kcu
		            ON kcu.constraint_name = tc.constraint_name
		           AND kcu.table_schema = tc.table_schema
		         WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_schema = 'public'
		       ) k ON k.table_name = c.table_name AND k.column_name = c.column_name
		 WHERE c.table_schema = 'public'
		   AND c.table_name NOT IN ('schema_migrations')
		 ORDER BY c.table_name, c.ordinal_position`).Scan(&cols).Error
	if err != nil {
		return 0, translate(err)
	}

	tableIDs := make(map[string]int64)
	synced := 0

	for _, col := range cols {
		tableID, ok := tableIDs[col.TableName]
		if !ok {
			profile, known := tableProfiles[col.TableName]
			if !known {
				// A physical table with no profile is registered as technical
				// and not browsable, so a forgotten table can never leak.
				profile = tableProfile{Module: "TECHNICAL", TableType: model.DDTableTypeTechnical}
			}
			if err := conn.Exec(`
				INSERT INTO dd_tables (table_name, module, table_type, description_en,
				                       is_company_dependent, is_browsable)
				VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT (table_name) DO UPDATE
				   SET module = EXCLUDED.module,
				       table_type = EXCLUDED.table_type,
				       is_company_dependent = EXCLUDED.is_company_dependent,
				       is_browsable = EXCLUDED.is_browsable,
				       description_en = COALESCE(dd_tables.description_en, EXCLUDED.description_en),
				       changed_at = now()`,
				col.TableName, profile.Module, profile.TableType, nullIfEmpty(profile.Description),
				profile.CompanyDependent, profile.Browsable).Error; err != nil {
				return synced, translate(err)
			}

			var id int64
			if err := conn.Raw(`SELECT id FROM dd_tables WHERE table_name = ?`, col.TableName).
				Scan(&id).Error; err != nil {
				return synced, translate(err)
			}
			tableIDs[col.TableName] = id
			tableID = id
		}

		length := col.MaxLength
		if length == nil {
			length = col.Precision
		}

		if err := conn.Exec(`
			INSERT INTO dd_fields (table_id, field_name, position, data_type, length, decimals,
			                       is_key, is_required, is_pii, label_medium)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (table_id, field_name) DO UPDATE
			   SET position = EXCLUDED.position,
			       data_type = EXCLUDED.data_type,
			       length = EXCLUDED.length,
			       decimals = EXCLUDED.decimals,
			       is_key = EXCLUDED.is_key,
			       is_required = EXCLUDED.is_required,
			       is_pii = dd_fields.is_pii OR EXCLUDED.is_pii,
			       label_medium = COALESCE(dd_fields.label_medium, EXCLUDED.label_medium),
			       changed_at = now()`,
			tableID, col.ColumnName, col.Position, col.DataType, length, col.Scale,
			col.IsKey, col.IsNullable == "NO", piiFields[col.ColumnName],
			defaultLabel(col.ColumnName)).Error; err != nil {
			return synced, translate(err)
		}
		synced++
	}

	return synced, nil
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// defaultLabel turns snake_case into a readable label, giving the dictionary a
// usable starting point that a business user can then refine.
func defaultLabel(column string) string {
	words := strings.Split(column, "_")
	for i, w := range words {
		switch w {
		case "id":
			words[i] = "ID"
		case "uom":
			words[i] = "UoM"
		case "no":
			words[i] = "No."
		case "pct":
			words[i] = "%"
		default:
			if w != "" {
				words[i] = strings.ToUpper(w[:1]) + w[1:]
			}
		}
	}
	label := strings.Join(words, " ")
	if len(label) > 40 {
		label = label[:40]
	}
	return label
}

// --- data browser (§G2) --------------------------------------------------

type browserRepository struct {
	db *gorm.DB
}

func NewBrowserRepository(db *gorm.DB) interfaces.BrowserRepository {
	return &browserRepository{db: db}
}

// Query builds the SELECT from already-validated metadata. The table name and
// every field name were checked against dd_tables / dd_fields by the service;
// every value is a bound parameter, so no caller-supplied text is ever
// concatenated into the statement.
func (r *browserRepository) Query(ctx context.Context, q interfaces.BrowserQuery) (interfaces.BrowserResult, error) {
	var result interfaces.BrowserResult

	if len(q.Fields) == 0 {
		return result, apperrors.ErrValidation.Msgf("no readable field selected")
	}

	quoted := make([]string, 0, len(q.Fields))
	for _, f := range q.Fields {
		quoted = append(quoted, quoteIdent(f))
	}

	where := []string{"TRUE"}
	args := []any{}

	// Rule 5: for a company-dependent table the company filter is injected
	// here and there is no code path by which the caller can remove it.
	if len(q.CompanyIDs) > 0 {
		where = append(where, "company_id IN ?")
		args = append(args, q.CompanyIDs)
	}

	for _, f := range q.Filters {
		clause, clauseArgs, err := buildPredicate(interfaces.Filter{
			Field:    quoteIdent(f.Field),
			Operator: f.Operator,
			Values:   f.Values,
		})
		if err != nil {
			return result, err
		}
		where = append(where, clause)
		args = append(args, clauseArgs...)
	}

	order := ""
	if len(q.Sort) > 0 {
		parts := make([]string, 0, len(q.Sort))
		for _, s := range q.Sort {
			direction := " ASC"
			if s.Descending {
				direction = " DESC"
			}
			parts = append(parts, quoteIdent(s.Field)+direction)
		}
		order = " ORDER BY " + strings.Join(parts, ", ")
	}

	conn := database.Conn(ctx, r.db)

	// A per-query statement timeout keeps one careless selection from tying up
	// a connection (rule 7). SET LOCAL needs a transaction to be scoped to.
	err := conn.Transaction(func(tx *gorm.DB) error {
		if q.Timeout > 0 {
			ms := int(q.Timeout.Milliseconds())
			if err := tx.Exec(fmt.Sprintf("SET LOCAL statement_timeout = %d", ms)).Error; err != nil {
				return err
			}
		}

		countSQL := "SELECT COUNT(*) FROM " + quoteIdent(q.TableName) +
			" WHERE " + strings.Join(where, " AND ")
		if err := tx.Raw(countSQL, args...).Scan(&result.Total).Error; err != nil {
			return err
		}

		limit := q.Size
		if q.RowCap > 0 && limit > q.RowCap {
			limit = q.RowCap
		}
		offset := (q.Page - 1) * q.Size
		if offset < 0 {
			offset = 0
		}

		dataSQL := "SELECT " + strings.Join(quoted, ", ") +
			" FROM " + quoteIdent(q.TableName) +
			" WHERE " + strings.Join(where, " AND ") + order +
			" LIMIT ? OFFSET ?"

		rows, err := tx.Raw(dataSQL, append(append([]any{}, args...), limit, offset)...).Rows()
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			record := map[string]any{}
			if err := tx.ScanRows(rows, &record); err != nil {
				return err
			}
			result.Rows = append(result.Rows, record)
		}
		return rows.Err()
	})
	if err != nil {
		return result, translate(err)
	}

	result.Fields = q.Fields
	return result, nil
}

// quoteIdent double-quotes an identifier that has already been whitelisted
// against the data dictionary. The embedded-quote escape is belt and braces:
// no validated identifier can contain one.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
