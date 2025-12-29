package ui

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/aeolun/superchat/pkg/client"
	"github.com/aeolun/superchat/pkg/client/assets"
	"github.com/aeolun/superchat/pkg/client/crypto"
	"github.com/aeolun/superchat/pkg/client/ui/commands"
	"github.com/aeolun/superchat/pkg/client/ui/modal"
	"github.com/aeolun/superchat/pkg/protocol"
	"github.com/aeolun/superchat/pkg/updater"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ViewState represents the current view
type ViewState int

const (
	ViewSplash ViewState = iota
	ViewChannelList
	ViewThreadList
	ViewThreadView
	ViewChatChannel
	ViewHelp
)

// ConnectionState represents the connection status
type ConnectionState int

const (
	StateConnected ConnectionState = iota
	StateDisconnected
	StateReconnecting
)

// AuthState represents the authentication status
type AuthState int

const (
	AuthStateNone           AuthState = iota // No auth attempted
	AuthStatePrompting                       // Password modal shown
	AuthStateAuthenticating                  // Waiting for server response
	AuthStateAuthenticated                   // Successfully authenticated
	AuthStateFailed                          // Last attempt failed
	AuthStateAnonymous                       // Explicitly chose anonymous
	AuthStateRegistering                     // Registration in progress
)

type presenceEntry struct {
	SessionID    uint64
	Nickname     string
	IsRegistered bool
	UserID       *uint64
	UserFlags    protocol.UserFlags
}

// DMChannel represents an active DM channel
type DMChannel struct {
	ChannelID       uint64
	OtherUserID     *uint64 // nil if other party is anonymous
	OtherNickname   string
	IsEncrypted     bool
	OtherPubKey     []byte // Other party's X25519 public key (for encrypted DMs)
	UnreadCount     uint32
	ParticipantLeft bool // True if the other participant has permanently left
}

// DMInvite represents an incoming pending DM request (from someone else)
type DMInvite struct {
	ChannelID        uint64
	FromUserID       *uint64 // nil if initiator is anonymous
	FromNickname     string
	EncryptionStatus uint8 // 0=not possible, 1=required, 2=optional (see protocol.DMEncryption*)
}

// OutgoingDMInvite represents a DM we initiated that's waiting for acceptance
type OutgoingDMInvite struct {
	InviteID   uint64  // The invite/channel ID from DM_PENDING
	ToUserID   *uint64 // nil if target is anonymous
	ToNickname string
}

// Model represents the application state
type Model struct {
	// Connection and state
	conn             client.ConnectionInterface
	state            client.StateInterface
	keyStore         *crypto.KeyStore
	connectionState  ConnectionState
	reconnectAttempt int
	switchingMethod  bool   // True when user is trying a different connection method
	connGeneration   uint64 // Incremented each time we replace the connection

	// Directory mode (for server discovery)
	directoryMode      bool
	throttle           int
	logger             *log.Logger
	awaitingServerList bool                  // True when we've requested LIST_SERVERS
	availableServers   []protocol.ServerInfo // Servers from directory

	// Current view and modals
	mainView    MainView
	modalStack  modal.ModalStack
	currentView ViewState // DEPRECATED: will be removed during migration

	// Server state
	serverConfig       *protocol.ServerConfigMessage
	channels           []protocol.Channel
	currentChannel     *protocol.Channel
	expandedChannelID  *uint64                   // Which channel is expanded in sidebar (nil = none)
	subchannels        []protocol.SubchannelInfo // Subchannels for the expanded channel
	loadingSubchannels bool                      // Whether subchannels are being loaded
	threads            []protocol.Message        // Root messages
	currentThread    *protocol.Message
	threadReplies    []protocol.Message // All replies in current thread
	onlineUsers      uint32
	userDirectory    map[string]uint64
	hasActiveChannel bool
	activeChannelID  uint64
	channelRoster    map[uint64]map[uint64]presenceEntry // channelID -> sessionID -> entry
	serverRoster     map[uint64]presenceEntry            // sessionID -> entry
	selfSessionID    *uint64
	showUserSidebar  bool
	unreadCounts     map[uint64]uint32 // channelID -> unread count

	// Loading states
	loadingChannels      bool // True if fetching channel list
	loadingThreadList    bool // True if fetching initial thread list
	loadingThreadReplies bool // True if fetching thread replies
	loadingMore          bool // True if we're currently loading more threads
	loadingMoreReplies   bool // True if we're currently loading more replies
	sendingMessage       bool // True if posting/editing a message
	allThreadsLoaded     bool // True if we've reached the end of threads
	allRepliesLoaded     bool // True if we've reached the end of replies in current thread

	// UI state
	width              int
	height             int
	channelCursor      int
	threadCursor       int
	replyCursor        int
	threadViewport     viewport.Model  // Viewport for thread view
	threadListViewport viewport.Model  // Viewport for thread list
	chatViewport       viewport.Model  // Viewport for chat channel view
	splashViewport     viewport.Model  // Viewport for splash screen
	spinner            spinner.Model   // Loading spinner
	newMessageIDs      map[uint64]bool // Track new messages in current thread
	confirmingDelete   bool
	pendingDeleteID    uint64

	// Chat channel state
	chatMessages  []protocol.Message // Linear list of all messages in chat channel
	chatInput     string             // Current input in chat channel (deprecated - use chatTextarea)
	chatTextarea  textarea.Model     // Textarea for chat input
	loadingChat   bool               // True if loading chat messages
	allChatLoaded bool               // True if we've reached the beginning of chat history

	// Input state
	nickname             string
	pendingNickname      string  // Nickname we sent to server, waiting for confirmation
	authTargetNickname   string  // Nickname we're attempting to authenticate as
	nicknameIsRegistered bool    // True if current nickname belongs to a registered user
	userID               *uint64 // Set when authenticated (V2), nil for anonymous
	userFlags            protocol.UserFlags
	composeInput         string // Temporary storage for compose state
	composeParentID      *uint64
	composeMessageID     *uint64 // Message ID when editing

	// Auth state (V2)
	authState         AuthState
	authAttempts      int       // For rate limiting
	authCooldownUntil time.Time // For rate limiting
	authErrorMessage  string    // For displaying errors in password modal

	// First post warning (session-level, resets on restart)
	firstPostWarningAskedThisSession bool // True if warning was shown this session

	// Initialization state machine
	initStateMachine *InitStateMachine

	// Error and status
	errorMessage           string
	statusMessage          string
	statusVersion          uint64 // Incremented each time statusMessage is set, for timeout tracking
	serverDisconnectReason string // Reason provided by server in DISCONNECT message
	showHelp               bool
	firstRun               bool

	// Privacy delay for "Go Anonymous" feature
	privacyDelayActive   bool      // True during privacy delay countdown
	privacyDelayEnd      time.Time // When the delay ends
	privacyDelayNickname string    // Nickname to use after reconnect

	// Version tracking
	currentVersion  string
	latestVersion   string
	updateAvailable bool

	// Real-time updates
	pendingUpdates []protocol.Message

	// Keepalive
	lastPingSent time.Time
	pingInterval time.Duration

	// Notifications
	lastInteractionTime  time.Time
	notificationIconPath string

	// Command system
	commands *commands.Registry

	// Bandwidth optimization
	threadRepliesCache     map[uint64][]protocol.Message // Cached thread replies
	threadHighestMessageID map[uint64]uint64             // Highest message ID seen per thread

	// Direct Messages (V3)
	dmChannels         []DMChannel          // Active DM channels
	pendingDMInvites   []DMInvite           // Incoming DM requests awaiting response
	outgoingDMInvites  []OutgoingDMInvite   // Outgoing DM requests we're waiting on
	dmChannelKeys      map[uint64][]byte    // channelID -> derived AES key for encryption
	encryptionKeyPub  []byte               // Our X25519 public key (nil if not set up)
	encryptionKeyPriv []byte               // Our X25519 private key (nil if not set up)
	dmCursor          int                  // Cursor position in DM list
	showDMList        bool                 // True when viewing DM list instead of channels
}

