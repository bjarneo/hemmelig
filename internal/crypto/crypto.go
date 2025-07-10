package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/curve25519"
)

// Encrypt encrypts plaintext using AES-GCM with the given key.
func Encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts ciphertext using AES-GCM with the given key.
func Decrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}
	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, actualCiphertext, nil)
}

// PerformKeyExchange performs a Curve25519 key exchange.
// It returns the shared key and the peer's public key.
func PerformKeyExchange(conn io.ReadWriter, isInitiator bool) ([]byte, []byte, error) {
	var privateKey, publicKey [32]byte
	if _, err := rand.Read(privateKey[:]); err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}
	curve25519.ScalarBaseMult(&publicKey, &privateKey)

	var theirPublicKey [32]byte
	if isInitiator {
		// Initiator sends its public key first, then receives peer's
		if _, err := conn.Write(publicKey[:]); err != nil {
			return nil, nil, fmt.Errorf("failed to send public key: %w", err)
		}
		if _, err := io.ReadFull(conn, theirPublicKey[:]); err != nil {
			return nil, nil, fmt.Errorf("failed to receive public key: %w", err)
		}
	} else {
		// Responder receives peer's public key first, then sends its own
		if _, err := io.ReadFull(conn, theirPublicKey[:]); err != nil {
			return nil, nil, fmt.Errorf("failed to receive public key: %w", err)
		}
		if _, err := conn.Write(publicKey[:]); err != nil {
			return nil, nil, fmt.Errorf("failed to send public key: %w", err)
		}
	}

	sharedKey, err := curve25519.X25519(privateKey[:], theirPublicKey[:])
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compute shared key: %w", err)
	}

	return sharedKey, theirPublicKey[:], nil
}
