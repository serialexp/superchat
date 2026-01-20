package modal

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ErrorModal displays an error message that must be acknowledged
type ErrorModal struct {
	title   string
	message string
	onClose func() tea.Cmd
}

// NewErrorModal creates a new error modal
func NewErrorModal(title, message string, onClose func() tea.Cmd) *ErrorModal {
	return &ErrorModal{
		title:   title,
		message: message,
		onClose: onClose,
	}
}

// Type returns the modal type
func (m *ErrorModal) Type() ModalType {
	return ModalError
}

// HandleKey processes keyboard input
func (m *ErrorModal) HandleKey(msg tea.KeyMsg) (bool, Modal, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc", " ":
		// Close modal on any of these keys
		var cmd tea.Cmd
		if m.onClose != nil {
			cmd = m.onClose()
		}
		return true, nil, cmd
	}
	return true, m, nil
}

// Update handles bubbletea messages
func (m *ErrorModal) Update(msg tea.Msg) tea.Cmd {
	return nil
}

// Init returns the initial command
func (m *ErrorModal) Init() tea.Cmd {
	return nil
}

// Render returns the modal content
func (m *ErrorModal) Render(width, height int) string {
	errorColor := lipgloss.Color("#FF5555") // Red for errors

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(errorColor).
		MarginBottom(1).
		Align(lipgloss.Center)

	messageStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		MarginBottom(1)

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true)

	// Build content
	var content string

	// Title
	content += titleStyle.Render(m.title) + "\n\n"

	// Message
	content += messageStyle.Render(m.message) + "\n\n"

	// Hint
	content += hintStyle.Render("Press Enter or Esc to dismiss")

	// Create border style
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(errorColor).
		Padding(1, 2)

	// Calculate modal size
	modalWidth := 50
	if width < modalWidth+4 {
		modalWidth = width - 4
	}

	// Render in bordered box
	box := borderStyle.Width(modalWidth - 4).Render(content)

	// Center the modal
	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		box,
	)
}

// IsBlockingInput returns whether this modal blocks input to the main view
func (m *ErrorModal) IsBlockingInput() bool {
	return true
}
