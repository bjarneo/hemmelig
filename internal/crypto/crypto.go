package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"

	"github.com/cloudflare/circl/dh/x25519"
)

// GenerateKeyPair generates a new X25519 key pair.
func GenerateKeyPair() (x25519.Key, error) {
	var privateKey x25519.Key
	if _, err := rand.Read(privateKey[:]); err != nil {
		return x25519.Key{}, err
	}
	return privateKey, nil
}

// ComputeSharedSecret computes the shared secret between a private key and a public key.
func ComputeSharedSecret(privateKey, publicKey x25519.Key) ([]byte, error) {
	var sharedKey x25519.Key
	x25519.Shared(&sharedKey, &privateKey, &publicKey)
	return sharedKey[:], nil
}

// Encrypt encrypts data using AES-GCM.
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

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts data using AES-GCM.
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
		return nil, io.ErrUnexpectedEOF
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
