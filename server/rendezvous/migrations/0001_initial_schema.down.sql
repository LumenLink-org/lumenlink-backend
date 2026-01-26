-- Rollback migration: 0001_initial_schema.down.sql
-- Drops all tables and functions created in the initial schema

-- Drop triggers first
DROP TRIGGER IF EXISTS log_gateway_status_change ON gateways;
DROP TRIGGER IF EXISTS update_gateways_updated_at ON gateways;

-- Drop functions
DROP FUNCTION IF EXISTS log_gateway_status_change();
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS rate_limits;
DROP TABLE IF EXISTS gateway_status_history;
DROP TABLE IF EXISTS discovery_logs;
DROP TABLE IF EXISTS operator_metrics;
DROP TABLE IF EXISTS attestations;
DROP TABLE IF EXISTS config_packs;
DROP TABLE IF EXISTS gateways;

-- Note: We don't drop uuid-ossp extension as it might be used by other databases
