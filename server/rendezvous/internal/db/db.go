package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// Database wraps a PostgreSQL connection pool
type Database struct {
	pool *sql.DB
}

// ErrGatewayNotFound is returned when a gateway ID does not exist.
var ErrGatewayNotFound = errors.New("gateway not found")

// NewFromPool creates a Database from an existing connection pool (for testing).
func NewFromPool(pool *sql.DB) *Database {
	return &Database{pool: pool}
}

// New creates a new database connection pool with retry on connect.
// Uses bounded retries with exponential backoff (max 5 attempts, ~30s total).
func New(ctx context.Context, databaseURL string) (*Database, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Verify connection with retries (handles DB not yet ready at startup)
	const maxAttempts = 5
	baseDelay := 2 * time.Second
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = db.PingContext(pingCtx)
		cancel()
		if err == nil {
			return &Database{pool: db}, nil
		}
		if attempt == maxAttempts {
			return nil, fmt.Errorf("failed to ping database after %d attempts: %w", maxAttempts, err)
		}
		delay := baseDelay * time.Duration(1<<uint(attempt-1))
		if delay > 10*time.Second {
			delay = 10 * time.Second
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled while waiting for database: %w", ctx.Err())
		case <-time.After(delay):
			// retry
		}
	}
	return nil, fmt.Errorf("failed to connect to database")
}

// Close closes the database connection pool
func (d *Database) Close() error {
	return d.pool.Close()
}

// Pool returns the underlying connection pool
func (d *Database) Pool() *sql.DB {
	return d.pool
}

// Health checks database health
func (d *Database) Health(ctx context.Context) error {
	return d.pool.PingContext(ctx)
}

// GetGatewaysByRegion returns gateways in a specific region
func (d *Database) GetGatewaysByRegion(ctx context.Context, region string) ([]*Gateway, error) {
	query := `
		SELECT id, public_key, ip_address, port, transport_types, discovery_channels,
		       region, bandwidth_mbps, current_users, max_users, status, is_honeypot,
		       created_at, last_seen, updated_at
		FROM gateways
		WHERE region = $1 AND status = 'active' AND is_honeypot = FALSE
		ORDER BY current_users ASC
		LIMIT 100
	`
	
	rows, err := d.pool.QueryContext(ctx, query, region)
	if err != nil {
		return nil, fmt.Errorf("failed to query gateways: %w", err)
	}
	defer rows.Close()

	var gateways []*Gateway
	for rows.Next() {
		var gw Gateway
		var transportTypes pq.StringArray
		var discoveryChannels pq.StringArray
		
		err := rows.Scan(
			&gw.ID, &gw.PublicKey, &gw.IPAddress, &gw.Port,
			&transportTypes, &discoveryChannels,
			&gw.Region, &gw.BandwidthMbps, &gw.CurrentUsers, &gw.MaxUsers,
			&gw.Status, &gw.IsHoneypot, &gw.CreatedAt, &gw.LastSeen, &gw.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan gateway: %w", err)
		}

		gw.TransportTypes = []string(transportTypes)
		gw.DiscoveryChannels = []string(discoveryChannels)
		gateways = append(gateways, &gw)
	}

	return gateways, rows.Err()
}

// GetAllGateways returns all active gateways
func (d *Database) GetAllGateways(ctx context.Context) ([]*Gateway, error) {
	query := `
		SELECT id, public_key, ip_address, port, transport_types, discovery_channels,
		       region, bandwidth_mbps, current_users, max_users, status, is_honeypot,
		       created_at, last_seen, updated_at
		FROM gateways
		WHERE status IN ('active', 'degraded')
		ORDER BY current_users DESC, last_seen DESC
		LIMIT 1000
	`
	
	rows, err := d.pool.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query gateways: %w", err)
	}
	defer rows.Close()

	var gateways []*Gateway
	for rows.Next() {
		var gw Gateway
		var transportTypes pq.StringArray
		var discoveryChannels pq.StringArray
		
		err := rows.Scan(
			&gw.ID, &gw.PublicKey, &gw.IPAddress, &gw.Port,
			&transportTypes, &discoveryChannels,
			&gw.Region, &gw.BandwidthMbps, &gw.CurrentUsers, &gw.MaxUsers,
			&gw.Status, &gw.IsHoneypot, &gw.CreatedAt, &gw.LastSeen, &gw.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan gateway: %w", err)
		}

		gw.TransportTypes = []string(transportTypes)
		gw.DiscoveryChannels = []string(discoveryChannels)
		gateways = append(gateways, &gw)
	}

	return gateways, rows.Err()
}

