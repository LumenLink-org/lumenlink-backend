package geo

import (
	"context"
	"os"
	"strconv"
	"strings"
	"unicode"

	"rendezvous/internal/db"
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
			sorted[i].Load = calculateGatewayLoad(sorted[i])
			sorted[j].Load = calculateGatewayLoad(sorted[j])
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
	if load < 0 {
		load = 0
	}
	if load > 1 {
		load = 1
	}

	_, err := b.db.Pool().ExecContext(
		ctx,
		`UPDATE gateways
		 SET current_users = CASE
			 WHEN max_users IS NULL THEN current_users
			 ELSE LEAST(GREATEST(ROUND($1 * max_users)::int, 0), max_users)
		 END,
		 last_seen = NOW()
		 WHERE id = $2`,
		load,
		gatewayID,
	)
	return err
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
	regionPreference := map[string][]string{
		"us-east-1":     {"us-east-1", "us-west-1", "eu-west-1", "ap-southeast-1"},
		"us-west-1":     {"us-west-1", "us-east-1", "ap-southeast-1", "eu-west-1"},
		"eu-west-1":     {"eu-west-1", "eu-central-1", "us-east-1", "ap-southeast-1"},
		"eu-central-1":  {"eu-central-1", "eu-west-1", "us-east-1", "ap-southeast-1"},
		"ap-southeast-1": {"ap-southeast-1", "ap-east-1", "us-west-1", "eu-west-1"},
		"ap-east-1":     {"ap-east-1", "ap-southeast-1", "us-west-1", "eu-west-1"},
		"me-south-1":    {"me-south-1", "eu-central-1", "eu-west-1", "ap-southeast-1"},
	}

	candidates, ok := regionPreference[clientRegion]
	if !ok {
		candidates = []string{"us-east-1", "us-west-1", "eu-west-1", "ap-southeast-1"}
	}

	for _, region := range candidates {
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
	_ = ctx

	keys := rolloutEnvKeys(configVersion, region)
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			if percent, err := strconv.Atoi(value); err == nil {
				return clampPercentage(percent), nil
			}
		}
	}

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

func calculateGatewayLoad(gw *db.Gateway) float64 {
	if gw.MaxUsers == nil || *gw.MaxUsers == 0 {
		return 0.5
	}
	return float64(gw.CurrentUsers) / float64(*gw.MaxUsers)
}

func rolloutEnvKeys(configVersion, region string) []string {
	var keys []string
	normalizedVersion := normalizeRolloutKey(configVersion)
	normalizedRegion := normalizeRolloutKey(region)

	if normalizedVersion != "" && normalizedRegion != "" {
		keys = append(keys, "LUMENLINK_ROLLOUT_PERCENTAGE_"+normalizedVersion+"_"+normalizedRegion)
	}
	if normalizedVersion != "" {
		keys = append(keys, "LUMENLINK_ROLLOUT_PERCENTAGE_"+normalizedVersion)
	}
	if normalizedRegion != "" {
		keys = append(keys, "LUMENLINK_ROLLOUT_PERCENTAGE_"+normalizedRegion)
	}
	keys = append(keys, "LUMENLINK_ROLLOUT_PERCENTAGE")

	return keys
}

func normalizeRolloutKey(value string) string {
	if value == "" {
		return ""
	}
	upper := strings.ToUpper(value)
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return '_'
	}, upper)
	return strings.Trim(normalized, "_")
}

func clampPercentage(percent int) int {
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}
