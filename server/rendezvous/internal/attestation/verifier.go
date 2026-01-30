package attestation

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/option"
	playintegrity "google.golang.org/api/playintegrity/v1"

	"github.com/bas-d/appattest/attestation"
	"rendezvous/internal/db"
	"rendezvous/internal/metrics"
)

// AttestationResult represents the result of attestation verification
type AttestationResult struct {
	IsValid        bool
	DeviceIntegrity string
	Platform       string
	DeviceID       string
	Timestamp      time.Time
	Reason         string // If invalid, reason for failure
}

// AttestationRequest represents an attestation verification request
type AttestationRequest struct {
	Platform  string `json:"platform"`  // "android" or "ios"
	Token     string `json:"token"`      // Play Integrity token or DCAppAttest token
	DeviceID  string `json:"device_id"`
	KeyID     string `json:"key_id"`    // For iOS DCAppAttest
}

// AttestationService handles remote attestation verification
type AttestationService struct {
	db *db.Database
	// In production, these would be API clients for Play Integrity and DCAppAttest
	playIntegrityClient          *playintegrity.Service
	playIntegrityInitOnce        sync.Once
	playIntegrityInitErr         error
	playIntegrityPackageName     string
	playIntegrityAllowBasic      bool
	playIntegrityRequireLicensed bool
	playIntegrityMaxAge          time.Duration
	playIntegrityCredentialsFile string
	playIntegrityCredentialsJSON string

	appleTeamID     string
	appleBundleID   string
	appleProduction bool
	allowBypass     bool
}

// NewAttestationService creates a new attestation service
func NewAttestationService(database *db.Database) *AttestationService {
	playIntegrityMaxAge := 5 * time.Minute
	if maxAge := strings.TrimSpace(os.Getenv("PLAY_INTEGRITY_MAX_AGE_SECONDS")); maxAge != "" {
		if seconds, err := strconv.Atoi(maxAge); err == nil && seconds > 0 {
			playIntegrityMaxAge = time.Duration(seconds) * time.Second
		}
	}

	return &AttestationService{
		db:                          database,
		playIntegrityPackageName:     strings.TrimSpace(os.Getenv("PLAY_INTEGRITY_PACKAGE_NAME")),
		playIntegrityAllowBasic:      strings.ToLower(os.Getenv("PLAY_INTEGRITY_ALLOW_BASIC")) == "true",
		playIntegrityRequireLicensed: strings.ToLower(os.Getenv("PLAY_INTEGRITY_REQUIRE_LICENSED")) != "false",
		playIntegrityMaxAge:          playIntegrityMaxAge,
		playIntegrityCredentialsFile: strings.TrimSpace(os.Getenv("PLAY_INTEGRITY_CREDENTIALS_FILE")),
		playIntegrityCredentialsJSON: strings.TrimSpace(os.Getenv("PLAY_INTEGRITY_CREDENTIALS_JSON")),
		appleTeamID:     strings.TrimSpace(os.Getenv("APPLE_TEAM_ID")),
		appleBundleID:   strings.TrimSpace(os.Getenv("APPLE_BUNDLE_ID")),
		appleProduction: strings.ToLower(os.Getenv("APPLE_PRODUCTION")) != "false",
		allowBypass:     envAllowsBypass(),
	}
}

// VerifyAttestation verifies a device attestation token
func (s *AttestationService) VerifyAttestation(
	ctx context.Context,
	req *AttestationRequest,
) (*AttestationResult, error) {
	var result *AttestationResult
	var err error

	switch req.Platform {
	case "android":
		result, err = s.verifyPlayIntegrity(ctx, req)
	case "ios":
		result, err = s.verifyDCAppAttest(ctx, req)
	default:
		metrics.AttestationTotal.WithLabelValues(req.Platform, "invalid").Inc()
		metrics.AttestationFailures.WithLabelValues(req.Platform, "unsupported_platform").Inc()
		return &AttestationResult{
			IsValid: false,
			Reason:  "unsupported_platform",
		}, nil
	}

	if err != nil {
		metrics.AttestationTotal.WithLabelValues(req.Platform, "error").Inc()
		metrics.AttestationFailures.WithLabelValues(req.Platform, "verification_error").Inc()
		return &AttestationResult{
			IsValid: false,
			Reason:  "verification_error",
		}, err
	}

	if result.IsValid {
		metrics.AttestationTotal.WithLabelValues(req.Platform, "valid").Inc()
	} else {
		metrics.AttestationTotal.WithLabelValues(req.Platform, "invalid").Inc()
		metrics.AttestationFailures.WithLabelValues(req.Platform, result.Reason).Inc()
	}

	// Store attestation record in database
	if err := s.storeAttestation(ctx, req, result); err != nil {
		// Log error but don't fail verification
		log.Printf("attestation store failed for device=%s platform=%s: %v", req.DeviceID, req.Platform, err)
	}

	return result, nil
}

