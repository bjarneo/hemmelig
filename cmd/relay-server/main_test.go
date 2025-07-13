package main

import (
	"bufio"
	"encoding/json"
	"net"
	"testing"
)

func startServer(t *testing.T) (*RelayServer, net.Listener) {
	server := NewRelayServer(1024 * 1024)
	listener, err := net.Listen("tcp", ":8081")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	go server.Start(listener)
	return server, listener
}

func connect(t *testing.T) net.Conn {
	conn, err := net.Dial("tcp", ":8081")
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	return conn
}

func TestSessionCreation(t *testing.T) {
	_, listener := startServer(t)
	defer listener.Close()
	conn := connect(t)
	defer conn.Close()

	msg := ClientMessage{
		Command: "CREATE",
	}
	msgBytes, _ := json.Marshal(msg)
	conn.Write(append(msgBytes, '\n'))

	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	var respMsg map[string]interface{}
	if err := json.Unmarshal([]byte(response), &respMsg); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if respMsg["type"] != "session_created" {
		t.Errorf("Expected response type 'session_created', got '%s'", respMsg["type"])
	}

	if _, ok := respMsg["sessionID"].(string); !ok {
		t.Error("Response has no sessionID")
	}
}

func TestJoinSession(t *testing.T) {
	server, listener := startServer(t)
	defer listener.Close()

	// Create a session
	conn1 := connect(t)
	defer conn1.Close()
	createMsg := ClientMessage{Command: "CREATE"}
	createBytes, _ := json.Marshal(createMsg)
	conn1.Write(append(createBytes, '\n'))
	reader1 := bufio.NewReader(conn1)
	createResp, _ := reader1.ReadString('\n')
	var createRespMsg map[string]interface{}
	json.Unmarshal([]byte(createResp), &createRespMsg)
	sessionID := createRespMsg["sessionID"].(string)

	// Join the session
	conn2 := connect(t)
	defer conn2.Close()
	joinMsg := ClientMessage{Command: "JOIN", SessionID: sessionID}
	joinBytes, _ := json.Marshal(joinMsg)
	conn2.Write(append(joinBytes, '\n'))

	// Check if the second client received the public key of the first client
	reader2 := bufio.NewReader(conn2)
	joinResp, err := reader2.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	var joinRespMsg map[string]interface{}
	if err := json.Unmarshal([]byte(joinResp), &joinRespMsg); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if joinRespMsg["type"] != "public_key" {
		t.Errorf("Expected response type 'public_key', got '%s'", joinRespMsg["type"])
	}

	// Read the "Joined session" message
	_, err = reader2.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	// Check if the session has two clients
	s, exists := server.sessions[sessionID]
	if !exists {
		t.Fatalf("Session not found")
	}
	if len(s.Clients) != 2 {
		t.Errorf("Expected 2 clients in session, got %d", len(s.Clients))
	}
}

func TestMessageRelay(t *testing.T) {
	server, listener := startServer(t)
	defer listener.Close()

	// Create a session
	conn1 := connect(t)
	defer conn1.Close()
	createMsg := ClientMessage{Command: "CREATE"}
	createBytes, _ := json.Marshal(createMsg)
	conn1.Write(append(createBytes, '\n'))
	reader1 := bufio.NewReader(conn1)
	createResp, _ := reader1.ReadString('\n')
	var createRespMsg map[string]interface{}
	json.Unmarshal([]byte(createResp), &createRespMsg)
	sessionID := createRespMsg["sessionID"].(string)
	var client1ID string
	for id := range server.sessions[sessionID].Clients {
		client1ID = id
	}

	// Join the session
	conn2 := connect(t)
	defer conn2.Close()
	joinMsg := ClientMessage{Command: "JOIN", SessionID: sessionID}
	joinBytes, _ := json.Marshal(joinMsg)
	conn2.Write(append(joinBytes, '\n'))
	reader2 := bufio.NewReader(conn2)
	reader2.ReadString('\n') // public key message

	var client2ID string
	for id := range server.sessions[sessionID].Clients {
		if id != client1ID {
			client2ID = id
		}
	}

	// Send a message from client 1 to client 2
	relayMsg := map[string]interface{}{
		"type":       "message",
		"recipient":  client2ID,
		"ciphertext": "hello",
	}
	relayBytes, _ := json.Marshal(relayMsg)
	conn1.Write(append(relayBytes, '\n'))

	// Read the public key and "Joined session" messages and ignore them
	_, err := reader2.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}
	_, err = reader2.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	// Check if client 2 received the message
	relayResp, err := reader2.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	var relayRespMsg map[string]interface{}
	if err := json.Unmarshal([]byte(relayResp), &relayRespMsg); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if relayRespMsg["type"] != "message" {
		t.Errorf("Expected response type 'message', got '%s'", relayRespMsg["type"])
	}
	if relayRespMsg["ciphertext"] != "hello" {
		t.Errorf("Expected ciphertext 'hello', got '%s'", relayRespMsg["ciphertext"])
	}
	if relayRespMsg["sender"] != client1ID {
		t.Errorf("Expected sender '%s', got '%s'", client1ID, relayRespMsg["sender"])
	}
}
