package ui

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dothash/hemmelig-cli/internal/filetransfer"
	"github.com/dothash/hemmelig-cli/internal/network"
	"github.com/dothash/hemmelig-cli/internal/protocol"
)

// Implement the MessageSender interface
func (m *Model) SendError(err error) {
	m.Program.Send(ErrorMsg{Err: err})
}

func (m *Model) SendInfo(info string) {
	m.Program.Send(InfoMsg{Info: info})
}

func (m *Model) SendConnection(conn net.Conn) {
	m.Program.Send(ConnectionMsg{Conn: conn})
}

func (m *Model) SendSharedKey(key []byte) {
	m.Program.Send(SharedKeyMsg{Key: key})
}

func (m *Model) SendReceivedNickname(nickname string) {
	m.Program.Send(ReceivedNicknameMsg{Nickname: nickname})
}

func (m *Model) SendReceivedText(text string) {
	m.Program.Send(ReceivedTextMsg{Text: text})
}

func (m *Model) SendFileOffer(metadata protocol.FileMetadata) {
	m.Program.Send(FileOfferMsg{Metadata: metadata})
}

func (m *Model) SendFileOfferAccepted(metadata protocol.FileMetadata) {
	m.Program.Send(FileOfferAcceptedMsg{Metadata: metadata})
}

func (m *Model) SendFileOfferRejected() {
	m.Program.Send(FileOfferRejectedMsg{})
}

func (m *Model) SendFileOfferFailed(reason string) {
	m.Program.Send(FileOfferFailedMsg{Reason: reason})
}

func (m *Model) SendFileSendingComplete() {
	m.Program.Send(FileSendingCompleteMsg{})
}

func (m *Model) SendFileChunk(chunk []byte) {
	m.Program.Send(FileChunkMsg{Chunk: chunk})
}

func (m *Model) SendFileDone() {
	m.Program.Send(FileDoneMsg{})
}

func (m *Model) SendProgress(percent float64) {
	m.Program.Send(m.Progress.SetPercent(percent)())}

func (m *Model) SendPeerPublicKey(publicKey []byte) {
	m.Program.Send(PeerPublicKeyMsg{PublicKey: publicKey})}

func (m *Model) SendMyPublicKey(publicKey []byte) {
	m.Program.Send(MyPublicKeyMsg{PublicKey: publicKey})}


type InfoMsg struct {
	Info string
}

// Model represents the Bubble Tea UI model.
type Model struct {
	RelayServerAddr string
	SessionID       string
	Command         string // Added to store the command (CREATE/JOIN)
	Status          string
	Conn            net.Conn
	SharedKey       []byte
	Err             error
	Program         *tea.Program

	Nickname     string
	PeerNickname string

	Viewport viewport.Model
	Textarea textarea.Model
	Progress progress.Model
	Messages []string
	IsReady  bool
	IsConnected bool

	// File transfer state
	IsTransferring     bool
	IsReceiving        bool
	IsAwaitingAcceptance bool
	PendingOffer       protocol.FileMetadata
	ReceivingFile      *os.File
	TotalBytesReceived int64
	ShowHelp           bool
	PeerFingerprint    string
	MyFingerprint      string
	MaxFileSize        int64
}

// NewModel creates a new UI model.
func NewModel(relayServerAddr, sessionID, nickname, command string, maxFileSize int64) *Model {
	ta := textarea.New()
	ta.Placeholder = "Type a message or /send <file_path>..."
	ta.Focus()
	ta.CharLimit = 0
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	vp := viewport.New(80, 10)
	vp.SetContent("Waiting for connection...")

	prog := progress.New(progress.WithDefaultGradient())

	return &Model{
		RelayServerAddr: relayServerAddr,
		SessionID:       sessionID,
		Nickname:        nickname,
		Status:          fmt.Sprintf("Connecting to relay server %s...", relayServerAddr),
		Textarea:        ta,
		Viewport:        vp,
		Progress:        prog,
		Messages:        []string{},
		Command:         command,
		MaxFileSize:     maxFileSize * 1024 * 1024, // Convert MB to bytes
	}
}

