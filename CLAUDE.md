# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SuperChat is a terminal-based threaded chat application with a custom binary protocol. The V1 implementation focuses on anonymous users, TCP connections, and forum-style threading. The codebase is designed for forward compatibility with V2 features (user registration, SSH, DMs, encryption) through extensible protocol design and forward-compatible database schemas.

**Current Status:** V2 Complete! 🎉
- See `docs/versions/V2.md` for V2 implementation summary
- See `docs/versions/V1.md` for V1 specification and rationale
- See `docs/versions/V3.md` for planned V3 features (not started)

## ⚠️ CRITICAL: Protocol Changes ⚠️

**NEVER modify the binary protocol without updating documentation first!**

Before making ANY changes to the protocol (adding fields, changing message types, modifying encoding):

1. **FIRST**: Update `docs/PROTOCOL.md` with the complete specification
   - **CHECK FOR DUPLICATES!** Search PROTOCOL.md for your desired message type code (e.g., `0x1C`) to ensure it's not already in use
   - Document the exact wire format
   - Specify field types, sizes, and encoding
   - Include examples of the binary layout
   - Note version compatibility implications

2. **THEN**: Implement the change in code
   - Update `pkg/protocol/messages.go`
   - Add comprehensive tests (encoding, decoding, round-trip, error cases)
   - Update both client and server handlers

3. **FINALLY**: Update this file (CLAUDE.md) if the change affects usage patterns

**Rationale**: The protocol is the contract between client and server. Undocumented protocol changes lead to:
- Client/server incompatibility
- Impossible debugging (no reference for wire format)
- Breaking changes without warning
- Lost knowledge of encoding decisions

**If you're about to modify protocol code, STOP and update PROTOCOL.md first!**

## UI Terminology (Important!)

**User often confuses these two screens - always clarify which one they mean:**

- **Splash Screen** (`ViewSplash`): First-run only welcome screen shown once. Most users never see this again.
- **Channel List Welcome Message** (`renderChannelList`): The actual "welcome" users see every time - shown on the channel list view when no channel is selected. **This is what users usually mean when they say "splash screen" or "welcome screen".**

When user asks to update the "splash" or "welcome" screen, confirm which one they mean!

## UI Layout Guidelines

**ALWAYS use flexbox layouts from `github.com/76creates/stickers/flexbox` for any UI layout work.**

There is no downside to using flexbox everywhere - it provides:
- Automatic alignment and spacing
- Consistent sizing behavior
- Proper handling of terminal dimensions
- Clean, maintainable code

**Example: Modal with flexbox layout**

```go
import "github.com/76creates/stickers/flexbox"

func (m *MyModal) Render(width, height int) string {
	// Create vertical layout
	layout := flexbox.New(width, height)

	// Row 1: Title (fixed height)
	titleRow := layout.NewRow().AddCells(
		flexbox.NewCell(1, 1).SetContent(
			lipgloss.NewStyle().Bold(true).Render("My Modal Title"),
		),
	)

	// Row 2: Content area (flexible height)
	contentHeight := height - 4 // Subtract title + footer + spacing
	contentRow := layout.NewRow().AddCells(
		flexbox.NewCell(1, contentHeight).SetContent(buildContent()),
	)

	// Row 3: Footer (fixed height)
	footerRow := layout.NewRow().AddCells(
		flexbox.NewCell(1, 1).SetContent(
			lipgloss.NewStyle().Foreground(lipgloss.Color("240")).
				Render("[Enter] Confirm  [Esc] Cancel"),
		),
	)

	layout.AddRows([]*flexbox.Row{titleRow, contentRow, footerRow})
	return layout.Render()
}
```

**Horizontal layouts (multi-column)**

```go
// Create horizontal layout for side-by-side content
contentLayout := flexbox.NewHorizontal(width, height)

// Column 1: Sidebar (ratio 1 = 25%)
sidebarCol := contentLayout.NewColumn().AddCells(
	flexbox.NewCell(1, 1).SetContent(sidebarContent),
)

// Column 2: Main content (ratio 3 = 75%)
mainCol := contentLayout.NewColumn().AddCells(
	flexbox.NewCell(1, 1).SetContent(mainContent),
)

contentLayout.AddColumns([]*flexbox.Column{sidebarCol, mainCol})
return contentLayout.Render()
```

