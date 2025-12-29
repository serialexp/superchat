// ABOUTME: CommandExecutor implementation for terminal client
// ABOUTME: Implements shared command system interface
package ui

import (
	"github.com/aeolun/superchat/pkg/client/commands"
	"github.com/aeolun/superchat/pkg/client/ui/modal"
	"github.com/aeolun/superchat/pkg/protocol"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// Verify Model implements CommandExecutor interface
var _ commands.CommandExecutor = (*Model)(nil)

// === State Queries (CommandExecutor interface) ===

func (m Model) GetCurrentView() commands.ViewID {
	return commands.ViewIDFromInt(int(m.currentView))
}

func (m Model) GetActiveModal() commands.ModalType {
	topModal := m.modalStack.TopType()

	// Map terminal modal types to shared modal types
	switch topModal {
	case modal.ModalCompose:
		return commands.ModalCompose
	case modal.ModalHelp:
		return commands.ModalHelp
	case modal.ModalServerSelector:
		return commands.ModalServerSelector
	case modal.ModalAdminPanel:
		return commands.ModalAdminPanel
	case modal.ModalCreateChannel:
		return commands.ModalCreateChannel
	case modal.ModalNicknameSetup:
		return commands.ModalNicknameSetup
	case modal.ModalPasswordAuth:
		return commands.ModalPasswordAuth
	case modal.ModalRegistration:
		return commands.ModalRegistration
	case modal.ModalDeleteConfirm:
		return commands.ModalDeleteConfirm
	default:
		return commands.ModalNone
	}
}

func (m Model) HasSelectedMessage() bool {
	return m.currentThread != nil
}

func (m Model) HasSelectedChannel() bool {
	return m.currentChannel != nil
}

func (m Model) HasSelectedThread() bool {
	return len(m.threads) > 0 && m.threadCursor >= 0 && m.threadCursor < len(m.threads)
}

func (m Model) GetSelectedMessageIndex() int {
	if m.currentView == ViewThreadView && m.currentThread != nil {
		return m.replyCursor
	}
	return -1
}

func (m Model) IsComposing() bool {
	return m.modalStack.TopType() == modal.ModalCompose
}

func (m Model) HasComposeContent() bool {
	// Content checking is handled internally by the compose modal
	// This is for interface compatibility only
	return m.IsComposing()
}

func (m Model) IsAdmin() bool {
	return m.userFlags&1 != 0 // Admin flag is bit 0
}

func (m Model) IsRegisteredUser() bool {
	return m.userID != nil
}

func (m Model) IsConnected() bool {
	return m.connectionState == StateConnected
}

func (m Model) HasThreads() bool {
	return len(m.threads) > 0
}

func (m Model) HasChannels() bool {
	return len(m.channels) > 0
}

func (m Model) CanGoBack() bool {
	// Can go back if not in channel list view
	return m.currentView != ViewChannelList
}

// === Action Execution ===

// ExecuteAction performs the given action
// This is called by the shared command system
func (m Model) ExecuteAction(actionID string) error {
	// Note: This returns error to satisfy the interface, but we actually
	// need to return (Model, tea.Cmd) for the terminal client.
	// The actual execution happens in ExecuteActionWithReturn below.
	// This method is here for interface compatibility.
	return nil
}

// ExecuteActionWithReturn executes an action and returns updated model and command
// This is the actual method used by the terminal client
func (m Model) ExecuteActionWithReturn(actionID string) (Model, tea.Cmd) {
	switch actionID {
	// === Global Actions ===
	case commands.ActionHelp:
		return m.toggleHelp()

	case commands.ActionQuit:
		return m, m.saveAndQuit()

	case commands.ActionServerList:
		return m.openServerSelector()

	// === Navigation Actions ===
	case commands.ActionNavigateUp:
		return m.navigateUp()

	case commands.ActionNavigateDown:
		return m.navigateDown()

	case commands.ActionSelect:
		return m.selectItem()

	case commands.ActionGoBack:
		return m.goBack()

	// === Messaging Actions ===
	case commands.ActionComposeNewThread:
		return m.openNewThreadCompose()

	case commands.ActionComposeReply:
		return m.openReplyCompose()

	case commands.ActionEditMessage:
		return m.openEditMessage()

	case commands.ActionDeleteMessage:
		return m.deleteMessage()

	// Note: ActionSendMessage is NOT handled here - it's handled internally
	// by the compose modal via its onSend callback

	// === Admin Actions ===
	case commands.ActionAdminPanel:
		return m.openAdminPanel()

	case commands.ActionCreateChannel:
		return m.openCreateChannel()

	default:
		// Unknown action - just return unchanged
		return m, nil
	}
}

// === Helper methods that map to existing functionality ===

func (m Model) toggleHelp() (Model, tea.Cmd) {
	// Generate help content from shared commands
	helpContent := commands.GenerateHelpContent(&m)
	helpModal := modal.NewHelpModal(helpContent)
	m.modalStack.Push(helpModal)
	return m, nil
}

func (m Model) openServerSelector() (Model, tea.Cmd) {
	// Open server selector modal with empty local servers (discovery not active)
	serverModal := modal.NewServerSelectorModal(m.availableServers, []protocol.ServerInfo{}, false)
	m.modalStack.Push(serverModal)
	return m, nil
}

func (m Model) navigateUp() (Model, tea.Cmd) {
	switch m.currentView {
	case ViewChannelList:
		if m.channelCursor > 0 {
			m.channelCursor--
		}
	case ViewThreadList:
		if m.threadCursor > 0 {
			m.threadCursor--
			// Content will be updated in View()
		}
	case ViewThreadView:
		if m.replyCursor > 0 {
			m.replyCursor--
			// Content will be updated in View()
		}
	}
	return m, nil
}

func (m Model) navigateDown() (Model, tea.Cmd) {
	switch m.currentView {
	case ViewChannelList:
		maxIndex := m.getVisibleChannelListItemCount() - 1
		if m.channelCursor < maxIndex {
			m.channelCursor++
		}
	case ViewThreadList:
		if m.threadCursor < len(m.threads)-1 {
			m.threadCursor++
			// Content will be updated in View()
		}
	case ViewThreadView:
		maxCursor := len(m.threadReplies) // 0 = root, 1+ = replies
		if m.replyCursor < maxCursor {
			m.replyCursor++
			// Content will be updated in View()
		}
	}
	return m, nil
}

func (m Model) selectItem() (Model, tea.Cmd) {
	switch m.currentView {
	case ViewChannelList:
		return m.selectCurrentChannel()
	case ViewThreadList:
		return m.selectCurrentThread()
	}
	return m, nil
}

func (m Model) goBack() (Model, tea.Cmd) {
	// If modal is open, close it
	if m.modalStack.Top() != nil {
		m.modalStack.Pop()
		return m, nil
	}

	// Otherwise navigate back in views
	switch m.currentView {
	case ViewThreadView:
		m.currentView = ViewThreadList
		m.currentThread = nil
		m.threadReplies = nil
		m.replyCursor = 0
	case ViewThreadList, ViewChatChannel:
		m.currentView = ViewChannelList
		m.currentChannel = nil
		// Leave channel
		if m.hasActiveChannel {
			return m, m.sendLeaveChannel(m.activeChannelID, false)
		}
	}
	return m, nil
}

func (m Model) openNewThreadCompose() (Model, tea.Cmd) {
	if m.currentChannel == nil {
		return m, nil
	}
	// Set compose state for new thread
	m.composeParentID = nil
	m.composeMessageID = nil
	m.showComposeModal(modal.ComposeModeNewThread, "")
	return m, nil
}

func (m Model) openReplyCompose() (Model, tea.Cmd) {
	msg, ok := m.selectedMessage()
	if !ok {
		return m, nil
	}
	// Set compose state for reply
	m.composeParentID = &msg.ID
	m.composeMessageID = nil
	m.showComposeModal(modal.ComposeModeReply, "")
	return m, nil
}

func (m Model) openEditMessage() (Model, tea.Cmd) {
	msg, ok := m.selectedMessage()
	if !ok {
		return m, nil
	}
	// TODO: Check if message is owned by current user
	// Set compose state for edit
	m.composeParentID = nil
	m.composeMessageID = &msg.ID
	m.showComposeModal(modal.ComposeModeEdit, msg.Content)
	return m, nil
}

func (m Model) deleteMessage() (Model, tea.Cmd) {
	msg, ok := m.selectedMessage()
	if !ok {
		return m, nil
	}
	// TODO: Check if message is owned by current user
	deleteModal := modal.NewDeleteConfirmModal(
		msg.ID,
		func(messageID uint64) tea.Cmd {
			return m.sendDeleteMessage(messageID)
		},
		func() tea.Cmd {
			return nil
		},
	)
	m.modalStack.Push(deleteModal)
	return m, nil
}

func (m Model) openAdminPanel() (Model, tea.Cmd) {
	if !m.IsAdmin() {
		return m, nil
	}
	// Use existing method that has all the complex wiring
	m.showAdminPanel()
	return m, nil
}

func (m Model) openCreateChannel() (Model, tea.Cmd) {
	if !m.IsAdmin() {
		return m, nil
	}
	// Use existing method that has all the callback wiring
	m.showCreateChannelModal()
	return m, nil
}

// === Existing helper methods that we're calling ===

func (m Model) selectCurrentChannel() (Model, tea.Cmd) {
	item := m.getChannelListItemAtCursor()
	if item == nil {
		return m, nil
	}

	switch item.Type {
	case ChannelListItemDM:
		// DM channel selected - join it as a chat channel
		dm := item.DM
		m.currentChannel = &protocol.Channel{
			ID:   dm.ChannelID,
			Name: dm.OtherNickname,
			Type: 0, // Chat type
		}
		m.currentView = ViewChatChannel
		m.loadingChat = true
		m.allChatLoaded = false
		m.chatMessages = nil
		m.chatTextarea.Reset()
		m.chatTextarea.Focus()
		return m, tea.Batch(
			m.sendJoinChannel(dm.ChannelID),
			m.requestChatMessages(dm.ChannelID),
			m.sendSubscribeChannel(dm.ChannelID),
			textarea.Blink,
		)

	case ChannelListItemPendingDM:
		// Pending DM invite selected - show the DM request modal
		invite := item.PendingDM
		dmModal := modal.NewDMRequestModal(modal.DMRequestModalConfig{
			ChannelID:        invite.ChannelID,
			FromNickname:     invite.FromNickname,
			FromUserID:       invite.FromUserID,
			EncryptionStatus: invite.EncryptionStatus,
			OnAccept: func(channelID uint64, allowUnencrypted bool) tea.Cmd {
				return m.sendAllowUnencrypted(channelID, false)
			},
			OnDecline: func(channelID uint64) tea.Cmd {
				return func() tea.Msg {
					return DMDeclinedMsg{ChannelID: channelID}
				}
			},
			OnSetupEncryption: func(channelID uint64) tea.Cmd {
				return nil
			},
		})
		m.modalStack.Push(dmModal)
		return m, nil

	case ChannelListItemChannel:
		ch := item.Channel
		m.currentChannel = ch

		// Join and subscribe to channel
		var cmds []tea.Cmd
		cmds = append(cmds, m.sendJoinChannel(ch.ID))
		cmds = append(cmds, m.sendSubscribeChannel(ch.ID))

		// Request appropriate data based on channel type
		if ch.Type == 0 {
			// Chat channel
			m.currentView = ViewChatChannel
			m.loadingChat = true
			m.chatMessages = nil
			m.chatTextarea.Reset()
			m.chatTextarea.Focus()
			cmds = append(cmds, m.requestChatMessages(ch.ID))
			cmds = append(cmds, textarea.Blink)
		} else {
			// Forum channel
			m.currentView = ViewThreadList
			m.loadingThreadList = true
			m.threads = nil
			cmds = append(cmds, m.requestThreadList(ch.ID))
		}

		return m, tea.Batch(cmds...)

	case ChannelListItemSubchannel:
		// Subchannel selected
		sub := item.Subchannel
		m.currentChannel = &protocol.Channel{
			ID:   sub.ID,
			Name: sub.Name,
			Type: sub.Type,
		}

		var cmds []tea.Cmd
		cmds = append(cmds, m.sendJoinChannel(sub.ID))
		cmds = append(cmds, m.sendSubscribeChannel(sub.ID))

		if sub.Type == 0 {
			m.currentView = ViewChatChannel
			m.loadingChat = true
			m.chatMessages = nil
			m.chatTextarea.Reset()
			m.chatTextarea.Focus()
			cmds = append(cmds, m.requestChatMessages(sub.ID))
			cmds = append(cmds, textarea.Blink)
		} else {
			m.currentView = ViewThreadList
			m.loadingThreadList = true
			m.threads = nil
			cmds = append(cmds, m.requestThreadList(sub.ID))
		}

		return m, tea.Batch(cmds...)
	}

	return m, nil
}

func (m Model) selectCurrentThread() (Model, tea.Cmd) {
	if m.threadCursor >= 0 && m.threadCursor < len(m.threads) {
		thread := m.threads[m.threadCursor]
		m.currentThread = &thread
		m.currentView = ViewThreadView
		m.replyCursor = 0

		return m, m.requestThreadReplies(thread.ID)
	}
	return m, nil
}