// verifyPlayIntegrity verifies Android Play Integrity API token
func (s *AttestationService) verifyPlayIntegrity(
	ctx context.Context,
	req *AttestationRequest,
) (*AttestationResult, error) {
	result := &AttestationResult{
		Platform:  "android",
		DeviceID:  req.DeviceID,
		Timestamp: time.Now(),
	}

	if err := s.initPlayIntegrityClient(ctx); err != nil {
		if s.allowBypass {
			result.IsValid = true
			result.DeviceIntegrity = "BYPASS_ENABLED"
			return result, nil
		}
		result.IsValid = false
		result.Reason = "play_integrity_not_configured"
		return result, nil
	}

	response, err := s.playIntegrityClient.V1.DecodeIntegrityToken(
		s.playIntegrityPackageName,
		&playintegrity.DecodeIntegrityTokenRequest{IntegrityToken: req.Token},
	).Do()
	if err != nil {
		result.IsValid = false
		result.Reason = "play_integrity_api_error"
		return result, err
	}

	payload := response.TokenPayloadExternal
	if payload == nil || payload.RequestDetails == nil {
		result.IsValid = false
		result.Reason = "missing_token_payload"
		return result, nil
	}

	if payload.RequestDetails.RequestPackageName != "" &&
		payload.RequestDetails.RequestPackageName != s.playIntegrityPackageName {
		result.IsValid = false
		result.Reason = "package_name_mismatch"
		return result, nil
	}

	if payload.RequestDetails.TimestampMillis != "" {
		if timestampMs, err := strconv.ParseInt(payload.RequestDetails.TimestampMillis, 10, 64); err == nil {
			tokenTime := time.UnixMilli(timestampMs)
			if time.Since(tokenTime) > s.playIntegrityMaxAge {
				result.IsValid = false
				result.Reason = "attestation_expired"
				return result, nil
			}
		}
	}

	if payload.AppIntegrity == nil || payload.AppIntegrity.AppRecognitionVerdict != "PLAY_RECOGNIZED" {
		result.IsValid = false
		result.Reason = "app_not_recognized"
		return result, nil
	}

	if s.playIntegrityRequireLicensed {
		if payload.AccountDetails == nil || payload.AccountDetails.AppLicensingVerdict != "LICENSED" {
			result.IsValid = false
			result.Reason = "app_not_licensed"
			return result, nil
		}
	}

	verdicts := []string{}
	if payload.DeviceIntegrity != nil {
		verdicts = payload.DeviceIntegrity.DeviceRecognitionVerdict
	}
	result.DeviceIntegrity = bestDeviceIntegrity(verdicts)

	if hasIntegrityVerdict(verdicts, "MEETS_STRONG_INTEGRITY") {
		result.IsValid = true
		return result, nil
	}
	if s.playIntegrityAllowBasic && hasIntegrityVerdict(verdicts, "MEETS_BASIC_INTEGRITY") {
		result.IsValid = true
		return result, nil
	}

	result.IsValid = false
	result.Reason = "device_integrity_failed"

	return result, nil
}