See `pkg/client/ui/view/channel_list.go` for a complete real-world example.

## Build Commands

**IMPORTANT: Always use `make build` to build both client and server together.**

```bash
# Build both server and client (PREFERRED)
make build

# Build server only (if needed)
go build -o superchat-server ./cmd/server

# Build client only (if needed)
go build -o superchat ./cmd/client

# Run server
go run ./cmd/server

# Run client
go run ./cmd/client
```

**Note:** When implementing features that touch both client and server, always use `make build` to ensure both compile successfully.

## Logging

**Server logs:**
- `server.log` - All server activity (connections, messages, errors). Truncated on each server startup. Located in `~/.local/share/superchat/` or `$XDG_DATA_HOME/superchat/`.
- `errors.log` - Error-level logs only (append mode, persists across restarts). Located in `~/.local/share/superchat/` or `$XDG_DATA_HOME/superchat/`.
- `debug.log` - Server debug logs (when enabled). Located in `~/.local/share/superchat/` or `$XDG_DATA_HOME/superchat/`.

**Load test logs:**
- `loadtest.log` - All load test output (same as console output). Truncated on each test run. Written to the current working directory.

**Client logs:**
- `debug.log` - Client debug logs (located in `~/.config/superchat-client/` or `$XDG_CONFIG_HOME/superchat-client/`). Append mode, persists across restarts.

**Notes:**
- Server log files are written to the data directory (`~/.local/share/superchat/`) and are git-ignored.
- Loadtest log files are written to the current working directory and are git-ignored.
- Client debug.log is written to the config directory and is also git-ignored.

## Testing

```bash
# Run all tests with race detector
make test

# Generate coverage for all packages (combined report)
make coverage

# Generate HTML coverage report
make coverage-html

# Generate separate LCOV files per package (protocol, server, client)
make coverage-lcov

# Check protocol package has at least 85% coverage (enforced)
make coverage-protocol

# View coverage summary
make coverage-summary

# Run fuzzing tests
make fuzz

# Clean coverage artifacts
make clean

# Run load/performance tests
go build -o loadtest ./cmd/loadtest
./loadtest --server localhost:6465 --clients 200 --duration 10s --min-delay 100ms --max-delay 500ms
```

### Load Testing

The `loadtest` tool simulates concurrent clients to stress test the server:

**Available flags:**
- `--server` - Server address (default: "localhost:6465")
- `--clients` - Number of concurrent clients (default: 10)
- `--duration` - Test duration (default: 1m)
- `--min-delay` - Minimum delay between posts (default: 100ms)
- `--max-delay` - Maximum delay between posts (default: 1s)

**Example commands:**
```bash
# Light load: 50 clients for 10 seconds
./loadtest --server localhost:6465 --clients 50 --duration 10s

# Heavy load: 200 clients for 1 minute
./loadtest --server localhost:6465 --clients 200 --duration 1m

# Stress test: 500 clients with aggressive posting
./loadtest --server localhost:6465 --clients 500 --duration 30s --min-delay 50ms --max-delay 200ms
```

**How it works:**
- Each client connects with a randomly generated username (e.g., "partmir", "mostra")
- Picks a random channel to join
- 10% chance to create a new thread, 90% chance to reply to existing message
- Posts messages with random delays between min-delay and max-delay
- Refreshes message list every 10 posts to discover new threads

**Metrics reported:**
- Messages posted and throughput (msg/s)
- Success rate (%)
- Average response time (ms)
- Efficiency (actual vs expected throughput)
- Failure breakdown (post failures, fetch failures, timeouts, connection errors)

### Performance Profiling

Current observations:
- Local machine maxes out at ~10k concurrent connections (CPU-bound)
- Average response time: 75ms (after switching to in-memory writes with background flush)
- CPU is the limiting factor, not kernel limits

To profile:
```bash
# Add to server main.go:
import _ "net/http/pprof"
import "net/http"

# Start pprof server:
go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()

# During load test, capture 30s CPU profile:
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Check for:
# - Broadcasting overhead (N writes per message)
# - Lock contention (RWMutex with 10k goroutines)
# - Allocation pressure / GC
# - Protocol encoding cost
```

