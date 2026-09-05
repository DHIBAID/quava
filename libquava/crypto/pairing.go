package crypto

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	ProtocolName        = "quava-pairing"
	ProtocolVersionInfo = "quava-pairing-v1"
)

// Identity contains the long-term Ed25519 identity key pair.
type Identity struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
}

// GenerateIdentityKeyPair creates a fresh Ed25519 identity key pair.
func GenerateIdentityKeyPair() (Identity, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, fmt.Errorf("generate ed25519 identity: %w", err)
	}
	return Identity{PrivateKey: privateKey, PublicKey: publicKey}, nil
}

// DeviceIDFromPublicKey derives the canonical 16-byte device identifier.
func DeviceIDFromPublicKey(publicKey ed25519.PublicKey) string {
	sum := sha256.Sum256(publicKey)
	return hex.EncodeToString(sum[:16])
}

// DeviceIDBytes returns the 16-byte binary device identifier.
func DeviceIDBytes(publicKey ed25519.PublicKey) []byte {
	sum := sha256.Sum256(publicKey)
	return append([]byte(nil), sum[:16]...)
}

// GenerateTransactionID returns a fresh 16-byte transaction ID.
func GenerateTransactionID() ([]byte, error) {
	transactionID := make([]byte, 16)
	if _, err := rand.Read(transactionID); err != nil {
		return nil, fmt.Errorf("generate transaction id: %w", err)
	}
	return transactionID, nil
}

// GenerateNonce returns a fresh 32-byte nonce.
func GenerateNonce() ([]byte, error) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return nonce, nil
}

// GenerateX25519KeyPair creates a fresh X25519 key pair.
func GenerateX25519KeyPair() ([]byte, []byte, error) {
	curve := ecdh.X25519()
	privateKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate x25519 key pair: %w", err)
	}
	return privateKey.Bytes(), privateKey.PublicKey().Bytes(), nil
}

// X25519SharedSecret derives the shared secret from an X25519 key pair.
func X25519SharedSecret(privateKey []byte, peerPublicKey []byte) ([]byte, error) {
	curve := ecdh.X25519()
	privateKeyObject, err := curve.NewPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("parse x25519 private key: %w", err)
	}
	publicKeyObject, err := curve.NewPublicKey(peerPublicKey)
	if err != nil {
		return nil, fmt.Errorf("parse x25519 public key: %w", err)
	}
	sharedSecret, err := privateKeyObject.ECDH(publicKeyObject)
	if err != nil {
		return nil, fmt.Errorf("derive x25519 shared secret: %w", err)
	}
	return sharedSecret, nil
}

// HKDFSHA256 derives key material using HKDF-SHA-256.
func HKDFSHA256(salt, ikm, info []byte, length int) ([]byte, error) {
	reader := hkdf.New(sha256.New, ikm, salt, info)
	derived := make([]byte, length)
	if _, err := reader.Read(derived); err != nil {
		return nil, fmt.Errorf("hkdf-sha256: %w", err)
	}
	return derived, nil
}

// VerificationCode calculates the six-digit pairing code from the authenticated transcript inputs.
func VerificationCode(initiatorPublicKey, responderPublicKey, initiatorNonce, responderNonce, transactionID []byte) uint32 {
	input := make([]byte, 0, len("quava-pairing-code")+len(initiatorPublicKey)+len(responderPublicKey)+len(initiatorNonce)+len(responderNonce)+len(transactionID))
	input = append(input, []byte("quava-pairing-code")...)
	input = append(input, initiatorPublicKey...)
	input = append(input, responderPublicKey...)
	input = append(input, initiatorNonce...)
	input = append(input, responderNonce...)
	input = append(input, transactionID...)

	digest := sha256.Sum256(input)
	value := uint32(digest[0])<<24 | uint32(digest[1])<<16 | uint32(digest[2])<<8 | uint32(digest[3])
	return value % 1_000_000
}

// TranscriptDigest returns SHA-256 over the canonical transcript bytes.
func TranscriptDigest(transcript []byte) []byte {
	sum := sha256.Sum256(transcript)
	return append([]byte(nil), sum[:]...)
}

