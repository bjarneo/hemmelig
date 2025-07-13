package ui

import (
	"fmt"
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/bjarneo/jot/internal/util"
)

type InitialModel struct {
	program         *tea.Program
	relayServerAddr string
	maxFileSize     int
	choice          string
	sessionIDInput  textinput.Model
	nicknameInput   textinput.Model
	state           initialState
	err             error
}

type initialState int

const (
	chooseCreateOrJoin initialState = iota
	enterSessionID
	enterNickname
)

func NewInitialModel(relayServerAddr string, maxFileSize int) *InitialModel {
	sessionIDInput := textinput.New()
	// Placeholder will be set dynamically based on choice
	nicknameInput := textinput.New()
	nicknameInput.Placeholder = "Your Nickname"

	m := &InitialModel{
		relayServerAddr: relayServerAddr,
		maxFileSize:     maxFileSize,
		sessionIDInput:  sessionIDInput,
		nicknameInput:   nicknameInput,
		state:           chooseCreateOrJoin,
	}
	// Initial focus depends on the first state, which is chooseCreateOrJoin, so no input is focused yet.
	return m
}

func (m *InitialModel) Init() tea.Cmd {
	return textinput.Blink // General blink command, specific input focus is handled in Update
}

func (m *InitialModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			switch m.state {
			case chooseCreateOrJoin:
				// Not used, selection is based on 'c' or 'j'
			case enterSessionID:
				// Session ID entered (or skipped for create), move to nickname
				m.state = enterNickname
				m.nicknameInput.SetValue("") // Clear nickname input in case of re-entry
				m.nicknameInput.Focus()
				return m, textinput.Blink
			case enterNickname:
				// Nickname entered, transition to the main UI
				nickname := strings.TrimSpace(m.nicknameInput.Value())
				if nickname == "" {
					nickname = util.GenerateRandomNickname()
				}
				sessionID := strings.TrimSpace(m.sessionIDInput.Value())
				command := m.choice

				mainModel := NewModel(m.relayServerAddr, sessionID, nickname, command, int64(m.maxFileSize))
				mainModel.Program = m.program
				return mainModel, mainModel.Init()
			}
		case tea.KeyRunes:
			if m.state == chooseCreateOrJoin {
				s := msg.String()
				s = strings.TrimSpace(strings.ToUpper(s))
				if s == "C" {
					m.choice = "CREATE"
					m.state = enterSessionID
					m.sessionIDInput.Placeholder = "Desired Session ID (optional, press Enter to auto-generate)"
					m.sessionIDInput.SetValue("") // Clear previous value
					m.sessionIDInput.Focus()
					return m, textinput.Blink
				} else if s == "J" {
					m.choice = "JOIN"
					m.state = enterSessionID
					m.sessionIDInput.Placeholder = "Session ID to Join"
					m.sessionIDInput.SetValue("") // Clear previous value
					m.sessionIDInput.Focus()
					return m, textinput.Blink
				}
			}
		}
	case error:
		m.err = msg
		return m, nil
	}

	switch m.state {
	case enterSessionID:
		m.sessionIDInput, cmd = m.sessionIDInput.Update(msg)
	case enterNickname:
		m.nicknameInput, cmd = m.nicknameInput.Update(msg)
	}

	return m, cmd
}

func (m *InitialModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress any key to quit.", m.err)
	}

	switch m.state {
	case chooseCreateOrJoin:
		return "Do you want to (C)reate a new session or (J)oin an existing one? (C/J)\n"
	case enterSessionID:
		var title string
		if m.choice == "CREATE" {
			title = "Enter desired Session ID (optional, press Enter to auto-generate):"
		} else {
			title = "Enter the Session ID to join:"
		}
		return fmt.Sprintf(
			"%s\n%s\n\n(esc to quit)",
			title,
			m.sessionIDInput.View(),
		)
	case enterNickname:
		return fmt.Sprintf(
			"Enter your nickname (or press Enter for a random one):\n%s\n\n(esc to quit)",
			m.nicknameInput.View(),
		)
	default:
		return ""
	}
}

func (m *InitialModel) SetProgram(p *tea.Program) {
	m.program = p
}

func StartInitialUI(relayServerAddr string, maxFileSize int) {
	initialModel := NewInitialModel(relayServerAddr, maxFileSize)
	p := tea.NewProgram(initialModel, tea.WithAltScreen())
	initialModel.SetProgram(p)

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
