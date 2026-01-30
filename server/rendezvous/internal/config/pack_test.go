package config

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"rendezvous/internal/db"
)

func init() {
	os.Setenv("LUMENLINK_ALLOW_EPHEMERAL_SIGNING_KEY", "true")
}

func TestVerifyConfigPack(t *testing.T) {
	ctx := context.Background()
	database := mustTestDB(t)
	defer database.Close()

	svc, err := NewConfigService(database)
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}

	pack, err := svc.GenerateConfigPack(ctx, "client-1", "us-east-1", nil)
	if err != nil {
		t.Fatalf("GenerateConfigPack: %v", err)
	}

	if !svc.VerifyConfigPack(pack) {
		t.Error("VerifyConfigPack: expected valid signature")
	}

	// Tamper with pack
	pack.Gateways = append(pack.Gateways, GatewayInfo{ID: "tampered"})
	if svc.VerifyConfigPack(pack) {
		t.Error("VerifyConfigPack: expected invalid after tampering")
	}
}

func TestVerifyConfigPack_WrongKey(t *testing.T) {
	ctx := context.Background()
	database := mustTestDB(t)
	defer database.Close()

	svc, err := NewConfigService(database)
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}

	pack, err := svc.GenerateConfigPack(ctx, "client-1", "us-east-1", nil)
	if err != nil {
		t.Fatalf("GenerateConfigPack: %v", err)
	}

	// Replace public key with different key
	_, wrongPub, _ := ed25519.GenerateKey(rand.Reader)
	pack.PublicKey = wrongPub
	if svc.VerifyConfigPack(pack) {
		t.Error("VerifyConfigPack: expected invalid with wrong public key")
	}
}

func TestGenerateConfigPack_Structure(t *testing.T) {
	ctx := context.Background()
	database := mustTestDB(t)
	defer database.Close()

	svc, err := NewConfigService(database)
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}

	pack, err := svc.GenerateConfigPack(ctx, "client-1", "us-east-1", nil)
	if err != nil {
		t.Fatalf("GenerateConfigPack: %v", err)
	}

	if pack.Version != "1.0" {
		t.Errorf("Version: got %q, want 1.0", pack.Version)
	}
	if pack.Timestamp <= 0 {
		t.Error("Timestamp: expected positive")
	}
	if pack.Signature == nil || len(pack.Signature) == 0 {
		t.Error("Signature: expected non-empty")
	}
	if pack.PublicKey == nil || len(pack.PublicKey) != ed25519.PublicKeySize {
		t.Error("PublicKey: expected 32 bytes")
	}
	if pack.Metadata == nil || pack.Metadata["client_id"] != "client-1" {
		t.Errorf("Metadata: got %v", pack.Metadata)
	}
	if len(pack.Transports) == 0 {
		t.Error("Transports: expected non-empty")
	}
	if len(pack.Discovery.Channels) == 0 {
		t.Error("Discovery.Channels: expected non-empty")
	}
}

// Regression suite for config pack signing - ensures signing/verification edge cases
func TestVerifyConfigPack_EmptySignature(t *testing.T) {
	database := mustTestDB(t)
	defer database.Close()

	svc, err := NewConfigService(database)
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}

	pack, err := svc.GenerateConfigPack(context.Background(), "client-1", "us-east-1", nil)
	if err != nil {
		t.Fatalf("GenerateConfigPack: %v", err)
	}

	pack.Signature = nil
	if svc.VerifyConfigPack(pack) {
		t.Error("VerifyConfigPack: expected invalid with nil signature")
	}

	pack.Signature = []byte{}
	if svc.VerifyConfigPack(pack) {
		t.Error("VerifyConfigPack: expected invalid with empty signature")
	}
}

func TestVerifyConfigPack_TamperedSignature(t *testing.T) {
	database := mustTestDB(t)
	defer database.Close()

	svc, err := NewConfigService(database)
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}

	pack, err := svc.GenerateConfigPack(context.Background(), "client-1", "us-east-1", nil)
	if err != nil {
		t.Fatalf("GenerateConfigPack: %v", err)
	}

	// Flip bits in signature
	pack.Signature[0] ^= 0xFF
	if svc.VerifyConfigPack(pack) {
		t.Error("VerifyConfigPack: expected invalid with tampered signature")
	}
}

func TestVerifyConfigPack_TamperedMetadata(t *testing.T) {
	database := mustTestDB(t)
	defer database.Close()

	svc, err := NewConfigService(database)
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}

	pack, err := svc.GenerateConfigPack(context.Background(), "client-1", "us-east-1", nil)
	if err != nil {
		t.Fatalf("GenerateConfigPack: %v", err)
	}

	pack.Metadata["client_id"] = "attacker"
	if svc.VerifyConfigPack(pack) {
		t.Error("VerifyConfigPack: expected invalid after metadata tampering")
	}
}

func TestVerifyConfigPack_TamperedVersion(t *testing.T) {
	database := mustTestDB(t)
	defer database.Close()

	svc, err := NewConfigService(database)
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}

	pack, err := svc.GenerateConfigPack(context.Background(), "client-1", "us-east-1", nil)
	if err != nil {
		t.Fatalf("GenerateConfigPack: %v", err)
	}

	pack.Version = "2.0"
	if svc.VerifyConfigPack(pack) {
		t.Error("VerifyConfigPack: expected invalid after version tampering")
	}
}

