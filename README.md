# SuperChat

Terminal-based threaded chat application with a custom binary protocol.

| Channel List | Thread View | Chat View |
|:---:|:---:|:---:|
| ![Channel List](website/public/superchat.png) | ![Thread View](website/public/thread-view.png) | ![Chat View](website/public/chat-view.png) |

## Features

- **Terminal UI** - Beautiful keyboard-driven interface built with Bubble Tea
- **Threaded Discussions** - Reddit/forum-style nested conversations with unlimited depth
- **Anonymous by Default** - No registration needed, pick a nickname and start chatting
- **Real-time Updates** - See new messages as they arrive
- **Vim-like Navigation** - j/k to navigate, Enter to select, Esc to go back
- **Auto-Reconnect** - Automatic reconnection with exponential backoff
- **Self-Updating** - Built-in update mechanism to keep your client current

## Installation

### One-line installer (Linux/macOS/FreeBSD)

```bash
# Install to ~/.local/bin (user install, no sudo required)
curl -fsSL https://raw.githubusercontent.com/aeolun/superchat/main/install.sh | sh

# Or install to /usr/bin (system-wide, requires sudo)
curl -fsSL https://raw.githubusercontent.com/aeolun/superchat/main/install.sh | sudo sh -s -- --global
```

This installs `sc` (client) and `scd` (server).

### Manual Installation

Download binaries from [GitHub Releases](https://github.com/aeolun/superchat/releases/latest) for your platform.

## Usage

### Client

```bash
# Connect to the default server (superchat.win)
sc

# Connect to a custom server (sc:// protocol, default port 6465)
sc --server sc://yourserver.com

# Connect over SSH (defaults to port 6466)
# SSH connection automatically signs you in and registers your SSH key
sc --server ssh://user@yourserver.com
# On first SSH connect you'll be asked to verify and accept the server's host key

# Check version
sc --version

# Update to latest version
sc update
```

### Server

```bash
# Run server with default settings
scd

# Specify custom port
scd --port 9191

# Specify custom database path
scd --db /path/to/database.db

# Check version
scd --version
```

## Configuration

### Client Configuration

Client configuration is stored at `~/.config/superchat/config.toml` (respects `XDG_CONFIG_HOME`).

Example client configuration:

```toml
[connection]
default_server = "superchat.win"
default_port = 6465
auto_reconnect = true
reconnect_max_delay_seconds = 30

[local]
state_db = "~/.local/share/superchat/state.db"
last_nickname = ""
auto_set_nickname = true

[ui]
show_timestamps = true
timestamp_format = "relative"
theme = "default"
```

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| ↑ / k | Move up |
| ↓ / j | Move down |
| Enter | Select / Open |
| n | New thread (in channel) |
| r | Reply (in thread) / Refresh |
| Esc | Go back / Cancel |
| h / ? | Toggle help |
| q | Quit (from main view) |
| Ctrl+D | Send message (in compose) |
| Ctrl+Enter | Send message (in compose) |

## Self-Updating

SuperChat includes a built-in self-update mechanism:

1. On startup, the client checks for new releases (non-intrusively)
2. If an update is available, you'll see a notification on the welcome screen
3. Run `sc update` to download and install the latest version
4. The update happens seamlessly - the client restarts automatically with the new version

The updater:
- Preserves your installation location (user vs system install)
- Handles sudo automatically if needed
- Works on Linux, macOS, and FreeBSD (Windows requires manual restart)

## Building from Source

```bash
# Install Go 1.23 or later

# Clone the repository
git clone https://github.com/aeolun/superchat.git
cd superchat

# Build (embeds version from git tags)
make build

# Run tests
make test

# Run with coverage
make coverage
```

## Architecture

SuperChat uses a three-layer architecture:

- **Protocol Layer** (`pkg/protocol/`) - Binary protocol encoding/decoding with frame-based wire format
- **Server Layer** (`pkg/server/`) - TCP server with session management and SQLite database
- **Client Layer** (`pkg/client/`) - TUI client with local state persistence and auto-reconnect

See [CLAUDE.md](CLAUDE.md) for detailed architecture documentation.

## Docker

Run the server in Docker:

```bash
docker run -d \
  --name superchat \
  -p 6465:6465 \
  -v superchat-data:/data \
  aeolun/superchat:latest
```

See [DOCKER.md](DOCKER.md) for more information.

## License

MIT License - see LICENSE file for details.

## Contributing

Contributions welcome! Please feel free to submit a Pull Request.
