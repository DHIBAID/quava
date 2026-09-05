package crypto

import (
	"testing"

	"github.com/google/uuid"
)

func TestGenerateAndVerifyHMACUsesIdentifier(t *testing.T) {
	identifier := uuid.New()
	token, err := GenerateHMAC("pair-check", identifier, "shared-secret")
	if err != nil {
		t.Fatalf("GenerateHMAC returned error: %v", err)
	}

	data, verifiedIdentifier, err := VerifyHMAC(token, "shared-secret", 5)
	if err != nil {
		t.Fatalf("VerifyHMAC returned error: %v", err)
	}
	if data != "pair-check" {
		t.Fatalf("data mismatch: got %q want %q", data, "pair-check")
	}
	if verifiedIdentifier != identifier {
		t.Fatalf("identifier mismatch: got %s want %s", verifiedIdentifier, identifier)
	}
}

func TestGenerateSharedKeyAndEncryptPayload(t *testing.T) {
	identifier := uuid.New()
	sharedKey, err := GenerateSharedKey("pair-secret")
	if err != nil {
		t.Fatalf("GenerateSharedKey returned error: %v", err)
	}

	token, err := ShareKeyWithHMAC(identifier, "pair-secret", sharedKey)
	if err != nil {
		t.Fatalf("ShareKeyWithHMAC returned error: %v", err)
	}

	verifiedKey, verifiedIdentifier, err := VerifySharedKeyEnvelope(token, "pair-secret", 5)
	if err != nil {
		t.Fatalf("VerifySharedKeyEnvelope returned error: %v", err)
	}
	if verifiedKey != sharedKey {
		t.Fatalf("verified key mismatch: got %q want %q", verifiedKey, sharedKey)
	}
	if verifiedIdentifier != identifier {
		t.Fatalf("verified identifier mismatch: got %s want %s", verifiedIdentifier, identifier)
	}

	ciphertext, err := EncryptPayload(sharedKey, "hello quava")
	if err != nil {
		t.Fatalf("EncryptPayload returned error: %v", err)
	}

	plaintext, err := DecryptPayload(sharedKey, ciphertext)
	if err != nil {
		t.Fatalf("DecryptPayload returned error: %v", err)
	}
	if plaintext != "hello quava" {
		t.Fatalf("plaintext mismatch: got %q want %q", plaintext, "hello quava")
	}
}
