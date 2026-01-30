package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"rendezvous/internal/attestation"
	"rendezvous/internal/config"
	"rendezvous/internal/db"
	"rendezvous/internal/geo"
)

func init() {
	gin.SetMode(gin.TestMode)
	os.Setenv("LUMENLINK_ALLOW_EPHEMERAL_SIGNING_KEY", "true")
	os.Setenv("LUMENLINK_ALLOW_ATTESTATION_BYPASS", "true")
}

func TestHealth(t *testing.T) {
	router := gin.New()
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if body["status"] != "healthy" {
		t.Errorf("status: got %q, want healthy", body["status"])
	}
}

func TestGetAttestationChallenge(t *testing.T) {
	database := mustTestDB(t)
	defer database.Close()

	configSvc, err := config.NewConfigService(database)
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}
	attestSvc := attestation.NewAttestationService(database)
	geoBalancer := geo.NewBalancer(database)
	handler := NewHandler(configSvc, attestSvc, geoBalancer, database)

	router := gin.New()
	router.GET("/api/v1/attest/challenge", handler.GetAttestationChallenge)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/attest/challenge", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if body["challenge"] == "" {
		t.Error("challenge: expected non-empty")
	}
}

func TestVerifyAttestation_AndroidBypass(t *testing.T) {
	database := mustTestDBWithAttestStorage(t)
	defer database.Close()

	configSvc, err := config.NewConfigService(database)
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}
	attestSvc := attestation.NewAttestationService(database)
	geoBalancer := geo.NewBalancer(database)
	handler := NewHandler(configSvc, attestSvc, geoBalancer, database)

	router := gin.New()
	router.POST("/api/v1/attest", handler.VerifyAttestation)

	body := []byte(`{"platform":"android","device_id":"test-device","token":"{}"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/attest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if v, ok := resp["verified"].(bool); !ok || !v {
		t.Errorf("verified: got %v, want true", resp["verified"])
	}
}

func TestVerifyAttestation_UnsupportedPlatform(t *testing.T) {
	database := mustTestDB(t)
	defer database.Close()

	configSvc, err := config.NewConfigService(database)
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}
	attestSvc := attestation.NewAttestationService(database)
	geoBalancer := geo.NewBalancer(database)
	handler := NewHandler(configSvc, attestSvc, geoBalancer, database)

	router := gin.New()
	router.POST("/api/v1/attest", handler.VerifyAttestation)

	body := []byte(`{"platform":"desktop","device_id":"test","token":"x"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/attest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", w.Code)
	}
}

func TestGetConfig_InvalidRequest(t *testing.T) {
	database := mustTestDB(t)
	defer database.Close()

	configSvc, err := config.NewConfigService(database)
	if err != nil {
		t.Fatalf("NewConfigService: %v", err)
	}
	attestSvc := attestation.NewAttestationService(database)
	geoBalancer := geo.NewBalancer(database)
	handler := NewHandler(configSvc, attestSvc, geoBalancer, database)

	router := gin.New()
	router.POST("/api/v1/config", handler.GetConfig)

	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", w.Code)
	}
}

func mustTestDB(t *testing.T) *db.Database {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	// Mock GetGatewaysByRegion and GetHoneypotGateways for config service
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{
		"id", "public_key", "ip_address", "port", "transport_types", "discovery_channels",
		"region", "bandwidth_mbps", "current_users", "max_users", "status", "is_honeypot",
		"created_at", "last_seen", "updated_at",
	}))
	mock.ExpectQuery(`SELECT`).WillReturnRows(sqlmock.NewRows([]string{
		"id", "public_key", "ip_address", "port", "transport_types", "discovery_channels",
		"region", "bandwidth_mbps", "current_users", "max_users", "status", "is_honeypot",
		"created_at", "last_seen", "updated_at",
	}))

	return db.NewFromPool(sqlDB)
}

func mustTestDBWithAttestStorage(t *testing.T) *db.Database {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	mock.ExpectExec(`INSERT INTO attestations`).WillReturnResult(sqlmock.NewResult(1, 1))
	return db.NewFromPool(sqlDB)
}