// GetHoneypotGateways returns honeypot gateways for a region
func (d *Database) GetHoneypotGateways(ctx context.Context, region string) ([]*Gateway, error) {
	query := `
		SELECT id, public_key, ip_address, port, transport_types, discovery_channels,
		       region, bandwidth_mbps, current_users, max_users, status, is_honeypot,
		       created_at, last_seen, updated_at
		FROM gateways
		WHERE region = $1 AND status = 'active' AND is_honeypot = TRUE
		ORDER BY current_users ASC
		LIMIT 10
	`

	rows, err := d.pool.QueryContext(ctx, query, region)
	if err != nil {
		return nil, fmt.Errorf("failed to query honeypot gateways: %w", err)
	}
	defer rows.Close()

	var gateways []*Gateway
	for rows.Next() {
		var gw Gateway
		var transportTypes pq.StringArray
		var discoveryChannels pq.StringArray

		err := rows.Scan(
			&gw.ID, &gw.PublicKey, &gw.IPAddress, &gw.Port,
			&transportTypes, &discoveryChannels,
			&gw.Region, &gw.BandwidthMbps, &gw.CurrentUsers, &gw.MaxUsers,
			&gw.Status, &gw.IsHoneypot, &gw.CreatedAt, &gw.LastSeen, &gw.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan honeypot gateway: %w", err)
		}

		gw.TransportTypes = []string(transportTypes)
		gw.DiscoveryChannels = []string(discoveryChannels)
		gateways = append(gateways, &gw)
	}

	return gateways, rows.Err()
}

// RecordGatewayStatus updates gateway status and records metrics.
func (d *Database) RecordGatewayStatus(
	ctx context.Context,
	gatewayID string,
	status string,
	usersConnected int,
	bandwidthUsedMbps int,
	packetsForwarded int64,
	uptimePercent float64,
) error {
	tx, err := d.pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	result, err := tx.ExecContext(
		ctx,
		`UPDATE gateways SET status = $1, current_users = $2, last_seen = NOW() WHERE id = $3`,
		status,
		usersConnected,
		gatewayID,
	)
	if err != nil {
		return fmt.Errorf("failed to update gateway status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read update result: %w", err)
	}
	if rowsAffected == 0 {
		return ErrGatewayNotFound
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO operator_metrics
		 (time, gateway_id, users_connected, bandwidth_used_mbps, packets_forwarded, uptime_percent)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		time.Now().UTC(),
		gatewayID,
		usersConnected,
		bandwidthUsedMbps,
		packetsForwarded,
		uptimePercent,
	)
	if err != nil {
		return fmt.Errorf("failed to insert operator metrics: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit gateway status: %w", err)
	}

	return nil
}

// RecordDiscoveryLog inserts a discovery log entry.
func (d *Database) RecordDiscoveryLog(
	ctx context.Context,
	channelType string,
	gatewayID *string,
	clientIP *string,
	region *string,
	success bool,
	latencyMs *int,
	errorMessage *string,
) error {
	var latencyValue interface{}
	if latencyMs != nil {
		latencyValue = *latencyMs
	}

	var errorValue interface{}
	if errorMessage != nil {
		errorValue = *errorMessage
	}

	_, err := d.pool.ExecContext(
		ctx,
		`INSERT INTO discovery_logs
		 (channel_type, gateway_id, client_ip, region, success, latency_ms, error_message)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		channelType,
		gatewayID,
		clientIP,
		region,
		success,
		latencyValue,
		errorValue,
	)
	if err != nil {
		return fmt.Errorf("failed to insert discovery log: %w", err)
	}

	return nil
}