// NewModel creates a new application model
func NewModel(conn client.ConnectionInterface, state client.StateInterface, currentVersion string, directoryMode bool, throttle int, logger *log.Logger, dataDir string, initialConnErr error) Model {

	firstRun := state.GetFirstRun()
	initialView := ViewChannelList
	initialMainView := MainViewChannelList
	if firstRun {
		initialView = ViewSplash
		initialMainView = MainViewSplash
	}

	nickname := state.GetLastNickname()
	userID := state.GetUserID()

	// Create spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = Styles.Spinner

	// Create textarea for chat input
	ta := textarea.New()
	ta.Placeholder = "Type a message..."
	ta.Prompt = ""
	ta.CharLimit = 0 // No limit (server will enforce max message length)
	ta.SetWidth(80)  // Will be resized dynamically
	ta.SetHeight(3)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle() // Remove cursor line styling
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false) // Disable multiline (Enter sends message)

	// Style the textarea with a border
	ta.FocusedStyle.Base = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")). // Primary color
		Padding(0, 1)
	ta.BlurredStyle.Base = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")). // Muted color
		Padding(0, 1)

	m := Model{
		conn:                   conn,
		state:                  state,
		keyStore:               crypto.NewKeyStore(state.GetStateDir()),
		connectionState:        StateConnected, // Always connected (either to directory or chat server)
		reconnectAttempt:       0,
		directoryMode:          directoryMode,
		throttle:               throttle,
		logger:                 logger,
		awaitingServerList:     false,
		availableServers:       nil,
		mainView:               initialMainView,
		modalStack:             modal.ModalStack{},
		currentView:            initialView, // DEPRECATED
		firstRun:               firstRun,
		nickname:               nickname,
		userID:                 userID,
		currentVersion:         currentVersion,
		channels:               []protocol.Channel{},
		threads:                []protocol.Message{},
		threadReplies:          []protocol.Message{},
		spinner:                s,
		chatTextarea:           ta,
		newMessageIDs:          make(map[uint64]bool),
		userDirectory:          make(map[string]uint64),
		threadRepliesCache:     make(map[uint64][]protocol.Message),
		threadHighestMessageID: make(map[uint64]uint64),
		pingInterval:           18 * time.Second, // Send ping every 18 seconds (3 pings within 60s timeout)
		lastPingSent:           time.Now(),
		lastInteractionTime:    time.Now(), // Initialize to now (active on startup)
		channelRoster:          make(map[uint64]map[uint64]presenceEntry),
		serverRoster:           make(map[uint64]presenceEntry),
		unreadCounts:           make(map[uint64]uint32),
		dmChannelKeys:          make(map[uint64][]byte),
	}

	// Initialize notification icon (write to data directory if needed)
	iconPath, err := assets.GetIconPath(dataDir, state)
	if err != nil && logger != nil {
		logger.Printf("Failed to write notification icon: %v", err)
	} else {
		m.notificationIconPath = iconPath
	}

	// Initialize state machine - detect SSH connection by address prefix
	isSSH := strings.HasPrefix(conn.GetAddress(), "ssh://")
	m.initStateMachine = NewInitStateMachine(isSSH)

	// Initialize command registry
	m.commands = commands.NewRegistry()
	m.registerCommands()

	// If initial connection failed, show connection failed modal
	if initialConnErr != nil {
		// Show connection failed modal with retry/switch/quit options
		m.modalStack.Push(modal.NewConnectionFailedModal(conn.GetAddress(), initialConnErr.Error()))
		m.connectionState = StateDisconnected
	} else if directoryMode {
		// If in directory mode, show server selector immediately
		// Check if this is first launch (no saved server)
		savedServer, _ := state.GetConfig("directory_selected_server")
		isFirstLaunch := savedServer == ""
		connType := conn.GetConnectionType()
		m.modalStack.Push(modal.NewServerSelectorLoading(isFirstLaunch, connType))
		m.awaitingServerList = true
	}

	return m
}

