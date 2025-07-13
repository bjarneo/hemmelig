package network

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors" // Added missing import
	"fmt"
	"io"
	"net"

	"github.com/bjarneo/jot/internal/core"
	"github.com/bjarneo/jot/internal/crypto"
	"github.com/bjarneo/jot/internal/protocol"
)

// ListenForMessages reads and processes incoming messages from the connection.
func ListenForMessages(conn net.Conn, sender core.MessageSender) {
	reader := bufio.NewReader(conn)

	for {
		messageBytes, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				sender.SendConnectionClosed()
			} else {
				sender.SendError(fmt.Errorf("connection read error: %w", err))
			}
			return
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(messageBytes, &msg); err != nil {
			sender.SendError(fmt.Errorf("failed to unmarshal message: %w", err))
			continue
		}

		switch msg["type"] {
		case "user_joined":
			sender.SendUserJoined(msg["userID"].(string), msg["nickname"].(string), []byte(msg["publicKey"].(string)))
		case "user_left":
			sender.SendUserLeft(msg["userID"].(string))
		case "public_key":
			sender.SendPublicKey(msg["userID"].(string), msg["nickname"].(string), []byte(msg["publicKey"].(string)))
		case "message":
			senderID := msg["sender"].(string)
			ciphertext := []byte(msg["ciphertext"].(string))
			sender.SendReceivedText(senderID, ciphertext)
		case "file_offer":
			senderID := msg["sender"].(string)
			var meta protocol.FileMetadata
			if err := json.Unmarshal([]byte(msg["metadata"].(string)), &meta); err != nil {
				sender.SendError(fmt.Errorf("failed to decode file offer: %w", err))
				continue
			}
			sender.SendFileOffer(meta, senderID)
		case "file_accept":
			var meta protocol.FileMetadata
			if err := json.Unmarshal([]byte(msg["metadata"].(string)), &meta); err != nil {
				sender.SendError(fmt.Errorf("failed to decode file acceptance: %w", err))
				continue
			}
			sender.SendFileOfferAccepted(meta)
		case "file_reject":
			senderID := msg["sender"].(string)
			sender.SendFileOfferRejected(senderID)
		case "file_chunk":
			sender.SendFileChunk([]byte(msg["chunk"].(string)))
		case "file_done":
			sender.SendFileDone()
		default:
			sender.SendError(fmt.Errorf("received unknown message type: %s", msg["type"]))
		}
	}
}

// SendData sends a JSON message over the connection.
func SendData(conn net.Conn, data interface{}) error {
	msgBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	_, err = conn.Write(append(msgBytes, '\n'))
	return err
}
