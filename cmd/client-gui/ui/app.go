// ABOUTME: Main UI application state and layout for Gio GUI client
// ABOUTME: Handles connection management and top-level rendering
package ui

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/aeolun/superchat/pkg/client"
	"github.com/aeolun/superchat/pkg/client/commands"
	"github.com/aeolun/superchat/pkg/protocol"
)

// ComposeMode indicates what we're composing
type ComposeMode int

const (
	ComposeModeNewThread ComposeMode = iota
	ComposeModeReply
)

// ComposeModal allows users to compose messages
type ComposeModal struct {
	mode      ComposeMode
	editor    widget.Editor
	replyTo   *protocol.Message // Set when replying to a message
	submitBtn widget.Clickable
	cancelBtn widget.Clickable
}

// WindowInvalidator is an interface for invalidating the window
type WindowInvalidator interface {
	Invalidate()
}

// App represents the GUI application state
type App struct {
	conn         client.ConnectionInterface
	state        client.StateInterface
	theme        *material.Theme
	version      string
	nickname     string
	onlineUsers  uint32
	showUserList bool
	throttle     int // Bandwidth throttle in bytes/sec
	window       WindowInvalidator // Reference to window for triggering redraws

	// View state
	mainView     commands.ViewID
	composeModal *ComposeModal // Active compose modal (nil if not open)

	// Focus state for keyboard navigation
	channelFocusIndex int // Current focused channel index
	threadFocusIndex  int // Current focused thread index
	replyFocusIndex   int // Current focused reply index (0 = root, 1+ = replies)

	// Channel list state
	channels       []protocol.Channel
	loadingChannels bool
	channelButtons []widget.Clickable
	channelList    widget.List

	// Current channel state
	selectedChannel *protocol.Channel

	// Thread list state (forum channels)
	threads        []protocol.Message
	loadingThreads bool
	threadButtons  []widget.Clickable
	threadList     widget.List
	newThreadBtn   widget.Clickable

	// Thread view state (single thread with replies)
	currentThread      *protocol.Message
	threadReplies      []protocol.Message
	loadingReplies     bool
	threadViewport     widget.List
	replyButtons       []widget.Clickable // One per message (root + replies)

	// Chat state (chat channels)
	chatMessages   []protocol.Message
	loadingChat    bool
	chatList       widget.List
}

// NewApp creates a new GUI application
func NewApp(conn client.ConnectionInterface, state client.StateInterface, theme *material.Theme, version string, throttle int, window WindowInvalidator) *App {
	// Get nickname from state
	nickname := state.GetLastNickname()

	app := &App{
		conn:            conn,
		state:           state,
		theme:           theme,
		version:         version,
		nickname:        nickname,
		loadingChannels: true,
		onlineUsers:     0,
		showUserList:    false,
		throttle:        throttle,
		window:          window,
		mainView:        commands.ViewChannelList,
		composeModal:    nil,
		channelList: widget.List{
			List: layout.List{
				Axis: layout.Vertical,
			},
		},
		threadList: widget.List{
			List: layout.List{
				Axis: layout.Vertical,
			},
		},
		chatList: widget.List{
			List: layout.List{
				Axis: layout.Vertical,
			},
		},
		threadViewport: widget.List{
			List: layout.List{
				Axis: layout.Vertical,
			},
		},
	}

	// Start fetching channels
	go app.fetchChannels()

	// Start listening for server messages
	go app.listenForMessages()

	return app
}

// fetchChannels requests channel list from server
func (a *App) fetchChannels() {
	// Send LIST_CHANNELS request
	msg := &protocol.ListChannelsMessage{
		FromChannelID: 0,
		Limit:         1000,
	}
	if err := a.conn.SendMessage(protocol.TypeListChannels, msg); err != nil {
		log.Printf("Failed to send LIST_CHANNELS: %v", err)
		return
	}
}

// listenForMessages handles incoming server messages
func (a *App) listenForMessages() {
	for frame := range a.conn.Incoming() {
		switch frame.Type {
		case protocol.TypeChannelList:
			resp := &protocol.ChannelListMessage{}
			if err := resp.Decode(frame.Payload); err != nil {
				log.Printf("Failed to decode channel list: %v", err)
				continue
			}
			a.channels = resp.Channels
			a.loadingChannels = false
			log.Printf("Loaded %d channels", len(a.channels))
			// Trigger window redraw
			if a.window != nil {
				a.window.Invalidate()
			}

		case protocol.TypeMessageList:
			resp := &protocol.MessageListMessage{}
			if err := resp.Decode(frame.Payload); err != nil {
				log.Printf("Failed to decode message list: %v", err)
				continue
			}

			// Determine what this response is for based on ParentID
			if resp.ParentID != nil {
				// This is a thread replies response (has a parent)
				// Sort replies in depth-first order for proper threading
				if a.currentThread != nil {
					a.threadReplies = client.SortThreadReplies(resp.Messages, a.currentThread.ID)
				} else {
					a.threadReplies = resp.Messages
				}
				a.loadingReplies = false
				log.Printf("Loaded %d thread replies", len(a.threadReplies))
			} else if a.mainView == commands.ViewChatChannel {
				// Chat messages (no parent, chat channel)
				a.chatMessages = resp.Messages
				a.loadingChat = false
				log.Printf("Loaded %d chat messages", len(a.chatMessages))
			} else {
				// Thread list (no parent, forum channel)
				a.threads = resp.Messages
				a.loadingThreads = false
				log.Printf("Loaded %d threads", len(a.threads))
			}
			// Trigger window redraw
			if a.window != nil {
				a.window.Invalidate()
			}

		default:
			// Ignore unknown messages for now
		}
	}
}

