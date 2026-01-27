-- LumenLink TimescaleDB Migration
-- Migration: 0002_add_timescale.up.sql
-- Description: Converts operator_metrics to TimescaleDB hypertable for time-series optimization

-- Enable TimescaleDB extension
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- Convert operator_metrics to hypertable
-- This enables automatic partitioning by time and optimized queries
SELECT create_hypertable(
    'operator_metrics',
    'time',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);

-- Optional TimescaleDB optimizations (skip if unsupported)
DO $$
BEGIN
    -- Add compression policy (compress data older than 7 days)
    -- This reduces storage requirements for historical data
    PERFORM add_compression_policy('operator_metrics', INTERVAL '7 days', if_not_exists => TRUE);

    -- Add retention policy (drop data older than 90 days)
    -- Adjust retention period as needed
    PERFORM add_retention_policy('operator_metrics', INTERVAL '90 days', if_not_exists => TRUE);

    -- Create continuous aggregate for hourly statistics
    -- Pre-aggregates data for faster queries
    CREATE MATERIALIZED VIEW IF NOT EXISTS operator_metrics_hourly
    WITH (timescaledb.continuous) AS
    SELECT
        time_bucket('1 hour', time) AS bucket,
        gateway_id,
        AVG(users_connected) AS avg_users_connected,
        MAX(users_connected) AS max_users_connected,
        AVG(bandwidth_used_mbps) AS avg_bandwidth_mbps,
        MAX(bandwidth_used_mbps) AS max_bandwidth_mbps,
        SUM(packets_forwarded) AS total_packets_forwarded,
        AVG(uptime_percent) AS avg_uptime_percent,
        AVG(latency_ms) AS avg_latency_ms,
        SUM(error_count) AS total_errors
    FROM operator_metrics
    GROUP BY bucket, gateway_id;

    -- Create index on continuous aggregate
    CREATE INDEX IF NOT EXISTS idx_operator_metrics_hourly_bucket_gateway
        ON operator_metrics_hourly(bucket DESC, gateway_id);

    -- Create continuous aggregate for daily statistics
    CREATE MATERIALIZED VIEW IF NOT EXISTS operator_metrics_daily
    WITH (timescaledb.continuous) AS
    SELECT
        time_bucket('1 day', time) AS bucket,
        gateway_id,
        AVG(users_connected) AS avg_users_connected,
        MAX(users_connected) AS max_users_connected,
        AVG(bandwidth_used_mbps) AS avg_bandwidth_mbps,
        MAX(bandwidth_used_mbps) AS max_bandwidth_mbps,
        SUM(packets_forwarded) AS total_packets_forwarded,
        AVG(uptime_percent) AS avg_uptime_percent,
        AVG(latency_ms) AS avg_latency_ms,
        SUM(error_count) AS total_errors
    FROM operator_metrics
    GROUP BY bucket, gateway_id;

    -- Create index on daily aggregate
    CREATE INDEX IF NOT EXISTS idx_operator_metrics_daily_bucket_gateway
        ON operator_metrics_daily(bucket DESC, gateway_id);

    -- Refresh policy for continuous aggregates (refresh every hour)
    PERFORM add_continuous_aggregate_policy('operator_metrics_hourly',
        start_offset => INTERVAL '3 hours',
        end_offset => INTERVAL '1 hour',
        schedule_interval => INTERVAL '1 hour',
        if_not_exists => TRUE
    );

    PERFORM add_continuous_aggregate_policy('operator_metrics_daily',
        start_offset => INTERVAL '3 days',
        end_offset => INTERVAL '1 day',
        schedule_interval => INTERVAL '1 day',
        if_not_exists => TRUE
    );
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'Skipping TimescaleDB optimizations: %', SQLERRM;
END $$;
