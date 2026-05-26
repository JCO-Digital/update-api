package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type LicensePayload struct {
	ClientID string `json:"cid"`
	Expires  int64  `json:"exp"`
	Slug     string `json:"slug"`
}

func GenerateLicenseKey(slug, clientID string, expiresAt time.Time, secret string) (string, error) {
	payload := LicensePayload{
		ClientID: clientID,
		Expires:  expiresAt.Unix(),
		Slug:     slug,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	payloadBase64 := base64.StdEncoding.EncodeToString(payloadBytes)

	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payloadBase64))
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))

	return fmt.Sprintf("%s.%s", payloadBase64, signature), nil
}

func ValidateLicenseKey(key, slug, secret string) (*LicensePayload, error) {
	parts := strings.Split(key, ".")
	if len(parts) != 2 {
		return nil, errors.New("invalid license key format")
	}

	payloadBase64 := parts[0]
	signature := parts[1]

	// Verify signature
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payloadBase64))
	expectedSignature := base64.StdEncoding.EncodeToString(h.Sum(nil))

	if signature != expectedSignature {
		return nil, errors.New("invalid license signature")
	}

	// Decode payload
	payloadBytes, err := base64.StdEncoding.DecodeString(payloadBase64)
	if err != nil {
		return nil, errors.New("invalid payload encoding")
	}

	var payload LicensePayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, errors.New("invalid payload data")
	}

	// Verify slug
	if payload.Slug != slug {
		return nil, errors.New("license key for different plugin")
	}

	// Verify expiration
	if time.Now().Unix() > payload.Expires {
		return nil, errors.New("license key expired")
	}

	return &payload, nil
}