// selectChannel handles channel selection
func (a *App) selectChannel(channel *protocol.Channel) {
	a.selectedChannel = channel

	// Send JOIN_CHANNEL
	joinMsg := &protocol.JoinChannelMessage{ChannelID: channel.ID}
	if err := a.conn.SendMessage(protocol.TypeJoinChannel, joinMsg); err != nil {
		log.Printf("Failed to join channel: %v", err)
		return
	}

	// Send SUBSCRIBE_CHANNEL
	subMsg := &protocol.SubscribeChannelMessage{ChannelID: channel.ID}
	if err := a.conn.SendMessage(protocol.TypeSubscribeChannel, subMsg); err != nil {
		log.Printf("Failed to subscribe to channel: %v", err)
		return
	}

	// Route to appropriate view based on channel type
	if channel.Type == 0 {
		// Chat channel (type 0) - linear chat
		a.mainView = commands.ViewChatChannel
		a.loadingChat = true
		a.chatMessages = nil
		a.requestChatMessages(channel.ID)
	} else {
		// Forum channel (type 1) - threaded discussions
		a.mainView = commands.ViewThreadList
		a.loadingThreads = true
		a.threads = nil
		a.threadFocusIndex = 0 // Reset thread focus when entering thread list
		a.requestThreadList(channel.ID)
	}

	log.Printf("Selected channel: %s (type %d)", channel.Name, channel.Type)

	// Trigger window redraw
	if a.window != nil {
		a.window.Invalidate()
	}
}

// selectThread handles thread selection (opens thread view)
func (a *App) selectThread(thread *protocol.Message) {
	a.currentThread = thread
	a.mainView = commands.ViewThreadView
	a.loadingReplies = true
	a.threadReplies = nil
	a.replyFocusIndex = 0 // Reset reply focus when entering thread view

	// Request thread replies
	a.requestThreadReplies(thread.ID)

	log.Printf("Selected thread: %s", thread.Content)

	// Trigger window redraw
	if a.window != nil {
		a.window.Invalidate()
	}
}

// requestThreadList requests thread list from server
func (a *App) requestThreadList(channelID uint64) {
	msg := &protocol.ListMessagesMessage{
		ChannelID:    channelID,
		SubchannelID: nil,
		ParentID:     nil, // nil = root messages only (threads)
		Limit:        50,
		BeforeID:     nil,
		AfterID:      nil,
	}
	if err := a.conn.SendMessage(protocol.TypeListMessages, msg); err != nil {
		log.Printf("Failed to request thread list: %v", err)
	}
}

// requestChatMessages requests chat messages from server
func (a *App) requestChatMessages(channelID uint64) {
	msg := &protocol.ListMessagesMessage{
		ChannelID:    channelID,
		SubchannelID: nil,
		ParentID:     nil, // No parent ID = all messages (chat has no threading)
		Limit:        100,
		BeforeID:     nil,
		AfterID:      nil,
	}
	if err := a.conn.SendMessage(protocol.TypeListMessages, msg); err != nil {
		log.Printf("Failed to request chat messages: %v", err)
	}
}

// requestThreadReplies requests replies for a specific thread
func (a *App) requestThreadReplies(threadID uint64) {
	if a.selectedChannel == nil {
		log.Printf("Cannot request thread replies: no channel selected")
		return
	}

	msg := &protocol.ListMessagesMessage{
		ChannelID:    a.selectedChannel.ID,
		SubchannelID: nil,
		ParentID:     &threadID, // ParentID = thread root message
		Limit:        50,
		BeforeID:     nil,
		AfterID:      nil,
	}
	if err := a.conn.SendMessage(protocol.TypeListMessages, msg); err != nil {
		log.Printf("Failed to request thread replies: %v", err)
	}
}

// openComposeModal opens the compose modal
func (a *App) openComposeModal(mode ComposeMode, replyTo *protocol.Message) {
	modal := &ComposeModal{
		mode:    mode,
		editor:  widget.Editor{SingleLine: false, Submit: false},
		replyTo: replyTo,
	}
	a.composeModal = modal

	log.Printf("Opened compose modal (mode: %d)", mode)

	// Trigger window redraw
	if a.window != nil {
		a.window.Invalidate()
	}
}

// closeComposeModal closes the compose modal
func (a *App) closeComposeModal() {
	a.composeModal = nil

	// Trigger window redraw
	if a.window != nil {
		a.window.Invalidate()
	}
}

