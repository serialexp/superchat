package ui

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aeolun/superchat/pkg/client"
	"github.com/aeolun/superchat/pkg/client/auth"
	"github.com/aeolun/superchat/pkg/client/commands"
	"github.com/aeolun/superchat/pkg/client/crypto"
	"github.com/aeolun/superchat/pkg/client/ui/modal"
	"github.com/aeolun/superchat/pkg/protocol"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gen2brain/beeep"
)

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Initialize or resize thread view viewport
		if m.threadViewport.Width == 0 || m.threadViewport.Height == 0 {
			m.threadViewport = viewport.New(msg.Width-2, msg.Height-6)
			m.threadViewport.SetContent(m.buildThreadContent())
		} else {
			m.threadViewport.Width = msg.Width - 2
			m.threadViewport.Height = msg.Height - 6
		}

		// Initialize or resize thread list viewport
		threadListWidth := msg.Width - msg.Width/4 - 1 - 4 // Account for channel pane, space, and border+padding
		if threadListWidth < 30 {
			threadListWidth = 30
		}
		if m.threadListViewport.Width == 0 || m.threadListViewport.Height == 0 {
			m.threadListViewport = viewport.New(threadListWidth, msg.Height-6)
			m.threadListViewport.SetContent(m.buildThreadListContent())
		} else {
			m.threadListViewport.Width = threadListWidth
			m.threadListViewport.Height = msg.Height - 6
		}

		// Initialize or resize chat viewport (message area only, input is separate)
		chatHeight := msg.Height - 6 - 3 // Reserve 3 lines for input field
		if chatHeight < 5 {
			chatHeight = 5
		}
		if m.chatViewport.Width == 0 || m.chatViewport.Height == 0 {
			m.chatViewport = viewport.New(msg.Width-4, chatHeight)
			m.chatViewport.SetContent(m.buildChatMessages())
		} else {
			m.chatViewport.Width = msg.Width - 4
			m.chatViewport.Height = chatHeight
		}

		// Resize chat textarea
		m.chatTextarea.SetWidth(msg.Width - 4)

		// Initialize or resize splash viewport
		// Modal max height: 20 lines (fits 80x24 with margins)
		// If terminal is smaller, use most of available height
		modalHeight := 20
		if msg.Height < 20 {
			modalHeight = msg.Height - 2 // Leave 2 lines margin
			if modalHeight < 10 {
				modalHeight = 10
			}
		}
		// Content height = modal - 4 (border 2 + padding 2)
		// Viewport height = content - 2 (title 1 + prompt 1)
		contentHeight := modalHeight - 4
		viewportHeight := contentHeight - 2
		if viewportHeight < 8 {
			viewportHeight = 8
		}

		if m.splashViewport.Width == 0 || m.splashViewport.Height == 0 {
			m.splashViewport = viewport.New(58, viewportHeight)
			m.splashViewport.SetContent(m.buildSplashContent())
		} else {
			m.splashViewport.Width = 58
			m.splashViewport.Height = viewportHeight
			m.splashViewport.SetContent(m.buildSplashContent())
		}

		return m, nil

	case PostChatMessageMsg:
		// Handle deferred chat message posting (after registration warning dismissed)
		m.chatTextarea.Reset()
		return m.sendChatMessageWithContent(msg.Content)

	case ServerFrameMsg:
		return m.handleServerFrame(msg.Frame)

	case ErrorMsg:
		// Only show non-disconnect errors (disconnect is handled by DisconnectedMsg)
		if msg.Err.Error() != "disconnected from server" {
			m.errorMessage = msg.Err.Error()
		}
		return m, listenForServerFrames(m.conn, m.connGeneration)

	case ConnectedMsg:
		if m.logger != nil {
			m.logger.Printf("Received ConnectedMsg - handling connection success")
		}
		return m.handleReconnected()

	case DisconnectedMsg:
		if m.logger != nil {
			m.logger.Printf("DEBUG: Received DisconnectedMsg with generation %d (current generation: %d)", msg.Generation, m.connGeneration)
		}

		// Ignore disconnect messages from old connection generations
		if msg.Generation < m.connGeneration {
			if m.logger != nil {
				m.logger.Printf("Ignoring DisconnectedMsg from old connection generation %d (current: %d)", msg.Generation, m.connGeneration)
			}
			return m, nil
		}

		if m.logger != nil {
			m.logger.Printf("DEBUG: Processing DisconnectedMsg (not filtered)")
		}

		m.connectionState = StateDisconnected
		m.errorMessage = ""

		// Show connection failed modal to give user options
		// BUT: Don't show if there's already a connection-related modal active
		activeModalType := m.modalStack.TopType()
		if activeModalType != modal.ModalConnectionFailed && activeModalType != modal.ModalConnectionMethod {
			// Check if we have a server disconnect reason (from DISCONNECT message)
			if m.serverDisconnectReason != "" {
				m.modalStack.Push(modal.NewConnectionFailedModalWithReason(m.conn.GetAddress(), m.serverDisconnectReason))
				m.serverDisconnectReason = "" // Clear after use
			} else {
				m.modalStack.Push(modal.NewConnectionFailedModal(m.conn.GetAddress(), "Connection lost"))
			}
		}

		// Continue listening for frames from current connection
		return m, listenForServerFrames(m.conn, m.connGeneration)

	case ReconnectingMsg:
		m.connectionState = StateReconnecting
		m.reconnectAttempt = msg.Attempt
		m.errorMessage = ""
		return m, listenForServerFrames(m.conn, m.connGeneration)

	case PrivacyDelayTickMsg:
		if m.privacyDelayActive {
			remaining := time.Until(m.privacyDelayEnd)
			if remaining <= 0 {
				return m, func() tea.Msg { return PrivacyDelayCompleteMsg{} }
			}
			// Continue ticking every 100ms for smooth countdown
			return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
				return PrivacyDelayTickMsg{}
			})
		}
		return m, nil

	case PrivacyDelayCompleteMsg:
		if m.privacyDelayActive {
			m.privacyDelayActive = false

			// Set the new anonymous nickname
			m.nickname = m.privacyDelayNickname
			m.privacyDelayNickname = ""

			// Clear auth state
			m.userID = nil
			m.userFlags = 0
			m.authState = AuthStateAnonymous
			m.state.SetUserID(nil)
			m.state.SetLastNickname(m.nickname)

			// Re-enable auto-reconnect and connect
			m.conn.EnableAutoReconnect()
			if err := m.conn.Connect(); err != nil {
				m.errorMessage = fmt.Sprintf("Reconnect failed: %v", err)
				m.connectionState = StateDisconnected
				return m, nil
			}
			m.connectionState = StateReconnecting
			return m, listenForServerFrames(m.conn, m.connGeneration)
		}
		return m, nil

	case ServerListTimeoutMsg:
		// Server list request timed out
		if m.awaitingServerList {
			m.awaitingServerList = false
			// Update modal to show timeout error
			if activeModal := m.modalStack.Top(); activeModal != nil {
				if serverModal, ok := activeModal.(*modal.ServerSelectorModal); ok {
					serverModal.SetError("Directory server not responding (timeout after 5s)")
				}
			}
		}
		return m, nil

	case TickMsg:
		// Check if we need to send a ping (only if connected)
		if m.connectionState == StateConnected {
			now := time.Time(msg)
			if now.Sub(m.lastPingSent) >= m.pingInterval {
				m.lastPingSent = now
				return m, tea.Batch(tickCmd(), m.sendPing())
			}
		}
		return m, tickCmd()

	case VersionCheckMsg:
		m.latestVersion = msg.LatestVersion
		m.updateAvailable = msg.UpdateAvailable
		return m, nil

	case ForceRenderMsg:
		// No-op message just to trigger a re-render
		return m, nil

	case ClearStatusMsg:
		// Only clear if version matches (prevents stale timeouts from clearing new messages)
		if msg.Version == m.statusVersion {
			m.statusMessage = ""
		}
		return m, nil

	case InitTimeoutCheckMsg:
		// Check if initialization timeout has been reached
		if m.initStateMachine.OnTimeout() {
			// Timeout reached - assume TCP connection, send SET_NICKNAME
			if m.logger != nil {
				m.logger.Printf("[InitStateMachine] AUTH_RESPONSE timeout, assuming TCP connection")
			}
			if m.nickname != "" {
				return m, tea.Batch(
					m.sendSetNickname(),
					m.sendGetUserInfo(m.nickname),
				)
			}
			return m, nil
		}

		// Not timed out yet, check if we're still waiting
		if m.initStateMachine.State() == InitStateAwaitingAuth {
			// Still waiting for AUTH_RESPONSE, schedule another check with exponential backoff
			delay := m.initStateMachine.NextCheckDelay()
			if m.logger != nil {
				m.logger.Printf("[InitStateMachine] Next timeout check in %v", delay)
			}
			return m, checkInitTimeout(delay)
		}

		// Already transitioned (e.g., AUTH_RESPONSE arrived), stop checking
		return m, nil

	case NicknameSentMsg:
		// Store the nickname we sent so we can use it when server confirms
		m.pendingNickname = msg.Nickname
		return m, nil

	case GoAnonymousMsg:
		// User chose to browse anonymously instead of authenticating
		// The nickname is already set on the server - we just reset auth state
		// and continue using the same nickname (without authentication)

		// Reset auth state
		m.authState = AuthStateAnonymous
		m.authTargetNickname = ""
		m.authAttempts = 0
		m.authErrorMessage = ""
		m.authCooldownUntil = time.Time{}
		m.userID = nil
		m.userFlags = 0
		m.state.SetUserID(nil)

		// If we have a pending nickname (server accepted it but we haven't processed NICKNAME_RESPONSE yet),
		// update to it now so the UI shows the correct nickname
		if m.pendingNickname != "" {
			m.nickname = m.pendingNickname
			m.pendingNickname = ""
		}

		// Don't send SET_NICKNAME again - it's already set on the server
		return m, nil

	case modal.ServerSelectedMsg:
		// User selected a server to connect to
		return m.handleServerSelected(msg.Server)

	case modal.CustomServerInputMsg:
		// User entered a custom server address
		return m.handleCustomServerInput(msg.Address)

	case modal.ServerSelectorCancelledMsg:
		// User cancelled server selection
		// If we're in directory mode with no saved server (first run), exit the app
		// Otherwise (user pressed Ctrl+L to switch), just continue with current connection
		savedServer, _ := m.state.GetConfig("directory_selected_server")
		if m.directoryMode && savedServer == "" {
			// First run, user declined to choose a server - exit
			return m, m.saveAndQuit()
		}
		// User just closed the server selector, continue normally
		return m, nil

	case modal.ConnectionFailedRetryMsg:
		// User wants to retry connection
		m.switchingMethod = false // Clear flag when user takes action
		return m.handleConnectionRetry()

	case modal.ConnectionFailedTryMethodMsg:
		// User wants to try a different connection method
		m.switchingMethod = true // Set flag to prevent overlay during method selection
		return m.handleTryDifferentMethod()

	case modal.ConnectionFailedSwitchServerMsg:
		// User wants to switch to server selector
		m.switchingMethod = false // Clear flag when user takes action
		return m.handleSwitchToServerSelector()

	case modal.ConnectionMethodSelectedMsg:
		// User selected a connection method to try
		return m.handleConnectionMethodSelected(msg)

	case modal.ConnectionMethodCancelledMsg:
		// User cancelled method selection - go back to connection failed modal
		m.modalStack.Pop() // Remove ConnectionMethodModal
		m.modalStack.Push(modal.NewConnectionFailedModal(m.conn.GetAddress(), "Connection method selection cancelled"))
		return m, nil

	case ConnectionAttemptResultMsg:
		// Async connection attempt completed
		return m.handleConnectionAttemptResult(msg)

	case ExecuteCommandMsg:
		// Execute command from command palette
		cmd := m.commands.GetCommandByName(msg.CommandName, int(m.currentView), m.modalStack.TopType(), &m)
		if cmd != nil {
			updatedModel, teaCmd := cmd.Execute(&m)
			if model, ok := updatedModel.(*Model); ok {
				return *model, teaCmd
			}
		}
		return m, nil

	case modal.PushModalMsg:
		// Push the modal onto the stack (for proper modal overlay)
		m.modalStack.Push(msg.Modal)
		return m, msg.Cmd

	case EncryptionKeyGeneratedMsg:
		// Store the generated encryption keys (always do this, keys are valid)
		m.encryptionKeyPub = msg.PublicKey[:]
		m.encryptionKeyPriv = msg.PrivateKey[:]

		// Persist the key to disk
		m.persistEncryptionKey(msg.PrivateKey[:])

		// Only show success if the modal is still active (user didn't cancel)
		if !m.modalStack.HasType(modal.ModalEncryptionSetup) {
			// Modal was cancelled, just keep the keys silently
			return m, nil
		}

		// Close the encryption setup modal
		m.modalStack.RemoveByType(modal.ModalEncryptionSetup)
		return m, m.setStatus("Encryption key generated successfully")

	case DMKeyGeneratedStartMsg:
		// Store the generated encryption keys (always do this, keys are valid)
		m.encryptionKeyPub = msg.PublicKey[:]
		m.encryptionKeyPriv = msg.PrivateKey[:]

		// Persist the key to disk
		m.persistEncryptionKey(msg.PrivateKey[:])

		// Only start the DM if the encryption modal is still active
		// If user pressed ESC, the modal is gone and we should not proceed
		if !m.modalStack.HasType(modal.ModalEncryptionSetup) {
			// Modal was cancelled, just keep the keys but don't start DM
			return m, nil
		}

		// Close the encryption setup modal
		m.modalStack.RemoveByType(modal.ModalEncryptionSetup)
		// Now start the DM with encryption enabled
		m.statusMessage = fmt.Sprintf("Starting encrypted DM with %s...", msg.Nickname)
		return m, m.sendStartDM(msg.TargetType, msg.TargetUserID, msg.Nickname, false)

	case DMDeclinedMsg:
		// Remove the declined invite from pending list and notify server
		for i, invite := range m.pendingDMInvites {
			if invite.ChannelID == msg.ChannelID {
				m.pendingDMInvites = append(m.pendingDMInvites[:i], m.pendingDMInvites[i+1:]...)
				break
			}
		}
		// Send decline message to server so initiator gets notified
		return m, m.sendDeclineDM(msg.ChannelID)

	case DMDeclinedLocalMsg:
		// We've declined a DM invite and sent it to the server
		return m, m.setStatus("DM request declined")

	case StartDMSelectedMsg:
		// User selected someone to DM - close the start DM modal and show encryption choice
		m.modalStack.RemoveByType(modal.ModalStartDM)
		m.startDMWithUser(msg.UserID, msg.Nickname)
		return m, nil

	default:
		// Always update spinner (it manages its own tick messages)
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)

		// Update all modals that implement UpdatableModal
		var modalCmds []tea.Cmd
		m.modalStack.ForEach(func(mod modal.Modal) {
			if updatable, ok := mod.(modal.UpdatableModal); ok {
				if modalCmd := updatable.Update(msg); modalCmd != nil {
					modalCmds = append(modalCmds, modalCmd)
				}
			}
		})

		// Update viewport content if we're currently loading something
		if m.loadingThreadList || m.loadingMore {
			m.threadListViewport.SetContent(m.buildThreadListContent())
		}
		if m.loadingThreadReplies || m.loadingMoreReplies {
			m.threadViewport.SetContent(m.buildThreadContent())
		}

		// Batch all commands
		allCmds := append([]tea.Cmd{cmd}, modalCmds...)
		return m, tea.Batch(allCmds...)
	}

	return m, nil
}

