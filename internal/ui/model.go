package ui

import (
	"bufio"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bjarneo/jot/internal/filetransfer"
	"github.com/bjarneo/jot/internal/network"
	"github.com/bjarneo/jot/internal/protocol"
)

type programMessageSender struct {
	program *tea.Program
}

func (pms *programMessageSender) SendError(err error) {
	pms.program.Send(ErrorMsg{Err: err})
}

func (pms *programMessageSender) SendInfo(info string) {
	pms.program.Send(InfoMsg{Info: info})
}

func (pms *programMessageSender) SendConnection(conn net.Conn) {
	pms.program.Send(ConnectionMsg{Conn: conn})
}

func (pms *programMessageSender) SendSharedKey(key []byte) {
	pms.program.Send(SharedKeyMsg{Key: key})
}

func (pms *programMessageSender) SendReceivedNickname(nickname string) {
	pms.program.Send(ReceivedNicknameMsg{Nickname: nickname})
}

func (pms *programMessageSender) SendReceivedText(text string) {
	pms.program.Send(ReceivedTextMsg{Text: text})
}

func (pms *programMessageSender) SendFileOffer(metadata protocol.FileMetadata) {
	pms.program.Send(FileOfferMsg{Metadata: metadata})
}

func (pms *programMessageSender) SendFileOfferAccepted(metadata protocol.FileMetadata) {
	pms.program.Send(FileOfferAcceptedMsg{Metadata: metadata})
}

func (pms *programMessageSender) SendFileOfferRejected() {
	pms.program.Send(FileOfferRejectedMsg{})
}

func (pms *programMessageSender) SendFileOfferFailed(reason string) {
	pms.program.Send(FileOfferFailedMsg{Reason: reason})
}

func (pms *programMessageSender) SendFileSendingComplete() {
	pms.program.Send(FileSendingCompleteMsg{})
}

func (pms *programMessageSender) SendFileChunk(chunk []byte) {
	pms.program.Send(FileChunkMsg{Chunk: chunk})
}

func (pms *programMessageSender) SendFileDone() {
	pms.program.Send(FileDoneMsg{})
}

func (pms *programMessageSender) SendProgress(percent float64) {
	pms.program.Send(FileTransferProgress(percent))
}

func (pms *programMessageSender) SendPeerPublicKey(publicKey []byte) {
	pms.program.Send(PeerPublicKeyMsg{PublicKey: publicKey})
}

func (pms *programMessageSender) SendMyPublicKey(publicKey []byte) {
	pms.program.Send(MyPublicKeyMsg{PublicKey: publicKey})
}

func (pms *programMessageSender) SendConnectionClosed() {
	pms.program.Send(ConnectionClosedMsg{})
}

type InfoMsg struct {
	Info string
}

// Model represents the Bubble Tea UI model.
type Model struct {
	RelayServerAddr string
	SessionID       string
	Command         string
	Status          string
	Conn            net.Conn
	SharedKey       []byte
	Err             error
	Program         *tea.Program

	Nickname     string
	PeerNickname string

	chatArea    ChatAreaModel
	Progress    progress.Model
	Messages    []Message
	IsReady     bool
	IsConnected bool

	IsTransferring       bool
	IsReceiving          bool
	IsAwaitingAcceptance bool
	PendingOffer         protocol.FileMetadata
	ReceivingFile        *os.File
	TotalBytesReceived   int64
	ShowHelp             bool
	PeerFingerprint      string
	MyFingerprint        string
	MaxFileSize          int64
}