// registerCommands sets up all keyboard commands
func (m *Model) registerCommands() {
	// === Global Commands ===

	// Quit application
	m.commands.Register(commands.NewCommand().
		Keys("q").
		Name("Quit").
		Aliases("Exit").
		Help("Quit the application").
		Global().
		InModals(modal.ModalNone). // Only available when no modal is open
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			return model, model.saveAndQuit()
		}).
		Priority(900).
		Build())

	// Toggle help
	m.commands.Register(commands.NewCommand().
		Keys("h", "?").
		Name("Help").
		Help("Toggle help screen").
		Global().
		InModals(modal.ModalNone). // Only available when no modal is open
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			// Generate help content for current context
			helpContent := model.commands.GenerateHelp(int(model.currentView), model.modalStack.TopType(), model)
			helpModal := modal.NewHelpModal(helpContent)
			model.modalStack.Push(helpModal)
			return model, nil
		}).
		Priority(950).
		Build())

	// Server selector
	m.commands.Register(commands.NewCommand().
		Keys("ctrl+l").
		Name("Server List").
		Help("List available servers").
		Global().
		InModals(modal.ModalNone). // Only available when no modal is open
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			// Show loading modal immediately and request server list
			// If server doesn't support directory, error will show in modal
			// Not first launch when using Ctrl+L (they're switching servers)
			connType := model.conn.GetConnectionType()
			serverModal := modal.NewServerSelectorLoading(false, connType)
			model.modalStack.Push(serverModal)
			return model, model.requestServerList()
		}).
		Priority(940).
		Build())

	// Close help overlay with ESC - now handled by HelpModal itself

	// === ThreadView Commands ===

	// Navigate up
	m.commands.Register(commands.NewCommand().
		Keys("up", "k").
		Name("Navigate").
		Help("Move selection up").
		InViews(int(ViewThreadView)).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			if model.replyCursor > 0 {
				model.replyCursor--
				model.markCurrentMessageAsRead()
				model.threadViewport.SetContent(model.buildThreadContent())
				model.scrollToKeepCursorVisible()
			}
			return model, nil
		}).
		Priority(10).
		Build())

	// Navigate down
	m.commands.Register(commands.NewCommand().
		Keys("down", "j").
		Name("Navigate").
		Help("Move selection down").
		InViews(int(ViewThreadView)).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			if model.replyCursor < len(model.threadReplies) {
				model.replyCursor++
				model.markCurrentMessageAsRead()
				model.threadViewport.SetContent(model.buildThreadContent())
				model.scrollToKeepCursorVisible()

				// Load more replies if needed
				if !model.loadingMoreReplies && !model.allRepliesLoaded && len(model.threadReplies) > 0 {
					remainingReplies := len(model.threadReplies) - model.replyCursor
					if remainingReplies <= 3 {
						model.loadingMoreReplies = true
						return model, model.loadMoreReplies()
					}
				}
			}
			return model, nil
		}).
		Priority(10).
		Build())

	// Reply to message
	m.commands.Register(commands.NewCommand().
		Keys("r").
		Name("Reply").
		Help("Reply to the selected message").
		InViews(int(ViewThreadView)).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			var parentID uint64
			if model.replyCursor == 0 {
				if model.currentThread != nil {
					parentID = model.currentThread.ID
				}
			} else if model.replyCursor-1 < len(model.threadReplies) {
				parentID = model.threadReplies[model.replyCursor-1].ID
			}

			// Store parent ID for when compose modal sends
			model.composeParentID = &parentID

			if model.nickname == "" {
				// Need to set nickname first
				model.showNicknameSetupModal()
				return model, nil
			}

			model.showComposeWithWarning(modal.ComposeModeReply, "")
			return model, nil
		}).
		Priority(20).
		Build())

	// Edit message
	m.commands.Register(commands.NewCommand().
		Keys("e").
		Name("Edit").
		Help("Edit your own message").
		InViews(int(ViewThreadView)).
		When(func(i interface{}) bool {
			model := i.(*Model)
			msg, ok := model.selectedMessage()
			if !ok {
				return false
			}
			if isDeletedMessageContent(msg.Content) {
				return false
			}

			// Only registered users can edit messages (anonymous messages cannot be edited)
			if msg.AuthorUserID == nil {
				return false
			}

			// Check if we're authenticated and own this message
			if model.userID == nil {
				return false
			}
			return *model.userID == *msg.AuthorUserID
		}).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			msg, _ := model.selectedMessage()

			// Store message ID for when compose modal sends
			model.composeMessageID = &msg.ID
			model.composeParentID = nil

			model.showComposeModal(modal.ComposeModeEdit, msg.Content)
			return model, nil
		}).
		Priority(30).
		Build())

	// Delete message
	m.commands.Register(commands.NewCommand().
		Keys("d").
		Name("Delete").
		Help("Delete your own message").
		InViews(int(ViewThreadView)).
		When(func(i interface{}) bool {
			model := i.(*Model)
			msg, ok := model.selectedMessage()
			if !ok {
				return false
			}
			if isDeletedMessageContent(msg.Content) {
				return false
			}

			// Only registered users can delete messages (anonymous messages cannot be deleted)
			if msg.AuthorUserID == nil {
				return false
			}

			// Check if we're authenticated and own this message
			if model.userID == nil {
				return false
			}
			return *model.userID == *msg.AuthorUserID
		}).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			msg, _ := model.selectedMessage()

			// Create delete confirmation modal
			deleteModal := modal.NewDeleteConfirmModal(
				msg.ID,
				func(msgID uint64) tea.Cmd {
					model.statusMessage = "Deleting message..."
					return tea.Batch(
						listenForServerFrames(model.conn, model.connGeneration),
						model.sendDeleteMessage(msgID),
					)
				},
				func() tea.Cmd {
					model.statusMessage = "Deletion canceled"
					return nil
				},
			)
			model.modalStack.Push(deleteModal)
			model.statusMessage = ""
			return model, nil
		}).
		Priority(40).
		Build())

	// Start DM with message author
	m.commands.Register(commands.NewCommand().
		Keys("d").
		Name("DM Author").
		Help("Start a DM with the message author").
		InViews(int(ViewThreadView)).
		When(func(i interface{}) bool {
			model := i.(*Model)
			msg, ok := model.selectedMessage()
			if !ok {
				return false
			}
			// Can't DM yourself
			if model.isOwnMessage(*msg) {
				return false
			}
			// Need to have a nickname set
			if model.nickname == "" {
				return false
			}
			// Can DM if author has a user ID (registered) or nickname (anonymous)
			return msg.AuthorNickname != ""
		}).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			msg, _ := model.selectedMessage()
			return model.startDMWithUser(msg.AuthorUserID, msg.AuthorNickname)
		}).
		Priority(45). // Lower priority than delete (40)
		Build())

	// Start DM with any online user
	m.commands.Register(commands.NewCommand().
		Keys("ctrl+d").
		Name("New DM").
		Help("Start a DM with any online user").
		Global().
		When(func(i interface{}) bool {
			model := i.(*Model)
			// Need to have a nickname set and be connected
			return model.nickname != "" && model.conn != nil
		}).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			model.showStartDMModal()
			return model, nil
		}).
		Priority(10).
		Build())

	// Back to thread list
	m.commands.Register(commands.NewCommand().
		Keys("esc").
		Name("Back").
		Help("Return to thread list").
		InViews(int(ViewThreadView)).
		InModals(modal.ModalNone). // Only available when no modal is open
		When(func(i interface{}) bool {
			// Always available when no modal is open
			return true
		}).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			model.currentView = ViewThreadList
			var cmd tea.Cmd
			if model.currentThread != nil {
				cmd = model.sendUnsubscribeThread(model.currentThread.ID)
			}
			model.threadReplies = []protocol.Message{}
			model.replyCursor = 0
			model.confirmingDelete = false
			model.pendingDeleteID = 0
			model.allRepliesLoaded = false
			model.loadingMoreReplies = false
			return model, cmd
		}).
		Priority(800).
		Build())

	// === ThreadList Commands ===

	// Navigate up in thread list
	m.commands.Register(commands.NewCommand().
		Keys("up", "k").
		Name("Navigate").
		Help("Move selection up").
		InViews(int(ViewThreadList)).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			if model.threadCursor > 0 {
				model.threadCursor--
				model.threadListViewport.SetContent(model.buildThreadListContent())
				model.scrollThreadListToKeepCursorVisible()
			}
			return model, nil
		}).
		Priority(10).
		Build())

	// Navigate down in thread list
	m.commands.Register(commands.NewCommand().
		Keys("down", "j").
		Name("Navigate").
		Help("Move selection down").
		InViews(int(ViewThreadList)).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			if model.threadCursor < len(model.threads)-1 {
				model.threadCursor++
				model.threadListViewport.SetContent(model.buildThreadListContent())
				model.scrollThreadListToKeepCursorVisible()

				// Load more threads if needed
				if !model.loadingMore && !model.allThreadsLoaded && len(model.threads) > 0 {
					remainingThreads := len(model.threads) - model.threadCursor - 1
					if remainingThreads <= 25 {
						model.loadingMore = true
						return model, model.loadMoreThreads()
					}
				}
			}
			return model, nil
		}).
		Priority(10).
		Build())

	// Open thread
	m.commands.Register(commands.NewCommand().
		Keys("enter").
		Name("Open").
		Help("Open the selected thread").
		InViews(int(ViewThreadList)).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			if model.threadCursor < len(model.threads) {
				selectedThread := model.threads[model.threadCursor]
				model.currentThread = &selectedThread
				model.currentView = ViewThreadView
				model.replyCursor = 0
				model.newMessageIDs = make(map[uint64]bool)
				model.confirmingDelete = false
				model.allRepliesLoaded = false // Reset pagination state

				// Check if we have cached data
				var cmd tea.Cmd
				if cachedReplies, ok := model.threadRepliesCache[selectedThread.ID]; ok {
					// Load cached replies immediately
					model.threadReplies = cachedReplies
					model.threadViewport.GotoTop()

					// Fetch only new messages since last cache (no loading indicator for incremental)
					highestID := model.threadHighestMessageID[selectedThread.ID]
					cmd = tea.Batch(
						model.requestThreadRepliesAfter(selectedThread.ID, highestID),
						model.sendSubscribeThread(selectedThread.ID),
					)
				} else {
					// No cache, fetch all from server
					model.loadingThreadReplies = true
					model.threadViewport.SetContent(model.buildThreadContent()) // Show initial spinner
					model.threadViewport.GotoTop()
					cmd = tea.Batch(
						model.requestThreadReplies(selectedThread.ID),
						model.sendSubscribeThread(selectedThread.ID),
					)
				}
				return model, cmd
			}
			return model, nil
		}).
		Priority(50).
		Build())

	// New thread
	m.commands.Register(commands.NewCommand().
		Keys("n").
		Name("New Thread").
		Help("Create a new thread").
		InViews(int(ViewThreadList)).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)

			// Clear parent ID for new thread
			model.composeParentID = nil

			if model.nickname == "" {
				// Need to set nickname first
				model.showNicknameSetupModal()
				return model, nil
			}

			model.showComposeWithWarning(modal.ComposeModeNewThread, "")
			return model, nil
		}).
		Priority(60).
		Build())

	// Refresh thread list
	m.commands.Register(commands.NewCommand().
		Keys("r").
		Name("Refresh").
		Help("Refresh the thread list").
		InViews(int(ViewThreadList)).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			if model.currentChannel != nil {
				model.loadingThreadList = true
				model.threads = []protocol.Message{}                                // Clear threads
				model.threadListViewport.SetContent(model.buildThreadListContent()) // Show initial spinner
				return model, model.requestThreadList(model.currentChannel.ID)
			}
			return model, nil
		}).
		Priority(70).
		Build())

	// Back to channel list
	m.commands.Register(commands.NewCommand().
		Keys("esc").
		Name("Back").
		Help("Return to channel list").
		InViews(int(ViewThreadList)).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			model.currentView = ViewChannelList
			model.confirmingDelete = false
			var cmd tea.Cmd
			if model.currentChannel != nil {
				cmd = tea.Batch(
					model.sendLeaveChannel(model.currentChannel.ID, false),
					model.sendUnsubscribeChannel(model.currentChannel.ID),
				)
				model.clearActiveChannel()
			}
			model.currentChannel = nil
			model.threads = []protocol.Message{}
			model.threadCursor = 0
			model.loadingMore = false
			model.allThreadsLoaded = false
			return model, cmd
		}).
		Priority(800).
		Build())

	// Back to channel list from chat view
	m.commands.Register(commands.NewCommand().
		Keys("esc").
		Name("Back").
		Help("Return to channel list").
		InViews(int(ViewChatChannel)).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			model.currentView = ViewChannelList
			var cmd tea.Cmd
			if model.currentChannel != nil {
				cmd = tea.Batch(
					model.sendLeaveChannel(model.currentChannel.ID, false),
					model.sendUnsubscribeChannel(model.currentChannel.ID),
				)
				model.clearActiveChannel()
			}
			model.currentChannel = nil
			model.chatMessages = []protocol.Message{}
			model.chatTextarea.Blur() // Unfocus textarea
			model.chatTextarea.Reset()
			return model, cmd
		}).
		Priority(800).
		Build())

	// === ChannelList Commands ===

	// Navigate up in channel list
	m.commands.Register(commands.NewCommand().
		Keys("up", "k").
		Name("Navigate").
		Help("Move selection up").
		InViews(int(ViewChannelList)).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			if model.channelCursor > 0 {
				model.channelCursor--
			}
			return model, nil
		}).
		Priority(10).
		Build())

	// Navigate down in channel list
	m.commands.Register(commands.NewCommand().
		Keys("down", "j").
		Name("Navigate").
		Help("Move selection down").
		InViews(int(ViewChannelList)).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			maxIndex := model.getVisibleChannelListItemCount() - 1
			if model.channelCursor < maxIndex {
				model.channelCursor++
			}
			return model, nil
		}).
		Priority(10).
		Build())

	// NOTE: "enter" key handling for channel list is in update.go handleChannelListKeys()
	// It uses getChannelListItemAtCursor() to properly handle DMs, channels, and pending invites

	// Refresh channel list
	m.commands.Register(commands.NewCommand().
		Keys("r").
		Name("Refresh").
		Help("Refresh the channel list").
		InViews(int(ViewChannelList)).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			model.loadingChannels = true
			return model, model.requestChannelList()
		}).
		Priority(70).
		Build())

	// Create new channel
	m.commands.Register(commands.NewCommand().
		Keys("c").
		Name("Create Channel").
		Help("Create a new channel (registered users only)").
		InViews(int(ViewChannelList)).
		When(func(i interface{}) bool {
			model := i.(*Model)
			// Only allow channel creation for registered users
			return model.authState == AuthStateAuthenticated && model.userID != nil
		}).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			model.showCreateChannelModal()
			return model, nil
		}).
		Priority(80).
		Build())

	// Ctrl+R to open registration modal
	m.commands.Register(commands.NewCommand().
		Keys("ctrl+r").
		Name("Register").
		Help("Register this nickname").
		Global().
		When(func(i interface{}) bool {
			model := i.(*Model)
			// Allow registration for anonymous users with a nickname that is NOT already registered
			return model.authState == AuthStateAnonymous &&
				model.nickname != "" &&
				!model.nicknameIsRegistered
		}).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			model.showRegistrationModal()
			return model, nil
		}).
		Priority(10).
		Build())

	// Ctrl+S to sign in (when nickname is registered)
	m.commands.Register(commands.NewCommand().
		Keys("ctrl+s").
		Name("Sign In").
		Help("Sign in with password").
		Global().
		When(func(i interface{}) bool {
			model := i.(*Model)
			// Only for anonymous users with registered nickname
			return model.authState != AuthStateAuthenticated &&
				model.nickname != "" &&
				model.nicknameIsRegistered
		}).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			model.showPasswordModal()
			return model, nil
		}).
		Priority(10).
		Build())

	// Ctrl+A to go anonymous
	m.commands.Register(commands.NewCommand().
		Keys("ctrl+a").
		Name("Go Anonymous").
		Help("Post anonymously").
		Global().
		When(func(i interface{}) bool {
			model := i.(*Model)
			// Only available when authenticated
			return model.authState == AuthStateAuthenticated
		}).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			model.showGoAnonymousModal()
			return model, nil
		}).
		Priority(10).
		Build())

	// Ctrl+N to set or change nickname
	m.commands.Register(commands.NewCommand().
		Keys("ctrl+n").
		Name("Nickname").
		Help("Set or change nickname").
		Global().
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			if model.nickname == "" {
				model.showNicknameSetupModal()
			} else {
				model.showNicknameChangeModal()
			}
			return model, nil
		}).
		Priority(10).
		Build())

	// Ctrl+K to manage SSH keys
	m.commands.Register(commands.NewCommand().
		Keys("ctrl+k").
		Name("SSH Keys").
		Help("Manage SSH keys").
		Global().
		When(func(i interface{}) bool {
			model := i.(*Model)
			// Only available when authenticated
			return model.authState == AuthStateAuthenticated
		}).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			if model.logger != nil {
				model.logger.Printf("[DEBUG] Ctrl+K pressed, authState=%d, showing modal and requesting keys", model.authState)
			}
			// Show modal immediately with empty keys (loading state)
			model.showSSHKeyManagerModal(nil)
			// Request SSH key list from server
			return model, model.sendListSSHKeys()
		}).
		Priority(10).
		Build())

	// Toggle user sidebar with U key
	m.commands.Register(commands.NewCommand().
		Keys("u").
		Name("Users Sidebar").
		Help("Toggle user sidebar").
		Global().
		InModals(modal.ModalNone).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			model.showUserSidebar = !model.showUserSidebar

			var cmds []tea.Cmd
			if model.showUserSidebar {
				if model.currentChannel != nil && model.hasActiveChannel {
					cmds = append(cmds, model.sendListChannelUsers(model.currentChannel.ID))
				} else {
					cmds = append(cmds, model.sendListUsers(false))
				}
				cmds = append(cmds, func() tea.Msg { return ForceRenderMsg{} })
				return model, tea.Batch(cmds...)
			}
			cmds = append(cmds, func() tea.Msg { return ForceRenderMsg{} })
			return model, tea.Batch(cmds...)
		}).
		Priority(60).
		Build())

	// Admin panel with A key
	m.commands.Register(commands.NewCommand().
		Keys("A").
		Name("Admin Panel").
		Help("Open admin panel").
		Global().
		InModals(modal.ModalNone). // Only available when no modal is open
		When(func(i interface{}) bool {
			model := i.(*Model)
			return model.isAdmin()
		}).
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			model.showAdminPanel()
			return model, nil
		}).
		Priority(910).
		Build())

	// Command palette with / (IRC-style)
	m.commands.Register(commands.NewCommand().
		Keys("/").
		Name("Command").
		Help("Open command palette (IRC-style)").
		Global().
		InModals(modal.ModalNone). // Only available when no modal is open
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			model.showCommandPalette("/")
			return model, nil
		}).
		Priority(930).
		Build())

	// Command palette with : (vim-style)
	m.commands.Register(commands.NewCommand().
		Keys(":").
		Name("Command").
		Help("Open command palette (vim-style)").
		Global().
		InModals(modal.ModalNone). // Only available when no modal is open
		Do(func(i interface{}) (interface{}, tea.Cmd) {
			model := i.(*Model)
			model.showCommandPalette(":")
			return model, nil
		}).
		Priority(920).
		Build())
}