// Init initializes the model.
func (m *Model) Init() tea.Cmd {
	return func() tea.Msg {
		conn, err := net.Dial("tcp", m.RelayServerAddr)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to connect to relay server: %w", err)}
		}

		initialMsg := struct {
			Command   string `json:"command"`
			SessionID string `json:"sessionID,omitempty"`
		}{
			Command:   m.Command,
			SessionID: m.SessionID,
		}

		msgBytes, err := json.Marshal(initialMsg)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to marshal initial message: %w", err)}
		}

		_, err = conn.Write(append(msgBytes, '\n'))
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to send initial message to relay server: %w", err)}
		}

		// Read response from relay server
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
			m.Messages = append(m.Messages, SystemStyle.Render(fmt.Sprintf("New session created with ID: %s. Share this ID with your peer.\n", m.SessionID)))
		}

		m.Program.Send(ConnectionMsg{Conn: conn})
		network.ListenForMessages(conn, nil, m, m.Command == "CREATE") // Pass isInitiator flag
		return nil
	}
}

// Update handles messages and updates the model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		taCmd tea.Cmd
		vpCmd tea.Cmd
		pgCmd tea.Cmd
	)

	m.Textarea, taCmd = m.Textarea.Update(msg)
	m.Viewport, vpCmd = m.Viewport.Update(msg)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			if m.Conn != nil {
				m.Conn.Close()
			}
			return m, tea.Quit
		case tea.KeyRunes:
			if len(msg.Runes) > 0 && msg.Runes[0] == '?' {
				m.ShowHelp = !m.ShowHelp
				return m, nil
			}
		case tea.KeyEnter:
			// If we are currently in the process of confirming a file transfer
			if m.PendingOffer.FileName != "" {
				text := strings.ToLower(strings.TrimSpace(m.Textarea.Value()))
				m.Textarea.Reset()

				if text == "y" || text == "yes" {
					m.Messages = append(m.Messages, SystemStyle.Render("Accepting file transfer..."))
					metaBytes, _ := m.PendingOffer.ToJSON()
					cmd := func() tea.Msg {
						if err := network.SendData(m.Conn, m.SharedKey, protocol.TypeFileAccept, metaBytes); err != nil {
							return ErrorMsg{Err: err}
						}
						return nil
					}
					// Prepare to receive the file
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
					return m, cmd
				} else {
					m.Messages = append(m.Messages, SystemStyle.Render("Rejected file transfer."))
					cmd := func() tea.Msg {
						if err := network.SendData(m.Conn, m.SharedKey, protocol.TypeFileReject, nil); err != nil {
							return ErrorMsg{Err: err}
						}
						return nil
					}
					m.PendingOffer = protocol.FileMetadata{}
					return m, cmd
				}
			}

			// Normal message or command
			if m.IsReady && !m.IsTransferring && !m.IsAwaitingAcceptance {
				text := strings.TrimSpace(m.Textarea.Value())
				if text == "" {
					return m, nil
				}
				m.Textarea.Reset()

				if strings.HasPrefix(text, "/send ") {
					filePath := strings.TrimPrefix(text, "/send ")
					m.Messages = append(m.Messages, SystemStyle.Render(fmt.Sprintf("Offering to send file: %s", filePath)))
					m.IsAwaitingAcceptance = true
					m.Status = fmt.Sprintf("TRANSFERRING: Offering to send %s", filepath.Base(filePath))
										cmd := func() tea.Msg {
						filetransfer.RequestSendFile(m.Conn, m.SharedKey, filePath, m, m.MaxFileSize)
						return nil
					}
					return m, cmd
				}

				m.Messages = append(m.Messages, fmt.Sprintf("%s %s%s", TimestampStyle.Render(time.Now().Format("15:04")), SenderStyle.Render(m.Nickname+": "), text))
				cmd := func() tea.Msg {
					if err := network.SendData(m.Conn, m.SharedKey, protocol.TypeText, []byte(text)); err != nil {
						return ErrorMsg{Err: err}
					}
					return nil
				}
				return m, cmd
			}
		}

	case tea.WindowSizeMsg:
		headerHeight := lipgloss.Height(m.headerView())
		infoPaneHeight := lipgloss.Height(m.infoPaneView())
		footerHeight := lipgloss.Height(m.footerView())
		verticalMargin := headerHeight + infoPaneHeight + footerHeight
		m.Viewport.Width = msg.Width
		m.Viewport.Height = msg.Height - verticalMargin
		ViewportStyle.Width(msg.Width - 2)
		m.Textarea.SetWidth(msg.Width)
		TextareaStyle.Width(msg.Width - 2)
		m.Progress.Width = msg.Width - 4
		if m.IsReady {
			m.Viewport.SetContent(strings.Join(m.Messages, "\n"))
		}

	case progress.FrameMsg:
		progressModel, cmd := m.Progress.Update(msg)
		m.Progress = progressModel.(progress.Model)
		pgCmd = cmd

	case ConnectionMsg:
		m.Conn = msg.Conn
		m.Status = "CONNECTING: Performing key exchange..."
		m.IsConnected = true

	case SharedKeyMsg:
		m.SharedKey = msg.Key
		m.Status = fmt.Sprintf("CONNECTED to %s: Exchanging nicknames...", m.Conn.RemoteAddr().String())
		// Send our nickname to the peer
		cmd := func() tea.Msg {
			if err := network.SendData(m.Conn, m.SharedKey, protocol.TypeNickname, []byte(m.Nickname)); err != nil {
				return ErrorMsg{Err: err}
			}
			return nil
		}
		return m, cmd

	case MyPublicKeyMsg:
		hash := sha256.Sum256(msg.PublicKey)
		m.MyFingerprint = fmt.Sprintf("%x", hash[:8])
	case PeerPublicKeyMsg:
		hash := sha256.Sum256(msg.PublicKey)
		m.PeerFingerprint = fmt.Sprintf("%x", hash[:8]) // Use first 8 bytes for a shorter fingerprint
		m.Messages = append(m.Messages, SystemStyle.Render(fmt.Sprintf("Peer's Key Fingerprint: %s", m.PeerFingerprint)))
		m.Messages = append(m.Messages, SystemStyle.Render("Please verify this fingerprint with your peer through a trusted channel."))

	case ReceivedNicknameMsg:
		m.PeerNickname = msg.Nickname
		m.Status = fmt.Sprintf("CONNECTED to %s: Chatting with %s", m.Conn.RemoteAddr().String(), m.PeerNickname)
		m.IsReady = true
		m.Viewport.SetContent(SystemStyle.Render(fmt.Sprintf("Welcome to secure chat! You are %s, connected to %s. Type /send <file_path> to send a file.", m.Nickname, m.PeerNickname)))
		m.Viewport.GotoBottom()

	case ReceivedTextMsg:
		m.Messages = append(m.Messages, fmt.Sprintf("%s %s%s", TimestampStyle.Render(time.Now().Format("15:04")), ReceiverStyle.Render(m.PeerNickname+": "), msg.Text))

	case FileOfferMsg:
		m.PendingOffer = msg.Metadata
		m.Messages = append(m.Messages, SystemStyle.Render(fmt.Sprintf("Peer wants to send you a file: %s (%.2f MB). Accept? (y/n)", msg.Metadata.FileName, float64(msg.Metadata.FileSize)/1024/1024)))
		m.Status = fmt.Sprintf("TRANSFERRING: Receiving file offer for %s", msg.Metadata.FileName)

	case FileOfferAcceptedMsg:
		m.IsAwaitingAcceptance = false
		m.IsTransferring = true
		m.Progress.SetPercent(0)
		m.Status = fmt.Sprintf("TRANSFERRING: Sending %s", filepath.Base(msg.Metadata.OriginalPath))
		m.Messages = append(m.Messages, SystemStyle.Render(fmt.Sprintf("Peer accepted file: %s. Starting transfer...", msg.Metadata.FileName)))
		return m, func() tea.Msg {
			filetransfer.SendFileChunks(m.Conn, m.SharedKey, msg.Metadata.OriginalPath, m)
			return nil
		}

	case FileOfferRejectedMsg:
		m.IsAwaitingAcceptance = false
		m.Messages = append(m.Messages, SystemStyle.Render("Peer rejected the file transfer."))
		if m.IsConnected {
			m.Status = fmt.Sprintf("CONNECTED to %s: Chatting with %s", m.Conn.RemoteAddr().String(), m.PeerNickname)
		} else {
			m.Status = "Idle"
		}

	case FileOfferFailedMsg:
		m.IsAwaitingAcceptance = false
		m.Messages = append(m.Messages, ErrorStyle.Render("File offer failed: "+msg.Reason))
		if m.IsConnected {
			m.Status = fmt.Sprintf("CONNECTED to %s: Chatting with %s", m.Conn.RemoteAddr().String(), m.PeerNickname)
		} else {
			m.Status = "Idle"
		}

	case FileSendingCompleteMsg:
		m.IsTransferring = false
		m.Messages = append(m.Messages, SystemStyle.Render("File transfer complete."))
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
			progress := float64(m.TotalBytesReceived) / float64(m.PendingOffer.FileSize)
			pgCmd = m.Progress.SetPercent(progress)
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
			m.Messages = append(m.Messages, SystemStyle.Render("File transfer complete."))
			if m.IsConnected {
				m.Status = fmt.Sprintf("CONNECTED to %s: Chatting with %s", m.Conn.RemoteAddr().String(), m.PeerNickname)
			} else {
				m.Status = "Idle"
			}
		}

	case ProgressMsg:
		progressModel, cmd := m.Progress.Update(msg)
		m.Progress = progressModel.(progress.Model)
		pgCmd = cmd

	case InfoMsg:
		m.Messages = append(m.Messages, SystemStyle.Render(msg.Info))
		return m, nil

	case ErrorMsg:
		m.Err = msg.Err
		return m, tea.Quit
	}

	m.Viewport.SetContent(strings.Join(m.Messages, "\n"))
	m.Viewport.GotoBottom()

	return m, tea.Batch(taCmd, vpCmd, pgCmd)
}

