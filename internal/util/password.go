package util

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// GenerateSecurePassword generates a cryptographically secure random password
func GenerateSecurePassword(length int) (string, error) {
	// Generate random bytes
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random password: %w", err)
	}

	// Encode as base64 (URL-safe variant to avoid special characters that might need escaping)
	encodedPassword := base64.URLEncoding.EncodeToString(bytes)

	// Trim to desired length (base64 encoding makes it slightly longer)
	if len(encodedPassword) > length {
		encodedPassword = encodedPassword[:length]
	}

	return encodedPassword, nil
}
