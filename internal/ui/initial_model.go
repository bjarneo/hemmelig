package ui

import (
	"fmt"
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dothash/hemmelig-cli/internal/util"
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
	sessionIDInput.Placeholder = "Session ID"
	sessionIDInput.Focus()
	nicknameInput := textinput.New()
	nicknameInput.Placeholder = "Your Nickname"

	return &InitialModel{
		relayServerAddr: relayServerAddr,
		maxFileSize:     maxFileSize,
		sessionIDInput:  sessionIDInput,
		nicknameInput:   nicknameInput,
		state:           chooseCreateOrJoin,
	}
}

func (m *InitialModel) Init() tea.Cmd {
	return textinput.Blink
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
				m.state = enterNickname
				m.nicknameInput.Focus()
				return m, textinput.Blink
			case enterNickname:
				// Transition to the main UI
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
					m.state = enterNickname
					m.nicknameInput.Focus()
					return m, textinput.Blink
				} else if s == "J" {
					m.choice = "JOIN"
					m.state = enterSessionID
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
		return fmt.Sprintf(
			"Enter the session ID to join:\n%s\n\n(esc to quit)",
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
