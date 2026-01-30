package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"
	"rendezvous/internal/api"
	"rendezvous/internal/attestation"
	"rendezvous/internal/config"
	"rendezvous/internal/db"
	"rendezvous/internal/geo"
	_ "rendezvous/internal/metrics"
)

func main() {
	// Production: disable Gin debug mode (prevents stack trace leaks)
	if strings.ToLower(os.Getenv("GO_ENV")) == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Production guard: fail fast if attestation bypass is enabled in production
	if err := checkProductionAttestationGuard(); err != nil {
		log.Fatal(err)
	}

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

	// Production: fail fast if config signing key is missing
	if strings.ToLower(os.Getenv("GO_ENV")) == "production" {
		if os.Getenv("LUMENLINK_ALLOW_EPHEMERAL_SIGNING_KEY") == "true" {
			log.Fatal("LUMENLINK_ALLOW_EPHEMERAL_SIGNING_KEY must be false in production")
		}
		if os.Getenv("LUMENLINK_CONFIG_SIGNING_PRIVATE_KEY") == "" {
			log.Fatal("LUMENLINK_CONFIG_SIGNING_PRIVATE_KEY is required in production")
		}
	}

	// Initialize services
	configService, err := config.NewConfigService(database)
	if err != nil {
		log.Fatalf("Failed to initialize config service: %v", err)
	}
	attestationService := attestation.NewAttestationService(database)
	geoBalancer := geo.NewBalancer(database)

	// Initialize API handler
	handler := api.NewHandler(configService, attestationService, geoBalancer, database)

	// Setup router
	router := gin.Default()

	// Security headers (all responses)
	router.Use(securityHeaders())

	// CORS: strict in production, permissive in dev
	router.Use(corsMiddleware())

	// Health check (no rate limit)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// Prometheus metrics
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API routes - using /api/v1 to match frontend expectations (rate limited)
	apiLimiter := newRateLimiter(100, 10) // 100 req/min burst 10
	apiGroup := router.Group("/api/v1")
	apiGroup.Use(apiLimiter.middleware())
	{
		apiGroup.POST("/config", handler.GetConfig)
		apiGroup.GET("/attest/challenge", handler.GetAttestationChallenge)
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
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
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

// checkProductionAttestationGuard returns an error if attestation bypass is enabled in production.
// This prevents accidental deployment with LUMENLINK_ALLOW_ATTESTATION_BYPASS=true when GO_ENV=production.
func checkProductionAttestationGuard() error {
	if strings.ToLower(os.Getenv("GO_ENV")) != "production" {
		return nil
	}
	v := strings.ToLower(os.Getenv("LUMENLINK_ALLOW_ATTESTATION_BYPASS"))
	if v == "true" || v == "1" || v == "yes" {
		return fmt.Errorf("LUMENLINK_ALLOW_ATTESTATION_BYPASS must not be enabled in production (GO_ENV=production). Set it to false or remove it")
	}
	return nil
}

// securityHeaders adds security headers to all responses.
func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}

// corsMiddleware returns CORS config: strict in production, permissive in dev.
func corsMiddleware() gin.HandlerFunc {
	origins := os.Getenv("CORS_ALLOWED_ORIGINS")
	if origins == "" {
		if strings.ToLower(os.Getenv("GO_ENV")) == "production" {
			// Production default: only lumenlink.org
			origins = "https://lumenlink.org,https://www.lumenlink.org"
		} else {
			// Dev default: allow localhost
			origins = "http://localhost:3000,http://127.0.0.1:3000,http://localhost:3001"
		}
	}
	allowList := strings.Split(origins, ",")
	for i := range allowList {
		allowList[i] = strings.TrimSpace(allowList[i])
	}
	cfg := cors.Config{
		AllowOrigins:     allowList,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	return cors.New(cfg)
}

// rateLimiter provides per-client rate limiting.
type rateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	r        rate.Limit
	b        int
}

func newRateLimiter(perMin int, burst int) *rateLimiter {
	return &rateLimiter{
		limiters: make(map[string]*rate.Limiter),
		r:        rate.Limit(float64(perMin) / 60),
		b:        burst,
	}
}

func (rl *rateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.RLock()
	l, ok := rl.limiters[ip]
	rl.mu.RUnlock()
	if ok {
		return l
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	l, ok = rl.limiters[ip]
	if !ok {
		l = rate.NewLimiter(rl.r, rl.b)
		rl.limiters[ip] = l
	}
	return l
}

func (rl *rateLimiter) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if ip == "" {
			ip = "unknown"
		}
		limiter := rl.getLimiter(ip)
		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate_limit_exceeded"})
			return
		}
		c.Next()
	}
}
