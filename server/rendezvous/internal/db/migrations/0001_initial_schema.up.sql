-- LumenLink Initial Database Schema
-- Migration: 0001_initial_schema.up.sql
-- Description: Creates all core tables for gateways, config packs, attestations, and logs

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Gateways table
-- Stores information about relay servers/gateways
CREATE TABLE gateways (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    public_key BYTEA NOT NULL UNIQUE,
    ip_address INET NOT NULL,
    port INTEGER NOT NULL CHECK (port > 0 AND port <= 65535),
    transport_types TEXT[] NOT NULL DEFAULT '{}',
    discovery_channels TEXT[] NOT NULL DEFAULT '{}',
    region VARCHAR(10) NOT NULL,
    bandwidth_mbps INTEGER CHECK (bandwidth_mbps > 0),
    current_users INTEGER DEFAULT 0 CHECK (current_users >= 0),
    max_users INTEGER CHECK (max_users > 0),
    status VARCHAR(20) DEFAULT 'active' CHECK (status IN ('active', 'degraded', 'offline', 'maintenance')),
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    last_seen TIMESTAMPTZ,
    updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for gateways
CREATE INDEX idx_gateways_region ON gateways(region);
CREATE INDEX idx_gateways_status ON gateways(status);
CREATE INDEX idx_gateways_last_seen ON gateways(last_seen);
CREATE INDEX idx_gateways_public_key ON gateways USING hash(public_key);
CREATE INDEX idx_gateways_region_status ON gateways(region, status);

-- Config Packs table
-- Stores signed configuration packs distributed to clients
CREATE TABLE config_packs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version INTEGER NOT NULL CHECK (version > 0),
    region VARCHAR(10) NOT NULL,
    content JSONB NOT NULL,
    signature BYTEA NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    expires_at TIMESTAMPTZ,
    created_by VARCHAR(255) -- Operator/admin who created this pack
);

-- Indexes for config_packs
CREATE INDEX idx_config_packs_region_version ON config_packs(region, version DESC);
CREATE INDEX idx_config_packs_expires_at ON config_packs(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX idx_config_packs_content ON config_packs USING gin(content);

-- Attestations table
-- Stores device attestation records (Play Integrity, DCAppAttest)
CREATE TABLE attestations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id VARCHAR(255) NOT NULL,
    platform VARCHAR(20) NOT NULL CHECK (platform IN ('android', 'ios', 'desktop')),
    token TEXT NOT NULL,
    verified BOOLEAN DEFAULT FALSE NOT NULL,
    verified_at TIMESTAMPTZ,
    device_integrity VARCHAR(50), -- e.g., 'MEETS_STRONG_INTEGRITY'
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    expires_at TIMESTAMPTZ
);

-- Indexes for attestations
CREATE INDEX idx_attestations_device ON attestations(device_id);
CREATE INDEX idx_attestations_verified ON attestations(verified);
CREATE INDEX idx_attestations_platform ON attestations(platform);
CREATE INDEX idx_attestations_device_platform ON attestations(device_id, platform);
CREATE INDEX idx_attestations_expires_at ON attestations(expires_at) WHERE expires_at IS NOT NULL;

-- Operator Metrics table (will be converted to hypertable in next migration)
-- Time-series data for gateway performance metrics
CREATE TABLE operator_metrics (
    time TIMESTAMPTZ NOT NULL,
    gateway_id UUID NOT NULL REFERENCES gateways(id) ON DELETE CASCADE,
    users_connected INTEGER DEFAULT 0 CHECK (users_connected >= 0),
    bandwidth_used_mbps INTEGER DEFAULT 0 CHECK (bandwidth_used_mbps >= 0),
    packets_forwarded BIGINT DEFAULT 0 CHECK (packets_forwarded >= 0),
    uptime_percent DECIMAL(5,2) CHECK (uptime_percent >= 0 AND uptime_percent <= 100),
    latency_ms INTEGER CHECK (latency_ms >= 0),
    error_count INTEGER DEFAULT 0 CHECK (error_count >= 0)
);

-- Indexes for operator_metrics (before TimescaleDB conversion)
CREATE INDEX idx_operator_metrics_time ON operator_metrics(time DESC);
CREATE INDEX idx_operator_metrics_gateway ON operator_metrics(gateway_id);
CREATE INDEX idx_operator_metrics_time_gateway ON operator_metrics(time DESC, gateway_id);

-- Discovery Channel Logs table
-- Analytics for discovery channel success/failure
CREATE TABLE discovery_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_type VARCHAR(50) NOT NULL CHECK (channel_type IN (
        'gps', 'fm_rds', 'dtv', 'plc', 'gsm_cb', 'lte_sib', 
        'iot_mqtt', 'blockchain', 'satellite', 'intranet', 'social'
    )),
    gateway_id UUID REFERENCES gateways(id) ON DELETE SET NULL,
    client_ip INET,
    region VARCHAR(10),
    success BOOLEAN NOT NULL,
    latency_ms INTEGER CHECK (latency_ms >= 0),
    error_message TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes for discovery_logs
CREATE INDEX idx_discovery_logs_channel ON discovery_logs(channel_type);
CREATE INDEX idx_discovery_logs_created ON discovery_logs(created_at DESC);
CREATE INDEX idx_discovery_logs_success ON discovery_logs(success);
CREATE INDEX idx_discovery_logs_region ON discovery_logs(region);
CREATE INDEX idx_discovery_logs_channel_success ON discovery_logs(channel_type, success);

-- Gateway Status History table
-- Track gateway status changes over time
CREATE TABLE gateway_status_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gateway_id UUID NOT NULL REFERENCES gateways(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL CHECK (status IN ('active', 'degraded', 'offline', 'maintenance')),
    reason TEXT,
    changed_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

CREATE INDEX idx_gateway_status_history_gateway ON gateway_status_history(gateway_id, changed_at DESC);

-- Rate Limiting table
-- Track API rate limits per device/IP
CREATE TABLE rate_limits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    identifier VARCHAR(255) NOT NULL, -- device_id or IP address
    endpoint VARCHAR(100) NOT NULL,
    request_count INTEGER DEFAULT 1 CHECK (request_count >= 0),
    window_start TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

CREATE INDEX idx_rate_limits_identifier_endpoint ON rate_limits(identifier, endpoint);
CREATE INDEX idx_rate_limits_window_start ON rate_limits(window_start);

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger to auto-update updated_at on gateways
CREATE TRIGGER update_gateways_updated_at BEFORE UPDATE ON gateways
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function to log gateway status changes
CREATE OR REPLACE FUNCTION log_gateway_status_change()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.status IS DISTINCT FROM NEW.status THEN
        INSERT INTO gateway_status_history (gateway_id, status, reason)
        VALUES (NEW.id, NEW.status, 'Status changed from ' || OLD.status || ' to ' || NEW.status);
    END IF;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger to log status changes
CREATE TRIGGER log_gateway_status_change AFTER UPDATE OF status ON gateways
    FOR EACH ROW EXECUTE FUNCTION log_gateway_status_change();
