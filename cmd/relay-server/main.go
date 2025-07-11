package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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

// Session represents a chat session with two connected clients.
type Session struct {
	ID      string
	Clients [2]net.Conn
	mu      sync.Mutex
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
func (s *RelayServer) Start(addr string) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	log.Printf("Relay server listening on %s", addr)

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
}

// handleConnection handles a new client connection.
func (s *RelayServer) handleConnection(conn net.Conn) {
	log.Println("New anonymous connection received.")

	// Set a deadline for reading the initial message to prevent Slowloris attacks.
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

	// Reset the deadline to allow for long-lived connections.
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
	defer s.mu.Unlock()

	requestedSessionID := clientMsg.SessionID
	finalSessionID := requestedSessionID
	var session *Session
	var exists bool

	switch clientMsg.Command {
	case "CREATE":
		if requestedSessionID != "" {
			// User provided a session ID
			session, exists = s.sessions[requestedSessionID]
			if exists {
				// Collision: prepend a short unique ID
				log.Printf("Session ID '%s' already exists. Generating a new one.", requestedSessionID)
				prefix := generateShortID(6) // Generate a 6-character hex prefix (3 bytes)
				finalSessionID = prefix + "-" + requestedSessionID
				// Check again for the highly unlikely case of collision with the new ID
				_, exists = s.sessions[finalSessionID]
				for exists { // Keep generating until unique
					prefix = generateShortID(6)
					finalSessionID = prefix + "-" + requestedSessionID
					_, exists = s.sessions[finalSessionID]
				}
				log.Printf("Using modified session ID: '%s'", finalSessionID)
			} else {
				// User-provided ID is unique
				finalSessionID = requestedSessionID
			}
		} else {
			// User did not provide a session ID, generate a new UUID
			finalSessionID = uuid.New().String()
		}

		session = &Session{ID: finalSessionID}
		session.Clients[0] = conn
		s.sessions[finalSessionID] = session
		atomic.AddInt64(&totalSessions, 1)
		log.Printf("New session created with ID '%s'. Total active sessions: %d", finalSessionID, len(s.sessions))
		conn.Write([]byte(fmt.Sprintf("Session created: %s\n", finalSessionID)))

	case "JOIN":
		session, exists = s.sessions[requestedSessionID]
		if !exists || session.Clients[1] != nil {
			log.Printf("Attempted to join session '%s' which does not exist or is full.", requestedSessionID)
			conn.Write([]byte("Error: Session not found or full\n"))
			conn.Close()
			return
		}
		session.Clients[1] = conn
		finalSessionID = requestedSessionID // For logging and consistency
		log.Printf("Client joined session '%s'. Total active sessions: %d", finalSessionID, len(s.sessions))
		conn.Write([]byte(fmt.Sprintf("Joined session: %s\n", finalSessionID)))

		// Start relaying data between clients
		go s.relayData(session.Clients[0], session.Clients[1], finalSessionID)
		go s.relayData(session.Clients[1], session.Clients[0], finalSessionID)

	default:
		log.Println("Received unknown command from a client.")
		conn.Write([]byte("Error: Unknown command\n"))
		conn.Close()
		return
	}
}

// relayData relays data from src to dst, closing the session on error or inactivity.
func (s *RelayServer) relayData(src, dst net.Conn, sessionID string) {
	defer func() {
		src.Close()
		dst.Close()
		s.mu.Lock()
		if _, ok := s.sessions[sessionID]; ok {
			delete(s.sessions, sessionID)
			log.Printf("Session closed. Total active sessions: %d", len(s.sessions))
		}
		s.mu.Unlock()
	}()

	// Use a limited reader to prevent bandwidth abuse.
	// We wrap the source connection with a reader that will return EOF
	// after maxDataRelayed bytes have been read.
	limitedSrc := io.LimitReader(src, s.maxDataRelayed)

	// Continuously copy data, but also manage an inactivity timer.
	// We do this by setting a deadline on the underlying connection before each read.
	for {
		if err := src.SetReadDeadline(time.Now().Add(5 * time.Minute)); err != nil {
			log.Println("Could not set read deadline for a session.")
			return
		}

		// Copy a chunk of data. io.Copy will use our limitedSrc.
		// We copy in chunks to allow the deadline to be checked periodically.
		_, err := io.CopyN(dst, limitedSrc, 4096)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Println("A session timed out due to 5 minutes of inactivity.")
			} else if err != io.EOF {
				// This could be a "read past limit" error from LimitReader, which is fine.
				log.Println("Data relay finished for a session.")
			}
			// On any error (timeout, EOF, limit reached), we exit.
			return
		}
	}
}

func main() {
	maxDataRelayed := flag.Int64("max-data-relayed", 50, "Maximum data to relay per session in MB")
	flag.Parse()

	server := NewRelayServer(*maxDataRelayed * 1024 * 1024) // Convert MB to bytes
	server.Start(":8080")
}
