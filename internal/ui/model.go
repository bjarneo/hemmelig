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

func (pms *programMessageSender) SendUserJoined(userID, nickname string, publicKey []byte) {
	pms.program.Send(UserJoinedMsg{UserID: userID, Nickname: nickname, PublicKey: publicKey})
}

func (pms *programMessageSender) SendUserLeft(userID string) {
	pms.program.Send(UserLeftMsg{UserID: userID})
}

func (pms *programMessageSender) SendPublicKey(userID, nickname string, publicKey []byte) {
	pms.program.Send(PublicKeyMsg{UserID: userID, Nickname: nickname, PublicKey: publicKey})
}

func (pms *programMessageSender) SendReceivedText(senderID string, ciphertext []byte) {
	pms.program.Send(ReceivedTextMsg{SenderID: senderID, Ciphertext: ciphertext})
}

func (pms *programMessageSender) SendFileOffer(metadata protocol.FileMetadata, senderID string) {
	pms.program.Send(FileOfferMsg{Metadata: metadata, SenderID: senderID})
}

func (pms *programMessageSender) SendFileOfferAccepted(metadata protocol.FileMetadata) {
	pms.program.Send(FileOfferAcceptedMsg{Metadata: metadata})
}

func (pms *programMessageSender) SendFileOfferRejected(senderID string) {
	pms.program.Send(FileOfferRejectedMsg{SenderID: senderID})
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

func (pms *programMessageSender) SendConnectionClosed() {
	pms.program.Send(ConnectionClosedMsg{})
}

type InfoMsg struct {
	Info string
}

import "github.com/bjarneo/jot/internal/crypto"

// Model represents the Bubble Tea UI model.
type Model struct {
	RelayServerAddr string
	SessionID       string
	Command         string
	Status          string
	Conn            net.Conn
	Err             error
	Program         *tea.Program

	Nickname      string
	privateKey    []byte
	publicKey     []byte
	sharedSecrets map[string][]byte // map[userID]sharedKey

	chatArea    ChatAreaModel
	Progress    progress.Model
	Messages    []Message
	IsReady     bool
	IsConnected bool

	IsTransferring       bool
	IsReceiving          bool
	IsAwaitingAcceptance bool
	PendingOffer         protocol.FileMetadata
	FileOfferSenderID    string
	ReceivingFile        *os.File
	TotalBytesReceived   int64
	ShowHelp             bool
	MyFingerprint        string
	PeerFingerprints     map[string]string // map[userID]fingerprint
	MaxFileSize          int64
	Participants         map[string]string // map[userID]nickname
}

func NewModel(relayServerAddr, sessionID, nickname, command string, maxFileSize int64, privateKey, publicKey []byte) *Model {
	initialWidth := 80
	initialChatAreaHeight := 20

	ca := NewChatAreaModel(initialWidth, initialChatAreaHeight, nickname)
	prog := progress.New(progress.WithDefaultGradient())

	m := &Model{
		RelayServerAddr:  relayServerAddr,
		SessionID:        sessionID,
		Nickname:         nickname,
		Status:           fmt.Sprintf("Connecting to relay server %s...", relayServerAddr),
		chatArea:         ca,
		Progress:         prog,
		Messages:         []Message{{Timestamp: time.Now(), Sender: "System", Content: "Waiting for connection..."}},
		Command:          command,
		MaxFileSize:      maxFileSize * 1024 * 1024,
		privateKey:       privateKey,
		publicKey:        publicKey,
		sharedSecrets:    make(map[string][]byte),
		PeerFingerprints: make(map[string]string),
		Participants:     make(map[string]string),
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
			Nickname  string `json:"nickname"`
			PublicKey []byte `json:"publicKey"`
		}{
			Command:   m.Command,
			SessionID: m.SessionID,
			Nickname:  m.Nickname,
			PublicKey: m.publicKey,
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
		for {
			response, err := reader.ReadString('\n')
			if err != nil {
				return ErrorMsg{Err: fmt.Errorf("failed to read response from relay server: %w", err)}
			}

			if strings.HasPrefix(response, "Joined session:") {
				break
			}

			if strings.HasPrefix(response, "Error:") {
				return ErrorMsg{Err: fmt.Errorf("relay server error: %s", strings.TrimSpace(response))}
			}

			var respMsg map[string]interface{}
			if err := json.Unmarshal([]byte(response), &respMsg); err != nil {
				// Ignore messages that are not valid JSON
				continue
			}

			if respMsg["type"] == "error" {
				return ErrorMsg{Err: fmt.Errorf("relay server error: %s", respMsg["message"])}
			}

			if respMsg["type"] == "session_created" {
				m.SessionID = respMsg["sessionID"].(string)
				break
			}
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
				for userID, sharedKey := range m.sharedSecrets {
					filetransfer.RequestSendFile(m.Conn, sharedKey, filePath, &programMessageSender{program: m.Program}, m.MaxFileSize, userID)
				}
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
			for userID, fingerprint := range m.PeerFingerprints {
				nickname := m.Participants[userID]
				m.Messages = append(m.Messages, Message{Timestamp: now, Sender: "System", Content: fmt.Sprintf("%s's Key Fingerprint: %s", nickname, fingerprint)})
			}
		} else {
			m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: m.Nickname, Content: text})
			cmd := func() tea.Msg {
				for userID, sharedKey := range m.sharedSecrets {
					encryptedText, err := crypto.Encrypt([]byte(text), sharedKey)
					if err != nil {
						return ErrorMsg{Err: err}
					}
					msg := map[string]interface{}{
						"type":       "message",
						"recipient":  userID,
						"ciphertext": encryptedText,
					}
					if err := network.SendData(m.Conn, msg); err != nil {
						return ErrorMsg{Err: err}
					}
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
						sharedKey := m.sharedSecrets[m.FileOfferSenderID]
						metaBytes, _ := m.PendingOffer.ToJSON()
						encryptedMeta, _ := crypto.Encrypt(metaBytes, sharedKey)
						msg := map[string]interface{}{
							"type":      "file_accept",
							"recipient": m.FileOfferSenderID,
							"metadata":  encryptedMeta,
						}
						network.SendData(m.Conn, msg)

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
						msg := map[string]interface{}{
							"type":      "file_reject",
							"recipient": m.FileOfferSenderID,
						}
						network.SendData(m.Conn, msg)
						m.PendingOffer = protocol.FileMetadata{}
					}
				}
			}
		}

	case tea.WindowSizeMsg:
		headerHeight := lipgloss.Height(m.headerView())
		verticalMargin := headerHeight
		chatAreaHeight := msg.Height - verticalMargin
		if chatAreaHeight < 0 {
			chatAreaHeight = 0
		}
		m.chatArea.SetDimensions(msg.Width, chatAreaHeight)
		StatusStyle = StatusStyle.Width(msg.Width)

	case ConnectionMsg:
		m.Conn = msg.Conn
		m.Status = "CONNECTING: Waiting for other users..."
		m.IsConnected = true
		go network.ListenForMessages(m.Conn, &programMessageSender{program: m.Program})

	case UserJoinedMsg:
		m.Participants[msg.UserID] = msg.Nickname
		m.Status = "CONNECTED"
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "System", Content: fmt.Sprintf("%s has joined the chat.", msg.Nickname)})
		var privateKey, publicKey [32]byte
		copy(privateKey[:], m.privateKey)
		copy(publicKey[:], msg.PublicKey)
		sharedSecret, err := crypto.ComputeSharedSecret(privateKey, publicKey)
		if err != nil {
			m.Err = err
			return m, tea.Quit
		}
		m.sharedSecrets[msg.UserID] = sharedSecret
		hash := sha256.Sum256(msg.PublicKey)
		m.PeerFingerprints[msg.UserID] = fmt.Sprintf("%x", hash[:8])

	case UserLeftMsg:
		nickname := m.Participants[msg.UserID]
		delete(m.Participants, msg.UserID)
		delete(m.sharedSecrets, msg.UserID)
		delete(m.PeerFingerprints, msg.UserID)
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "System", Content: fmt.Sprintf("%s has left the chat.", nickname)})

	case PublicKeyMsg:
		m.Participants[msg.UserID] = msg.Nickname
		m.Status = "CONNECTED"
		var privateKey, publicKey [32]byte
		copy(privateKey[:], m.privateKey)
		copy(publicKey[:], msg.PublicKey)
		sharedSecret, err := crypto.ComputeSharedSecret(privateKey, publicKey)
		if err != nil {
			m.Err = err
			return m, tea.Quit
		}
		m.sharedSecrets[msg.UserID] = sharedSecret
		hash := sha256.Sum256(msg.PublicKey)
		m.PeerFingerprints[msg.UserID] = fmt.Sprintf("%x", hash[:8])

	case ReceivedTextMsg:
		sharedKey, ok := m.sharedSecrets[msg.SenderID]
		if !ok {
			// This should not happen if the server is working correctly
			return m, nil
		}
		plaintext, err := crypto.Decrypt(msg.Ciphertext, sharedKey)
		if err != nil {
			m.Err = err
			return m, tea.Quit
		}
		senderNickname := m.Participants[msg.SenderID]
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: senderNickname, Content: string(plaintext)})

	case FileOfferMsg:
		m.PendingOffer = msg.Metadata
		m.FileOfferSenderID = msg.SenderID
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "System", Content: fmt.Sprintf("Peer wants to send you a file: %s (%.2f MB). Accept? (y/n)", msg.Metadata.FileName, float64(msg.Metadata.FileSize)/1024/1024)})
		m.Status = fmt.Sprintf("TRANSFERRING: Receiving file offer for %s", msg.Metadata.FileName)

	case FileOfferAcceptedMsg:
		m.IsAwaitingAcceptance = false
		m.IsTransferring = true
		m.Progress.SetPercent(0)
		m.Status = fmt.Sprintf("TRANSFERRING: Sending %s", filepath.Base(msg.Metadata.OriginalPath))
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "System", Content: fmt.Sprintf("Peer accepted file: %s. Starting transfer...", msg.Metadata.FileName)})
		cmds = append(cmds, func() tea.Msg {
			sharedKey := m.sharedSecrets[msg.SenderID]
			filetransfer.SendFileChunks(m.Conn, sharedKey, msg.Metadata.OriginalPath, &programMessageSender{program: m.Program}, msg.SenderID)
			return nil
		})

	case FileOfferRejectedMsg:
		m.IsAwaitingAcceptance = false
		nickname := m.Participants[msg.SenderID]
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "System", Content: fmt.Sprintf("%s rejected the file transfer.", nickname)})
		if m.IsConnected {
			m.Status = "CONNECTED"
		} else {
			m.Status = "Idle"
		}

	case FileOfferFailedMsg:
		m.IsAwaitingAcceptance = false
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "Error", Content: "File offer failed: " + msg.Reason})
		if m.IsConnected {
			m.Status = "CONNECTED"
		} else {
			m.Status = "Idle"
		}

	case FileSendingCompleteMsg:
		m.IsTransferring = false
		m.Messages = append(m.Messages, Message{Timestamp: time.Now(), Sender: "System", Content: "File transfer complete."})
		if m.IsConnected {
			m.Status = "CONNECTED"
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
				m.Status = "CONNECTED"
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

	m.chatArea.Participants = m.Participants
	chatAreaViewString := m.chatArea.View(m.Messages, m.Participants)
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
			"  /fingerprint      - Show your and other users' key fingerprints\n" +
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
		return m.Progress.View()
	}
	if m.PendingOffer.FileName != "" {
		return "Accept file? (y/n)"
	}
	return ""
}
