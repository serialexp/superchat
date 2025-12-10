package modal

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CreateSubchannelModal allows users to create a new subchannel within a channel
type CreateSubchannelModal struct {
	parentChannelID   uint64
	parentChannelName string
	nameInput         string
	descriptionInput  string
	channelType       uint8 // 0 = chat, 1 = forum (default)
	focusedField      int   // 0 = name, 1 = description, 2 = type
	errorMessage      string
	onConfirm         func(parentID uint64, name, description string, channelType uint8) tea.Cmd
	onCancel          func() tea.Cmd
}

// NewCreateSubchannelModal creates a new subchannel creation modal
func NewCreateSubchannelModal(parentChannelID uint64, parentChannelName string, onConfirm func(uint64, string, string, uint8) tea.Cmd, onCancel func() tea.Cmd) *CreateSubchannelModal {
	return &CreateSubchannelModal{
		parentChannelID:   parentChannelID,
		parentChannelName: parentChannelName,
		nameInput:         "",
		descriptionInput:  "",
		channelType:       1, // Default to forum
		focusedField:      0,
		errorMessage:      "",
		onConfirm:         onConfirm,
		onCancel:          onCancel,
	}
}

// Type returns the modal type
func (m *CreateSubchannelModal) Type() ModalType {
	return ModalCreateSubchannel
}

// HandleKey processes keyboard input
func (m *CreateSubchannelModal) HandleKey(msg tea.KeyMsg) (bool, Modal, tea.Cmd) {
	switch msg.String() {
	case "tab":
		// Cycle through fields: name -> description -> type -> name
		m.focusedField = (m.focusedField + 1) % 3
		return true, m, nil

	case "shift+tab":
		// Cycle backwards
		m.focusedField = (m.focusedField - 1 + 3) % 3
		return true, m, nil

	case "enter":
		// Validate inputs
		if len(m.nameInput) < 3 {
			m.errorMessage = "Subchannel name must be at least 3 characters"
			return true, m, nil
		}
		if len(m.nameInput) > 30 {
			m.errorMessage = "Subchannel name must be at most 30 characters"
			return true, m, nil
		}

		// Validate name is URL-friendly (alphanumeric, hyphens, underscores)
		for _, c := range m.nameInput {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
				m.errorMessage = "Name can only contain letters, numbers, hyphens, and underscores"
				return true, m, nil
			}
		}

		// Submit subchannel creation
		var cmd tea.Cmd
		if m.onConfirm != nil {
			cmd = m.onConfirm(m.parentChannelID, m.nameInput, m.descriptionInput, m.channelType)
		}

		return true, nil, cmd // Close modal

	case "esc":
		// Cancel subchannel creation
		var cmd tea.Cmd
		if m.onCancel != nil {
			cmd = m.onCancel()
		}
		return true, nil, cmd // Close modal

	case "backspace":
		switch m.focusedField {
		case 0:
			if len(m.nameInput) > 0 {
				m.nameInput = m.nameInput[:len(m.nameInput)-1]
			}
		case 1:
			if len(m.descriptionInput) > 0 {
				m.descriptionInput = m.descriptionInput[:len(m.descriptionInput)-1]
			}
		case 2:
			// No backspace action for channel type (it's a toggle)
		}
		return true, m, nil

	case " ":
		// Explicitly handle space key
		switch m.focusedField {
		case 0:
			// Don't allow spaces in name (URL-friendly)
			m.errorMessage = "Spaces not allowed in name"
		case 1:
			m.descriptionInput += " "
		case 2:
			// Toggle channel type when focused on type field
			if m.channelType == 0 {
				m.channelType = 1
			} else {
				m.channelType = 0
			}
		}
		return true, m, nil

	default:
		// Handle text input
		if msg.Type == tea.KeyRunes {
			switch m.focusedField {
			case 0:
				m.nameInput += string(msg.Runes)
				m.errorMessage = "" // Clear error on input
			case 1:
				m.descriptionInput += string(msg.Runes)
			case 2:
				// No text input for channel type (it's a toggle field)
			}
			return true, m, nil
		}

		// Consume all other keys
		return true, m, nil
	}
}

// Render returns the modal content
func (m *CreateSubchannelModal) Render(width, height int) string {
	primaryColor := lipgloss.Color("#00D0D0")
	mutedColor := lipgloss.Color("240")
	errorColor := lipgloss.Color("196")

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(primaryColor).
		Align(lipgloss.Center).
		MarginBottom(1).
		Render("Create Subchannel")

	parentInfo := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Align(lipgloss.Center).
		MarginBottom(1).
		Render(fmt.Sprintf("In channel: #%s", m.parentChannelName))

	inputFocusedStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("170")).
		Padding(0, 1).
		Width(50)

	inputBlurredStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(50)

	// Name input field
	nameDisplay := m.nameInput
	if m.focusedField == 0 {
		nameDisplay += "█"
	}
	var nameStyle lipgloss.Style
	if m.focusedField == 0 {
		nameStyle = inputFocusedStyle
	} else {
		nameStyle = inputBlurredStyle
	}
	nameField := nameStyle.Render("Name: " + nameDisplay)

	// Description input field
	descriptionDisplay := m.descriptionInput
	if m.focusedField == 1 {
		descriptionDisplay += "█"
	}
	var descriptionStyle lipgloss.Style
	if m.focusedField == 1 {
		descriptionStyle = inputFocusedStyle
	} else {
		descriptionStyle = inputBlurredStyle
	}
	descriptionField := descriptionStyle.Render("Desc: " + descriptionDisplay)

	// Channel type selector
	var typeDisplay string
	if m.channelType == 0 {
		typeDisplay = "Chat (linear conversation)"
	} else {
		typeDisplay = "Forum (threaded discussion)"
	}
	if m.focusedField == 2 {
		typeDisplay += " █"
	}
	var typeStyle lipgloss.Style
	if m.focusedField == 2 {
		typeStyle = inputFocusedStyle
	} else {
		typeStyle = inputBlurredStyle
	}
	typeField := typeStyle.Render("Type: " + typeDisplay)

	// Error message if validation failed
	var errorMsg string
	if m.errorMessage != "" {
		errorMsg = "\n" + lipgloss.NewStyle().
			Foreground(errorColor).
			Align(lipgloss.Center).
			Render(m.errorMessage)
	}

	// Field descriptions
	fieldDescriptions := lipgloss.NewStyle().
		Foreground(mutedColor).
		Align(lipgloss.Left).
		MarginTop(1).
		Render(strings.Join([]string{
			"Name: URL-friendly (e.g., 'announcements')",
			"Desc: Optional description",
			"Type: [Space] to toggle between chat and forum",
		}, "\n"))

	// Status message
	statusMsg := lipgloss.NewStyle().
		Foreground(mutedColor).
		Align(lipgloss.Center).
		MarginTop(1).
		Render("[Tab] Next field  [Enter] Create  [ESC] Cancel")

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		"",
		title,
		parentInfo,
		nameField,
		descriptionField,
		typeField,
		errorMsg,
		fieldDescriptions,
		statusMsg,
		"",
	)

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primaryColor).
		Padding(1, 3).
		Width(60).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

// IsBlockingInput returns true (this modal blocks all input)
func (m *CreateSubchannelModal) IsBlockingInput() bool {
	return true
}