// sendMessage sends a message to the server
func (a *App) sendMessage(content string, parentID *uint64) {
	if a.selectedChannel == nil {
		log.Printf("Cannot send message: no channel selected")
		return
	}

	msg := &protocol.PostMessageMessage{
		ChannelID:    a.selectedChannel.ID,
		SubchannelID: nil,
		ParentID:     parentID,
		Content:      content,
	}

	if err := a.conn.SendMessage(protocol.TypePostMessage, msg); err != nil {
		log.Printf("Failed to send message: %v", err)
		return
	}

	log.Printf("Posted message (parent: %v, content: %s)", parentID, content[:min(len(content), 50)])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Layout renders the application UI
func (a *App) Layout(gtx layout.Context) layout.Dimensions {
	// Handle keyboard shortcuts BEFORE widgets consume events
	// This allows modal commands (like Ctrl+D to send) to work
	// even when editor has focus
	a.handleKeyboardShortcuts(gtx)

	// Render base view based on mainView state
	switch a.mainView {
	case commands.ViewChannelList:
		a.layoutChannelListView(gtx)
	case commands.ViewThreadList:
		a.layoutThreadListView(gtx)
	case commands.ViewThreadView:
		a.layoutThreadViewView(gtx)
	case commands.ViewChatChannel:
		a.layoutChatChannelView(gtx)
	default:
		a.layoutChannelListView(gtx)
	}

	// Render modal overlay if modal is open
	var dims layout.Dimensions
	if a.composeModal != nil {
		dims = a.layoutModalOverlay(gtx)
	} else {
		dims = layout.Dimensions{Size: gtx.Constraints.Max}
	}

	return dims
}

// handleKeyboardShortcuts processes keyboard shortcuts
// Strategy: Always consume modifier keys (Ctrl+*, Alt+*) before widgets see them
// Let regular text keys through to editor
func (a *App) handleKeyboardShortcuts(gtx layout.Context) {
	eventCount := 0
	for {
		// Get all keyboard events
		ev, ok := gtx.Event(key.Filter{})
		if !ok {
			break
		}

		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}

		// Debug: Log ALL modifier events to see what macOS is sending
		if e.Modifiers != 0 {
			log.Printf("[KEYBOARD DEBUG] Event with modifiers: Name='%v' Modifiers=%v (Ctrl=%v, Command=%v, Alt=%v)",
				e.Name, e.Modifiers,
				e.Modifiers.Contain(key.ModCtrl),
				e.Modifiers.Contain(key.ModCommand),
				e.Modifiers.Contain(key.ModAlt))
		}

		// Check if this has modifiers - always intercept those
		// Note: ModCommand is the Command key on macOS, Ctrl is ModCtrl
		hasModifiers := e.Modifiers.Contain(key.ModCtrl) ||
						e.Modifiers.Contain(key.ModCommand) ||
						e.Modifiers.Contain(key.ModAlt)

		// When modal is open with editor, only intercept modifier shortcuts and Escape
		// Let all other keys (text input, navigation) pass through to the editor widget
		if a.composeModal != nil && !hasModifiers && e.Name != key.NameEscape {
			// Skip processing - let editor handle this event naturally
			continue
		}

		// Convert Gio key event to string format
		keyStr := a.keyEventToString(e)
		if keyStr == "" {
			if e.Modifiers.Contain(key.ModCtrl) {
				log.Printf("[KEYBOARD DEBUG] keyEventToString returned empty for Ctrl event!")
			}
			continue
		}

		eventCount++
		modalStatus := "no modal"
		if a.composeModal != nil {
			modalStatus = "modal open"
		}
		log.Printf("[KEYBOARD] Event #%d: key='%s' view=%s modal=%s",
			eventCount, keyStr, a.GetCurrentView(), modalStatus)

		// Check if this key matches a shared command
		sharedCmd := commands.FindCommandForKey(keyStr, a)

		if sharedCmd != nil {
			// This is a command key - execute it
			log.Printf("[KEYBOARD] ✓ Handled by command system: '%s' -> %s",
				sharedCmd.Name, sharedCmd.ActionID)
			err := a.ExecuteAction(sharedCmd.ActionID)
			if err != nil {
				log.Printf("[KEYBOARD] ✗ Command execution error: %v", err)
			}
			continue
		}

		// No command match
		log.Printf("[KEYBOARD] ○ No handler for key '%s' (view=%s)", keyStr, a.GetCurrentView())
	}
}

// keyEventToString converts a Gio key event to a string format
// matching the terminal client's key string format
func (a *App) keyEventToString(e key.Event) string {
	// Handle modifier combinations
	// On macOS, Command key is the primary modifier, but we normalize to "ctrl" for cross-platform
	ctrl := e.Modifiers.Contain(key.ModCtrl)
	command := e.Modifiers.Contain(key.ModCommand)
	alt := e.Modifiers.Contain(key.ModAlt)
	shift := e.Modifiers.Contain(key.ModShift)

	// Normalize: treat Command (macOS) the same as Ctrl (Linux/Windows)
	isModified := ctrl || command

	// Get base key name as string
	keyName := string(e.Name)

	// Convert special keys to terminal format
	switch e.Name {
	case key.NameEscape:
		return "esc"
	case key.NameReturn:
		if isModified {
			return "ctrl+enter"
		}
		return "enter"
	case key.NameUpArrow:
		return "up"
	case key.NameDownArrow:
		return "down"
	case key.NameLeftArrow:
		return "left"
	case key.NameRightArrow:
		return "right"
	case key.NameDeleteBackward:
		return "backspace"
	case key.NameTab:
		return "tab"
	case key.NameSpace:
		return " "
	}

	// Handle ctrl/command+letter combinations
	if isModified && len(keyName) == 1 {
		if alt || shift {
			// Don't handle complex modifier combinations for now
			return ""
		}
		return "ctrl+" + strings.ToLower(keyName)
	}

	// Handle regular letters (convert to lowercase to match terminal)
	if len(keyName) == 1 && !isModified && !alt {
		return strings.ToLower(keyName)
	}

	// Unknown key combination
	return ""
}

