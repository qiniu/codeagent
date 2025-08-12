package app

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivateKeyLoader_LoadFromBytes(t *testing.T) {
	loader := NewPrivateKeyLoader()

	// Generate test key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	t.Run("PKCS#1 format", func(t *testing.T) {
		// Encode in PKCS#1 format
		pkcs1Data := x509.MarshalPKCS1PrivateKey(privateKey)
		pemBlock := &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: pkcs1Data,
		}
		pemData := pem.EncodeToMemory(pemBlock)

		loadedKey, err := loader.LoadFromBytes(pemData)
		require.NoError(t, err)
		assert.NotNil(t, loadedKey)
		assert.Equal(t, privateKey.N, loadedKey.N)
	})

	t.Run("PKCS#8 format", func(t *testing.T) {
		// Encode in PKCS#8 format
		pkcs8Data, err := x509.MarshalPKCS8PrivateKey(privateKey)
		require.NoError(t, err)
		pemBlock := &pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: pkcs8Data,
		}
		pemData := pem.EncodeToMemory(pemBlock)

		loadedKey, err := loader.LoadFromBytes(pemData)
		require.NoError(t, err)
		assert.NotNil(t, loadedKey)
		assert.Equal(t, privateKey.N, loadedKey.N)
	})

	t.Run("empty data", func(t *testing.T) {
		_, err := loader.LoadFromBytes([]byte{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})

	t.Run("invalid PEM", func(t *testing.T) {
		_, err := loader.LoadFromBytes([]byte("not a pem"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode PEM")
	})

	t.Run("invalid PEM block type", func(t *testing.T) {
		pemBlock := &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: []byte("invalid"),
		}
		pemData := pem.EncodeToMemory(pemBlock)

		_, err := loader.LoadFromBytes(pemData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid PEM block type")
	})
}

func TestPrivateKeyLoader_LoadFromFile(t *testing.T) {
	loader := NewPrivateKeyLoader()

	// Generate test key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Create temporary file
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "test_key.pem")

	// Write key to file
	pkcs1Data := x509.MarshalPKCS1PrivateKey(privateKey)
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: pkcs1Data,
	}
	pemData := pem.EncodeToMemory(pemBlock)
	err = os.WriteFile(keyFile, pemData, 0600)
	require.NoError(t, err)

	t.Run("valid file", func(t *testing.T) {
		loadedKey, err := loader.LoadFromFile(keyFile)
		require.NoError(t, err)
		assert.NotNil(t, loadedKey)
		assert.Equal(t, privateKey.N, loadedKey.N)
	})

	t.Run("empty path", func(t *testing.T) {
		_, err := loader.LoadFromFile("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := loader.LoadFromFile("/nonexistent/path")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read")
	})
}

func TestPrivateKeyLoader_LoadFromEnv(t *testing.T) {
	loader := NewPrivateKeyLoader()

	// Generate test key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Encode key
	pkcs1Data := x509.MarshalPKCS1PrivateKey(privateKey)
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: pkcs1Data,
	}
	pemData := pem.EncodeToMemory(pemBlock)

	envVar := "TEST_PRIVATE_KEY"
	
	t.Run("valid env var", func(t *testing.T) {
		// Set environment variable
		os.Setenv(envVar, string(pemData))
		defer os.Unsetenv(envVar)

		loadedKey, err := loader.LoadFromEnv(envVar)
		require.NoError(t, err)
		assert.NotNil(t, loadedKey)
		assert.Equal(t, privateKey.N, loadedKey.N)
	})

	t.Run("empty env var name", func(t *testing.T) {
		_, err := loader.LoadFromEnv("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})

	t.Run("unset env var", func(t *testing.T) {
		_, err := loader.LoadFromEnv("NONEXISTENT_ENV_VAR")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty or not set")
	})
}

func TestLoadPrivateKey(t *testing.T) {
	// Generate test key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Encode key
	pkcs1Data := x509.MarshalPKCS1PrivateKey(privateKey)
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: pkcs1Data,
	}
	pemData := pem.EncodeToMemory(pemBlock)

	// Create temporary file
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "test_key.pem")
	err = os.WriteFile(keyFile, pemData, 0600)
	require.NoError(t, err)

	envVar := "TEST_PRIVATE_KEY_FALLBACK"
	os.Setenv(envVar, string(pemData))
	defer os.Unsetenv(envVar)

	t.Run("file path priority", func(t *testing.T) {
		loadedKey, err := LoadPrivateKey(keyFile, envVar, string(pemData))
		require.NoError(t, err)
		assert.NotNil(t, loadedKey)
		assert.Equal(t, privateKey.N, loadedKey.N)
	})

	t.Run("env var fallback", func(t *testing.T) {
		loadedKey, err := LoadPrivateKey("", envVar, string(pemData))
		require.NoError(t, err)
		assert.NotNil(t, loadedKey)
		assert.Equal(t, privateKey.N, loadedKey.N)
	})

	t.Run("content fallback", func(t *testing.T) {
		loadedKey, err := LoadPrivateKey("", "", string(pemData))
		require.NoError(t, err)
		assert.NotNil(t, loadedKey)
		assert.Equal(t, privateKey.N, loadedKey.N)
	})

	t.Run("no sources", func(t *testing.T) {
		_, err := LoadPrivateKey("", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no valid private key source")
	})
}