func TestVerifyConfigPack_TamperedTimestamp(t *testing.T) {
	database := mustTestDB(t)
	defer database.Close()

	svc, err := NewConfigService(database)
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}

	pack, err := svc.GenerateConfigPack(context.Background(), "client-1", "us-east-1", nil)
	if err != nil {
		t.Fatalf("GenerateConfigPack: %v", err)
	}

	pack.Timestamp = 0
	if svc.VerifyConfigPack(pack) {
		t.Error("VerifyConfigPack: expected invalid after timestamp tampering")
	}
}

func TestVerifyConfigPack_EmptyPublicKey(t *testing.T) {
	database := mustTestDB(t)
	defer database.Close()

	svc, err := NewConfigService(database)
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}

	pack, err := svc.GenerateConfigPack(context.Background(), "client-1", "us-east-1", nil)
	if err != nil {
		t.Fatalf("GenerateConfigPack: %v", err)
	}

	pack.PublicKey = nil
	if svc.VerifyConfigPack(pack) {
		t.Error("VerifyConfigPack: expected invalid with nil public key")
	}

	pack.PublicKey = make([]byte, 16) // Wrong length
	if svc.VerifyConfigPack(pack) {
		t.Error("VerifyConfigPack: expected invalid with wrong-length public key")
	}
}

func TestGenerateConfigPack_ConsistentStructure(t *testing.T) {
	database := mustTestDB(t)
	defer database.Close()

	svc, err := NewConfigService(database)
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}

	pack1, err := svc.GenerateConfigPack(context.Background(), "client-1", "us-east-1", nil)
	if err != nil {
		t.Fatalf("GenerateConfigPack: %v", err)
	}
	pack2, err := svc.GenerateConfigPack(context.Background(), "client-1", "us-east-1", nil)
	if err != nil {
		t.Fatalf("GenerateConfigPack: %v", err)
	}

	// Both should verify
	if !svc.VerifyConfigPack(pack1) || !svc.VerifyConfigPack(pack2) {
		t.Error("both packs should verify")
	}
	// Same public key
	if len(pack1.PublicKey) != len(pack2.PublicKey) {
		t.Error("public keys should match length")
	}
	// Signature length consistent (Ed25519 = 64 bytes)
	if len(pack1.Signature) != ed25519.SignatureSize || len(pack2.Signature) != ed25519.SignatureSize {
		t.Errorf("signature length: got %d, %d; want %d", len(pack1.Signature), len(pack2.Signature), ed25519.SignatureSize)
	}
}

func TestGenerateConfigPack_AttestationInvalid_IncludesHoneypots(t *testing.T) {
	ctx := context.Background()
	database := mustTestDBWithHoneypots(t)
	defer database.Close()

	svc, err := NewConfigService(database)
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}

	pack, err := svc.GenerateConfigPack(ctx, "client-1", "us-east-1", &AttestationResult{IsValid: false})
	if err != nil {
		t.Fatalf("GenerateConfigPack: %v", err)
	}

	hasHoneypot := false
	for _, gw := range pack.Gateways {
		if gw.IsHoneypot {
			hasHoneypot = true
			break
		}
	}
	if !hasHoneypot {
		t.Error("expected honeypot gateways when attestation invalid")
	}
}

func mustTestDB(t *testing.T) *db.Database {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	mock.ExpectQuery(`SELECT id, public_key`).WillReturnRows(sqlmock.NewRows([]string{
		"id", "public_key", "ip_address", "port", "transport_types", "discovery_channels",
		"region", "bandwidth_mbps", "current_users", "max_users", "status", "is_honeypot",
		"created_at", "last_seen", "updated_at",
	}))
	mock.ExpectQuery(`SELECT id, public_key`).WillReturnRows(sqlmock.NewRows([]string{
		"id", "public_key", "ip_address", "port", "transport_types", "discovery_channels",
		"region", "bandwidth_mbps", "current_users", "max_users", "status", "is_honeypot",
		"created_at", "last_seen", "updated_at",
	}))

	return db.NewFromPool(sqlDB)
}

func mustTestDBWithHoneypots(t *testing.T) *db.Database {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	pubKey := make([]byte, ed25519.PublicKeySize)
	rand.Read(pubKey)
	now := time.Now()

	// GetGatewaysByRegion - empty
	mock.ExpectQuery(`SELECT id, public_key`).WillReturnRows(sqlmock.NewRows([]string{
		"id", "public_key", "ip_address", "port", "transport_types", "discovery_channels",
		"region", "bandwidth_mbps", "current_users", "max_users", "status", "is_honeypot",
		"created_at", "last_seen", "updated_at",
	}))

	// GetHoneypotGateways - one honeypot
	rows := sqlmock.NewRows([]string{
		"id", "public_key", "ip_address", "port", "transport_types", "discovery_channels",
		"region", "bandwidth_mbps", "current_users", "max_users", "status", "is_honeypot",
		"created_at", "last_seen", "updated_at",
	})
	rows.AddRow(
		"honeypot-id", pubKey, "10.0.0.1", 443,
		"{masque,xtls}", "{gps}",
		"us-east-1", 100, 0, 100, "active", true,
		now, now, now,
	)
	mock.ExpectQuery(`SELECT id, public_key`).WillReturnRows(rows)

	return db.NewFromPool(sqlDB)
}