// showCommandPalette displays the command palette modal
func (m *Model) showCommandPalette(prefix string) {
	// Get available command names for current context
	availableCommands := m.commands.GetCommandNames(int(m.currentView), m.modalStack.TopType(), m)

	commandPalette := modal.NewCommandPaletteModal(
		prefix,
		availableCommands,
		func(commandName string) tea.Cmd {
			// Return a message to execute the command in the main update loop
			// This allows the model to be properly updated
			return func() tea.Msg {
				return ExecuteCommandMsg{CommandName: commandName}
			}
		},
		func() tea.Cmd {
			// Canceled
			return nil
		},
	)
	m.modalStack.Push(commandPalette)
}

// Modal helper methods

// showPasswordModal displays the password authentication modal
func (m *Model) showPasswordModal() {
	targetNickname := m.authTargetNickname
	if targetNickname == "" {
		targetNickname = m.pendingNickname
	}
	if targetNickname == "" {
		targetNickname = m.nickname
	}

	m.authTargetNickname = targetNickname
	m.authState = AuthStatePrompting

	passwordModal := modal.NewPasswordAuthModal(
		targetNickname,
		m.authErrorMessage,
		m.authCooldownUntil,
		false, // not authenticating initially
		func(password []byte) tea.Cmd {
			m.authState = AuthStateAuthenticating
			m.authErrorMessage = ""
			return m.sendAuthRequest(password)
		},
		func() tea.Cmd {
			// Browse anonymously - send message to be handled in Update()
			// (Can't modify Model directly here due to bubbletea value semantics)
			return func() tea.Msg {
				return GoAnonymousMsg{TargetNickname: targetNickname}
			}
		},
	)
	m.modalStack.Push(passwordModal)
}

// showRegistrationModal displays the registration modal
func (m *Model) showRegistrationModal() {
	registrationModal := modal.NewRegistrationModal(
		m.nickname,
		func(password []byte) tea.Cmd {
			m.authState = AuthStateRegistering
			return m.sendRegisterUser(password)
		},
		func() tea.Cmd {
			// Canceled registration
			return nil
		},
	)
	m.modalStack.Push(registrationModal)
}

