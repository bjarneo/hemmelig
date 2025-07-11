package ui

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SubmitInputMsg is a tea.Msg that signals text was submitted from the textarea.
type SubmitInputMsg struct{ Content string }

// FocusTextareaMsg is a tea.Msg to command the ChatAreaModel to focus its textarea.
type FocusTextareaMsg struct{}

// ChatAreaModel represents the UI model for the chat message display and input area.
type ChatAreaModel struct {
	viewport      viewport.Model
	textarea      textarea.Model
	width         int
	height        int // Represents the total height allocated to this component
	senderStyle   lipgloss.Style
	viewportStyle lipgloss.Style
	inputStyle    lipgloss.Style

	messageRenderer *lipgloss.Renderer
	// Nickname for the "You: " prompt, could be configurable
	userNickname string
}

// Message struct for displaying messages, consistent with how renderMessages expects it.
// This is now part of the ui package.
type Message struct {
	Timestamp time.Time
	Sender    string
	Content   string
}

// NewChatAreaModel creates a new UI model for the chat area.
// It requires initial dimensions.
func NewChatAreaModel(initialWidth, initialHeight int, userNickname string) ChatAreaModel {
	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	// ta.Focus() // Focus will be managed by the main model

	ta.CharLimit = 0
	ta.SetWidth(initialWidth) // Will be updated by WindowSizeMsg
	ta.SetHeight(1)          // Starts as single line, expands automatically

	// Define styles for the textarea prompt and text
	// The prompt text will be dynamic based on userNickname
	focusedPromptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	blurredPromptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")) // Or a different color for blurred

	ta.FocusedStyle.Prompt = focusedPromptStyle.Render(userNickname + ": ")
	ta.BlurredStyle.Prompt = blurredPromptStyle.Render(userNickname + ": ")
	ta.ShowLineNumbers = false

	vp := viewport.New(initialWidth, initialHeight-3) // Initial guess for viewport height
	// vp.Style, inputStyle, viewportStyle will be defined in View() based on current width/height

	return ChatAreaModel{
		textarea:        ta,
		viewport:        vp,
		width:           initialWidth,
		height:          initialHeight, // Total height for this component
		userNickname:    userNickname,
		messageRenderer: lipgloss.DefaultRenderer(),
		senderStyle:     lipgloss.NewStyle().Bold(true), // Example, can be configured
	}
}

// Init is a no-op for a sub-component.
func (m ChatAreaModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the ChatAreaModel.
// It returns the updated ChatAreaModel and a tea.Cmd.
// Note: This model itself is returned, not tea.Model, as it's a concrete type.
func (m ChatAreaModel) Update(msg tea.Msg) (ChatAreaModel, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
		cmds  []tea.Cmd
	)

	m.textarea, tiCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, tiCmd, vpCmd)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		// tea.KeyCtrlC, tea.KeyEsc are handled by the main model.
		case tea.KeyEnter:
			inputValue := strings.TrimSpace(m.textarea.Value())
			if inputValue != "" {
				m.textarea.Reset()
				// Return a command to the main model indicating input was submitted
				return m, func() tea.Msg { return SubmitInputMsg{Content: inputValue} }
			}
		}
	case FocusTextareaMsg:
		cmds = append(cmds, m.textarea.Focus())
	// WindowSizeMsg is handled by SetDimensions, called by the main model.
	}

	return m, tea.Batch(cmds...)
}

// SetDimensions updates the internal width and height, and resizes components.
// This should be called by the main model when it processes tea.WindowSizeMsg.
// The height passed here is the total height allocated for the chat area (viewport + input).
func (m *ChatAreaModel) SetDimensions(width, totalAllocatedHeight int) {
	m.width = width
	m.height = totalAllocatedHeight

	// Input area height calculation (border + text + border)
	// Textarea height adapts, so use its current requirement.
	inputBoxVisualHeight := m.textarea.Height() + 2 // +2 for top/bottom border of the input box container
	if inputBoxVisualHeight < 3 {
		inputBoxVisualHeight = 3 // Minimum 1 line content + 2 border lines
	}
	if inputBoxVisualHeight > totalAllocatedHeight { // Cap input height if it's too large
		inputBoxVisualHeight = totalAllocatedHeight
	}


	vpHeight := totalAllocatedHeight - inputBoxVisualHeight
	if vpHeight < 0 {
		vpHeight = 0
	}

	m.viewport.Width = m.width // Viewport uses full component width before its own padding/borders
	m.viewport.Height = vpHeight
	m.textarea.SetWidth(m.width) // Textarea uses full component width before its container's padding/borders

	// Styles (viewportStyle, inputStyle) are dynamically sized in View()
	// So, no need to set their width/height here directly, but m.width/m.height (overall)
	// and calculated vpHeight/inputBoxVisualHeight are used in View().
}

