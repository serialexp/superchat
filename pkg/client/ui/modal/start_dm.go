package modal

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// OnlineUser represents a user available for DM
type OnlineUser struct {
	SessionID    uint64
	Nickname     string
	IsRegistered bool
	UserID       *uint64
}

// StartDMModal allows selecting an online user to start a DM with
type StartDMModal struct {
	users           []OnlineUser
	filteredUsers   []OnlineUser
	selectedIndex   int
	searchQuery     string
	selfSessionID   *uint64
	onSelectUser    func(userID *uint64, nickname string) tea.Cmd
}

// NewStartDMModal creates a new start DM modal
func NewStartDMModal(users []OnlineUser, selfSessionID *uint64, onSelectUser func(userID *uint64, nickname string) tea.Cmd) *StartDMModal {
	m := &StartDMModal{
		users:         users,
		selectedIndex: 0,
		searchQuery:   "",
		selfSessionID: selfSessionID,
		onSelectUser:  onSelectUser,
	}
	m.filterUsers()
	return m
}

// Type returns the modal type
func (m *StartDMModal) Type() ModalType {
	return ModalStartDM
}

// filterUsers updates the filtered user list based on search query
func (m *StartDMModal) filterUsers() {
	m.filteredUsers = make([]OnlineUser, 0)
	query := strings.ToLower(m.searchQuery)

	for _, user := range m.users {
		// Exclude self
		if m.selfSessionID != nil && user.SessionID == *m.selfSessionID {
			continue
		}

		// Filter by search query
		if query != "" && !strings.Contains(strings.ToLower(user.Nickname), query) {
			continue
		}

		m.filteredUsers = append(m.filteredUsers, user)
	}

	// Reset selection if out of bounds
	if m.selectedIndex >= len(m.filteredUsers) {
		m.selectedIndex = max(0, len(m.filteredUsers)-1)
	}
}

// HandleKey processes keyboard input
func (m *StartDMModal) HandleKey(msg tea.KeyMsg) (bool, Modal, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		return true, nil, nil

	case "up", "ctrl+p":
		if m.selectedIndex > 0 {
			m.selectedIndex--
		}
		return true, m, nil

	case "down", "ctrl+n":
		if m.selectedIndex < len(m.filteredUsers)-1 {
			m.selectedIndex++
		}
		return true, m, nil

	case "enter":
		if len(m.filteredUsers) > 0 && m.onSelectUser != nil {
			user := m.filteredUsers[m.selectedIndex]
			return true, nil, m.onSelectUser(user.UserID, user.Nickname)
		}
		return true, m, nil

	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.filterUsers()
		}
		return true, m, nil

	default:
		// Handle text input for search
		if msg.Type == tea.KeyRunes {
			m.searchQuery += string(msg.Runes)
			m.filterUsers()
			return true, m, nil
		}
		return true, m, nil
	}
}

// Render returns the modal content
func (m *StartDMModal) Render(width, height int) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		MarginBottom(1)

	searchStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("170")).
		Padding(0, 1).
		Width(46)

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true)

	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205"))

	registeredStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("46"))

	anonStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("208")).
		Italic(true)

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 2).
		Width(54).
		Height(min(height-4, 20))

	// Title
	title := titleStyle.Render("Start Direct Message")

	// Search field
	var searchDisplay string
	if m.searchQuery == "" {
		searchDisplay = "█" + hintStyle.Render(" Type to search...")
	} else {
		searchDisplay = m.searchQuery + "█"
	}
	searchField := searchStyle.Render(searchDisplay)

	// User list
	var userLines []string
	if len(m.filteredUsers) == 0 {
		if len(m.users) <= 1 {
			userLines = append(userLines, hintStyle.Render("No other users online"))
		} else {
			userLines = append(userLines, hintStyle.Render("No users match your search"))
		}
	} else {
		// Show up to 10 users, centered around selection
		maxVisible := 10
		start := 0
		if len(m.filteredUsers) > maxVisible {
			start = m.selectedIndex - maxVisible/2
			if start < 0 {
				start = 0
			}
			if start+maxVisible > len(m.filteredUsers) {
				start = len(m.filteredUsers) - maxVisible
			}
		}
		end := min(start+maxVisible, len(m.filteredUsers))

		for i := start; i < end; i++ {
			user := m.filteredUsers[i]

			// Format user entry
			var prefix string
			var style lipgloss.Style
			if i == m.selectedIndex {
				prefix = "> "
				style = selectedStyle
			} else {
				prefix = "  "
				if user.IsRegistered {
					style = registeredStyle
				} else {
					style = anonStyle
				}
			}

			// Add registration indicator
			var suffix string
			if !user.IsRegistered {
				suffix = " (anon)"
			}

			line := prefix + style.Render(user.Nickname+suffix)
			userLines = append(userLines, line)
		}

		// Show scroll indicators if needed
		if start > 0 {
			userLines = append([]string{hintStyle.Render("  ↑ more users above")}, userLines...)
		}
		if end < len(m.filteredUsers) {
			userLines = append(userLines, hintStyle.Render("  ↓ more users below"))
		}
	}

	// Help text
	help := hintStyle.Render("[↑/↓] Navigate  [Enter] Start DM  [Esc] Cancel")

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		searchField,
		"",
		lipgloss.JoinVertical(lipgloss.Left, userLines...),
		"",
		help,
	)

	modal := modalStyle.Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

// IsBlockingInput returns true (this modal blocks all input)
func (m *StartDMModal) IsBlockingInput() bool {
	return true
}
