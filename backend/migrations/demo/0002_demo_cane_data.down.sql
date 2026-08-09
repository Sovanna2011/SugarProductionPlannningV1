-- Reverse of 0002_demo_cane_data.up.sql
DELETE FROM warehouse_materials
 WHERE warehouse_id IN (SELECT id FROM warehouses WHERE warehouse_code = 'CANEYARD');
DELETE FROM warehouses WHERE warehouse_code = 'CANEYARD';
DELETE FROM cane_fields;
DELETE FROM growers;
DELETE FROM cane_varieties;
