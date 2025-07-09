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
	listenAddr := flag.String("listen", "", "Address to listen on (e.g., :8080)")
	connectAddr := flag.String("connect", "", "Address to connect to (e.g., localhost:8080)")
	flag.Parse()

	if (*listenAddr == "" && *connectAddr == "") || (*listenAddr != "" && *connectAddr != "") {
		fmt.Println("Usage: go-chat -listen <address> OR go-chat -connect <address>")
		os.Exit(1)
	}

	// --- Nickname Prompt ---
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter your nickname (or press Enter for a random Mr. Robot name): ")
	nickname, _ := reader.ReadString('\n')
	nickname = strings.TrimSpace(nickname)

	if nickname == "" {
		nickname = util.GenerateRandomNickname()
		fmt.Printf("No nickname entered. You are now %s.\n", nickname)
	}

	var initialModel *ui.Model
	if *listenAddr != "" {
		initialModel = ui.NewModel("server", *listenAddr, nickname)
	} else {
		initialModel = ui.NewModel("client", *connectAddr, nickname)
	}

	p := tea.NewProgram(initialModel, tea.WithAltScreen())

	initialModel.Program = p // Assign the program to the model

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}