func NewModel(relayServerAddr, sessionID, nickname, command string, maxFileSize int64) *Model {
	initialWidth := 80
	initialChatAreaHeight := 20

	ca := NewChatAreaModel(initialWidth, initialChatAreaHeight, nickname)
	prog := progress.New(progress.WithDefaultGradient())

	m := &Model{
		RelayServerAddr: relayServerAddr,
		SessionID:       sessionID,
		Nickname:        nickname,
		Status:          fmt.Sprintf("Connecting to relay server %s...", relayServerAddr),
		chatArea:        ca,
		Progress:        prog,
		Messages:        []Message{{Timestamp: time.Now(), Sender: "System", Content: "Waiting for connection..."}},
		Command:         command,
		MaxFileSize:     maxFileSize * 1024 * 1024,
	}
	return m
}

func (m *Model) Init() tea.Cmd {
	return func() tea.Msg {
		var conn net.Conn
		var err error
		if strings.HasPrefix(m.RelayServerAddr, "localhost:") {
			conn, err = net.Dial("tcp", m.RelayServerAddr)
		} else {
			conn, err = tls.Dial("tcp", m.RelayServerAddr, nil)
		}

		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to connect to relay server: %w", err)}
		}

		initialMsgStruct := struct {
			Command   string `json:"command"`
			SessionID string `json:"sessionID,omitempty"`
		}{
			Command:   m.Command,
			SessionID: m.SessionID,
		}

		msgBytes, err := json.Marshal(initialMsgStruct)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to marshal initial message: %w", err)}
		}

		_, err = conn.Write(append(msgBytes, '\n'))
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to send initial message to relay server: %w", err)}
		}

		reader := bufio.NewReader(conn)
		response, err := reader.ReadString('\n')
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to read response from relay server: %w", err)}
		}

		if strings.HasPrefix(response, "Error:") {
			return ErrorMsg{Err: fmt.Errorf("relay server error: %s", strings.TrimSpace(response))}
		}

		if strings.HasPrefix(response, "Session created:") {
			m.SessionID = strings.TrimSpace(strings.TrimPrefix(response, "Session created:"))
		}

		return ConnectionMsg{Conn: conn}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		chatAreaCmd tea.Cmd
		cmds        []tea.Cmd
	)

	if m.IsTransferring {
		var currentPgCmd tea.Cmd
		newProgress, currentPgCmd := m.Progress.Update(msg)
		if newProgressModel, ok := newProgress.(progress.Model); ok {
			m.Progress = newProgressModel
		}
		if currentPgCmd != nil {
			cmds = append(cmds, currentPgCmd)
		}
	}

	m.chatArea, chatAreaCmd = m.chatArea.Update(msg)
	if chatAreaCmd != nil {
		cmds = append(cmds, chatAreaCmd)
	}

	switch msg := msg.(type) {
	case SubmitInputMsg:
		text := strings.TrimSpace(msg.Content)
		if text == "" {
			return m, tea.Batch(cmds...)
		}

		if strings.HasPrefix(text, "/send ") {
			filePath := strings.TrimPrefix(text, "/send ")
			m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "System", Content: fmt.Sprintf("Offering to send file: %s", filePath)})
			m.IsAwaitingAcceptance = true
			m.Status = fmt.Sprintf("TRANSFERRING: Offering to send %s", filepath.Base(filePath))
			cmd := func() tea.Msg {
				filetransfer.RequestSendFile(m.Conn, m.SharedKey, filePath, &programMessageSender{program: m.Program}, m.MaxFileSize)
				return nil
			}
			cmds = append(cmds, cmd)
		} else if text == "/help" {
			m.ShowHelp = !m.ShowHelp
		} else if text == "/fingerprint" {
			now := time.Now()
			if m.MyFingerprint != "" {
				m.Messages = append(m.Messages, Message{Timestamp: now, Sender: "System", Content: fmt.Sprintf("Your Key Fingerprint: %s", m.MyFingerprint)})
			} else {
				m.Messages = append(m.Messages, Message{Timestamp: now, Sender: "System", Content: "Your Key Fingerprint is not yet available."})
			}
			if m.PeerFingerprint != "" {
				m.Messages = append(m.Messages, Message{Timestamp: now, Sender: "System", Content: fmt.Sprintf("Peer's Key Fingerprint: %s", m.PeerFingerprint)})
			} else {
				m.Messages = append(m.Messages, Message{Timestamp: now, Sender: "System", Content: "Peer is not connected or their fingerprint is not yet available."})
			}
		} else {
			m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: m.Nickname, Content: text})
			cmd := func() tea.Msg {
				if err := network.SendData(m.Conn, m.SharedKey, protocol.TypeText, []byte(text)); err != nil {
					return ErrorMsg{Err: err}
				}
				return nil
			}
			cmds = append(cmds, cmd)
		}

	case tea.KeyMsg:
		if m.ShowHelp {
			if msg.Type == tea.KeyEsc {
				m.ShowHelp = false
			}
		} else {
			switch msg.Type {
			case tea.KeyCtrlC, tea.KeyEsc:
				if m.Conn != nil {
					m.Conn.Close()
				}
				return m, tea.Quit
			case tea.KeyRunes:
				if m.PendingOffer.FileName != "" && len(msg.Runes) > 0 {
					switch msg.Runes[0] {
					case 'y', 'Y':
						m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "System", Content: "Accepting file transfer..."})
						metaBytes, _ := m.PendingOffer.ToJSON()
						cmd := func() tea.Msg {
							if err := network.SendData(m.Conn, m.SharedKey, protocol.TypeFileAccept, metaBytes); err != nil {
								return ErrorMsg{Err: err}
							}
							return nil
						}
						cmds = append(cmds, cmd)
						file, err := os.Create(filepath.Base(m.PendingOffer.FileName))
						if err != nil {
							m.Err = err
							return m, tea.Quit
						}
						m.IsTransferring = true
						m.IsReceiving = true
						m.ReceivingFile = file
						m.TotalBytesReceived = 0
						m.Progress.SetPercent(0)
					case 'n', 'N':
						m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "System", Content: "Rejected file transfer."})
						cmd := func() tea.Msg {
							if err := network.SendData(m.Conn, m.SharedKey, protocol.TypeFileReject, nil); err != nil {
								return ErrorMsg{Err: err}
							}
							return nil
						}
						cmds = append(cmds, cmd)
						m.PendingOffer = protocol.FileMetadata{}
					}
				}
			}
		}

	case tea.WindowSizeMsg:
		headerHeight := lipgloss.Height(m.headerView())
		var currentFooterHeight int
		if m.IsTransferring || m.PendingOffer.FileName != "" {
			currentFooterHeight = 1 + TextareaStyle.GetVerticalBorderSize()
		} else {
			currentFooterHeight = 0
		}
		verticalMargin := headerHeight + currentFooterHeight
		chatAreaHeight := msg.Height - verticalMargin
		if chatAreaHeight < 0 {
			chatAreaHeight = 0
		}
		m.chatArea.SetDimensions(msg.Width, chatAreaHeight)
		StatusStyle = StatusStyle.Width(msg.Width)
		TextareaStyle = TextareaStyle.Width(msg.Width)
		progressContainerContentWidth := msg.Width - TextareaStyle.GetHorizontalBorderSize() - TextareaStyle.GetHorizontalPadding()
		if progressContainerContentWidth < 0 {
			progressContainerContentWidth = 0
		}
		m.Progress.Width = progressContainerContentWidth

	case ConnectionMsg:
		m.Conn = msg.Conn
		m.Status = "CONNECTING: Performing key exchange..."
		m.IsConnected = true
		go network.ListenForMessages(m.Conn, nil, &programMessageSender{program: m.Program}, m.Command == "CREATE")

	case SharedKeyMsg:
		m.SharedKey = msg.Key
		m.Status = fmt.Sprintf("CONNECTED to %s: Exchanging nicknames...", m.Conn.RemoteAddr().String())
		cmd := func() tea.Msg {
			if err := network.SendData(m.Conn, m.SharedKey, protocol.TypeNickname, []byte(m.Nickname)); err != nil {
				return ErrorMsg{Err: err}
			}
			return nil
		}
		cmds = append(cmds, cmd)

	case MyPublicKeyMsg:
		hash := sha256.Sum256(msg.PublicKey)
		m.MyFingerprint = fmt.Sprintf("%x", hash[:8])
	case PeerPublicKeyMsg:
		hash := sha256.Sum256(msg.PublicKey)
		m.PeerFingerprint = fmt.Sprintf("%x", hash[:8])
		now := time.Now()
		if m.MyFingerprint == "" {
			m.Messages = append(m.Messages, Message{Timestamp: now, Sender: "System", Content: "Attempting to display fingerprints; your own fingerprint is not yet available."})
		} else {
			m.Messages = append(m.Messages, Message{Timestamp: now, Sender: "System", Content: fmt.Sprintf("Your Key Fingerprint: %s", m.MyFingerprint)})
		}
		m.Messages = append(m.Messages, Message{Timestamp: now, Sender: "System", Content: fmt.Sprintf("Peer's Key Fingerprint: %s", m.PeerFingerprint)})
		m.Messages = append(m.Messages, Message{Timestamp: now, Sender: "System", Content: "Please verify these fingerprints with your peer through a trusted channel."})

	case ReceivedNicknameMsg:
		m.PeerNickname = msg.Nickname
		m.Status = fmt.Sprintf("CONNECTED to %s: Chatting with %s", m.Conn.RemoteAddr().String(), m.PeerNickname)
		m.IsReady = true
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "System", Content: fmt.Sprintf("Welcome to secure chat! You are %s, connected to %s. Type /help for a list of commands or /send <file_path> to send a file.", m.Nickname, m.PeerNickname)})
		cmds = append(cmds, func() tea.Msg { return FocusTextareaMsg{} })

	case ReceivedTextMsg:
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: m.PeerNickname, Content: msg.Text})

	case FileOfferMsg:
		m.PendingOffer = msg.Metadata
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "System", Content: fmt.Sprintf("Peer wants to send you a file: %s (%.2f MB). Accept? (y/n)", msg.Metadata.FileName, float64(msg.Metadata.FileSize)/1024/1024)})
		m.Status = fmt.Sprintf("TRANSFERRING: Receiving file offer for %s", msg.Metadata.FileName)

	case FileOfferAcceptedMsg:
		m.IsAwaitingAcceptance = false
		m.IsTransferring = true
		m.Progress.SetPercent(0)
		m.Status = fmt.Sprintf("TRANSFERRING: Sending %s", filepath.Base(msg.Metadata.OriginalPath))
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "System", Content: fmt.Sprintf("Peer accepted file: %s. Starting transfer...", msg.Metadata.FileName)})
		cmds = append(cmds, func() tea.Msg {
			filetransfer.SendFileChunks(m.Conn, m.SharedKey, msg.Metadata.OriginalPath, &programMessageSender{program: m.Program})
			return nil
		})

	case FileOfferRejectedMsg:
		m.IsAwaitingAcceptance = false
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "System", Content: "Peer rejected the file transfer."})
		if m.IsConnected {
			m.Status = fmt.Sprintf("CONNECTED to %s: Chatting with %s", m.Conn.RemoteAddr().String(), m.PeerNickname)
		} else {
			m.Status = "Idle"
		}

	case FileOfferFailedMsg:
		m.IsAwaitingAcceptance = false
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "Error", Content: "File offer failed: " + msg.Reason})
		if m.IsConnected {
			m.Status = fmt.Sprintf("CONNECTED to %s: Chatting with %s", m.Conn.RemoteAddr().String(), m.PeerNickname)
		} else {
			m.Status = "Idle"
		}

	case FileSendingCompleteMsg:
		m.IsTransferring = false
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "System", Content: "File transfer complete."})
		if m.IsConnected {
			m.Status = fmt.Sprintf("CONNECTED to %s: Chatting with %s", m.Conn.RemoteAddr().String(), m.PeerNickname)
		} else {
			m.Status = "Idle"
		}

	case FileChunkMsg:
		if m.IsReceiving && m.ReceivingFile != nil {
			bytesWritten, err := m.ReceivingFile.Write(msg.Chunk)
			if err != nil {
				m.Err = err
				return m, tea.Quit
			}
			m.TotalBytesReceived += int64(bytesWritten)
			progressVal := float64(m.TotalBytesReceived) / float64(m.PendingOffer.FileSize)
			cmds = append(cmds, m.Progress.SetPercent(progressVal))
		}

	case FileDoneMsg:
		if m.IsTransferring {
			if m.IsReceiving {
				m.ReceivingFile.Close()
				m.ReceivingFile = nil
				m.PendingOffer = protocol.FileMetadata{}
			}
			m.IsTransferring = false
			m.IsReceiving = false
			m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "System", Content: "File transfer complete."})
			if m.IsConnected {
				m.Status = fmt.Sprintf("CONNECTED to %s: Chatting with %s", m.Conn.RemoteAddr().String(), m.PeerNickname)
			} else {
				m.Status = "Idle"
			}
		}

	case FileTransferProgress:
		percent := float64(msg)
		cmds = append(cmds, m.Progress.SetPercent(percent))
		if percent >= 1.0 && !m.IsReceiving {
			cmds = append(cmds, func() tea.Msg { return FileSendingCompleteMsg{} })
		}

	case InfoMsg:
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "System", Content: msg.Info})

	case ConnectionClosedMsg:
		m.IsConnected = false
		m.Status = "DISCONNECTED: Connection closed by server (session may have timed out)."
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "Error", Content: m.Status})

	case ErrorMsg:
		m.Err = msg.Err
		return m, tea.Quit
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	if m.Err != nil {
		return fmt.Sprintf("An error occurred: %v\n\nPress Ctrl+C to quit.", m.Err)
	}

	if m.ShowHelp {
		return m.helpView()
	}

	chatAreaViewString := m.chatArea.View(m.Messages)
	footerString := m.footerView()

	if footerString != "" {
		return fmt.Sprintf(
			"%s\n%s\n%s",
			m.headerView(),
			chatAreaViewString,
			footerString,
		)
	}
	return fmt.Sprintf(
		"%s\n%s",
		m.headerView(),
		chatAreaViewString,
	)
}

