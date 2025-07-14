package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

var totalSessions int64

// generateShortID generates a short random hex string.
func generateShortID(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a timestamp-based string if crypto/rand fails, though unlikely.
		return fmt.Sprintf("%x", time.Now().UnixNano())[:length]
	}
	return hex.EncodeToString(bytes)
}

// Client represents a connected client.
type Client struct {
	conn      net.Conn
	id        string
	nickname  string
	publicKey []byte
}

// Session represents a chat session with multiple connected clients.
type Session struct {
	ID      string
	OwnerID string // The ID of the client who created the session
	Clients map[string]*Client
	mu      sync.Mutex
	Banned  map[string]bool // Map of banned client IDs
}

// RelayServer holds the state of the relay server.
type RelayServer struct {
	sessions       map[string]*Session
	mu             sync.Mutex
	maxDataRelayed int64
}

// NewRelayServer creates a new RelayServer instance.
func NewRelayServer(maxDataRelayed int64) *RelayServer {
	return &RelayServer{
		sessions:       make(map[string]*Session),
		maxDataRelayed: maxDataRelayed,
	}
}

// Start listens for incoming connections and handles them.
func (s *RelayServer) Start(listener net.Listener) {
	log.Printf("Relay server listening on %s", listener.Addr())

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}
		go s.handleConnection(conn)
	}
}

// ClientMessage represents the initial message from a client.
type ClientMessage struct {
	Command   string `json:"command"` // "CREATE" or "JOIN"
	SessionID string `json:"sessionID,omitempty"`
	Nickname  string `json:"nickname,omitempty"`
	PublicKey []byte `json:"publicKey,omitempty"`
}

// handleConnection handles a new client connection.
func (s *RelayServer) handleConnection(conn net.Conn) {
	log.Println("New anonymous connection received.")

	if err := conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		log.Println("Could not set read deadline for new connection.")
		conn.Close()
		return
	}

	reader := bufio.NewReader(conn)
	messageBytes, err := reader.ReadBytes('\n')
	if err != nil {
		log.Println("Error reading initial message from new connection.")
		conn.Close()
		return
	}

	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		log.Println("Could not reset read deadline for connection.")
		conn.Close()
		return
	}

	var clientMsg ClientMessage
	if err := json.Unmarshal(messageBytes, &clientMsg); err != nil {
		log.Println("Error unmarshaling initial message from connection.")
		conn.Close()
		return
	}

	s.mu.Lock()

	clientID := uuid.New().String()
	client := &Client{
		conn:      conn,
		id:        clientID,
		nickname:  clientMsg.Nickname,
		publicKey: clientMsg.PublicKey,
	}

	switch clientMsg.Command {
	case "CREATE":
		sessionID := clientMsg.SessionID
		if sessionID == "" {
			sessionID = uuid.New().String()
		} else {
			if _, exists := s.sessions[sessionID]; exists {
				log.Printf("Session ID '%s' already exists. Generating a new one.", sessionID)
				conn.Write([]byte("Error: Session ID already exists\n"))
				s.mu.Unlock()
				conn.Close()
				return
			}
		}

		session := &Session{
			ID:      sessionID,
			OwnerID: clientID,
			Clients: make(map[string]*Client),
			Banned:  make(map[string]bool),
		}
		session.Clients[clientID] = client
		s.sessions[sessionID] = session
		atomic.AddInt64(&totalSessions, 1)
		log.Printf("New session created with ID '%s' by client '%s'. Total active sessions: %d", sessionID, clientID, len(s.sessions))
		resp := map[string]interface{}{
			"type":      "session_created",
			"sessionID": sessionID,
		}
		respBytes, _ := json.Marshal(resp)
		conn.Write(append(respBytes, '\n'))
		s.mu.Unlock()
		go s.relayData(client, session)

	case "JOIN":
		session, exists := s.sessions[clientMsg.SessionID]
		if !exists {
			log.Printf("Attempted to join session '%s' which does not exist.", clientMsg.SessionID)
			conn.Write([]byte("Error: Session not found\n"))
			s.mu.Unlock()
			conn.Close()
			return
		}

		session.mu.Lock()
		if len(session.Clients) >= 256 {
			log.Printf("Attempted to join session '%s' which is full.", clientMsg.SessionID)
			conn.Write([]byte("Error: Session is full\n"))
			session.mu.Unlock()
			s.mu.Unlock()
			conn.Close()
			return
		}
		if session.Banned[clientID] {
			log.Printf("Banned client '%s' attempted to join session '%s'", clientID, clientMsg.SessionID)
			conn.Write([]byte("Error: You are banned from this session\n"))
			session.mu.Unlock()
			s.mu.Unlock()
			conn.Close()
			return
		}

		// Notify existing clients of the new user's public key
		notification, _ := json.Marshal(map[string]interface{}{
			"type":      "user_joined",
			"userID":    client.id,
			"nickname":  client.nickname,
			"publicKey": client.publicKey,
		})
		notification = append(notification, '\n')
		for _, existingClient := range session.Clients {
			existingClient.conn.Write(notification)
		}

		// Send existing clients' public keys to the new client
		for _, existingClient := range session.Clients {
			keyInfo, _ := json.Marshal(map[string]interface{}{
				"type":      "public_key",
				"userID":    existingClient.id,
				"nickname":  existingClient.nickname,
				"publicKey": existingClient.publicKey,
			})
			keyInfo = append(keyInfo, '\n')
			client.conn.Write(keyInfo)
		}

		session.Clients[clientID] = client
		log.Printf("Client '%s' joined session '%s'. Total active sessions: %d", clientID, session.ID, len(s.sessions))
		conn.Write([]byte(fmt.Sprintf("Joined session: %s\n", session.ID)))
		session.mu.Unlock()
		s.mu.Unlock()
		go s.relayData(client, session)

	default:
		s.mu.Unlock()
		log.Println("Received unknown command from a client.")
		conn.Write([]byte("Error: Unknown command\n"))
		conn.Close()
		return
	}
}

