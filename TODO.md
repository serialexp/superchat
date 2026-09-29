# TODO

## Web Client / TUI Parity Gaps

### Missing Features

- [ ] **Admin panel** — TUI has full admin panel (ban/unban users, ban/unban IPs, delete users, view ban list). Web client has no admin UI.
- [x] **Go Anonymous** — Ctrl+A disconnects and reconnects as anonymous.
- [x] **Nickname change** — Ctrl+N opens change nickname modal.
- [x] **Password change** — Ctrl+P opens change password modal.
- [x] **Subchannel support** — TUI supports creating/browsing subchannels under parent channels.
- [ ] **Unread counts (channels)** — TUI shows per-channel unread message badges. Web client only tracks unread for DMs.
- [ ] **Message edit history** — TUI exposes edit versioning for moderation. Not exposed in web client.
- [ ] **Desktop notifications** — TUI has sound/notification when user is idle. Not implemented in web client.
- [ ] **Command palette** — TUI has `/` or `:` searchable command list. Not implemented in web client.
- [ ] **Auto-reconnect** — TUI has exponential backoff reconnection. Not implemented in web client.
- [ ] **Refresh actions** — TUI has manual refresh of channels/threads (`r` key). Not implemented in web client.
- [ ] **Sign in flow** — TUI has explicit Ctrl+S sign-in when nickname is registered. Web client has password modal but no dedicated sign-in trigger.
- [ ] **User info queries** — TUI can check if a nickname is registered. Not exposed as an action in web client.

### Deliberately Skipped (not applicable to browser)

- [x] ~~**SSH key authentication** — Terminal-native feature, browsers can't safely access private keys.~~
- [x] ~~**SSH key management** — Add/delete/relabel SSH keys (Ctrl+K). Not applicable without SSH auth.~~
- [x] ~~**SSH connection method** — Browser uses WebSocket, not SSH.~~
- [x] ~~**Client config file** — TUI uses `client-config.toml`. Web client uses localStorage/UI.~~

## Abuse Control Gaps

Found while adding the word filter (`pkg/moderation`). None of these are caused by
that change; all predate it. Of the `[limits]` block, only `max_message_length`
is actually enforced.

- [ ] **IP bans do nothing.** `BAN_IP` writes a row and the TUI admin panel lists
      them, but `GetActiveBanForIP` (`database.go:2151`, `memdb.go:1493`) has no
      callers anywhere. Nothing checks it on TCP accept, in `websocket.go`, or at
      session setup. A banned IP reconnects immediately.
- [ ] **`SET_NICKNAME` never checks bans** (`handlers.go:419`). User bans are only
      consulted in `AUTH_REQUEST` (`handlers.go:231`) and the SSH paths
      (`ssh.go:193,396`), so a banned user simply stays anonymous, takes the same
      nickname and keeps posting. Nickname-based bans are unenforceable against
      anonymous sessions.
- [ ] **`message_rate_limit` is not enforced server-side.** It only reaches clients
      via `SERVER_CONFIG.MaxMessageRate` (`server.go:740`). `handlePostMessage`
      has no rate check and `ErrCodeMessageRateLimit` (5001) is never returned.
      Honour system; `pkg/botlib` ignores it.
- [ ] **`max_connections_per_ip` is not enforced** — same advertised-only pattern.
- [ ] **Anonymous poster IPs are not visible in any UI.** This is the original
      complaint that prompted the word filter. `[filter]` log lines in
      `server.log` now carry nickname + IP for censored messages, but that only
      covers people who trip the filter. A general "show IP" affordance in the
      admin panel is still missing — and would be of limited use until IP bans
      are actually enforced (first item above).

## Word Filter Follow-ups

- [x] ~~**No retroactive pass.**~~ Both the server and the archiver now censor
      already-stored messages during startup (`moderation.Filter.Backfill`),
      controlled by `backfill_on_start` / `--word-filter-backfill`. The archiver
      regenerates HTML after, so published pages get cleaned too.
- [ ] **Nicknames are not filtered.** Someone can still register or pick a slur as
      a nickname; only message content is censored. Rejecting such nicknames in
      `handleSetNickname` is probably better than starring them.
- [ ] **Channel names/descriptions are not filtered** either. The archiver stores
      `Channel.description` and renders it into every channel page.
- [ ] **The startup pass is unconditional, not incremental.** It reads the whole
      `Message` table on every boot (writing only changed rows). That is the same
      order as MemDB's own startup load, and it means adding a term to
      `extra_words` cleans history at the next restart with no extra command —
      but on a very large archive it is wasted I/O. If that bites, record the
      highest id scanned plus a hash of the word list and only scan past it.
- [ ] **`go vet` flags pre-existing lock-copy issues in `pkg/database/memdb.go`**
      (lines ~753-834, `Message` contains a `sync/atomic.Uint32`). Unrelated to
      the filter, but `go vet ./pkg/database/` is not clean.