### Coverage Requirements

- **Protocol package (`pkg/protocol/*`)**: **85% coverage required** - build fails if not met
- Server package (`pkg/server/*`): 80-90% target
- Client package (`pkg/client/*`): 70-80% target

The protocol package has strict 85% coverage requirement because it's the foundation of client-server communication and must be thoroughly tested. The remaining ~15% is primarily trivial error-forwarding code in wrapper methods (e.g., `Encode()` wrappers that call `EncodeTo()`) which would require extensive busywork to test without adding meaningful value.

## Architecture

### Three-Layer Structure

1. **Protocol Layer** (`pkg/protocol/`): Binary protocol encoding/decoding
   - Frame-based wire format with length prefix
   - Message type definitions (0x01-0x98)
   - All encoding uses `EncodeTo(io.Writer)` pattern for efficiency
   - Zero message types between client→server and server→client

2. **Server Layer** (`pkg/server/`): TCP server and message handlers
   - Session management (anonymous and registered users)
   - Database operations (SQLite)
   - Real-time message broadcasting to connected clients
   - Rate limiting and timeout enforcement

3. **Client Layer** (`pkg/client/`): TUI client with bubbletea
   - Connection management with auto-reconnect
   - Local state persistence (SQLite in `~/.config/superchat-client/`)
   - Keyboard-driven navigation with vim-like keys
   - Multiple views: channel list, thread list, thread view, compose

### Protocol Design

**Wire Format:**
```
[Length (4B)][Version (1B)][Type (1B)][Flags (1B)][Payload (N bytes)]
```

- Length excludes itself, includes Version+Type+Flags+Payload
- Protocol version is **always validated** on both encode and decode
- Max frame size: 1MB
- Flags: bit 0 = compression, bit 1 = encryption

**Key Protocol Principles:**
- **V1 subset**: Not all message types are implemented in V1. Unimplemented messages return ERROR 1001.
- **Forward compatibility**: V1 clients ignore unknown server message types (for V2 compatibility)
- **Encoding pattern**: All message types use `EncodeTo(io.Writer)` and `Decode([]byte)` methods
- **Optional fields**: Use `(bool present, value)` encoding (wastes 7 bits but much simpler)

See `docs/PROTOCOL.md` for full specification.

### Database Schema

**V1 Tables** (anonymous-only):
- `Channel`: Top-level containers (admin-created only in V1)
- `Session`: Active TCP connections (no persistent users)
- `Message`: All messages with threading (parent_id, thread_depth)
- `MessageVersion`: Edit/delete history for moderation

**Key Schema Decisions:**
- `Message.author_user_id` is NULL for V1 (anonymous), populated in V2 (registered)
- `Message.thread_depth` is denormalized (0-5+) for fast display
- Soft-delete via `deleted_at`, hard-delete via retention policy
- All foreign keys have explicit CASCADE/SET NULL behavior

See `docs/DATA_MODEL.md` for full schema.

### Database Migrations

SuperChat uses an automatic migration system for schema evolution:

- **Automatic execution**: Runs on server startup before loading MemDB
- **Automatic backup**: Creates timestamped backup before applying any migrations
- **Version tracking**: `schema_migrations` table tracks applied migrations
- **Embedded files**: Migration SQL files embedded in binary via Go embed

**Migration files**: `pkg/database/migrations/<version>_<name>.sql`

**Creating a new migration:**
1. Determine next version: `sqlite3 db.db "SELECT MAX(version) FROM schema_migrations"`
2. Create file: `pkg/database/migrations/002_add_feature.sql`
3. Write SQL (use `IF NOT EXISTS` for safety)
4. Test with isolated database
5. Commit

**Backup format**: `superchat.db.backup-v<version>-<timestamp>`

See `docs/MIGRATIONS.md` for complete guide.

### Client UI Architecture

**Bubbletea Model-View-Update:**
- `Model` holds all application state (connection, server data, UI state, input state)
- `Update()` handles all events (keyboard, server messages, timer ticks)
- `View()` renders current view based on `currentView` state

**View States:**
- `ViewSplash`: First-run welcome screen
- `ViewNicknameSetup`: Nickname input modal
- `ViewChannelList`: Channel sidebar + instructions
- `ViewThreadList`: Channel sidebar + thread list (root messages)
- `ViewThreadView`: Single thread with nested replies
- `ViewCompose`: Message composition modal

