package ui

import (
	"net"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/dothash/hemmelig-cli/internal/protocol"
)

// --- Bubbletea Messages ---

type (
	ConnectionMsg        struct{ Conn net.Conn }
	SharedKeyMsg         struct{ Key []byte }
	ReceivedNicknameMsg  struct{ Nickname string }
	ReceivedTextMsg      struct{ Text string }
	FileOfferMsg         struct{ Metadata protocol.FileMetadata }
	FileOfferAcceptedMsg struct{ Metadata protocol.FileMetadata } // Sent from receiver to sender
	FileOfferRejectedMsg struct{}
	FileChunkMsg         struct{ Chunk []byte }
	FileDoneMsg          struct{}
	ProgressMsg          progress.FrameMsg
	ErrorMsg             struct{ Err error }
)
