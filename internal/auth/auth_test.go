package auth

import (
	"testing"
	"time"
)

func TestGenerateAndValidateLicense(t *testing.T) {
	secret := "test-secret-key"
	slug := "test-plugin"
	clientID := "user-123"
	expiry := time.Now().Add(1 * time.Hour)

	// Test Generation
	key, err := GenerateLicenseKey(slug, clientID, expiry, secret)
	if err != nil {
		t.Fatalf("Failed to generate license: %v", err)
	}

	if key == "" {
		t.Fatal("Generated key is empty")
	}

	// Test Valid Validation
	payload, err := ValidateLicenseKey(key, slug, secret)
	if err != nil {
		t.Fatalf("Failed to validate valid license: %v", err)
	}

	if payload.ClientID != clientID {
		t.Errorf("Expected client ID %s, got %s", clientID, payload.ClientID)
	}

	// Test Invalid Secret
	_, err = ValidateLicenseKey(key, slug, "wrong-secret")
	if err == nil {
		t.Error("Validation should fail with wrong secret")
	}

	// Test Wrong Slug
	_, err = ValidateLicenseKey(key, "other-plugin", secret)
	if err == nil {
		t.Error("Validation should fail with wrong slug")
	}

	// Test Expiry
	expiredTime := time.Now().Add(-1 * time.Hour)
	expiredKey, _ := GenerateLicenseKey(slug, clientID, expiredTime, secret)
	_, err = ValidateLicenseKey(expiredKey, slug, secret)
	if err == nil || err.Error() != "license key expired" {
		t.Error("Validation should fail for expired key")
	}
}

func TestAnonymizeIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"192.168.1.50", "192.168.x.x"},
		{"8.8.8.8", "8.8.x.x"},
		{"127.0.0.1", "127.0.x.x"},
		{"2001:db8:85a3:0000:0000:8a2e:0370:7334", "2001:db8:x:x:x:x:x:x"},
		{"invalid-ip", "unknown"},
	}

	for _, tt := range tests {
		result := AnonymizeIP(tt.input)
		if result != tt.expected {
			t.Errorf("AnonymizeIP(%s) = %s; want %s", tt.input, result, tt.expected)
		}
	}
}