// handleChannelListNavigation handles keyboard navigation in channel list view
// Returns true if handled (blocks shared command processing)
func (a *App) handleChannelListNavigation(keyStr string) bool {
	// All navigation is now handled by shared commands
	return false
}

// handleThreadListNavigation handles keyboard navigation in thread list view
// Returns true if handled (blocks shared command processing)
func (a *App) handleThreadListNavigation(keyStr string) bool {
	// All navigation is now handled by shared commands
	return false
}

// handleThreadViewNavigation handles keyboard navigation in thread view
// Returns true if handled (blocks shared command processing)
func (a *App) handleThreadViewNavigation(keyStr string) bool {
	// All navigation is now handled by shared commands
	return false
}

// layoutChannelListView renders the channel list view
func (a *App) layoutChannelListView(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Header
		layout.Rigid(a.layoutHeader),
		// Main content (sidebar + welcome)
		layout.Flexed(1, a.layoutChannelListContent),
	)
}

// layoutThreadListView renders the thread list view
func (a *App) layoutThreadListView(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Header
		layout.Rigid(a.layoutHeader),
		// Main content (sidebar + threads)
		layout.Flexed(1, a.layoutThreadListContent),
	)
}

// layoutChatChannelView renders the chat channel view
func (a *App) layoutChatChannelView(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Header
		layout.Rigid(a.layoutHeader),
		// Main content (sidebar + chat)
		layout.Flexed(1, a.layoutChatContent),
	)
}

// layoutHeader renders the header with version, user, stats, traffic
func (a *App) layoutHeader(gtx layout.Context) layout.Dimensions {
	// Format header text
	left := fmt.Sprintf("SuperChat %s", a.version)

	status := "Disconnected"
	if a.conn.IsConnected() {
		if a.nickname != "" {
			status = fmt.Sprintf("Connected: ~%s", a.nickname)
		} else {
			status = "Connected (anonymous)"
		}
		if a.onlineUsers > 0 {
			status += fmt.Sprintf("  %d users", a.onlineUsers)
		}

		// Add traffic counters
		sent := client.FormatBytes(a.conn.GetBytesSent())
		recv := client.FormatBytes(a.conn.GetBytesReceived())
		status += fmt.Sprintf("  ↑%s ↓%s", sent, recv)

		// Add bandwidth throttle indicator if throttling is enabled
		if a.throttle > 0 {
			bandwidth := client.FormatBandwidth(a.throttle)
			status += fmt.Sprintf("  ⏱ %s", bandwidth)
		}
	}

	return layout.Inset{
		Top:    unit.Dp(8),
		Bottom: unit.Dp(8),
		Left:   unit.Dp(16),
		Right:  unit.Dp(16),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Body1(a.theme, left)
				return label.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Body1(a.theme, status)
				return label.Layout(gtx)
			}),
		)
	})
}

// layoutChannelListContent renders the channel list view content
func (a *App) layoutChannelListContent(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		// Left sidebar: channels
		layout.Rigid(a.layoutChannelSidebar),
		// Main content area: welcome text
		layout.Flexed(1, a.layoutWelcome),
	)
}

// layoutThreadListContent renders the thread list view content
func (a *App) layoutThreadListContent(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		// Left sidebar: channels
		layout.Rigid(a.layoutChannelSidebar),
		// Main content area: thread list
		layout.Flexed(1, a.layoutThreadList),
	)
}

// layoutChatContent renders the chat channel view content
func (a *App) layoutChatContent(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		// Left sidebar: channels
		layout.Rigid(a.layoutChannelSidebar),
		// Main content area: chat messages
		layout.Flexed(1, a.layoutChat),
	)
}

