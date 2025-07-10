package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Session represents a chat session with two connected clients.
type Session struct {
	ID      string
	Clients [2]net.Conn
	mu      sync.Mutex
}

// RelayServer holds the state of the relay server.
type RelayServer struct {
	sessions map[string]*Session
	mu       sync.Mutex
}

// NewRelayServer creates a new RelayServer instance.
func NewRelayServer() *RelayServer {
	return &RelayServer{
		sessions: make(map[string]*Session),
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
	log.Printf("New connection from %s", conn.RemoteAddr())

	// Set a deadline for reading the initial message to prevent Slowloris attacks.
	if err := conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		log.Printf("Could not set read deadline for %s: %v", conn.RemoteAddr(), err)
		conn.Close()
		return
	}

	reader := bufio.NewReader(conn)
	messageBytes, err := reader.ReadBytes('\n')
	if err != nil {
		log.Printf("Error reading initial message from %s: %v", conn.RemoteAddr(), err)
		conn.Close()
		return
	}

	// Reset the deadline to allow for long-lived connections.
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		log.Printf("Could not reset read deadline for %s: %v", conn.RemoteAddr(), err)
		conn.Close()
		return
	}

	var clientMsg ClientMessage
	if err := json.Unmarshal(messageBytes, &clientMsg); err != nil {
		log.Printf("Error unmarshaling initial message from %s: %v", conn.RemoteAddr(), err)
		conn.Close()
		return
	}



	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID := clientMsg.SessionID
	session, exists := s.sessions[sessionID]

	switch clientMsg.Command {
	case "CREATE":
		if exists {
			log.Printf("Session %s already exists, cannot create.", sessionID)
			conn.Write([]byte("Error: Session already exists\n"))
			conn.Close()
			return
		}
		sessionID = uuid.New().String()
		session = &Session{ID: sessionID}
		session.Clients[0] = conn
		s.sessions[sessionID] = session
		log.Printf("Created new session %s for client %s", sessionID, conn.RemoteAddr())
		conn.Write([]byte(fmt.Sprintf("Session created: %s\n", sessionID)))

	case "JOIN":
		if !exists || session.Clients[1] != nil {
			log.Printf("Session %s does not exist or is full.", sessionID)
			conn.Write([]byte("Error: Session not found or full\n"))
			conn.Close()
			return
		}
		session.Clients[1] = conn
		log.Printf("Client %s joined session %s", conn.RemoteAddr(), sessionID)
		conn.Write([]byte(fmt.Sprintf("Joined session: %s\n", sessionID)))

		// Start relaying data between clients
		go s.relayData(session.Clients[0], session.Clients[1], sessionID)
		go s.relayData(session.Clients[1], session.Clients[0], sessionID)

	default:
		log.Printf("Unknown command from %s: %s", conn.RemoteAddr(), clientMsg.Command)
		conn.Write([]byte("Error: Unknown command\n"))
		conn.Close()
		return
	}
}

// relayData relays data from src to dst.
func (s *RelayServer) relayData(src, dst net.Conn, sessionID string) {
	defer func() {
		src.Close()
		dst.Close()
		log.Printf("Session %s closed due to disconnection.", sessionID)
		s.mu.Lock()
		delete(s.sessions, sessionID)
		s.mu.Unlock()
	}()

	_, err := io.Copy(dst, src)
	if err != nil {
		log.Printf("Error relaying data in session %s: %v", sessionID, err)
	}
}

func main() {
	server := NewRelayServer()
	server.Start(":8080") // Default port for the relay server
}