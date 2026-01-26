package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// Database wraps a PostgreSQL connection pool
type Database struct {
	pool *sql.DB
}

// New creates a new database connection pool
func New(ctx context.Context, databaseURL string) (*Database, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Verify connection
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Database{pool: db}, nil
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
		       region, bandwidth_mbps, current_users, max_users, status,
		       created_at, last_seen, updated_at
		FROM gateways
		WHERE region = $1 AND status = 'active'
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
			&gw.Status, &gw.CreatedAt, &gw.LastSeen, &gw.UpdatedAt,
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
		       region, bandwidth_mbps, current_users, max_users, status,
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
			&gw.Status, &gw.CreatedAt, &gw.LastSeen, &gw.UpdatedAt,
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
	// TODO: Add is_honeypot column to gateways table
	// For now, return empty list
	return []*Gateway{}, nil
}
