package modal

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// EncryptionSetupModal guides the user through setting up encryption for DMs
type EncryptionSetupModal struct {
	dmChannelID    *uint64 // nil if setting up key for general use
	reason         string
	hasSSHKey      bool   // True if user has SSH key we can derive from
	hasExistingKey bool   // True if user already has a generated key
	cursor         int    // 0=Generate, 1=Use SSH (if available), 2=Skip (if allowed)
	canSkip        bool   // True if unencrypted is allowed
	onGenerate     func() tea.Cmd
	onUseSSH       func() tea.Cmd
	onSkip         func() tea.Cmd
	onCancel       func() tea.Cmd
}

// EncryptionSetupConfig holds configuration for creating an encryption setup modal
type EncryptionSetupConfig struct {
	DMChannelID    *uint64
	Reason         string
	HasSSHKey      bool
	HasExistingKey bool
	CanSkip        bool
	OnGenerate     func() tea.Cmd
	OnUseSSH       func() tea.Cmd
	OnSkip         func() tea.Cmd
	OnCancel       func() tea.Cmd
}

// NewEncryptionSetupModal creates a new encryption setup modal
func NewEncryptionSetupModal(config EncryptionSetupConfig) *EncryptionSetupModal {
	return &EncryptionSetupModal{
		dmChannelID:    config.DMChannelID,
		reason:         config.Reason,
		hasSSHKey:      config.HasSSHKey,
		hasExistingKey: config.HasExistingKey,
		canSkip:        config.CanSkip,
		onGenerate:     config.OnGenerate,
		onUseSSH:       config.OnUseSSH,
		onSkip:         config.OnSkip,
		onCancel:       config.OnCancel,
		cursor:         0,
	}
}

// Type returns the modal type
func (m *EncryptionSetupModal) Type() ModalType {
	return ModalEncryptionSetup
}

// HandleKey processes keyboard input
func (m *EncryptionSetupModal) HandleKey(msg tea.KeyMsg) (bool, Modal, tea.Cmd) {
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

	case "esc":
		var cmd tea.Cmd
		if m.onCancel != nil {
			cmd = m.onCancel()
		}
		return true, nil, cmd

	default:
		return true, m, nil
	}
}

func (m *EncryptionSetupModal) numOptions() int {
	options := 1 // Generate new key
	if m.hasSSHKey {
		options++ // Use SSH key
	}
	if m.canSkip {
		options++ // Skip (unencrypted)
	}
	return options
}

func (m *EncryptionSetupModal) getOptionIndex(option string) int {
	idx := 0
	switch option {
	case "generate":
		return 0
	case "ssh":
		if m.hasSSHKey {
			return 1
		}
		return -1
	case "skip":
		if m.hasSSHKey {
			idx = 2
		} else {
			idx = 1
		}
		if m.canSkip {
			return idx
		}
		return -1
	}
	return -1
}

func (m *EncryptionSetupModal) handleSelection() (bool, Modal, tea.Cmd) {
	// Map cursor to action
	action := m.cursorToAction()

	switch action {
	case "generate":
		if m.onGenerate != nil {
			return true, nil, m.onGenerate()
		}
	case "ssh":
		if m.onUseSSH != nil {
			return true, nil, m.onUseSSH()
		}
	case "skip":
		if m.onSkip != nil {
			return true, nil, m.onSkip()
		}
	}

	return true, nil, nil
}

func (m *EncryptionSetupModal) cursorToAction() string {
	cursor := m.cursor
	if cursor == 0 {
		return "generate"
	}
	cursor--

	if m.hasSSHKey {
		if cursor == 0 {
			return "ssh"
		}
		cursor--
	}

	if m.canSkip && cursor == 0 {
		return "skip"
	}

	return ""
}

// Render returns the modal content
func (m *EncryptionSetupModal) Render(width, height int) string {
	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 2).
		Width(56)

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
	title := titleStyle.Render("Encryption Setup")

	// Build description
	var desc string
	if m.reason != "" {
		desc = m.reason
	} else {
		desc = "Set up end-to-end encryption for private messages."
	}

	desc += "\n\n"
	desc += mutedStyle.Render("Your encryption key will be stored locally.\nMessages are encrypted before leaving your device.")

	// Build options
	var options []struct {
		label string
		desc  string
	}

	if m.hasExistingKey {
		options = append(options, struct {
			label string
			desc  string
		}{"Use Existing Key", "Use your previously generated key"})
	} else {
		options = append(options, struct {
			label string
			desc  string
		}{"Generate New Key", "Create a new encryption key"})
	}

	if m.hasSSHKey {
		options = append(options, struct {
			label string
			desc  string
		}{"Derive from SSH Key", "Use your SSH key for encryption"})
	}

	if m.canSkip {
		options = append(options, struct {
			label string
			desc  string
		}{"Skip (Unencrypted)", "Continue without encryption"})
	}

	var optionLines string
	for i, opt := range options {
		prefix := "  "
		style := normalStyle
		if i == m.cursor {
			prefix = "> "
			style = selectedStyle
		}
		optionLines += prefix + style.Render(opt.label) + "\n"
		optionLines += "    " + mutedStyle.Render(opt.desc) + "\n"
	}

	// Build help text
	help := mutedStyle.Render("\n[↑/↓] Navigate  [Enter] Select  [Esc] Cancel")

	content := title + "\n\n" + desc + "\n\n" + optionLines + help

	modal := modalStyle.Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

// IsBlockingInput returns true (this modal blocks all input)
func (m *EncryptionSetupModal) IsBlockingInput() bool {
	return true
}

// EncryptionKeyGeneratedMsg is sent when a new encryption key is generated
type EncryptionKeyGeneratedMsg struct {
	PublicKey [32]byte
}

// EncryptionKeyFromSSHMsg is sent when encryption key is derived from SSH
type EncryptionKeyFromSSHMsg struct {
	PublicKey [32]byte
}
