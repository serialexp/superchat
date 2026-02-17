package modal

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConnectionFailedRetryMsg is sent when user wants to retry connection
type ConnectionFailedRetryMsg struct{}

// ConnectionFailedSwitchServerMsg is sent when user wants to switch to server selector
type ConnectionFailedSwitchServerMsg struct{}

// ConnectionFailedTryMethodMsg is sent when user wants to try a different connection method
type ConnectionFailedTryMethodMsg struct{}

// ConnectionFailedModal displays connection failure with recovery options
type ConnectionFailedModal struct {
	serverAddr         string
	errorMessage       string
	serverDisconnected bool   // True if server sent DISCONNECT message
	disconnectReason   string // Reason from server DISCONNECT message
	cursor             int    // 0 = Retry, 1 = Try Different Method, 2 = Switch Server, 3 = Quit
}

// NewConnectionFailedModal creates a new connection failed modal
func NewConnectionFailedModal(serverAddr string, errorMessage string) *ConnectionFailedModal {
	return &ConnectionFailedModal{
		serverAddr:         serverAddr,
		errorMessage:       errorMessage,
		serverDisconnected: false,
		disconnectReason:   "",
		cursor:             0,
	}
}

// NewConnectionFailedModalWithReason creates a connection failed modal with server disconnect reason
func NewConnectionFailedModalWithReason(serverAddr string, reason string) *ConnectionFailedModal {
	return &ConnectionFailedModal{
		serverAddr:         serverAddr,
		errorMessage:       "",
		serverDisconnected: true,
		disconnectReason:   reason,
		cursor:             0,
	}
}

// Type returns the modal type
func (m *ConnectionFailedModal) Type() ModalType {
	return ModalConnectionFailed
}

// HandleKey processes keyboard input
func (m *ConnectionFailedModal) HandleKey(msg tea.KeyMsg) (bool, Modal, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return true, m, nil

	case "down", "j":
		if m.cursor < 3 {
			m.cursor++
		}
		return true, m, nil

	case "r":
		// Retry connection (shortcut)
		return true, nil, func() tea.Msg {
			return ConnectionFailedRetryMsg{}
		}

	case "m":
		// Try different method (shortcut)
		return true, nil, func() tea.Msg {
			return ConnectionFailedTryMethodMsg{}
		}

	case "s", "ctrl+l":
		// Switch to server selector (shortcut)
		return true, nil, func() tea.Msg {
			return ConnectionFailedSwitchServerMsg{}
		}

	case "esc":
		// Dismiss modal and keep browsing cached data
		return true, nil, nil

	case "q":
		// Quit application
		return true, nil, tea.Quit

	case "enter":
		// Select current option
		switch m.cursor {
		case 0: // Retry
			return true, nil, func() tea.Msg {
				return ConnectionFailedRetryMsg{}
			}
		case 1: // Try Different Method
			return true, nil, func() tea.Msg {
				return ConnectionFailedTryMethodMsg{}
			}
		case 2: // Switch Server
			return true, nil, func() tea.Msg {
				return ConnectionFailedSwitchServerMsg{}
			}
		case 3: // Quit
			return true, nil, tea.Quit
		}
		return true, m, nil

	default:
		// Consume all other keys
		return true, m, nil
	}
}

// Render returns the modal content
func (m *ConnectionFailedModal) Render(width, height int) string {
	primaryColor := lipgloss.Color("#FF6B6B") // Red for error

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(primaryColor).
		MarginBottom(1).
		Align(lipgloss.Center)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF6B6B")).
		MarginBottom(1)

	serverStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true).
		MarginBottom(1)

	optionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	selectedStyle := lipgloss.NewStyle().
		Foreground(primaryColor).
		Bold(true)

	keyHintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	// Build content
	var content string

	// Title (different for server disconnect vs connection error)
	if m.serverDisconnected {
		content += titleStyle.Render("Server Disconnected") + "\n\n"
	} else {
		content += titleStyle.Render("⚠ Connection Failed") + "\n\n"
	}

	// Server address
	content += serverStyle.Render("Server: "+m.serverAddr) + "\n"

	// Error or disconnect reason
	if m.serverDisconnected {
		content += errorStyle.Render("Reason: "+m.disconnectReason) + "\n\n"
	} else {
		content += errorStyle.Render("Error: "+m.errorMessage) + "\n\n"
	}

	// Options
	content += "What would you like to do?\n\n"

	// Option 0: Retry
	if m.cursor == 0 {
		content += selectedStyle.Render("→ Retry connection") + " " + keyHintStyle.Render("[R]") + "\n"
	} else {
		content += optionStyle.Render("  Retry connection") + " " + keyHintStyle.Render("[R]") + "\n"
	}

	// Option 1: Try Different Method
	if m.cursor == 1 {
		content += selectedStyle.Render("→ Try different connection method") + " " + keyHintStyle.Render("[M]") + "\n"
	} else {
		content += optionStyle.Render("  Try different connection method") + " " + keyHintStyle.Render("[M]") + "\n"
	}

	// Option 2: Switch Server
	if m.cursor == 2 {
		content += selectedStyle.Render("→ Switch to different server") + " " + keyHintStyle.Render("[S]") + "\n"
	} else {
		content += optionStyle.Render("  Switch to different server") + " " + keyHintStyle.Render("[S]") + "\n"
	}

	// Option 3: Quit
	if m.cursor == 3 {
		content += selectedStyle.Render("→ Quit") + " " + keyHintStyle.Render("[Q]") + "\n"
	} else {
		content += optionStyle.Render("  Quit") + " " + keyHintStyle.Render("[Q]") + "\n"
	}

	// Navigation hint
	content += "\n"
	content += keyHintStyle.Render("[↑/↓] Navigate  [Enter] Select  [Esc] Dismiss  [Q] Quit")

	// Create border style
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primaryColor).
		Padding(1, 2)

	// Calculate modal size (fixed width, auto height)
	modalWidth := 60
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

// IsBlockingInput returns whether this modal blocks input to the main view.
// Returns false so users can browse cached data while disconnected.
func (m *ConnectionFailedModal) IsBlockingInput() bool {
	return false
}