// View renders the UI.
func (m *Model) View() string {
	if m.Err != nil {
		return fmt.Sprintf("An error occurred: %v\n\nPress Ctrl+C to quit.", m.Err)
	}

	if m.ShowHelp {
		return m.helpView()
	}

	return fmt.Sprintf(
		"%s\n%s\n%s\n%s",
		m.headerView(),
		m.infoPaneView(),
		ViewportStyle.Render(m.Viewport.View()),
		m.footerView(),
	)
}

func (m *Model) helpView() string {
	return lipgloss.NewStyle().Padding(1, 2).Border(lipgloss.RoundedBorder()).Render(
		"Available Commands:\n" +
			"  /send <file_path> - Send a file\n" +
			"  /quit             - Disconnect and exit\n" +
			"\nKeybindings:\n" +
			"  ?                 - Toggle help\n" +
			"  Ctrl+C/Esc        - Disconnect and exit\n" +
			"  Enter             - Send message or confirm file transfer\n",
	)
}

func (m *Model) infoPaneView() string {
	myKey := InfoBoxStyle.Render(fmt.Sprintf("Your Fingerprint: %s", m.MyFingerprint))
	peerKey := InfoBoxStyle.Render(fmt.Sprintf("Peer Fingerprint: %s", m.PeerFingerprint))

	return lipgloss.JoinHorizontal(lipgloss.Top, myKey, peerKey)
}

func (m *Model) headerView() string {
	return StatusStyle.Render(m.Status)
}

func (m *Model) footerView() string {
	if m.IsTransferring {
		return TextareaStyle.Render(m.Progress.View())
	}
	if m.PendingOffer.FileName != "" {
		return TextareaStyle.Render("Accept file? (y/n)")
	}
	return TextareaStyle.Render(m.Textarea.View())
}
