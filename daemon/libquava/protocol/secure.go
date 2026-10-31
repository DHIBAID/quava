package protocol

import (
	"crypto/cipher"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

// Once a session is authenticated, every frame in both directions is sealed
// with ChaCha20-Poly1305. Each direction has its own key and a counter that
// is used as the nonce, so a frame can neither be read, altered, replayed,
// reordered nor reflected back at its sender without the connection failing.
const (
	// FrameKeySize is the length in bytes of a per-direction frame key.
	FrameKeySize = chacha20poly1305.KeySize

	frameOverhead = chacha20poly1305.Overhead
)

// ErrDecrypt means a frame failed authentication. The connection is unusable
// afterwards: the peer is not speaking the encrypted protocol, or someone
// tampered with the stream.
var ErrDecrypt = errors.New("protocol: frame failed authentication (is the other side up to date?)")

var frameAssociatedData = []byte("quava-frame-v1")

type frameCipher struct {
	aead    cipher.AEAD
	counter uint64
}

func newFrameCipher(key []byte) (*frameCipher, error) {
	if len(key) != FrameKeySize {
		return nil, fmt.Errorf("protocol: frame key must be %d bytes", FrameKeySize)
	}

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}

	return &frameCipher{aead: aead}, nil
}

// nextNonce returns the nonce for the next frame: 4 zero bytes followed by the
// big-endian counter. Counters never repeat for a given key.
func (f *frameCipher) nextNonce() ([]byte, error) {
	if f.counter == ^uint64(0) {
		return nil, errors.New("protocol: frame counter exhausted")
	}

	nonce := make([]byte, f.aead.NonceSize())
	for i := 0; i < 8; i++ {
		nonce[len(nonce)-1-i] = byte(f.counter >> (8 * i))
	}
	f.counter++

	return nonce, nil
}

func (f *frameCipher) seal(plaintext []byte) ([]byte, error) {
	nonce, err := f.nextNonce()
	if err != nil {
		return nil, err
	}

	return f.aead.Seal(nil, nonce, plaintext, frameAssociatedData), nil
}

func (f *frameCipher) open(sealed []byte) ([]byte, error) {
	nonce, err := f.nextNonce()
	if err != nil {
		return nil, err
	}

	plaintext, err := f.aead.Open(nil, nonce, sealed, frameAssociatedData)
	if err != nil {
		return nil, ErrDecrypt
	}

	return plaintext, nil
}