// relayData relays data from a client to all other clients in the session.
func (s *RelayServer) relayData(client *Client, session *Session) {
	defer func() {
		s.removeClient(client, session)
		// Notify remaining clients
		notification, _ := json.Marshal(map[string]interface{}{
			"type":   "user_left",
			"userID": client.id,
		})
		notification = append(notification, '\n')
		session.mu.Lock()
		for _, otherClient := range session.Clients {
			otherClient.conn.Write(notification)
		}
		session.mu.Unlock()
	}()

	reader := bufio.NewReader(client.conn)
	for {
		if err := client.conn.SetReadDeadline(time.Now().Add(5 * time.Minute)); err != nil {
			log.Printf("Could not set read deadline for client '%s'.", client.id)
			return
		}

		messageBytes, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(messageBytes, &msg); err != nil {
			log.Printf("Error unmarshaling message from client '%s': %v", client.id, err)
			continue
		}

		msg["sender"] = client.id

		if msg["type"] == "message" || msg["type"] == "file_offer" || msg["type"] == "file_accept" || msg["type"] == "file_reject" || msg["type"] == "file_chunk" || msg["type"] == "file_done" {
			recipientID, ok := msg["recipient"].(string)
			if !ok {
				log.Printf("Message from client '%s' has no recipient", client.id)
				continue
			}

			session.mu.Lock()
			if recipient, ok := session.Clients[recipientID]; ok {
				outBytes, _ := json.Marshal(msg)
				outBytes = append(outBytes, '\n')
				if _, err := recipient.conn.Write(outBytes); err != nil {
					log.Printf("Error relaying message to client '%s': %v", recipient.id, err)
				}
			}
			session.mu.Unlock()
		} else {
			// Broadcast to all clients
			session.mu.Lock()
			for _, otherClient := range session.Clients {
				if otherClient.id != client.id {
					outBytes, _ := json.Marshal(msg)
					outBytes = append(outBytes, '\n')
					if _, err := otherClient.conn.Write(outBytes); err != nil {
						log.Printf("Error broadcasting message to client '%s': %v", otherClient.id, err)
					}
				}
			}
			session.mu.Unlock()
		}
	}
}

func (s *RelayServer) removeClient(client *Client, session *Session) {
	client.conn.Close()
	session.mu.Lock()
	delete(session.Clients, client.id)
	log.Printf("Client '%s' disconnected from session '%s'.", client.id, session.ID)

	if len(session.Clients) == 0 {
		session.mu.Unlock()
		s.mu.Lock()
		delete(s.sessions, session.ID)
		log.Printf("Session '%s' closed. Total active sessions: %d", session.ID, len(s.sessions))
		s.mu.Unlock()
	} else {
		session.mu.Unlock()
	}
}

func main() {
	maxDataRelayed := flag.Int64("max-data-relayed", 50, "Maximum data to relay per session in MB")
	addr := flag.String("addr", ":8080", "Address to listen on")
	flag.Parse()

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	server := NewRelayServer(*maxDataRelayed * 1024 * 1024) // Convert MB to bytes
	server.Start(listener)
}