// handleKeyPress handles keyboard input
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Update last interaction time (for notification idle detection)
	m.lastInteractionTime = time.Now()

	// Debug: log ctrl+k and ctrl+l specifically
	if key == "ctrl+k" {
		if m.logger != nil {
			m.logger.Printf("[DEBUG] handleKeyPress: Received ctrl+k, authState=%d, modalStack.Top=%v", m.authState, m.modalStack.TopType())
		}
	}
	if key == "ctrl+l" {
		if m.logger != nil {
			m.logger.Printf("[DEBUG] handleKeyPress: Received ctrl+l, view=%d, modalStack.Top=%v", m.currentView, m.modalStack.TopType())
		}
	}

	// Special case: ctrl+c always quits immediately
	if key == "ctrl+c" {
		return m, m.saveAndQuit()
	}

	// Check if active modal handles this key
	if activeModal := m.modalStack.Top(); activeModal != nil {
		// Modal is active - let it handle the key
		handled, newModal, cmd := activeModal.HandleKey(msg)

		if newModal == nil {
			// Modal requested to close
			m.modalStack.Pop()
		} else if newModal.Type() != activeModal.Type() {
			// Modal wants to be replaced with a different modal
			m.modalStack.Pop()
			m.modalStack.Push(newModal)
		}
		// else: modal stays the same

		if handled {
			return m, cmd
		}

		// Key not handled by modal - block it if modal is blocking
		if activeModal.IsBlockingInput() {
			return m, nil
		}
	}

	// No modal active or modal didn't handle the key

	// For chat view, only bypass command registry for normal keys (not Ctrl/Alt combinations)
	// This allows Ctrl+R, Ctrl+N, etc. to still work while typing goes to input
	if m.currentView == ViewChatChannel {
		// Check if this is a modifier key combination
		isModifierCombo := strings.HasPrefix(key, "ctrl+") ||
			strings.HasPrefix(key, "alt+") ||
			strings.HasPrefix(key, "shift+")

		if !isModifierCombo {
			// Normal key - send to chat input handler
			return m.handleChatChannelKeys(msg)
		}
		// Modifier combo - fall through to command registry
	}

	// Check shared commands first (new system)
	if sharedCmd := commands.FindCommandForKey(key, &m); sharedCmd != nil {
		if m.logger != nil {
			m.logger.Printf("[DEBUG] Executing shared command: %s (action: %s)", sharedCmd.Name, sharedCmd.ActionID)
		}
		return m.ExecuteActionWithReturn(sharedCmd.ActionID)
	}

	// Route through legacy command registry (old system - will be phased out)
	activeModalType := m.modalStack.TopType()
	if cmd := m.commands.GetCommand(key, int(m.currentView), activeModalType, &m); cmd != nil {
		if key == "ctrl+l" && m.logger != nil {
			m.logger.Printf("[DEBUG] Found ctrl+l command in legacy registry, executing...")
		}
		newModel, teaCmd := cmd.Execute(&m)
		if model, ok := newModel.(*Model); ok {
			return *model, teaCmd
		}
		return m, teaCmd
	}

	// Debug: log if ctrl+l command not found
	if key == "ctrl+l" && m.logger != nil {
		m.logger.Printf("[DEBUG] ctrl+l command NOT found in any registry")
	}

	// Fall back to existing key handlers (during migration period)
	return m.handleLegacyKeyPress(msg)
}

// handleLegacyKeyPress contains existing key handling code
// This will be gradually emptied as commands are migrated to the new system
func (m Model) handleLegacyKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// View-specific handling
	switch m.currentView {
	case ViewSplash:
		return m.handleSplashKeys(msg)
	case ViewChannelList:
		return m.handleChannelListKeys(msg)
	case ViewThreadList:
		return m.handleThreadListKeys(msg)
	case ViewThreadView:
		return m.handleThreadViewKeys(msg)
	case ViewChatChannel:
		return m.handleChatChannelKeys(msg)
	}

	return m, nil
}

// handleSplashKeys handles splash screen keys
func (m Model) handleSplashKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle scrolling keys
	switch msg.String() {
	case "up", "k":
		m.splashViewport.LineUp(1)
		return m, nil
	case "down", "j":
		m.splashViewport.LineDown(1)
		return m, nil
	case "pgup":
		m.splashViewport.ViewUp()
		return m, nil
	case "pgdown":
		m.splashViewport.ViewDown()
		return m, nil
	}

	// Any other key continues - go straight to browsing
	m.currentView = ViewChannelList
	m.loadingChannels = true

	// Don't auto-send SET_NICKNAME here - wait for AUTH_RESPONSE to arrive first (if SSH)
	// We'll send it from handleServerConfig after a short delay
	return m, m.requestChannelList()
}

// handleChannelListKeys handles channel list navigation
func (m Model) handleChannelListKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.channelCursor > 0 {
			m.channelCursor--
		}
		return m, nil

	case "down", "j":
		maxIndex := m.getVisibleChannelListItemCount() - 1
		if m.channelCursor < maxIndex {
			m.channelCursor++
		}
		return m, nil

	case "enter":
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
			channel := item.Channel
			// Check if channel has subchannels
			if channel.HasSubchannels && channel.SubchannelCount > 0 {
				// Toggle expand/collapse
				if m.expandedChannelID != nil && *m.expandedChannelID == channel.ID {
					// Collapse: clicking on already expanded channel
					m.expandedChannelID = nil
					m.subchannels = nil
					m.loadingSubchannels = false
				} else {
					// Expand: request subchannels
					channelID := channel.ID
					m.expandedChannelID = &channelID
					m.subchannels = nil
					m.loadingSubchannels = true
					return m, m.requestSubchannels(channel.ID)
				}
			} else {
				// Channel without subchannels - go directly to thread list
				selectedChannel := *channel
				m.currentChannel = &selectedChannel
				m.currentView = ViewThreadList
				m.loadingMore = false
				m.allThreadsLoaded = false
				return m, tea.Batch(
					m.sendJoinChannel(selectedChannel.ID),
					m.requestThreadList(selectedChannel.ID),
					m.sendSubscribeChannel(selectedChannel.ID),
				)
			}

		case ChannelListItemSubchannel:
			// Subchannel selected - navigate to it
			// For now, treat subchannel as a channel (they share the same ID space)
			subchannelID := item.Subchannel.ID
			m.currentChannel = &protocol.Channel{
				ID:             subchannelID,
				Name:           item.Subchannel.Name,
				Description:    item.Subchannel.Description,
				Type:           item.Subchannel.Type,
				RetentionHours: item.Subchannel.RetentionHours,
			}
			m.currentView = ViewThreadList
			m.loadingMore = false
			m.allThreadsLoaded = false
			return m, tea.Batch(
				m.sendJoinChannel(subchannelID),
				m.requestThreadList(subchannelID),
				m.sendSubscribeChannel(subchannelID),
			)
		}
		return m, nil

	case "r":
		// Refresh channel list
		m.expandedChannelID = nil
		m.subchannels = nil
		return m, m.requestChannelList()

	case "s":
		// Create subchannel - only works when cursor is on a channel (not subchannel)
		item := m.getChannelListItemAtCursor()
		if item != nil && item.Type == ChannelListItemChannel && item.Channel != nil {
			channel := item.Channel
			// Show create subchannel modal
			m.modalStack.Push(modal.NewCreateSubchannelModal(
				channel.ID,
				channel.Name,
				func(parentID uint64, name, description string, channelType uint8) tea.Cmd {
					return m.sendCreateSubchannel(parentID, name, description, channelType)
				},
				func() tea.Cmd { return nil },
			))
		}
		return m, nil

	case "esc":
		// If a channel is expanded, collapse it first
		if m.expandedChannelID != nil {
			m.expandedChannelID = nil
			m.subchannels = nil
			m.loadingSubchannels = false
			return m, nil
		}
		return m, m.saveAndQuit()
	}

	return m, nil
}

// handleThreadListKeys handles thread list navigation
func (m Model) handleThreadListKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.threadCursor > 0 {
			m.threadCursor--
			m.threadListViewport.SetContent(m.buildThreadListContent())
			m.scrollThreadListToKeepCursorVisible()
		}
		return m, nil

	case "down", "j":
		if m.threadCursor < len(m.threads)-1 {
			m.threadCursor++
			m.threadListViewport.SetContent(m.buildThreadListContent())
			m.scrollThreadListToKeepCursorVisible()

			// Load more threads if we're getting close to the end (within 25 threads)
			if !m.loadingMore && !m.allThreadsLoaded && len(m.threads) > 0 {
				remainingThreads := len(m.threads) - m.threadCursor - 1
				if remainingThreads <= 25 {
					m.loadingMore = true
					return m, m.loadMoreThreads()
				}
			}
		}
		return m, nil

	case "pgup":
		// Jump up by half the viewport height
		jumpSize := m.threadListViewport.Height / 2
		if jumpSize < 1 {
			jumpSize = 1
		}
		m.threadCursor -= jumpSize
		if m.threadCursor < 0 {
			m.threadCursor = 0
		}
		m.threadListViewport.SetContent(m.buildThreadListContent())
		m.scrollThreadListToKeepCursorVisible()
		return m, nil

	case "pgdown":
		// Jump down by half the viewport height
		jumpSize := m.threadListViewport.Height / 2
		if jumpSize < 1 {
			jumpSize = 1
		}
		m.threadCursor += jumpSize
		if m.threadCursor >= len(m.threads) {
			m.threadCursor = len(m.threads) - 1
		}
		m.threadListViewport.SetContent(m.buildThreadListContent())
		m.scrollThreadListToKeepCursorVisible()

		// Load more threads if we're getting close to the end (within 25 threads)
		if !m.loadingMore && !m.allThreadsLoaded && len(m.threads) > 0 {
			remainingThreads := len(m.threads) - m.threadCursor - 1
			if remainingThreads <= 25 {
				m.loadingMore = true
				return m, m.loadMoreThreads()
			}
		}
		return m, nil

	case "enter":
		if m.threadCursor < len(m.threads) {
			selectedThread := m.threads[m.threadCursor]
			m.currentThread = &selectedThread
			m.currentView = ViewThreadView
			m.replyCursor = 0
			m.newMessageIDs = make(map[uint64]bool) // Clear new message tracking
			m.confirmingDelete = false
			m.threadViewport.SetContent(m.buildThreadContent())
			m.threadViewport.GotoTop()
			return m, tea.Batch(
				m.requestThreadReplies(selectedThread.ID),
				m.sendSubscribeThread(selectedThread.ID),
			)
		}
		return m, nil

	case "r":
		// Refresh thread list
		if m.currentChannel != nil {
			return m, m.requestThreadList(m.currentChannel.ID)
		}
		return m, nil

	case "esc":
		// Back to channel list
		m.currentView = ViewChannelList
		m.confirmingDelete = false
		var cmd tea.Cmd
		if m.currentChannel != nil {
			cmd = tea.Batch(
				m.sendLeaveChannel(m.currentChannel.ID, false),
				m.sendUnsubscribeChannel(m.currentChannel.ID),
			)
			m.clearActiveChannel()
		}
		m.currentChannel = nil
		m.threads = []protocol.Message{}
		m.threadCursor = 0
		m.loadingMore = false
		m.allThreadsLoaded = false
		return m, cmd
	}

	return m, nil
}

// handleThreadViewKeys handles thread view navigation
func (m Model) handleThreadViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// Old delete confirmation handling removed - now handled by DeleteConfirmModal

	switch msg.String() {
	case "up", "k":
		if m.replyCursor > 0 {
			m.replyCursor--
			m.markCurrentMessageAsRead()
			m.threadViewport.SetContent(m.buildThreadContent())
			m.scrollToKeepCursorVisible()
		}
		return m, nil

	case "down", "j":
		if m.replyCursor < len(m.threadReplies) {
			m.replyCursor++
			m.markCurrentMessageAsRead()
			m.threadViewport.SetContent(m.buildThreadContent())
			m.scrollToKeepCursorVisible()
		}
		return m, nil

	case "esc":
		// Back to thread list
		m.currentView = ViewThreadList
		var cmd tea.Cmd
		if m.currentThread != nil {
			cmd = m.sendUnsubscribeThread(m.currentThread.ID)
		}
		m.threadReplies = []protocol.Message{}
		m.replyCursor = 0
		m.confirmingDelete = false
		m.pendingDeleteID = 0
		return m, cmd

	default:
		// Pass unhandled keys to viewport for scrolling (pgup/pgdown/etc)
		m.threadViewport, cmd = m.threadViewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

// handleChatChannelKeys handles keyboard input in chat channel view
func (m Model) handleChatChannelKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.String() {
	case "esc":
		// Exit chat and return to channel list
		m.currentView = ViewChannelList
		var cmd tea.Cmd
		if m.currentChannel != nil {
			channelID := m.currentChannel.ID
			cmd = tea.Batch(
				m.sendLeaveChannel(channelID, false),
				m.sendUnsubscribeChannel(channelID),
			)
			m.clearActiveChannel()
		}
		m.currentChannel = nil
		m.chatMessages = []protocol.Message{}
		m.chatTextarea.Reset()
		m.chatTextarea.Blur() // Unfocus when leaving
		return m, cmd

	case "ctrl+w":
		// Close DM permanently - remove from sidebar
		if m.currentChannel != nil && m.isCurrentChannelDM() {
			channelID := m.currentChannel.ID
			// Remove from local DM list
			newDMs := make([]DMChannel, 0, len(m.dmChannels))
			for _, dm := range m.dmChannels {
				if dm.ChannelID != channelID {
					newDMs = append(newDMs, dm)
				}
			}
			m.dmChannels = newDMs
			// Also remove encryption key if present
			delete(m.dmChannelKeys, channelID)

			// Return to channel list and send permanent leave
			m.currentView = ViewChannelList
			cmd := tea.Batch(
				m.sendLeaveChannel(channelID, true), // permanent=true
				m.sendUnsubscribeChannel(channelID),
			)
			m.clearActiveChannel()
			m.currentChannel = nil
			m.chatMessages = []protocol.Message{}
			m.chatTextarea.Reset()
			m.chatTextarea.Blur()
			return m, cmd
		}
		return m, nil

	case "enter":
		// Send message if input is not empty
		content := strings.TrimSpace(m.chatTextarea.Value())
		if content != "" {
			// Check if we should show registration warning
			if m.shouldShowRegistrationWarning() {
				// Store content and show warning - use a message to avoid stale closure
				m.showRegistrationWarningModal(func() tea.Cmd {
					// Return a message that Update will handle with current model state
					return func() tea.Msg {
						return PostChatMessageMsg{Content: content}
					}
				})
				return m, nil
			} else {
				// Send directly
				m.chatTextarea.Reset() // Clear textarea
				return m.sendChatMessageWithContent(content)
			}
		}
		return m, nil

	case "up", "down", "pgup", "pgdown":
		// Allow scrolling through message history
		m.chatViewport, cmd = m.chatViewport.Update(msg)
		return m, cmd

	default:
		// Pass all other keys to the textarea
		m.chatTextarea, cmd = m.chatTextarea.Update(msg)
		return m, cmd
	}
}