func (m *Model) helpView() string {
	return lipgloss.NewStyle().Padding(1, 2).Border(lipgloss.RoundedBorder()).Render(
		"Available Commands:\n" +
			"  /send <file_path> - Send a file\n" +
			"  /help             - Toggle this help message\n" +
			"  /quit             - Disconnect and exit (Ctrl+C/Esc also works)\n" +
			"  /fingerprint      - Show your and peer's key fingerprints\n" +
			"\nKeybindings:\n" +
			"  Ctrl+C/Esc        - Disconnect and exit\n" +
			"  Enter             - Send message\n" +
			"\nFile Transfer:\n" +
			"  'y' or 'Y'        - Accept incoming file offer\n" +
			"  'n' or 'N'        - Reject incoming file offer\n" +
			"\n(Press Esc to close this help menu)",
	)
}

func (m *Model) headerView() string {
	if m.SessionID != "" {
		return StatusStyle.Render(fmt.Sprintf("%s | Session ID: %s", m.Status, m.SessionID))
	}
	return StatusStyle.Render(m.Status)
}

func (m *Model) footerView() string {
	if m.IsTransferring {
		return TextareaStyle.Render(m.Progress.View())
	}
	if m.PendingOffer.FileName != "" {
		return TextareaStyle.Render("Accept file? (y/n)")
	}
	return ""
}