// layoutChannelSidebar renders the channel list sidebar
func (a *App) layoutChannelSidebar(gtx layout.Context) layout.Dimensions {
	// Calculate sidebar width (1/4 of screen)
	sidebarWidth := gtx.Constraints.Max.X / 4
	if sidebarWidth < 150 {
		sidebarWidth = 150
	}

	gtx.Constraints.Max.X = sidebarWidth
	gtx.Constraints.Min.X = sidebarWidth

	return layout.Inset{
		Top:    unit.Dp(8),
		Bottom: unit.Dp(8),
		Left:   unit.Dp(8),
		Right:  unit.Dp(4),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Draw border around sidebar
		return widget.Border{
			Color: color.NRGBA{R: 200, G: 200, B: 200, A: 255},
			Width: unit.Dp(1),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// Add padding inside border
			return layout.Inset{
				Top:    unit.Dp(8),
				Bottom: unit.Dp(8),
				Left:   unit.Dp(8),
				Right:  unit.Dp(8),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					// Title
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						title := material.Body1(a.theme, "Channels")
						title.Font.Weight = 700 // Bold
						return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, title.Layout)
					}),
					// Server address
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						addr := a.conn.GetAddress()
						// Hide default port
						addr = strings.TrimSuffix(addr, ":6465")
						label := material.Caption(a.theme, addr)
						return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, label.Layout)
					}),
					// Channel list
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if a.loadingChannels {
							label := material.Body2(a.theme, "Loading...")
							return label.Layout(gtx)
						}

						// Ensure we have enough buttons for all channels
						for len(a.channelButtons) < len(a.channels) {
							a.channelButtons = append(a.channelButtons, widget.Clickable{})
						}

						return material.List(a.theme, &a.channelList).Layout(gtx, len(a.channels), func(gtx layout.Context, i int) layout.Dimensions {
							channel := a.channels[i]

							// Check if button was clicked
							if a.channelButtons[i].Clicked(gtx) {
								a.channelFocusIndex = i // Update focus when clicking
								go a.selectChannel(&channel)
							}

							channelType := "#"
							if channel.Type == 0 {
								channelType = ">"
							}

							// Add focus indicator (▶) for focused item
							focusIndicator := "  "
							if i == a.channelFocusIndex {
								focusIndicator = "▶ "
							}
							text := fmt.Sprintf("%s%s %s", focusIndicator, channelType, channel.Name)

							// Render as clickable button
							btn := material.Button(a.theme, &a.channelButtons[i], text)
							btn.TextSize = unit.Sp(14)

							// Highlight selected channel (blue) or focused channel (light blue)
							if a.selectedChannel != nil && a.selectedChannel.ID == channel.ID {
								btn.Background = color.NRGBA{R: 100, G: 149, B: 237, A: 255} // Cornflower blue
								btn.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}      // White text
							} else if i == a.channelFocusIndex {
								btn.Background = color.NRGBA{R: 173, G: 216, B: 230, A: 255} // Light blue
								btn.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}            // Black text
							} else {
								btn.Background = color.NRGBA{R: 240, G: 240, B: 240, A: 255} // Light gray
								btn.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}            // Black text
							}

							return layout.Inset{
								Top:    unit.Dp(2),
								Bottom: unit.Dp(2),
							}.Layout(gtx, btn.Layout)
						})
					}),
				)
			})
		})
	})
}

// layoutWelcome renders the welcome text
func (a *App) layoutWelcome(gtx layout.Context) layout.Dimensions {
	welcomeText := `Welcome to SuperChat!

Select a channel from the left to start browsing.

Anonymous vs Registered:
• Anonymous: Post as ~username (no password)
• Registered: Post as username

Press [Ctrl+N] to create a new thread once in a channel.
Press [Ctrl+H] for help.`

	return layout.Inset{
		Top:    unit.Dp(8),
		Bottom: unit.Dp(8),
		Left:   unit.Dp(4),
		Right:  unit.Dp(8),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Draw border around main content
		return widget.Border{
			Color: color.NRGBA{R: 200, G: 200, B: 200, A: 255},
			Width: unit.Dp(1),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// Add padding inside border
			return layout.Inset{
				Top:    unit.Dp(24),
				Bottom: unit.Dp(24),
				Left:   unit.Dp(24),
				Right:  unit.Dp(24),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				label := material.Body1(a.theme, welcomeText)
				return label.Layout(gtx)
			})
		})
	})
}

