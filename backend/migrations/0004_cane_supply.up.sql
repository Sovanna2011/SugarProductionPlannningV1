-- =====================================================================
-- Sugarcane supply: growers, fields, the harvest plan and the deliveries
-- that are its actual.
--
-- The mill plan answers "what will each line produce"; this answers the
-- question upstream of it — "where does the cane come from, on which day,
-- and how much of it actually arrived". It reuses what already exists
-- rather than duplicating it:
--
--   * the harvest plan hangs off planning_versions, so one version carries
--     both the cane plan and the production plan through one status
--     machine and one approval;
--   * a posted delivery writes an ordinary inventory movement, so cane
--     stock, capacity and the ledger need no special case;
--   * the same audit, optimistic-lock and logical-delete columns apply.
--
-- Version numbering note: 0003 was the demo master data, which now lives
-- in its own migration source (migrations/demo). The number is retired
-- rather than reused so that a database migrated before the split fails
-- loudly instead of silently skipping this migration.
-- =====================================================================

-- ---------------------------------------------------------------------
-- Cane varieties — cross-company master data, like materials (§14)
-- ---------------------------------------------------------------------
CREATE TABLE cane_varieties (
    id                BIGSERIAL PRIMARY KEY,
    variety_code      VARCHAR(30)  NOT NULL UNIQUE,
    variety_name      VARCHAR(200) NOT NULL,
    maturity_months   INTEGER      CHECK (maturity_months IS NULL OR maturity_months > 0),
    -- Commercial cane sugar: the recoverable sugar per tonne of cane,
    -- expressed as a percentage. NULL means "not maintained", never zero.
    typical_ccs_pct   NUMERIC(9,4) CHECK (typical_ccs_pct IS NULL OR (typical_ccs_pct >= 0 AND typical_ccs_pct <= 100)),
    description       TEXT,
    is_active         BOOLEAN     NOT NULL DEFAULT TRUE,
    version           INTEGER     NOT NULL DEFAULT 1,
    created_by        BIGINT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by        BIGINT,
    changed_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------
-- Growers — company-dependent master data
-- ---------------------------------------------------------------------
CREATE TABLE growers (
    id              BIGSERIAL PRIMARY KEY,
    company_id      BIGINT NOT NULL REFERENCES companies(id),
    grower_code     VARCHAR(20)  NOT NULL,
    grower_name     VARCHAR(200) NOT NULL,
    grower_type     VARCHAR(20)  NOT NULL DEFAULT 'OUT_GROWER'
        CHECK (grower_type IN ('ESTATE','OUT_GROWER','CONTRACTOR')),
    -- Own cane comes off the company's own estates; purchased cane is bought
    -- in under a supply contract. The two are planned in the same matrix but
    -- reported apart, because only purchased cane carries a cane bill.
    supply_type     VARCHAR(20)  NOT NULL DEFAULT 'PURCHASED'
        CHECK (supply_type IN ('OWN_ESTATE','PURCHASED')),
    zone            VARCHAR(60),
    contact_name    VARCHAR(200),
    contact_phone   VARCHAR(40),
    -- The tonnage the grower is contracted to deliver this season. NULL is
    -- "no contract quantity agreed" — it is not a zero commitment.
    contract_tons   NUMERIC(18,3) CHECK (contract_tons IS NULL OR contract_tons >= 0),
    contract_no     VARCHAR(40),
    -- Contract price per tonne of cane. NULL means "not agreed yet", which is
    -- why a purchased-cane value is reported as unavailable rather than zero.
    price_per_ton   NUMERIC(18,4) CHECK (price_per_ton IS NULL OR price_per_ton >= 0),
    currency        CHAR(3),
    transport_km    NUMERIC(9,2)  CHECK (transport_km IS NULL OR transport_km >= 0),
    is_active       BOOLEAN     NOT NULL DEFAULT TRUE,
    version         INTEGER     NOT NULL DEFAULT 1,
    created_by      BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by      BIGINT,
    changed_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, grower_code),
    -- referenced by the composite foreign keys below, which is how a
    -- cane document is stopped from joining two companies (§C2 / E-VAL-010)
    UNIQUE (id, company_id)
);
CREATE INDEX ix_growers_company_zone ON growers(company_id, zone);
CREATE INDEX ix_growers_supply_type ON growers(company_id, supply_type);