// verifyDCAppAttest verifies iOS DCAppAttest token
func (s *AttestationService) verifyDCAppAttest(
	ctx context.Context,
	req *AttestationRequest,
) (*AttestationResult, error) {
	result := &AttestationResult{
		Platform:  "ios",
		DeviceID:  req.DeviceID,
		Timestamp: time.Now(),
	}

	if req.Token == "" {
		result.IsValid = false
		result.Reason = "missing_token"
		return result, nil
	}

	appID := s.appleAppID()
	if appID == "" {
		if s.allowBypass {
			result.IsValid = true
			result.DeviceIntegrity = "BYPASS_ENABLED"
			return result, nil
		}
		result.IsValid = false
		result.Reason = "missing_dcappattest_config"
		return result, nil
	}

	var aar attestation.AuthenticatorAttestationResponse
	if err := json.Unmarshal([]byte(req.Token), &aar); err != nil {
		result.IsValid = false
		result.Reason = "invalid_attestation_format"
		return result, nil
	}

	publicKey, receipt, err := aar.Verify(appID, s.appleProduction)
	if err != nil {
		result.IsValid = false
		result.Reason = "dcappattest_verification_failed"
		return result, err
	}

	_ = receipt // Store receipt for fraud assessment if needed
	_ = publicKey // Store public key for assertion verification if needed

	result.IsValid = true
	result.DeviceIntegrity = "MEETS_STRONG_INTEGRITY"
	return result, nil
}

func (s *AttestationService) appleAppID() string {
	if s.appleTeamID == "" || s.appleBundleID == "" {
		return ""
	}
	return s.appleTeamID + "." + s.appleBundleID
}

// GenerateChallenge returns a random base64url-encoded challenge for iOS App Attest.
func (s *AttestationService) GenerateChallenge(ctx context.Context) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// storeAttestation stores attestation record in database
func (s *AttestationService) storeAttestation(
	ctx context.Context,
	req *AttestationRequest,
	result *AttestationResult,
) error {
	var verifiedAt sql.NullTime
	if result.IsValid {
		verifiedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}

	_, err := s.db.Pool().ExecContext(ctx, `
		INSERT INTO attestations (
			device_id, platform, token, verified, verified_at, device_integrity, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`, req.DeviceID, req.Platform, req.Token, result.IsValid, verifiedAt, result.DeviceIntegrity)
	return err
}

func envAllowsBypass() bool {
	value := strings.ToLower(os.Getenv("LUMENLINK_ALLOW_ATTESTATION_BYPASS"))
	return value == "1" || value == "true" || value == "yes"
}

// ShouldUseHoneypot determines if a client should receive honeypot gateways
func (s *AttestationService) ShouldUseHoneypot(result *AttestationResult) bool {
	if result == nil {
		return true // No attestation = use honeypot
	}

	if !result.IsValid {
		return true // Invalid attestation = use honeypot
	}

	// Check device integrity level
	if result.DeviceIntegrity != "MEETS_STRONG_INTEGRITY" {
		return true // Weak integrity = use honeypot
	}

	return false // Valid attestation with strong integrity = no honeypot
}

func (s *AttestationService) initPlayIntegrityClient(ctx context.Context) error {
	s.playIntegrityInitOnce.Do(func() {
		if s.playIntegrityPackageName == "" {
			s.playIntegrityInitErr = errors.New("missing package name")
			return
		}

		opts := []option.ClientOption{}
		if s.playIntegrityCredentialsFile != "" {
			opts = append(opts, option.WithCredentialsFile(s.playIntegrityCredentialsFile))
		} else if s.playIntegrityCredentialsJSON != "" {
			opts = append(opts, option.WithCredentialsJSON([]byte(s.playIntegrityCredentialsJSON)))
		}

		service, err := playintegrity.NewService(ctx, opts...)
		if err != nil {
			s.playIntegrityInitErr = err
			return
		}

		s.playIntegrityClient = service
	})

	return s.playIntegrityInitErr
}

func hasIntegrityVerdict(verdicts []string, verdict string) bool {
	for _, value := range verdicts {
		if value == verdict {
			return true
		}
	}
	return false
}

func bestDeviceIntegrity(verdicts []string) string {
	if hasIntegrityVerdict(verdicts, "MEETS_STRONG_INTEGRITY") {
		return "MEETS_STRONG_INTEGRITY"
	}
	if hasIntegrityVerdict(verdicts, "MEETS_DEVICE_INTEGRITY") {
		return "MEETS_DEVICE_INTEGRITY"
	}
	if hasIntegrityVerdict(verdicts, "MEETS_BASIC_INTEGRITY") {
		return "MEETS_BASIC_INTEGRITY"
	}
	return "UNKNOWN"
}
