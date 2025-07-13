package ui

import (
	"net"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/dothash/jot/internal/protocol"
)

// --- Bubbletea Messages ---

type (
	ConnectionMsg          struct{ Conn net.Conn }
	SharedKeyMsg           struct{ Key []byte }
	ReceivedNicknameMsg    struct{ Nickname string }
	ReceivedTextMsg        struct{ Text string }
	FileOfferMsg           struct{ Metadata protocol.FileMetadata }
	FileOfferAcceptedMsg   struct{ Metadata protocol.FileMetadata } // Sent from receiver to sender
	FileOfferRejectedMsg   struct{}
	FileOfferFailedMsg     struct{ Reason string }
	FileSendingCompleteMsg struct{}
	FileChunkMsg           struct{ Chunk []byte }
	FileDoneMsg            struct{}
	ProgressMsg            progress.FrameMsg
	FileTransferProgress   float64
	MyPublicKeyMsg         struct{ PublicKey []byte }
	PeerPublicKeyMsg       struct{ PublicKey []byte }
	ConnectionClosedMsg    struct{}
	ErrorMsg               struct{ Err error }
)