// sendChatMessageWithContent sends a chat message with the given content
func (m Model) sendChatMessageWithContent(content string) (Model, tea.Cmd) {
	if m.currentChannel == nil {
		return m, nil
	}

	if content == "" {
		return m, nil
	}

	channelID := m.currentChannel.ID
	messageContent := content

	// Check if this channel has encryption enabled
	if key, ok := m.dmChannelKeys[channelID]; ok {
		encrypted, err := crypto.EncryptMessage(key, []byte(content))
		if err != nil {
			m.errorMessage = fmt.Sprintf("Failed to encrypt message: %v", err)
			return m, nil
		}
		messageContent = string(encrypted)
	}

	// Send POST_MESSAGE
	msg := &protocol.PostMessageMessage{
		ChannelID:    channelID,
		SubchannelID: nil,
		ParentID:     nil, // Chat channels have no threading
		Content:      messageContent,
	}

	return m, func() tea.Msg {
		if err := m.conn.SendMessage(protocol.TypePostMessage, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

// selectedMessage returns the currently highlighted message, if any.
func (m Model) selectedMessage() (*protocol.Message, bool) {
	if m.currentThread == nil {
		return nil, false
	}
	if m.replyCursor == 0 {
		return m.currentThread, true
	}
	idx := m.replyCursor - 1
	if idx >= 0 && idx < len(m.threadReplies) {
		return &m.threadReplies[idx], true
	}
	return nil, false
}

// selectedMessageID returns the id of the highlighted message.
func (m Model) selectedMessageID() (uint64, bool) {
	msg, ok := m.selectedMessage()
	if !ok {
		return 0, false
	}
	return msg.ID, true
}

func (m Model) selectedMessageDeleted() bool {
	msg, ok := m.selectedMessage()
	if !ok {
		return false
	}
	return isDeletedMessageContent(msg.Content)
}

// handleServerFrame processes incoming server frames
func (m Model) handleServerFrame(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	// Ignore nil frames (can happen when channel closes)
	if frame == nil {
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	switch frame.Type {
	case protocol.TypeServerConfig:
		return m.handleServerConfig(frame)
	case protocol.TypeAuthResponse:
		return m.handleAuthResponse(frame)
	case protocol.TypeRegisterResponse:
		return m.handleRegisterResponse(frame)
	case protocol.TypeNicknameResponse:
		return m.handleNicknameResponse(frame)
	case protocol.TypeUserInfo:
		return m.handleUserInfo(frame)
	case protocol.TypeChannelList:
		return m.handleChannelList(frame)
	case protocol.TypeSubchannelList:
		return m.handleSubchannelList(frame)
	case protocol.TypeServerList:
		return m.handleServerList(frame)
	case protocol.TypeChannelCreated:
		return m.handleChannelCreated(frame)
	case protocol.TypeSubchannelCreated:
		return m.handleSubchannelCreated(frame)
	case protocol.TypeChannelDeleted:
		return m.handleChannelDeleted(frame)
	case protocol.TypeJoinResponse:
		return m.handleJoinResponse(frame)
	case protocol.TypeLeaveResponse:
		return m.handleLeaveResponse(frame)
	case protocol.TypeMessageList:
		return m.handleMessageList(frame)
	case protocol.TypeMessagePosted:
		return m.handleMessagePosted(frame)
	case protocol.TypeNewMessage:
		return m.handleNewMessage(frame)
	case protocol.TypeMessageEdited:
		return m.handleMessageEdited(frame)
	case protocol.TypeMessageDeleted:
		return m.handleMessageDeleted(frame)
	case protocol.TypeSubscribeOk:
		return m.handleSubscribeOk(frame)
	case protocol.TypeError:
		return m.handleError(frame)
	case protocol.TypeSSHKeyList:
		return m.handleSSHKeyList(frame)
	case protocol.TypeSSHKeyAdded:
		return m.handleSSHKeyAdded(frame)
	case protocol.TypeSSHKeyLabelUpdated:
		return m.handleSSHKeyLabelUpdated(frame)
	case protocol.TypeSSHKeyDeleted:
		return m.handleSSHKeyDeleted(frame)
	case protocol.TypeUserBanned:
		return m.handleUserBanned(frame)
	case protocol.TypeIPBanned:
		return m.handleIPBanned(frame)
	case protocol.TypeUserUnbanned:
		return m.handleUserUnbanned(frame)
	case protocol.TypeIPUnbanned:
		return m.handleIPUnbanned(frame)
	case protocol.TypeBanList:
		return m.handleBanList(frame)
	case protocol.TypeUserList:
		return m.handleUserList(frame)
	case protocol.TypeUserDeleted:
		return m.handleUserDeleted(frame)
	case protocol.TypeDisconnect:
		return m.handleDisconnect(frame)
	case protocol.TypeChannelUserList:
		return m.handleChannelUserList(frame)
	case protocol.TypeChannelPresence:
		return m.handleChannelPresence(frame)
	case protocol.TypeServerPresence:
		return m.handleServerPresence(frame)
	case protocol.TypeUnreadCounts:
		return m.handleUnreadCounts(frame)
	// DM responses
	case protocol.TypeKeyRequired:
		return m.handleKeyRequired(frame)
	case protocol.TypeDMReady:
		return m.handleDMReady(frame)
	case protocol.TypeDMPending:
		return m.handleDMPending(frame)
	case protocol.TypeDMRequest:
		return m.handleDMRequest(frame)
	case protocol.TypeDMParticipantLeft:
		return m.handleDMParticipantLeft(frame)
	case protocol.TypeDMDeclined:
		return m.handleDMDeclined(frame)
	}

	// Continue listening
	return m, listenForServerFrames(m.conn, m.connGeneration)
}

// InitTimeoutCheckMsg triggers periodic checking of initialization timeout
type InitTimeoutCheckMsg struct{}

// ClearStatusMsg clears the status message after a timeout
type ClearStatusMsg struct {
	Version uint64 // Only clear if this matches current statusVersion
}

// statusTimeout returns a command that clears the status after 3 seconds
func statusTimeout(version uint64) tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return ClearStatusMsg{Version: version}
	})
}

// setStatus sets the status message and returns the timeout command
func (m *Model) setStatus(message string) tea.Cmd {
	m.statusVersion++
	m.statusMessage = message
	return statusTimeout(m.statusVersion)
}

// handleServerConfig processes SERVER_CONFIG
func (m Model) handleServerConfig(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.ServerConfigMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode SERVER_CONFIG: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	m.serverConfig = msg
	statusCmd := m.setStatus(fmt.Sprintf("Connected (protocol v%d)", msg.ProtocolVersion))

	// Transition state machine based on connection type
	m.initStateMachine.OnServerConfig()

	if m.logger != nil {
		m.logger.Printf("[InitStateMachine] State after SERVER_CONFIG: %s", m.initStateMachine.State())
	}

	// If TCP connection (not SSH), send SET_NICKNAME immediately
	if m.initStateMachine.NeedsNickname() && m.nickname != "" {
		if m.logger != nil {
			m.logger.Printf("[InitStateMachine] TCP connection, sending SET_NICKNAME")
		}
		return m, tea.Batch(
			listenForServerFrames(m.conn, m.connGeneration),
			m.sendSetNickname(),
			m.sendGetUserInfo(m.nickname),
			statusCmd,
		)
	}

	// SSH connection - wait for AUTH_RESPONSE, check timeout with exponential backoff
	return m, tea.Batch(
		listenForServerFrames(m.conn, m.connGeneration),
		checkInitTimeout(m.initStateMachine.NextCheckDelay()),
		statusCmd,
	)
}

// checkInitTimeout returns a command that checks initialization timeout after a delay
func checkInitTimeout(delay time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(delay)
		return InitTimeoutCheckMsg{}
	}
}

