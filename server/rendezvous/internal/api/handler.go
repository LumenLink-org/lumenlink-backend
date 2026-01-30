package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"rendezvous/internal/attestation"
	"rendezvous/internal/config"
	"rendezvous/internal/db"
	"rendezvous/internal/geo"
	"rendezvous/internal/metrics"
)

// Handler handles HTTP API requests
type Handler struct {
	configService      *config.ConfigService
	attestationService *attestation.AttestationService
	geoBalancer        *geo.GeoBalancer
	database           *db.Database
}

var allowedGatewayStatuses = map[string]struct{}{
	"active":      {},
	"degraded":    {},
	"offline":     {},
	"maintenance": {},
}

var allowedDiscoveryChannels = map[string]struct{}{
	"gps":        {},
	"fm_rds":     {},
	"dtv":        {},
	"plc":        {},
	"gsm_cb":     {},
	"lte_sib":    {},
	"iot_mqtt":   {},
	"blockchain": {},
	"satellite":  {},
	"intranet":   {},
	"social":     {},
}

// NewHandler creates a new API handler
func NewHandler(
	configService *config.ConfigService,
	attestationService *attestation.AttestationService,
	geoBalancer *geo.GeoBalancer,
	database *db.Database,
) *Handler {
	return &Handler{
		configService:      configService,
		attestationService: attestationService,
		geoBalancer:        geoBalancer,
		database:           database,
	}
}

// GetConfigRequest represents a config request
type GetConfigRequest struct {
	DeviceID    string `json:"device_id" binding:"required"`
	Platform    string `json:"platform" binding:"required"`
	Region      string `json:"region"`
	Attestation string `json:"attestation"` // Attestation token
	Version     string `json:"version"`     // Client version
}

// GetConfigResponse represents a config response
type GetConfigResponse struct {
	ConfigPack *config.SignedConfigPack `json:"config_pack"`
}

// GetConfig handles config pack requests
func (h *Handler) GetConfig(c *gin.Context) {
	var req GetConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify attestation if provided
	var attestationResult *attestation.AttestationResult
	if req.Attestation != "" {
		attestReq := &attestation.AttestationRequest{
			Platform: req.Platform,
			Token:    req.Attestation,
			DeviceID: req.DeviceID,
		}
		
		result, err := h.attestationService.VerifyAttestation(c.Request.Context(), attestReq)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "attestation_verification_failed"})
			return
		}
		attestationResult = result
	}

	// Select region
	region := req.Region
	if region == "" {
		// Auto-detect region from Cloudflare or other CDN headers
		country := c.GetHeader("CF-IPCountry")
		if country != "" {
			region = h.mapCountryToRegion(country)
		} else {
			// Fallback to default region
			region = "us-east-1"
		}
	}

	// Convert attestation result to config package type
	var configAttestationResult *config.AttestationResult
	if attestationResult != nil {
		configAttestationResult = &config.AttestationResult{
			IsValid:         attestationResult.IsValid,
			DeviceIntegrity: attestationResult.DeviceIntegrity,
		}
	}

	// Generate config pack
	pack, err := h.configService.GenerateConfigPack(
		c.Request.Context(),
		req.DeviceID,
		region,
		configAttestationResult,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "config_generation_failed"})
		return
	}

	metrics.ConfigPackGenerated.WithLabelValues(region).Inc()

	c.JSON(http.StatusOK, GetConfigResponse{
		ConfigPack: pack,
	})
}

// mapCountryToRegion maps ISO country codes to infrastructure regions
func (h *Handler) mapCountryToRegion(country string) string {
	mapping := map[string]string{
		"CN": "ap-east-1",
		"IR": "me-south-1",
		"RU": "eu-central-1",
		"US": "us-east-1",
		"GB": "eu-west-1",
		"DE": "eu-central-1",
		// Add more mappings as needed
	}
	
	if region, ok := mapping[country]; ok {
		return region
	}
	return "us-east-1" // Default
}

// VerifyAttestationRequest represents an attestation verification request
type VerifyAttestationRequest struct {
	Platform string `json:"platform" binding:"required"`
	Token    string `json:"token" binding:"required"`
	DeviceID string `json:"device_id" binding:"required"`
	KeyID    string `json:"key_id"` // For iOS DCAppAttest
}

