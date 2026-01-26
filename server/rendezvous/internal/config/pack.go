package config

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/unitech-for-good/lumenlink/rendezvous/internal/attestation"
	"github.com/unitech-for-good/lumenlink/rendezvous/internal/db"
)

// SignedConfigPack represents a signed configuration pack
type SignedConfigPack struct {
	Version      string                 `json:"version"`
	Timestamp    int64                  `json:"timestamp"`
	Gateways     []GatewayInfo          `json:"gateways"`
	Transports   []TransportConfig      `json:"transports"`
	Discovery    DiscoveryConfig        `json:"discovery"`
	Metadata     map[string]interface{} `json:"metadata"`
	Signature    []byte                 `json:"signature"`
	PublicKey    []byte                 `json:"public_key"`
}

// GatewayInfo contains gateway connection information
type GatewayInfo struct {
	ID          string   `json:"id"`
	Address     string   `json:"address"`
	Port        int      `json:"port"`
	Transports  []string `json:"transports"` // e.g., ["masque", "xtls", "parasite"]
	Region      string   `json:"region"`
	Load        float64  `json:"load"` // 0.0-1.0
	IsHoneypot  bool     `json:"is_honeypot"`
	PublicKey   []byte   `json:"public_key"`
}

// TransportConfig contains transport-specific configuration
type TransportConfig struct {
	Type        string            `json:"type"` // masque, xtls, parasite, ssh
	Endpoints   []string          `json:"endpoints"`
	Fingerprint string            `json:"fingerprint"` // TLS fingerprint to mimic
	Options     map[string]string `json:"options"`
}

// DiscoveryConfig contains discovery channel configuration
type DiscoveryConfig struct {
	Channels    []string `json:"channels"` // gps, fm_rds, dtv, plc, etc.
	ScanInterval int     `json:"scan_interval"` // seconds
	BatteryAware bool    `json:"battery_aware"`
}

// ConfigService handles config pack generation and signing
type ConfigService struct {
	db          *db.Database
	privateKey  ed25519.PrivateKey
	publicKey   ed25519.PublicKey
}

// NewConfigService creates a new config service
func NewConfigService(database *db.Database) (*ConfigService, error) {
	privateKey, publicKey, err := loadSigningKeys()
	if err != nil {
		return nil, err
	}

	return &ConfigService{
		db:         database,
		privateKey: privateKey,
		publicKey:  publicKey,
	}, nil
}

func loadSigningKeys() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	privateKeyB64 := os.Getenv("LUMENLINK_CONFIG_SIGNING_PRIVATE_KEY")
	publicKeyB64 := os.Getenv("LUMENLINK_CONFIG_SIGNING_PUBLIC_KEY")
	allowEphemeral := os.Getenv("LUMENLINK_ALLOW_EPHEMERAL_SIGNING_KEY")

	if privateKeyB64 == "" {
		if allowEphemeral == "1" || allowEphemeral == "true" || allowEphemeral == "TRUE" {
			publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				return nil, nil, err
			}
			return privateKey, publicKey, nil
		}
		return nil, nil, fmt.Errorf("config signing private key is required")
	}

	privateKeyBytes, err := base64.StdEncoding.DecodeString(privateKeyB64)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid config signing private key encoding: %w", err)
	}
	if len(privateKeyBytes) != ed25519.PrivateKeySize {
		return nil, nil, fmt.Errorf("invalid config signing private key length")
	}

	privateKey := ed25519.PrivateKey(privateKeyBytes)
	var publicKey ed25519.PublicKey
	if publicKeyB64 != "" {
		publicKeyBytes, err := base64.StdEncoding.DecodeString(publicKeyB64)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid config signing public key encoding: %w", err)
		}
		if len(publicKeyBytes) != ed25519.PublicKeySize {
			return nil, nil, fmt.Errorf("invalid config signing public key length")
		}
		publicKey = ed25519.PublicKey(publicKeyBytes)
	} else {
		publicKey = privateKey.Public().(ed25519.PublicKey)
	}

	return privateKey, publicKey, nil
}

// GenerateConfigPack generates a signed config pack for a client
func (s *ConfigService) GenerateConfigPack(
	ctx context.Context,
	clientID string,
	region string,
	attestationResult *AttestationResult,
) (*SignedConfigPack, error) {
	// Get gateways based on geo-load balancing
	gateways, err := s.selectGateways(ctx, region, attestationResult)
	if err != nil {
		return nil, err
	}

	// Get transport configurations
	transports := s.getTransportConfigs()

	// Get discovery configuration
	discovery := s.getDiscoveryConfig()

	// Create config pack
	pack := &SignedConfigPack{
		Version:    "1.0",
		Timestamp:  time.Now().Unix(),
		Gateways:   gateways,
		Transports: transports,
		Discovery:  discovery,
		Metadata: map[string]interface{}{
			"client_id": clientID,
			"region":    region,
		},
		PublicKey: s.publicKey,
	}

	// Sign the config pack
	signature, err := s.signConfigPack(pack)
	if err != nil {
		return nil, err
	}
	pack.Signature = signature

	return pack, nil
}

