package ui

import "github.com/charmbracelet/lipgloss"

var (
	ViewportStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62"))
	TextareaStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205"))
	StatusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	ErrorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	SenderStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	ReceiverStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	SystemStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	TimestampStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Faint(true)
)