// handleNicknameResponse processes NICKNAME_RESPONSE
func (m Model) handleNicknameResponse(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.NicknameResponseMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode response: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	var statusCmd tea.Cmd
	if msg.Success {
		// Use the pending nickname we sent (stored when we sent the request)
		if m.pendingNickname != "" {
			m.nickname = m.pendingNickname
			m.pendingNickname = "" // Clear it
		}

		// Try to load any stored encryption key for this nickname/user
		m.loadEncryptionKey()

		// Show appropriate status message based on auth state
		if m.authState == AuthStateAuthenticated {
			// Registered user changed nickname - PRESERVE authenticated state
			statusCmd = m.setStatus(fmt.Sprintf("Nickname changed to %s", m.nickname))
			// DO NOT change m.authState - keep it as AuthStateAuthenticated
		} else {
			// Anonymous user set nickname (may prompt for auth if registered)
			statusCmd = m.setStatus(fmt.Sprintf("Nickname set to %s", m.nickname))
			// Set auth state to anonymous (allows registration with Ctrl+R)
			// This may change later if USER_INFO shows nickname is registered
			m.authState = AuthStateAnonymous
		}

		// Close the nickname modal if open
		m.modalStack.RemoveByType(modal.ModalNicknameChange)
		m.modalStack.RemoveByType(modal.ModalNicknameSetup)
	} else {
		// Nickname rejected (invalid format, banned, etc.)
		m.errorMessage = msg.Message
	}

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

// handleUserInfo processes USER_INFO response
func (m Model) handleUserInfo(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.UserInfoMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode USER_INFO: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	// Log the response for debugging
	if m.logger != nil {
		m.logger.Printf("[DEBUG] USER_INFO response: nickname=%s, isRegistered=%v, online=%v", msg.Nickname, msg.IsRegistered, msg.Online)
		m.logger.Printf("[DEBUG] Current nickname=%s, pending=%s", m.nickname, m.pendingNickname)
	}

	// Update our tracking of whether this nickname is registered
	// Only update if this is info about our current or pending nickname
	if msg.Nickname == m.nickname || msg.Nickname == m.pendingNickname {
		m.nicknameIsRegistered = msg.IsRegistered
		if m.logger != nil {
			m.logger.Printf("[DEBUG] Updated nicknameIsRegistered=%v", m.nicknameIsRegistered)
		}

		// If this nickname is registered and we're not authenticated, show password modal
		if msg.IsRegistered && m.authState != AuthStateAuthenticated && !m.directoryMode {
			// Show password modal if this is our current or pending nickname
			// (handles race condition where USER_INFO arrives before NICKNAME_RESPONSE)
			if msg.Nickname == m.nickname || msg.Nickname == m.pendingNickname {
				m.authTargetNickname = msg.Nickname
				m.authErrorMessage = ""
				m.authAttempts = 0
				m.authCooldownUntil = time.Time{}
				m.showPasswordModal()
				if m.logger != nil {
					m.logger.Printf("[DEBUG] Showing password modal for registered nickname: %s", msg.Nickname)
				}
			}
		}
	}

	// Force a re-render so the UI updates (footer commands change based on nicknameIsRegistered)
	return m, tea.Batch(
		listenForServerFrames(m.conn, m.connGeneration),
		func() tea.Msg { return ForceRenderMsg{} },
	)
}

// ForceRenderMsg triggers a re-render without any other action
type ForceRenderMsg struct{}

// GoAnonymousMsg is sent when user chooses to browse anonymously
type GoAnonymousMsg struct {
	TargetNickname string // The registered nickname they were trying to use
}

// handleAuthResponse processes AUTH_RESPONSE
func (m Model) handleAuthResponse(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.AuthResponseMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		if m.logger != nil {
			m.logger.Printf("[ERROR] Failed to decode AUTH_RESPONSE: %v", err)
		}
		m.errorMessage = fmt.Sprintf("Failed to decode AUTH_RESPONSE: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	if m.logger != nil {
		m.logger.Printf("[DEBUG] AUTH_RESPONSE: Success=%v, UserID=%d, Nickname='%s', Message='%s'", msg.Success, msg.UserID, msg.Nickname, msg.Message)
	}

	if msg.Success {
		// Successfully authenticated
		m.authState = AuthStateAuthenticated
		m.userID = &msg.UserID
		if msg.UserFlags != nil {
			m.userFlags = *msg.UserFlags
		} else {
			m.userFlags = 0
		}
		m.authAttempts = 0
		m.authErrorMessage = ""
		m.authTargetNickname = ""

		// Transition state machine to Ready
		if m.initStateMachine.OnAuthResponse() {
			if m.logger != nil {
				m.logger.Printf("[InitStateMachine] Transitioned to Ready after AUTH_RESPONSE")
			}
		}

		// Update nickname from server (especially important for SSH auth)
		if msg.Nickname != "" {
			if m.logger != nil {
				m.logger.Printf("[DEBUG] AUTH_RESPONSE: setting nickname to '%s' (was '%s')", msg.Nickname, m.nickname)
			}
			m.nickname = msg.Nickname
			m.state.SetLastNickname(msg.Nickname)
		}

		statusCmd := m.setStatus(fmt.Sprintf("Authenticated as %s", m.nickname))

		// Save user ID to state
		m.state.SetUserID(&msg.UserID)

		// Try to load any stored encryption key for this user
		m.loadEncryptionKey()

		// Close password modal if it's open
		m.modalStack.RemoveByType(modal.ModalPasswordAuth)

		return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
	}

	m.userFlags = 0
	// Authentication failed
	m.authState = AuthStatePrompting
	m.authAttempts++
	m.authErrorMessage = msg.Message

	// Apply rate limiting with exponential backoff
	if m.authAttempts >= 5 {
		m.errorMessage = "Too many failed attempts. Please restart the application."
		m.authState = AuthStateFailed
		m.modalStack.RemoveByType(modal.ModalPasswordAuth)
	} else {
		if m.authAttempts >= 2 {
			// Exponential backoff: 1s, 2s, 4s, 8s
			cooldownSeconds := 1 << (m.authAttempts - 2) // 2^(attempts-2)
			m.authCooldownUntil = time.Now().Add(time.Duration(cooldownSeconds) * time.Second)
		}
		// Update password modal with error message
		m.modalStack.RemoveByType(modal.ModalPasswordAuth)
		m.showPasswordModal()
	}

	return m, listenForServerFrames(m.conn, m.connGeneration)
}

// handleRegisterResponse processes REGISTER_RESPONSE
func (m Model) handleRegisterResponse(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.RegisterResponseMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode REGISTER_RESPONSE: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	var statusCmd tea.Cmd
	if msg.Success {
		// Successfully registered
		m.authState = AuthStateAuthenticated
		m.userID = &msg.UserID
		m.userFlags = 0
		statusCmd = m.setStatus(fmt.Sprintf("Registered as %s", m.nickname))

		// Save user ID to state
		m.state.SetUserID(&msg.UserID)

		// Close registration modal if it's open
		m.modalStack.RemoveByType(modal.ModalRegistration)
	} else {
		m.userFlags = 0
		// Registration failed - close modal and show error
		m.modalStack.RemoveByType(modal.ModalRegistration)
		m.errorMessage = "Registration failed. Please try again."
	}

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

// handleChannelList processes CHANNEL_LIST
func (m Model) handleChannelList(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	m.loadingChannels = false

	msg := &protocol.ChannelListMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode channel list: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	m.channels = msg.Channels
	statusCmd := m.setStatus(fmt.Sprintf("Loaded %d channels", len(m.channels)))

	// Request unread counts for all channels
	if len(m.channels) > 0 {
		targets := make([]protocol.UnreadTarget, len(m.channels))
		for i, channel := range m.channels {
			targets[i] = protocol.UnreadTarget{
				ChannelID:    channel.ID,
				SubchannelID: nil,
				ThreadID:     nil,
			}
		}

		var sinceTimestamp *int64
		if m.userID == nil {
			// Anonymous user: use locally stored last seen timestamp
			lastSeen := m.state.GetLastSeenTimestamp()
			if lastSeen > 0 {
				sinceTimestamp = &lastSeen
			}
			// If no last seen timestamp, skip the request (first time user)
			if sinceTimestamp == nil {
				return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
			}
		}
		// For registered users, sinceTimestamp stays nil and server uses stored UserChannelState

		unreadMsg := &protocol.GetUnreadCountsMessage{
			SinceTimestamp: sinceTimestamp,
			Targets:        targets,
		}

		if err := m.conn.SendMessage(protocol.TypeGetUnreadCounts, unreadMsg); err != nil {
			m.errorMessage = fmt.Sprintf("Failed to request unread counts: %v", err)
		}
	}

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

// handleSubchannelList processes SUBCHANNEL_LIST (0x96)
func (m Model) handleSubchannelList(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	m.loadingSubchannels = false

	msg := &protocol.SubchannelListMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode subchannel list: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	// Only update if this is for the currently expanded channel
	if m.expandedChannelID != nil && *m.expandedChannelID == msg.ChannelID {
		m.subchannels = msg.Subchannels
	}

	return m, listenForServerFrames(m.conn, m.connGeneration)
}

// handleSubchannelCreated processes SUBCHANNEL_CREATED (0x88)
func (m Model) handleSubchannelCreated(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.SubchannelCreatedMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode subchannel created: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	var statusCmd tea.Cmd
	if msg.Success {
		statusCmd = m.setStatus(msg.Message)

		// If the parent channel is currently expanded, add the new subchannel to the list
		if m.expandedChannelID != nil && *m.expandedChannelID == msg.ChannelID {
			newSubchannel := protocol.SubchannelInfo{
				ID:             msg.SubchannelID,
				Name:           msg.Name,
				Description:    msg.Description,
				Type:           msg.Type,
				RetentionHours: msg.RetentionHours,
			}
			m.subchannels = append(m.subchannels, newSubchannel)
		}

		// Update the parent channel's subchannel count
		for i := range m.channels {
			if m.channels[i].ID == msg.ChannelID {
				m.channels[i].SubchannelCount++
				m.channels[i].HasSubchannels = true
				break
			}
		}
	} else {
		m.errorMessage = msg.Message
	}

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

// handleServerList processes SERVER_LIST (0x9B)
func (m Model) handleServerList(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.ServerListMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode server list: %v", err)

		// Update modal to show error
		if activeModal := m.modalStack.Top(); activeModal != nil {
			if serverModal, ok := activeModal.(*modal.ServerSelectorModal); ok {
				serverModal.SetError(fmt.Sprintf("Failed to decode server list: %v", err))
			}
		}

		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	// Clear awaiting flag since we got a response
	m.awaitingServerList = false

	var statusCmd tea.Cmd
	// Update the server selector modal with the received list
	if activeModal := m.modalStack.Top(); activeModal != nil {
		if serverModal, ok := activeModal.(*modal.ServerSelectorModal); ok {
			serverModal.SetServers(msg.Servers)

			if len(msg.Servers) == 0 {
				statusCmd = m.setStatus("No servers available")
			} else {
				statusCmd = m.setStatus(fmt.Sprintf("Loaded %d servers", len(msg.Servers)))
			}
		}
	}

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

// handleChannelCreated processes CHANNEL_CREATED (response + broadcast)
func (m Model) handleChannelCreated(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.ChannelCreatedMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.statusMessage = "" // Clear in-progress status
		m.errorMessage = fmt.Sprintf("Failed to decode channel created: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	var statusCmd tea.Cmd
	if msg.Success {
		// Close the create channel modal if it's open
		m.modalStack.RemoveByType(modal.ModalCreateChannel)

		// Add the new channel to the list
		newChannel := protocol.Channel{
			ID:             msg.ChannelID,
			Name:           msg.Name,
			Description:    msg.Description,
			UserCount:      0,
			IsOperator:     true, // Creator is always operator
			Type:           msg.Type,
			RetentionHours: msg.RetentionHours,
		}
		m.channels = append(m.channels, newChannel)

		statusCmd = m.setStatus(fmt.Sprintf("Channel '%s' created successfully", msg.Name))
	} else {
		// Keep modal open but show error
		m.statusMessage = "" // Clear in-progress status
		m.errorMessage = msg.Message
	}

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

// handleChannelDeleted processes CHANNEL_DELETED (response + broadcast)
func (m Model) handleChannelDeleted(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.ChannelDeletedMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.statusMessage = "" // Clear in-progress status
		m.errorMessage = fmt.Sprintf("Failed to decode channel deleted: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	var statusCmd tea.Cmd
	if msg.Success {
		// Close the delete channel modal if it's open
		m.modalStack.RemoveByType(modal.ModalDeleteChannel)

		// Remove the channel from the list
		for i, ch := range m.channels {
			if ch.ID == msg.ChannelID {
				m.channels = append(m.channels[:i], m.channels[i+1:]...)

				// If we were in the deleted channel, navigate to channel list
				if m.currentChannel != nil && m.currentChannel.ID == msg.ChannelID {
					m.clearActiveChannel()
					m.currentChannel = nil
					m.threads = nil
					m.currentThread = nil
					m.threadReplies = nil
					m.currentView = ViewChannelList
				}

				break
			}
		}

		statusCmd = m.setStatus(msg.Message)
	} else {
		// Keep modal open but show error
		m.statusMessage = "" // Clear in-progress status
		m.errorMessage = msg.Message
	}

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

// handleJoinResponse processes JOIN_RESPONSE
func (m Model) handleJoinResponse(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.JoinResponseMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode join response: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	if msg.Success {
		statusCmd := m.setStatus(msg.Message)
		m.setActiveChannel(msg.ChannelID)
		cmds := []tea.Cmd{listenForServerFrames(m.conn, m.connGeneration), statusCmd}
		if m.showUserSidebar {
			cmds = append(cmds, m.sendListChannelUsers(msg.ChannelID))
			cmds = append(cmds, func() tea.Msg { return ForceRenderMsg{} })
		}
		return m, tea.Batch(cmds...)
	}

	m.errorMessage = msg.Message
	return m, listenForServerFrames(m.conn, m.connGeneration)
}

// handleLeaveResponse processes LEAVE_RESPONSE
func (m Model) handleLeaveResponse(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.LeaveResponseMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode leave response: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	var statusCmd tea.Cmd
	if msg.Success {
		statusCmd = m.setStatus(msg.Message)
		m.clearActiveChannel()
		delete(m.channelRoster, msg.ChannelID)
	} else {
		m.errorMessage = msg.Message
	}

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

// handleMessageList processes MESSAGE_LIST
func (m Model) handleMessageList(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.MessageListMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode message list: %v", err)
		m.loadingThreadList = false
		m.loadingThreadReplies = false
		m.loadingMore = false
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	// Decrypt messages if channel has encryption enabled
	if m.currentChannel != nil {
		if key, ok := m.dmChannelKeys[m.currentChannel.ID]; ok {
			for i := range msg.Messages {
				decrypted, err := crypto.DecryptMessage(key, []byte(msg.Messages[i].Content))
				if err != nil {
					if m.logger != nil {
						m.logger.Printf("[DM] Failed to decrypt message %d: %v", msg.Messages[i].ID, err)
					}
					msg.Messages[i].Content = "[Encrypted message - decryption failed]"
				} else {
					msg.Messages[i].Content = string(decrypted)
				}
			}
		}
	}

	var statusCmd tea.Cmd
	if msg.ParentID == nil {
		// Root messages - could be thread list OR chat messages
		// Check if we're in chat view
		if m.currentView == ViewChatChannel {
			// Chat messages (linear)
			m.loadingChat = false

			// Initial load - replace chat messages and sort by timestamp (oldest first)
			m.chatMessages = msg.Messages
			sort.Slice(m.chatMessages, func(i, j int) bool {
				return m.chatMessages[i].CreatedAt.Before(m.chatMessages[j].CreatedAt)
			})

			m.allChatLoaded = len(msg.Messages) < 100
			statusCmd = m.setStatus(fmt.Sprintf("Loaded %d messages", len(m.chatMessages)))

			// Update viewport to show loaded messages
			m.chatViewport.SetContent(m.buildChatMessages())

			// Auto-scroll to bottom (newest messages)
			m.chatViewport.GotoBottom()
		} else {
			// Thread list (forum view)
			m.loadingThreadList = false

			if m.loadingMore {
				// Append to existing threads
				m.threads = append(m.threads, msg.Messages...)
				m.loadingMore = false

				// If we got fewer than 25, we've reached the end
				if len(msg.Messages) < 25 {
					m.allThreadsLoaded = true
				}

				statusCmd = m.setStatus(fmt.Sprintf("Loaded %d more threads", len(msg.Messages)))
			} else {
				// Initial load - replace threads
				m.threads = msg.Messages
				m.allThreadsLoaded = len(msg.Messages) < 25
				statusCmd = m.setStatus(fmt.Sprintf("Loaded %d threads", len(m.threads)))
			}

			// Update viewport to show loaded threads
			m.threadListViewport.SetContent(m.buildThreadListContent())
		}
	} else {
		// Thread replies - sort them in depth-first order
		m.loadingThreadReplies = false
		isLoadingMore := m.loadingMoreReplies
		m.loadingMoreReplies = false

		if m.currentThread != nil {
			newReplies := msg.Messages

			if isLoadingMore {
				// Pagination: append to existing replies
				m.threadReplies = append(m.threadReplies, newReplies...)
				m.threadReplies = client.SortThreadReplies(m.threadReplies, m.currentThread.ID)

				// Check if we've reached the end
				if len(newReplies) < 10 {
					m.allRepliesLoaded = true
				}

				statusCmd = m.setStatus(fmt.Sprintf("Loaded %d more replies", len(newReplies)))
			} else if cachedReplies, ok := m.threadRepliesCache[m.currentThread.ID]; ok && len(newReplies) > 0 {
				// Incremental update: merge cached and new replies
				merged := append(cachedReplies, newReplies...)
				m.threadReplies = client.SortThreadReplies(merged, m.currentThread.ID)
				statusCmd = m.setStatus(fmt.Sprintf("Loaded %d new replies", len(newReplies)))
			} else {
				// Initial load: replace replies
				m.threadReplies = client.SortThreadReplies(msg.Messages, m.currentThread.ID)
				m.allRepliesLoaded = len(msg.Messages) < 10
				statusCmd = m.setStatus(fmt.Sprintf("Loaded %d replies", len(m.threadReplies)))
			}

			// Cache the sorted replies
			m.threadRepliesCache[m.currentThread.ID] = m.threadReplies

			// Track highest message ID
			highestID := uint64(0)
			for _, reply := range m.threadReplies {
				if reply.ID > highestID {
					highestID = reply.ID
				}
			}
			if highestID > 0 {
				m.threadHighestMessageID[m.currentThread.ID] = highestID
			}
		} else {
			m.threadReplies = msg.Messages
			statusCmd = m.setStatus(fmt.Sprintf("Loaded %d replies", len(m.threadReplies)))
		}

		// Update viewport to show loaded replies
		m.threadViewport.SetContent(m.buildThreadContent())
	}

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

// handleMessagePosted processes MESSAGE_POSTED
func (m Model) handleMessagePosted(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	m.sendingMessage = false

	msg := &protocol.MessagePostedMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.statusMessage = "" // Clear "Sending..." status
		m.errorMessage = fmt.Sprintf("Failed to decode response: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	var statusCmd tea.Cmd
	if msg.Success {
		statusCmd = m.setStatus("Message posted")

		// Don't request message lists - rely on NEW_MESSAGE broadcasts instead
		// The server will broadcast our message to us as a subscriber, and handleNewMessage
		// will add it to the appropriate list (threads or threadReplies)
	} else {
		m.statusMessage = "" // Clear "Sending..." status
		m.errorMessage = msg.Message
	}

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

// handleNewMessage processes NEW_MESSAGE broadcasts
func (m Model) handleNewMessage(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.NewMessageMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode new message: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	// Convert to protocol.Message
	newMsg := protocol.Message(*msg)

	// Decrypt content if this channel has encryption enabled
	if key, ok := m.dmChannelKeys[newMsg.ChannelID]; ok {
		decrypted, err := crypto.DecryptMessage(key, []byte(newMsg.Content))
		if err != nil {
			if m.logger != nil {
				m.logger.Printf("[DM] Failed to decrypt message %d: %v", newMsg.ID, err)
			}
			// Show encrypted indicator instead of garbage
			newMsg.Content = "[Encrypted message - decryption failed]"
		} else {
			newMsg.Content = string(decrypted)
		}
	}

	// Add to appropriate list
	if m.currentChannel != nil && newMsg.ChannelID == m.currentChannel.ID {
		if newMsg.ParentID == nil {
			// New root message - could be chat or thread depending on view
			if m.currentView == ViewChatChannel {
				// Chat message - append to end (newest last)
				m.chatMessages = append(m.chatMessages, newMsg)
				m.chatViewport.SetContent(m.buildChatMessages())
				// Auto-scroll to bottom to show new message
				m.chatViewport.GotoBottom()

			// Send desktop notification if user is idle
			if m.shouldNotifyForMessage(newMsg) {
				m.sendDesktopNotification(newMsg)
			}
			} else {
				// Forum thread - add to threads
				m.threads = append([]protocol.Message{newMsg}, m.threads...)
				// Sort threads by created_at descending (newest first)
				sort.Slice(m.threads, func(i, j int) bool {
					return m.threads[i].CreatedAt.After(m.threads[j].CreatedAt)
				})
				m.threadListViewport.SetContent(m.buildThreadListContent())

				// If this is our own new thread and we're in thread list view, select it
				// Server adds ~ prefix for anonymous users
				var isOwnThread bool
				if m.authState == AuthStateAuthenticated {
					isOwnThread = newMsg.AuthorNickname == m.nickname
				} else {
					isOwnThread = newMsg.AuthorNickname == "~"+m.nickname
				}

				if m.currentView == ViewThreadList && isOwnThread {
					for i, thread := range m.threads {
						if thread.ID == newMsg.ID {
							m.threadCursor = i
							break
						}
					}
				}
			}
		} else if m.currentThread != nil && newMsg.ParentID != nil {
			// Check if this message belongs to the current thread
			// (either replying to root or to any existing reply)
			belongsToThread := *newMsg.ParentID == m.currentThread.ID
			if !belongsToThread {
				// Check if it's a reply to any existing reply in the thread
				for _, reply := range m.threadReplies {
					if *newMsg.ParentID == reply.ID {
						belongsToThread = true
						break
					}
				}
			}

			if belongsToThread {
				// Reply to current thread - add it
				m.threadReplies = append(m.threadReplies, newMsg)
				// Sort replies in depth-first order based on tree structure
				m.threadReplies = client.SortThreadReplies(m.threadReplies, m.currentThread.ID)

				// Update cache with new message
				m.threadRepliesCache[m.currentThread.ID] = m.threadReplies

				// Update highest message ID if this is newer
				if newMsg.ID > m.threadHighestMessageID[m.currentThread.ID] {
					m.threadHighestMessageID[m.currentThread.ID] = newMsg.ID
				}

				if m.currentView == ViewThreadView {
					// Check if this is our own message
					// Server adds ~ prefix for anonymous users
					var isOwnMessage bool
					if m.authState == AuthStateAuthenticated {
						isOwnMessage = newMsg.AuthorNickname == m.nickname
					} else {
						isOwnMessage = newMsg.AuthorNickname == "~"+m.nickname
					}

					if isOwnMessage {
						// Scroll to and select our own message
						for i, reply := range m.threadReplies {
							if reply.ID == newMsg.ID {
								m.replyCursor = i + 1 // +1 because 0 is root
								break
							}
						}
						m.threadViewport.SetContent(m.buildThreadContent())
						m.scrollToKeepCursorVisible()
					} else {
						// Mark others' messages as new
						m.newMessageIDs[newMsg.ID] = true
						m.threadViewport.SetContent(m.buildThreadContent())

						// Send desktop notification if user is idle
						if m.shouldNotifyForMessage(newMsg) {
							m.sendDesktopNotification(newMsg)
						}
					}
				} else {
					m.threadViewport.SetContent(m.buildThreadContent())
				}
			}
		}
	}

	return m, listenForServerFrames(m.conn, m.connGeneration)
}

// handleMessageDeleted processes MESSAGE_DELETED confirmations and broadcasts.
func (m Model) handleMessageDeleted(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	m.sendingMessage = false

	msg := &protocol.MessageDeletedMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode message deletion: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	var statusCmd tea.Cmd
	if msg.Success {
		m.applyMessageDeletion(msg.MessageID, msg.Message)
		statusCmd = m.setStatus("Message deleted")
	} else {
		m.errorMessage = msg.Message
	}

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

// handleMessageEdited processes MESSAGE_EDITED confirmations and broadcasts.
func (m Model) handleMessageEdited(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	m.sendingMessage = false

	msg := &protocol.MessageEditedMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode message edit: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	var statusCmd tea.Cmd
	if msg.Success {
		m.applyMessageEdit(msg.MessageID, msg.NewContent, msg.EditedAt)
		statusCmd = m.setStatus("Message edited")
	} else {
		m.errorMessage = msg.Message
	}

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

// handleSubscribeOk processes SUBSCRIBE_OK confirmations
func (m Model) handleSubscribeOk(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.SubscribeOkMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode subscribe OK: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	// Subscription confirmed - no user-visible action needed
	// The subscription is now active on the server
	_ = msg // silence unused variable warning

	return m, listenForServerFrames(m.conn, m.connGeneration)
}

// handleError processes ERROR messages
func (m Model) handleError(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.ErrorMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode error: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	m.errorMessage = fmt.Sprintf("Error %d: %s", msg.ErrorCode, msg.Message)

	return m, listenForServerFrames(m.conn, m.connGeneration)
}

// handleDisconnect processes DISCONNECT messages from the server
func (m Model) handleDisconnect(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.DisconnectMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode disconnect message: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	// Store the disconnect reason for display in modal
	if msg.Reason != nil && *msg.Reason != "" {
		m.serverDisconnectReason = *msg.Reason
	} else {
		m.serverDisconnectReason = "No reason provided"
	}

	// Disable auto-reconnect for server-initiated disconnects
	// The user should explicitly choose to reconnect via the modal
	m.conn.DisableAutoReconnect()

	// The connection will be closed by the server shortly
	// Continue listening for the actual disconnect event
	return m, listenForServerFrames(m.conn, m.connGeneration)
}

// Command helpers

// applyMessageDeletion updates local state to reflect a deleted message.
func (m *Model) applyMessageDeletion(messageID uint64, replacement string) {
	if m.pendingDeleteID == messageID {
		m.pendingDeleteID = 0
		m.confirmingDelete = false
	}

	updatedThreadList := false
	for i := range m.threads {
		if m.threads[i].ID == messageID {
			m.threads[i].Content = replacement
			updatedThreadList = true
		}
	}

	if m.currentThread != nil && m.currentThread.ID == messageID {
		m.currentThread.Content = replacement
	}

	for i := range m.threadReplies {
		if m.threadReplies[i].ID == messageID {
			m.threadReplies[i].Content = replacement
		}
	}

	delete(m.newMessageIDs, messageID)

	if updatedThreadList {
		m.threadListViewport.SetContent(m.buildThreadListContent())
	}
	if m.currentView == ViewThreadView {
		m.threadViewport.SetContent(m.buildThreadContent())
	}
}

// applyMessageEdit updates local state to reflect an edited message.
func (m *Model) applyMessageEdit(messageID uint64, newContent string, editedAt time.Time) {
	updatedThreadList := false
	for i := range m.threads {
		if m.threads[i].ID == messageID {
			m.threads[i].Content = newContent
			m.threads[i].EditedAt = &editedAt
			updatedThreadList = true
		}
	}

	if m.currentThread != nil && m.currentThread.ID == messageID {
		m.currentThread.Content = newContent
		m.currentThread.EditedAt = &editedAt
	}

	for i := range m.threadReplies {
		if m.threadReplies[i].ID == messageID {
			m.threadReplies[i].Content = newContent
			m.threadReplies[i].EditedAt = &editedAt
		}
	}

	if updatedThreadList {
		m.threadListViewport.SetContent(m.buildThreadListContent())
	}
	if m.currentView == ViewThreadView {
		m.threadViewport.SetContent(m.buildThreadContent())
	}
}

func (m Model) sendSetNickname() tea.Cmd {
	return m.sendSetNicknameWith(m.nickname)
}

// NicknameSentMsg is sent after we send a nickname change request
type NicknameSentMsg struct {
	Nickname    string
	GoAnonymous bool // If true, clear userID and authState when nickname changes
}

func (m Model) sendSetNicknameWith(nickname string) tea.Cmd {
	return m.sendSetNicknameWithAnonymous(nickname, false)
}

func (m Model) sendSetNicknameWithAnonymous(nickname string, goAnonymous bool) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.SetNicknameMessage{
			Nickname: nickname,
		}
		if err := m.conn.SendMessage(protocol.TypeSetNickname, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		// Return a message with the nickname we just sent
		return NicknameSentMsg{
			Nickname:    nickname,
			GoAnonymous: goAnonymous,
		}
	}
}

func (m Model) sendLogout() tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.LogoutMessage{}
		if err := m.conn.SendMessage(protocol.TypeLogout, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendAuthRequest(password []byte) tea.Cmd {
	return func() tea.Msg {
		targetNickname := m.authTargetNickname
		if targetNickname == "" {
			targetNickname = m.pendingNickname
		}
		if targetNickname == "" {
			targetNickname = m.nickname
		}

		// Hash password client-side before sending
		passwordHash := auth.HashPassword(string(password), targetNickname)

		msg := &protocol.AuthRequestMessage{
			Nickname: targetNickname,
			Password: passwordHash,
		}
		if err := m.conn.SendMessage(protocol.TypeAuthRequest, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		// Zero password bytes after sending
		for i := range password {
			password[i] = 0
		}
		return nil
	}
}

func (m Model) sendRegisterUser(password []byte) tea.Cmd {
	return func() tea.Msg {
		// Hash password client-side before sending
		passwordHash := auth.HashPassword(string(password), m.nickname)

		msg := &protocol.RegisterUserMessage{
			Password: passwordHash,
		}
		if err := m.conn.SendMessage(protocol.TypeRegisterUser, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		// Zero password bytes after sending
		for i := range password {
			password[i] = 0
		}
		return nil
	}
}

func (m Model) sendGetUserInfo(nickname string) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.GetUserInfoMessage{
			Nickname: nickname,
		}
		if err := m.conn.SendMessage(protocol.TypeGetUserInfo, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendChangePassword(oldPassword, newPassword []byte) tea.Cmd {
	return func() tea.Msg {
		// Hash passwords client-side before sending
		// Note: Empty passwords remain empty (for password removal)
		var oldHash, newHash string
		if len(oldPassword) > 0 {
			oldHash = auth.HashPassword(string(oldPassword), m.nickname)
		}
		if len(newPassword) > 0 {
			newHash = auth.HashPassword(string(newPassword), m.nickname)
		}

		msg := &protocol.ChangePasswordRequest{
			OldPassword: oldHash,
			NewPassword: newHash,
		}
		if err := m.conn.SendMessage(protocol.TypeChangePassword, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) requestChannelList() tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.ListChannelsMessage{
			FromChannelID: 0,
			Limit:         1000,
		}
		if err := m.conn.SendMessage(protocol.TypeListChannels, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) requestSubchannels(channelID uint64) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.GetSubchannelsMessage{
			ChannelID: channelID,
		}
		if err := m.conn.SendMessage(protocol.TypeGetSubchannels, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendCreateSubchannel(parentID uint64, name, description string, channelType uint8) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.CreateSubchannelMessage{
			ChannelID:      parentID,
			Name:           name,
			Description:    description,
			Type:           channelType,
			RetentionHours: 168, // Default to 7 days
		}
		if err := m.conn.SendMessage(protocol.TypeCreateSubchannel, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

// ServerListTimeoutMsg is sent when server list request times out
type ServerListTimeoutMsg struct{}

func (m Model) requestServerList() tea.Cmd {
	// Send the LIST_SERVERS message
	sendCmd := func() tea.Msg {
		msg := &protocol.ListServersMessage{
			Limit: 100, // Request up to 100 servers
		}
		if err := m.conn.SendMessage(protocol.TypeListServers, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}

	// Start a timeout timer (5 seconds)
	timeoutCmd := func() tea.Msg {
		time.Sleep(5 * time.Second)
		return ServerListTimeoutMsg{}
	}

	return tea.Batch(sendCmd, timeoutCmd)
}

func (m Model) sendJoinChannel(channelID uint64) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.JoinChannelMessage{
			ChannelID:    channelID,
			SubchannelID: nil,
		}
		if err := m.conn.SendMessage(protocol.TypeJoinChannel, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendLeaveChannel(channelID uint64, permanent bool) tea.Cmd {
	return func() tea.Msg {
		if channelID == 0 {
			return nil
		}

		// Update read state to "now" before leaving
		// Use UnixMilli() to match database timestamp format (milliseconds)
		now := time.Now().UnixMilli()

		updateMsg := &protocol.UpdateReadStateMessage{
			ChannelID:    channelID,
			SubchannelID: nil,
			Timestamp:    now,
		}
		if err := m.conn.SendMessage(protocol.TypeUpdateReadState, updateMsg); err != nil {
			// Log error but don't fail the leave operation
			if m.logger != nil {
				m.logger.Printf("Failed to update read state: %v", err)
			}
		}

		// Also update local state
		if err := m.state.UpdateReadState(channelID, nil, nil, now); err != nil {
			if m.logger != nil {
				m.logger.Printf("Failed to update local read state: %v", err)
			}
		}

		msg := &protocol.LeaveChannelMessage{
			ChannelID:    channelID,
			SubchannelID: nil,
			Permanent:    permanent,
		}
		if err := m.conn.SendMessage(protocol.TypeLeaveChannel, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendListChannelUsers(channelID uint64) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.ListChannelUsersMessage{
			ChannelID:    channelID,
			SubchannelID: nil,
		}
		if err := m.conn.SendMessage(protocol.TypeListChannelUsers, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendCreateChannel(name, displayName, description string, channelType uint8) tea.Cmd {
	return func() tea.Msg {
		var desc *string
		if description != "" {
			desc = &description
		}

		msg := &protocol.CreateChannelMessage{
			Name:           name,
			DisplayName:    displayName,
			Description:    desc,
			ChannelType:    channelType, // 0=chat, 1=forum
			RetentionHours: 168,         // 7 days default
		}
		if err := m.conn.SendMessage(protocol.TypeCreateChannel, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

// SSH Key Management Functions

func (m Model) sendListSSHKeys() tea.Cmd {
	return func() tea.Msg {
		if m.logger != nil {
			m.logger.Printf("[DEBUG] sendListSSHKeys: Sending LIST_SSH_KEYS request")
		}
		msg := &protocol.ListSSHKeysRequest{}
		if err := m.conn.SendMessage(protocol.TypeListSSHKeys, msg); err != nil {
			if m.logger != nil {
				m.logger.Printf("[DEBUG] sendListSSHKeys: Error sending: %v", err)
			}
			return ErrorMsg{Err: err}
		}
		if m.logger != nil {
			m.logger.Printf("[DEBUG] sendListSSHKeys: Request sent successfully")
		}
		return nil
	}
}

func (m Model) sendAddSSHKey(publicKey, label string) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.AddSSHKeyRequest{
			PublicKey: publicKey,
			Label:     label,
		}
		if err := m.conn.SendMessage(protocol.TypeAddSSHKey, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendUpdateSSHKeyLabel(keyID uint64, newLabel string) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.UpdateSSHKeyLabelRequest{
			KeyID:    int64(keyID),
			NewLabel: newLabel,
		}
		if err := m.conn.SendMessage(protocol.TypeUpdateSSHKeyLabel, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendDeleteSSHKey(keyID uint64) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.DeleteSSHKeyRequest{
			KeyID: int64(keyID),
		}
		if err := m.conn.SendMessage(protocol.TypeDeleteSSHKey, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) requestThreadList(channelID uint64) tea.Cmd {
	return func() tea.Msg {
		limit := uint16(m.height - 6)
		if limit < 10 {
			limit = 10 // Minimum limit
		}
		msg := &protocol.ListMessagesMessage{
			ChannelID:    channelID,
			SubchannelID: nil,
			Limit:        limit,
			BeforeID:     nil,
			ParentID:     nil,
			AfterID:      nil,
		}
		if err := m.conn.SendMessage(protocol.TypeListMessages, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) requestChatMessages(channelID uint64) tea.Cmd {
	return func() tea.Msg {
		limit := uint16(100) // Load last 100 messages initially
		msg := &protocol.ListMessagesMessage{
			ChannelID:    channelID,
			SubchannelID: nil,
			Limit:        limit,
			BeforeID:     nil,
			ParentID:     nil, // No parent ID = root messages only (chat has no threading)
			AfterID:      nil,
		}
		if err := m.conn.SendMessage(protocol.TypeListMessages, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) loadMoreThreads() tea.Cmd {
	return func() tea.Msg {
		if m.currentChannel == nil || len(m.threads) == 0 {
			return nil
		}

		// Get the ID of the oldest thread we have
		oldestThreadID := m.threads[len(m.threads)-1].ID

		limit := uint16(m.height - 6)
		if limit < 10 {
			limit = 10 // Minimum limit
		}
		msg := &protocol.ListMessagesMessage{
			ChannelID:    m.currentChannel.ID,
			SubchannelID: nil,
			Limit:        limit,
			BeforeID:     &oldestThreadID,
			ParentID:     nil,
			AfterID:      nil,
		}
		if err := m.conn.SendMessage(protocol.TypeListMessages, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) requestThreadReplies(threadID uint64) tea.Cmd {
	return func() tea.Msg {
		// Load only enough to fill the screen initially
		// Page is 24 rows high, 3 lines per message = ~8 messages visible
		// Load 10 to have a bit of buffer
		limit := uint16(10)

		msg := &protocol.ListMessagesMessage{
			ChannelID:    m.currentChannel.ID,
			SubchannelID: nil,
			Limit:        limit,
			BeforeID:     nil,
			ParentID:     &threadID,
			AfterID:      nil,
		}
		if err := m.conn.SendMessage(protocol.TypeListMessages, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

// loadMoreReplies loads more replies in the current thread (pagination)
func (m Model) loadMoreReplies() tea.Cmd {
	return func() tea.Msg {
		if m.currentThread == nil || len(m.threadReplies) == 0 {
			return nil
		}

		// Get the ID of the oldest reply we have
		oldestReplyID := m.threadReplies[len(m.threadReplies)-1].ID

		limit := uint16(10)
		msg := &protocol.ListMessagesMessage{
			ChannelID:    m.currentChannel.ID,
			SubchannelID: nil,
			Limit:        limit,
			BeforeID:     &oldestReplyID,
			ParentID:     &m.currentThread.ID,
			AfterID:      nil,
		}
		if err := m.conn.SendMessage(protocol.TypeListMessages, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

// requestThreadRepliesAfter requests only new thread replies after a specific message ID
func (m Model) requestThreadRepliesAfter(threadID uint64, afterID uint64) tea.Cmd {
	return func() tea.Msg {
		// Only fetch new messages, not all 200
		msg := &protocol.ListMessagesMessage{
			ChannelID:    m.currentChannel.ID,
			SubchannelID: nil,
			Limit:        50, // Reasonable limit for new messages
			BeforeID:     nil,
			ParentID:     &threadID,
			AfterID:      &afterID,
		}
		if err := m.conn.SendMessage(protocol.TypeListMessages, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendPostMessage(channelID uint64, parentID *uint64, content string) tea.Cmd {
	return func() tea.Msg {
		messageContent := content

		// Check if this channel has encryption enabled
		if key, ok := m.dmChannelKeys[channelID]; ok {
			encrypted, err := crypto.EncryptMessage(key, []byte(content))
			if err != nil {
				return ErrorMsg{Err: fmt.Errorf("failed to encrypt message: %w", err)}
			}
			// Store encrypted bytes as string (will be binary data)
			messageContent = string(encrypted)
		}

		msg := &protocol.PostMessageMessage{
			ChannelID:    channelID,
			SubchannelID: nil,
			ParentID:     parentID,
			Content:      messageContent,
		}
		if err := m.conn.SendMessage(protocol.TypePostMessage, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendDeleteMessage(messageID uint64) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.DeleteMessageMessage{
			MessageID: messageID,
		}
		if err := m.conn.SendMessage(protocol.TypeDeleteMessage, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendEditMessage(messageID uint64, newContent string) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.EditMessageMessage{
			MessageID:  messageID,
			NewContent: newContent,
		}
		if err := m.conn.SendMessage(protocol.TypeEditMessage, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendPing() tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.PingMessage{
			Timestamp: time.Now().UnixMilli(),
		}
		if err := m.conn.SendMessage(protocol.TypePing, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendSubscribeThread(threadID uint64) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.SubscribeThreadMessage{
			ThreadID: threadID,
		}
		if err := m.conn.SendMessage(protocol.TypeSubscribeThread, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendUnsubscribeThread(threadID uint64) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.UnsubscribeThreadMessage{
			ThreadID: threadID,
		}
		if err := m.conn.SendMessage(protocol.TypeUnsubscribeThread, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendSubscribeChannel(channelID uint64) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.SubscribeChannelMessage{
			ChannelID:    channelID,
			SubchannelID: nil,
		}
		if err := m.conn.SendMessage(protocol.TypeSubscribeChannel, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendUnsubscribeChannel(channelID uint64) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.UnsubscribeChannelMessage{
			ChannelID:    channelID,
			SubchannelID: nil,
		}
		if err := m.conn.SendMessage(protocol.TypeUnsubscribeChannel, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

// markCurrentMessageAsRead removes the "new" indicator from the currently selected message
func (m *Model) markCurrentMessageAsRead() {
	if m.replyCursor == 0 && m.currentThread != nil {
		// Root message selected
		delete(m.newMessageIDs, m.currentThread.ID)
	} else if m.replyCursor > 0 && m.replyCursor-1 < len(m.threadReplies) {
		// Reply message selected
		reply := m.threadReplies[m.replyCursor-1]
		delete(m.newMessageIDs, reply.ID)
	}
}

func isDeletedMessageContent(content string) bool {
	return strings.HasPrefix(content, "[deleted")
}

// handleReconnected handles successful reconnection
func (m Model) handleReconnected() (tea.Model, tea.Cmd) {
	m.connectionState = StateConnected
	m.reconnectAttempt = 0
	m.errorMessage = ""

	// Re-request data based on current view
	cmds := []tea.Cmd{
		listenForServerFrames(m.conn, m.connGeneration),
		m.setStatus("Reconnected successfully"),
	}

	// Re-send nickname if we have one (but not if already authenticated - SSH already set it)
	if m.nickname != "" && m.authState != AuthStateAuthenticated {
		cmds = append(cmds, m.sendSetNickname())
		cmds = append(cmds, m.sendGetUserInfo(m.nickname))
	} else if m.authState == AuthStateAuthenticated {
		// Already authenticated (e.g., SSH), just query user info
		cmds = append(cmds, m.sendGetUserInfo(m.nickname))
	}

	// Re-request channel list
	m.loadingChannels = true
	cmds = append(cmds, m.requestChannelList())

	// If we're in a channel, rejoin and reload threads
	if m.currentChannel != nil {
		m.loadingThreadList = true
		m.threads = []protocol.Message{}                            // Clear threads
		m.threadListViewport.SetContent(m.buildThreadListContent()) // Show initial spinner
		cmds = append(cmds, m.sendJoinChannel(m.currentChannel.ID))
		cmds = append(cmds, m.requestThreadList(m.currentChannel.ID))

		// Re-subscribe to channel if we're in thread list or thread view
		if m.currentView == ViewThreadList || m.currentView == ViewThreadView {
			cmds = append(cmds, m.sendSubscribeChannel(m.currentChannel.ID))
		}

		// If we're viewing a specific thread, reload replies and re-subscribe
		if m.currentThread != nil && m.currentView == ViewThreadView {
			m.loadingThreadReplies = true
			m.threadReplies = []protocol.Message{}              // Clear replies
			m.threadViewport.SetContent(m.buildThreadContent()) // Show initial spinner
			cmds = append(cmds, m.requestThreadReplies(m.currentThread.ID))
			cmds = append(cmds, m.sendSubscribeThread(m.currentThread.ID))
		}
	}

	return m, tea.Batch(cmds...)
}

// handleServerSelected processes server selection from the server selector modal
func (m Model) handleServerSelected(server protocol.ServerInfo) (tea.Model, tea.Cmd) {
	// Store server info for connection
	serverAddr := fmt.Sprintf("%s:%d", server.Hostname, server.Port)

	// Save to state for next startup (using directory-specific key)
	if err := m.state.SetConfig("directory_selected_server", serverAddr); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to save server address: %v", err)
		return m, nil
	}

	// Check if selected server is the same as current connection
	// If so, we can reuse the connection (directory server is also a chat server)
	currentAddr := m.conn.GetAddress()
	if currentAddr == serverAddr {
		if m.logger != nil {
			m.logger.Printf("Selected server is same as directory server, reusing connection")
		}
		// Just update state and proceed with normal chat flow
		m.directoryMode = false
		m.modalStack.Pop()

		// Start normal operation
		cmds := []tea.Cmd{
			m.requestChannelList(),
			m.setStatus(fmt.Sprintf("Connected to %s", server.Name)),
		}

		// Send nickname if we have one (but not if already authenticated)
		if m.nickname != "" && m.authState != AuthStateAuthenticated {
			cmds = append(cmds, m.sendSetNickname())
		}

		m.loadingChannels = true
		return m, tea.Batch(cmds...)
	}

	// Different server - need to reconnect
	if m.logger != nil {
		m.logger.Printf("Switching from %s to %s", currentAddr, serverAddr)
	}

	// Disconnect from directory server
	m.conn.Disconnect()

	// Use helper function to resolve connection method based on history
	address := client.ResolveConnectionMethod(serverAddr, m.state, m.logger)

	// For SSH and WebSocket connections, we may need to strip the port
	// since the connection handler will add the default port
	if strings.HasPrefix(address, "ssh://") || strings.HasPrefix(address, "ws://") || strings.HasPrefix(address, "wss://") {
		// Extract scheme and host
		if idx := strings.Index(address, "://"); idx != -1 {
			scheme := address[:idx+3]
			hostPart := address[idx+3:]

			// Strip port if present for SSH and WebSocket
			if scheme == "ssh://" || scheme == "ws://" || scheme == "wss://" {
				if host, _, err := net.SplitHostPort(hostPart); err == nil {
					// Had a port, use just the host
					address = scheme + host
				}
				// Otherwise keep as-is (no port to strip)
			}
		}
	} else if !strings.Contains(address, "://") {
		// No scheme returned, add default sc:// for TCP
		address = "sc://" + address
	}

	if m.logger != nil {
		m.logger.Printf("Resolved connection address: %s", address)
	}

	// Create connection to new server (returns concrete *Connection type)
	conn, err := client.NewConnection(address)
	if err != nil {
		m.errorMessage = fmt.Sprintf("Failed to create connection: %v", err)
		return m, nil
	}

	// Set logger if we have one (using concrete type methods)
	if m.logger != nil {
		conn.SetLogger(m.logger)
		m.logger.Printf("Connecting to server: %s", conn.GetAddress())
	}

	// Apply bandwidth throttling if requested (using concrete type methods)
	if m.throttle > 0 {
		conn.SetThrottle(m.throttle)
		if m.logger != nil {
			m.logger.Printf("Bandwidth throttling enabled: %d bytes/sec", m.throttle)
		}
	}

	// Connect to server
	if err := conn.Connect(); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to connect to %s: %v", server.Name, err)
		return m, nil
	}

	// Update model state
	m.conn = conn
	m.connectionState = StateConnected
	m.directoryMode = false

	// Save successful connection method for future use
	// Check the actual connection address to distinguish between ws:// and wss://
	connType := conn.GetConnectionType()
	connAddr := conn.GetAddress()
	if connType == "websocket" {
		if strings.HasPrefix(connAddr, "wss://") {
			connType = "wss"
		} else if strings.HasPrefix(connAddr, "ws://") {
			connType = "ws"
		}
	}
	if err := m.state.SaveSuccessfulConnection(serverAddr, connType); err != nil && m.logger != nil {
		m.logger.Printf("Failed to save successful connection method: %v", err)
	}

	// Close the server selector modal
	m.modalStack.Pop()

	// Start normal operation
	cmds := []tea.Cmd{
		listenForServerFrames(m.conn, m.connGeneration),
		m.requestChannelList(),
		m.setStatus(fmt.Sprintf("Connected to %s", server.Name)),
	}

	// Send nickname if we have one (but not if already authenticated)
	if m.nickname != "" && m.authState != AuthStateAuthenticated {
		cmds = append(cmds, m.sendSetNickname())
	}

	m.loadingChannels = true

	return m, tea.Batch(cmds...)
}

// handleCustomServerInput processes custom server address entry
func (m Model) handleCustomServerInput(address string) (tea.Model, tea.Cmd) {
	// Parse the address (add default port if not specified)
	serverAddr := address
	if !strings.Contains(serverAddr, ":") {
		serverAddr = serverAddr + ":6465"
	}

	// Create a temporary ServerInfo for handleServerSelected
	server := protocol.ServerInfo{
		Name:        serverAddr,
		Description: "Custom server",
		Hostname:    strings.Split(serverAddr, ":")[0],
		Port:        6465, // Will be parsed from serverAddr
	}

	// Parse port from address
	if parts := strings.Split(serverAddr, ":"); len(parts) == 2 {
		if port, err := strconv.Atoi(parts[1]); err == nil {
			server.Port = uint16(port)
		}
	}

	// Save to state for next startup
	if err := m.state.SetConfig("directory_selected_server", serverAddr); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to save server address: %v", err)
		return m, nil
	}

	// Reuse the server selection logic
	return m.handleServerSelected(server)
}

// handleConnectionRetry attempts to reconnect to the same server
func (m Model) handleConnectionRetry() (tea.Model, tea.Cmd) {
	// Close the connection failed modal
	m.modalStack.Pop()

	// Try to reconnect
	if err := m.conn.Connect(); err != nil {
		// Connection still failing - show modal again
		m.modalStack.Push(modal.NewConnectionFailedModal(m.conn.GetAddress(), err.Error()))
		m.connectionState = StateDisconnected
		if m.logger != nil {
			m.logger.Printf("Retry connection failed: %v", err)
		}
		return m, nil
	}

	// Connection successful!
	m.connectionState = StateConnected
	if m.logger != nil {
		m.logger.Printf("Retry connection succeeded")
	}

	// Save successful connection method for future use
	connType := m.conn.GetConnectionType()
	connAddr := m.conn.GetAddress()
	if connType == "websocket" {
		if strings.HasPrefix(connAddr, "wss://") {
			connType = "wss"
		} else if strings.HasPrefix(connAddr, "ws://") {
			connType = "ws"
		}
	}
	serverAddr := m.conn.GetRawAddress()
	if err := m.state.SaveSuccessfulConnection(serverAddr, connType); err != nil && m.logger != nil {
		m.logger.Printf("Failed to save successful connection method: %v", err)
	}

	// Start normal operation
	cmds := []tea.Cmd{
		listenForServerFrames(m.conn, m.connGeneration),
		m.requestChannelList(),
		m.setStatus("Connected successfully"),
	}

	// Send nickname if we have one (but not if already authenticated)
	if m.nickname != "" && m.authState != AuthStateAuthenticated {
		cmds = append(cmds, m.sendSetNickname())
	}

	m.loadingChannels = true

	return m, tea.Batch(cmds...)
}

// handleSwitchToServerSelector switches to the server selector modal
func (m Model) handleSwitchToServerSelector() (tea.Model, tea.Cmd) {
	// Close the connection failed modal
	m.modalStack.Pop()

	// Show server selector loading modal
	// Not first launch when switching from connection failed modal
	connType := m.conn.GetConnectionType()
	m.modalStack.Push(modal.NewServerSelectorLoading(false, connType))
	m.awaitingServerList = true
	m.directoryMode = true

	if m.logger != nil {
		m.logger.Printf("Switching to server selector")
	}

	// Request server list
	return m, m.requestServerList()
}

// handleTryDifferentMethod shows the connection method selection modal
func (m Model) handleTryDifferentMethod() (tea.Model, tea.Cmd) {
	// Close the connection failed modal
	m.modalStack.Pop()

	// Determine what method failed
	connType := m.conn.GetConnectionType()
	var failedMethod modal.ConnectionMethod
	switch connType {
	case "tcp":
		failedMethod = modal.MethodTCP
	case "ssh":
		failedMethod = modal.MethodSSH
	case "websocket":
		failedMethod = modal.MethodWebSocket
	default:
		// Unknown method, default to TCP
		failedMethod = modal.MethodTCP
	}

	// Show connection method modal
	serverAddr := m.conn.GetAddress()
	methodModal := modal.NewConnectionMethodModal(serverAddr, failedMethod, "Connection failed")
	m.modalStack.Push(methodModal)

	if m.logger != nil {
		m.logger.Printf("Showing connection method modal (failed method: %s)", failedMethod)
	}

	return m, nil
}

// handleConnectionMethodSelected attempts connection with the user-selected method
func (m Model) handleConnectionMethodSelected(msg modal.ConnectionMethodSelectedMsg) (tea.Model, tea.Cmd) {
	// Close the connection method modal
	m.modalStack.Pop()

	// Set flag to prevent showing "CONNECTION LOST" modal from old connection cleanup
	m.switchingMethod = true

	method := string(msg.Method)
	rawAddr := m.conn.GetRawAddress() // Get raw address without scheme

	if m.logger != nil {
		m.logger.Printf("Attempting connection with method: %s to %s", method, rawAddr)
	}

	// Show connecting modal while attempting connection
	connectingModal := modal.NewConnectingModal(method, rawAddr)
	m.modalStack.Push(connectingModal)

	// Construct the appropriate URL scheme with the raw address
	// For SSH and WebSocket, strip the port and let the connection handler add the default port
	var address string
	switch msg.Method {
	case modal.MethodTCP:
		address = "sc://" + rawAddr
	case modal.MethodSSH:
		// Strip port from rawAddr for SSH (it will use default 6466)
		host, _, err := net.SplitHostPort(rawAddr)
		if err != nil {
			// No port in rawAddr, use as-is
			address = "ssh://" + rawAddr
		} else {
			// Had a port, use just the host and let SSH add default port
			address = "ssh://" + host
		}
	case modal.MethodWebSocket:
		// Strip port from rawAddr for WebSocket (it will use default 6467)
		host, _, err := net.SplitHostPort(rawAddr)
		if err != nil {
			// No port in rawAddr, use as-is
			address = "ws://" + rawAddr
		} else {
			// Had a port, use just the host and let WebSocket add default port
			address = "ws://" + host
		}
	}

	// Create new connection with the selected method
	conn, err := client.NewConnection(address)
	if err != nil {
		// Failed to parse address - show error immediately
		m.modalStack.RemoveByType(modal.ModalConnecting)
		m.modalStack.Push(modal.NewConnectionFailedModal(rawAddr, fmt.Sprintf("Invalid address for %s: %v", method, err)))
		return m, nil
	}

	// Set logger on the new connection
	if m.logger != nil {
		conn.SetLogger(m.logger)
	}

	// Close the old connection before replacing it
	// This prevents stray DisconnectedMsg from the old connection
	if m.conn != nil {
		if m.logger != nil {
			m.logger.Printf("DEBUG: Closing old connection (current generation: %d)", m.connGeneration)
		}
		m.conn.Close()
	}

	// Increment generation counter to ignore messages from old connection
	m.connGeneration++
	if m.logger != nil {
		m.logger.Printf("DEBUG: Incremented generation counter to %d", m.connGeneration)
	}

	// Replace our connection
	m.conn = conn

	// Attempt connection asynchronously and start spinner
	return m, tea.Batch(
		connectingModal.Init(),      // Start the spinner ticking
		m.attemptConnection(method), // Start connection attempt
	)
}

// attemptConnection performs the connection attempt in a goroutine
func (m Model) attemptConnection(method string) tea.Cmd {
	return func() tea.Msg {
		err := m.conn.Connect()
		return ConnectionAttemptResultMsg{
			Success: err == nil,
			Method:  method,
			Error:   err,
		}
	}
}

// handleConnectionAttemptResult processes the result of an async connection attempt
func (m Model) handleConnectionAttemptResult(msg ConnectionAttemptResultMsg) (tea.Model, tea.Cmd) {
	// Remove the connecting modal
	m.modalStack.RemoveByType(modal.ModalConnecting)

	if !msg.Success {
		// Connection failed - show error modal
		m.modalStack.Push(modal.NewConnectionFailedModal(m.conn.GetAddress(), msg.Error.Error()))
		m.connectionState = StateDisconnected
		// Keep switchingMethod=true to prevent overlay flash
		if m.logger != nil {
			m.logger.Printf("Connection with method %s failed: %v", msg.Method, msg.Error)
		}
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	// Connection successful!
	m.switchingMethod = false // Clear flag on success
	m.connectionState = StateConnected
	if m.logger != nil {
		m.logger.Printf("Connection with method %s succeeded", msg.Method)
	}

	// Save successful connection method for future use
	if err := m.state.SaveSuccessfulConnection(m.conn.GetRawAddress(), msg.Method); err != nil {
		if m.logger != nil {
			m.logger.Printf("Failed to save successful connection method: %v", err)
		}
	}

	// Start normal operation
	cmds := []tea.Cmd{
		listenForServerFrames(m.conn, m.connGeneration),
		m.requestChannelList(),
		m.setStatus(fmt.Sprintf("Connected via %s", msg.Method)),
	}

	// Send nickname if we have one (but not if already authenticated)
	if m.nickname != "" && m.authState != AuthStateAuthenticated {
		cmds = append(cmds, m.sendSetNickname())
	}

	m.loadingChannels = true

	return m, tea.Batch(cmds...)
}

// SSH Key Management Handlers

func (m Model) handleSSHKeyList(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.SSHKeyListResponse{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode SSH_KEY_LIST: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	// Convert to modal.SSHKeyInfo format
	keys := make([]modal.SSHKeyInfo, len(msg.Keys))
	for i, key := range msg.Keys {
		keys[i] = modal.SSHKeyInfo{
			ID:          uint64(key.ID),
			Fingerprint: key.Fingerprint,
			KeyType:     key.KeyType,
			Label:       key.Label,
			AddedAt:     time.UnixMilli(key.AddedAt),
			LastUsedAt:  nil,
		}
		if key.LastUsedAt > 0 {
			t := time.UnixMilli(key.LastUsedAt)
			keys[i].LastUsedAt = &t
		}
	}

	// Show SSH key manager modal
	m.showSSHKeyManagerModal(keys)

	return m, listenForServerFrames(m.conn, m.connGeneration)
}

func (m Model) handleSSHKeyAdded(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.SSHKeyAddedResponse{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode SSH_KEY_ADDED: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	if msg.Success {
		statusCmd := m.setStatus(fmt.Sprintf("SSH key added: %s", msg.Fingerprint))
		// Refresh the key list
		return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), m.sendListSSHKeys(), statusCmd)
	}

	m.errorMessage = msg.ErrorMessage
	return m, listenForServerFrames(m.conn, m.connGeneration)
}

func (m Model) handleSSHKeyLabelUpdated(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.SSHKeyLabelUpdatedResponse{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode SSH_KEY_LABEL_UPDATED: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	if msg.Success {
		statusCmd := m.setStatus("SSH key label updated")
		// Refresh the key list
		return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), m.sendListSSHKeys(), statusCmd)
	}

	m.errorMessage = msg.ErrorMessage
	return m, listenForServerFrames(m.conn, m.connGeneration)
}

func (m Model) handleSSHKeyDeleted(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.SSHKeyDeletedResponse{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode SSH_KEY_DELETED: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	if msg.Success {
		statusCmd := m.setStatus("SSH key deleted")
		// Refresh the key list
		return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), m.sendListSSHKeys(), statusCmd)
	} else {
		m.errorMessage = msg.ErrorMessage
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}
}

// Admin Functions

func (m Model) sendBanUser(msg *protocol.BanUserMessage) tea.Cmd {
	return func() tea.Msg {
		if err := m.conn.SendMessage(protocol.TypeBanUser, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendBanIP(msg *protocol.BanIPMessage) tea.Cmd {
	return func() tea.Msg {
		if err := m.conn.SendMessage(protocol.TypeBanIP, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendUnbanUser(msg *protocol.UnbanUserMessage) tea.Cmd {
	return func() tea.Msg {
		if err := m.conn.SendMessage(protocol.TypeUnbanUser, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendUnbanIP(msg *protocol.UnbanIPMessage) tea.Cmd {
	return func() tea.Msg {
		if err := m.conn.SendMessage(protocol.TypeUnbanIP, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendDeleteUser(msg *protocol.DeleteUserMessage) tea.Cmd {
	return func() tea.Msg {
		if err := m.conn.SendMessage(protocol.TypeDeleteUser, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendDeleteChannel(msg *protocol.DeleteChannelMessage) tea.Cmd {
	return func() tea.Msg {
		if err := m.conn.SendMessage(protocol.TypeDeleteChannel, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendListBans(includeExpired bool) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.ListBansMessage{
			IncludeExpired: includeExpired,
		}
		if err := m.conn.SendMessage(protocol.TypeListBans, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendListUsers(includeOffline bool) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.ListUsersMessage{
			Limit:          500, // Max limit
			IncludeOffline: includeOffline,
		}
		if err := m.conn.SendMessage(protocol.TypeListUsers, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

// DM send commands

func (m Model) sendStartDM(targetType uint8, targetUserID uint64, targetNickname string, allowUnencrypted bool) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.StartDMMessage{
			TargetType:       targetType,
			TargetUserID:     targetUserID,
			TargetNickname:   targetNickname,
			AllowUnencrypted: allowUnencrypted,
		}
		if err := m.conn.SendMessage(protocol.TypeStartDM, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendProvidePublicKey(keyType uint8, publicKey [32]byte, label string) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.ProvidePublicKeyMessage{
			KeyType:   keyType,
			PublicKey: publicKey,
			Label:     label,
		}
		if err := m.conn.SendMessage(protocol.TypeProvidePublicKey, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendAllowUnencrypted(dmChannelID uint64, permanent bool) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.AllowUnencryptedMessage{
			DMChannelID: dmChannelID,
			Permanent:   permanent,
		}
		if err := m.conn.SendMessage(protocol.TypeAllowUnencrypted, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) sendDeclineDM(dmChannelID uint64) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.DeclineDMMessage{
			DMChannelID: dmChannelID,
		}
		if err := m.conn.SendMessage(protocol.TypeDeclineDM, msg); err != nil {
			return ErrorMsg{Err: err}
		}
		// Return a local message to remove from pending list
		return DMDeclinedLocalMsg{ChannelID: dmChannelID}
	}
}

// DMDeclinedLocalMsg is sent when we decline a DM (local action, before server confirms)
type DMDeclinedLocalMsg struct {
	ChannelID uint64
}

// Admin response handlers

func (m Model) handleUserBanned(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.UserBannedMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.statusMessage = "" // Clear in-progress status
		m.errorMessage = fmt.Sprintf("Failed to decode USER_BANNED: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	var statusCmd tea.Cmd
	if msg.Success {
		statusCmd = m.setStatus(msg.Message)
		// Close the ban user modal
		m.modalStack.RemoveByType(modal.ModalBanUser)
	} else {
		m.statusMessage = "" // Clear in-progress status
		m.errorMessage = msg.Message
	}

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

func (m Model) handleIPBanned(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.IPBannedMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.statusMessage = "" // Clear in-progress status
		m.errorMessage = fmt.Sprintf("Failed to decode IP_BANNED: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	var statusCmd tea.Cmd
	if msg.Success {
		statusCmd = m.setStatus(msg.Message)
		// Close the ban IP modal
		m.modalStack.RemoveByType(modal.ModalBanIP)
	} else {
		m.statusMessage = "" // Clear in-progress status
		m.errorMessage = msg.Message
	}

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

func (m Model) handleUserUnbanned(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.UserUnbannedMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.statusMessage = "" // Clear in-progress status
		m.errorMessage = fmt.Sprintf("Failed to decode USER_UNBANNED: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	var statusCmd tea.Cmd
	if msg.Success {
		statusCmd = m.setStatus(msg.Message)
		// Close the unban modal
		m.modalStack.RemoveByType(modal.ModalUnban)
	} else {
		m.statusMessage = "" // Clear in-progress status
		m.errorMessage = msg.Message
	}

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

func (m Model) handleIPUnbanned(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.IPUnbannedMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.statusMessage = "" // Clear in-progress status
		m.errorMessage = fmt.Sprintf("Failed to decode IP_UNBANNED: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	var statusCmd tea.Cmd
	if msg.Success {
		statusCmd = m.setStatus(msg.Message)
		// Close the unban modal
		m.modalStack.RemoveByType(modal.ModalUnban)
	} else {
		m.statusMessage = "" // Clear in-progress status
		m.errorMessage = msg.Message
	}

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

func (m Model) handleBanList(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.BanListMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode BAN_LIST: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	// Find and update the view bans modal directly on the stack
	topModal := m.modalStack.Top()
	if topModal != nil && topModal.Type() == modal.ModalViewBans {
		if viewBansModal, ok := topModal.(*modal.ViewBansModal); ok {
			// Convert protocol bans to modal.BanEntry
			banEntries := make([]modal.BanEntry, len(msg.Bans))
			for i, ban := range msg.Bans {
				banEntries[i] = modal.BanEntry{
					BanType:     ban.Type,
					TargetID:    ban.UserID,
					Nickname:    ban.Nickname,
					IPCIDR:      ban.IPCIDR,
					Reason:      ban.Reason,
					BannedAt:    ban.BannedAt,
					BannedUntil: ban.BannedUntil,
					BannedBy:    ban.BannedBy,
					IsShadowban: ban.Shadowban,
				}
			}
			viewBansModal.SetBans(banEntries)
		}
	}

	return m, listenForServerFrames(m.conn, m.connGeneration)
}

func (m Model) handleUserList(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.UserListMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode USER_LIST: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	if m.logger != nil {
		m.logger.Printf("[DEBUG] Received USER_LIST with %d users", len(msg.Users))
	}

	updatedDirectory := make(map[string]uint64, len(msg.Users))
	for _, user := range msg.Users {
		if user.UserID != nil && user.Nickname != "" {
			updatedDirectory[strings.ToLower(user.Nickname)] = *user.UserID
		}
	}
	m.userDirectory = updatedDirectory

	// Find and update the list users modal directly on the stack
	topModal := m.modalStack.Top()
	if topModal != nil && topModal.Type() == modal.ModalListUsers {
		if listUsersModal, ok := topModal.(*modal.ListUsersModal); ok {
			// Convert protocol user entries to modal.UserEntry
			userEntries := make([]modal.UserEntry, len(msg.Users))
			for i, user := range msg.Users {
				userEntries[i] = modal.UserEntry{
					Nickname:     user.Nickname,
					IsRegistered: user.IsRegistered,
					UserID:       user.UserID,
					Online:       user.Online,
				}
			}
			listUsersModal.SetUsers(userEntries)
			if m.logger != nil {
				m.logger.Printf("[DEBUG] Updated ListUsersModal with %d users", len(userEntries))
			}
		}
	} else if m.logger != nil {
		m.logger.Printf("[DEBUG] ListUsersModal not on top of stack (top=%v)", topModal)
	}

	return m, listenForServerFrames(m.conn, m.connGeneration)
}

func (m Model) handleChannelUserList(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.ChannelUserListMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode CHANNEL_USER_LIST: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	roster := make(map[uint64]presenceEntry, len(msg.Users))
	for _, user := range msg.Users {
		entry := presenceEntry{
			SessionID:    user.SessionID,
			Nickname:     user.Nickname,
			IsRegistered: user.IsRegistered,
			UserID:       cloneUint64Ptr(user.UserID),
			UserFlags:    user.UserFlags,
		}
		roster[user.SessionID] = entry
		if entry.Nickname == m.nickname {
			id := entry.SessionID
			m.selfSessionID = &id
		}
	}
	if len(roster) == 0 {
		delete(m.channelRoster, msg.ChannelID)
	} else {
		m.channelRoster[msg.ChannelID] = roster
	}

	cmd := tea.Batch(
		listenForServerFrames(m.conn, m.connGeneration),
		func() tea.Msg { return ForceRenderMsg{} },
	)
	return m, cmd
}

func (m Model) handleChannelPresence(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.ChannelPresenceMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode CHANNEL_PRESENCE: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	entry := presenceEntry{
		SessionID:    msg.SessionID,
		Nickname:     msg.Nickname,
		IsRegistered: msg.IsRegistered,
		UserID:       cloneUint64Ptr(msg.UserID),
		UserFlags:    msg.UserFlags,
	}

	// Track presence for all users
	if msg.Joined {
		m.upsertChannelPresence(msg.ChannelID, entry)
	} else {
		m.removeChannelPresence(msg.ChannelID, msg.SessionID)
	}

	// Only adjust channel user count for OTHER users (not ourselves)
	// We handle our own count adjustments in setActiveChannel/clearActiveChannel
	isSelf := m.selfSessionID != nil && *m.selfSessionID == msg.SessionID
	if !isSelf {
		if msg.Joined {
			m.adjustChannelUserCount(msg.ChannelID, 1)
		} else {
			m.adjustChannelUserCount(msg.ChannelID, -1)
		}
	}

	var statusCmd tea.Cmd
	if m.hasActiveChannel && m.activeChannelID == msg.ChannelID && m.currentChannel != nil {
		action := "left"
		if msg.Joined {
			action = "joined"
		}
		statusCmd = m.setStatus(fmt.Sprintf("%s has %s %s", entry.Nickname, action, m.currentChannel.Name))
	}

	cmd := tea.Batch(
		listenForServerFrames(m.conn, m.connGeneration),
		func() tea.Msg { return ForceRenderMsg{} },
		statusCmd,
	)
	return m, cmd
}

func (m Model) handleServerPresence(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.ServerPresenceMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode SERVER_PRESENCE: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	entry := presenceEntry{
		SessionID:    msg.SessionID,
		Nickname:     msg.Nickname,
		IsRegistered: msg.IsRegistered,
		UserID:       cloneUint64Ptr(msg.UserID),
		UserFlags:    msg.UserFlags,
	}

	if msg.Online {
		m.upsertServerPresence(entry)
	} else {
		m.removeServerPresence(msg.SessionID)
	}

	cmd := tea.Batch(
		listenForServerFrames(m.conn, m.connGeneration),
		func() tea.Msg { return ForceRenderMsg{} },
	)
	return m, cmd
}

// handleUnreadCounts processes UNREAD_COUNTS (0x97)
func (m Model) handleUnreadCounts(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.UnreadCountsMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode UNREAD_COUNTS: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	// Update unread counts map
	for _, count := range msg.Counts {
		// For now, only handle channel-level counts (no subchannel or thread)
		if count.SubchannelID == nil && count.ThreadID == nil {
			m.unreadCounts[count.ChannelID] = count.UnreadCount
		}
	}

	return m, listenForServerFrames(m.conn, m.connGeneration)
}

func (m Model) handleUserDeleted(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.UserDeletedMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.statusMessage = "" // Clear in-progress status
		m.errorMessage = fmt.Sprintf("Failed to decode USER_DELETED: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	cmds := []tea.Cmd{listenForServerFrames(m.conn, m.connGeneration)}

	if msg.Success {
		cmds = append(cmds, m.setStatus(msg.Message))
		// Close the delete user modal
		m.modalStack.RemoveByType(modal.ModalDeleteUser)
		cmds = append(cmds, m.sendListUsers(true))
	} else {
		m.statusMessage = "" // Clear in-progress status
		m.errorMessage = msg.Message
	}

	return m, tea.Batch(cmds...)
}

// DM response handlers

func (m Model) handleKeyRequired(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.KeyRequiredMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode KEY_REQUIRED: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	if m.logger != nil {
		m.logger.Printf("[DM] KEY_REQUIRED received: %s (channel: %v)", msg.Reason, msg.DMChannelID)
	}

	// Server is asking us to provide an encryption key
	// Show the encryption setup modal
	m.showEncryptionSetupModal(msg.DMChannelID, msg.Reason)

	return m, listenForServerFrames(m.conn, m.connGeneration)
}

func (m Model) handleDMReady(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.DMReadyMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode DM_READY: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	if m.logger != nil {
		m.logger.Printf("[DM] DM_READY: channel=%d, other=%s, encrypted=%v",
			msg.ChannelID, msg.OtherNickname, msg.IsEncrypted)
	}

	// Add the DM channel to our list
	dmChannel := DMChannel{
		ChannelID:     msg.ChannelID,
		OtherUserID:   msg.OtherUserID,
		OtherNickname: msg.OtherNickname,
		IsEncrypted:   msg.IsEncrypted,
		UnreadCount:   0,
	}

	if msg.IsEncrypted {
		dmChannel.OtherPubKey = msg.OtherPublicKey[:]

		// Derive the channel encryption key
		if m.encryptionKeyPriv != nil {
			sharedSecret, err := crypto.ComputeSharedSecret(m.encryptionKeyPriv, msg.OtherPublicKey[:])
			if err != nil {
				m.errorMessage = fmt.Sprintf("Failed to compute shared secret: %v", err)
				return m, listenForServerFrames(m.conn, m.connGeneration)
			}

			channelKey, err := crypto.DeriveChannelKey(sharedSecret, msg.ChannelID)
			if err != nil {
				m.errorMessage = fmt.Sprintf("Failed to derive channel key: %v", err)
				return m, listenForServerFrames(m.conn, m.connGeneration)
			}

			m.dmChannelKeys[msg.ChannelID] = channelKey
			if m.logger != nil {
				m.logger.Printf("[DM] Derived channel key for channel %d", msg.ChannelID)
			}
		} else {
			m.errorMessage = "Cannot set up encrypted DM: no encryption key available"
			return m, listenForServerFrames(m.conn, m.connGeneration)
		}
	}

	// Check if this DM already exists, update if so
	found := false
	for i, existing := range m.dmChannels {
		if existing.ChannelID == msg.ChannelID {
			m.dmChannels[i] = dmChannel
			found = true
			break
		}
	}
	if !found {
		m.dmChannels = append(m.dmChannels, dmChannel)
	}

	// Remove any pending invite from this user (incoming)
	// Note: We match by nickname because the invite's "ChannelID" is actually the invite ID,
	// not the channel ID that DM_READY returns
	for i, invite := range m.pendingDMInvites {
		if invite.FromNickname == msg.OtherNickname {
			m.pendingDMInvites = append(m.pendingDMInvites[:i], m.pendingDMInvites[i+1:]...)
			break
		}
	}

	// Remove any outgoing invite to this user
	for i, invite := range m.outgoingDMInvites {
		if invite.ToNickname == msg.OtherNickname {
			m.outgoingDMInvites = append(m.outgoingDMInvites[:i], m.outgoingDMInvites[i+1:]...)
			break
		}
	}

	statusCmd := m.setStatus(fmt.Sprintf("DM with %s is ready", msg.OtherNickname))

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

func (m Model) handleDMPending(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.DMPendingMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode DM_PENDING: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	if m.logger != nil {
		m.logger.Printf("[DM] DM_PENDING: channel=%d, waiting for %s: %s",
			msg.DMChannelID, msg.WaitingForNickname, msg.Reason)
	}

	// Add to outgoing invites list (avoid duplicates)
	found := false
	for i, existing := range m.outgoingDMInvites {
		if existing.InviteID == msg.DMChannelID {
			m.outgoingDMInvites[i].ToNickname = msg.WaitingForNickname
			m.outgoingDMInvites[i].ToUserID = msg.WaitingForUserID
			found = true
			break
		}
	}
	if !found {
		m.outgoingDMInvites = append(m.outgoingDMInvites, OutgoingDMInvite{
			InviteID:   msg.DMChannelID,
			ToUserID:   msg.WaitingForUserID,
			ToNickname: msg.WaitingForNickname,
		})
	}

	// Show status that we're waiting for the other user
	statusCmd := m.setStatus(fmt.Sprintf("Waiting for %s to accept...", msg.WaitingForNickname))

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

func (m Model) handleDMRequest(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.DMRequestMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode DM_REQUEST: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	if m.logger != nil {
		m.logger.Printf("[DM] DM_REQUEST: channel=%d, from=%s, encryptionStatus=%d",
			msg.DMChannelID, msg.FromNickname, msg.EncryptionStatus)
	}

	// Add to pending invites
	invite := DMInvite{
		ChannelID:        msg.DMChannelID,
		FromUserID:       msg.FromUserID,
		FromNickname:     msg.FromNickname,
		EncryptionStatus: msg.EncryptionStatus,
	}

	// Check if invite already exists
	found := false
	for i, existing := range m.pendingDMInvites {
		if existing.ChannelID == msg.DMChannelID {
			m.pendingDMInvites[i] = invite
			found = true
			break
		}
	}
	if !found {
		m.pendingDMInvites = append(m.pendingDMInvites, invite)
	}

	// Show the DM request modal
	dmModal := modal.NewDMRequestModal(modal.DMRequestModalConfig{
		ChannelID:        msg.DMChannelID,
		FromNickname:     msg.FromNickname,
		FromUserID:       msg.FromUserID,
		EncryptionStatus: msg.EncryptionStatus,
		OnAccept: func(channelID uint64, allowUnencrypted bool) tea.Cmd {
			if allowUnencrypted {
				return m.sendAllowUnencrypted(channelID, false)
			}
			// If not allowing unencrypted, we need to provide a key
			// For now, just allow unencrypted as fallback
			return m.sendAllowUnencrypted(channelID, false)
		},
		OnDecline: func(channelID uint64) tea.Cmd {
			return func() tea.Msg {
				return DMDeclinedMsg{ChannelID: channelID}
			}
		},
		OnSetupEncryption: func(channelID uint64) tea.Cmd {
			// TODO: Show encryption setup modal
			return nil
		},
	})
	m.modalStack.Push(dmModal)

	return m, listenForServerFrames(m.conn, m.connGeneration)
}

func (m Model) handleDMParticipantLeft(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.DMParticipantLeftMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode DM_PARTICIPANT_LEFT: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	if m.logger != nil {
		m.logger.Printf("[DM] Participant left: channel=%d, nickname=%s", msg.DMChannelID, msg.Nickname)
	}

	// Mark the DM channel as having lost its participant
	for i, dm := range m.dmChannels {
		if dm.ChannelID == msg.DMChannelID {
			m.dmChannels[i].ParticipantLeft = true
			break
		}
	}

	// If currently viewing this DM channel, add a system message to the chat
	if m.currentChannel != nil && m.currentChannel.ID == msg.DMChannelID {
		// Add a synthetic system message
		systemMsg := protocol.Message{
			ID:             0, // System messages use ID 0
			ChannelID:      msg.DMChannelID,
			AuthorNickname: "", // Empty author indicates system message
			Content:        fmt.Sprintf("⚠️ %s has left the conversation", msg.Nickname),
			CreatedAt:      time.Now(),
		}
		m.chatMessages = append(m.chatMessages, systemMsg)
		// Rebuild viewport content and scroll to show new message
		m.chatViewport.SetContent(m.buildChatMessages())
		m.chatViewport.GotoBottom()
	}

	// Show a status message
	statusCmd := m.setStatus(fmt.Sprintf("%s has left the DM conversation", msg.Nickname))

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

func (m Model) handleDMDeclined(frame *protocol.Frame) (tea.Model, tea.Cmd) {
	msg := &protocol.DMDeclinedMessage{}
	if err := msg.Decode(frame.Payload); err != nil {
		m.errorMessage = fmt.Sprintf("Failed to decode DM_DECLINED: %v", err)
		return m, listenForServerFrames(m.conn, m.connGeneration)
	}

	if m.logger != nil {
		m.logger.Printf("[DM] DM_DECLINED: channel=%d, nickname=%s", msg.DMChannelID, msg.Nickname)
	}

	// Remove from outgoing invites list
	for i, invite := range m.outgoingDMInvites {
		if invite.InviteID == msg.DMChannelID {
			m.outgoingDMInvites = append(m.outgoingDMInvites[:i], m.outgoingDMInvites[i+1:]...)
			break
		}
	}

	// Show a status message with timeout
	statusCmd := m.setStatus(fmt.Sprintf("%s declined your DM request", msg.Nickname))

	return m, tea.Batch(listenForServerFrames(m.conn, m.connGeneration), statusCmd)
}

// showEncryptionSetupModal displays the modal for setting up encryption
func (m *Model) showEncryptionSetupModal(dmChannelID *uint64, reason string) {
	// Check if user has SSH key (authenticated via SSH)
	hasSSHKey := m.userID != nil && m.authState == AuthStateAuthenticated

	// Check if we already have an encryption key
	hasExistingKey := m.encryptionKeyPub != nil

	encModal := modal.NewEncryptionSetupModal(modal.EncryptionSetupConfig{
		DMChannelID:    dmChannelID,
		Reason:         reason,
		HasSSHKey:      hasSSHKey,
		HasExistingKey: hasExistingKey,
		CanSkip:        true, // For now, always allow skipping
		OnGenerate: func() tea.Cmd {
			return m.generateAndSendEncryptionKey(dmChannelID)
		},
		OnUseSSH: func() tea.Cmd {
			return m.deriveAndSendEncryptionKeyFromSSH(dmChannelID)
		},
		OnSkip: func() tea.Cmd {
			if dmChannelID != nil {
				return m.sendAllowUnencrypted(*dmChannelID, false)
			}
			return nil
		},
		OnCancel: func() tea.Cmd {
			return nil
		},
	})
	m.modalStack.Push(encModal)
}

// generateAndSendEncryptionKey generates a new X25519 key pair and sends the public key to the server
func (m *Model) generateAndSendEncryptionKey(dmChannelID *uint64) tea.Cmd {
	return func() tea.Msg {
		// Generate new key pair
		kp, err := crypto.GenerateX25519KeyPair()
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to generate encryption key: %w", err)}
		}

		// Determine key type based on user registration status
		// Anonymous users can only use ephemeral (session-only) keys
		var keyType uint8 = protocol.KeyTypeGenerated
		label := "generated"
		if m.userID == nil {
			keyType = protocol.KeyTypeEphemeral
			label = "ephemeral"
		}

		msg := &protocol.ProvidePublicKeyMessage{
			KeyType:   keyType,
			PublicKey: kp.PublicKey,
			Label:     label,
		}
		if err := m.conn.SendMessage(protocol.TypeProvidePublicKey, msg); err != nil {
			return ErrorMsg{Err: err}
		}

		return EncryptionKeyGeneratedMsg{
			PublicKey:  kp.PublicKey,
			PrivateKey: kp.PrivateKey,
		}
	}
}

// deriveAndSendEncryptionKeyFromSSH derives an X25519 key from the user's SSH key
func (m *Model) deriveAndSendEncryptionKeyFromSSH(dmChannelID *uint64) tea.Cmd {
	return func() tea.Msg {
		// TODO: Access SSH private key and derive X25519 key
		// This requires access to the SSH agent or key file
		// For now, fall back to generating a new key
		return ErrorMsg{Err: fmt.Errorf("SSH key derivation not yet implemented")}
	}
}

// EncryptionKeyGeneratedMsg is sent when a new encryption key is generated
type EncryptionKeyGeneratedMsg struct {
	PublicKey  [32]byte
	PrivateKey [32]byte
}

// DMDeclinedMsg is sent when user declines a DM request
type DMDeclinedMsg struct {
	ChannelID uint64
}

// shouldNotifyForMessage checks if we should send a desktop notification for this message
func (m Model) shouldNotifyForMessage(msg protocol.Message) bool {
	// Don't notify for our own messages
	if m.isOwnMessage(msg) {
		return false
	}

	// Check if user has been idle for 5+ minutes
	idleTime := time.Since(m.lastInteractionTime)
	if idleTime < 5*time.Minute {
		return false
	}

	return true
}

// sendDesktopNotification sends a desktop notification for a message
func (m Model) sendDesktopNotification(msg protocol.Message) {
	// Build notification title and message
	title := "SuperChat"
	if m.currentChannel != nil {
		title = fmt.Sprintf("SuperChat - %s", m.currentChannel.Name)
	}

	// Truncate message content to 100 chars for notification
	content := msg.Content
	if len(content) > 100 {
		content = content[:97] + "..."
	}

	body := fmt.Sprintf("%s: %s", msg.AuthorNickname, content)

	// Send notification (best-effort, don't fail if it doesn't work)
	err := beeep.Notify(title, body, m.notificationIconPath)
	if err != nil && m.logger != nil {
		m.logger.Printf("Failed to send desktop notification: %v", err)
	}
}