// SignTranscript signs the transcript digest with Ed25519.
func SignTranscript(privateKey ed25519.PrivateKey, transcript []byte) []byte {
	digest := TranscriptDigest(transcript)
	signature := ed25519.Sign(privateKey, digest)
	return append([]byte(nil), signature...)
}

// VerifyTranscript verifies an Ed25519 signature over the transcript digest.
func VerifyTranscript(publicKey ed25519.PublicKey, transcript, signature []byte) error {
	digest := TranscriptDigest(transcript)
	if !ed25519.Verify(publicKey, digest, signature) {
		return fmt.Errorf("ed25519 verification failed")
	}
	return nil
}

// TranscriptBytes builds the transcript used for authentication and verification.
func TranscriptBytes(selectedVersion uint64, transactionID []byte, initiatorDeviceID, responderDeviceID, initiatorPublicKey, responderPublicKey, initiatorNonce, responderNonce, initiatorEphemeralPublic, responderEphemeralPublic []byte) []byte {
	transcript := make([]byte, 0, len(ProtocolName)+len(transactionID)+len(initiatorDeviceID)+len(responderDeviceID)+len(initiatorPublicKey)+len(responderPublicKey)+len(initiatorNonce)+len(responderNonce)+len(initiatorEphemeralPublic)+len(responderEphemeralPublic)+8)
	transcript = append(transcript, []byte(ProtocolName)...)
	transcript = append(transcript, byte(selectedVersion>>56), byte(selectedVersion>>48), byte(selectedVersion>>40), byte(selectedVersion>>32), byte(selectedVersion>>24), byte(selectedVersion>>16), byte(selectedVersion>>8), byte(selectedVersion))
	transcript = append(transcript, transactionID...)
	transcript = append(transcript, initiatorDeviceID...)
	transcript = append(transcript, responderDeviceID...)
	transcript = append(transcript, initiatorPublicKey...)
	transcript = append(transcript, responderPublicKey...)
	transcript = append(transcript, initiatorNonce...)
	transcript = append(transcript, responderNonce...)
	transcript = append(transcript, initiatorEphemeralPublic...)
	transcript = append(transcript, responderEphemeralPublic...)
	return transcript
}

// MasterSecret derives the 32-byte master secret for the transaction.
func MasterSecret(initiatorNonce, responderNonce, sharedSecret []byte) ([]byte, error) {
	saltInput := make([]byte, 0, len(initiatorNonce)+len(responderNonce))
	saltInput = append(saltInput, initiatorNonce...)
	saltInput = append(saltInput, responderNonce...)
	salt := sha256.Sum256(saltInput)
	return HKDFSHA256(salt[:], sharedSecret, []byte(ProtocolVersionInfo), 32)
}

// PeerCredential derives the persistent peer credential from the master secret.
func PeerCredential(transactionID, masterSecret []byte) ([]byte, error) {
	salt := sha256.Sum256(transactionID)
	return HKDFSHA256(salt[:], masterSecret, []byte("quava-peer-credential-v1"), 32)
}

// ConfirmationMAC calculates the pair-complete confirmation MAC.
func ConfirmationMAC(masterSecret, transactionID []byte) []byte {
	mac := hmac.New(sha256.New, masterSecret)
	mac.Write([]byte("quava-pair-complete"))
	mac.Write(transactionID)
	return mac.Sum(nil)
}

// SessionCipher encrypts a payload with ChaCha20-Poly1305.
func SessionCipher(key, plaintext, additionalData []byte) ([]byte, error) {
	cipher, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("chacha20poly1305: %w", err)
	}
	nonce := make([]byte, cipher.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate session nonce: %w", err)
	}
	sealed := cipher.Seal(append([]byte(nil), nonce...), nonce, plaintext, additionalData)
	return sealed, nil
}

// SessionDecipher decrypts a payload sealed by SessionCipher.
func SessionDecipher(key, ciphertext, additionalData []byte) ([]byte, error) {
	cipher, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("chacha20poly1305: %w", err)
	}
	if len(ciphertext) < cipher.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce := ciphertext[:cipher.NonceSize()]
	sealed := ciphertext[cipher.NonceSize():]
	plaintext, err := cipher.Open(nil, nonce, sealed, additionalData)
	if err != nil {
		return nil, fmt.Errorf("decrypt session payload: %w", err)
	}
	return plaintext, nil
}
