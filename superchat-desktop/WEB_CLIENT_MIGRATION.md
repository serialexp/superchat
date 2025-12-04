# Web Client to SolidJS Desktop Migration Checklist

This document tracks the migration of all existing web client features to the SolidJS desktop app.

## Architecture Foundation

- [ ] Create global store architecture (`src/store/app-store.ts`)
  - [ ] Connection state
  - [ ] Normalized data (channels, messages, indexes)
  - [ ] UI state (active channel, view state, compose state)
  - [ ] Traffic stats
- [ ] Create selectors for derived state (`src/store/selectors.ts`)
  - [ ] `currentChannel`
  - [ ] `currentChannelMessages`
  - [ ] `currentThreadList`
  - [ ] `currentThread` (with reply tree)
- [ ] Refactor SuperChatClient to event-based pattern
  - [ ] Add event emitter to existing client
  - [ ] Emit events for all message types
  - [ ] Keep existing methods, add event layer
- [ ] Create ProtocolBridge (`src/lib/protocol-bridge.ts`)
  - [ ] Wire NEW_MESSAGE → store
  - [ ] Wire CHANNEL_LIST → store
  - [ ] Wire JOIN_RESPONSE → store
  - [ ] Wire MESSAGE_LIST → store
  - [ ] Wire ERROR → store + toast
- [ ] Create message indexer (`src/lib/message-indexer.ts`)
  - [ ] Build threadIndex (channelId → rootMessageIds)
  - [ ] Build replyIndex (parentId → childMessageIds)

## Connection Features

- [ ] Server selection UI with status indicators
  - [ ] Display list of available servers
  - [ ] Online/offline/checking status badges
  - [ ] WS/WSS protocol badges (secure/insecure)
  - [ ] Server status probing before connection
- [ ] Server options
  - [ ] Current host detection (skip 'wails', 'localhost', '')
  - [ ] superchat.win server option
  - [ ] Custom server URL input
- [ ] Connection speed throttling selector
  - [ ] Dropdown with modem speeds (14.4k, 28.8k, 33.6k, 56k, 128k ISDN, 256k DSL, 512k, 1Mbps)
  - [ ] "No limit" option
  - [ ] Throttle send queue implementation
  - [ ] Throttle receive buffer with processing interval
- [ ] LocalStorage persistence
  - [ ] Save/restore nickname
  - [ ] Save/restore selected server index
  - [ ] Save/restore custom URL
  - [ ] Save/restore WS/WSS preference
- [ ] Connection state management
  - [ ] Connecting state with spinner
  - [ ] Connected state
  - [ ] Disconnected state
  - [ ] Error state with message display

## Layout & UI Structure

- [ ] Mobile-responsive layout
  - [ ] Hamburger menu button (display: none on desktop)
  - [ ] Sidebar slide-in animation on mobile
  - [ ] Overlay backdrop when sidebar open
  - [ ] Close sidebar on channel selection (mobile)
- [ ] Sidebar (250px width)
  - [ ] Header with user info
  - [ ] Disconnect button
  - [ ] Channel list container
  - [ ] Scroll overflow handling
- [ ] Main content area
  - [ ] Content header
    - [ ] Channel title with connection indicator (green dot)
    - [ ] Traffic stats display
    - [ ] Back button (conditional, for thread detail view)
  - [ ] Message display area (flex: 1, scrollable)
  - [ ] Compose area (sticky bottom)

## Channel Features

