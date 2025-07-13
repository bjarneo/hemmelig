package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/bjarneo/jot/internal/ui"
)

func main() {
	const maxFileSize = 10 // MB
	relayServerAddr := flag.String("relay-server", "relay.jot.app:443", "Address of the relay server (e.g., localhost:8080)")
	flag.Parse()

	if *relayServerAddr == "" {
		fmt.Println("Usage: jot -relay-server <address>")
		os.Exit(1)
	}

	ui.StartInitialUI(*relayServerAddr, maxFileSize)
}
