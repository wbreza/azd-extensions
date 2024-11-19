package internal

import (
	"testing"
)

func TestSignatureManager(t *testing.T) {
	privateKeyPath := "../private_key.pem"
	publicKeyPath := "../public_key.pem"
	data := []byte("This is test data.")

	sm, err := NewSignatureManagerFromFiles(privateKeyPath, publicKeyPath)
	if err != nil {
		t.Fatalf("Failed to create SignatureManager: %v", err)
	}

	// Test signing
	signature, err := sm.Sign(data)
	if err != nil {
		t.Fatalf("Failed to sign data: %v", err)
	}

	// Test verification
	err = sm.Verify(data, signature)
	if err != nil {
		t.Fatalf("Signature verification failed: %v", err)
	}
}