// layoutThreadList renders the thread list
func (a *App) layoutThreadList(gtx layout.Context) layout.Dimensions {
	return layout.Inset{
		Top:    unit.Dp(8),
		Bottom: unit.Dp(8),
		Left:   unit.Dp(4),
		Right:  unit.Dp(8),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Draw border around main content
		return widget.Border{
			Color: color.NRGBA{R: 200, G: 200, B: 200, A: 255},
			Width: unit.Dp(1),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// Add padding inside border
			return layout.Inset{
				Top:    unit.Dp(8),
				Bottom: unit.Dp(8),
				Left:   unit.Dp(8),
				Right:  unit.Dp(8),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					// Title bar with "New Thread" button
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
							// Left: Title
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								var title string
								if a.selectedChannel != nil {
									channelPrefix := "#"
									if a.selectedChannel.Type == 0 {
										channelPrefix = ">"
									}
									title = fmt.Sprintf("%s%s - Threads", channelPrefix, a.selectedChannel.Name)
								} else {
									title = "Threads"
								}
								label := material.H6(a.theme, title)
								label.Font.Weight = 700
								return label.Layout(gtx)
							}),

							// Spacer
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Dimensions{}
							}),

							// Right: New Thread button
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if a.newThreadBtn.Clicked(gtx) {
									a.openComposeModal(ComposeModeNewThread, nil)
								}
								btn := material.Button(a.theme, &a.newThreadBtn, "New Thread")
								btn.Background = color.NRGBA{R: 100, G: 149, B: 237, A: 255}
								btn.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
								return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, btn.Layout)
							}),
						)
					}),
					// Thread list
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if a.loadingThreads {
							label := material.Body2(a.theme, "Loading threads...")
							return label.Layout(gtx)
						}

						if len(a.threads) == 0 {
							label := material.Body2(a.theme, "(no threads)")
							return label.Layout(gtx)
						}

						// Ensure we have enough buttons for all threads
						for len(a.threadButtons) < len(a.threads) {
							a.threadButtons = append(a.threadButtons, widget.Clickable{})
						}

						return material.List(a.theme, &a.threadList).Layout(gtx, len(a.threads), func(gtx layout.Context, i int) layout.Dimensions {
							thread := a.threads[i]

							// Check if button was clicked
							if a.threadButtons[i].Clicked(gtx) {
								a.threadFocusIndex = i // Update focus when clicking
								threadCopy := thread
								go a.selectThread(&threadCopy)
							}

							// Format thread components separately
							// Server already prefixes anonymous users with ~
							author := thread.AuthorNickname
							preview := client.ExtractThreadTitle(thread.Content, 60)
							preview = strings.ReplaceAll(preview, "\n", " ")
							timeStr := client.FormatRelativeTime(thread.CreatedAt)

							replyCount := ""
							if thread.ReplyCount > 0 {
								replyCount = fmt.Sprintf(" (%d)", thread.ReplyCount)
							}

							// Add focus indicator (▶) for focused item
							focusIndicator := ""
							if i == a.threadFocusIndex {
								focusIndicator = "▶ "
							}

							// Render as clickable with space-between layout
							return layout.Inset{
								Top:    unit.Dp(2),
								Bottom: unit.Dp(2),
							}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								// Make entire row clickable
								return a.threadButtons[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									// Background color based on focus
									bgColor := color.NRGBA{R: 240, G: 240, B: 240, A: 255}
									if i == a.threadFocusIndex {
										bgColor = color.NRGBA{R: 173, G: 216, B: 230, A: 255} // Light blue for focus
									}
									return material.ButtonLayoutStyle{
										Background:   bgColor,
										CornerRadius: unit.Dp(4),
										Button:       &a.threadButtons[i],
									}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
												// Left: focus indicator + author + preview
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													leftText := fmt.Sprintf("%s%s %s", focusIndicator, author, preview)
													label := material.Body2(a.theme, leftText)
													label.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
													label.TextSize = unit.Sp(13)
													return label.Layout(gtx)
												}),
												// Spacer: fills remaining space
												layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
													return layout.Spacer{Width: unit.Dp(0)}.Layout(gtx)
												}),
												// Right: time + replies
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													rightText := fmt.Sprintf("%s%s", timeStr, replyCount)
													label := material.Body2(a.theme, rightText)
													label.Color = color.NRGBA{R: 128, G: 128, B: 128, A: 255} // Gray
													label.TextSize = unit.Sp(12)
													return label.Layout(gtx)
												}),
											)
										})
									})
								})
							})
						})
					}),
				)
			})
		})
	})
}

// layoutChat renders the chat messages
func (a *App) layoutChat(gtx layout.Context) layout.Dimensions {
	return layout.Inset{
		Top:    unit.Dp(8),
		Bottom: unit.Dp(8),
		Left:   unit.Dp(4),
		Right:  unit.Dp(8),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Draw border around main content
		return widget.Border{
			Color: color.NRGBA{R: 200, G: 200, B: 200, A: 255},
			Width: unit.Dp(1),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// Add padding inside border
			return layout.Inset{
				Top:    unit.Dp(8),
				Bottom: unit.Dp(8),
				Left:   unit.Dp(8),
				Right:  unit.Dp(8),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if a.loadingChat {
					label := material.Body1(a.theme, "Loading messages...")
					return label.Layout(gtx)
				}

				if len(a.chatMessages) == 0 {
					label := material.Body1(a.theme, "No messages yet. Type a message to start chatting.")
					return label.Layout(gtx)
				}

				return material.List(a.theme, &a.chatList).Layout(gtx, len(a.chatMessages), func(gtx layout.Context, i int) layout.Dimensions {
					msg := a.chatMessages[i]

					// Format message display
					// Server already prefixes anonymous users with ~
					author := msg.AuthorNickname
					text := fmt.Sprintf("[%s] %s", author, msg.Content)
					label := material.Body2(a.theme, text)

					return layout.Inset{
						Top:    unit.Dp(2),
						Bottom: unit.Dp(2),
					}.Layout(gtx, label.Layout)
				})
			})
		})
	})
}

// layoutThreadViewView renders the thread view (single thread with replies)
func (a *App) layoutThreadViewView(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Header
		layout.Rigid(a.layoutHeader),
		// Main content: thread with replies
		layout.Flexed(1, a.layoutThreadView),
	)
}