// View renders the chat area (viewport and input).
// It takes the messages to display as a parameter from the main model.
func (m *ChatAreaModel) View(messagesToDisplay []Message) string {
	// Update viewport content
	renderedMsgs := m.renderMessages(messagesToDisplay)
	m.viewport.SetContent(renderedMsgs)
	// Scroll to bottom if content changed, or if explicitly told to.
	// Main model can manage scroll state or this component can always goto bottom.
	// For simplicity here, let's assume main model handles when to scroll or we always scroll.
	m.viewport.GotoBottom()


	// --- Define styles dynamically based on current dimensions ---
	// Viewport style: Border on top, left, right. No bottom border as input box provides it.
	// Padding is applied to the content area of the viewport.
	currentViewportStyle := lipgloss.NewStyle().
		Width(m.width). // Outer width for the viewport's styled box
		Height(m.viewport.Height). // Calculated height for the viewport's styled box
		Border(lipgloss.NormalBorder(), true, true, false, true). // Top, Right, No Bottom, Left
		PaddingLeft(1).
		PaddingRight(1)
	m.viewportStyle = currentViewportStyle


	// Input box style
	// The textarea.Height() determines how many lines it needs. Add 2 for container border.
	inputBoxVisualHeight := m.textarea.Height() + 2
	if inputBoxVisualHeight < 3 { inputBoxVisualHeight = 3 }
	if inputBoxVisualHeight > m.height { // Cap input height if it's too large for allocated space
		// This scenario should ideally be prevented by SetDimensions logic
		inputBoxVisualHeight = m.height
	}


	currentInputStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true). // Full border for input box
		Width(m.width).                        // Outer width
		Height(inputBoxVisualHeight).
		PaddingLeft(1). // Padding for text area within its border
		PaddingRight(1)
	m.inputStyle = currentInputStyle

	// Update textarea prompt dynamically if nickname can change or for consistent styling
	// This assumes userNickname in ChatAreaModel is kept up-to-date by main model if it can change.
	// Or, it's set once at init.
	promptText := m.userNickname + ": "
	if m.textarea.Focused() {
		m.textarea.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render(promptText)
	} else {
		m.textarea.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render(promptText) // Or different style for blurred
	}
	textareaViewString := m.textarea.View()

	// Combine viewport and input box
	return lipgloss.JoinVertical(lipgloss.Left,
		m.viewportStyle.Render(m.viewport.View()),
		m.inputStyle.Render(textareaViewString),
	)
}

// renderMessages formats and wraps messages for display.
// It now takes messages as a parameter.
func (m *ChatAreaModel) renderMessages(messagesToDisplay []Message) string {
	var renderedOutputLines []string

	localTimestampStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Faint(true)
	// Using m.userNickname to differentiate styling for user's own messages vs peer's.
	// System/Error senders will be handled specially.

	viewportInternalContentWidth := m.width - m.viewportStyle.GetHorizontalBorderSize() - m.viewportStyle.GetHorizontalPadding()
	if viewportInternalContentWidth < 1 {
		viewportInternalContentWidth = 1
	}

	for _, msg := range messagesToDisplay {
		timestampStr := localTimestampStyle.Render(msg.Timestamp.Format("15:04"))

		var senderStr string
		var prefix string
		var finalContent string

		if msg.Sender == "System" || msg.Sender == "Error" {
			isError := msg.Sender == "Error"
			systemOrErrorStyle := lipgloss.NewStyle().Italic(true)
			if isError {
				systemOrErrorStyle = systemOrErrorStyle.Foreground(lipgloss.Color("196")) // Error color from styles.go
			} else {
				systemOrErrorStyle = systemOrErrorStyle.Foreground(lipgloss.Color("244")) // System color from styles.go
			}
			// For system/error, content is directly styled. Prefix is just timestamp.
			// Content is assumed to be raw and will be wrapped.
			prefix = fmt.Sprintf("%s --- ", timestampStr) // System messages might not need <Sender>
			finalContent = systemOrErrorStyle.Render(msg.Content)
		} else if msg.Sender == m.userNickname {
			senderStr = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render("<" + msg.Sender + ">") // User's sender color (SenderStyle)
			prefix = fmt.Sprintf("%s %s ", timestampStr, senderStr)
			finalContent = msg.Content // Raw content for user's own messages
		} else { // Peer's message
			senderStr = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("<" + msg.Sender + ">") // Peer's sender color (ReceiverStyle)
			prefix = fmt.Sprintf("%s %s ", timestampStr, senderStr)
			finalContent = msg.Content // Raw content for peer messages
		}

		prefixLen := lipgloss.Width(prefix)
		maxContentWidth := viewportInternalContentWidth - prefixLen
		if maxContentWidth < 1 {
			maxContentWidth = 1
		}

		renderer := m.messageRenderer
		if renderer == nil {
			renderer = lipgloss.DefaultRenderer()
		}

		messageStyle := lipgloss.NewStyle().Width(maxContentWidth).Renderer(renderer)
		renderedContent := messageStyle.Render(finalContent) // Render the (potentially pre-styled for system) content

		contentLines := strings.Split(renderedContent, "\n")

		fullMessageLine := prefix + contentLines[0]
		renderedOutputLines = append(renderedOutputLines, fullMessageLine)

		if len(contentLines) > 1 {
			indentation := strings.Repeat(" ", prefixLen)
			for i := 1; i < len(contentLines); i++ {
				renderedOutputLines = append(renderedOutputLines, indentation+contentLines[i])
			}
		}
	}
	return strings.Join(renderedOutputLines, "\n")
}
