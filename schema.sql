-- GENERATED FILE — do not edit.
-- Concatenation of backend/migrations/*.up.sql, produced by `make schema`.
-- The migrations are the source of truth; this file is the readable
-- companion the specification refers to.


-- ===== 0001_core_schema.up.sql =====
-- =====================================================================
-- Sugar Production Planning V1 — core schema
-- Technical Specification Part B (B1–B4)
--
-- Conventions (B1):
--   * tables snake_case, plural
--   * surrogate PK  : id BIGSERIAL              (OQ-1 default: BIGINT)
--   * audit columns : is_active, version, created_by/at, changed_by/at
--   * quantities    : NUMERIC(18,3)
--   * percentages   : NUMERIC(9,4)
--   * timestamps    : TIMESTAMPTZ, server clock only (never from client)
--   * business dates: DATE
--   * deletion      : logical for master data, reversal for documents
-- =====================================================================

CREATE EXTENSION IF NOT EXISTS btree_gist;

-- ---------------------------------------------------------------------
-- SECURITY
-- ---------------------------------------------------------------------

CREATE TABLE companies (
    id                  BIGSERIAL PRIMARY KEY,
    company_code        VARCHAR(10)  NOT NULL UNIQUE,
    company_name        VARCHAR(200) NOT NULL,
    local_currency      CHAR(3)      NOT NULL DEFAULT 'USD',
    group_currency      CHAR(3)      NOT NULL DEFAULT 'USD',
    timezone            VARCHAR(64)  NOT NULL DEFAULT 'UTC',
    fiscal_year_variant VARCHAR(10),
    country_code        CHAR(2),
    is_active           BOOLEAN      NOT NULL DEFAULT TRUE,
    version             INTEGER      NOT NULL DEFAULT 1,
    created_by          BIGINT,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    changed_by          BIGINT,
    changed_at          TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id                    BIGSERIAL PRIMARY KEY,
    username              VARCHAR(60)  NOT NULL UNIQUE,
    email                 VARCHAR(200) NOT NULL UNIQUE,
    full_name             VARCHAR(200) NOT NULL,
    password_hash         VARCHAR(255) NOT NULL,
    is_locked             BOOLEAN      NOT NULL DEFAULT FALSE,
    failed_login_count    INTEGER      NOT NULL DEFAULT 0,
    must_change_password  BOOLEAN      NOT NULL DEFAULT FALSE,
    last_login_at         TIMESTAMPTZ,
    locale                VARCHAR(10)  NOT NULL DEFAULT 'en',
    is_active             BOOLEAN      NOT NULL DEFAULT TRUE,
    version               INTEGER      NOT NULL DEFAULT 1,
    created_by            BIGINT,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT now(),
    changed_by            BIGINT,
    changed_at            TIMESTAMPTZ  NOT NULL DEFAULT now()
);
-- §10: a company_id column on users is explicitly forbidden — the link is
-- user_companies, so one login can serve many companies.

CREATE TABLE roles (
    id          BIGSERIAL PRIMARY KEY,
    role_code   VARCHAR(40)  NOT NULL UNIQUE,
    role_name   VARCHAR(120) NOT NULL,
    description TEXT,
    is_active   BOOLEAN      NOT NULL DEFAULT TRUE,
    version     INTEGER      NOT NULL DEFAULT 1,
    created_by  BIGINT,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    changed_by  BIGINT,
    changed_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE permissions (
    id              BIGSERIAL PRIMARY KEY,
    permission_code VARCHAR(80)  NOT NULL UNIQUE,   -- <MODULE>.<OBJECT>.<ACTION>
    module          VARCHAR(40)  NOT NULL,
    description     TEXT,
    is_active       BOOLEAN      NOT NULL DEFAULT TRUE,
    version         INTEGER      NOT NULL DEFAULT 1,
    created_by      BIGINT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    changed_by      BIGINT,
    changed_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE role_permissions (
    id            BIGSERIAL PRIMARY KEY,
    role_id       BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id BIGINT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    is_active     BOOLEAN     NOT NULL DEFAULT TRUE,
    version       INTEGER     NOT NULL DEFAULT 1,
    created_by    BIGINT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by    BIGINT,
    changed_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (role_id, permission_id)
);

CREATE TABLE user_companies (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id)     ON DELETE CASCADE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    is_default BOOLEAN     NOT NULL DEFAULT FALSE,
    is_active  BOOLEAN     NOT NULL DEFAULT TRUE,
    version    INTEGER     NOT NULL DEFAULT 1,
    created_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by BIGINT,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, company_id)
);
-- at most one default company per user
CREATE UNIQUE INDEX ux_user_default_company
    ON user_companies(user_id) WHERE is_default AND is_active;

CREATE TABLE user_company_roles (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id)     ON DELETE CASCADE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    role_id    BIGINT NOT NULL REFERENCES roles(id)     ON DELETE CASCADE,
    is_active  BOOLEAN     NOT NULL DEFAULT TRUE,
    version    INTEGER     NOT NULL DEFAULT 1,
    created_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by BIGINT,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, company_id, role_id)
);
-- §13: this table is what realises "same user, different role per company"
CREATE INDEX ix_ucr_user_company ON user_company_roles(user_id, company_id);

CREATE TABLE refresh_tokens (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    jti        VARCHAR(64)  NOT NULL,
    expires_at TIMESTAMPTZ  NOT NULL,
    revoked_at TIMESTAMPTZ,
    replaced_by BIGINT      REFERENCES refresh_tokens(id),
    user_agent VARCHAR(255),
    ip_address VARCHAR(64),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX ix_refresh_tokens_user ON refresh_tokens(user_id, expires_at);

-- ---------------------------------------------------------------------
-- MASTER DATA
-- ---------------------------------------------------------------------

CREATE TABLE uoms (
    id          BIGSERIAL PRIMARY KEY,
    uom_code    VARCHAR(10) NOT NULL UNIQUE,
    uom_name    VARCHAR(60) NOT NULL,
    dimension   VARCHAR(20) NOT NULL DEFAULT 'MASS',  -- MASS | VOLUME | ENERGY | COUNT
    decimals    INTEGER     NOT NULL DEFAULT 3,
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
    version     INTEGER     NOT NULL DEFAULT 1,
    created_by  BIGINT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by  BIGINT,
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- conversion to the dimension's base unit: qty_base = qty * numerator / denominator
CREATE TABLE uom_conversions (
    id            BIGSERIAL PRIMARY KEY,
    from_uom_id   BIGINT NOT NULL REFERENCES uoms(id),
    to_uom_id     BIGINT NOT NULL REFERENCES uoms(id),
    numerator     NUMERIC(18,6) NOT NULL CHECK (numerator   > 0),
    denominator   NUMERIC(18,6) NOT NULL CHECK (denominator > 0),
    is_active     BOOLEAN     NOT NULL DEFAULT TRUE,
    version       INTEGER     NOT NULL DEFAULT 1,
    created_by    BIGINT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by    BIGINT,
    changed_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (from_uom_id, to_uom_id)
);

-- §14 / OQ-2: materials are cross-company master data
CREATE TABLE materials (
    id                    BIGSERIAL PRIMARY KEY,
    material_code         VARCHAR(40)  NOT NULL UNIQUE,
    material_name         VARCHAR(200) NOT NULL,
    material_type         VARCHAR(20)  NOT NULL
        CHECK (material_type IN ('RAW','SEMI_FINISHED','FINISHED','BY_PRODUCT','UTILITY')),
    material_group        VARCHAR(40),
    base_uom_id           BIGINT       NOT NULL REFERENCES uoms(id),
    conditioning_required BOOLEAN      NOT NULL DEFAULT FALSE,  -- §F7 routing rule
    is_stock_managed      BOOLEAN      NOT NULL DEFAULT TRUE,   -- OQ-8: electricity = false
    is_active             BOOLEAN      NOT NULL DEFAULT TRUE,
    version               INTEGER      NOT NULL DEFAULT 1,
    created_by            BIGINT,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT now(),
    changed_by            BIGINT,
    changed_at            TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE company_materials (
    id                 BIGSERIAL PRIMARY KEY,
    company_id         BIGINT NOT NULL REFERENCES companies(id),
    material_id        BIGINT NOT NULL REFERENCES materials(id),
    planning_enabled   BOOLEAN NOT NULL DEFAULT TRUE,
    production_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    inventory_enabled  BOOLEAN NOT NULL DEFAULT TRUE,
    sales_enabled      BOOLEAN NOT NULL DEFAULT FALSE,
    is_active          BOOLEAN     NOT NULL DEFAULT TRUE,
    version            INTEGER     NOT NULL DEFAULT 1,
    created_by         BIGINT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by         BIGINT,
    changed_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, material_id)
);

CREATE TABLE packaging_types (
    id                  BIGSERIAL PRIMARY KEY,
    packaging_code      VARCHAR(20)  NOT NULL UNIQUE,
    packaging_name      VARCHAR(100) NOT NULL,
    nominal_quantity    NUMERIC(18,3),
    nominal_uom_id      BIGINT REFERENCES uoms(id),
    is_bulk             BOOLEAN     NOT NULL DEFAULT FALSE,
    is_active           BOOLEAN     NOT NULL DEFAULT TRUE,
    version             INTEGER     NOT NULL DEFAULT 1,
    created_by          BIGINT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by          BIGINT,
    changed_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE warehouses (
    id                   BIGSERIAL PRIMARY KEY,
    company_id           BIGINT NOT NULL REFERENCES companies(id),
    warehouse_code       VARCHAR(20)  NOT NULL,
    warehouse_name       VARCHAR(200) NOT NULL,
    warehouse_type       VARCHAR(30)  NOT NULL
        CHECK (warehouse_type IN ('WAREHOUSE','TANK','SILO','PRODUCTION_STORAGE')),
    capacity             NUMERIC(18,3) CHECK (capacity IS NULL OR capacity >= 0),
    capacity_uom_id      BIGINT REFERENCES uoms(id),
    allow_negative_stock BOOLEAN     NOT NULL DEFAULT FALSE,
    location             VARCHAR(200),
    is_active            BOOLEAN     NOT NULL DEFAULT TRUE,
    version              INTEGER     NOT NULL DEFAULT 1,
    created_by           BIGINT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by           BIGINT,
    changed_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, warehouse_code),
    -- capacity is only meaningful together with its unit
    CHECK (capacity IS NULL OR capacity_uom_id IS NOT NULL)
);

-- OQ-4 (recommended yes): restrict a tank/silo to specific materials
CREATE TABLE warehouse_materials (
    id           BIGSERIAL PRIMARY KEY,
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE CASCADE,
    material_id  BIGINT NOT NULL REFERENCES materials(id),
    is_active    BOOLEAN     NOT NULL DEFAULT TRUE,
    version      INTEGER     NOT NULL DEFAULT 1,
    created_by   BIGINT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by   BIGINT,
    changed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (warehouse_id, material_id)
);

CREATE TABLE production_lines (
    id           BIGSERIAL PRIMARY KEY,
    company_id   BIGINT NOT NULL REFERENCES companies(id),
    line_code    VARCHAR(20)  NOT NULL,
    line_name    VARCHAR(200) NOT NULL,
    capacity_per_day NUMERIC(18,3),
    capacity_uom_id  BIGINT REFERENCES uoms(id),
    is_active    BOOLEAN     NOT NULL DEFAULT TRUE,
    version      INTEGER     NOT NULL DEFAULT 1,
    created_by   BIGINT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by   BIGINT,
    changed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, line_code)
);

-- §19–§26 modelled as data, not code
CREATE TABLE processes (
    id             BIGSERIAL PRIMARY KEY,
    process_code   VARCHAR(30)  NOT NULL UNIQUE,
    process_name   VARCHAR(200) NOT NULL,
    sequence_no    INTEGER      NOT NULL DEFAULT 0,
    description    TEXT,
    is_active      BOOLEAN     NOT NULL DEFAULT TRUE,
    version        INTEGER     NOT NULL DEFAULT 1,
    created_by     BIGINT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by     BIGINT,
    changed_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE process_materials (
    id                    BIGSERIAL PRIMARY KEY,
    process_id            BIGINT NOT NULL REFERENCES processes(id) ON DELETE CASCADE,
    material_id           BIGINT NOT NULL REFERENCES materials(id),
    io_type               VARCHAR(10) NOT NULL CHECK (io_type IN ('INPUT','OUTPUT')),
    requires_conditioning BOOLEAN     NOT NULL DEFAULT FALSE,   -- §F7
    expected_yield_pct    NUMERIC(9,4),                          -- phase 2 yield variance
    is_active             BOOLEAN     NOT NULL DEFAULT TRUE,
    version               INTEGER     NOT NULL DEFAULT 1,
    created_by            BIGINT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by            BIGINT,
    changed_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (process_id, material_id, io_type)
);

CREATE TABLE movement_types (
    id                 BIGSERIAL PRIMARY KEY,
    movement_code      VARCHAR(30)  NOT NULL UNIQUE,
    movement_name      VARCHAR(200) NOT NULL,
    direction          VARCHAR(3)   NOT NULL CHECK (direction IN ('IN','OUT')),
    affects_stock      BOOLEAN      NOT NULL DEFAULT TRUE,
    is_planning_relevant BOOLEAN    NOT NULL DEFAULT TRUE,
    is_transfer        BOOLEAN      NOT NULL DEFAULT FALSE,
    is_adjustment      BOOLEAN      NOT NULL DEFAULT FALSE,
    counterpart_id     BIGINT REFERENCES movement_types(id),  -- TRANSFER_OUT ↔ TRANSFER_IN
    is_active          BOOLEAN     NOT NULL DEFAULT TRUE,
    version            INTEGER     NOT NULL DEFAULT 1,
    created_by         BIGINT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by         BIGINT,
    changed_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE seasons (
    id          BIGSERIAL PRIMARY KEY,
    company_id  BIGINT NOT NULL REFERENCES companies(id),
    season_code VARCHAR(20)  NOT NULL,
    season_name VARCHAR(200) NOT NULL,
    start_date  DATE NOT NULL,
    end_date    DATE NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'PLANNING'
        CHECK (status IN ('PLANNING','OPEN','CLOSED')),
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
    version     INTEGER     NOT NULL DEFAULT 1,
    created_by  BIGINT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by  BIGINT,
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, season_code),
    CHECK (end_date > start_date),
    -- §27: seasons of one company must not overlap
    EXCLUDE USING gist (
        company_id WITH =,
        daterange(start_date, end_date, '[]') WITH &&
    ) WHERE (is_active)
);

-- ---------------------------------------------------------------------
-- PLANNING
-- ---------------------------------------------------------------------

CREATE TABLE planning_versions (
    id                     BIGSERIAL PRIMARY KEY,
    company_id             BIGINT NOT NULL REFERENCES companies(id),
    season_id              BIGINT NOT NULL REFERENCES seasons(id),
    version_no             INTEGER      NOT NULL,      -- open-ended, never capped at 3
    version_name           VARCHAR(200) NOT NULL,
    description            TEXT,
    status                 VARCHAR(20)  NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT','SUBMITTED','APPROVED','LOCKED','CANCELLED')),
    copied_from_version_id BIGINT REFERENCES planning_versions(id),  -- lineage only
    submitted_by           BIGINT REFERENCES users(id),
    submitted_at           TIMESTAMPTZ,
    approved_by            BIGINT REFERENCES users(id),
    approved_at            TIMESTAMPTZ,
    locked_by              BIGINT REFERENCES users(id),
    locked_at              TIMESTAMPTZ,
    is_active              BOOLEAN     NOT NULL DEFAULT TRUE,
    version                INTEGER     NOT NULL DEFAULT 1,
    created_by             BIGINT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by             BIGINT,
    changed_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, season_id, version_no)
);

CREATE TABLE plan_headers (
    id                  BIGSERIAL PRIMARY KEY,
    company_id          BIGINT NOT NULL REFERENCES companies(id),
    season_id           BIGINT NOT NULL REFERENCES seasons(id),
    planning_version_id BIGINT NOT NULL REFERENCES planning_versions(id) ON DELETE CASCADE,
    movement_type_id    BIGINT NOT NULL REFERENCES movement_types(id),
    production_line_id  BIGINT REFERENCES production_lines(id),
    document_no         VARCHAR(40)  NOT NULL,
    description         VARCHAR(255),
    date_from           DATE NOT NULL,
    date_to             DATE NOT NULL,
    is_active           BOOLEAN     NOT NULL DEFAULT TRUE,
    version             INTEGER     NOT NULL DEFAULT 1,
    created_by          BIGINT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by          BIGINT,
    changed_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (date_to >= date_from),
    UNIQUE (company_id, document_no)
);
CREATE INDEX ix_plan_headers_lookup
    ON plan_headers(company_id, season_id, planning_version_id, movement_type_id);

CREATE TABLE plan_items (
    id                 BIGSERIAL PRIMARY KEY,
    plan_header_id     BIGINT NOT NULL REFERENCES plan_headers(id) ON DELETE CASCADE,
    plan_date          DATE   NOT NULL,
    production_line_id BIGINT REFERENCES production_lines(id),
    material_id        BIGINT NOT NULL REFERENCES materials(id),
    process_id         BIGINT REFERENCES processes(id),
    warehouse_id       BIGINT REFERENCES warehouses(id),
    packaging_type_id  BIGINT REFERENCES packaging_types(id),   -- OQ-3: nullable
    quantity           NUMERIC(18,3) NOT NULL CHECK (quantity >= 0),
    uom_id             BIGINT NOT NULL REFERENCES uoms(id),
    remark             VARCHAR(500),
    is_active          BOOLEAN     NOT NULL DEFAULT TRUE,
    version            INTEGER     NOT NULL DEFAULT 1,
    created_by         BIGINT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by         BIGINT,
    changed_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- the business key that makes the Date × Line matrix round-trippable (§28).
-- NULLS NOT DISTINCT so an unset line/process still collides as expected.
CREATE UNIQUE INDEX ux_plan_items_business_key
    ON plan_items (plan_header_id, plan_date, production_line_id, material_id, process_id)
    NULLS NOT DISTINCT;
CREATE INDEX ix_plan_items_header_date ON plan_items(plan_header_id, plan_date);
CREATE INDEX ix_plan_items_material_date ON plan_items(material_id, plan_date);

-- ---------------------------------------------------------------------
-- ACTUAL  (§32 — structurally parallel to planning, but independent:
--          deliberately NO foreign key to planning_versions)
-- ---------------------------------------------------------------------

CREATE TABLE actual_headers (
    id                 BIGSERIAL PRIMARY KEY,
    company_id         BIGINT NOT NULL REFERENCES companies(id),
    season_id          BIGINT REFERENCES seasons(id),
    movement_type_id   BIGINT NOT NULL REFERENCES movement_types(id),
    production_line_id BIGINT REFERENCES production_lines(id),
    document_no        VARCHAR(40) NOT NULL,
    description        VARCHAR(255),
    posting_date       DATE NOT NULL,
    posting_status     VARCHAR(20) NOT NULL DEFAULT 'DRAFT'
        CHECK (posting_status IN ('DRAFT','POSTED','REVERSED')),
    posted_by          BIGINT REFERENCES users(id),
    posted_at          TIMESTAMPTZ,
    reversed_by        BIGINT REFERENCES users(id),
    reversed_at        TIMESTAMPTZ,
    reversal_of_id     BIGINT REFERENCES actual_headers(id),
    idempotency_key    VARCHAR(80),
    is_active          BOOLEAN     NOT NULL DEFAULT TRUE,
    version            INTEGER     NOT NULL DEFAULT 1,
    created_by         BIGINT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by         BIGINT,
    changed_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, document_no)
);
CREATE UNIQUE INDEX ux_actual_headers_idempotency
    ON actual_headers(company_id, idempotency_key) WHERE idempotency_key IS NOT NULL;

CREATE TABLE actual_items (
    id                 BIGSERIAL PRIMARY KEY,
    actual_header_id   BIGINT NOT NULL REFERENCES actual_headers(id) ON DELETE CASCADE,
    actual_date        DATE   NOT NULL,
    production_line_id BIGINT REFERENCES production_lines(id),
    material_id        BIGINT NOT NULL REFERENCES materials(id),
    process_id         BIGINT REFERENCES processes(id),
    warehouse_id       BIGINT REFERENCES warehouses(id),
    packaging_type_id  BIGINT REFERENCES packaging_types(id),
    quantity           NUMERIC(18,3) NOT NULL CHECK (quantity >= 0),
    uom_id             BIGINT NOT NULL REFERENCES uoms(id),
    shift              VARCHAR(10),        -- OQ-7: optional, unused in phase 1
    remark             VARCHAR(500),
    is_active          BOOLEAN     NOT NULL DEFAULT TRUE,
    version            INTEGER     NOT NULL DEFAULT 1,
    created_by         BIGINT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by         BIGINT,
    changed_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_actual_items_header_date ON actual_items(actual_header_id, actual_date);
CREATE INDEX ix_actual_items_material    ON actual_items(material_id, actual_date);

-- ---------------------------------------------------------------------
-- INVENTORY
-- ---------------------------------------------------------------------

-- §33: append-only ledger. quantity is ALWAYS positive; the sign comes from
-- movement_types.direction.
CREATE TABLE inventory_movements (
    id                   BIGSERIAL PRIMARY KEY,
    company_id           BIGINT NOT NULL REFERENCES companies(id),
    document_no          VARCHAR(40) NOT NULL,
    line_no              INTEGER     NOT NULL DEFAULT 1,
    transaction_date     DATE   NOT NULL,
    material_id          BIGINT NOT NULL REFERENCES materials(id),
    warehouse_id         BIGINT NOT NULL REFERENCES warehouses(id),
    movement_type_id     BIGINT NOT NULL REFERENCES movement_types(id),
    packaging_type_id    BIGINT REFERENCES packaging_types(id),
    process_id           BIGINT REFERENCES processes(id),
    quantity             NUMERIC(18,3) NOT NULL CHECK (quantity > 0),
    uom_id               BIGINT NOT NULL REFERENCES uoms(id),
    reference_document   VARCHAR(40),
    reference_id         BIGINT,
    source_module        VARCHAR(20) NOT NULL
        CHECK (source_module IN ('ACTUAL','MANUAL','TRANSFER','OPENING','REVERSAL')),
    transfer_group_id    VARCHAR(40),
    reversed_movement_id BIGINT REFERENCES inventory_movements(id),
    is_reversed          BOOLEAN     NOT NULL DEFAULT FALSE,
    remark               VARCHAR(500),
    is_active            BOOLEAN     NOT NULL DEFAULT TRUE,
    version              INTEGER     NOT NULL DEFAULT 1,
    created_by           BIGINT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by           BIGINT,
    changed_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, document_no, line_no)
);
CREATE INDEX ix_inv_mov_company_date ON inventory_movements(company_id, transaction_date);
CREATE INDEX ix_inv_mov_stock_path
    ON inventory_movements(company_id, warehouse_id, material_id, transaction_date);
CREATE INDEX ix_inv_mov_document      ON inventory_movements(document_no);
CREATE INDEX ix_inv_mov_transfer      ON inventory_movements(transfer_group_id)
    WHERE transfer_group_id IS NOT NULL;

-- materialised balance (B3) — maintained incrementally on posting and
-- recomputed by the nightly reconciliation job.
CREATE TABLE inventory_balances (
    id                BIGSERIAL PRIMARY KEY,
    company_id        BIGINT NOT NULL REFERENCES companies(id),
    warehouse_id      BIGINT NOT NULL REFERENCES warehouses(id),
    material_id       BIGINT NOT NULL REFERENCES materials(id),
    packaging_type_id BIGINT REFERENCES packaging_types(id),
    balance_date      DATE   NOT NULL,
    opening_qty       NUMERIC(18,3) NOT NULL DEFAULT 0,
    in_qty            NUMERIC(18,3) NOT NULL DEFAULT 0,
    out_qty           NUMERIC(18,3) NOT NULL DEFAULT 0,
    closing_qty       NUMERIC(18,3) NOT NULL DEFAULT 0,
    uom_id            BIGINT REFERENCES uoms(id),
    version           INTEGER     NOT NULL DEFAULT 1,
    created_by        BIGINT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by        BIGINT,
    changed_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ux_inventory_balances_key
    ON inventory_balances (company_id, warehouse_id, material_id, packaging_type_id, balance_date)
    NULLS NOT DISTINCT;
CREATE INDEX ix_inventory_balances_date ON inventory_balances(company_id, balance_date);

-- ---------------------------------------------------------------------
-- TECHNICAL
-- ---------------------------------------------------------------------

CREATE TABLE number_ranges (
    id          BIGSERIAL PRIMARY KEY,
    company_id  BIGINT NOT NULL REFERENCES companies(id),
    object_type VARCHAR(30) NOT NULL,      -- PLAN | ACTUAL | INVENTORY | TRANSFER
    fiscal_year INTEGER     NOT NULL,
    prefix      VARCHAR(10) NOT NULL,
    current_no  BIGINT      NOT NULL DEFAULT 0,
    length      INTEGER     NOT NULL DEFAULT 6,
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
    version     INTEGER     NOT NULL DEFAULT 1,
    created_by  BIGINT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by  BIGINT,
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, object_type, fiscal_year)
);

CREATE TABLE audit_log (
    id          BIGSERIAL PRIMARY KEY,
    table_name  VARCHAR(80) NOT NULL,
    record_id   BIGINT,
    company_id  BIGINT,
    action      VARCHAR(20) NOT NULL
        CHECK (action IN ('INSERT','UPDATE','DELETE','APPROVE','POST','REVERSE',
                          'LOGIN','LOGIN_FAIL','PERMISSION_DENIED','BROWSE','EXPORT')),
    changed_by  BIGINT,
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    old_values  JSONB,
    new_values  JSONB,
    request_id  VARCHAR(64),
    ip_address  VARCHAR(64)
);
CREATE INDEX ix_audit_record  ON audit_log(table_name, record_id);
CREATE INDEX ix_audit_company ON audit_log(company_id, changed_at);
CREATE INDEX ix_audit_changed_at_brin ON audit_log USING brin (changed_at);

-- ---------------------------------------------------------------------
-- DATA DICTIONARY (Part G)
-- ---------------------------------------------------------------------

CREATE TABLE dd_domains (
    id           BIGSERIAL PRIMARY KEY,
    domain_name  VARCHAR(60) NOT NULL UNIQUE,
    data_type    VARCHAR(30) NOT NULL,
    length       INTEGER,
    decimals     INTEGER,
    fixed_values JSONB,
    description  TEXT,
    is_active    BOOLEAN     NOT NULL DEFAULT TRUE,
    version      INTEGER     NOT NULL DEFAULT 1,
    created_by   BIGINT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by   BIGINT,
    changed_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE dd_value_helps (
    id              BIGSERIAL PRIMARY KEY,
    value_help_name VARCHAR(60) NOT NULL UNIQUE,
    help_type       VARCHAR(20) NOT NULL CHECK (help_type IN ('FIXED','TABLE')),
    fixed_values    JSONB,
    source_table    VARCHAR(80),
    key_field       VARCHAR(80),
    text_field      VARCHAR(80),
    is_active       BOOLEAN     NOT NULL DEFAULT TRUE,
    version         INTEGER     NOT NULL DEFAULT 1,
    created_by      BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by      BIGINT,
    changed_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE dd_tables (
    id                   BIGSERIAL PRIMARY KEY,
    table_name           VARCHAR(80) NOT NULL UNIQUE,
    module               VARCHAR(40) NOT NULL,
    table_type           VARCHAR(20) NOT NULL
        CHECK (table_type IN ('MASTER','TRANSACTION','CUSTOMIZING','TECHNICAL')),
    description_en       VARCHAR(255),
    description_local    VARCHAR(255),
    business_description TEXT,
    is_company_dependent BOOLEAN NOT NULL DEFAULT FALSE,
    is_browsable         BOOLEAN NOT NULL DEFAULT FALSE,
    is_active            BOOLEAN     NOT NULL DEFAULT TRUE,
    version              INTEGER     NOT NULL DEFAULT 1,
    created_by           BIGINT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by           BIGINT,
    changed_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE dd_fields (
    id                   BIGSERIAL PRIMARY KEY,
    table_id             BIGINT NOT NULL REFERENCES dd_tables(id) ON DELETE CASCADE,
    field_name           VARCHAR(80) NOT NULL,
    position             INTEGER     NOT NULL DEFAULT 0,
    label_short          VARCHAR(20),
    label_medium         VARCHAR(40),
    label_long           VARCHAR(100),
    domain_id            BIGINT REFERENCES dd_domains(id),
    value_help_id        BIGINT REFERENCES dd_value_helps(id),
    data_type            VARCHAR(30) NOT NULL,
    length               INTEGER,
    decimals             INTEGER,
    is_key               BOOLEAN NOT NULL DEFAULT FALSE,
    is_required          BOOLEAN NOT NULL DEFAULT FALSE,
    is_pii               BOOLEAN NOT NULL DEFAULT FALSE,
    is_browsable         BOOLEAN NOT NULL DEFAULT TRUE,
    business_description TEXT,
    is_active            BOOLEAN     NOT NULL DEFAULT TRUE,
    version              INTEGER     NOT NULL DEFAULT 1,
    created_by           BIGINT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by           BIGINT,
    changed_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (table_id, field_name)
);

-- ---------------------------------------------------------------------
-- §F3 — status guard as a database trigger, so that no code path
-- (including a future batch job) can write to a non-editable version.
-- ---------------------------------------------------------------------

CREATE OR REPLACE FUNCTION fn_plan_status_guard() RETURNS TRIGGER AS $$
DECLARE
    v_status TEXT;
    v_header BIGINT;
BEGIN
    IF TG_TABLE_NAME = 'plan_items' THEN
        v_header := COALESCE(NEW.plan_header_id, OLD.plan_header_id);
        SELECT pv.status INTO v_status
          FROM plan_headers ph
          JOIN planning_versions pv ON pv.id = ph.planning_version_id
         WHERE ph.id = v_header;
    ELSE
        SELECT pv.status INTO v_status
          FROM planning_versions pv
         WHERE pv.id = COALESCE(NEW.planning_version_id, OLD.planning_version_id);
    END IF;

    IF v_status IN ('APPROVED','LOCKED','CANCELLED') THEN
        RAISE EXCEPTION
            'E-PLAN-007: planning version is % and cannot be modified', v_status
            USING ERRCODE = 'raise_exception';
    END IF;

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_plan_items_status_guard
    BEFORE INSERT OR UPDATE OR DELETE ON plan_items
    FOR EACH ROW EXECUTE FUNCTION fn_plan_status_guard();

CREATE TRIGGER trg_plan_headers_status_guard
    BEFORE INSERT OR UPDATE OR DELETE ON plan_headers
    FOR EACH ROW EXECUTE FUNCTION fn_plan_status_guard();

-- §34 — an inventory movement may never join two companies. The warehouse
-- must belong to the same company as the movement.
CREATE OR REPLACE FUNCTION fn_movement_company_guard() RETURNS TRIGGER AS $$
DECLARE
    v_wh_company BIGINT;
BEGIN
    SELECT company_id INTO v_wh_company FROM warehouses WHERE id = NEW.warehouse_id;
    IF v_wh_company IS DISTINCT FROM NEW.company_id THEN
        RAISE EXCEPTION
            'E-VAL-010: cross-company reference — warehouse % does not belong to company %',
            NEW.warehouse_id, NEW.company_id
            USING ERRCODE = 'raise_exception';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_inv_mov_company_guard
    BEFORE INSERT OR UPDATE ON inventory_movements
    FOR EACH ROW EXECUTE FUNCTION fn_movement_company_guard();

-- ===== 0002_reference_data.up.sql =====
-- =====================================================================
-- Reference / customizing data.
-- Applied in every environment (DEV → PRD): this is configuration, not
-- demo data. Demo data lives in 0003 and is DEV/SIT only.
-- =====================================================================

-- ---------------------------------------------------------------------
-- Permissions  (<MODULE>.<OBJECT>.<ACTION>)
-- ---------------------------------------------------------------------
INSERT INTO permissions (permission_code, module, description) VALUES
    ('MASTER.COMPANY.VIEW',        'MASTER',  'Display companies'),
    ('MASTER.COMPANY.EDIT',        'MASTER',  'Maintain companies'),
    ('MASTER.MATERIAL.VIEW',       'MASTER',  'Display materials'),
    ('MASTER.MATERIAL.EDIT',       'MASTER',  'Maintain materials and company assignment'),
    ('MASTER.WAREHOUSE.VIEW',      'MASTER',  'Display warehouses, tanks and silos'),
    ('MASTER.WAREHOUSE.EDIT',      'MASTER',  'Maintain warehouses, tanks and silos'),
    ('MASTER.PRODUCTIONLINE.VIEW', 'MASTER',  'Display production lines'),
    ('MASTER.PRODUCTIONLINE.EDIT', 'MASTER',  'Maintain production lines'),
    ('MASTER.PACKAGING.VIEW',      'MASTER',  'Display packaging types'),
    ('MASTER.PACKAGING.EDIT',      'MASTER',  'Maintain packaging types'),
    ('MASTER.PROCESS.VIEW',        'MASTER',  'Display production processes'),
    ('MASTER.PROCESS.EDIT',        'MASTER',  'Maintain production processes and their materials'),
    ('MASTER.MOVEMENTTYPE.VIEW',   'MASTER',  'Display movement types'),
    ('MASTER.MOVEMENTTYPE.EDIT',   'MASTER',  'Maintain movement types'),
    ('MASTER.UOM.VIEW',            'MASTER',  'Display units of measure'),
    ('MASTER.UOM.EDIT',            'MASTER',  'Maintain units of measure'),
    ('MASTER.SEASON.VIEW',         'MASTER',  'Display seasons'),
    ('MASTER.SEASON.EDIT',         'MASTER',  'Maintain seasons'),

    ('PLAN.VERSION.VIEW',          'PLAN',    'Display planning versions'),
    ('PLAN.VERSION.CREATE',        'PLAN',    'Create a planning version'),
    ('PLAN.VERSION.EDIT',          'PLAN',    'Change planning version attributes'),
    ('PLAN.VERSION.COPY',          'PLAN',    'Copy a planning version'),
    ('PLAN.VERSION.SUBMIT',        'PLAN',    'Submit a planning version for approval'),
    ('PLAN.VERSION.APPROVE',       'PLAN',    'Approve a planning version'),
    ('PLAN.VERSION.LOCK',          'PLAN',    'Lock an approved planning version'),
    ('PLAN.VERSION.CANCEL',        'PLAN',    'Cancel a planning version'),
    ('PLAN.ITEM.VIEW',             'PLAN',    'Display plan documents and items'),
    ('PLAN.ITEM.EDIT',             'PLAN',    'Maintain plan documents and the planning matrix'),
    ('PLAN.EDIT.SUBMITTED',        'PLAN',    'Change plan data of a SUBMITTED version'),

    ('ACTUAL.VIEW',                'ACTUAL',  'Display actual production documents'),
    ('ACTUAL.CREATE',              'ACTUAL',  'Create actual production documents'),
    ('ACTUAL.EDIT',                'ACTUAL',  'Change draft actual production documents'),
    ('ACTUAL.POST',                'ACTUAL',  'Post an actual production document'),
    ('ACTUAL.REVERSE',             'ACTUAL',  'Reverse a posted actual production document'),

    ('INV.MOVEMENT.VIEW',          'INV',     'Display inventory movements'),
    ('INV.MOVEMENT.CREATE',        'INV',     'Create a manual inventory movement'),
    ('INV.MOVEMENT.REVERSE',       'INV',     'Reverse an inventory movement'),
    ('INV.TRANSFER.CREATE',        'INV',     'Create a warehouse transfer'),
    ('INV.ADJUSTMENT.CREATE',      'INV',     'Create an inventory adjustment'),
    ('INV.BALANCE.VIEW',           'INV',     'Display stock balances and capacity'),

    ('REPORT.PLANACTUAL.VIEW',     'REPORT',  'Display the plan vs actual report'),
    ('REPORT.PRODUCTION.VIEW',     'REPORT',  'Display the production summary report'),
    ('REPORT.INVENTORY.VIEW',      'REPORT',  'Display the inventory movement report'),
    ('REPORT.CAPACITY.VIEW',       'REPORT',  'Display the capacity utilisation report'),
    ('REPORT.EXPORT',              'REPORT',  'Export report results'),

    ('DD.VIEW',                    'DD',      'Display the data dictionary'),
    ('DD.MAINTAIN',                'DD',      'Maintain data dictionary descriptions'),
    ('BROWSER.VIEW',               'BROWSER', 'Use the data browser'),
    ('BROWSER.EXPORT',             'BROWSER', 'Export data browser results'),

    ('ADMIN.USER.MANAGE',          'ADMIN',   'Maintain users and their company assignments'),
    ('ADMIN.ROLE.MANAGE',          'ADMIN',   'Maintain roles and role permissions'),
    ('ADMIN.NUMBERRANGE.MANAGE',   'ADMIN',   'Maintain document number ranges'),
    ('ADMIN.AUDIT.VIEW',           'ADMIN',   'Display the audit log')
ON CONFLICT (permission_code) DO NOTHING;

-- ---------------------------------------------------------------------
-- Roles
-- ---------------------------------------------------------------------
INSERT INTO roles (role_code, role_name, description) VALUES
    ('ADMIN',      'System Administrator', 'Full technical and functional authorisation'),
    ('PLANNER',    'Production Planner',   'Creates and maintains planning versions and the planning matrix'),
    ('APPROVER',   'Planning Approver',    'Approves and locks planning versions'),
    ('OPERATOR',   'Production Operator',  'Records and posts actual production'),
    ('INVENTORY',  'Inventory Clerk',      'Maintains stock movements, transfers and adjustments'),
    ('VIEWER',     'Display User',         'Read-only access to master data, documents and reports')
ON CONFLICT (role_code) DO NOTHING;

-- Role → permission assignment, expressed as patterns so that adding a
-- permission to an existing family does not need a new migration.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.role_code = 'ADMIN'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.role_code = 'PLANNER'
  AND (p.permission_code LIKE '%.VIEW'
       OR p.permission_code IN ('PLAN.VERSION.CREATE','PLAN.VERSION.EDIT','PLAN.VERSION.COPY',
                                'PLAN.VERSION.SUBMIT','PLAN.VERSION.CANCEL','PLAN.ITEM.EDIT',
                                'REPORT.EXPORT'))
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.role_code = 'APPROVER'
  AND (p.permission_code LIKE '%.VIEW'
       OR p.permission_code IN ('PLAN.VERSION.APPROVE','PLAN.VERSION.LOCK','PLAN.VERSION.CANCEL',
                                'REPORT.EXPORT'))
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.role_code = 'OPERATOR'
  AND (p.permission_code LIKE '%.VIEW'
       OR p.permission_code IN ('ACTUAL.CREATE','ACTUAL.EDIT','ACTUAL.POST','ACTUAL.REVERSE'))
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.role_code = 'INVENTORY'
  AND (p.permission_code LIKE '%.VIEW'
       OR p.permission_code IN ('INV.MOVEMENT.CREATE','INV.MOVEMENT.REVERSE',
                                'INV.TRANSFER.CREATE','INV.ADJUSTMENT.CREATE'))
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.role_code = 'VIEWER' AND p.permission_code LIKE '%.VIEW'
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------
-- Units of measure
-- ---------------------------------------------------------------------
INSERT INTO uoms (uom_code, uom_name, dimension, decimals) VALUES
    ('TON', 'Metric Tonne', 'MASS',   3),
    ('KG',  'Kilogram',     'MASS',   3),
    ('L',   'Litre',        'VOLUME', 3),
    ('M3',  'Cubic Metre',  'VOLUME', 3),
    ('MWH', 'Megawatt Hour','ENERGY', 3),
    ('KWH', 'Kilowatt Hour','ENERGY', 3),
    ('BAG', 'Bag',          'COUNT',  0),
    ('PCS', 'Piece',        'COUNT',  0)
ON CONFLICT (uom_code) DO NOTHING;

INSERT INTO uom_conversions (from_uom_id, to_uom_id, numerator, denominator)
SELECT f.id, t.id, c.num, c.den
FROM (VALUES
    ('KG', 'TON', 1::numeric,    1000::numeric),
    ('TON','KG',  1000::numeric, 1::numeric),
    ('KWH','MWH', 1::numeric,    1000::numeric),
    ('MWH','KWH', 1000::numeric, 1::numeric),
    ('L',  'M3',  1::numeric,    1000::numeric),
    ('M3', 'L',   1000::numeric, 1::numeric)
) AS c(from_code, to_code, num, den)
JOIN uoms f ON f.uom_code = c.from_code
JOIN uoms t ON t.uom_code = c.to_code
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------
-- Movement types (§33 — direction drives the sign, never a hard-coded code)
-- ---------------------------------------------------------------------
INSERT INTO movement_types
    (movement_code, movement_name, direction, affects_stock, is_planning_relevant, is_transfer, is_adjustment) VALUES
    ('CANE_INTAKE',        'Sugarcane Intake',       'IN',  TRUE,  TRUE,  FALSE, FALSE),
    ('PRODUCTION_RECEIPT', 'Production Receipt',     'IN',  TRUE,  TRUE,  FALSE, FALSE),
    ('PRODUCTION_ISSUE',   'Production Issue',       'OUT', TRUE,  TRUE,  FALSE, FALSE),
    ('REMELT_ISSUE',       'Remelt Issue',           'OUT', TRUE,  TRUE,  FALSE, FALSE),
    ('TRANSFER_IN',        'Transfer In',            'IN',  TRUE,  FALSE, TRUE,  FALSE),
    ('TRANSFER_OUT',       'Transfer Out',           'OUT', TRUE,  FALSE, TRUE,  FALSE),
    ('SALE',               'Sale / Goods Issue',     'OUT', TRUE,  TRUE,  FALSE, FALSE),
    ('ADJUSTMENT_IN',      'Inventory Adjustment +', 'IN',  TRUE,  FALSE, FALSE, TRUE),
    ('ADJUSTMENT_OUT',     'Inventory Adjustment -', 'OUT', TRUE,  FALSE, FALSE, TRUE),
    ('OPENING_BALANCE',    'Opening Balance',        'IN',  TRUE,  FALSE, FALSE, FALSE),
    ('POWER_EXPORT',       'Electricity Export',     'OUT', FALSE, TRUE,  FALSE, FALSE)
ON CONFLICT (movement_code) DO NOTHING;

UPDATE movement_types m SET counterpart_id = c.id
FROM movement_types c
WHERE (m.movement_code, c.movement_code) IN
      (('TRANSFER_OUT','TRANSFER_IN'), ('TRANSFER_IN','TRANSFER_OUT'))
  AND m.counterpart_id IS DISTINCT FROM c.id;

-- ---------------------------------------------------------------------
-- Materials (§19–§26 / flow diagram)
-- ---------------------------------------------------------------------
INSERT INTO materials
    (material_code, material_name, material_type, material_group, base_uom_id,
     conditioning_required, is_stock_managed)
SELECT m.code, m.name, m.mtype, m.mgroup, u.id, m.conditioning, m.stock
FROM (VALUES
    ('SUGARCANE',   'Sugarcane',            'RAW',           'CANE',      'TON', FALSE, TRUE),
    ('RAW_SUGAR',   'Raw Sugar',            'SEMI_FINISHED', 'SUGAR',     'TON', FALSE, TRUE),
    ('MOLASSES',    'Molasses',             'BY_PRODUCT',    'MOLASSES',  'TON', FALSE, TRUE),
    ('BAGASSE',     'Bagasse',              'BY_PRODUCT',    'BIOMASS',   'TON', FALSE, TRUE),
    -- OQ-8: electricity in MWh, tracked as production quantity, not warehouse stock
    ('ELECTRICITY', 'Electricity',          'UTILITY',       'ENERGY',    'MWH', FALSE, FALSE),
    ('REMELT_LIQ',  'Remelt Liquor',        'SEMI_FINISHED', 'SUGAR',     'TON', FALSE, TRUE),
    -- §F7 routing rule: refined + super refined are conditioned in the silo …
    ('REFINED',     'Refined Sugar',        'FINISHED',      'SUGAR',     'TON', TRUE,  TRUE),
    ('SUPER_REF',   'Super Refined Sugar',  'FINISHED',      'SUGAR',     'TON', TRUE,  TRUE),
    -- … white sugar goes direct and never touches the silo
    ('WHITE',       'White Sugar',          'FINISHED',      'SUGAR',     'TON', FALSE, TRUE)
) AS m(code, name, mtype, mgroup, uom, conditioning, stock)
JOIN uoms u ON u.uom_code = m.uom
ON CONFLICT (material_code) DO NOTHING;

-- ---------------------------------------------------------------------
-- Packaging types
-- ---------------------------------------------------------------------
INSERT INTO packaging_types (packaging_code, packaging_name, nominal_quantity, nominal_uom_id, is_bulk)
SELECT p.code, p.name, p.qty, u.id, p.bulk
FROM (VALUES
    ('BAG50',  'Bag 50 kg',     50::numeric,   'KG',  FALSE),
    ('BAG25',  'Bag 25 kg',     25::numeric,   'KG',  FALSE),
    ('BAG10',  'Bag 10 kg',     10::numeric,   'KG',  FALSE),
    ('BAG1',   'Bag 1 kg',      1::numeric,    'KG',  FALSE),
    ('JUMBO',  'Jumbo Bag 1 t', 1000::numeric, 'KG',  FALSE),
    ('BULK',   'Bulk',          NULL::numeric, NULL,  TRUE)
) AS p(code, name, qty, uom, bulk)
LEFT JOIN uoms u ON u.uom_code = p.uom
ON CONFLICT (packaging_code) DO NOTHING;

-- ---------------------------------------------------------------------
-- Processes and their material network (§F7 — data, not code)
-- ---------------------------------------------------------------------
INSERT INTO processes (process_code, process_name, sequence_no, description) VALUES
    ('MILLING',      'Milling',      10, 'Cane is milled into raw sugar, molasses and bagasse'),
    ('POWER',        'Power Plant',  20, 'Bagasse is burned to generate electricity'),
    ('REMELT',       'Remelt',       30, 'Raw sugar is remelted into liquor'),
    ('REFINING',     'Refining',     40, 'Remelt liquor is refined into refined, super refined and white sugar'),
    ('CONDITIONING', 'Conditioning', 50, 'Refined and super refined sugar are conditioned in the silo'),
    ('PACKING',      'Packing',      60, 'Finished sugar is packed into its final packaging')
ON CONFLICT (process_code) DO NOTHING;

INSERT INTO process_materials (process_id, material_id, io_type, requires_conditioning)
SELECT pr.id, ma.id, x.io, x.cond
FROM (VALUES
    ('MILLING',      'SUGARCANE',   'INPUT',  FALSE),
    ('MILLING',      'RAW_SUGAR',   'OUTPUT', FALSE),
    ('MILLING',      'MOLASSES',    'OUTPUT', FALSE),
    ('MILLING',      'BAGASSE',     'OUTPUT', FALSE),
    ('POWER',        'BAGASSE',     'INPUT',  FALSE),
    ('POWER',        'ELECTRICITY', 'OUTPUT', FALSE),
    ('REMELT',       'RAW_SUGAR',   'INPUT',  FALSE),
    ('REMELT',       'REMELT_LIQ',  'OUTPUT', FALSE),
    ('REFINING',     'REMELT_LIQ',  'INPUT',  FALSE),
    ('REFINING',     'REFINED',     'OUTPUT', TRUE),
    ('REFINING',     'SUPER_REF',   'OUTPUT', TRUE),
    ('REFINING',     'WHITE',       'OUTPUT', FALSE),
    ('CONDITIONING', 'REFINED',     'INPUT',  TRUE),
    ('CONDITIONING', 'SUPER_REF',   'INPUT',  TRUE),
    ('CONDITIONING', 'REFINED',     'OUTPUT', TRUE),
    ('CONDITIONING', 'SUPER_REF',   'OUTPUT', TRUE),
    ('PACKING',      'REFINED',     'INPUT',  FALSE),
    ('PACKING',      'SUPER_REF',   'INPUT',  FALSE),
    ('PACKING',      'WHITE',       'INPUT',  FALSE),
    ('PACKING',      'REFINED',     'OUTPUT', FALSE),
    ('PACKING',      'SUPER_REF',   'OUTPUT', FALSE),
    ('PACKING',      'WHITE',       'OUTPUT', FALSE)
) AS x(process_code, material_code, io, cond)
JOIN processes  pr ON pr.process_code  = x.process_code
JOIN materials  ma ON ma.material_code = x.material_code
ON CONFLICT (process_id, material_id, io_type) DO NOTHING;

-- ===== 0003_demo_master_data.up.sql =====
-- =====================================================================
-- Demo master data (DEV / SIT only — guarded by MIGRATE_INCLUDE_DEMO).
-- Models the plant layout of the inventory / material flow diagram.
--
-- Demo *users* are not created here: password hashing (argon2id) belongs
-- to the application, so the bootstrapper in Go creates them instead.
-- =====================================================================

INSERT INTO companies (company_code, company_name, local_currency, group_currency, timezone, country_code) VALUES
    ('1000', 'Sugar Mill Main Plant',  'KHR', 'USD', 'Asia/Phnom_Penh', 'KH'),
    ('2000', 'Sugar Mill North',       'KHR', 'USD', 'Asia/Phnom_Penh', 'KH'),
    ('3000', 'Sugar Refinery Export',  'USD', 'USD', 'Asia/Singapore',  'SG')
ON CONFLICT (company_code) DO NOTHING;

-- every company is relevant for every material of the demo network
INSERT INTO company_materials
    (company_id, material_id, planning_enabled, production_enabled, inventory_enabled, sales_enabled)
SELECT c.id, m.id, TRUE, TRUE, m.is_stock_managed,
       m.material_type IN ('FINISHED','BY_PRODUCT')
FROM companies c
CROSS JOIN materials m
WHERE c.company_code IN ('1000','2000','3000')
ON CONFLICT (company_id, material_id) DO NOTHING;

-- ---------------------------------------------------------------------
-- Storage locations
-- ---------------------------------------------------------------------
INSERT INTO warehouses
    (company_id, warehouse_code, warehouse_name, warehouse_type, capacity, capacity_uom_id, location)
SELECT c.id, w.code, w.name, w.wtype, w.capacity, u.id, w.location
FROM (VALUES
    ('1000', 'MT1',     'Molasses Tank 1',        'TANK',               5000::numeric,  'TON', 'Tank Farm'),
    ('1000', 'MT2',     'Molasses Tank 2',        'TANK',               5000::numeric,  'TON', 'Tank Farm'),
    ('1000', 'MT3',     'Molasses Tank 3',        'TANK',               3000::numeric,  'TON', 'Tank Farm'),
    ('1000', 'RW1',     'Raw Sugar Warehouse 1',  'WAREHOUSE',          20000::numeric, 'TON', 'Mill Area'),
    ('1000', 'RW2',     'Raw Sugar Warehouse 2',  'WAREHOUSE',          15000::numeric, 'TON', 'Mill Area'),
    ('1000', 'SILO1',   'Condition Silo 1',       'SILO',               3000::numeric,  'TON', 'Refinery'),
    ('1000', 'SW1',     'Sugar Warehouse 1',      'WAREHOUSE',          10000::numeric, 'TON', 'Refinery'),
    ('1000', 'SW2',     'Sugar Warehouse 2',      'WAREHOUSE',          10000::numeric, 'TON', 'Refinery'),
    ('1000', 'SW3',     'Sugar Warehouse 3',      'WAREHOUSE',          8000::numeric,  'TON', 'Refinery'),
    ('1000', 'BAGYARD', 'Bagasse Yard',           'PRODUCTION_STORAGE', NULL::numeric,  NULL,  'Power Plant'),
    ('2000', 'MT1',     'Molasses Tank 1',        'TANK',               2000::numeric,  'TON', 'Tank Farm'),
    ('2000', 'RW1',     'Raw Sugar Warehouse 1',  'WAREHOUSE',          8000::numeric,  'TON', 'Mill Area'),
    ('2000', 'SW1',     'Sugar Warehouse 1',      'WAREHOUSE',          4000::numeric,  'TON', 'Mill Area'),
    ('3000', 'SILO1',   'Condition Silo 1',       'SILO',               2000::numeric,  'TON', 'Refinery'),
    ('3000', 'SW1',     'Sugar Warehouse 1',      'WAREHOUSE',          6000::numeric,  'TON', 'Refinery'),
    ('3000', 'SW2',     'Sugar Warehouse 2',      'WAREHOUSE',          6000::numeric,  'TON', 'Refinery')
) AS w(company_code, code, name, wtype, capacity, uom, location)
JOIN companies c ON c.company_code = w.company_code
LEFT JOIN uoms u ON u.uom_code = w.uom
ON CONFLICT (company_id, warehouse_code) DO NOTHING;

-- OQ-4: a tank holds molasses, a silo holds conditioned sugar — nothing else
INSERT INTO warehouse_materials (warehouse_id, material_id)
SELECT w.id, m.id
FROM (VALUES
    ('MT1',     'MOLASSES'),
    ('MT2',     'MOLASSES'),
    ('MT3',     'MOLASSES'),
    ('RW1',     'RAW_SUGAR'),
    ('RW2',     'RAW_SUGAR'),
    ('SILO1',   'REFINED'),
    ('SILO1',   'SUPER_REF'),
    ('SW1',     'REFINED'),
    ('SW2',     'WHITE'),
    ('SW3',     'SUPER_REF'),
    ('BAGYARD', 'BAGASSE')
) AS x(warehouse_code, material_code)
JOIN warehouses w ON w.warehouse_code = x.warehouse_code
JOIN materials  m ON m.material_code  = x.material_code
ON CONFLICT (warehouse_id, material_id) DO NOTHING;

-- ---------------------------------------------------------------------
-- Production lines
-- ---------------------------------------------------------------------
INSERT INTO production_lines (company_id, line_code, line_name, capacity_per_day, capacity_uom_id)
SELECT c.id, l.code, l.name, l.cap, u.id
FROM (VALUES
    ('1000', 'LINE1', 'Mill Line 1',      6000::numeric, 'TON'),
    ('1000', 'LINE2', 'Mill Line 2',      4500::numeric, 'TON'),
    ('1000', 'LINE3', 'Refinery Line 1',  1200::numeric, 'TON'),
    ('1000', 'LINE4', 'Refinery Line 2',  900::numeric,  'TON'),
    ('1000', 'LINE5', 'Packing Line 1',   800::numeric,  'TON'),
    ('2000', 'LINE1', 'Mill Line 1',      3000::numeric, 'TON'),
    ('2000', 'LINE2', 'Mill Line 2',      2500::numeric, 'TON'),
    ('3000', 'LINE1', 'Refinery Line 1',  1500::numeric, 'TON'),
    ('3000', 'LINE2', 'Packing Line 1',   1000::numeric, 'TON')
) AS l(company_code, code, name, cap, uom)
JOIN companies c ON c.company_code = l.company_code
JOIN uoms u ON u.uom_code = l.uom
ON CONFLICT (company_id, line_code) DO NOTHING;

-- ---------------------------------------------------------------------
-- Seasons  (non-overlapping per company — enforced by the exclusion constraint)
--
-- The open season is deliberately anchored to the installation date rather
-- than to fixed literals. A posting must fall inside an open season (§F4
-- rule 6), so a demo season with hard-coded dates would stop being usable the
-- moment the calendar moved past it.
-- ---------------------------------------------------------------------
INSERT INTO seasons (company_id, season_code, season_name, start_date, end_date, status)
SELECT c.id, s.code, s.name, s.sd, s.ed, s.status
FROM (VALUES
    ('PREVIOUS',
     'Previous Crushing Season',
     (date_trunc('year', now()) - INTERVAL '1 year')::date,
     (date_trunc('year', now()) - INTERVAL '1 day')::date,
     'CLOSED'),
    ('CURRENT',
     'Current Crushing Season',
     date_trunc('year', now())::date,
     (date_trunc('year', now()) + INTERVAL '1 year' - INTERVAL '1 day')::date,
     'OPEN')
) AS s(code, name, sd, ed, status)
CROSS JOIN companies c
WHERE c.company_code IN ('1000','2000','3000')
ON CONFLICT (company_id, season_code) DO NOTHING;

-- ---------------------------------------------------------------------
-- Number ranges (§35) — one per company / object / fiscal year
-- ---------------------------------------------------------------------
INSERT INTO number_ranges (company_id, object_type, fiscal_year, prefix, current_no, length)
SELECT c.id, o.object_type, y.fy, o.prefix, 0, 6
FROM companies c
CROSS JOIN (VALUES
    ('PLAN',      'PLAN'),
    ('ACTUAL',    'ACT'),
    ('INVENTORY', 'INV'),
    ('TRANSFER',  'TRF')
) AS o(object_type, prefix)
CROSS JOIN (VALUES (EXTRACT(YEAR FROM now())::int), (EXTRACT(YEAR FROM now())::int + 1)) AS y(fy)
WHERE c.company_code IN ('1000','2000','3000')
ON CONFLICT (company_id, object_type, fiscal_year) DO NOTHING;