**Lipgloss Width Handling:**
- `.Width(x)` sets **content width**, borders are added **on top**
- A pane with `.Width(50).Border(RoundedBorder())` renders at **52 characters** total
- Always subtract border width (2) from desired total width when using borders

**⚠️ CRITICAL: BubbleTea Command Closures and Model State**

BubbleTea's Update function works asynchronously:
1. `Update()` returns `(Model, Cmd)`
2. The returned `Model` becomes the new application state
3. Commands execute with a **copy** of the model at the time they were created

**The Problem:**
When you create a command closure (like `model.sendLeaveChannel()`), it captures the model **at that moment**. If that model's state gets updated by a later message handler (like `handleJoinResponse` setting `activeChannelID`), the command closure still has the **old model state**.

**Example Bug Pattern:**
```go
// User presses Escape in chat view
func escapeHandler(model *Model) tea.Cmd {
    // Command created here with current model state
    return model.sendLeaveChannel()  // ❌ Uses model.activeChannelID (might be 0!)
}

func (m Model) sendLeaveChannel() tea.Cmd {
    return func() tea.Msg {
        // This closure captured m.activeChannelID when command was created
        // Even if handleJoinResponse later sets activeChannelID=20,
        // this closure still sees the old value (0)
        if m.activeChannelID == 0 {
            return nil  // ❌ Returns early with stale state!
        }
        // ... send leave message ...
    }
}
```

**The Fix:**
Pass explicit parameters to command functions instead of relying on captured model state:

```go
// ✅ GOOD: Pass channel ID explicitly
func escapeHandler(model *Model) tea.Cmd {
    if model.currentChannel != nil {
        return model.sendLeaveChannel(model.currentChannel.ID)
    }
    return nil
}

func (m Model) sendLeaveChannel(channelID uint64) tea.Cmd {
    return func() tea.Msg {
        // Uses explicit parameter, not stale model state
        if channelID == 0 {
            return nil
        }
        // ... send leave message with channelID ...
    }
}
```

**Key Takeaway:**
- Model fields that get updated by server responses (`activeChannelID`, etc.) are unreliable in command closures
- Always pass explicit parameters for critical data instead of relying on captured model state
- Use synchronously-set fields (`currentChannel`, etc.) that were set in the same Update cycle, not async server responses

### Real-Time Updates

Server broadcasts these message types to relevant clients:
- `NEW_MESSAGE`: Sent to all users in a channel when someone posts
- `CHANNEL_CREATED`: Sent to all users when admin creates channel
- `SERVER_STATS`: Periodic broadcast (every 30s) with online user count

Client buffers broadcasts during message composition to avoid disrupting the user.

## Common Patterns

### Adding a New Message Type

1. Define message type constant in `pkg/protocol/messages.go`
2. Add struct with `EncodeTo(io.Writer)` and `Decode([]byte)` methods
3. Write comprehensive tests (table-driven, round-trip, error cases)
4. Add handler in `pkg/server/handlers.go` (or stub with ERROR 1001 for V2)
5. Add client handling in `pkg/client/ui/update.go`

### Protocol Encoding Pattern

All message encoding follows this pattern:
```go
func (m *MessageType) EncodeTo(w io.Writer) error {
    if err := WriteUint64(w, m.Field1); err != nil {
        return err
    }
    if err := WriteString(w, m.Field2); err != nil {
        return err
    }
    return WriteOptionalUint64(w, m.OptionalField)
}
```

Never allocate intermediate buffers - write directly to io.Writer for efficiency.

### Adding a Database Migration

**CRITICAL**: Migrations must include tests validating data integrity!

1. **Determine next version**: `sqlite3 db.db "SELECT MAX(version) FROM schema_migrations"`
2. **Create migration file**: `pkg/database/migrations/00X_description.sql`
   - Use `IF NOT EXISTS` for safety
   - Include indexes in same migration
   - Add SQL comments for complex transformations
3. **Update `migration_path_test.go`** (REQUIRED):
   - Add test case in `TestMigrationPath`
   - Create sample data in old schema (`setupData`)
   - Validate schema changes (`validateSchema`)
   - Validate data integrity (`validateData`)
   - Update `TestFullMigrationPath` if data format changes
