DROP INDEX IF EXISTS idx_gateways_honeypot_region;
ALTER TABLE gateways DROP COLUMN IF EXISTS is_honeypot;
