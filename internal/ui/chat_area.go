package ui

import (
	"fmt"
	"os"
	"path/filepath" // Added for filepath.Glob
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
	ta.SetHeight(1)           // Starts as single line, expands automatically

	// Define styles for the textarea prompt and text
	promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	ta.FocusedStyle.Prompt = promptStyle // Assign the style object
	ta.BlurredStyle.Prompt = promptStyle // Assign the style object (can be different if desired)
	ta.ShowLineNumbers = false

	vp := viewport.New(initialWidth, initialHeight-3) // Initial guess for viewport height

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
		case tea.KeyTab:
			currentText := m.textarea.Value()
			if strings.HasPrefix(currentText, "/send ") {
				partialPath := strings.TrimPrefix(currentText, "/send ")

				// If tilde for home directory is used, expand it
				if strings.HasPrefix(partialPath, "~") {
					if homeDir, err := os.UserHomeDir(); err == nil {
						partialPath = filepath.Join(homeDir, partialPath[1:])
					}
				}

				// Add a '*' for globbing if not already present or to expand directory
				globPath := partialPath
				if !strings.HasSuffix(globPath, "*") {
					globPath += "*"
				}

				matches, err := filepath.Glob(globPath)
				if err == nil && len(matches) > 0 {
					if len(matches) == 1 {
						// Single match, complete it
						m.textarea.SetValue("/send " + matches[0])
						m.textarea.CursorEnd() // Move cursor to end
					} else {
						// Multiple matches, find common prefix
						prefix := commonPrefix(matches)
						if prefix != "" && len(prefix) > len(partialPath) {
							m.textarea.SetValue("/send " + prefix)
							m.textarea.CursorEnd()
						}
					}
				}
				// Prevent Tab from being processed further (e.g., by terminal)
				return m, nil // Absorb the Tab key event
			}
		}
	case FocusTextareaMsg:
		cmds = append(cmds, m.textarea.Focus())
		// WindowSizeMsg is handled by SetDimensions, called by the main model.
	}

	return m, tea.Batch(cmds...)
}

// commonPrefix finds the longest common prefix among a list of strings.
func commonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}
	prefix := strs[0]
	for _, s := range strs[1:] {
		for !strings.HasPrefix(s, prefix) {
			if len(prefix) == 0 {
				return ""
			}
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}

// SetDimensions updates the internal width and height, and resizes components.
// This should be called by the main model when it processes tea.WindowSizeMsg.
// The height passed here is the total height allocated for the chat area (viewport + input).
func (m *ChatAreaModel) SetDimensions(width, totalAllocatedHeight int) {
	m.width = width
	m.height = totalAllocatedHeight

	// Define the intended input box style to measure its chrome (borders + vertical padding)
	// This must match the style defined later in View() for consistency.
	// Assuming NormalBorder (1px top, 1px bottom = 2px border) and no vertical padding for the container.
	// If View() adds vertical padding to inputStyle, it must be accounted for here.
	inputBoxStyleForMeasurement := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true)
		// If inputStyle in View() has PaddingTop/Bottom, add them here too:
		// PaddingTop(0).
		// PaddingBottom(0).

	inputBoxChromeHeight := inputBoxStyleForMeasurement.GetVerticalBorderSize() + inputBoxStyleForMeasurement.GetVerticalPadding()

	calculatedInputBoxHeight := m.textarea.Height() + inputBoxChromeHeight
	if calculatedInputBoxHeight < (1 + inputBoxChromeHeight) { // Min 1 line of text area content
		calculatedInputBoxHeight = 1 + inputBoxChromeHeight
	}

	// Ensure the calculated height does not exceed the total allocated height
	if calculatedInputBoxHeight > totalAllocatedHeight {
		calculatedInputBoxHeight = totalAllocatedHeight
	}
	// Assign to a variable that View might also use or re-calculate similarly
	// For now, SetDimensions determines the split.
	inputBoxFinalHeight := calculatedInputBoxHeight

	vpHeight := totalAllocatedHeight - inputBoxFinalHeight
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
		Width(m.width).                                           // Outer width for the viewport's styled box
		Height(m.viewport.Height).                                // Calculated height for the viewport's styled box
		Border(lipgloss.NormalBorder(), true, true, false, true). // Top, Right, No Bottom, Left
		PaddingLeft(1).
		PaddingRight(1)
	m.viewportStyle = currentViewportStyle

	// Input box style
	// Define the base style properties first (border, padding)
	baseInputStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true). // Full border for input box
		PaddingLeft(1).                        // Padding for text area within its border
		PaddingRight(1)
		// PaddingTop(0). // Explicitly 0, or consistent with SetDimensions measurement
		// PaddingBottom(0).

	// Calculate the required height for the input box container
	inputBoxChromeHeightView := baseInputStyle.GetVerticalBorderSize() + baseInputStyle.GetVerticalPadding()
	inputBoxRequiredHeightView := m.textarea.Height() + inputBoxChromeHeightView

	minInputBoxHeight := 1 + inputBoxChromeHeightView // Min 1 line of content
	if inputBoxRequiredHeightView < minInputBoxHeight {
		inputBoxRequiredHeightView = minInputBoxHeight
	}

	// Ensure the height doesn't exceed the total allocated height for the chat area (m.height)
	// and also doesn't exceed the portion of m.height not used by the viewport.
	// The viewport height (m.viewport.Height) was set by SetDimensions.
	// So, the input box should take m.height - m.viewport.Height.
	// This ensures consistency with SetDimensions.
	finalInputBoxHeight := m.height - m.viewport.Height
	if finalInputBoxHeight < minInputBoxHeight { // Safety, should not happen if SetDimensions is correct
		finalInputBoxHeight = minInputBoxHeight
	}

	m.inputStyle = baseInputStyle.Copy().
		Width(m.width).
		Height(finalInputBoxHeight) // Use the height determined by SetDimensions' allocation

	// Update textarea prompt dynamically
	m.textarea.Prompt = m.userNickname + ": "
	// The styles for the prompt (FocusedStyle.Prompt, BlurredStyle.Prompt) were set in NewChatAreaModel.
	// The textarea component will use those styles when rendering its prompt.
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
