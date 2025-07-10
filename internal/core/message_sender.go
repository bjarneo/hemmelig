package core

import (
	"net"

	"github.com/dothash/hemmelig-cli/internal/protocol"
)

// MessageSender defines the interface for sending messages to the UI.
type MessageSender interface {
	SendError(err error)
	SendConnection(conn net.Conn)
	SendSharedKey(key []byte)
	SendReceivedNickname(nickname string)
	SendReceivedText(text string)
	SendFileOffer(metadata protocol.FileMetadata)
	SendFileOfferAccepted(metadata protocol.FileMetadata)
	SendFileOfferRejected()
	SendFileChunk(chunk []byte)
	SendFileDone()
	SendProgress(percent float64)
	SendPeerPublicKey(publicKey []byte)
	SendMyPublicKey(publicKey []byte)
}
