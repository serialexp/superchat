package modal

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/aeolun/superchat/pkg/protocol"
)

// DMRequestModal shows an incoming DM request for the user to accept or decline
type DMRequestModal struct {
	channelID         uint64
	fromNickname      string
	fromUserID        *uint64
	encryptionStatus  uint8 // 0=not possible, 1=required, 2=optional
	onAccept          func(channelID uint64, allowUnencrypted bool) tea.Cmd
	onDecline         func(channelID uint64) tea.Cmd
	onSetupEncryption func(channelID uint64) tea.Cmd
	cursor            int // 0=Accept, 1=Decline, 2=Setup Encryption (if available)
}

// DMRequestModalConfig holds configuration for creating a DM request modal
type DMRequestModalConfig struct {
	ChannelID         uint64
	FromNickname      string
	FromUserID        *uint64
	EncryptionStatus  uint8
	OnAccept          func(channelID uint64, allowUnencrypted bool) tea.Cmd
	OnDecline         func(channelID uint64) tea.Cmd
	OnSetupEncryption func(channelID uint64) tea.Cmd
}

// NewDMRequestModal creates a new DM request modal
func NewDMRequestModal(config DMRequestModalConfig) *DMRequestModal {
	return &DMRequestModal{
		channelID:         config.ChannelID,
		fromNickname:      config.FromNickname,
		fromUserID:        config.FromUserID,
		encryptionStatus:  config.EncryptionStatus,
		onAccept:          config.OnAccept,
		onDecline:         config.OnDecline,
		onSetupEncryption: config.OnSetupEncryption,
		cursor:            0,
	}
}

// Type returns the modal type
func (m *DMRequestModal) Type() ModalType {
	return ModalDMRequest
}

// HandleKey processes keyboard input
func (m *DMRequestModal) HandleKey(msg tea.KeyMsg) (bool, Modal, tea.Cmd) {
	numOptions := m.numOptions()

	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return true, m, nil

	case "down", "j":
		if m.cursor < numOptions-1 {
			m.cursor++
		}
		return true, m, nil

	case "enter":
		return m.handleSelection()

	case "y":
		// Quick accept (unencrypted)
		// Only allowed if encryption is not required
		if m.encryptionStatus != protocol.DMEncryptionRequired && m.onAccept != nil {
			return true, nil, m.onAccept(m.channelID, true)
		}
		return true, m, nil

	case "n", "esc":
		// Decline
		var cmd tea.Cmd
		if m.onDecline != nil {
			cmd = m.onDecline(m.channelID)
		}
		return true, nil, cmd

	case "e":
		// Quick setup encryption (only for Required or Optional)
		if m.encryptionStatus != protocol.DMEncryptionNotPossible && m.onSetupEncryption != nil {
			return true, nil, m.onSetupEncryption(m.channelID)
		}
		return true, m, nil

	default:
		return true, m, nil
	}
}

func (m *DMRequestModal) numOptions() int {
	options := 2 // Accept, Decline
	// Show "Setup Encryption" option only if encryption is possible (Required or Optional)
	if m.encryptionStatus != protocol.DMEncryptionNotPossible {
		options++
	}
	return options
}

func (m *DMRequestModal) handleSelection() (bool, Modal, tea.Cmd) {
	switch m.cursor {
	case 0: // Accept
		if m.onAccept != nil {
			// Determine if we're accepting as unencrypted
			// For NotPossible: always unencrypted
			// For Required: should not reach here without setting up encryption first
			// For Optional: accepting without setup means unencrypted
			allowUnencrypted := m.encryptionStatus != protocol.DMEncryptionRequired
			return true, nil, m.onAccept(m.channelID, allowUnencrypted)
		}
		return true, nil, nil

	case 1: // Decline
		if m.onDecline != nil {
			return true, nil, m.onDecline(m.channelID)
		}
		return true, nil, nil

	case 2: // Setup Encryption (if available)
		if m.encryptionStatus != protocol.DMEncryptionNotPossible && m.onSetupEncryption != nil {
			return true, nil, m.onSetupEncryption(m.channelID)
		}
		return true, m, nil

	default:
		return true, m, nil
	}
}

// Render returns the modal content
func (m *DMRequestModal) Render(width, height int) string {
	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 2).
		Width(50)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205"))

	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205"))

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	// Build title
	title := titleStyle.Render("DM Request")

	// Build description
	var desc string
	if m.fromUserID != nil {
		desc = fmt.Sprintf("%s wants to start a direct message.", m.fromNickname)
	} else {
		desc = fmt.Sprintf("~%s wants to start a direct message.", m.fromNickname)
	}

	// Add encryption status note
	var encryptionNote string
	switch m.encryptionStatus {
	case protocol.DMEncryptionNotPossible:
		encryptionNote = mutedStyle.Render("\nThis will be an unencrypted conversation.\n(The other user has not set up encryption.)")
	case protocol.DMEncryptionRequired:
		encryptionNote = mutedStyle.Render("\nEncryption required. You need to set up\nan encryption key to chat with this user.")
	case protocol.DMEncryptionOptional:
		encryptionNote = mutedStyle.Render("\nEncryption available. You can set up encryption\nfor a secure conversation, or chat unencrypted.")
	}

	// Build options
	options := []string{"Accept", "Decline"}
	if m.encryptionStatus != protocol.DMEncryptionNotPossible {
		options = append(options, "Setup Encryption")
	}

	var optionLines string
	for i, opt := range options {
		prefix := "  "
		style := normalStyle
		if i == m.cursor {
			prefix = "> "
			style = selectedStyle
		}
		optionLines += prefix + style.Render(opt) + "\n"
	}

	// Build help text
	help := mutedStyle.Render("\n[↑/↓] Navigate  [Enter] Select  [Esc] Decline")

	content := title + "\n\n" + desc + encryptionNote + "\n\n" + optionLines + help

	modal := modalStyle.Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

// IsBlockingInput returns true (this modal blocks all input)
func (m *DMRequestModal) IsBlockingInput() bool {
	return true
}

// DMRequestAcceptedMsg is sent when a DM request is accepted
type DMRequestAcceptedMsg struct {
	ChannelID        uint64
	AllowUnencrypted bool
}

// DMRequestDeclinedMsg is sent when a DM request is declined
type DMRequestDeclinedMsg struct {
	ChannelID uint64
}
