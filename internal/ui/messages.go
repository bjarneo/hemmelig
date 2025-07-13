package ui

import (
	"net"

	"github.com/bjarneo/jot/internal/protocol"
)

// --- Generic Messages ---

type ErrorMsg struct{ Err error }
type ConnectionClosedMsg struct{}

// --- UI Interaction Messages ---

type FocusTextareaMsg struct{}
type SubmitInputMsg struct{ Content string }

// --- Network/Connection Related Messages ---

type ConnectionMsg struct{ Conn net.Conn }

type UserJoinedMsg struct {
	UserID    string
	Nickname  string
	PublicKey []byte
}

type UserLeftMsg struct {
	UserID string
}

type PublicKeyMsg struct {
	UserID    string
	Nickname  string
	PublicKey []byte
}

type ReceivedTextMsg struct {
	SenderID   string
	Ciphertext []byte
}

// --- File Transfer Messages ---

type FileOfferMsg struct {
	Metadata protocol.FileMetadata
	SenderID string
}
type FileOfferAcceptedMsg struct {
	Metadata protocol.FileMetadata
	SenderID string
}
type FileOfferRejectedMsg struct{ SenderID string }
type FileOfferFailedMsg struct{ Reason string }
type FileSendingCompleteMsg struct{}
type FileChunkMsg struct{ Chunk []byte }
type FileDoneMsg struct{}
type FileTransferProgress float64
