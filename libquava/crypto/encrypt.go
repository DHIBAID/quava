package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func HashSHA256(data string, key []byte) []byte {
	normalized := strings.ToLower(strings.TrimSpace(data))

	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(normalized))

	return mac.Sum(nil)
}

func GenerateHMAC(data string, identifier uuid.UUID, encryptionKey string) (string, error) {
	if encryptionKey == "" {
		return "", fmt.Errorf("secret key cannot be empty")
	}
	if identifier == uuid.Nil {
		return "", fmt.Errorf("identifier cannot be empty")
	}

	timestamp := time.Now().Unix()
	payload := fmt.Sprintf("%s|%d|%s", data, timestamp, identifier.String())

	h := hmac.New(sha256.New, []byte(encryptionKey))
	h.Write([]byte(payload))
	signature := h.Sum(nil)

	encodedPayload := fmt.Sprintf("%s.%s", payload, base64.URLEncoding.EncodeToString(signature))
	return base64.URLEncoding.EncodeToString([]byte(encodedPayload)), nil
}

func VerifyHMAC(hmacToken string, encryptionKey string, ttlMinutes int) (data string, identifier uuid.UUID, err error) {
	if encryptionKey == "" {
		return "", uuid.Nil, fmt.Errorf("secret key cannot be empty")
	}
	if strings.TrimSpace(hmacToken) == "" {
		return "", uuid.Nil, fmt.Errorf("HMAC token cannot be empty")
	}

	decoded, err := base64.URLEncoding.DecodeString(hmacToken)
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("invalid HMAC token format")
	}

	parts := strings.Split(string(decoded), ".")
	if len(parts) != 2 {
		return "", uuid.Nil, fmt.Errorf("invalid HMAC token structure")
	}

	payload := parts[0]
	signatureStr := parts[1]

	signature, err := base64.URLEncoding.DecodeString(signatureStr)
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("invalid signature format")
	}

	h := hmac.New(sha256.New, []byte(encryptionKey))
	h.Write([]byte(payload))
	expectedSignature := h.Sum(nil)
	if !hmac.Equal(signature, expectedSignature) {
		return "", uuid.Nil, fmt.Errorf("invalid HMAC signature")
	}

	dataParts := strings.Split(payload, "|")
	if len(dataParts) != 3 {
		return "", uuid.Nil, fmt.Errorf("invalid data format")
	}

	data = dataParts[0]

	timestamp, err := strconv.ParseInt(dataParts[1], 10, 64)
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("invalid timestamp format")
	}

	parsedIdentifier, err := uuid.Parse(dataParts[2])
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("invalid userID format")
	}

	if ttlMinutes < 0 {
		ttlMinutes = 0
	}
	currentTime := time.Now().Unix()
	expiryTime := timestamp + int64(ttlMinutes*60)
	if currentTime > expiryTime {
		return "", uuid.Nil, fmt.Errorf("HMAC has expired")
	}

	return data, parsedIdentifier, nil
}

func GenerateSharedKey(baseSecret string) (string, error) {
	if strings.TrimSpace(baseSecret) == "" {
		return "", fmt.Errorf("secret key cannot be empty")
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate shared key: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func ShareKeyWithHMAC(identifier uuid.UUID, encryptionKey, sharedKey string) (string, error) {
	return GenerateHMAC(sharedKey, identifier, encryptionKey)
}

func VerifySharedKeyEnvelope(hmacToken string, encryptionKey string, ttlMinutes int) (sharedKey string, identifier uuid.UUID, err error) {
	return VerifyHMAC(hmacToken, encryptionKey, ttlMinutes)
}

func EncryptPayload(key, plaintext string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("secret key cannot be empty")
	}
	if plaintext == "" {
		return "", nil
	}

	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return "", fmt.Errorf("aes.NewCipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cipher.NewGCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func DecryptPayload(key, payload string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("secret key cannot be empty")
	}
	if strings.TrimSpace(payload) == "" {
		return "", nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("decode payload: %w", err)
	}

	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return "", fmt.Errorf("aes.NewCipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cipher.NewGCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, encrypted := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt payload: %w", err)
	}

	return string(plaintext), nil
}
