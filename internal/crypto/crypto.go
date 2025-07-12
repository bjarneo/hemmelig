package crypto

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/dothash/hemmelig-cli/internal/protocol" // Added for protocol.TypePublicKeyExchange
	"golang.org/x/crypto/curve25519"
)

const (
	maxPublicKeySize = 32               // Size of Curve25519 public keys
	maxMessageSize   = 10 * 1024 * 1024 // 10MB, arbitrary limit for other messages
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

// Helper for PerformKeyExchange to read one TLV message (unencrypted payload)
// The caller is responsible for providing a buffered reader.
func readTLVFromConn(reader *bufio.Reader) (byte, []byte, error) {
	msgType, err := reader.ReadByte()
	if err != nil {
		return 0, nil, fmt.Errorf("readTLV: failed to read msgType: %w", err)
	}

	var length uint32
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
		return 0, nil, fmt.Errorf("readTLV: failed to read length: %w", err)
	}

	// Safety limit for public key payload
	if msgType == protocol.TypePublicKeyExchange {
		if length != maxPublicKeySize {
			return 0, nil, fmt.Errorf("readTLV: public key exchange payload must be 32 bytes, got %d", length)
		}
	} else {
		// Generic length check for other types, if this function were to be used more broadly.
		// For now, it's only used for TypePublicKeyExchange.
		if length > maxMessageSize {
			return 0, nil, fmt.Errorf("readTLV: message length %d too large", length)
		}
		if length == 0 && msgType != protocol.TypeFileReject { // TypeFileReject can have 0 length data
			return 0, nil, errors.New("readTLV: received zero length payload for message type that requires data")
		}
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return 0, nil, fmt.Errorf("readTLV: failed to read payload: %w", err)
	}
	return msgType, payload, nil
}

// PerformKeyExchange performs a Curve25519 key exchange using TLV-formatted messages for public keys.
// It returns the shared key, the user's public key, and the peer's public key.
func PerformKeyExchange(conn io.ReadWriter, isInitiator bool) ([]byte, []byte, []byte, error) {
	var privateKey, publicKey [32]byte
	if _, err := rand.Read(privateKey[:]); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}
	curve25519.ScalarBaseMult(&publicKey, &privateKey)

	var theirPublicKeyBytes [32]byte

	// Create a buffered reader for the connection once.
	// conn is an io.ReadWriter; we need io.Reader for bufio.NewReader.
	// We also need io.Writer for sending.
	reader := bufio.NewReader(conn)
	writer := conn // conn itself is an io.Writer

	if isInitiator {
		// Initiator sends its public key first (TLV, unencrypted)
		payloadToSend := publicKey[:]
		msgHeader := make([]byte, 1+4) // 1 byte for type, 4 bytes for length
		msgHeader[0] = protocol.TypePublicKeyExchange
		binary.BigEndian.PutUint32(msgHeader[1:], uint32(len(payloadToSend)))
		fullMsg := append(msgHeader, payloadToSend...)

		if _, err := writer.Write(fullMsg); err != nil {
			return nil, nil, nil, fmt.Errorf("initiator failed to send public key: %w", err)
		}

		// Then, initiator receives peer's key (TLV, unencrypted)
		recvMsgType, recvPayload, err := readTLVFromConn(reader) // Pass the bufio.Reader
		if err != nil {
			return nil, nil, nil, fmt.Errorf("initiator failed to read peer's public key: %w", err)
		}
		if recvMsgType != protocol.TypePublicKeyExchange {
			return nil, nil, nil, fmt.Errorf("initiator expected TypePublicKeyExchange, got %d", recvMsgType)
		}
		if len(recvPayload) != 32 {
			return nil, nil, nil, fmt.Errorf("initiator received peer public key of wrong size: %d", len(recvPayload))
		}
		copy(theirPublicKeyBytes[:], recvPayload)

	} else { // Responder
		// Responder receives peer's key first (TLV, unencrypted)
		recvMsgType, recvPayload, err := readTLVFromConn(reader) // Pass the bufio.Reader
		if err != nil {
			return nil, nil, nil, fmt.Errorf("responder failed to read peer's public key: %w", err)
		}
		if recvMsgType != protocol.TypePublicKeyExchange {
			return nil, nil, nil, fmt.Errorf("responder expected TypePublicKeyExchange, got %d", recvMsgType)
		}
		if len(recvPayload) != 32 {
			return nil, nil, nil, fmt.Errorf("responder received peer public key of wrong size: %d", len(recvPayload))
		}
		copy(theirPublicKeyBytes[:], recvPayload)

		// Then, responder sends its public key (TLV, unencrypted)
		payloadToSend := publicKey[:]
		msgHeader := make([]byte, 1+4) // 1 byte for type, 4 bytes for length
		msgHeader[0] = protocol.TypePublicKeyExchange
		binary.BigEndian.PutUint32(msgHeader[1:], uint32(len(payloadToSend)))
		fullMsg := append(msgHeader, payloadToSend...)

		if _, err := writer.Write(fullMsg); err != nil {
			return nil, nil, nil, fmt.Errorf("responder failed to send public key: %w", err)
		}
	}

	sharedKeyVal, err := curve25519.X25519(privateKey[:], theirPublicKeyBytes[:])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to compute shared key: %w", err)
	}

	return sharedKeyVal, publicKey[:], theirPublicKeyBytes[:], nil
}
