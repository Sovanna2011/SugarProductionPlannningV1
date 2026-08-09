-- =====================================================================
-- Demo cane supply data (DEV / SIT only).
--
-- Two supply sources, because the difference is the point of the module:
-- the company's own estates, and purchased cane bought in from
-- out-growers and harvesting contractors under a priced contract.
-- =====================================================================

INSERT INTO cane_varieties (variety_code, variety_name, maturity_months, typical_ccs_pct, description) VALUES
    ('K88-92',  'Khon Kaen 88-92',   11, 13.8000, 'High yield, good ratooning, the estate workhorse'),
    ('KPS01',   'Kasetsart 01',      12, 14.6000, 'High CCS, prefers irrigated land'),
    ('LK92-11', 'Lam Kok 92-11',     10, 12.9000, 'Early maturing, opens the season'),
    ('VMC86',   'VMC 86-550',        13, 14.2000, 'Late maturing, drought tolerant'),
    ('F156',    'Formosa 156',       12, 13.1000, 'Older variety, still grown by smallholders')
ON CONFLICT (variety_code) DO NOTHING;

-- ---------------------------------------------------------------------
-- Growers
-- ---------------------------------------------------------------------
INSERT INTO growers
    (company_id, grower_code, grower_name, grower_type, supply_type, zone,
     contact_name, contact_phone, contract_tons, contract_no, price_per_ton, currency, transport_km)
SELECT c.id, g.code, g.name, g.gtype, g.supply, g.zone,
       g.contact, g.phone, g.tons, g.contract, g.price, g.currency, g.km
FROM (VALUES
    ('1000', 'EST-01', 'Company Estate North',       'ESTATE',     'OWN_ESTATE', 'North',
     'Estate Office',     '+855 12 000 101', NULL::numeric,   NULL,        NULL::numeric,  NULL,  8.5::numeric),
    ('1000', 'EST-02', 'Company Estate River',       'ESTATE',     'OWN_ESTATE', 'River',
     'Estate Office',     '+855 12 000 102', NULL::numeric,   NULL,        NULL::numeric,  NULL,  12.0::numeric),
    ('1000', 'OG-101', 'Sok Thida Farm',             'OUT_GROWER', 'PURCHASED',  'North',
     'Sok Thida',         '+855 12 111 201', 18000::numeric,  'CN-1000-01', 31.5000::numeric, 'USD', 22.0::numeric),
    ('1000', 'OG-102', 'Chan Dara Plantation',       'OUT_GROWER', 'PURCHASED',  'East',
     'Chan Dara',         '+855 12 111 202', 24000::numeric,  'CN-1000-02', 32.2500::numeric, 'USD', 31.5::numeric),
    ('1000', 'OG-103', 'Mekong Smallholder Group',   'OUT_GROWER', 'PURCHASED',  'River',
     'Group Secretary',   '+855 12 111 203', 9000::numeric,   'CN-1000-03', 30.0000::numeric, 'USD', 44.0::numeric),
    ('1000', 'CT-201', 'Angkor Harvest Contractors', 'CONTRACTOR', 'PURCHASED',  'South',
     'Operations Desk',   '+855 12 111 301', 12000::numeric,  'CN-1000-04', 33.7500::numeric, 'USD', 18.0::numeric),
    ('2000', 'EST-01', 'North Plant Estate',         'ESTATE',     'OWN_ESTATE', 'Plant',
     'Estate Office',     '+855 12 000 201', NULL::numeric,   NULL,        NULL::numeric,  NULL,  5.0::numeric),
    ('2000', 'OG-101', 'Battambang Cane Co-op',      'OUT_GROWER', 'PURCHASED',  'West',
     'Co-op Chair',       '+855 12 222 201', 15000::numeric,  'CN-2000-01', 30.5000::numeric, 'USD', 27.0::numeric)
) AS g(company_code, code, name, gtype, supply, zone, contact, phone, tons, contract, price, currency, km)
JOIN companies c ON c.company_code = g.company_code
ON CONFLICT (company_id, grower_code) DO NOTHING;

