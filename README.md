<p align="center">
  <img src="logo.png" alt="Hemmelig Logo" width="200"/>
</p>

# Hemmelig: Encrypted Chat with Relay Server

This project is the official TUI version of [Hemmelig.app](https://github.com/HemmeligOrg/Hemmelig.app), created by the original author.

Hemmelig is a secure, end-to-end encrypted chat and file transfer application designed for direct peer-to-peer communication. It uses a **Relay Server** to overcome Network Address Translation (NAT) issues, allowing users behind restrictive firewalls to connect securely. The relay server acts as a simple, hardened switchboard, forwarding encrypted data between clients without ever decrypting or inspecting the messages.

## Features

- **End-to-End Encryption:** All messages and files are encrypted using **AES-256-GCM**. The 256-bit symmetric key is derived from a Curve25519 key exchange.
- **NAT Traversal:** The relay server allows clients to connect even when behind restrictive firewalls.
- **Secure File Transfer:** Securely send files between connected peers with a configurable size limit (default 10MB).
- **Terminal User Interface (TUI):** A responsive and user-friendly terminal UI that adapts to your window size.
- **Tab Completion:** Basic tab completion for file paths when using the `/send` command.
- **Trust On First Use (TOFU):** Verify peer identity through public key fingerprints.

## Installation

You can install the `hemmelig` client by downloading a pre-compiled binary from the [GitHub Releases page](https://github.com/bjarneo/hemmelig/releases) or by using the installation script below.

### Install Script (Linux & macOS)

This script will automatically detect your OS and architecture, download the latest binary, verify its checksum, and install it to `$HOME/.local/bin`.

```bash
curl -sSL https://raw.githubusercontent.com/bjarneo/hemmelig/main/install.sh | sh
```

After installation, you may need to add the installation directory to your shell's `PATH`.

```bash
# For Bash users (usually in ~/.bashrc)
export PATH="$HOME/.local/bin:$PATH"

# For Zsh users (usually in ~/.zshrc)
export PATH="$HOME/.local/bin:$PATH"
```

## How to Use

### 1. Build the Applications

Ensure you have Go installed. Navigate to the project root directory and run:

```bash
go build -o hemmelig ./cmd/hemmelig
go build -o relay-server ./cmd/relay-server
```

### 2. Start the Relay Server

Run the relay server in a terminal. By default, it listens on port `8080`.

```bash
./relay-server
```

You can customize the server's behavior with the following flag:

- `-max-data-relayed <MB>`: Sets the maximum amount of data (in MB) a single session can relay before being terminated. Defaults to 50MB.

### 3. Start the Hemmelig Client

Open a new terminal to start the client. You will be prompted to create or join a session.

```bash
./hemmelig
```

The client can be customized with the following flags:

- `-relay-server-addr <address>`: Specifies the address of the relay server (e.g., `localhost:8080`).
- `-session-id <id>`: Immediately joins a session with the given ID without prompting.
- `-max-file-size <MB>`: Sets the maximum size (in MB) for files you can send. Defaults to 10MB.

## Security Features

The relay server has been hardened against several common attacks:

- **Connection Flooding / Slowloris Attack:** The server enforces a 30-second timeout for new connections. If a client fails to send its initial `CREATE` or `JOIN` command within this window, its connection is dropped.
- **Bandwidth Exhaustion:** To prevent a malicious client from consuming unlimited bandwidth, the total amount of data that can be relayed in a single session is capped (default 50MB, configurable via the `-max-data-relayed` flag).
- **Inactivity Timeout:** Sessions are automatically terminated if no data is sent or received from either client for 5 minutes, freeing up server resources.

## Communication Flow

```mermaid
sequenceDiagram
    participant C1 as Client 1
    participant RS as Relay Server
    participant C2 as Client 2

    C1->>RS: 1. TCP Connect & CREATE Session
    RS-->>C1: 2. Session ID
    Note over C1: Waiting for peer...

    C2->>RS: 3. TCP Connect & JOIN Session (with ID)
    RS-->>C2: 4. Join Confirmation
    RS-->>C1: 5. Peer Joined Notification

    par Key Exchange
        C1->>RS: Send Public Key
        RS->>C2: Forward Public Key
    and
        C2->>RS: Send Public Key
        RS->>C1: Forward Public Key
    end
    Note over C1,C2: Both clients now have the shared secret

    loop Encrypted Communication
        C1->>RS: Encrypted Data (Messages/Files)
        RS->>C2: Forward Encrypted Data
        C2->>RS: Encrypted Data (Messages/Files)
        RS->>C1: Forward Encrypted Data
    end
```

## Trust On First Use (TOFU)

**TOFU** is a security model where the first time you connect to a peer, you save their public key fingerprint. On all future connections, the client will verify that the fingerprint matches.

In Hemmelig, after the key exchange, the client displays the peer's fingerprint. It is crucial for you to **manually verify this fingerprint** with your peer through a trusted out-of-band channel (e.g., a phone call). This ensures your connection is secure and not being intercepted by a Man-in-the-Middle (MitM) attack.