4. **Run tests**: `go test ./pkg/database -run TestMigrationPath -v`
5. **Test with real database**: `./superchat-server --db /tmp/test.db`
6. **Commit together**: Migration SQL + test updates (never separate!)

**Example v1→v2 migration**:
```sql
-- 002_add_user_table.sql
CREATE TABLE IF NOT EXISTS User (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nickname TEXT UNIQUE NOT NULL,
    registered INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

-- Existing messages keep NULL author_user_id (anonymous)
-- No data transformation needed
```

See `docs/MIGRATIONS.md` for complete guide.

### Database Transactions

Always wrap multi-table operations in transactions:
```go
tx, err := db.Begin()
if err != nil {
    return err
}
defer tx.Rollback() // Safe to call even after Commit

// ... multiple operations ...

if err := tx.Commit(); err != nil {
    return err
}
```

Critical for: user registration, channel creation, message posting with versions.

## Version Status

**V1:** Complete ✅
- Anonymous users, TCP connections, forum threading, client-side state

**V2:** Complete ✅
- ✅ User registration with passwords
- ✅ User-created channels
- ✅ Message editing (EDIT_MESSAGE, MESSAGE_EDITED)
- ✅ SSH key authentication with auto-registration
- ✅ Chat channel type (type 0 for linear chat, type 1 for threaded forum)

**See `docs/versions/V2.md` for complete V2 feature documentation.**

**V3 (Future):**
- Subchannels (two-level hierarchy)
- Direct messages with encryption
- Message compression

**Compatibility Strategy:**
- Protocol messages 0x01-0x98 are defined but many return ERROR 1001 until implemented
- Database has nullable fields for V2 features (`author_user_id`, `subchannel_id`)
- V1 leaves these NULL, V2 populates them
- No schema migration needed for V1→V2 upgrade (additive only)

## Key Files

**Primary Documentation:**
- `docs/PROTOCOL.md`: Complete binary protocol specification
- `docs/DATA_MODEL.md`: Database schema and relationships
- `docs/MIGRATIONS.md`: Database migration system guide
- `docs/IMPROVEMENTS_ROADMAP.md`: Pre-release checklist and UX improvements

**Version Documentation:**
- `docs/versions/V1.md`: V1 feature scope and implementation (COMPLETE)
- `docs/versions/V2.md`: V2 feature summary (COMPLETE) ⭐
- `docs/versions/V3.md`: V3 planned features (NOT STARTED)

**Historical Feature Design Documentation:**
- `docs/feature_design/`: Architectural decision records and implementation plans for major features
  - `subscription_system.md`: Design and implementation of the message subscription/broadcast optimization (COMPLETED)
  - `ssh_authentication.md`: Complete SSH key authentication implementation plan (COMPLETED)
  - `loadtest_race_condition_debug.md`: Historical debugging session that fixed TCP write race condition

**Core Implementation Files:**
- `pkg/protocol/frame.go`: Frame encoding/decoding
- `pkg/protocol/messages.go`: All message type definitions
- `pkg/server/handlers.go`: Server message handlers
- `pkg/database/migrations.go`: Migration system implementation
- `pkg/database/migrations/*.sql`: Schema migration files
- `pkg/client/ui/model.go`: Client state model
- `pkg/client/ui/update.go`: Event handling and server message processing
- `pkg/client/ui/view.go`: TUI rendering

## Important Constraints

- **Max frame size**: 1MB (prevents DoS)
- **Session timeout**: 60 seconds of inactivity (client sends PING every 30s)
- **Message retention**: 7 days default (configurable per channel)
- **Rate limiting**: 10 messages/minute per session (V1 default)

## Testing Philosophy

- **Protocol package**: 90% coverage required minimum
- Use table-driven tests for all message encode/decode
- Test both success and error paths (especially io.Writer failures)
- Property-based tests (rapid) for serialization round-trips
- Fuzzing for malformed input handling
- Integration tests for multi-client real-time broadcasts
- Anything less than a 100% success rate, is _NOT_ production-ready performance. Don't suggest 
that we should accept failures just because we're operating under heavy load. Under heavy load we 
should gracefully degrade, NOT fail.