// showNicknameChangeModal displays the nickname change modal
func (m *Model) showNicknameChangeModal() {
	nicknameChangeModal := modal.NewNicknameChangeModal(
		m.nickname,
		func(newNickname string) tea.Cmd {
			// Don't modify m.nickname here due to bubbletea value semantics
			// It will be updated in handleNicknameResponse when server confirms
			// But DO set pendingNickname to avoid race condition with server response
			m.pendingNickname = newNickname
			m.state.SetLastNickname(newNickname)
			return tea.Batch(
				m.sendSetNicknameWith(newNickname),
				m.sendGetUserInfo(newNickname),
			)
		},
		func() tea.Cmd {
			// Canceled nickname change
			return nil
		},
	)
	m.modalStack.Push(nicknameChangeModal)
}

// showGoAnonymousModal displays a modal to go anonymous (for registered users)
func (m *Model) showGoAnonymousModal() {
	// Create a modal asking for new anonymous nickname
	nicknameModal := modal.NewNicknameChangeModal(
		"", // Don't pre-fill with current nickname
		func(newNickname string) tea.Cmd {
			// Store the nickname for after reconnect
			m.privacyDelayNickname = newNickname

			// Random delay between 1-5 seconds for privacy
			delay := time.Duration(1000+rand.Intn(4000)) * time.Millisecond
			m.privacyDelayEnd = time.Now().Add(delay)
			m.privacyDelayActive = true

			// Disable auto-reconnect and disconnect
			m.conn.DisableAutoReconnect()
			m.conn.Disconnect()

			// Start the countdown ticker
			return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
				return PrivacyDelayTickMsg{}
			})
		},
		func() tea.Cmd {
			// Canceled
			return nil
		},
	)
	m.modalStack.Push(nicknameModal)
}

// showComposeModal displays the compose modal
func (m *Model) showComposeModal(mode modal.ComposeMode, initialContent string) {
	composeModal := modal.NewComposeModal(
		mode,
		initialContent,
		func(content string) tea.Cmd {
			// Determine what to do based on mode
			var cmd tea.Cmd
			m.sendingMessage = true
			if mode == modal.ComposeModeEdit {
				if m.composeMessageID != nil {
					cmd = m.sendEditMessage(*m.composeMessageID, content)
				}
			} else {
				if m.currentChannel != nil {
					cmd = m.sendPostMessage(m.currentChannel.ID, m.composeParentID, content)
				}
			}
			// Clear compose state
			m.composeInput = ""
			m.composeMessageID = nil
			m.composeParentID = nil
			m.statusMessage = m.spinner.View() + " Sending..."
			return cmd
		},
		func() tea.Cmd {
			// Canceled compose
			m.composeInput = ""
			m.composeMessageID = nil
			m.composeParentID = nil
			return nil
		},
	)
	m.modalStack.Push(composeModal)
}

// showNicknameSetupModal displays the nickname setup modal (first run or nickname needed)
func (m *Model) showNicknameSetupModal() {
	nicknameSetupModal := modal.NewNicknameSetupModal(
		m.nickname,
		func(nickname string) tea.Cmd {
			m.nickname = nickname
			m.state.SetLastNickname(nickname)
			if m.firstRun {
				m.state.SetFirstRunComplete()
				m.firstRun = false
			}
			return tea.Batch(
				m.sendSetNickname(),
				m.sendGetUserInfo(nickname),
			)
		},
		func() tea.Cmd {
			// Quit if they cancel nickname setup
			return tea.Quit
		},
	)
	m.modalStack.Push(nicknameSetupModal)
}

// showSSHKeyManagerModal displays the SSH key manager modal
func (m *Model) showSSHKeyManagerModal(keys []modal.SSHKeyInfo) {
	sshKeyManagerModal := modal.NewSSHKeyManagerModal(
		keys,
		func(publicKey, label string) tea.Cmd {
			// Send ADD_SSH_KEY request
			return m.sendAddSSHKey(publicKey, label)
		},
		func(keyID uint64, newLabel string) tea.Cmd {
			// Send UPDATE_SSH_KEY_LABEL request
			return m.sendUpdateSSHKeyLabel(keyID, newLabel)
		},
		func(keyID uint64) tea.Cmd {
			// Send DELETE_SSH_KEY request
			return m.sendDeleteSSHKey(keyID)
		},
		func() tea.Cmd {
			// Remove password (send CHANGE_PASSWORD with empty new password)
			return m.sendChangePassword([]byte{}, []byte{})
		},
		func() tea.Cmd {
			// Close modal
			return nil
		},
	)
	m.modalStack.Push(sshKeyManagerModal)
}

// showCreateChannelModal displays the channel creation modal
func (m *Model) showCreateChannelModal() {
	createChannelModal := modal.NewCreateChannelModal(
		func(name, displayName, description string, channelType uint8) tea.Cmd {
			m.statusMessage = "Creating channel..."
			return tea.Batch(
				listenForServerFrames(m.conn, m.connGeneration),
				m.sendCreateChannel(name, displayName, description, channelType),
			)
		},
		func() tea.Cmd {
			// Canceled channel creation
			return nil
		},
	)
	m.modalStack.Push(createChannelModal)
}

// showRegistrationWarningModal displays the first post warning modal
func (m *Model) showRegistrationWarningModal(onProceed func() tea.Cmd) {
	registrationWarningModal := modal.NewRegistrationWarningModal(
		func() tea.Cmd {
			// Post anonymously and don't ask again
			m.state.SetFirstPostWarningDismissed()
			m.firstPostWarningAskedThisSession = true
			if onProceed != nil {
				return onProceed()
			}
			return nil
		},
		func() tea.Cmd {
			// Post anonymously but ask again later
			m.firstPostWarningAskedThisSession = true
			if onProceed != nil {
				return onProceed()
			}
			return nil
		},
		func() tea.Cmd {
			// Register first
			m.showRegistrationModal()
			return nil
		},
		func() tea.Cmd {
			// Cancel posting
			return nil
		},
	)
	m.modalStack.Push(registrationWarningModal)
}

// showStartDMModal displays the modal to start a DM with any online user
func (m *Model) showStartDMModal() {
	// Convert serverRoster to OnlineUser slice
	users := make([]modal.OnlineUser, 0, len(m.serverRoster))
	for _, entry := range m.serverRoster {
		users = append(users, modal.OnlineUser{
			SessionID:    entry.SessionID,
			Nickname:     entry.Nickname,
			IsRegistered: entry.IsRegistered,
			UserID:       entry.UserID,
		})
	}

	startDMModal := modal.NewStartDMModal(
		users,
		m.selfSessionID,
		func(userID *uint64, nickname string) tea.Cmd {
			// Return a message to trigger the DM flow in Update
			return func() tea.Msg {
				return StartDMSelectedMsg{
					UserID:   userID,
					Nickname: nickname,
				}
			}
		},
	)
	m.modalStack.Push(startDMModal)
}

// StartDMSelectedMsg is sent when user selects someone to DM from the modal
type StartDMSelectedMsg struct {
	UserID   *uint64
	Nickname string
}

// shouldShowRegistrationWarning returns true if we should show the registration warning
func (m *Model) shouldShowRegistrationWarning() bool {
	// Don't show if user is authenticated (registered)
	if m.authState == AuthStateAuthenticated && m.userID != nil {
		return false
	}

	// Don't show for DM channels - privacy warning doesn't apply to private chats
	if m.currentChannel != nil && m.isCurrentChannelDM() {
		return false
	}

	// Don't show if permanently dismissed
	if m.state.GetFirstPostWarningDismissed() {
		return false
	}

	// Don't show if already asked this session
	if m.firstPostWarningAskedThisSession {
		return false
	}

	return true
}

// isCurrentChannelDM returns true if the current channel is a DM channel
func (m *Model) isCurrentChannelDM() bool {
	if m.currentChannel == nil {
		return false
	}
	for _, dm := range m.dmChannels {
		if dm.ChannelID == m.currentChannel.ID {
			return true
		}
	}
	return false
}

// Admin modal helper methods

func (m *Model) isAdmin() bool {
	return m.authState == AuthStateAuthenticated && m.userFlags.IsAdmin()
}

// showAdminPanel displays the admin panel modal
func (m *Model) showAdminPanel() {
	if !m.isAdmin() {
		return
	}
	m.modalStack.Push(m.createConfiguredAdminPanel())
}

