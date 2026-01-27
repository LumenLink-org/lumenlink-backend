package attestation

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"time"

	"rendezvous/internal/db"
	// "github.com/unitech-for-good/lumenlink/rendezvous/internal/db"
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
	playIntegrityAPIKey string
	appleTeamID         string
	appleKeyID          string
	allowBypass         bool
}

// NewAttestationService creates a new attestation service
func NewAttestationService(database *db.Database) *AttestationService {
	return &AttestationService{
		db:                  database,
		playIntegrityAPIKey: os.Getenv("PLAY_INTEGRITY_API_KEY"),
		appleTeamID:         os.Getenv("APPLE_TEAM_ID"),
		appleKeyID:          os.Getenv("APPLE_KEY_ID"),
		allowBypass:         envAllowsBypass(),
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
		return &AttestationResult{
			IsValid: false,
			Reason:  "unsupported_platform",
		}, nil
	}

	if err != nil {
		return &AttestationResult{
			IsValid: false,
			Reason:  "verification_error",
		}, err
	}

	// Store attestation record in database
	if err := s.storeAttestation(ctx, req, result); err != nil {
		// Log error but don't fail verification
		// TODO: Add logging
	}

	return result, nil
}

// verifyPlayIntegrity verifies Android Play Integrity API token
func (s *AttestationService) verifyPlayIntegrity(
	ctx context.Context,
	req *AttestationRequest,
) (*AttestationResult, error) {
	// PlayIntegrityVerdict represents the actual response from Google
	type PlayIntegrityVerdict struct {
		RequestDetails struct {
			RequestPackageName string `json:"requestPackageName"`
			TimestampMillis    int64  `json:"timestampMillis"`
		} `json:"requestDetails"`
		AppIntegrity struct {
			AppRecognitionVerdict string `json:"appRecognitionVerdict"`
			PackageName           string `json:"packageName"`
			CertificateSha256     []string `json:"certificateSha256Digest"`
		} `json:"appIntegrity"`
		DeviceIntegrity struct {
			DeviceRecognitionVerdict []string `json:"deviceRecognitionVerdict"`
		} `json:"deviceIntegrity"`
		AccountDetails struct {
			AppLicensingVerdict string `json:"appLicensingVerdict"`
		} `json:"accountDetails"`
	}

	result := &AttestationResult{
		Platform:  "android",
		DeviceID:  req.DeviceID,
		Timestamp: time.Now(),
	}

	// In production, we would use the Google Cloud API client:
	// verdict, err := s.playIntegrityClient.Integritytoken.Decode(packageName, req.Token).Do()
	
	// For testing and initial launch, we implement a validation check that can be
	// toggled. Real verification requires Google Cloud API credentials.
	
	var tokenData map[string]interface{}
	if err := json.Unmarshal([]byte(req.Token), &tokenData); err != nil {
		// If not valid JSON (which the real token is not, it's a signed JWS),
		// we assume it's a real token and check if API is configured.
		if s.playIntegrityAPIKey == "" {
			if s.allowBypass {
				result.IsValid = true
				result.DeviceIntegrity = "BYPASS_ENABLED"
				return result, nil
			}
			result.IsValid = false
			result.Reason = "missing_play_integrity_config"
			return result, nil
		}
		result.IsValid = false
		result.Reason = "play_integrity_unimplemented"
		return result, nil
	}

	// Dev/Mock mode: if the token is JSON, parse the mocked integrity fields
	if integrity, ok := tokenData["deviceIntegrity"].(string); ok {
		result.DeviceIntegrity = integrity
		if integrity == "MEETS_STRONG_INTEGRITY" || integrity == "MEETS_BASIC_INTEGRITY" {
			result.IsValid = true
		} else {
			result.IsValid = false
			result.Reason = "device_compromised"
		}
	} else {
		result.IsValid = false
		result.Reason = "invalid_mock_token"
	}

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

	// iOS App Attest verification logic:
	// 1. Verify the CBOR attestation object
	// 2. Validate the certificate chain from Apple's Root CA
	// 3. Extract the public key and associate it with the DeviceID
	
	if req.Token == "" || req.KeyID == "" {
		result.IsValid = false
		result.Reason = "missing_token_or_keyid"
		return result, nil
	}

	if s.appleTeamID == "" || s.appleKeyID == "" {
		if s.allowBypass {
			result.IsValid = true
			result.DeviceIntegrity = "BYPASS_ENABLED"
			return result, nil
		}
		result.IsValid = false
		result.Reason = "missing_dcappattest_config"
		return result, nil
	}

	// For production, integrate with a CBOR/COSE library to parse the attestation object
	result.IsValid = false
	result.Reason = "dcappattest_unimplemented"

	return result, nil
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
