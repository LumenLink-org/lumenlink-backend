package db

import (
	"os"
	"testing"
)

func TestRunMigrations_SkipWithoutDB(t *testing.T) {
	// Skip if no test database configured
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping migration tests")
	}

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	err := RunMigrations(databaseURL)
	if err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	version, dirty, err := GetMigrationVersion(databaseURL)
	if err != nil {
		t.Fatalf("GetMigrationVersion: %v", err)
	}
	if dirty {
		t.Error("database should not be dirty after migrations")
	}
	if version < 1 {
		t.Errorf("expected version >= 1, got %d", version)
	}
}