// VerifyAttestationResponse represents an attestation verification response
type VerifyAttestationResponse struct {
	Verified       bool   `json:"verified"`
	DeviceIntegrity string `json:"device_integrity,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

// GetAttestationChallenge returns a random challenge for iOS App Attest
func (h *Handler) GetAttestationChallenge(c *gin.Context) {
	challenge, err := h.attestationService.GenerateChallenge(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "challenge_generation_failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"challenge": challenge})
}

// VerifyAttestation handles attestation verification requests
func (h *Handler) VerifyAttestation(c *gin.Context) {
	var req VerifyAttestationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	attestReq := &attestation.AttestationRequest{
		Platform: req.Platform,
		Token:    req.Token,
		DeviceID: req.DeviceID,
		KeyID:    req.KeyID,
	}

	result, err := h.attestationService.VerifyAttestation(c.Request.Context(), attestReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "verification_failed"})
		return
	}

	response := VerifyAttestationResponse{
		Verified:        result.IsValid,
		DeviceIntegrity: result.DeviceIntegrity,
	}

	if !result.IsValid {
		response.Reason = result.Reason
		c.JSON(http.StatusUnauthorized, response)
		return
	}

	c.JSON(http.StatusOK, response)
}

// GatewayStatusRequest represents a gateway status update request
type GatewayStatusRequest struct {
	GatewayID        string  `json:"gateway_id" binding:"required"`
	Status           string  `json:"status" binding:"required"`
	UsersConnected   int     `json:"users_connected"`
	BandwidthUsedMbps int    `json:"bandwidth_used_mbps"`
	PacketsForwarded int64   `json:"packets_forwarded"`
	UptimePercent    float64 `json:"uptime_percent"`
}

// GatewayStatusResponse represents a gateway status response
type GatewayStatusResponse struct {
	Acknowledged bool `json:"acknowledged"`
}

// HandleGatewayStatus handles gateway status updates
func (h *Handler) HandleGatewayStatus(c *gin.Context) {
	var req GatewayStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if _, ok := allowedGatewayStatuses[req.Status]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_status"})
		return
	}
	if req.UsersConnected < 0 || req.BandwidthUsedMbps < 0 || req.PacketsForwarded < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_metrics"})
		return
	}
	if req.UptimePercent < 0 || req.UptimePercent > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_uptime"})
		return
	}

	if h.database != nil {
		err := h.database.RecordGatewayStatus(
			c.Request.Context(),
			req.GatewayID,
			req.Status,
			req.UsersConnected,
			req.BandwidthUsedMbps,
			req.PacketsForwarded,
			req.UptimePercent,
		)
		if err != nil {
			if errors.Is(err, db.ErrGatewayNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "gateway_not_found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gateway_status_store_failed"})
			return
		}
		metrics.GatewayStatusUpdates.Inc()
	}

	c.JSON(http.StatusOK, GatewayStatusResponse{
		Acknowledged: true,
	})
}

// DiscoveryLogRequest represents a discovery log entry
type DiscoveryLogRequest struct {
	ChannelType string `json:"channel_type" binding:"required"`
	GatewayID   string `json:"gateway_id,omitempty"`
	Success     bool   `json:"success"`
	LatencyMs   int    `json:"latency_ms,omitempty"`
	Error       string `json:"error,omitempty"`
}

// DiscoveryLogResponse represents a discovery log response
type DiscoveryLogResponse struct {
	Logged bool `json:"logged"`
}

// HandleDiscoveryLog handles discovery channel logs
func (h *Handler) HandleDiscoveryLog(c *gin.Context) {
	var req DiscoveryLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if _, ok := allowedDiscoveryChannels[req.ChannelType]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_channel_type"})
		return
	}
	if req.LatencyMs < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_latency"})
		return
	}

	if h.database != nil {
		var gatewayID *string
		if req.GatewayID != "" {
			gatewayID = &req.GatewayID
		}
		clientIP := c.ClientIP()
		var clientIPPtr *string
		if clientIP != "" {
			clientIPPtr = &clientIP
		}
		var regionPtr *string
		country := c.GetHeader("CF-IPCountry")
		if country != "" {
			region := h.mapCountryToRegion(country)
			regionPtr = &region
		}
		var latencyPtr *int
		if req.LatencyMs > 0 {
			latencyPtr = &req.LatencyMs
		}
		var errorPtr *string
		if req.Error != "" {
			errorPtr = &req.Error
		}

		if err := h.database.RecordDiscoveryLog(
			c.Request.Context(),
			req.ChannelType,
			gatewayID,
			clientIPPtr,
			regionPtr,
			req.Success,
			latencyPtr,
			errorPtr,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "discovery_log_store_failed"})
			return
		}
		successLabel := "false"
		if req.Success {
			successLabel = "true"
		}
		metrics.DiscoveryLogs.WithLabelValues(req.ChannelType, successLabel).Inc()
	}

	c.JSON(http.StatusOK, DiscoveryLogResponse{
		Logged: true,
	})
}

// GetGateways handles gateway listing requests for community page
func (h *Handler) GetGateways(c *gin.Context) {
	if h.database == nil {
		c.JSON(http.StatusOK, gin.H{
			"gateways": []interface{}{},
		})
		return
	}

	gateways, err := h.database.GetAllGateways(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to fetch gateways",
		})
		return
	}

	// Transform gateways to API response format
	gatewayList := make([]gin.H, 0, len(gateways))
	for _, gw := range gateways {
		// Calculate uptime from operator_metrics if available
		// For now, use a placeholder
		uptimePercent := 0.0
		if gw.LastSeen != nil {
			// Simple uptime calculation based on last seen
			// In production, this would query operator_metrics
			uptimePercent = 98.0 // Placeholder
		}

		callsign := "OP-unknown"
		if len(gw.ID) >= 8 {
			callsign = "OP-" + gw.ID[:8]
		} else if gw.ID != "" {
			callsign = "OP-" + gw.ID
		}
		gatewayList = append(gatewayList, gin.H{
			"id":            gw.ID,
			"callsign":      callsign,
			"region":        gw.Region,
			"status":        gw.Status,
			"current_users": gw.CurrentUsers,
			"max_users":     gw.MaxUsers,
			"last_seen":     gw.LastSeen,
			"uptime_percent": uptimePercent,
			// Note: lat/lng would come from a separate geolocation table
			// For now, we'll use region-based defaults
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"gateways": gatewayList,
	})
}
