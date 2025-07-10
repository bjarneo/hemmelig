package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dothash/hemmelig-cli/internal/ui"
	"github.com/dothash/hemmelig-cli/internal/util"
)

func main() {
	relayServerAddr := flag.String("relay-server-addr", "localhost:8080", "Address of the relay server (e.g., localhost:8080)")
	sessionID := flag.String("session-id", "", "Session ID to join or create")
	maxFileSize := flag.Int64("max-file-size", 10, "Maximum file size in MB")
	flag.Parse()

	if *relayServerAddr == "" {
		fmt.Println("Usage: hemmelig -relay-server-addr <address> [-session-id <id>]")
		os.Exit(1)
	}

	reader := bufio.NewReader(os.Stdin)
	var command string
	var inputSessionID string

	if *sessionID != "" { // If session ID is provided as a flag, assume JOIN
		command = "JOIN"
		inputSessionID = *sessionID
	} else {
		fmt.Print("Do you want to (C)reate a new session or (J)oin an existing one? (C/J): ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(strings.ToUpper(choice))

		if choice == "C" {
			command = "CREATE"
			inputSessionID = "" // Relay server will generate
		} else if choice == "J" {
			command = "JOIN"
			fmt.Print("Enter the session ID to join: ")
			inputSessionID, _ = reader.ReadString('\n')
			inputSessionID = strings.TrimSpace(inputSessionID)
			if inputSessionID == "" {
				fmt.Println("Session ID cannot be empty for joining.")
				os.Exit(1)
			}
		} else {
			fmt.Println("Invalid choice. Please enter 'C' or 'J'.")
			os.Exit(1)
		}
	}

	// --- Nickname Prompt ---
	fmt.Print("Enter your nickname (or press Enter for a random Mr. Robot name): ")
	nickname, _ := reader.ReadString('\n')
	nickname = strings.TrimSpace(nickname)

	if nickname == "" {
		nickname = util.GenerateRandomNickname()
		fmt.Printf("No nickname entered. You are now %s.\n", nickname)
	}

	initialModel := ui.NewModel(*relayServerAddr, inputSessionID, nickname, command, *maxFileSize)

	p := tea.NewProgram(initialModel, tea.WithAltScreen())

	initialModel.Program = p // Assign the program to the model

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}