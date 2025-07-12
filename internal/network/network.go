package network

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors" // Added missing import
	"fmt"
	"io"
	"net"

	"github.com/dothash/hemmelig-cli/internal/core"
	"github.com/dothash/hemmelig-cli/internal/crypto"
	"github.com/dothash/hemmelig-cli/internal/protocol"
)

// ListenForMessages reads and processes incoming messages from the connection.
func ListenForMessages(conn net.Conn, key []byte, sender core.MessageSender, isInitiator bool) {
	reader := bufio.NewReader(conn)

	// Perform key exchange if key is not provided (first message from peer)
	var sharedKey []byte
	var myPublicKey []byte
	var peerPublicKey []byte
	var err error

	if key == nil {
		sharedKey, myPublicKey, peerPublicKey, err = crypto.PerformKeyExchange(conn, isInitiator)
		if err != nil {
			sender.SendError(err)
			return
		}
		sender.SendSharedKey(sharedKey)
		sender.SendMyPublicKey(myPublicKey)
		sender.SendPeerPublicKey(peerPublicKey)
	} else {
		sharedKey = key
	}

	for {
		msgType, err := reader.ReadByte()
		if err != nil {
			// If we get an EOF, it means the connection was closed.
			// This could be the server terminating an inactive session.
			if err == io.EOF {
				sender.SendConnectionClosed()
			} else {
				sender.SendError(fmt.Errorf("connection read error: %w", err))
			}
			return
		}

		var length uint32
		if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
			sender.SendError(fmt.Errorf("failed to read length: %w", err))
			return
		}

		encryptedMsg := make([]byte, length)
		if _, err := io.ReadFull(reader, encryptedMsg); err != nil {
			sender.SendError(fmt.Errorf("failed to read message body: %w", err))
			return
		}

		decrypted, err := crypto.Decrypt(encryptedMsg, sharedKey)
		if err != nil {
			sender.SendError(fmt.Errorf("decryption failed: %w", err))
			continue
		}

		switch msgType {
		case protocol.TypeNickname:
			sender.SendReceivedNickname(string(decrypted))

		case protocol.TypeText:
			sender.SendReceivedText(string(decrypted))
		case protocol.TypeFileOffer:
			var meta protocol.FileMetadata
			if err := json.Unmarshal(decrypted, &meta); err != nil {
				sender.SendError(fmt.Errorf("failed to decode file offer: %w", err))
				continue
			}
			sender.SendFileOffer(meta)
		case protocol.TypeFileAccept:
			var meta protocol.FileMetadata
			if err := json.Unmarshal(decrypted, &meta); err != nil {
				sender.SendError(fmt.Errorf("failed to decode file acceptance: %w", err))
				continue
			}
			sender.SendFileOfferAccepted(meta)
		case protocol.TypeFileReject:
			sender.SendFileOfferRejected()
		case protocol.TypeFileChunk:
			sender.SendFileChunk(decrypted)
		case protocol.TypeFileDone:
			sender.SendFileDone()
		default:
			sender.SendError(fmt.Errorf("received unknown message type: %d", msgType))
		}
	}
}

// SendData encrypts and sends data over the connection.
// For TypePublicKeyExchange, data is sent unencrypted.
func SendData(conn net.Conn, sharedKey []byte, msgType byte, data []byte) error {
	var payloadToSend []byte
	var err error

	if msgType == protocol.TypePublicKeyExchange {
		payloadToSend = data // Send raw public key for exchange
	} else {
		if sharedKey == nil {
			// This check is important. If sharedKey is nil for other types, it's an error.
			return errors.New("shared key is nil, cannot encrypt non-PublicKeyExchange message")
		}
		payloadToSend, err = crypto.Encrypt(data, sharedKey)
		if err != nil {
			return fmt.Errorf("encryption failed: %w", err)
		}
	}

	// Prepend msgType and length
	msgHeader := make([]byte, 1+4) // 1 byte for type, 4 bytes for length
	msgHeader[0] = msgType
	binary.BigEndian.PutUint32(msgHeader[1:], uint32(len(payloadToSend)))

	fullMsg := append(msgHeader, payloadToSend...)

	_, err = conn.Write(fullMsg)
	return err
}
