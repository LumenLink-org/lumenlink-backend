package geo

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"rendezvous/internal/db"
)

func TestCalculateGatewayLoad(t *testing.T) {
	tests := []struct {
		name        string
		current     int
		maxUsers    *int
		wantLoad    float64
	}{
		{"zero max", 0, intPtr(0), 0.5},
		{"half load", 5, intPtr(10), 0.5},
		{"full load", 10, intPtr(10), 1.0},
		{"nil max", 5, nil, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &db.Gateway{CurrentUsers: tt.current, MaxUsers: tt.maxUsers}
			got := calculateGatewayLoad(gw)
			if got != tt.wantLoad {
				t.Errorf("calculateGatewayLoad() = %v, want %v", got, tt.wantLoad)
			}
		})
	}
}

func TestHashString(t *testing.T) {
	h1 := hashString("abc")
	h2 := hashString("abc")
	if h1 != h2 {
		t.Error("hashString should be deterministic")
	}
	if h1 < 0 || h1 >= 100 {
		t.Errorf("hashString should return 0-99, got %d", h1)
	}
}

func TestClampPercentage(t *testing.T) {
	if clampPercentage(-1) != 0 {
		t.Error("clampPercentage(-1) should be 0")
	}
	if clampPercentage(150) != 100 {
		t.Error("clampPercentage(150) should be 100")
	}
	if clampPercentage(50) != 50 {
		t.Error("clampPercentage(50) should be 50")
	}
}

func TestGetLoadBalancedGateways(t *testing.T) {
	ctx := context.Background()
	database := mustTestDB(t)
	defer database.Close()

	balancer := NewBalancer(database)
	gateways, err := balancer.GetLoadBalancedGateways(ctx, "us-east-1", 5)
	if err != nil {
		t.Fatalf("GetLoadBalancedGateways: %v", err)
	}
	if len(gateways) != 0 {
		t.Errorf("expected 0 gateways, got %d", len(gateways))
	}
}

func TestSelectRegion(t *testing.T) {
	ctx := context.Background()
	database := mustTestDB(t)
	defer database.Close()

	balancer := NewBalancer(database)
	region, err := balancer.SelectRegion(ctx, "", nil)
	if err != nil {
		t.Fatalf("SelectRegion: %v", err)
	}
	if region != "us-east-1" {
		t.Errorf("SelectRegion: got %q, want us-east-1", region)
	}
}

func TestGetRolloutPercentage(t *testing.T) {
	ctx := context.Background()
	database := mustTestDB(t)
	defer database.Close()

	balancer := NewBalancer(database)
	pct, err := balancer.GetRolloutPercentage(ctx, "1.0", "us-east-1")
	if err != nil {
		t.Fatalf("GetRolloutPercentage: %v", err)
	}
	if pct != 100 {
		t.Errorf("GetRolloutPercentage: got %d, want 100", pct)
	}
}

func TestShouldIncludeInRollout(t *testing.T) {
	ctx := context.Background()
	database := mustTestDB(t)
	defer database.Close()

	balancer := NewBalancer(database)
	incl, err := balancer.ShouldIncludeInRollout(ctx, "client-1", "1.0", "us-east-1")
	if err != nil {
		t.Fatalf("ShouldIncludeInRollout: %v", err)
	}
	if !incl {
		t.Error("ShouldIncludeInRollout: expected true for 100% rollout")
	}
}

func intPtr(n int) *int { return &n }

func mustTestDB(t *testing.T) *db.Database {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

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