-- ---------------------------------------------------------------------
-- Cane fields
-- ---------------------------------------------------------------------
INSERT INTO cane_fields
    (company_id, grower_id, variety_id, field_code, field_name, area_ha,
     expected_yield_tph, crop_cycle, zone, is_irrigated)
SELECT c.id, g.id, v.id, f.code, f.name, f.area, f.yield, f.cycle, f.zone, f.irrigated
FROM (VALUES
    ('1000', 'EST-01', 'K88-92',  'FLD-001', 'North Block A',      120.000::numeric, 78.000::numeric, 'PLANT',        'North', TRUE),
    ('1000', 'EST-01', 'KPS01',   'FLD-002', 'North Block B',       95.500::numeric, 82.000::numeric, 'RATOON_1',     'North', TRUE),
    ('1000', 'EST-02', 'K88-92',  'FLD-003', 'River Block A',      140.000::numeric, 71.000::numeric, 'RATOON_2',     'River', FALSE),
    ('1000', 'OG-101', 'LK92-11', 'FLD-101', 'Thida Plot 1',        45.000::numeric, 64.000::numeric, 'PLANT',        'North', FALSE),
    ('1000', 'OG-101', 'K88-92',  'FLD-102', 'Thida Plot 2',        38.250::numeric, 61.500::numeric, 'RATOON_1',     'North', FALSE),
    ('1000', 'OG-102', 'VMC86',   'FLD-111', 'Dara East Field',     88.000::numeric, 69.000::numeric, 'PLANT',        'East',  TRUE),
    ('1000', 'OG-102', 'KPS01',   'FLD-112', 'Dara South Field',    52.750::numeric, 74.000::numeric, 'RATOON_1',     'East',  FALSE),
    ('1000', 'OG-103', 'F156',    'FLD-121', 'Mekong Group Plots',  61.000::numeric, 55.000::numeric, 'RATOON_3',     'River', FALSE),
    ('1000', 'CT-201', 'VMC86',   'FLD-131', 'Southern Concession',110.000::numeric, 66.000::numeric, 'RATOON_1',     'South', FALSE),
    ('2000', 'EST-01', 'K88-92',  'FLD-001', 'Plant Estate Block',  75.000::numeric, 70.000::numeric, 'PLANT',        'Plant', TRUE),
    ('2000', 'OG-101', 'LK92-11', 'FLD-101', 'Co-op Consolidated',  96.000::numeric, 58.000::numeric, 'RATOON_2',     'West',  FALSE)
) AS f(company_code, grower_code, variety_code, code, name, area, yield, cycle, zone, irrigated)
JOIN companies     c ON c.company_code = f.company_code
JOIN growers       g ON g.company_id = c.id AND g.grower_code = f.grower_code
LEFT JOIN cane_varieties v ON v.variety_code = f.variety_code
ON CONFLICT (company_id, field_code) DO NOTHING;

-- ---------------------------------------------------------------------
-- Cane yards — where a posted delivery lands. The yard is production
-- storage: cane is weighed in and milled within the day, so its capacity
-- is the tipping area rather than a stock limit.
-- ---------------------------------------------------------------------
INSERT INTO warehouses
    (company_id, warehouse_code, warehouse_name, warehouse_type, capacity, capacity_uom_id, location)
SELECT c.id, w.code, w.name, 'PRODUCTION_STORAGE', w.capacity, u.id, w.location
FROM (VALUES
    ('1000', 'CANEYARD', 'Cane Yard',       12000::numeric, 'TON', 'Mill Area'),
    ('2000', 'CANEYARD', 'Cane Yard North',  6000::numeric, 'TON', 'Mill Area')
) AS w(company_code, code, name, capacity, uom, location)
JOIN companies c ON c.company_code = w.company_code
JOIN uoms      u ON u.uom_code = w.uom
ON CONFLICT (company_id, warehouse_code) DO NOTHING;

INSERT INTO warehouse_materials (warehouse_id, material_id)
SELECT w.id, m.id
FROM warehouses w
JOIN materials  m ON m.material_code = 'SUGARCANE'
WHERE w.warehouse_code = 'CANEYARD'
ON CONFLICT (warehouse_id, material_id) DO NOTHING;