// selectGateways selects gateways based on geo-load balancing and honeypot logic
func (s *ConfigService) selectGateways(
	ctx context.Context,
	region string,
	attestationResult *AttestationResult,
) ([]GatewayInfo, error) {
	// Query gateways from database
	gateways, err := s.db.GetGatewaysByRegion(ctx, region)
	if err != nil {
		return nil, err
	}

	// Apply honeypot logic
	// If attestation fails or is suspicious, include honeypots
	if attestationResult == nil || !attestationResult.IsValid {
		// Add honeypot gateways
		// TODO: Implement GetHoneypotGateways in db package
	}

	// Calculate load based on current users and max users
	// Sort by load (prefer lower load)
	sort.Slice(gateways, func(i, j int) bool {
		loadI := s.calculateLoad(gateways[i])
		loadJ := s.calculateLoad(gateways[j])
		return loadI < loadJ
	})

	// Select top 3-5 gateways
	maxGateways := 5
	if len(gateways) > maxGateways {
		gateways = gateways[:maxGateways]
	}

	// Convert to GatewayInfo
	result := make([]GatewayInfo, len(gateways))
	for i, gw := range gateways {
		result[i] = GatewayInfo{
			ID:         gw.ID,
			Address:    gw.IPAddress,
			Port:       gw.Port,
			Transports: gw.TransportTypes,
			Region:     gw.Region,
			Load:       s.calculateLoad(gw),
			IsHoneypot: false, // TODO: Add honeypot flag to Gateway model
			PublicKey:  gw.PublicKey,
		}
	}

	return result, nil
}

// calculateLoad calculates gateway load (0.0-1.0)
func (s *ConfigService) calculateLoad(gw *db.Gateway) float64 {
	if gw.MaxUsers == nil || *gw.MaxUsers == 0 {
		return 0.5 // Default load if max users not set
	}
	return float64(gw.CurrentUsers) / float64(*gw.MaxUsers)
}

// getTransportConfigs returns transport configurations
func (s *ConfigService) getTransportConfigs() []TransportConfig {
	return []TransportConfig{
		{
			Type:        "masque",
			Endpoints:   []string{"icloud.com", "www.icloud.com"},
			Fingerprint: "apple_icloud",
			Options: map[string]string{
				"quic_version": "1",
			},
		},
		{
			Type:        "xtls",
			Endpoints:   []string{"microsoft.com", "www.microsoft.com"},
			Fingerprint: "microsoft_edge",
			Options: map[string]string{
				"reality": "true",
			},
		},
		{
			Type:      "parasite",
			Endpoints: []string{"cdn.cloudflare.com", "cdnjs.cloudflare.com"},
			Options: map[string]string{
				"header_encoding": "base64url",
			},
		},
		{
			Type:      "ssh",
			Endpoints: []string{},
			Options: map[string]string{
				"obfuscated": "true",
			},
		},
	}
}

// getDiscoveryConfig returns discovery channel configuration
func (s *ConfigService) getDiscoveryConfig() DiscoveryConfig {
	return DiscoveryConfig{
		Channels:     []string{"gps", "fm_rds", "dtv", "plc", "gsm", "lte", "blockchain"},
		ScanInterval: 300, // 5 minutes
		BatteryAware: true,
	}
}

// signConfigPack signs a config pack
func (s *ConfigService) signConfigPack(pack *SignedConfigPack) ([]byte, error) {
	// Create a copy without signature for signing
	packCopy := *pack
	packCopy.Signature = nil

	// Marshal to JSON
	data, err := json.Marshal(packCopy)
	if err != nil {
		return nil, err
	}

	// Sign with Ed25519
	signature := ed25519.Sign(s.privateKey, data)
	return signature, nil
}

// VerifyConfigPack verifies a config pack signature
func (s *ConfigService) VerifyConfigPack(pack *SignedConfigPack) bool {
	// Create a copy without signature for verification
	packCopy := *pack
	signature := packCopy.Signature
	packCopy.Signature = nil

	// Marshal to JSON
	data, err := json.Marshal(packCopy)
	if err != nil {
		return false
	}

	// Verify signature
	return ed25519.Verify(pack.PublicKey, data, signature)
}
