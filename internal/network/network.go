package network

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/dothash/hemmelig-cli/internal/core"
	"github.com/dothash/hemmelig-cli/internal/crypto"
	"github.com/dothash/hemmelig-cli/internal/protocol"
)

// ListenAndServe starts a TCP listener and handles incoming connections.


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
			if err != io.EOF {
				sender.SendError(fmt.Errorf("connection closed by peer: %w", err))
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
func SendData(conn net.Conn, sharedKey []byte, msgType byte, data []byte) error {
	encrypted, err := crypto.Encrypt(data, sharedKey)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	msg := append([]byte{msgType}, binary.BigEndian.AppendUint32(nil, uint32(len(encrypted)))...)
	msg = append(msg, encrypted...)

	_, err = conn.Write(msg)
	return err
}