// createConfiguredAdminPanel creates a fully configured admin panel
// This is used both when initially opening the panel and when returning from sub-modals
func (m *Model) createConfiguredAdminPanel() modal.Modal {
	adminPanel := modal.NewAdminPanelModal()

	// Wire up menu item actions to create modals with handlers
	adminPanel.SetMenuActions(
		func() (modal.Modal, tea.Cmd) { return m.createBanUserModal() },
		func() (modal.Modal, tea.Cmd) { return m.createBanIPModal() },
		func() (modal.Modal, tea.Cmd) { return m.createListUsersModal() },
		func() (modal.Modal, tea.Cmd) { return m.createUnbanModal() },
		func() (modal.Modal, tea.Cmd) { return m.createViewBansModal() },
		func() (modal.Modal, tea.Cmd) { return m.createDeleteUserModal() },
		func() (modal.Modal, tea.Cmd) { return m.createDeleteChannelModal() },
	)

	return adminPanel
}

// createBanUserModal creates a ban user modal with submit handler
func (m *Model) createBanUserModal() (modal.Modal, tea.Cmd) {
	banUserModal := modal.NewBanUserModal()
	banUserModal.SetSubmitHandler(func(msg *protocol.BanUserMessage) tea.Cmd {
		m.statusMessage = "Banning user..."
		return m.sendBanUser(msg)
	})
	return banUserModal, nil
}

// createBanIPModal creates a ban IP modal with submit handler
func (m *Model) createBanIPModal() (modal.Modal, tea.Cmd) {
	banIPModal := modal.NewBanIPModal()
	banIPModal.SetSubmitHandler(func(msg *protocol.BanIPMessage) tea.Cmd {
		m.statusMessage = "Banning IP..."
		return m.sendBanIP(msg)
	})
	return banIPModal, nil
}

// createUnbanModal creates an unban modal with submit handlers
func (m *Model) createUnbanModal() (modal.Modal, tea.Cmd) {
	unbanModal := modal.NewUnbanModal()
	unbanModal.SetSubmitHandlers(
		func(msg *protocol.UnbanUserMessage) tea.Cmd {
			m.statusMessage = "Unbanning user..."
			return m.sendUnbanUser(msg)
		},
		func(msg *protocol.UnbanIPMessage) tea.Cmd {
			m.statusMessage = "Unbanning IP..."
			return m.sendUnbanIP(msg)
		},
	)
	return unbanModal, nil
}

// createViewBansModal creates a view bans modal with refresh handler
func (m *Model) createViewBansModal() (modal.Modal, tea.Cmd) {
	viewBansModal := modal.NewViewBansModal()
	viewBansModal.SetRefreshHandler(func(includeExpired bool) tea.Cmd {
		return m.sendListBans(includeExpired)
	})
	// Return the modal with initial load command
	return viewBansModal, m.sendListBans(false)
}

// createListUsersModal creates a list users modal with handlers
func (m *Model) createListUsersModal() (modal.Modal, tea.Cmd) {
	listUsersModal := modal.NewListUsersModal()
	listUsersModal.SetRefreshHandler(func(includeOffline bool) tea.Cmd {
		return m.sendListUsers(includeOffline)
	})
	listUsersModal.SetBanUserHandler(func(nickname string) {
		// Create and push ban user modal with pre-filled nickname
		banModal, _ := m.createBanUserModal()
		if banUserModal, ok := banModal.(*modal.BanUserModal); ok {
			banUserModal.SetNickname(nickname)
		}
		m.modalStack.Push(banModal)
	})
	listUsersModal.SetDeleteUserHandler(func(entry modal.UserEntry) {
		// Create and push delete user modal with pre-filled values
		deleteModal, _ := m.createDeleteUserModal()
		if deleteUserModal, ok := deleteModal.(*modal.DeleteUserModal); ok {
			deleteUserModal.SetUser(entry)
		}
		m.modalStack.Push(deleteModal)
	})
	// Return the modal with initial load command (show all users by default)
	return listUsersModal, m.sendListUsers(true)
}

// createDeleteUserModal creates a delete user modal with submit handler
func (m *Model) createDeleteUserModal() (modal.Modal, tea.Cmd) {
	deleteUserModal := modal.NewDeleteUserModal()
	deleteUserModal.SetSubmitHandler(func(nickname string, userID *uint64) tea.Cmd {
		var targetID uint64
		if userID != nil {
			targetID = *userID
		} else if id, ok := m.lookupUserID(nickname); ok {
			targetID = id
		} else {
			m.errorMessage = fmt.Sprintf("Unknown user '%s'. Refresh the user list (press [R]) and try again.", nickname)
			return nil
		}

		m.statusMessage = fmt.Sprintf("Deleting user %s...", nickname)
		return m.sendDeleteUser(&protocol.DeleteUserMessage{UserID: targetID})
	})
	return deleteUserModal, nil
}

// createDeleteChannelModal creates a delete channel modal with submit handler
func (m *Model) createDeleteChannelModal() (modal.Modal, tea.Cmd) {
	deleteChannelModal := modal.NewDeleteChannelModal()

	// Convert protocol.Channel to modal.ChannelInfo
	channels := make([]modal.ChannelInfo, len(m.channels))
	for i, ch := range m.channels {
		channels[i] = modal.ChannelInfo{
			ID:   ch.ID,
			Name: ch.Name,
		}
	}
	deleteChannelModal.SetChannels(channels)

	deleteChannelModal.SetSubmitHandler(func(msg *protocol.DeleteChannelMessage) tea.Cmd {
		m.statusMessage = "Deleting channel..."
		return m.sendDeleteChannel(msg)
	})
	return deleteChannelModal, nil
}

func (m *Model) lookupUserID(nickname string) (uint64, bool) {
	if m.userDirectory == nil {
		return 0, false
	}
	key := strings.ToLower(strings.TrimSpace(nickname))
	if key == "" {
		return 0, false
	}
	id, ok := m.userDirectory[key]
	return id, ok
}

func cloneUint64Ptr(src *uint64) *uint64 {
	if src == nil {
		return nil
	}
	val := *src
	return &val
}

func makeUint64Ptr(v uint64) *uint64 {
	val := v
	return &val
}

func (m *Model) ensureChannelRoster(channelID uint64) map[uint64]presenceEntry {
	roster, ok := m.channelRoster[channelID]
	if !ok {
		roster = make(map[uint64]presenceEntry)
		m.channelRoster[channelID] = roster
	}
	return roster
}

func (m *Model) upsertChannelPresence(channelID uint64, entry presenceEntry) {
	entry.UserID = cloneUint64Ptr(entry.UserID)
	roster := m.ensureChannelRoster(channelID)
	roster[entry.SessionID] = entry
	if entry.Nickname == m.nickname {
		m.selfSessionID = makeUint64Ptr(entry.SessionID)
	}
}

func (m *Model) removeChannelPresence(channelID uint64, sessionID uint64) {
	roster, ok := m.channelRoster[channelID]
	if !ok {
		return
	}
	delete(roster, sessionID)
	if len(roster) == 0 {
		delete(m.channelRoster, channelID)
	}
	if m.selfSessionID != nil && *m.selfSessionID == sessionID {
		m.selfSessionID = nil
	}
}

func (m *Model) upsertServerPresence(entry presenceEntry) {
	entry.UserID = cloneUint64Ptr(entry.UserID)
	m.serverRoster[entry.SessionID] = entry
	if entry.Nickname == m.nickname {
		m.selfSessionID = makeUint64Ptr(entry.SessionID)
	}
	m.onlineUsers = uint32(len(m.serverRoster))
}

func (m *Model) removeServerPresence(sessionID uint64) {
	delete(m.serverRoster, sessionID)
	m.onlineUsers = uint32(len(m.serverRoster))
	if m.selfSessionID != nil && *m.selfSessionID == sessionID {
		m.selfSessionID = nil
	}
	for channelID := range m.channelRoster {
		m.removeChannelPresence(channelID, sessionID)
	}
}

func (m *Model) sortedChannelPresence(channelID uint64) []presenceEntry {
	roster, ok := m.channelRoster[channelID]
	if !ok || len(roster) == 0 {
		return nil
	}
	entries := make([]presenceEntry, 0, len(roster))
	for _, entry := range roster {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Nickname) < strings.ToLower(entries[j].Nickname)
	})
	return entries
}

func (m *Model) sortedServerPresence() []presenceEntry {
	if len(m.serverRoster) == 0 {
		return nil
	}
	entries := make([]presenceEntry, 0, len(m.serverRoster))
	for _, entry := range m.serverRoster {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Nickname) < strings.ToLower(entries[j].Nickname)
	})
	return entries
}

func (m *Model) isSelfSession(sessionID uint64) bool {
	return m.selfSessionID != nil && *m.selfSessionID == sessionID
}