-- ---------------------------------------------------------------------
-- Cane fields (plots) — the unit a harvest is actually planned on
-- ---------------------------------------------------------------------
CREATE TABLE cane_fields (
    id                    BIGSERIAL PRIMARY KEY,
    company_id            BIGINT NOT NULL REFERENCES companies(id),
    grower_id             BIGINT NOT NULL,
    variety_id            BIGINT REFERENCES cane_varieties(id),
    field_code            VARCHAR(20)  NOT NULL,
    field_name            VARCHAR(200) NOT NULL,
    area_ha               NUMERIC(12,3) NOT NULL CHECK (area_ha > 0),
    -- Expected yield in tonnes per hectare. With the area above it gives the
    -- tonnage the planner starts from; NULL means the field has no yield
    -- history yet and the planner enters tonnes directly.
    expected_yield_tph    NUMERIC(12,3) CHECK (expected_yield_tph IS NULL OR expected_yield_tph > 0),
    crop_cycle            VARCHAR(20) NOT NULL DEFAULT 'PLANT'
        CHECK (crop_cycle IN ('PLANT','RATOON_1','RATOON_2','RATOON_3','RATOON_4_PLUS')),
    planting_date         DATE,
    expected_harvest_from DATE,
    expected_harvest_to   DATE,
    zone                  VARCHAR(60),
    is_irrigated          BOOLEAN     NOT NULL DEFAULT FALSE,
    is_active             BOOLEAN     NOT NULL DEFAULT TRUE,
    version               INTEGER     NOT NULL DEFAULT 1,
    created_by            BIGINT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by            BIGINT,
    changed_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (expected_harvest_to IS NULL OR expected_harvest_from IS NULL
           OR expected_harvest_to >= expected_harvest_from),
    UNIQUE (company_id, field_code),
    UNIQUE (id, company_id),
    FOREIGN KEY (grower_id, company_id) REFERENCES growers(id, company_id)
);
CREATE INDEX ix_cane_fields_grower ON cane_fields(company_id, grower_id);

-- ---------------------------------------------------------------------
-- Harvest plan — the Date × Grower matrix, inside a planning version
-- ---------------------------------------------------------------------
CREATE TABLE harvest_plan_headers (
    id                  BIGSERIAL PRIMARY KEY,
    company_id          BIGINT NOT NULL REFERENCES companies(id),
    season_id           BIGINT NOT NULL REFERENCES seasons(id),
    planning_version_id BIGINT NOT NULL REFERENCES planning_versions(id) ON DELETE CASCADE,
    document_no         VARCHAR(40) NOT NULL,
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
    UNIQUE (company_id, document_no),
    -- one harvest document per version: the matrix spans every grower, so
    -- there is nothing to split it by
    UNIQUE (company_id, planning_version_id)
);

