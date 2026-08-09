-- Reverse of 0004_cane_supply.up.sql
DROP TRIGGER IF EXISTS trg_cane_delivery_company_guard        ON cane_deliveries;
DROP TRIGGER IF EXISTS trg_harvest_plan_headers_status_guard  ON harvest_plan_headers;
DROP TRIGGER IF EXISTS trg_harvest_plan_items_status_guard    ON harvest_plan_items;
DROP FUNCTION IF EXISTS fn_delivery_company_guard();
DROP FUNCTION IF EXISTS fn_harvest_plan_status_guard();

DELETE FROM number_ranges WHERE object_type IN ('HARVEST','DELIVERY');
DELETE FROM role_permissions
 WHERE permission_id IN (SELECT id FROM permissions
                          WHERE module = 'CANE' OR permission_code = 'REPORT.CANE.VIEW');
DELETE FROM permissions WHERE module = 'CANE' OR permission_code = 'REPORT.CANE.VIEW';

DROP TABLE IF EXISTS cane_deliveries;
DROP TABLE IF EXISTS harvest_plan_items;
DROP TABLE IF EXISTS harvest_plan_headers;
DROP TABLE IF EXISTS cane_fields;
DROP TABLE IF EXISTS growers;
DROP TABLE IF EXISTS cane_varieties;