- [ ] Channel list rendering
  - [ ] Display all channels from CHANNEL_LIST
  - [ ] Type indicators (> for chat/type=0, # for forum/type=1)
  - [ ] Channel names
  - [ ] Active channel highlighting (left border + background)
  - [ ] Hover states
- [ ] Channel interaction
  - [ ] Click to join channel
  - [ ] Send JOIN_CHANNEL protocol message
  - [ ] Handle JOIN_RESPONSE
  - [ ] Subscribe to channel (SUBSCRIBE_CHANNEL)
  - [ ] Unsubscribe from previous channel on switch
  - [ ] Update UI state on successful join

## Message Features - Chat Channels (type=0)

- [ ] Flat message list rendering
  - [ ] Display all messages chronologically
  - [ ] Date separators (when date changes)
  - [ ] Monospace font for chat format
  - [ ] Message layout: `[time] [author] [content]`
  - [ ] Auto-scroll to bottom on new messages
  - [ ] Auto-scroll on initial load
- [ ] Virtual scrolling (for 1000+ messages)
  - [ ] Implement `useVirtualList` hook
  - [ ] Overscan 5 items above/below
  - [ ] Maintain scroll position on updates

## Message Features - Forum Channels (type=1)

- [ ] View state management
  - [ ] ThreadList view (default)
  - [ ] ThreadDetail view (when thread opened)
  - [ ] Back button visibility toggle
- [ ] Thread list view
  - [ ] Display root messages only (parent_id.present === 0)
  - [ ] Thread item styling (gradient background, hover effects)
  - [ ] Thread header (author + timestamp)
  - [ ] Thread preview (truncate content to ~80-150 chars)
  - [ ] Reply count display
  - [ ] Click to open thread
- [ ] Thread detail view
  - [ ] Display root message (highlighted, larger font)
  - [ ] Display nested replies
  - [ ] Recursive reply rendering (indented by depth)
  - [ ] Reply button on each message
  - [ ] Click reply button to set replyToId
  - [ ] Back to thread list button
  - [ ] Unsubscribe from thread on back
- [ ] Threading system
  - [ ] Build thread tree from flat message list
  - [ ] Handle parent_id references
  - [ ] Support nested replies (recursive)
  - [ ] Thread subscription (SUBSCRIBE_THREAD)
  - [ ] Thread unsubscription (UNSUBSCRIBE_THREAD)

## Compose Features

- [ ] Compose area visibility logic
  - [ ] Chat channels: always visible
  - [ ] Forum thread list: visible (for new threads)
  - [ ] Forum thread detail: only visible when replyToId set
- [ ] Textarea with dynamic placeholder
  - [ ] Chat: "Type a message..."
  - [ ] Forum thread list: "Start a new conversation..."
  - [ ] Forum thread detail (replying): "Type your reply..."
- [ ] Reply context UI
  - [ ] Show "Replying to [author]: [preview]" bar
  - [ ] Preview truncated to 50 chars
  - [ ] Cancel button (×) to clear reply
  - [ ] Highlight message being replied to (add `.reply-target` class)
  - [ ] Remove highlight when reply cancelled
- [ ] Send message handling
  - [ ] POST_MESSAGE with content
  - [ ] Include parent_id if replyToId is set
  - [ ] Clear textarea after send
  - [ ] Clear replyToId after send
  - [ ] Handle MESSAGE_POSTED response
- [ ] Keyboard shortcuts in compose area
  - [ ] Enter to send (without shift/ctrl)
  - [ ] Shift+Enter for newline
  - [ ] Escape to cancel reply

## Real-time Features

- [ ] WebSocket binary protocol handling
  - [ ] Frame assembly from fragments
  - [ ] Frame buffering (handle partial frames)
  - [ ] Expected frame length tracking
- [ ] NEW_MESSAGE broadcasts
  - [ ] Receive and parse NEW_MESSAGE
  - [ ] Add to store (messages Map)
  - [ ] Update thread/reply indexes
  - [ ] Update UI reactively
  - [ ] Auto-scroll for chat channels
  - [ ] Auto-scroll for thread detail view
- [ ] Channel subscription system
  - [ ] Send SUBSCRIBE_CHANNEL on join
  - [ ] Send UNSUBSCRIBE_CHANNEL on leave
  - [ ] Handle SUBSCRIBE_OK response
  - [ ] Track subscribedChannelId
- [ ] Thread subscription system
  - [ ] Send SUBSCRIBE_THREAD on open
  - [ ] Send UNSUBSCRIBE_THREAD on back
  - [ ] Handle SUBSCRIBE_OK response
  - [ ] Track subscribedThreadId
- [ ] Ping/pong keepalive
  - [ ] Send PING every 30 seconds
  - [ ] Handle PONG response
  - [ ] Clear interval on disconnect
- [ ] Traffic monitoring
  - [ ] Track bytesSent on every frame send
  - [ ] Track bytesReceived on every fragment receive
  - [ ] Update UI every 1 second
  - [ ] Display format: ↑{sent} ↓{received}
  - [ ] Show throttle speed indicator if throttling enabled
  - [ ] Show buffered bytes if receive buffer has data

## Keyboard Navigation (Terminal Client Pattern)

- [ ] Global keyboard handler
  - [ ] Implement `useKeyboard` hook
  - [ ] Register global keydown listener
  - [ ] Prevent default for handled keys
- [ ] Focus system
  - [ ] Always have a focused section (sidebar, message list, compose)
  - [ ] Visual selection indicator in focused section
  - [ ] Tab to switch focus between sections
- [ ] Sidebar navigation
  - [ ] j/Down arrow: move selection down
  - [ ] k/Up arrow: move selection up
  - [ ] Enter: join selected channel
  - [ ] Escape: unfocus sidebar
- [ ] Message list navigation (forum thread list)
  - [ ] j/Down arrow: move selection down
  - [ ] k/Up arrow: move selection up
  - [ ] Enter: open selected thread
  - [ ] r: reply to selected message (in thread detail)
  - [ ] Escape: back to thread list (if in thread detail)
- [ ] Compose area
  - [ ] i: focus compose textarea
  - [ ] Escape: unfocus compose, clear reply
  - [ ] Ctrl+Enter: send message (alternative to mouse click)
- [ ] Global shortcuts
  - [ ] Escape: cancel reply / back / unfocus
  - [ ] ?: show keyboard shortcuts help (future)

## Polish Features

- [ ] Toast notification system
  - [ ] Portal-based ToastContainer
  - [ ] Toast types: success, error, info
  - [ ] Auto-dismiss after 3 seconds
  - [ ] Slide-down animation
  - [ ] Multiple toasts stack vertically
- [ ] Status messages
  - [ ] Connection success
  - [ ] Connection error
  - [ ] Message send success/error
  - [ ] General error display
- [ ] Empty states
  - [ ] No channels: "Select a channel to start chatting"
  - [ ] No threads: "No threads yet / Start a conversation!"
  - [ ] No messages: "No messages yet / Start a conversation!"
- [ ] Loading states
  - [ ] Connection in progress (spinner)
  - [ ] Channel list loading
  - [ ] Message list loading
- [ ] Visual polish
  - [ ] Active channel left border (blue)
  - [ ] Thread item hover effects (lift + glow)
  - [ ] Reply target highlighting (blue glow)
  - [ ] Connection indicator (green dot) when channel active
  - [ ] Smooth transitions (0.2s)

## Error Handling

- [ ] Error boundaries
  - [ ] Wrap MainLayout in ErrorBoundary
  - [ ] Display error screen with retry button
- [ ] Protocol error handling
  - [ ] Handle ERROR message type
  - [ ] Parse error_code and message
  - [ ] Display toast notification
  - [ ] Log to console
- [ ] WebSocket error handling
  - [ ] Handle onerror event
  - [ ] Handle onclose event (clean vs dirty)
  - [ ] Clear intervals on close
  - [ ] Show connection lost message
- [ ] Message validation
  - [ ] Check minimum frame size (7 bytes)
  - [ ] Validate frame header
  - [ ] Handle decoding errors gracefully

## Data Management

- [ ] Message deduplication
  - [ ] Use message_id as Map key (automatic)
  - [ ] No duplicate messages on reconnect
- [ ] Memory management
  - [ ] Implement max messages per channel (1000)
  - [ ] LRU eviction for old messages
  - [ ] Clear messages when leaving channel (optional)
- [ ] Data normalization
  - [ ] Store channels in Map<bigint, Channel>
  - [ ] Store messages in Map<bigint, Message>
  - [ ] Build threadIndex for quick lookups
  - [ ] Build replyIndex for quick lookups

## Future Enhancements (Not Blocking Migration)

- [ ] Message editing
- [ ] Message deletion
- [ ] User registration
- [ ] SSH authentication
- [ ] DM support
- [ ] Emoji picker
- [ ] File uploads
- [ ] Search functionality
- [ ] Notification system
- [ ] Custom themes
- [ ] Keyboard shortcuts help modal

---

## Progress Tracking

**Phase 1 - Foundation:**
- [ ] Store architecture
- [ ] Event-based client
- [ ] ProtocolBridge
- [ ] Basic selectors

**Phase 2 - Layout:**
- [ ] MainLayout components
- [ ] Sidebar structure
- [ ] Channel list rendering

**Phase 3 - Chat View:**
- [ ] Flat message rendering
- [ ] Date separators
- [ ] Auto-scroll
- [ ] Compose for chat

**Phase 4 - Forum View:**
- [ ] Thread list rendering
- [ ] Thread detail rendering
- [ ] Reply system
- [ ] Navigation between views

**Phase 5 - Keyboard Navigation:**
- [ ] Focus system
- [ ] Selection indicators
- [ ] Navigation keys
- [ ] Global shortcuts

**Phase 6 - Polish:**
- [ ] Toast notifications
- [ ] Traffic stats
- [ ] Empty states
- [ ] Error boundaries

---

## Notes

- All features listed above exist in the current web client (`web-client/src/main.ts`)
- Keyboard navigation follows terminal client patterns (always focused, selection indicator)
- Mouse support is secondary - keyboard-first design
- Testing can be added after feature parity is achieved
