## Relay Server Implementation

**Goal:** Implement a relay server to facilitate NAT traversal for Hemmelig clients, allowing them to communicate even when direct peer-to-peer connections are not possible.

**Current Status:**
- Basic server structure created (`cmd/relay-server/main.go`).
- Listens for incoming TCP connections.

**Next Steps:**
1.  **Session Management:** Implement logic to create and manage chat sessions (rooms) on the relay server.
    - Clients will need to send a session ID to join or create a session.
    - Each session will hold references to two connected clients.
2.  **Data Relaying:** Once two clients are in a session, the server will forward all incoming data from one client to the other.
    - The server will *not* decrypt or inspect the data, maintaining end-to-end encryption.
3.  **Client Modifications:** Update the Hemmelig client to:
    - Connect to the relay server instead of attempting direct peer-to-peer connections.
    - Send session creation/joining requests to the relay server.
    - Send and receive messages through the relay server.
4.  **Error Handling & Robustness:** Add comprehensive error handling, graceful disconnections, and potentially timeouts.
5.  **Testing:** Develop unit and integration tests for both the relay server and the modified client.