func (m *Model) buildUserSidebarContent() string {
	var entries []presenceEntry
	var title string

	if m.currentChannel != nil && m.hasActiveChannel {
		entries = m.sortedChannelPresence(m.currentChannel.ID)
		title = fmt.Sprintf("Channel Users (%d)", len(entries))
	} else {
		entries = m.sortedServerPresence()
		title = fmt.Sprintf("Online Users (%d)", len(entries))
	}

	b := strings.Builder{}
	b.WriteString(UserSidebarTitleStyle.Render(title))
	b.WriteString("\n\n")

	if len(entries) == 0 {
		b.WriteString(MutedTextStyle.Render("  No users yet"))
		return b.String()
	}

	for i, entry := range entries {
		b.WriteString(m.formatPresenceEntry(entry))
		if i < len(entries)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m *Model) formatPresenceEntry(entry presenceEntry) string {
	prefix := entry.UserFlags.DisplayPrefix()
	displayName := entry.Nickname
	if !entry.IsRegistered {
		displayName = "~" + displayName
	}
	display := prefix + displayName
	if m.isSelfSession(entry.SessionID) {
		display += " (you)"
		return PresenceSelfStyle.Render(display)
	}
	return PresenceItemStyle.Render(display)
}

func (m *Model) adjustChannelUserCount(channelID uint64, delta int) {
	for i := range m.channels {
		if m.channels[i].ID == channelID {
			newCount := int(m.channels[i].UserCount) + delta
			if newCount < 0 {
				newCount = 0
			}
			m.channels[i].UserCount = uint32(newCount)
			if m.currentChannel != nil && m.currentChannel.ID == channelID {
				m.currentChannel.UserCount = uint32(newCount)
			}
			break
		}
	}
}

func (m *Model) setActiveChannel(channelID uint64) {
	if m.hasActiveChannel && m.activeChannelID == channelID {
		return
	}
	if m.hasActiveChannel && m.activeChannelID != channelID {
		m.adjustChannelUserCount(m.activeChannelID, -1)
	}
	m.adjustChannelUserCount(channelID, 1)
	m.activeChannelID = channelID
	m.hasActiveChannel = true
}

func (m *Model) clearActiveChannel() {
	if !m.hasActiveChannel {
		return
	}
	delete(m.channelRoster, m.activeChannelID)
	m.adjustChannelUserCount(m.activeChannelID, -1)
	m.hasActiveChannel = false
	m.activeChannelID = 0
}

// startDMWithUser initiates a DM with the specified user
// Shows encryption setup modal first if user doesn't have encryption set up
func (m *Model) startDMWithUser(userID *uint64, nickname string) (*Model, tea.Cmd) {
	// Determine target type
	var targetType uint8
	var targetUserID uint64

	if userID != nil {
		// Target by user ID (registered user)
		targetType = protocol.DMTargetByUserID
		targetUserID = *userID
	} else {
		// Target by nickname (anonymous user)
		targetType = protocol.DMTargetByNickname
	}

	// Show encryption setup modal first
	m.showDMEncryptionChoiceModal(targetType, targetUserID, nickname)
	return m, nil
}

// showDMEncryptionChoiceModal shows the encryption setup modal before starting a DM
func (m *Model) showDMEncryptionChoiceModal(targetType uint8, targetUserID uint64, nickname string) {
	// Check if user has SSH key (authenticated via SSH)
	hasSSHKey := m.userID != nil && m.authState == AuthStateAuthenticated

	// Check if we already have an encryption key
	hasExistingKey := m.encryptionKeyPub != nil

	encModal := modal.NewEncryptionSetupModal(modal.EncryptionSetupConfig{
		DMChannelID:    nil, // Not created yet
		Reason:         fmt.Sprintf("Set up encryption to chat securely with %s.", nickname),
		HasSSHKey:      hasSSHKey,
		HasExistingKey: hasExistingKey,
		CanSkip:        true, // Allow unencrypted DMs
		OnGenerate: func() tea.Cmd {
			// Generate key, then start DM
			return m.generateKeyAndStartDM(targetType, targetUserID, nickname)
		},
		OnUseSSH: func() tea.Cmd {
			// Derive from SSH, then start DM
			return m.deriveKeyFromSSHAndStartDM(targetType, targetUserID, nickname)
		},
		OnSkip: func() tea.Cmd {
			// Start unencrypted DM
			m.statusMessage = fmt.Sprintf("Starting DM with %s...", nickname)
			return m.sendStartDM(targetType, targetUserID, nickname, true)
		},
		OnCancel: func() tea.Cmd {
			return nil
		},
	})
	m.modalStack.Push(encModal)
}

// generateKeyAndStartDM generates a new encryption key and then starts the DM
func (m *Model) generateKeyAndStartDM(targetType uint8, targetUserID uint64, nickname string) tea.Cmd {
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

		// Send public key to server
		msg := &protocol.ProvidePublicKeyMessage{
			KeyType:   keyType,
			PublicKey: kp.PublicKey,
			Label:     label,
		}
		if err := m.conn.SendMessage(protocol.TypeProvidePublicKey, msg); err != nil {
			return ErrorMsg{Err: err}
		}

		// Return message that will update state and trigger DM start
		return DMKeyGeneratedStartMsg{
			PublicKey:    kp.PublicKey,
			PrivateKey:   kp.PrivateKey,
			TargetType:   targetType,
			TargetUserID: targetUserID,
			Nickname:     nickname,
		}
	}
}

// deriveKeyFromSSHAndStartDM derives encryption key from SSH and then starts the DM
func (m *Model) deriveKeyFromSSHAndStartDM(targetType uint8, targetUserID uint64, nickname string) tea.Cmd {
	return func() tea.Msg {
		// TODO: Access SSH private key and derive X25519 key
		// This requires access to the SSH agent or key file
		// For now, return an error
		return ErrorMsg{Err: fmt.Errorf("SSH key derivation not yet implemented")}
	}
}

// DMKeyGeneratedStartMsg is sent when encryption key is generated and DM should start
type DMKeyGeneratedStartMsg struct {
	PublicKey    [32]byte
	PrivateKey   [32]byte
	TargetType   uint8
	TargetUserID uint64
	Nickname     string
}

// persistEncryptionKey saves the encryption key to disk for future sessions.
// For registered users, keys are stored by userID.
// For anonymous users, keys are stored by nickname.
func (m *Model) persistEncryptionKey(privateKey []byte) {
	if m.keyStore == nil {
		return
	}

	serverHost := m.conn.GetAddress()

	var err error
	if m.userID != nil {
		// Registered user - store by userID
		err = m.keyStore.SaveKey(serverHost, *m.userID, privateKey)
	} else if m.nickname != "" {
		// Anonymous user - store by nickname
		err = m.keyStore.SaveAnonKey(serverHost, m.nickname, privateKey)
	}

	if err != nil && m.logger != nil {
		m.logger.Printf("Failed to persist encryption key: %v", err)
	}
}

// loadEncryptionKey loads any stored encryption key from disk.
// Returns true if a key was loaded successfully.
func (m *Model) loadEncryptionKey() bool {
	if m.keyStore == nil {
		return false
	}

	serverHost := m.conn.GetAddress()

	var privateKey []byte
	var err error

	if m.userID != nil {
		// Registered user - load by userID
		privateKey, err = m.keyStore.LoadKey(serverHost, *m.userID)
	} else if m.nickname != "" {
		// Anonymous user - load by nickname
		privateKey, err = m.keyStore.LoadAnonKey(serverHost, m.nickname)
	} else {
		return false
	}

	if err != nil {
		if m.logger != nil && err != crypto.ErrKeyNotFound {
			m.logger.Printf("Failed to load encryption key: %v", err)
		}
		return false
	}

	// Derive public key from private key
	publicKey, err := crypto.X25519PrivateToPublic(privateKey)
	if err != nil {
		if m.logger != nil {
			m.logger.Printf("Failed to derive public key: %v", err)
		}
		return false
	}

	m.encryptionKeyPriv = privateKey
	m.encryptionKeyPub = publicKey
	return true
}

// showComposeWithWarning shows the compose modal, potentially with registration warning first
func (m *Model) showComposeWithWarning(mode modal.ComposeMode, initialContent string) {
	if m.shouldShowRegistrationWarning() {
		// Show warning modal first, then compose modal when user proceeds
		m.showRegistrationWarningModal(func() tea.Cmd {
			m.showComposeModal(mode, initialContent)
			return nil
		})
	} else {
		// Go directly to compose
		m.showComposeModal(mode, initialContent)
	}
}

