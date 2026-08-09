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
