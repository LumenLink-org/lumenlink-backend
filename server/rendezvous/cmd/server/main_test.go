package main

import (
	"os"
	"testing"
)

func TestCheckProductionAttestationGuard(t *testing.T) {
	tests := []struct {
		name     string
		goEnv    string
		bypass   string
		wantErr  bool
	}{
		{"dev with bypass ok", "development", "true", false},
		{"dev empty bypass ok", "development", "", false},
		{"empty env with bypass ok", "", "true", false},
		{"production with bypass fails", "production", "true", true},
		{"production with bypass 1 fails", "production", "1", true},
		{"production with bypass yes fails", "production", "yes", true},
		{"production with bypass false ok", "production", "false", false},
		{"production with bypass empty ok", "production", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("GO_ENV", tt.goEnv)
			os.Setenv("LUMENLINK_ALLOW_ATTESTATION_BYPASS", tt.bypass)
			defer func() {
				os.Unsetenv("GO_ENV")
				os.Unsetenv("LUMENLINK_ALLOW_ATTESTATION_BYPASS")
			}()
			err := checkProductionAttestationGuard()
			if (err != nil) != tt.wantErr {
				t.Errorf("checkProductionAttestationGuard() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
