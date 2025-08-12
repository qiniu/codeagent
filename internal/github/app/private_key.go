package app

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

// PrivateKeyLoader handles loading RSA private keys from various sources
type PrivateKeyLoader struct{}

// NewPrivateKeyLoader creates a new private key loader
func NewPrivateKeyLoader() *PrivateKeyLoader {
	return &PrivateKeyLoader{}
}

// LoadFromFile loads RSA private key from a file path
func (l *PrivateKeyLoader) LoadFromFile(path string) (*rsa.PrivateKey, error) {
	if path == "" {
		return nil, fmt.Errorf("private key file path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file %s: %w", path, err)
	}

	return l.LoadFromBytes(data)
}

// LoadFromEnv loads RSA private key from environment variable
func (l *PrivateKeyLoader) LoadFromEnv(envVar string) (*rsa.PrivateKey, error) {
	if envVar == "" {
		return nil, fmt.Errorf("environment variable name is empty")
	}

	data := os.Getenv(envVar)
	if data == "" {
		return nil, fmt.Errorf("environment variable %s is empty or not set", envVar)
	}

	return l.LoadFromBytes([]byte(data))
}

// LoadFromBytes loads RSA private key from byte data
func (l *PrivateKeyLoader) LoadFromBytes(data []byte) (*rsa.PrivateKey, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("private key data is empty")
	}

	// Parse PEM block
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block containing private key")
	}

	// Check if it's a private key
	if block.Type != "RSA PRIVATE KEY" && block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("invalid PEM block type: %s, expected RSA PRIVATE KEY or PRIVATE KEY", block.Type)
	}

	// Parse the private key
	var privateKey *rsa.PrivateKey
	var err error

	if block.Type == "RSA PRIVATE KEY" {
		// PKCS#1 format
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse PKCS#1 private key: %w", err)
		}
	} else {
		// PKCS#8 format
		parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse PKCS#8 private key: %w", err)
		}

		var ok bool
		privateKey, ok = parsedKey.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA format")
		}
	}

	// Validate the key
	if err := privateKey.Validate(); err != nil {
		return nil, fmt.Errorf("invalid RSA private key: %w", err)
	}

	return privateKey, nil
}

// LoadPrivateKey loads private key with fallback strategy
// Priority: file path -> environment variable -> direct content
func LoadPrivateKey(filePath, envVar, content string) (*rsa.PrivateKey, error) {
	loader := NewPrivateKeyLoader()

	// Try loading from file first
	if filePath != "" {
		key, err := loader.LoadFromFile(filePath)
		if err == nil {
			return key, nil
		}
		// Log error but continue to try other methods
		fmt.Printf("Warning: failed to load private key from file %s: %v\n", filePath, err)
	}

	// Try loading from environment variable
	if envVar != "" {
		key, err := loader.LoadFromEnv(envVar)
		if err == nil {
			return key, nil
		}
		// Log error but continue to try other methods
		fmt.Printf("Warning: failed to load private key from env var %s: %v\n", envVar, err)
	}

	// Try loading from direct content
	if content != "" {
		key, err := loader.LoadFromBytes([]byte(content))
		if err == nil {
			return key, nil
		}
		return nil, fmt.Errorf("failed to load private key from content: %w", err)
	}

	return nil, fmt.Errorf("no valid private key source provided")
}