// layoutThreadView renders a single thread with all replies
func (a *App) layoutThreadView(gtx layout.Context) layout.Dimensions {
	return layout.Inset{
		Top:    unit.Dp(8),
		Bottom: unit.Dp(8),
		Left:   unit.Dp(8),
		Right:  unit.Dp(8),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Draw border around main content
		return widget.Border{
			Color: color.NRGBA{R: 200, G: 200, B: 200, A: 255},
			Width: unit.Dp(1),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// Add padding inside border
			return layout.Inset{
				Top:    unit.Dp(8),
				Bottom: unit.Dp(8),
				Left:   unit.Dp(8),
				Right:  unit.Dp(8),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if a.currentThread == nil {
					label := material.Body1(a.theme, "No thread selected")
					return label.Layout(gtx)
				}

				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					// Title: channel name and thread info
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						var title string
						if a.selectedChannel != nil {
							channelPrefix := "#"
							if a.selectedChannel.Type == 0 {
								channelPrefix = ">"
							}
							title = fmt.Sprintf("%s%s - Thread", channelPrefix, a.selectedChannel.Name)
						} else {
							title = "Thread"
						}
						label := material.H6(a.theme, title)
						label.Font.Weight = 700
						return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, label.Layout)
					}),
					// Thread content (root message + replies)
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if a.loadingReplies {
							label := material.Body2(a.theme, "Loading replies...")
							return label.Layout(gtx)
						}

						// Calculate depths for all messages
						depths := client.CalculateThreadDepths(a.currentThread.ID, a.threadReplies)

						// Count: root message + replies
						totalMessages := 1 + len(a.threadReplies)

						// Ensure we have enough reply buttons
						for len(a.replyButtons) < totalMessages {
							a.replyButtons = append(a.replyButtons, widget.Clickable{})
						}

						return material.List(a.theme, &a.threadViewport).Layout(gtx, totalMessages, func(gtx layout.Context, i int) layout.Dimensions {
							var msg protocol.Message
							var depth int
							isRoot := i == 0

							if isRoot {
								msg = *a.currentThread
								depth = 0
							} else {
								msg = a.threadReplies[i-1]
								depth = depths[msg.ID]
							}

							// Check if reply button was clicked
							if a.replyButtons[i].Clicked(gtx) {
								a.replyFocusIndex = i // Update focus when clicking
								msgCopy := msg
								a.openComposeModal(ComposeModeReply, &msgCopy)
							}

							// Format message
							// Server already prefixes anonymous users with ~
							author := msg.AuthorNickname
							timeStr := client.FormatRelativeTime(msg.CreatedAt)

							// Add focus indicator (▶) for focused message
							focusIndicator := ""
							if i == a.replyFocusIndex {
								focusIndicator = "▶ "
							}

							// Calculate indent based on depth (20dp per level)
							indentLeft := unit.Dp(depth * 20)

							// Render message with indentation
							return layout.Inset{
								Top:    unit.Dp(4),
								Bottom: unit.Dp(8),
								Left:   indentLeft,
								Right:  unit.Dp(0),
							}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								// For focused messages, draw background sized to content
								// Use layout.Stack to layer background behind content
								if i == a.replyFocusIndex {
									return layout.Stack{}.Layout(gtx,
										// Background layer (expanded to fill)
										layout.Expanded(func(gtx layout.Context) layout.Dimensions {
											stack := clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops)
											defer stack.Pop()
											paint.ColorOp{Color: color.NRGBA{R: 173, G: 216, B: 230, A: 255}}.Add(gtx.Ops)
											paint.PaintOp{}.Add(gtx.Ops)
											return layout.Dimensions{Size: gtx.Constraints.Min}
										}),
										// Content layer (stacked on top)
										layout.Stacked(func(gtx layout.Context) layout.Dimensions {
											return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
												// Author, time, and reply button
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
														// Left: Focus indicator + Author and time
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															authorText := fmt.Sprintf("%s%s • %s", focusIndicator, author, timeStr)
															label := material.Body2(a.theme, authorText)
															if isRoot {
																label.Font.Weight = 700 // Bold for root message
															}
															label.Color = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
															label.TextSize = unit.Sp(12)
															return label.Layout(gtx)
														}),
														// Spacer
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx)
														}),
														// Right: Reply button
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															btn := material.Button(a.theme, &a.replyButtons[i], "Reply")
															btn.TextSize = unit.Sp(10)
															btn.Background = color.NRGBA{R: 220, G: 220, B: 220, A: 255}
															btn.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
															btn.Inset = layout.Inset{
																Top:    unit.Dp(2),
																Bottom: unit.Dp(2),
																Left:   unit.Dp(6),
																Right:  unit.Dp(6),
															}
															return btn.Layout(gtx)
														}),
													)
												}),
												// Message content
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													label := material.Body1(a.theme, msg.Content)
													label.TextSize = unit.Sp(14)
													return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, label.Layout)
												}),
											)
										}),
									)
								}

								// Non-focused messages - just render content
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									// Author, time, and reply button
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
											// Left: Focus indicator + Author and time
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												authorText := fmt.Sprintf("%s%s • %s", focusIndicator, author, timeStr)
												label := material.Body2(a.theme, authorText)
												if isRoot {
													label.Font.Weight = 700 // Bold for root message
												}
												label.Color = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
												label.TextSize = unit.Sp(12)
												return label.Layout(gtx)
											}),
											// Spacer
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return layout.Spacer{Width: unit.Dp(8)}.Layout(gtx)
											}),
											// Right: Reply button
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												btn := material.Button(a.theme, &a.replyButtons[i], "Reply")
												btn.TextSize = unit.Sp(10)
												btn.Background = color.NRGBA{R: 220, G: 220, B: 220, A: 255}
												btn.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
												btn.Inset = layout.Inset{
													Top:    unit.Dp(2),
													Bottom: unit.Dp(2),
													Left:   unit.Dp(6),
													Right:  unit.Dp(6),
												}
												return btn.Layout(gtx)
											}),
										)
									}),
									// Message content
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										label := material.Body1(a.theme, msg.Content)
										label.TextSize = unit.Sp(14)
										return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, label.Layout)
									}),
								)
							})
						})
					}),
				)
			})
		})
	})
}

