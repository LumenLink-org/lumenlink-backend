package db

import (
	"database/sql"
	"time"
)

// Gateway represents a relay server/gateway
type Gateway struct {
	ID               string
	PublicKey        []byte
	IPAddress        string
	Port             int
	TransportTypes   []string
	DiscoveryChannels []string
	Region           string
	BandwidthMbps    *int
	CurrentUsers     int
	MaxUsers         *int
	Status           string
	IsHoneypot       bool
	Load             float64 // Current load 0.0-1.0
	CreatedAt        time.Time
	LastSeen         *time.Time
	UpdatedAt        time.Time
}

// ConfigPack represents a signed configuration pack
type ConfigPack struct {
	ID        string
	Version   int
	Region    string
	Content   []byte // JSONB stored as bytes
	Signature []byte
	CreatedAt time.Time
	ExpiresAt *time.Time
	CreatedBy *string
}

// Attestation represents a device attestation record
type Attestation struct {
	ID             string
	DeviceID       string
	Platform       string
	Token          string
	Verified       bool
	VerifiedAt     *time.Time
	DeviceIntegrity *string
	CreatedAt      time.Time
	ExpiresAt      *time.Time
}

// OperatorMetric represents a time-series metric entry
type OperatorMetric struct {
	Time              time.Time
	GatewayID         string
	UsersConnected    int
	BandwidthUsedMbps int
	PacketsForwarded  int64
	UptimePercent     *float64
	LatencyMs         *int
	ErrorCount        int
}

// DiscoveryLog represents a discovery channel log entry
type DiscoveryLog struct {
	ID          string
	ChannelType string
	GatewayID   *string
	ClientIP    *string
	Region      *string
	Success     bool
	LatencyMs   *int
	ErrorMessage *sql.NullString
	CreatedAt   time.Time
}

// GatewayStatusHistory represents a gateway status change
type GatewayStatusHistory struct {
	ID        string
	GatewayID string
	Status    string
	Reason    *string
	ChangedAt time.Time
}

// RateLimit represents a rate limit entry
type RateLimit struct {
	ID           string
	Identifier   string
	Endpoint     string
	RequestCount int
	WindowStart  time.Time
	CreatedAt    time.Time
}
