package geo

import (
	"context"
	"math/rand"
	"time"

	"rendezvous/internal/db"
	// "github.com/unitech-for-good/lumenlink/rendezvous/internal/db"
)

// GeoBalancer handles geo-load balancing for gateway selection
type GeoBalancer struct {
	db *db.Database
}

// NewBalancer creates a new geo balancer
func NewBalancer(database *db.Database) *GeoBalancer {
	return &GeoBalancer{
		db: database,
	}
}

// SelectRegion selects the best region for a client based on their location
func (b *GeoBalancer) SelectRegion(
	ctx context.Context,
	clientRegion string,
	preferredRegions []string,
) (string, error) {
	// If client specified a preferred region, use it if available
	if clientRegion != "" {
		available, err := b.isRegionAvailable(ctx, clientRegion)
		if err == nil && available {
			return clientRegion, nil
		}
	}

	// Check preferred regions
	for _, region := range preferredRegions {
		available, err := b.isRegionAvailable(ctx, region)
		if err == nil && available {
			return region, nil
		}
	}

	// Fallback to nearest available region
	nearest, err := b.findNearestRegion(ctx, clientRegion)
	if err == nil {
		return nearest, nil
	}

	// Last resort: return default region
	return "us-east-1", nil
}

// GetLoadBalancedGateways returns gateways for a region with load balancing
func (b *GeoBalancer) GetLoadBalancedGateways(
	ctx context.Context,
	region string,
	count int,
) ([]*db.Gateway, error) {
	// Get all gateways in region
	gateways, err := b.db.GetGatewaysByRegion(ctx, region)
	if err != nil {
		return nil, err
	}

	if len(gateways) == 0 {
		return []*db.Gateway{}, nil
	}

	// Sort by load (prefer lower load)
	// In production, use a more sophisticated algorithm (weighted random, etc.)
	sorted := make([]*db.Gateway, len(gateways))
	copy(sorted, gateways)
	
	// Simple load-based sorting
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].Load > sorted[j].Load {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Select top N gateways
	if count > len(sorted) {
		count = len(sorted)
	}

	return sorted[:count], nil
}

// UpdateGatewayLoad updates a gateway's load metric
func (b *GeoBalancer) UpdateGatewayLoad(
	ctx context.Context,
	gatewayID string,
	load float64,
) error {
	// TODO: Implement database update
	// This would update the gateways table with new load value
	return nil
}

// isRegionAvailable checks if a region has available gateways
func (b *GeoBalancer) isRegionAvailable(ctx context.Context, region string) (bool, error) {
	gateways, err := b.db.GetGatewaysByRegion(ctx, region)
	if err != nil {
		return false, err
	}

	// Check if any gateway is active and not overloaded
	for _, gw := range gateways {
		if gw.Status == "active" && gw.Load < 0.9 {
			return true, nil
		}
	}

	return false, nil
}

// findNearestRegion finds the nearest available region to the client
func (b *GeoBalancer) findNearestRegion(ctx context.Context, clientRegion string) (string, error) {
	// TODO: Implement region proximity calculation
	// This would use a region distance matrix or geolocation
	
	// Placeholder: return a random available region
	regions := []string{"us-east-1", "us-west-1", "eu-west-1", "ap-southeast-1"}
	
	rand.Seed(time.Now().UnixNano())
	for _, region := range regions {
		available, err := b.isRegionAvailable(ctx, region)
		if err == nil && available {
			return region, nil
		}
	}

	return "us-east-1", nil
}

// GetRolloutPercentage returns the rollout percentage for a config version
func (b *GeoBalancer) GetRolloutPercentage(
	ctx context.Context,
	configVersion string,
	region string,
) (int, error) {
	// TODO: Implement incremental rollout logic
	// This would check rollout configuration from database
	
	// Placeholder: return 100% (full rollout)
	return 100, nil
}

// ShouldIncludeInRollout determines if a client should receive a new config version
func (b *GeoBalancer) ShouldIncludeInRollout(
	ctx context.Context,
	clientID string,
	configVersion string,
	region string,
) (bool, error) {
	percentage, err := b.GetRolloutPercentage(ctx, configVersion, region)
	if err != nil {
		return false, err
	}

	// Simple hash-based rollout
	// In production, use a more sophisticated algorithm
	hash := hashString(clientID + configVersion + region)
	return (hash % 100) < percentage, nil
}

// hashString creates a simple hash from a string
func hashString(s string) int {
	hash := 0
	for _, char := range s {
		hash = hash*31 + int(char)
	}
	if hash < 0 {
		hash = -hash
	}
	return hash % 100
}
