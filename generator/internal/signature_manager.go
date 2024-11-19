package internal

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
)

// SignatureManager handles signing and verification of arbitrary data
type SignatureManager struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
}

// NewSignatureManagerFromFiles creates a SignatureManager from private and public key file paths
func NewSignatureManagerFromFiles(privateKeyPath, publicKeyPath string) (*SignatureManager, error) {
	privateKey, err := loadPrivateKey(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %w", err)
	}

	publicKey, err := loadPublicKey(publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load public key: %w", err)
	}

	return &SignatureManager{
		privateKey: privateKey,
		publicKey:  publicKey,
	}, nil
}

// Sign signs the given data and returns the Base64-encoded signature
func (sm *SignatureManager) Sign(data []byte) (string, error) {
	// Compute the SHA256 hash of the data
	hash := sha256.Sum256(data)

	// Sign the hash with the private key
	signature, err := rsa.SignPKCS1v15(nil, sm.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign data: %w", err)
	}

	// Encode the signature to Base64
	return base64.StdEncoding.EncodeToString(signature), nil
}

// Verify verifies the given data and its Base64-encoded signature
func (sm *SignatureManager) Verify(data []byte, signature string) error {
	// Compute the SHA256 hash of the data
	hash := sha256.Sum256(data)

	// Decode the Base64-encoded signature
	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// Verify the signature with the public key
	err = rsa.VerifyPKCS1v15(sm.publicKey, crypto.SHA256, hash[:], sigBytes)
	if err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	return nil
}

// loadPrivateKey loads an RSA private key, supporting both PKCS#1 and PKCS#8 formats
func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode private key PEM")
	}

	// Attempt to parse as PKCS#1
	if privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return privateKey, nil
	}

	// Attempt to parse as PKCS#8
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Ensure the parsed key is an RSA private key
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not an RSA key")
	}

	return rsaKey, nil
}

// loadPublicKey loads an RSA public key from a PEM file
func loadPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("invalid public key PEM format")
	}

	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	rsaPubKey, ok := publicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not RSA")
	}

	return rsaPubKey, nil
}
