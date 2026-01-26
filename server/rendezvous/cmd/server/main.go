package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/unitech-for-good/lumenlink/rendezvous/internal/api"
	"github.com/unitech-for-good/lumenlink/rendezvous/internal/attestation"
	"github.com/unitech-for-good/lumenlink/rendezvous/internal/config"
	"github.com/unitech-for-good/lumenlink/rendezvous/internal/db"
	"github.com/unitech-for-good/lumenlink/rendezvous/internal/geo"
)

func main() {
	// Initialize database
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable not set")
	}

	// Run database migrations
	log.Println("Running database migrations...")
	if err := db.RunMigrations(databaseURL); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Database migrations completed")

	// Initialize database connection
	ctx := context.Background()
	database, err := db.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Initialize Redis
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		log.Fatal("REDIS_URL environment variable not set")
	}

	// Initialize services
	configService, err := config.NewConfigService(database)
	if err != nil {
		log.Fatalf("Failed to initialize config service: %v", err)
	}
	attestationService := attestation.NewAttestationService()
	geoBalancer := geo.NewGeoBalancer()

	// Initialize API handler
	handler := api.NewHandler(configService, attestationService, geoBalancer, database)

	// Setup router
	router := gin.Default()

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// API routes - using /api/v1 to match frontend expectations
	apiGroup := router.Group("/api/v1")
	{
		apiGroup.POST("/config", handler.GetConfig)
		apiGroup.POST("/attest", handler.VerifyAttestation)
		apiGroup.POST("/gateway/status", handler.HandleGatewayStatus)
		apiGroup.POST("/discovery/log", handler.HandleDiscoveryLog)
		apiGroup.GET("/gateways", handler.GetGateways) // Community page endpoint
	}

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Graceful shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exited")
}
