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