// layoutModalOverlay renders the compose modal overlay
func (a *App) layoutModalOverlay(gtx layout.Context) layout.Dimensions {
	// Draw semi-transparent background
	bgStack := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	defer bgStack.Pop()
	paint.ColorOp{Color: color.NRGBA{R: 0, G: 0, B: 0, A: 128}}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)

	// Center the modal
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Modal container with fixed size
		modalWidth := gtx.Dp(unit.Dp(600))
		modalHeight := gtx.Dp(unit.Dp(400))
		gtx.Constraints.Min.X = modalWidth
		gtx.Constraints.Max.X = modalWidth
		gtx.Constraints.Min.Y = modalHeight
		gtx.Constraints.Max.Y = modalHeight

		// Draw modal background
		return widget.Border{
			Color:        color.NRGBA{R: 100, G: 149, B: 237, A: 255}, // Blue border
			Width:        unit.Dp(2),
			CornerRadius: unit.Dp(8),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// White background
			modalStack := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(8)).Push(gtx.Ops)
			defer modalStack.Pop()
			paint.ColorOp{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)

			// Modal content
			return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					// Title
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						var title string
						if a.composeModal.mode == ComposeModeNewThread {
							title = "Compose New Thread"
						} else {
							title = "Compose Reply"
						}
						label := material.H6(a.theme, title)
						label.Font.Weight = 700
						return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, label.Layout)
					}),

					// Parent message info (for replies)
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if a.composeModal.mode == ComposeModeReply && a.composeModal.replyTo != nil {
							replyText := fmt.Sprintf("Replying to: %s", a.composeModal.replyTo.AuthorNickname)
							label := material.Body2(a.theme, replyText)
							label.Color = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
							return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, label.Layout)
						}
						return layout.Dimensions{}
					}),

					// Text editor
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						// Request focus for the editor when modal is open
						// This now works correctly because handleKeyboardShortcuts
						// only intercepts modifier keys and lets regular text through
						gtx.Execute(key.FocusCmd{Tag: &a.composeModal.editor})

						// Draw border around editor
						return widget.Border{
							Color: color.NRGBA{R: 200, G: 200, B: 200, A: 255},
							Width: unit.Dp(1),
						}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							// Add padding inside border
							return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								editor := material.Editor(a.theme, &a.composeModal.editor, "Type your message here...")
								editor.TextSize = unit.Sp(14)
								return editor.Layout(gtx)
							})
						})
					}),

					// Thread title preview (for new threads)
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if a.composeModal.mode == ComposeModeNewThread && len(a.composeModal.editor.Text()) > 0 {
							title := client.ExtractThreadTitle(a.composeModal.editor.Text(), 60)
							title = strings.ReplaceAll(title, "\n", " ")
							previewText := fmt.Sprintf("Thread title preview: %s", title)
							label := material.Caption(a.theme, previewText)
							label.Color = color.NRGBA{R: 100, G: 149, B: 237, A: 255}
							return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, label.Layout)
						}
						return layout.Dimensions{}
					}),

					// Buttons
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
								// Left: Cancel button
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									btn := material.Button(a.theme, &a.composeModal.cancelBtn, "Cancel (Esc)")
									btn.Background = color.NRGBA{R: 200, G: 200, B: 200, A: 255}
									btn.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}

									// Check if button was clicked AFTER rendering
									if a.composeModal.cancelBtn.Clicked(gtx) {
										// Close modal on next frame to avoid issues with deferred Pop()
										go func() {
											a.closeComposeModal()
											if a.window != nil {
												a.window.Invalidate()
											}
										}()
									}

									return btn.Layout(gtx)
								}),

								// Spacer
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Dimensions{}
								}),

								// Right: Send button
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									btn := material.Button(a.theme, &a.composeModal.submitBtn, "Send (Ctrl+D)")
									btn.Background = color.NRGBA{R: 100, G: 149, B: 237, A: 255}
									btn.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}

									// Check if button was clicked AFTER rendering
									if a.composeModal.submitBtn.Clicked(gtx) {
										content := a.composeModal.editor.Text()
										if len(content) > 0 {
											var parentID *uint64
											if a.composeModal.mode == ComposeModeReply && a.composeModal.replyTo != nil {
												parentID = &a.composeModal.replyTo.ID
											}
											a.sendMessage(content, parentID)
											// Close modal on next frame to avoid issues with deferred Pop()
											go func() {
												a.closeComposeModal()
												if a.window != nil {
													a.window.Invalidate()
												}
											}()
										}
									}

									return btn.Layout(gtx)
								}),
							)
						})
					}),
				)
			})
		})
	})
}