// ChannelListItemType represents the type of item in the channel list
type ChannelListItemType int

const (
	ChannelListItemChannel ChannelListItemType = iota
	ChannelListItemSubchannel
	ChannelListItemDM
	ChannelListItemOutgoingDM
	ChannelListItemPendingDM
)

// ChannelListItem represents an item in the channel list (channel, subchannel, DM, or pending invite)
type ChannelListItem struct {
	Type          ChannelListItemType
	ChannelIndex  int                      // Index into m.channels (for channels)
	Channel       *protocol.Channel        // The channel (for channels or parent for subchannels)
	Subchannel    *protocol.SubchannelInfo // The subchannel (nil for channels)
	DM            *DMChannel               // The DM channel (for DMs)
	DMIndex       int                      // Index into m.dmChannels
	OutgoingDM    *OutgoingDMInvite        // The outgoing DM invite we're waiting on
	OutgoingIndex int                      // Index into m.outgoingDMInvites
	PendingDM     *DMInvite                // The pending DM invite (incoming)
	PendingIndex  int                      // Index into m.pendingDMInvites
}

// IsChannel returns true if this is a regular channel
func (i *ChannelListItem) IsChannel() bool {
	return i.Type == ChannelListItemChannel
}

// getVisibleChannelListItemCount returns the total number of visible items in the channel list
func (m *Model) getVisibleChannelListItemCount() int {
	count := len(m.dmChannels)        // Active DMs at the top
	count += len(m.outgoingDMInvites) // Outgoing invites (waiting)
	count += len(m.channels)          // Regular channels
	if m.expandedChannelID != nil {
		// Add subchannels for the expanded channel
		count += len(m.subchannels)
		if m.loadingSubchannels {
			count++ // Loading indicator
		}
	}
	count += len(m.pendingDMInvites) // Pending invites at the bottom
	return count
}

// getChannelListItemAtCursor returns the item at the current cursor position
func (m *Model) getChannelListItemAtCursor() *ChannelListItem {
	flatIndex := 0

	// First: DM channels at the top
	for i := range m.dmChannels {
		if flatIndex == m.channelCursor {
			dm := m.dmChannels[i]
			return &ChannelListItem{
				Type:    ChannelListItemDM,
				DM:      &dm,
				DMIndex: i,
			}
		}
		flatIndex++
	}

	// Second: Outgoing DM invites (waiting for acceptance)
	for i := range m.outgoingDMInvites {
		if flatIndex == m.channelCursor {
			invite := m.outgoingDMInvites[i]
			return &ChannelListItem{
				Type:          ChannelListItemOutgoingDM,
				OutgoingDM:    &invite,
				OutgoingIndex: i,
			}
		}
		flatIndex++
	}

	// Third: Regular channels
	for i, channel := range m.channels {
		if flatIndex == m.channelCursor {
			ch := channel // Copy to avoid referencing loop variable
			return &ChannelListItem{
				Type:         ChannelListItemChannel,
				ChannelIndex: i,
				Channel:      &ch,
				Subchannel:   nil,
			}
		}
		flatIndex++

		// Check if this channel is expanded
		if m.expandedChannelID != nil && *m.expandedChannelID == channel.ID {
			if m.loadingSubchannels {
				if flatIndex == m.channelCursor {
					// Cursor on loading indicator - treat as on channel
					ch := channel
					return &ChannelListItem{
						Type:         ChannelListItemChannel,
						ChannelIndex: i,
						Channel:      &ch,
						Subchannel:   nil,
					}
				}
				flatIndex++
			} else {
				for j := range m.subchannels {
					if flatIndex == m.channelCursor {
						ch := channel
						sub := m.subchannels[j]
						return &ChannelListItem{
							Type:         ChannelListItemSubchannel,
							ChannelIndex: i,
							Channel:      &ch,
							Subchannel:   &sub,
						}
					}
					flatIndex++
				}
			}
		}
	}

	// Fourth: Pending DM invites at the bottom (incoming)
	for i := range m.pendingDMInvites {
		if flatIndex == m.channelCursor {
			invite := m.pendingDMInvites[i]
			return &ChannelListItem{
				Type:         ChannelListItemPendingDM,
				PendingDM:    &invite,
				PendingIndex: i,
			}
		}
		flatIndex++
	}

	return nil
}

// Message types for bubbletea

// ServerFrameMsg wraps an incoming server frame
type ServerFrameMsg struct {
	Frame *protocol.Frame
}

// PostChatMessageMsg is sent when user confirms posting a chat message
// (e.g., after dismissing registration warning modal)
type PostChatMessageMsg struct {
	Content string
}

// ErrorMsg represents an error
type ErrorMsg struct {
	Err error
}

// ConnectedMsg is sent when successfully connected or reconnected
type ConnectedMsg struct{}

// DisconnectedMsg is sent when connection is lost
type DisconnectedMsg struct {
	Err        error
	Generation uint64 // Which connection generation sent this message
}

// ReconnectingMsg is sent when attempting to reconnect
type ReconnectingMsg struct {
	Attempt int
}

// TickMsg is sent periodically
type TickMsg time.Time

// VersionCheckMsg is sent with version check results
type VersionCheckMsg struct {
	LatestVersion   string
	UpdateAvailable bool
}

// WindowSizeMsg is sent when the terminal is resized
type WindowSizeMsg struct {
	Width  int
	Height int
}

// ConnectionAttemptResultMsg is sent when an async connection attempt completes
type ConnectionAttemptResultMsg struct {
	Success bool
	Method  string
	Error   error
}

// ExecuteCommandMsg is sent when a command should be executed (from command palette)
type ExecuteCommandMsg struct {
	CommandName string
}

// PrivacyDelayTickMsg is sent during privacy delay countdown
type PrivacyDelayTickMsg struct{}

// PrivacyDelayCompleteMsg is sent when privacy delay countdown finishes
type PrivacyDelayCompleteMsg struct{}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		listenForServerFrames(m.conn, m.connGeneration), // Always listen for frames
		tickCmd(),
		m.spinner.Tick,
		checkForUpdates(m.currentVersion), // Check for updates in background
	}

	// If in directory mode, request server list (selector modal already shown in NewModel)
	if m.directoryMode {
		// Send LIST_SERVERS request
		cmds = append(cmds, m.requestServerList())
		return tea.Batch(cmds...)
	}

	// Normal mode: proceed with channel list
	// If we're starting directly at channel list (not first run), request channels
	if m.currentView == ViewChannelList {
		m.loadingChannels = true
		cmds = append(cmds, m.requestChannelList())
		// Don't send SET_NICKNAME here - wait for DelayedNicknameMsg after SERVER_CONFIG
	}

	return tea.Batch(cmds...)
}

// listenForServerFrames listens for incoming server frames and connection state changes
func listenForServerFrames(conn client.ConnectionInterface, generation uint64) tea.Cmd {
	return func() tea.Msg {
		select {
		case frame := <-conn.Incoming():
			return ServerFrameMsg{Frame: frame}
		case err := <-conn.Errors():
			return ErrorMsg{Err: err}
		case stateUpdate := <-conn.StateChanges():
			switch stateUpdate.State {
			case client.StateTypeConnected:
				return ConnectedMsg{}
			case client.StateTypeDisconnected:
				// DEBUG: Add logging here if we had access to logger
				return DisconnectedMsg{Err: stateUpdate.Err, Generation: generation}
			case client.StateTypeReconnecting:
				return ReconnectingMsg{Attempt: stateUpdate.Attempt}
			}
		}
		return nil
	}
}

// tickCmd returns a command that sends a tick message every second
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// checkForUpdates checks for available updates in the background
func checkForUpdates(currentVersion string) tea.Cmd {
	return func() tea.Msg {
		// Check for updates in background (non-blocking)
		latestVersion, err := updater.CheckLatestVersion()
		if err != nil {
			// Silently fail - don't bother user with update check failures
			return nil
		}

		updateAvailable := updater.CompareVersions(currentVersion, latestVersion)

		return VersionCheckMsg{
			LatestVersion:   latestVersion,
			UpdateAvailable: updateAvailable,
		}
	}
}

// saveAndQuit saves the last seen timestamp and returns a quit command
func (m *Model) saveAndQuit() tea.Cmd {
	// Save the current timestamp so anonymous users can get unread counts on next session
	if err := m.state.UpdateLastSeenTimestamp(); err != nil && m.logger != nil {
		m.logger.Printf("Failed to save last seen timestamp: %v", err)
	}
	return tea.Quit
}
