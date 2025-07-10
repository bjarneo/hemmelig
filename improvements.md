# Security Enhancements for Relay Server

The following are recommended security enhancements to harden the relay server against potential attacks.

### Denial of Service (DoS) Vulnerabilities:

1.  **Unbounded Session Creation (Memory Exhaustion):**
    *   **Threat:** An attacker can repeatedly connect and send the `CREATE` command, exhausting server memory.
    *   **Mitigation:** Implement a hard limit on the total number of active sessions the server will allow.

2.  **Connection Flooding & Slowloris Attack (Resource Exhaustion):**
    *   **Threat:** An attacker can open many connections without sending data, or send data very slowly, to tie up server resources.
    *   **Mitigation:** Set a read deadline on new connections. If a client fails to send its initial message within a timeout period, the server should close the connection.

3.  **Unbounded Data Relaying (Bandwidth Exhaustion):**
    *   **Threat:** A malicious client, once in a session, could send an unlimited amount of data, consuming all available bandwidth.
    *   **Mitigation:** Consider replacing `io.Copy` with a method that limits the total amount of data that can be relayed per session (e.g., `io.CopyN`). This provides defense-in-depth against compromised clients that ignore the client-side file size limit.
