package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"rendezvous/internal/db"
)

func main() {
	var (
		databaseURL = flag.String("database-url", "", "PostgreSQL database URL")
		command     = flag.String("command", "up", "Migration command: up, down, version")
	)
	flag.Parse()

	// Get database URL from flag or environment
	if *databaseURL == "" {
		*databaseURL = os.Getenv("DATABASE_URL")
		if *databaseURL == "" {
			log.Fatal("DATABASE_URL environment variable or -database-url flag required")
		}
	}

	switch *command {
	case "up":
		fmt.Println("Running database migrations...")
		if err := db.RunMigrations(*databaseURL); err != nil {
			log.Fatalf("Migration failed: %v", err)
		}
		fmt.Println("Migrations completed successfully")

	case "down":
		fmt.Println("Rolling back last migration...")
		if err := db.RollbackLastMigration(*databaseURL); err != nil {
			log.Fatalf("Rollback failed: %v", err)
		}
		fmt.Println("Rollback completed successfully")

	case "version":
		version, dirty, err := db.GetMigrationVersion(*databaseURL)
		if err != nil {
			log.Fatalf("Failed to get version: %v", err)
		}
		if dirty {
			fmt.Printf("Current version: %d (DIRTY - manual intervention required)\n", version)
		} else {
			fmt.Printf("Current version: %d\n", version)
		}

	default:
		log.Fatalf("Unknown command: %s. Use: up, down, or version", *command)
	}
}
