package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dothash/hemmelig-cli/internal/ui"
)

func main() {
	const maxFileSize = 10 // MB
	relayServerAddr := flag.String("relay-server-addr", "relay.hemmelig.app:443", "Address of the relay server (e.g., localhost:8080)")
	flag.Parse()

	if *relayServerAddr == "" {
		fmt.Println("Usage: hemmelig -relay-server-addr <address>")
		os.Exit(1)
	}

	ui.StartInitialUI(*relayServerAddr, maxFileSize)
}

