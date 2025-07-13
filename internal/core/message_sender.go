package core

import (
	"net"

	"github.com/bjarneo/jot/internal/protocol"
)

// MessageSender defines an interface for sending messages to the UI.
type MessageSender interface {
	SendError(err error)
	SendInfo(info string)
	SendConnection(conn net.Conn)
	SendUserJoined(userID, nickname string, publicKey []byte)
	SendUserLeft(userID string)
	SendPublicKey(userID, nickname string, publicKey []byte)
	SendReceivedText(senderID string, ciphertext []byte)
	SendFileOffer(metadata protocol.FileMetadata, senderID string)
	SendFileOfferAccepted(metadata protocol.FileMetadata)
	SendFileOfferRejected(senderID string)
	SendFileOfferFailed(reason string)
	SendFileSendingComplete()
	SendFileChunk(chunk []byte)
	SendFileDone()
	SendProgress(percent float64)
	SendConnectionClosed()
}