CREATE TABLE harvest_plan_items (
    id                     BIGSERIAL PRIMARY KEY,
    harvest_plan_header_id BIGINT NOT NULL REFERENCES harvest_plan_headers(id) ON DELETE CASCADE,
    plan_date              DATE   NOT NULL,
    grower_id              BIGINT NOT NULL REFERENCES growers(id),
    cane_field_id          BIGINT REFERENCES cane_fields(id),
    variety_id             BIGINT REFERENCES cane_varieties(id),
    planned_tons           NUMERIC(18,3) NOT NULL CHECK (planned_tons >= 0),
    uom_id                 BIGINT NOT NULL REFERENCES uoms(id),
    planned_area_ha        NUMERIC(12,3) CHECK (planned_area_ha IS NULL OR planned_area_ha >= 0),
    expected_ccs_pct       NUMERIC(9,4)  CHECK (expected_ccs_pct IS NULL OR (expected_ccs_pct >= 0 AND expected_ccs_pct <= 100)),
    remark                 VARCHAR(500),
    is_active              BOOLEAN     NOT NULL DEFAULT TRUE,
    version                INTEGER     NOT NULL DEFAULT 1,
    created_by             BIGINT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by             BIGINT,
    changed_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- The business key of the Date × Grower grid. NULLS NOT DISTINCT so that a
-- row planned on the grower without naming a field still collides with
-- itself on replay, exactly as the production matrix does (§28).
CREATE UNIQUE INDEX ux_harvest_plan_items_business_key
    ON harvest_plan_items (harvest_plan_header_id, plan_date, grower_id, cane_field_id)
    NULLS NOT DISTINCT;
CREATE INDEX ix_harvest_plan_items_date ON harvest_plan_items(harvest_plan_header_id, plan_date);

-- ---------------------------------------------------------------------
-- Cane deliveries — the actual against the harvest plan
--
-- A weighbridge ticket. Posting one writes an ordinary inventory movement
-- for the net weight, so cane behaves like any other stock.
-- ---------------------------------------------------------------------
CREATE TABLE cane_deliveries (
    id                BIGSERIAL PRIMARY KEY,
    company_id        BIGINT NOT NULL REFERENCES companies(id),
    season_id         BIGINT REFERENCES seasons(id),
    document_no       VARCHAR(40) NOT NULL,
    delivery_date     DATE   NOT NULL,
    grower_id         BIGINT NOT NULL,
    cane_field_id     BIGINT,
    variety_id        BIGINT REFERENCES cane_varieties(id),
    material_id       BIGINT NOT NULL REFERENCES materials(id),
    movement_type_id  BIGINT NOT NULL REFERENCES movement_types(id),
    warehouse_id      BIGINT REFERENCES warehouses(id),
    uom_id            BIGINT NOT NULL REFERENCES uoms(id),
    ticket_no         VARCHAR(40),
    vehicle_no        VARCHAR(30),
    gross_tons        NUMERIC(18,3) NOT NULL CHECK (gross_tons > 0),
    tare_tons         NUMERIC(18,3) NOT NULL DEFAULT 0 CHECK (tare_tons >= 0),
    -- Net weight is derived, never entered: a stored generated column means
    -- no code path can leave it disagreeing with gross and tare.
    net_tons          NUMERIC(18,3) GENERATED ALWAYS AS (gross_tons - tare_tons) STORED,
    -- The contract price is copied from the grower when the ticket is
    -- recorded, not read through a join: re-negotiating a contract must not
    -- rewrite what an already-delivered load was worth.
    price_per_ton     NUMERIC(18,4) CHECK (price_per_ton IS NULL OR price_per_ton >= 0),
    currency          CHAR(3),
    ccs_pct           NUMERIC(9,4) CHECK (ccs_pct IS NULL OR (ccs_pct >= 0 AND ccs_pct <= 100)),
    trash_pct         NUMERIC(9,4) CHECK (trash_pct IS NULL OR (trash_pct >= 0 AND trash_pct <= 100)),
    is_burnt          BOOLEAN     NOT NULL DEFAULT FALSE,
    posting_status    VARCHAR(20) NOT NULL DEFAULT 'DRAFT'
        CHECK (posting_status IN ('DRAFT','POSTED','REVERSED')),
    posted_by         BIGINT REFERENCES users(id),
    posted_at         TIMESTAMPTZ,
    reversed_by       BIGINT REFERENCES users(id),
    reversed_at       TIMESTAMPTZ,
    idempotency_key   VARCHAR(80),
    remark            VARCHAR(500),
    is_active         BOOLEAN     NOT NULL DEFAULT TRUE,
    version           INTEGER     NOT NULL DEFAULT 1,
    created_by        BIGINT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    changed_by        BIGINT,
    changed_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (gross_tons > tare_tons),
    UNIQUE (company_id, document_no),
    FOREIGN KEY (grower_id, company_id)     REFERENCES growers(id, company_id),
    FOREIGN KEY (cane_field_id, company_id) REFERENCES cane_fields(id, company_id)
);
CREATE INDEX ix_cane_deliveries_date   ON cane_deliveries(company_id, delivery_date);
CREATE INDEX ix_cane_deliveries_grower ON cane_deliveries(company_id, grower_id, delivery_date);
-- A weighbridge ticket number is unique per company where it is given at all.
CREATE UNIQUE INDEX ux_cane_deliveries_ticket
    ON cane_deliveries(company_id, ticket_no) WHERE ticket_no IS NOT NULL;
CREATE UNIQUE INDEX ux_cane_deliveries_idempotency
    ON cane_deliveries(company_id, idempotency_key) WHERE idempotency_key IS NOT NULL;

-- ---------------------------------------------------------------------
-- §F3 status guard for the harvest plan — the same rule and the same error
-- code as the production plan, so no code path can write cane data into an
-- approved or locked version either.
-- ---------------------------------------------------------------------
CREATE OR REPLACE FUNCTION fn_harvest_plan_status_guard() RETURNS TRIGGER AS $$
DECLARE
    v_status TEXT;
BEGIN
    IF TG_TABLE_NAME = 'harvest_plan_items' THEN
        SELECT pv.status INTO v_status
          FROM harvest_plan_headers hh
          JOIN planning_versions pv ON pv.id = hh.planning_version_id
         WHERE hh.id = COALESCE(NEW.harvest_plan_header_id, OLD.harvest_plan_header_id);
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

CREATE TRIGGER trg_harvest_plan_items_status_guard
    BEFORE INSERT OR UPDATE OR DELETE ON harvest_plan_items
    FOR EACH ROW EXECUTE FUNCTION fn_harvest_plan_status_guard();

CREATE TRIGGER trg_harvest_plan_headers_status_guard
    BEFORE INSERT OR UPDATE OR DELETE ON harvest_plan_headers
    FOR EACH ROW EXECUTE FUNCTION fn_harvest_plan_status_guard();

-- A delivery may never send cane into another company's yard.
CREATE OR REPLACE FUNCTION fn_delivery_company_guard() RETURNS TRIGGER AS $$
DECLARE
    v_wh_company BIGINT;
BEGIN
    IF NEW.warehouse_id IS NULL THEN
        RETURN NEW;
    END IF;
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

CREATE TRIGGER trg_cane_delivery_company_guard
    BEFORE INSERT OR UPDATE ON cane_deliveries
    FOR EACH ROW EXECUTE FUNCTION fn_delivery_company_guard();

-- ---------------------------------------------------------------------
-- Permissions (<MODULE>.<OBJECT>.<ACTION>)
-- ---------------------------------------------------------------------
INSERT INTO permissions (permission_code, module, description) VALUES
    ('CANE.VARIETY.VIEW',   'CANE',   'Display cane varieties'),
    ('CANE.VARIETY.EDIT',   'CANE',   'Maintain cane varieties'),
    ('CANE.GROWER.VIEW',    'CANE',   'Display growers'),
    ('CANE.GROWER.EDIT',    'CANE',   'Maintain growers'),
    ('CANE.FIELD.VIEW',     'CANE',   'Display cane fields'),
    ('CANE.FIELD.EDIT',     'CANE',   'Maintain cane fields'),
    ('CANE.PLAN.VIEW',      'CANE',   'Display the harvest plan'),
    ('CANE.PLAN.EDIT',      'CANE',   'Maintain the harvest plan matrix'),
    ('CANE.DELIVERY.VIEW',  'CANE',   'Display cane deliveries'),
    ('CANE.DELIVERY.CREATE','CANE',   'Record a cane delivery'),
    ('CANE.DELIVERY.EDIT',  'CANE',   'Change a draft cane delivery'),
    ('CANE.DELIVERY.POST',  'CANE',   'Post a cane delivery to stock'),
    ('CANE.DELIVERY.REVERSE','CANE',  'Reverse a posted cane delivery'),
    ('REPORT.CANE.VIEW',    'REPORT', 'Display the cane plan vs actual report')
ON CONFLICT (permission_code) DO NOTHING;

-- The role → permission statements of 0002 are re-run so that the families
-- they describe pick up the permissions added above. They are expressed as
-- patterns for exactly this reason.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.role_code = 'ADMIN'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.role_code = 'PLANNER'
  AND (p.permission_code LIKE '%.VIEW'
       OR p.permission_code IN ('CANE.PLAN.EDIT','CANE.GROWER.EDIT','CANE.FIELD.EDIT',
                                'CANE.VARIETY.EDIT'))
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.role_code IN ('APPROVER','INVENTORY','VIEWER')
  AND p.permission_code LIKE '%.VIEW'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.role_code = 'OPERATOR'
  AND (p.permission_code LIKE '%.VIEW'
       OR p.permission_code IN ('CANE.DELIVERY.CREATE','CANE.DELIVERY.EDIT',
                                'CANE.DELIVERY.POST','CANE.DELIVERY.REVERSE'))
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------
-- Number ranges for the two new document types, for every company that
-- already has ranges maintained (§35).
-- ---------------------------------------------------------------------
INSERT INTO number_ranges (company_id, object_type, fiscal_year, prefix, current_no, length)
SELECT DISTINCT nr.company_id, o.object_type, nr.fiscal_year, o.prefix, 0, 6
FROM number_ranges nr
CROSS JOIN (VALUES
    ('HARVEST',  'HARV'),
    ('DELIVERY', 'CANE')
) AS o(object_type, prefix)
ON CONFLICT (company_id, object_type, fiscal_year) DO NOTHING;
