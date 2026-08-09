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
