# SuperChat Binary Protocol Specification

## Overview

This document defines the binary protocol used for communication between SuperChat clients and servers. The protocol is designed to be lightweight, efficient, and easy to implement.

## Connection Types

SuperChat supports two connection methods:

1. **SSH Connection**: Automatic authentication via SSH key
2. **TCP Connection**: Direct TCP socket with manual authentication

Both use the same binary protocol after connection is established.

## Frame Format

All messages use a simple frame-based format:

```
+-------------------+-------------------+------------------+------------------+------------------------+
| Length (4 bytes)  | Version (1 byte)  | Type (1 byte)    | Flags (1 byte)   | Payload (N bytes)      |
+-------------------+-------------------+------------------+------------------+------------------------+
| uint32 big-endian | uint8             | uint8            | uint8            | variable length        |
+-------------------+-------------------+------------------+------------------+------------------------+
```

- **Length**: Total size of Version + Type + Flags + Payload (excludes the length field itself)
- **Version**: Protocol version (current version: 1)
- **Type**: Message type identifier (see Message Types below)
- **Flags**: Bit flags for compression, encryption, and future extensions
- **Payload**: Message-specific data

**Protocol Version:**
- Current protocol version is **1**
- Server sends its protocol version in SERVER_CONFIG (first field)
- Both client and server must validate version on every message

**Versioning Philosophy:**
- Protocol versions aim to be **backwards compatible via extension**, not modification
- New versions add new message types and fields, but don't change existing ones
- **Forward compatibility:** Newer clients should work with older servers
  - Client can detect server version from SERVER_CONFIG
  - Client gracefully degrades features not supported by older server
  - Client doesn't send message types the server doesn't understand
- **Backward compatibility:** Older clients may work with newer servers
  - Server can detect client version from frame headers
  - Unknown message types should be ignored or return ERROR 1001 (unsupported version)
- If a breaking change is absolutely necessary, increment major version and treat as new protocol
- Goal: Avoid forcing synchronized upgrades of all clients and servers

**Flags Byte (bits):**
- Bit 0 (rightmost): Compression (0 = uncompressed, 1 = LZ4 compressed)
- Bit 1: Encryption (0 = plaintext, 1 = encrypted payload)
- Bits 2-7: Reserved for future use (must be 0)

**Examples:**
- `0x00` = No compression, no encryption
- `0x01` = Compressed, not encrypted
- `0x02` = Not compressed, encrypted
- `0x03` = Compressed and encrypted

**Max Frame Size**: 1 MB (1,048,576 bytes) to prevent DoS attacks

**Compression:**
- Applied to the entire payload after the Flags byte
- Uses **LZ4 block format** (much faster than gzip for real-time messaging)
- Structure: `[Uncompressed Size (u32)][LZ4 Compressed Data]`
- Recommended for payloads larger than 512 bytes
- Decompress before parsing payload structure
- LZ4 chosen for low latency and minimal CPU overhead

**Encryption:**
- Applied to the entire payload after optional compression
- Used for private/DM channels with end-to-end encryption
- Encryption details covered in DM section below

## Password Security

SuperChat uses a **client-side hashing with server-side double-hashing** approach for password authentication over TCP/WebSocket connections.

### Hashing Algorithm

**Client-side hash:**
```
password_hash = argon2id(password, salt=nickname, time=3, memory=64MB, threads=4, keyLen=32)
```

**Server-side hash:**
```
stored_hash = bcrypt(password_hash, cost=10)
```

### Security Properties

**Protection against network sniffing:**
- Password never transmitted in plaintext
- Attacker observing network traffic sees `password_hash`, not the original password
- `password_hash` cannot be used to derive the original password (Argon2id is one-way)

**Protection against password reuse:**
- If attacker obtains `password_hash` from network capture, they can authenticate to SuperChat
- However, they still cannot discover the original password
- Original password remains safe for use on other sites

**Protection against database breach:**
- Database stores `stored_hash = bcrypt(password_hash)`
- Attacker must crack bcrypt to get `password_hash`
- Even with `password_hash`, they still don't have the original password
- Double-hashing provides defense-in-depth

**Limitations (TCP/WebSocket connections):**
- ❌ Vulnerable to replay attacks (captured `password_hash` can be replayed)
- ❌ Vulnerable to MITM attacks (no connection-level encryption)
- ❌ Database breach + network capture = authentication credential compromised

**Recommendation:**
- **Use SSH connections for security-sensitive deployments**
- SSH provides connection-level encryption and eliminates all the above vulnerabilities
- TCP/WebSocket are acceptable for convenience, but SSH is recommended for security

### Why Not Challenge-Response?

Challenge-response authentication was considered but rejected because:
1. Adds protocol complexity (extra round-trip, challenge state management)
2. Doesn't protect against MITM attacks (TCP/WebSocket are unencrypted anyway)
3. SSH already provides proper encrypted authentication
4. Current approach is simpler and provides adequate protection for password reuse

## Data Types

### Primitive Types

- `uint8`: 1-byte unsigned integer
- `uint16`: 2-byte unsigned integer (big-endian)
- `uint32`: 4-byte unsigned integer (big-endian)
- `uint64`: 8-byte unsigned integer (big-endian)
- `int64`: 8-byte signed integer (big-endian)
- `bool`: 1-byte (0x00 = false, 0x01 = true)

### Composite Types

**String** (length-prefixed UTF-8):
```
+-------------------+------------------------+
| Length (uint16)   | Data (N bytes UTF-8)   |
+-------------------+------------------------+
```

**Timestamp** (Unix epoch in milliseconds):
```
+-------------------+
| int64             |
+-------------------+
```
- Always represents **server time** (not client time)
- Server sets all timestamps (`created_at`, `edited_at`, `deleted_at`, etc.)
- Clients should never send timestamps (except PING for RTT calculation)
- This eliminates clock skew issues between clients and ensures consistent message ordering

**Optional Field** (nullable):
```
+-------------------+------------------------+
| Present (bool)    | Value (if present)     |
+-------------------+------------------------+
```
- Uses full byte (bool) for presence flag instead of bit-packing
- **Tradeoff**: Wastes 7 bits per optional field, but much simpler to encode/decode
- Byte-aligned fields are faster to parse and easier to implement correctly
- For a chat protocol, the bandwidth savings from bit-packing are negligible
- Simplicity and implementation speed prioritized over micro-optimization

**Compressed Payload** (when Flags bit 0 is set):
```
+---------------------------+---------------------------+
| Uncompressed Size (u32)   | LZ4 Compressed Data       |
+---------------------------+---------------------------+
```
- `Uncompressed Size`: Size of data after decompression (for buffer allocation)
- `LZ4 Compressed Data`: Payload compressed using LZ4 block format
- After decompression, parse as normal payload structure based on message type

## Message Types

### Client → Server Messages

| Type | Name | Description |
|------|------|-------------|
| 0x01 | AUTH_REQUEST | Authenticate with password |
| 0x02 | SET_NICKNAME | Set/change nickname |
| 0x03 | REGISTER_USER | Register current nickname |
| 0x04 | LIST_CHANNELS | Request channel list |
| 0x05 | JOIN_CHANNEL | Join a channel/subchannel |
| 0x06 | LEAVE_CHANNEL | Leave a channel/subchannel |
| 0x07 | CREATE_CHANNEL | Create new channel |
| 0x08 | CREATE_SUBCHANNEL | Create new subchannel |
| 0x09 | LIST_MESSAGES | Request messages (with filters) |
| 0x0A | POST_MESSAGE | Post a new message |
| 0x0B | EDIT_MESSAGE | Edit an existing message |
| 0x0C | DELETE_MESSAGE | Delete a message (soft-delete) |
| 0x0D | ADD_SSH_KEY | Add SSH public key to account |
| 0x0E | CHANGE_PASSWORD | Change user password |
| 0x0F | GET_USER_INFO | Get info about a user |
| 0x10 | PING | Keepalive ping |
| 0x11 | DISCONNECT | Graceful disconnect notification |
| 0x12 | UPDATE_SSH_KEY_LABEL | Update SSH key label |
| 0x13 | DELETE_SSH_KEY | Delete SSH key from account |
| 0x14 | LIST_SSH_KEYS | Request list of user's SSH keys |
| 0x15 | GET_SUBCHANNELS | Request subchannels for a channel |
| 0x16 | LIST_USERS | Request list of online users |
| 0x17 | LIST_CHANNEL_USERS | Request active users in a specific channel |
| 0x18 | GET_UNREAD_COUNTS | Request unread counts for specific channels |
| 0x19 | START_DM | Initiate a direct message conversation |
| 0x1A | PROVIDE_PUBLIC_KEY | Upload public key for encryption |
| 0x1B | ALLOW_UNENCRYPTED | Explicitly allow unencrypted DMs |
| 0x1C | LOGOUT | Clear authentication, become anonymous |
| 0x1D | UPDATE_READ_STATE | Update last read timestamp for a channel |
| 0x51 | SUBSCRIBE_THREAD | Subscribe to thread updates |
| 0x52 | UNSUBSCRIBE_THREAD | Unsubscribe from thread updates |
| 0x53 | SUBSCRIBE_CHANNEL | Subscribe to new threads in channel |
| 0x54 | UNSUBSCRIBE_CHANNEL | Unsubscribe from channel updates |
| 0x55 | LIST_SERVERS | Request server list from directory |
| 0x56 | REGISTER_SERVER | Register server with directory |
| 0x57 | HEARTBEAT | Directory heartbeat (keep-alive) |
| 0x58 | VERIFY_RESPONSE | Response to verification challenge |
| 0x59 | BAN_USER | Ban a user (admin only) |
| 0x5A | BAN_IP | Ban an IP address/CIDR (admin only) |
| 0x5B | UNBAN_USER | Remove user ban (admin only) |
| 0x5C | UNBAN_IP | Remove IP ban (admin only) |
| 0x5D | LIST_BANS | Request list of all bans (admin only) |
| 0x5E | DELETE_USER | Delete a user account (admin only) |
| 0x5F | DELETE_CHANNEL | Delete a channel (admin only) |

### Server → Client Messages

| Type | Name | Description |
|------|------|-------------|
| 0x81 | AUTH_RESPONSE | Authentication result |
| 0x82 | NICKNAME_RESPONSE | Nickname change result |
| 0x83 | REGISTER_RESPONSE | Registration result |
| 0x84 | CHANNEL_LIST | List of channels |
| 0x85 | JOIN_RESPONSE | Join result with channel data |
| 0x86 | LEAVE_RESPONSE | Leave confirmation |
| 0x87 | CHANNEL_CREATED | Channel creation result |
| 0x88 | SUBCHANNEL_CREATED | Subchannel creation result |
| 0x89 | MESSAGE_LIST | List of messages |
| 0x8A | MESSAGE_POSTED | Message post confirmation |
| 0x8B | MESSAGE_EDITED | Edit confirmation |
| 0x8C | MESSAGE_DELETED | Delete confirmation |
| 0x8D | NEW_MESSAGE | Real-time message notification |
| 0x8E | PASSWORD_CHANGED | Password change result |
| 0x8F | USER_INFO | User information response |
| 0x90 | PONG | Ping response |
| 0x91 | ERROR | Error response |
| 0x92 | SSH_KEY_LABEL_UPDATED | SSH key label update result |
| 0x93 | SSH_KEY_DELETED | SSH key deletion result |
| 0x94 | SSH_KEY_LIST | List of user's SSH keys |
| 0x95 | SSH_KEY_ADDED | SSH key addition result |
| 0x96 | SUBCHANNEL_LIST | List of subchannels for a channel |
| 0x97 | UNREAD_COUNTS | Unread message counts response |
| 0x98 | SERVER_CONFIG | Server configuration and limits (sent on connect) |
| 0x99 | SUBSCRIBE_OK | Subscription confirmation |
| 0x9A | USER_LIST | List of online users |
| 0x9B | SERVER_LIST | List of discoverable servers |
| 0x9C | REGISTER_ACK | Server registration acknowledgment |
| 0x9D | HEARTBEAT_ACK | Heartbeat acknowledgment |
| 0x9E | VERIFY_REGISTRATION | Verification challenge for new servers |
| 0x9F | USER_BANNED | User ban result (admin response) |
| 0xA0 | SERVER_STATS | Server statistics (user counts, etc.) |
| 0xA1 | KEY_REQUIRED | Server needs encryption key before proceeding |
| 0xA2 | DM_READY | DM channel is ready to use |
| 0xA3 | DM_PENDING | Waiting for other party to complete key setup |
| 0xA4 | DM_REQUEST | Incoming DM request from another user |
| 0xA5 | IP_BANNED | IP ban result (admin response) |
| 0xA6 | USER_UNBANNED | User unban result (admin response) |
| 0xA7 | IP_UNBANNED | IP unban result (admin response) |
| 0xA8 | BAN_LIST | List of bans response (admin) |
| 0xA9 | USER_DELETED | User deletion result (admin response) |
| 0xAA | CHANNEL_DELETED | Channel deletion result (admin response) |
| 0xAB | CHANNEL_USER_LIST | Snapshot of users currently in a channel |
| 0xAC | CHANNEL_PRESENCE | Channel join/leave notification |
| 0xAD | SERVER_PRESENCE | Server-wide presence notification |

## Message Payloads

### 0x01 - AUTH_REQUEST (Client → Server)

Used when connecting to use a registered nickname.

```
+-------------------+----------------------+
| nickname (String) | password_hash (String) |
+-------------------+----------------------+
```

**Security Note:**
- Client sends `password_hash = argon2id(password, nickname_as_salt)`
- Server performs additional hashing: `stored_hash = bcrypt(password_hash)`
- This double-hashing approach:
  - Protects password from network sniffing (password never transmitted)
  - Protects password reuse across sites (attacker with hash can't derive original password)
  - Database breach requires cracking bcrypt to get client hash (and client hash still isn't the original password)
- For true connection-level security (protection against MITM), use SSH connection instead of TCP/WebSocket

### 0x81 - AUTH_RESPONSE (Server → Client)

```
+-------------------+-------------------+----------------------+----------------------+--------------------+
| success (bool)    | user_id (uint64)  | nickname (String)    | message (String)     | user_flags (uint8) |
|                   | (only if success) | (only if success)    | (error if failed)    | (success only, optional) |
+-------------------+-------------------+----------------------+----------------------+--------------------+
```

If success:
- `user_id`: The registered user's ID
- `nickname`: The authenticated user's registered nickname
- `message`: Welcome message or empty
- `user_flags`: Optional bitfield describing user capabilities (admins, moderators, etc.). Servers SHOULD include this when known so clients can tailor privileged UI. Bits: `0x01` (admin), `0x02` (moderator). Remaining bits are reserved for future roles.
- Clients receiving an AUTH_RESPONSE without `user_flags` MUST treat the value as `0x00` (regular user) for backward compatibility.

If failed:
- `user_id`: Omitted
- `nickname`: Omitted
- `message`: Error description

**Note:** The `nickname` field was added in V2 to support SSH authentication, where the client needs to know their authenticated nickname without sending SET_NICKNAME.

### 0x02 - SET_NICKNAME (Client → Server)

Used to set or change nickname.

```
+--------------------+
| nickname (String)  |
+--------------------+
```

**Behavior:**

**Anonymous users:**
- Can change nickname freely to any available nickname
- Nickname change only affects current session
- Previous messages keep old nickname

**Registered users:**
- Can change nickname to any available (unregistered) nickname
- Nickname change updates all existing messages to show new nickname automatically
- Database UPDATE on User.nickname only (messages link via author_user_id FK)
- Message.author_nickname is only used for anonymous users (where author_user_id is NULL)
- Cannot change to a nickname already registered by another user

### 0x82 - NICKNAME_RESPONSE (Server → Client)

```
+-------------------+-------------------+
| success (bool)    | message (String)  |
+-------------------+-------------------+
```

**Response cases:**
- Nickname is registered and client is not authenticated: `success = false`, `message = "Nickname registered, password required"`
- Nickname is available: `success = true`, `message = "Nickname changed to <nickname>"` (or "Nickname set to <nickname>")
- Nickname is invalid (format): `success = false`, `message = "Invalid nickname"`
- Nickname already taken (registered user trying to change): `success = false`, `message = "Nickname already in use"`

### 0x03 - REGISTER_USER (Client → Server)

Register current nickname with a password.

```
+----------------------+
| password_hash (String) |
+----------------------+
```

**Security Note:**
- Client sends `password_hash = argon2id(password, nickname_as_salt)`
- Server performs additional hashing: `stored_hash = bcrypt(password_hash)` and stores in database
- Same double-hashing approach as AUTH_REQUEST (see 0x01 for details)
- Password never transmitted over the wire

### 0x83 - REGISTER_RESPONSE (Server → Client)

```
+-------------------+-------------------+
| success (bool)    | user_id (uint64)  |
|                   | (only if success) |
+-------------------+-------------------+
```

### 0x04 - LIST_CHANNELS (Client → Server)

Request list of channels (without subchannels).

```
+------------------------+-------------------+
| from_channel_id (u64)  | limit (u16)       |
+------------------------+-------------------+
```

**Notes:**
- `from_channel_id`: Start listing from this channel ID (exclusive). Use 0 to start from beginning.
- `limit`: Maximum number of channels to return (default/max: 1000)
- Channels returned in ascending ID order
- Client can stop reading response early if it has enough channels
- For servers with many channels, client can request in batches

### 0x84 - CHANNEL_LIST (Server → Client)

```
+----------------------+----------------+
| channel_count (u16)  | channels []    |
+----------------------+----------------+

Each channel:
+-------------------+----------------------+------------------------+------------------------+
| channel_id (u64)  | name (String)        | description (String)   | user_count (u32)       |
+-------------------+----------------------+------------------------+------------------------+
| is_operator (bool)| type (u8)            | retention_hours(u32)   | has_subchannels (bool) |
+-------------------+----------------------+------------------------+------------------------+
| subchannel_count(u16) |
+----------------------+
```

**Notes:**
- Returns public channels (private channels excluded)
- Channels returned in ascending ID order
- `has_subchannels`: true if channel has subchannels defined
- `subchannel_count`: number of subchannels (0 if none)
- To get subchannels, use GET_SUBCHANNELS request
- If `channel_count < limit`, there are no more channels to fetch

### 0x15 - GET_SUBCHANNELS (Client → Server)

Request subchannels for a specific channel.

```
+-------------------+
| channel_id (u64)  |
+-------------------+
```

### 0x96 - SUBCHANNEL_LIST (Server → Client)

```
+-------------------+----------------------+----------------+
| channel_id (u64)  | subchannel_count(u16)| subchannels [] |
+-------------------+----------------------+----------------+

Each subchannel:
+----------------------+-------------------+------------------------+------------------------+
| subchannel_id (u64)  | name (String)     | description (String)   | type (u8)              |
+----------------------+-------------------+------------------------+------------------------+
| retention_hours(u32) |
+----------------------+
```

**Notes:**
- Response includes `channel_id` so client knows which channel these subchannels belong to
- Allows concurrent requests for multiple channels' subchannels

**Type (both channel and subchannel):**
- 0x00 = chat
- 0x01 = forum

**Type Semantics:**
- `type` is a UI hint for how clients should present the channel
- **Chat (0x00)**: Intended for real-time conversation
  - Client UI may emphasize chronological message flow
  - Threading is still supported by protocol, but may be de-emphasized in UI
  - Typically paired with short retention (but not required)
- **Forum (0x01)**: Intended for threaded discussions
  - Client UI should emphasize thread structure and navigation
  - Threading is expected and encouraged
  - Typically paired with longer retention (but not required)

**Important:** All clients MUST support displaying threaded messages regardless of channel type, since the protocol allows threading in both. The type only suggests the primary UI presentation style.

**Notes:**
- `type` and `retention_hours` on channel apply when channel has no subchannels
- If channel has subchannels, each subchannel has its own `type` and `retention_hours`
- `type` and `retention_hours` are independent - any combination is valid
- Unread counts are NOT included in channel list (use GET_UNREAD_COUNTS instead)

### 0x05 - JOIN_CHANNEL (Client → Server)

```
+-------------------+-----------------------------+
| channel_id (u64)  | subchannel_id (Optional u64)|
+-------------------+-----------------------------+
```

If `subchannel_id` is not present, join the channel at the root level (for channels without subchannels).

### 0x06 - LEAVE_CHANNEL (Client → Server)

```
+-------------------+-----------------------------+
| channel_id (u64)  | subchannel_id (Optional u64)|
+-------------------+-----------------------------+
```

Requests that the server remove the session from the given channel/subchannel. When `subchannel_id` is omitted the user leaves the root-level channel subscription.

### 0x85 - JOIN_RESPONSE (Server → Client)

```
+-------------------+-------------------+----------------------+
| success (bool)    | channel_id (u64)  | subchannel_id        |
|                   |                   | (Optional u64)       |
+-------------------+-------------------+----------------------+
| message (String)  |
+-------------------+
```

If failed, `message` contains error description.

### 0x86 - LEAVE_RESPONSE (Server → Client)

```
+-------------------+-------------------+-----------------------------+-------------------+
| success (bool)    | channel_id (u64)  | subchannel_id (Optional u64)| message (String)  |
+-------------------+-------------------+-----------------------------+-------------------+
```

Acknowledges a `LEAVE_CHANNEL` request. On success, the session is no longer counted as present in the channel. On failure, `message` describes why the leave operation was rejected.

### 0x09 - LIST_MESSAGES (Client → Server)

Request messages from a channel/subchannel.

```
+-------------------+-----------------------------+------------------------+
| channel_id (u64)  | subchannel_id (Optional u64)| limit (u16)            |
+-------------------+-----------------------------+------------------------+
| before_id (Optional u64)  | parent_id (Optional u64) |
+-------------------------------+-------------------------+
| after_id (Optional u64)   |
+---------------------------+
```

**Parameters:**
- `limit`: Max messages to return (default: 50, max: 200)
- `before_id`: Return messages older than this message ID (for backward pagination)
- `parent_id`: If set, only return replies to this message (thread view)
- `after_id`: Return messages newer than this message ID (for forward pagination / catching up)

**Behavior:**
- **Without `parent_id`**: Returns root messages only (thread starters, where `parent_id = null`)
  - Sorted by `created_at` descending (newest first) when using `before_id` or neither
  - Sorted by `created_at` ascending (oldest first) when using `after_id`
  - Each message includes `reply_count` to show thread size
  - Use for displaying thread list in channel/subchannel

- **With `parent_id`**: Returns all replies under that parent message
  - Does NOT include the parent message itself (client already has it)
  - Returns the full thread tree (all descendants)
  - Sorted by thread position (depth-first traversal for proper nesting)
  - Use for displaying a single thread's conversation

**Pagination:**
- **Without `before_id` or `after_id`**: Returns most recent messages (sorted newest-first)
- **With `before_id`**: Returns messages with `id < before_id` (older messages, sorted newest-first)
- **With `after_id`**: Returns messages with `id > after_id` (newer messages, sorted oldest-first)
- **Both set**: `before_id` takes precedence, `after_id` is ignored
- Allows scrolling backwards through history (`before_id`) or catching up with new messages (`after_id`)

**Use Case - Bandwidth Optimization:**
When reopening a thread, the client can request only messages posted since last view:
1. Client caches thread replies locally with the highest message ID seen
2. When reopening thread, send `after_id` = highest cached message ID
3. Server returns only new messages (id > after_id)
4. Client merges new messages with cache and re-sorts the tree
5. This avoids re-fetching all 50+ messages every time, fetching only the few new ones

### 0x89 - MESSAGE_LIST (Server → Client)

```
+----------------------+-----------------------------+-------------------+
| channel_id (u64)     | subchannel_id (Optional u64)| parent_id         |
|                      |                             | (Optional u64)    |
+----------------------+-----------------------------+-------------------+
| message_count (u16)  | messages []                 |
+----------------------+-----------------------------+

Each message:
+-------------------+-----------------------------+-------------------+
| message_id (u64)  | channel_id (u64)            | subchannel_id     |
|                   |                             | (Optional u64)    |
+-------------------+-----------------------------+-------------------+
| parent_id (Optional u64) | author_user_id (Optional u64) |
+------------------------------+--------------------------------+
| author_nickname (String)     | content (String)               |
+------------------------------+--------------------------------+
| created_at (Timestamp) | edited_at (Optional Timestamp) |
+------------------------+--------------------------------+
| thread_depth (u8)      | reply_count (u32)              |
+------------------------+--------------------------------+
```

**Notes:**
- Response includes the request context (`channel_id`, `subchannel_id`, `parent_id`) so clients can match responses to requests
- `author_user_id` is null for anonymous users
- `thread_depth`: 0 = root, 1+ = nested
- `reply_count`: Total number of replies (all descendants)

### 0x0A - POST_MESSAGE (Client → Server)

```
+-------------------+-----------------------------+-------------------+
| channel_id (u64)  | subchannel_id (Optional u64)| parent_id         |
|                   |                             | (Optional u64)    |
+-------------------+-----------------------------+-------------------+
| content (String)  |
+-------------------+
```

If `parent_id` is set, this is a reply. Otherwise, it's a root message.

### 0x8A - MESSAGE_POSTED (Server → Client)

Confirmation that message was posted successfully.

```
+-------------------+-------------------+
| success (bool)    | message_id (u64)  |
+-------------------+-------------------+
| message (String)  |
+-------------------+
```

**Note:** The server always sends `success=true` with this message type. If message posting fails (no nickname, invalid format, message too long, etc.), the server sends an ERROR (0x91) message instead. Therefore, `message_id` is always present and valid.

### 0x8D - NEW_MESSAGE (Server → Client)

Real-time notification of a new message (pushed to all users in the channel).

Uses the same format as a single message in MESSAGE_LIST:

```
+-------------------+-----------------------------+-------------------+
| message_id (u64)  | channel_id (u64)            | subchannel_id     |
|                   |                             | (Optional u64)    |
+-------------------+-----------------------------+-------------------+
| parent_id (Optional u64) | author_user_id (Optional u64) |
+------------------------------+--------------------------------+
| author_nickname (String)     | content (String)               |
+------------------------------+--------------------------------+
| created_at (Timestamp) | edited_at (Optional Timestamp) |
+------------------------+--------------------------------+
| thread_depth (u8)      | reply_count (u32)              |
+------------------------+--------------------------------+
```

### 0x0B - EDIT_MESSAGE (Client → Server)

```
+-------------------+-------------------+
| message_id (u64)  | content (String)  |
+-------------------+-------------------+
```

Only the original author can edit a message. Admins can edit any message.

### 0x8B - MESSAGE_EDITED (Server → Client)

Confirmation of edit + real-time notification to all users.

```
+-------------------+-------------------+------------------------+
| success (bool)    | message_id (u64)  | edited_at (Timestamp)  |
|                   |                   | (only if success)      |
+-------------------+-------------------+------------------------+
| new_content (String) | message (String)                        |
| (only if success)    | (error if failed)                       |
+----------------------+------------------------------------------+
```

### 0x0C - DELETE_MESSAGE (Client → Server)

```
+-------------------+
| message_id (u64)  |
+-------------------+
```

Only the original author can delete a message. Admins can delete any message. This performs a soft-delete (sets `deleted_at`),
preserving thread structure. Original content is saved in MessageVersion for moderation.

### 0x8C - MESSAGE_DELETED (Server → Client)

Confirmation of deletion + real-time notification to all users.

```
+-------------------+-------------------+------------------------+
| success (bool)    | message_id (u64)  | deleted_at (Timestamp) |
|                   |                   | (only if success)      |
+-------------------+-------------------+------------------------+
| message (String) (error if failed)                            |
+---------------------------------------------------------------+
```

### 0x0D - ADD_SSH_KEY (Client → Server)

Add an SSH public key to the authenticated user's account.

```
+----------------------+-------------------+
| public_key (String)  | label (String)    |
+----------------------+-------------------+
```

**Notes:**
- User must be authenticated
- `public_key`: Full SSH public key (e.g., "ssh-rsa AAAA... user@host")
- `label`: Optional user-friendly name (e.g., "Work Laptop") - can be empty string
- Server parses key, computes SHA256 fingerprint, stores in SSHKey table
- Duplicate keys (same fingerprint) return error

### 0x95 - SSH_KEY_ADDED (Server → Client)

```
+-------------------+-------------------+------------------------+------------------------+
| success (bool)    | key_id (i64)      | fingerprint (String)   | error_message (String) |
|                   | (if success)      | (if success)           | (if !success)          |
+-------------------+-------------------+------------------------+------------------------+
```

### 0x1D - UPDATE_READ_STATE (Client → Server)

Update last read timestamp for a channel/subchannel.

```
+-------------------+-----------------------------+----------------------+
| channel_id (u64)  | subchannel_id (Optional u64)| timestamp (i64)      |
+-------------------+-----------------------------+----------------------+
```

**Fields:**
- `channel_id`: Channel to mark as read
- `subchannel_id`: Optional subchannel (nullable for forward compatibility)
- `timestamp`: Unix timestamp (seconds) to mark as "last read at"

**Behavior:**
- **Registered users**: Server stores in `UserChannelState` table
- **Anonymous users**: Client should handle this locally (this message is accepted but ignored by server for anonymous sessions)
- Server accepts any timestamp value (client can move read position forward or backward)
- Client UI should prompt user when leaving a channel: "Mark as read?" with options:
  - "Yes, mark read to now"
  - "No, leave as-is"
  - "Always mark read automatically" (saves preference)

**When to send:**
- When user leaves a channel (if "mark as read" is enabled)
- When user manually triggers "mark as read" action
- NOT sent automatically on every message view (too chatty)

**Anonymous users:**
- Store `last_read_at` in local client database
- Persists across sessions on same device
- Use this timestamp when requesting GET_UNREAD_COUNTS

### 0x0E - CHANGE_PASSWORD (Client → Server)

Change the authenticated user's password, or remove password for SSH-only authentication.

```
+-----------------------------+-----------------------------+
| old_password_hash (String)  | new_password_hash (String)  |
+-----------------------------+-----------------------------+
```

**Notes:**
- User must be authenticated
- `old_password_hash`: Current password hash via `argon2id(password, nickname)` (empty string for SSH-registered users changing password for first time)
- `new_password_hash`: New password hash via `argon2id(password, nickname)` (minimum 8 characters plaintext before hashing)
- **Password Removal:** Send empty string for `new_password_hash` to remove password and use SSH-only authentication
  - Only allowed if user has at least one SSH key registered
  - Once removed, user can ONLY authenticate via SSH
  - If user loses SSH keys, they will be permanently locked out (admin intervention required)
- Server validates old password hash with bcrypt, double-hashes new password hash with bcrypt for storage

### 0x8E - PASSWORD_CHANGED (Server → Client)

```
+-------------------+------------------------+
| success (bool)    | error_message (String) |
|                   | (empty if success)     |
+-------------------+------------------------+
```

### 0x12 - UPDATE_SSH_KEY_LABEL (Client → Server)

Update the label for an SSH key.

```
+-------------------+----------------------+
| key_id (i64)      | new_label (String)   |
+-------------------+----------------------+
```

### 0x92 - SSH_KEY_LABEL_UPDATED (Server → Client)

```
+-------------------+------------------------+
| success (bool)    | error_message (String) |
|                   | (empty if success)     |
+-------------------+------------------------+
```

### 0x13 - DELETE_SSH_KEY (Client → Server)

Delete an SSH key from the user's account.

```
+-------------------+
| key_id (i64)      |
+-------------------+
```

**Notes:**
- User must be authenticated
- Cannot delete last SSH key if user has no password set
- Deletion cascades in database (ON DELETE CASCADE)

### 0x93 - SSH_KEY_DELETED (Server → Client)

```
+-------------------+------------------------+
| success (bool)    | error_message (String) |
|                   | (empty if success)     |
+-------------------+------------------------+
```

### 0x14 - LIST_SSH_KEYS (Client → Server)

Request list of all SSH keys for the authenticated user.

```
(No payload - user identified from session)
```

### 0x94 - SSH_KEY_LIST (Server → Client)

```
+-------------------+----------------+
| key_count (u32)   | keys []        |
+-------------------+----------------+

Each key:
+-------------------+------------------------+----------------------+-------------------+
| key_id (i64)      | fingerprint (String)   | key_type (String)    | label (String)    |
+-------------------+------------------------+----------------------+-------------------+
| added_at (Timestamp) | last_used_at (Timestamp)                                      |
+----------------------+-------------------------------------------------------------------+
```

**Notes:**
- `key_type`: e.g., "ssh-rsa", "ssh-ed25519"
- `label`: May be empty string
- `last_used_at`: 0 if never used
- `fingerprint`: SHA256 format (e.g., "SHA256:abc123...")

### 0x18 - GET_UNREAD_COUNTS (Client → Server)

Request unread message counts for specific channels/subchannels/threads.

```
+------------------------+----------------------+----------------+
| since_timestamp (i64?) | target_count (u16)   | targets []     |
+------------------------+----------------------+----------------+

Each target:
+-------------------+-----------------------------+----------------------+
| channel_id (u64)  | subchannel_id (Optional u64)| thread_id (Optional u64)|
+-------------------+-----------------------------+----------------------+
```

**Fields:**
- `since_timestamp`: Optional unix timestamp (seconds). If present, count messages after this time. If null:
  - **Registered users**: Server uses stored `last_read_at` from `UserChannelState` table
  - **Anonymous users**: Returns error (must provide timestamp)
- `target_count`: Number of targets to request counts for
- `targets`: Array of channel/subchannel/thread identifiers
  - `channel_id`: Required - which channel to count
  - `subchannel_id`: Optional - if present, count only this subchannel
  - `thread_id`: Optional - if present, count only messages in this thread (root message ID)

**Usage patterns:**
- **Channel-wide count**: `{channel_id: 1, subchannel_id: null, thread_id: null}` - all messages in channel
- **Specific thread**: `{channel_id: 1, subchannel_id: null, thread_id: 42}` - only replies to message 42
- **Subchannel thread**: `{channel_id: 1, subchannel_id: 5, thread_id: 100}` - thread in subchannel
- **Registered user, first request**: Omit `since_timestamp`, server uses stored state
- **Anonymous user**: Must always provide `since_timestamp` (typically from local client database)

**Response:**
Server responds with UNREAD_COUNTS (0x97) message.

**Performance Note:**
- Clients should request counts for visible items only (not all channels/threads at once)
- Server queries use indexed lookups:
  - Channel-wide: `WHERE channel_id = ? AND created_at > ?`
  - Thread-specific: `WHERE thread_id = ? AND created_at > ?`
- Very fast with proper indexes

### 0x97 - UNREAD_COUNTS (Server → Client)

Response with unread message counts for requested channels/threads.

```
+----------------------+----------------+
| count_count (u16)    | counts []      |
+----------------------+----------------+

Each count entry:
+-------------------+-----------------------------+----------------------+----------------------+
| channel_id (u64)  | subchannel_id (Optional u64)| thread_id (Optional u64)| unread_count (u32)   |
+-------------------+-----------------------------+----------------------+----------------------+
```

**Fields:**
- `count_count`: Number of count entries
- `counts`: Array of unread counts per channel/subchannel/thread
- `unread_count`: Number of messages created after the reference timestamp

**Notes:**
- Sent in response to GET_UNREAD_COUNTS (0x18)
- Count is based on either:
  - The `since_timestamp` provided in the request, OR
  - The stored `last_read_at` from `UserChannelState` table (registered users only)
- If a requested target has 0 unread messages, it's still included in the response with `unread_count = 0`
- Response entries match the structure of request targets (include `thread_id` if it was requested)

### 0x0F - GET_USER_INFO (Client → Server)

Request information about a user by nickname.

```
+-------------------+
| nickname (String) |
+-------------------+
```

**Notes:**
- Can be used to check if a nickname is registered before attempting authentication
- Useful for client UI to show "Sign In" vs "Register" options dynamically
- Returns basic public information about the user

### 0x8F - USER_INFO (Server → Client)

Response with user information.

```
+-------------------+---------------------+-------------------+
| nickname (String) | is_registered(bool) | user_id           |
|                   |                     | (Optional u64)    |
+-------------------+---------------------+-------------------+
| online (bool)     |
+-------------------+
```

**Fields:**
- `nickname`: Echo back the nickname that was queried
- `is_registered`: True if this nickname belongs to a registered user (has password)
- `user_id`: Only present if `is_registered = true`, the user's ID
- `online`: True if the user is currently connected (any session with this nickname)

**Notes:**
- For anonymous users with this nickname, `is_registered = false` and `user_id` is absent
- If no user exists with this nickname (neither registered nor currently online), `online = false` and `is_registered = false`
- Client can use `is_registered` to determine whether to show sign-in or registration UI

### 0x16 - LIST_USERS (Client → Server)

Request list of online or all users.

```
+-------------------+-------------------+
| limit (u16)       | include_offline   |
|                   | (optional bool)   |
+-------------------+-------------------+
```

**Fields:**
- `limit`: Maximum number of users to return (default: 100, max: 500)
- `include_offline`: (Optional) If present and true, include offline registered users. Admin-only feature. If absent or false, only online users are returned.

**Notes:**
- By default (or when `include_offline` is false), returns only currently connected users (active sessions)
- Includes both registered and anonymous users when online-only
- When `include_offline` is true (admin only), returns all registered users regardless of online status
- Anonymous users are never included when `include_offline` is true (they have no persistent identity)
- Results sorted by connection time for online users (most recent first)
- Server returns ERROR 1005 (permission denied) if non-admin requests with `include_offline = true`

### 0x17 - LIST_CHANNEL_USERS (Client → Server)

Request a snapshot of all sessions currently present in a channel or subchannel.

```
+-------------------+-----------------------------+
| channel_id (u64)  | subchannel_id (Optional u64)|
+-------------------+-----------------------------+
```

**Notes:**
- Clients typically request this immediately after a successful `JOIN_CHANNEL` to pre-populate their roster UI.
- When `subchannel_id` is omitted, the request targets the root-level channel membership.
- Older servers may not implement this request; clients should handle `ERROR 3000` (permission denied) or `ERROR 4001` (not found) gracefully.

### 0x9A - USER_LIST (Server → Client)

Response with list of users (online or all registered users if admin requested with include_offline).

```
+-------------------+----------------+
| user_count (u16)  | users []       |
+-------------------+----------------+

Each user:
+-------------------+---------------------+-------------------+-------------+
| nickname (String) | is_registered(bool) | user_id           | online(bool)|
|                   |                     | (Optional u64)    |             |
+-------------------+---------------------+-------------------+-------------+
```

**Fields (per user):**
- `nickname`: The user's current nickname (includes ~ prefix for anonymous users in display)
- `is_registered`: True if registered user, false if anonymous
- `user_id`: Only present if `is_registered = true`
- `online`: True if user has an active session (connected)

**Notes:**
- By default (when LIST_USERS has `include_offline = false`), shows only currently connected users
- Anonymous users appear with their session nickname and `online = true`
- When admin requests with `include_offline = true`, includes all registered users with their online status
- Offline registered users appear with `is_registered = true`, `online = false`
- Same user_id may appear multiple times if user has multiple sessions (all with `online = true`)
- Useful for admin features to show all users, not just online ones

### 0xAB - CHANNEL_USER_LIST (Server → Client)

Snapshot roster for a channel/subchannel, typically sent in response to `LIST_CHANNEL_USERS`.

```
+-------------------+-----------------------------+-------------------+
| channel_id (u64)  | subchannel_id (Optional u64)| user_count (u16)  |
+-------------------+-----------------------------+-------------------+

Each user:
+-------------------+----------------------+-------------------+----------------------+------------------+
| session_id (u64)  | nickname (String)    | is_registered(bool)| user_id (Optional u64)| user_flags (u8) |
+-------------------+----------------------+-------------------+----------------------+------------------+
```

**Notes:**
- `session_id` distinguishes multiple simultaneous connections from the same account.
- `user_flags` reuses the standard bitfield (`0x01` = admin, `0x02` = moderator). Unknown bits should be ignored for forward compatibility.
- A follow-up `CHANNEL_PRESENCE` event will be sent for subsequent joins/leaves so clients can keep the roster current without polling.

### 0xAC - CHANNEL_PRESENCE (Server → Client)

Real-time join/leave event for a channel or subchannel.

```
+-------------------+-----------------------------+-------------------+----------------------+-------------------+----------------------+------------------+---------------+
| channel_id (u64)  | subchannel_id (Optional u64)| session_id (u64)  | nickname (String)    | is_registered(bool)| user_id (Optional u64)| user_flags (u8) | joined (bool) |
+-------------------+-----------------------------+-------------------+----------------------+-------------------+----------------------+------------------+---------------+
```

- `joined = true` indicates the session entered the channel (via join, subscribe promotion, or reconnect).
- `joined = false` indicates the session left or disconnected.
- Clients should treat missing support by older servers as an absence of presence updates and may fall back to manual refreshes.

### 0xAD - SERVER_PRESENCE (Server → Client)

Server-wide presence event for connection lifecycle changes.

```
+-------------------+----------------------+-------------------+----------------------+------------------+---------------+
| session_id (u64)  | nickname (String)    | is_registered(bool)| user_id (Optional u64)| user_flags (u8) | online (bool) |
+-------------------+----------------------+-------------------+----------------------+------------------+---------------+
```

- `online = true` signals a new active session; `online = false` signals termination.
- Enables clients to keep a global roster synchronized after an initial `USER_LIST` snapshot.
- Servers MAY omit this message when no listeners have requested presence updates; clients must be resilient to its absence.

### 0x1C - LOGOUT (Client → Server)

Clear the current session's authentication and become anonymous.

```
(empty message - no payload)
```

**Notes:**
- Clears `session.UserID` on the server, making the session anonymous
- Preserves the current nickname, but user can no longer perform authenticated actions
- User can still post messages as an anonymous user with their current nickname
- To fully switch identity, send LOGOUT followed by SET_NICKNAME with a new name
- No response message - operation always succeeds
- Useful for "Go Anonymous" feature or switching between registered accounts

**Behavior:**
- After logout, session becomes anonymous (user_id = NULL)
- Current nickname is preserved unless explicitly changed
- If nickname is registered, subsequent SET_NICKNAME with same name will require authentication
- User loses access to authenticated-only features (creating channels, SSH keys, etc.)

### 0x07 - CREATE_CHANNEL (Client → Server)

```
+-------------------+------------------------+------------+------------------------+
| name (String)     | description (String)   | type (u8)  | retention_hours (u32)  |
+-------------------+------------------------+------------+------------------------+
```

**Type:**
- 0x00 = chat
- 0x01 = forum

**Notes:**
- `type` and `retention_hours` are used when channel has no subchannels
- If subchannels are added later, their individual type and retention_hours take precedence

### 0x87 - CHANNEL_CREATED (Server → Client)

Response to CREATE_CHANNEL request + broadcast to all connected clients.

```
+-------------------+-------------------+------------------------+
| success (bool)    | channel_id (u64)  | name (String)          |
|                   | (only if success) | (only if success)      |
+-------------------+-------------------+------------------------+
| description (String) | type (u8)      | retention_hours (u32)  |
| (only if success)    | (success only) | (only if success)      |
+-------------------------+----------------+------------------------+
| message (String)  |
+-------------------+
```

**Broadcast behavior:**
- Sent to the creating client as confirmation
- Also broadcast to ALL other connected clients as a real-time notification
- Clients should add the new channel to their channel list
- If `success = false`, only sent to requesting client (not broadcast)

### 0x08 - CREATE_SUBCHANNEL (Client → Server)

```
+-------------------+-------------------+------------------------+
| channel_id (u64)  | name (String)     | description (String)   |
+-------------------+-------------------+------------------------+
| type (u8)         | retention_hours (u32)                      |
+-------------------+------------------------------------------------+
```

**Type:**
- 0x00 = chat
- 0x01 = forum

### 0x88 - SUBCHANNEL_CREATED (Server → Client)

Response to CREATE_SUBCHANNEL request + broadcast to all connected clients.

```
+-------------------+----------------------+-------------------+
| success (bool)    | channel_id (u64)     | subchannel_id(u64)|
|                   | (only if success)    | (only if success) |
+-------------------+----------------------+-------------------+
| name (String)     | description (String) | type (u8)         |
| (only if success) | (only if success)    | (success only)    |
+-------------------+----------------------+-------------------+
| retention_hours (u32) | message (String)                   |
| (only if success)     |                                    |
+-----------------------+------------------------------------+
```

**Broadcast behavior:**
- Sent to the creating client as confirmation
- Also broadcast to ALL other connected clients as a real-time notification
- Clients should add the new subchannel to the appropriate channel in their list
- `channel_id` indicates which channel this subchannel belongs to
- If `success = false`, only sent to requesting client (not broadcast)

### 0x19 - START_DM (Client → Server)

Initiate a direct message conversation with another user.

```
+-------------------+---------------------------+-------------------------+
| target_type (u8)  | target_id (varies)        | allow_unencrypted(bool) |
+-------------------+---------------------------+-------------------------+
```

**Target Types:**
- 0x00 = by user_id (target_id is u64 user_id, registered users only)
- 0x01 = by nickname (target_id is String, could be registered or anonymous)
- 0x02 = by session_id (target_id is u64 session_id, for anonymous users)

**allow_unencrypted:**
- If true, initiator is willing to accept unencrypted DMs
- If false, DM must be encrypted or will fail

**Notes:**
- If targeting by nickname and multiple users/sessions have that nickname, server picks first match (prefer registered users)
- For anonymous users, targeting by session_id is more reliable

### 0x1A - PROVIDE_PUBLIC_KEY (Client → Server)

Upload an X25519 public key for DM encryption.

```
+-------------------+------------------------+-------------------------+
| key_type (u8)     | public_key (32 bytes)  | label (String)          |
+-------------------+------------------------+-------------------------+
```

**Key Types:**
- 0x00 = Derived from SSH key (Ed25519 → X25519 conversion)
- 0x01 = Generated X25519 key (for password-only users)
- 0x02 = Ephemeral X25519 key (for anonymous users, session-only)

**public_key:**
- X25519 public key (32 bytes, raw format)
- Used for Diffie-Hellman key agreement with other users

**label:**
- Optional human-readable label (e.g., "laptop", "phone", "work")
- Helps users manage multiple keys

**Notes:**
- Key is stored in `User.encryption_public_key` field
- For Ed25519 SSH users: derived from SSH key (automatic)
- For password-only users: generated client-side
- For anonymous users: stored temporarily (deleted on disconnect)
- Server never receives or stores private keys
- Client stores private key in `~/.superchat/keys/` (or derives from SSH key)

### 0x1B - ALLOW_UNENCRYPTED (Client → Server)

Explicitly allow unencrypted DMs for the current user.

```
+------------------------+-------------------+
| dm_channel_id (u64)    | permanent (bool)  |
+------------------------+-------------------+
```

**dm_channel_id:**
- The ID of the DM channel this response applies to
- Provided in the DM_REQUEST or KEY_REQUIRED message
- Ensures response is matched to the correct DM request

**permanent:**
- If true, allow all future DMs to be unencrypted (sets `User.allow_unencrypted_dms = true`)
- If false, only allow for current pending DM request (one-time exception)

**Notes:**
- Used when user doesn't want to set up encryption keys
- If `permanent = true`, server stores preference in `User.allow_unencrypted_dms`
- Permanent preference can be changed later through user settings
- Anonymous users can only use `permanent = false` (no persistent preference)

### 0xA1 - KEY_REQUIRED (Server → Client)

Server needs an encryption key before proceeding with DM.

```
+-------------------+---------------------------+
| reason (String)   | dm_channel_id (Optional u64)|
+-------------------+---------------------------+
```

**reason:**
- Human-readable explanation (e.g., "DM encryption requires a key")

**dm_channel_id:**
- If present, this is for a specific DM channel
- If absent, user needs a key for general DM functionality

**Client should:**
1. Display reason to user
2. Prompt user to choose:
   - Generate local keypair (ephemeral, device-specific)
   - Add existing SSH key (paste/select from ~/.ssh/)
   - Generate new SSH key (save to ~/.ssh/)
   - Allow unencrypted (if permitted by other party)
3. Send PROVIDE_PUBLIC_KEY or ALLOW_UNENCRYPTED

### 0xA2 - DM_READY (Server → Client)

DM channel is ready to use.

```
+-------------------+-------------------+------------------------+
| channel_id (u64)  | other_user_id     | other_nickname(String) |
|                   | (Optional u64)    |                        |
+-------------------+-------------------+------------------------+
| is_encrypted(bool)| other_public_key (Optional 32 bytes)      |
+-------------------+-------------------------------------------+
```

**Notes:**
- `other_user_id` is null if other party is anonymous
- `is_encrypted` indicates whether this DM uses encryption
- `other_public_key` is the other party's X25519 public key (32 bytes)
  - Only present if `is_encrypted = true`
  - Client computes shared secret: `X25519(my_private, other_public_key)`
  - Then derives channel key via HKDF with channel_id
- Client can now use standard JOIN_CHANNEL, POST_MESSAGE, etc. on this channel

### 0xA3 - DM_PENDING (Server → Client)

Waiting for other party to complete key setup.

```
+-------------------+---------------------------+------------------------+
| dm_channel_id(u64)| waiting_for_user_id       | waiting_for_nickname   |
|                   | (Optional u64)            | (String)               |
+-------------------+---------------------------+------------------------+
| reason (String)   |
+-------------------+
```

**reason:**
- "Waiting for <nickname> to set up encryption"
- "Waiting for <nickname> to accept DM request"

**Notes:**
- Sent to initiator while waiting for recipient to respond
- Client should display waiting indicator
- Will be followed by DM_READY or ERROR

### 0xA4 - DM_REQUEST (Server → Client)

Incoming DM request from another user.

```
+-------------------+-------------------------+------------------------+
| dm_channel_id(u64)| from_user_id            | from_nickname (String) |
|                   | (Optional u64)          |                        |
+-------------------+-------------------------+------------------------+
| requires_key(bool)| initiator_allows_unencrypted (bool)            |
+-------------------+----------------------------------------------------+
```

**requires_key:**
- True if recipient needs to set up a key before DM can proceed
- False if recipient already has a key or initiator allows unencrypted

**initiator_allows_unencrypted:**
- True if initiator is willing to accept unencrypted DMs
- False if initiator requires encryption

**Client should:**
- Notify user of incoming DM request
- If `requires_key = true`, prompt for key setup or allow unencrypted
- If `requires_key = false`, can accept immediately

### 0x10 - PING (Client → Server)

Keepalive heartbeat to maintain session when idle.

```
+-------------------+
| timestamp (int64) |
+-------------------+
```

**Notes:**
- Client's local timestamp for RTT calculation
- **Session timeout:** Server disconnects if no PING received for 60 seconds
- **CRITICAL: Clients MUST send PING to keep session alive**
  - Server ONLY updates `Session.last_activity` on PING messages
  - Other messages (POST_MESSAGE, LIST_MESSAGES, etc.) do NOT reset the idle timer
  - **Send PING every 30 seconds to maintain session** (regardless of other activity)
  - Failure to send PING will result in disconnection after 60 seconds
- Connection also closed if socket dies

**Rationale:**
- Updating session activity on every message creates excessive DB writes (55% overhead)
- PING provides explicit keepalive signal that is cheap to process
- Active clients posting messages every 100ms still need PING for session tracking

### 0x90 - PONG (Server → Client)

```
+---------------------------+
| client_timestamp (int64)  |
+---------------------------+
```

Echoes back the client's timestamp.

### 0x11 - DISCONNECT (Client → Server, Server → Client)

Graceful disconnect notification. Can be sent by either client or server to signal intentional disconnect.

```
+--------------------+
| reason (Optional String)|
+--------------------+
```

**Fields:**
- `reason` (Optional): Human-readable explanation for disconnect
  - If present: A disconnect reason is provided (e.g., "Server shutting down for maintenance", "Client closing connection")
  - If absent (empty payload): Generic disconnect with no specific reason

**Direction: Client → Server:**
- Client sends DISCONNECT before closing connection gracefully
- Allows server to clean up session immediately
- No response expected from server

**Direction: Server → Client:**
- Server sends DISCONNECT before forcibly closing client connection
- Common reasons:
  - `"Server shutting down for maintenance"` - Graceful server shutdown
  - `"Session timeout"` - No activity for 60+ seconds
  - `"Protocol violation"` - Client sent malformed messages
  - `"Kicked by operator"` - Admin action
- Client should display reason to user and not attempt immediate reconnect
- Connection will be closed by server shortly after sending this message

**Notes:**
- This is a **notification only** - no acknowledgment is required or expected
- Used for clean shutdown and user feedback (vs. abrupt connection drop)
- Helps distinguish intentional disconnects from network failures
- Client should display server-provided reason before auto-reconnect
- Empty reason (`reason` field absent) is valid for simple disconnects

### 0xA0 - SERVER_STATS (Server → Client)

Response to GET_SERVER_STATS request or sent periodically as a broadcast.

```
+---------------------------+---------------------------+
| total_users_online (u32)  | total_channels (u32)      |
+---------------------------+---------------------------+
```

**Fields:**
- `total_users_online`: Current number of connected users
- `total_channels`: Total number of public channels

**Delivery:**
- Sent as response to GET_SERVER_STATS request
- Optionally broadcast periodically to all connected clients (e.g., every 30 seconds)
- Periodic broadcast allows clients to show live user counts without polling

### 0x98 - SERVER_CONFIG (Server → Client)

Server configuration and limits. Sent automatically after successful connection (after AUTH_RESPONSE or when anonymous user connects).

```
+---------------------------+---------------------------+
| protocol_version (u8)     | max_message_rate (u16)    |
+---------------------------+---------------------------+
| max_channel_creates (u16) | inactive_cleanup_days(u16)|
| (per hour)                |                           |
+---------------------------+---------------------------+
| max_connections_per_ip(u8)| max_message_length (u32)  |
+---------------------------+---------------------------+
| max_thread_subs (u16)     | max_channel_subs (u16)    |
+---------------------------+---------------------------+
| directory_enabled (bool)  |                           |
+---------------------------+---------------------------+
```

**Fields:**
- `protocol_version`: Protocol version server speaks (must match client, currently 1)
- `max_message_rate`: Maximum messages per minute per user (rate limit)
- `max_channel_creates`: Maximum channel creations per user per hour
- `inactive_cleanup_days`: Days of inactivity before user state is purged (for registered users)
- `max_connections_per_ip`: Maximum simultaneous connections allowed per IP address
- `max_message_length`: Maximum length of message content in bytes
- `max_thread_subs`: Maximum thread subscriptions per session (default: 50)
- `max_channel_subs`: Maximum channel subscriptions per session (default: 10)
- `directory_enabled`: Whether this server can provide a list of discoverable servers via LIST_SERVERS request (false = regular server, true = directory server)

**Delivery:**
- Sent once automatically after connection is established
- For anonymous users: sent immediately after socket connection
- For authenticated users: sent after successful AUTH_RESPONSE
- For SSH users: sent after SSH authentication completes

**Client Usage:**
- **MUST check protocol_version first** - disconnect if mismatch
- Use rate limit values to implement client-side rate limiting (prevent hitting server limits)
- Display cleanup policy to users so they know their data retention
- Show connection limits in error messages when appropriate
- Validate message length before sending to avoid errors

### 0x51 - SUBSCRIBE_THREAD (Client → Server)

Subscribe to real-time updates for a specific thread. When subscribed, the client will receive NEW_MESSAGE notifications for all new messages posted to this thread (including replies at any depth).

```
+-------------------+
| thread_id (u64)   |
+-------------------+
```

**Notes:**
- `thread_id`: The root message ID of the thread to subscribe to
- Server validates that the thread exists (ERROR 4003 if not found)
- Server checks subscription limit per session (ERROR 5004 if exceeded)
- Client will receive NEW_MESSAGE for any message posted under this thread root
- On success, server responds with SUBSCRIBE_OK

**Recommended client behavior:**
- Subscribe when entering a thread view
- Unsubscribe when leaving the thread view
- Track subscriptions locally to avoid duplicate subscriptions

### 0x52 - UNSUBSCRIBE_THREAD (Client → Server)

Unsubscribe from a previously subscribed thread.

```
+-------------------+
| thread_id (u64)   |
+-------------------+
```

**Notes:**
- `thread_id`: The root message ID to unsubscribe from
- No error if already unsubscribed (idempotent)
- No response sent (fire-and-forget)

### 0x53 - SUBSCRIBE_CHANNEL (Client → Server)

Subscribe to new threads in a channel or subchannel. When subscribed, the client will receive NEW_MESSAGE notifications for new root messages (thread starters) posted to this channel.

```
+-------------------+-----------------------------+
| channel_id (u64)  | subchannel_id (Optional u64)|
+-------------------+-----------------------------+
```

**Notes:**
- Subscribe to root-level messages only (not replies)
- Server validates channel exists (ERROR 4001 if not found)
- Server validates subchannel exists if provided (ERROR 4004 if not found)
- Server checks subscription limit per session (ERROR 5005 if exceeded)
- On success, server responds with SUBSCRIBE_OK

**Recommended client behavior:**
- Subscribe when viewing a channel's thread list
- Unsubscribe when leaving the channel
- Typically combined with thread subscriptions for full coverage

### 0x54 - UNSUBSCRIBE_CHANNEL (Client → Server)

Unsubscribe from a previously subscribed channel.

```
+-------------------+-----------------------------+
| channel_id (u64)  | subchannel_id (Optional u64)|
+-------------------+-----------------------------+
```

**Notes:**
- No error if already unsubscribed (idempotent)
- No response sent (fire-and-forget)

### 0x99 - SUBSCRIBE_OK (Server → Client)

Confirmation that a subscription was successful.

```
+-------------------+-------------------+-----------------------------+
| type (u8)         | id (u64)          | subchannel_id (Optional u64)|
+-------------------+-------------------+-----------------------------+
```

**Type values:**
- 1 = Thread subscription confirmed
- 2 = Channel subscription confirmed

**Fields:**
- `type`: Indicates which type of subscription was confirmed
- `id`: The ID that was subscribed to (thread_id or channel_id depending on type)
- `subchannel_id`: Only present for channel subscriptions, null for thread subscriptions

**Notes:**
- Sent in response to SUBSCRIBE_THREAD or SUBSCRIBE_CHANNEL
- Client can use this to confirm the subscription was registered
- Not sent for unsubscribe operations

## Admin Protocol Messages

All admin messages require the user to be authenticated and listed in the server's `admin_users` configuration. Non-admin users attempting to use these messages will receive an ERROR response with code 3000 (Permission denied).

### 0x59 - BAN_USER (Client → Server)

Ban a user from the server (admin only).

```
+-------------------+----------------------+-------------------+
| user_id           | nickname             | reason (String)   |
| (Optional u64)    | (Optional String)    |                   |
+-------------------+----------------------+-------------------+
| shadowban (bool)  | duration_seconds     |
|                   | (Optional u64)       |
+-------------------+----------------------+
```

**Fields:**
- `user_id`: Optional user ID to ban (for registered users)
- `nickname`: Optional nickname to ban (for anonymous or registered users)
- `reason`: Human-readable reason for the ban (required)
- `shadowban`: If true, user can post but messages only visible to them
- `duration_seconds`: Ban duration in seconds (if absent = permanent ban)

**Notes:**
- At least one of `user_id` or `nickname` must be provided
- Shadowbanned users can still see the channel and post, but their messages are filtered for other users
- All admin actions are logged in the AdminAction table with admin's nickname and IP
- Bans are checked on authentication and message posting

### 0x9F - USER_BANNED (Server → Client)

Response to BAN_USER request.

```
+-------------------+-------------------+-------------------+
| success (bool)    | ban_id (u64)      | message (String)  |
|                   | (only if success) | (error if failed) |
+-------------------+-------------------+-------------------+
```

**Fields:**
- `success`: Whether the ban was created successfully
- `ban_id`: Database ID of the created ban (only if success)
- `message`: Success message or error description

**Response cases:**
- Success: `success = true`, `ban_id = <id>`, `message = "User <nickname> banned successfully"`
- Permission denied: `success = false`, `message = "Permission denied: admin access required"`
- Invalid input: `success = false`, `message = "Must provide either UserID or Nickname"`
- Database error: `success = false`, `message = "Failed to create ban"`

### 0x5A - BAN_IP (Client → Server)

Ban an IP address or CIDR range from the server (admin only).

```
+-------------------+-------------------+----------------------+
| ip_cidr (String)  | reason (String)   | duration_seconds     |
|                   |                   | (Optional u64)       |
+-------------------+-------------------+----------------------+
```

**Fields:**
- `ip_cidr`: IP address or CIDR range (e.g., "192.168.1.100" or "10.0.0.0/24")
- `reason`: Human-readable reason for the ban (required)
- `duration_seconds`: Ban duration in seconds (if absent = permanent ban)

**Notes:**
- Accepts both single IP addresses (e.g., "192.168.1.100") and CIDR ranges (e.g., "10.0.0.0/24")
- IP bans prevent connection entirely (checked on TCP connect)
- CIDR support allows banning entire subnets
- All admin actions are logged in the AdminAction table

### 0xA5 - IP_BANNED (Server → Client)

Response to BAN_IP request.

```
+-------------------+-------------------+-------------------+
| success (bool)    | ban_id (u64)      | message (String)  |
|                   | (only if success) | (error if failed) |
+-------------------+-------------------+-------------------+
```

**Fields:**
- `success`: Whether the ban was created successfully
- `ban_id`: Database ID of the created ban (only if success)
- `message`: Success message or error description

**Response cases:**
- Success: `success = true`, `ban_id = <id>`, `message = "IP <address> banned successfully"`
- Permission denied: `success = false`, `message = "Permission denied: admin access required"`
- Invalid CIDR: `success = false`, `message = "Invalid IP or CIDR format"`
- Database error: `success = false`, `message = "Failed to create ban"`

### 0x5B - UNBAN_USER (Client → Server)

Remove a user ban (admin only).

```
+-------------------+----------------------+
| user_id           | nickname             |
| (Optional u64)    | (Optional String)    |
+-------------------+----------------------+
```

**Fields:**
- `user_id`: Optional user ID to unban (for registered users)
- `nickname`: Optional nickname to unban

**Notes:**
- At least one of `user_id` or `nickname` must be provided
- Removes all active bans for the specified user
- If user has multiple bans (shouldn't happen), removes all of them
- All admin actions are logged in the AdminAction table

### 0xA6 - USER_UNBANNED (Server → Client)

Response to UNBAN_USER request.

```
+-------------------+----------------------+-------------------+
| success (bool)    | bans_removed (u64)   | message (String)  |
|                   | (only if success)    | (error if failed) |
+-------------------+----------------------+-------------------+
```

**Fields:**
- `success`: Whether the unban was successful
- `bans_removed`: Number of bans removed (typically 1, only if success)
- `message`: Success message or error description

**Response cases:**
- Success: `success = true`, `bans_removed = 1`, `message = "User <nickname> unbanned successfully"`
- No ban found: `success = false`, `bans_removed = 0`, `message = "No active ban found for user"`
- Permission denied: `success = false`, `message = "Permission denied: admin access required"`

### 0x5C - UNBAN_IP (Client → Server)

Remove an IP ban (admin only).

```
+-------------------+
| ip_cidr (String)  |
+-------------------+
```

**Fields:**
- `ip_cidr`: IP address or CIDR range to unban (must match ban exactly)

**Notes:**
- Must match the exact IP/CIDR that was banned
- For example, if "10.0.0.0/24" was banned, must unban "10.0.0.0/24" exactly
- All admin actions are logged in the AdminAction table

### 0xA7 - IP_UNBANNED (Server → Client)

Response to UNBAN_IP request.

```
+-------------------+----------------------+-------------------+
| success (bool)    | bans_removed (u64)   | message (String)  |
|                   | (only if success)    | (error if failed) |
+-------------------+----------------------+-------------------+
```

**Fields:**
- `success`: Whether the unban was successful
- `bans_removed`: Number of bans removed (typically 1, only if success)
- `message`: Success message or error description

**Response cases:**
- Success: `success = true`, `bans_removed = 1`, `message = "IP <address> unbanned successfully"`
- No ban found: `success = false`, `bans_removed = 0`, `message = "No active ban found for IP"`
- Permission denied: `success = false`, `message = "Permission denied: admin access required"`

### 0x5D - LIST_BANS (Client → Server)

Request list of all bans (admin only).

```
+----------------------+
| include_expired(bool)|
+----------------------+
```

**Fields:**
- `include_expired`: If true, include expired bans. If false, only active bans.

**Notes:**
- Returns all bans (user bans and IP bans)
- Expired bans have `banned_until < current_time`
- Permanent bans have `banned_until = NULL`

### 0xA8 - BAN_LIST (Server → Client)

Response with list of bans.

```
+-------------------+----------------+
| ban_count (u16)   | bans []        |
+-------------------+----------------+

Each ban:
+-------------------+-------------------+----------------------+
| ban_id (u64)      | ban_type (u8)     | user_id              |
|                   |                   | (Optional u64)       |
+-------------------+-------------------+----------------------+
| nickname          | ip_cidr           | reason (String)      |
| (Optional String) | (Optional String) |                      |
+-------------------+-------------------+----------------------+
| shadowban (bool)  | banned_at (Timestamp)                   |
+-------------------+-----------------------------------------+
| banned_until      | banned_by (String)                      |
| (Optional i64)    |                                         |
+-------------------+-----------------------------------------+
```

**Ban Types:**
- 0x00 = User ban
- 0x01 = IP ban

**Fields (per ban):**
- `ban_id`: Database ID of the ban
- `ban_type`: 0x00 for user ban, 0x01 for IP ban
- `user_id`: Only present for user bans (NULL for IP bans)
- `nickname`: Nickname at time of ban (only for user bans)
- `ip_cidr`: IP or CIDR (only for IP bans)
- `reason`: Admin-provided reason for the ban
- `shadowban`: True if this is a shadowban (only for user bans)
- `banned_at`: When the ban was created (timestamp in milliseconds)
- `banned_until`: When the ban expires (optional int64 timestamp in milliseconds, NULL = permanent)
- `banned_by`: Nickname of the admin who created the ban

**Notes:**
- User bans have `user_id` and `nickname` populated, `ip_cidr` is NULL
- IP bans have `ip_cidr` populated, `user_id` and `nickname` are NULL
- Shadowban field is always present but only meaningful for user bans
- Banned_until uses optional int64 (signed) to represent timestamp in milliseconds

### 0x5E - DELETE_USER (Client → Server)

Delete a user account permanently (admin only). Messages by the deleted user are anonymized (author_user_id set to NULL).

```
+-------------------+
| user_id (u64)     |
+-------------------+
```

**Fields:**
- `user_id`: User ID to delete

**Notes:**
- Admin-only operation (requires user_flags = 1)
- All messages by the user are anonymized (preserves content, sets author_user_id=NULL)
- All active sessions for the user are disconnected
- Cascades to delete SSH keys, sessions, and bans
- All admin actions are logged in the AdminAction table
- Admins cannot delete their own account

**Error cases:**
- Non-admin user: ERROR 1003 (Permission denied)
- User not found: `success = false`, `message = "User not found"`
- Self-deletion attempt: `success = false`, `message = "Cannot delete your own account"`

### 0xA9 - USER_DELETED (Server → Client)

Response to DELETE_USER request + broadcast to all connected clients.

```
+-------------------+-------------------+
| success (bool)    | message (String)  |
+-------------------+-------------------+
```

**Fields:**
- `success`: Whether the deletion was successful
- `message`: Success message or error description

**Response cases:**
- Success: `success = true`, `message = "User '<nickname>' deleted successfully (messages anonymized, N sessions disconnected)"`
- Permission denied: `success = false`, `message = "Permission denied: admin access required"`
- User not found: `success = false`, `message = "User not found"`
- Self-deletion: `success = false`, `message = "Cannot delete your own account"`

**Notes:**
- Broadcast to all connected clients so they can update their user lists
- Clients should handle the user being removed from their local cache

### 0x5F - DELETE_CHANNEL (Client → Server)

Delete a channel permanently (admin only). This will cascade delete all associated messages and subchannels.

```
+-------------------+----------------------+
| channel_id (u64)  | reason (String)      |
+-------------------+----------------------+
```

**Fields:**
- `channel_id`: Channel ID to delete
- `reason`: Admin-provided reason for deletion

**Notes:**
- Admin-only operation (requires user_flags = 1)
- Cascades to delete all messages, subchannels, and subscriptions
- All admin actions are logged in the AdminAction table
- Users currently in the deleted channel will receive an error on their next action

**Error cases:**
- Non-admin user: ERROR 1003 (Permission denied)
- Channel not found: ERROR 1004 (Channel not found)
- Invalid channel ID: ERROR 1002 (Invalid format)

### 0xAA - CHANNEL_DELETED (Server → Client)

Response to DELETE_CHANNEL request + broadcast to all connected clients.

```
+-------------------+-------------------+-------------------+
| success (bool)    | channel_id (u64)  | message (String)  |
+-------------------+-------------------+-------------------+
```

**Fields:**
- `success`: Whether the deletion was successful
- `channel_id`: ID of the deleted channel
- `message`: Success message or error description

**Response cases:**
- Success: `success = true`, `message = "Channel <name> deleted successfully"`
- Permission denied: `success = false`, `message = "Permission denied: admin access required"`
- Channel not found: `success = false`, `message = "Channel not found"`

**Notes:**
- Broadcast to all connected clients so they can update their channel lists
- Clients should remove the channel from their local cache

### 0x91 - ERROR (Server → Client)

Generic error response.

```
+-------------------+-------------------+
| error_code (u16)  | message (String)  |
+-------------------+-------------------+
```

**Error Code Categories (1000-9999):**

**1xxx - Protocol Errors:**
- 1000: Invalid message format
- 1001: Unsupported protocol version
- 1002: Invalid frame (malformed, oversized, etc.)
- 1003: Compression error
- 1004: Encryption error

**2xxx - Authentication Errors:**
- 2000: Authentication required
- 2001: Invalid credentials
- 2002: User already exists (registration)
- 2003: SSH key already registered
- 2004: Session expired

**3xxx - Authorization Errors:**
- 3000: Permission denied
- 3001: Not channel operator
- 3002: Not message author
- 3003: Channel is private

**4xxx - Resource Errors:**
- 4000: Resource not found
- 4001: Channel not found
- 4002: Message not found
- 4003: Thread not found
- 4004: Subchannel not found

**5xxx - Rate Limit Errors:**
- 5000: Rate limit exceeded (general)
- 5001: Message rate limit exceeded
- 5002: Channel creation rate limit exceeded
- 5003: Too many connections from IP
- 5004: Thread subscription limit exceeded (max 50 per session)
- 5005: Channel subscription limit exceeded (max 10 per session)

**6xxx - Validation Errors:**
- 6000: Invalid input
- 6001: Message too long
- 6002: Invalid channel name
- 6003: Invalid nickname
- 6004: Nickname already taken

**9xxx - Server Errors:**
- 9000: Internal server error
- 9001: Database error
- 9002: Service unavailable

## Server Discovery Protocol

SuperChat supports server discovery through directory services. Any SuperChat server can optionally act as a directory by enabling discovery mode. Servers can announce themselves to directories, and clients can browse available servers. This enables a federated-style discovery model similar to Mastodon's instance list, while keeping servers completely independent.

**Key Concepts:**
- **Directory**: Any SuperChat server running in directory mode (accepts REGISTER_SERVER requests and maintains a list of known servers)
- **Discoverable Server**: Any SuperChat server that announces itself to one or more directories
- **Client Discovery**: Clients connect to a directory to browse available servers, then disconnect and connect to chosen server

**Important:** A directory is just a regular SuperChat server with directory mode enabled. The same server binary provides both chat functionality and optional directory services. For example, `superchat.win:6465` serves as both a chat server AND a directory.

**Directory Configuration:**
- Clients maintain a list of directories (default: `superchat.win:6465`)
- Any server can enable directory mode (via `--enable-directory` flag)
- Servers can announce to multiple directories (via `--announce-to` flag)

**Directory Gossip Protocol:**
- Directories periodically query registered servers with LIST_SERVERS
- If a registered server is also a directory, it returns its known servers
- The querying directory discovers new servers and can register to them
- This creates a self-sustaining mesh network of directories
- If the primary directory (e.g., superchat.win) goes down, other directories continue operating
- Gossip interval: every 1-6 hours (configurable, randomized to avoid thundering herd)

**Anti-Spam Measures:**
- **Verification Challenge**: Directories connect back to verify servers are real and reachable
- **Rate Limiting**: 30 requests/hour per IP (enough for heartbeats + retries)
- **Adaptive Heartbeat**: Directories adjust heartbeat interval based on load
- **Deduplication**: Only one entry per hostname:port (re-registration updates existing)

**Trust Model:**
- Verification ensures servers are reachable, but doesn't prevent all abuse
- A malicious actor could still spin up many real servers to flood directories
- We rely on economic disincentives: running real servers is expensive and tedious
- The barrier to entry (actual server infrastructure) deters casual spam
- Directories operated by trusted community members are preferred

### 0x55 - LIST_SERVERS (Client/Directory → Server)

Request list of discoverable servers from a directory.

```
+-------------------+
| limit (u16)       |
+-------------------+
```

**Notes:**
- `limit`: Maximum number of servers to return (default: 100, max: 500)
- Returns servers sorted by last heartbeat (most recently active first)
- Only returns servers that have sent heartbeat within their interval window
- Can be sent by clients (browsing servers) or by directories (gossip protocol)
- Regular chat servers (not in directory mode) respond with empty SERVER_LIST (count: 0)

### 0x9B - SERVER_LIST (Server → Client/Directory)

Response with list of discoverable servers.

```
+-------------------+----------------+
| server_count(u16) | servers []     |
+-------------------+----------------+

Each server:
+-------------------+-------------------+-------------------+
| hostname (String) | port (u16)        | name (String)     |
+-------------------+-------------------+-------------------+
| description (String)                | user_count (u32)  |
+-------------------------------------+-------------------+
| max_users (u32)   | uptime_seconds (u64)                |
+-------------------+------------------------------------- +
| is_public (bool)  | channel_count (u32)                 |
+-------------------+-------------------------------------+
```

**Fields:**
- `hostname`: DNS hostname or IP address
- `port`: TCP port (typically 6465)
- `name`: Human-readable server name
- `description`: Server description/purpose
- `user_count`: Current number of connected users
- `max_users`: Maximum user capacity (0 = unlimited)
- `uptime_seconds`: How long server has been running
- `is_public`: Whether server accepts public registrations
- `channel_count`: Number of channels available on the server

**Notes:**
- Sorted by last heartbeat time (most recent first)
- Only includes servers with recent heartbeats (within 2x heartbeat interval)

### 0x56 - REGISTER_SERVER (Server → Directory)

Register or update server entry in directory.

```
+-------------------+-------------------+-------------------+
| hostname (String) | port (u16)        | name (String)     |
+-------------------+-------------------+-------------------+
| description (String)                | max_users (u32)   |
+-------------------------------------+-------------------+
| is_public (bool)  | channel_count (u32)                 |
+-------------------+-------------------------------------+
```

**Fields:**
- `hostname`: Server's publicly accessible hostname or IP
- `port`: Server's port (must be reachable from directory)
- `name`: Human-readable server name (e.g., "Gaming Community")
- `description`: Server description/purpose
- `max_users`: Maximum user capacity (0 = unlimited)
- `is_public`: Whether server accepts public registrations
- `channel_count`: Number of channels available on the server

**Behavior:**
- If hostname:port already registered: updates existing entry
- If new registration: triggers verification challenge (VERIFY_REGISTRATION)
- Directory may reject if rate limit exceeded (30/hour)

**Notes:**
- Server must respond to VERIFY_REGISTRATION challenge to complete registration
- After successful registration, server must send HEARTBEAT periodically
- Failed verification removes server from directory

### 0x9C - REGISTER_ACK (Directory → Server)

Acknowledgment of server registration with heartbeat interval.

```
+-------------------+------------------------+-------------------+
| success (bool)    | heartbeat_interval(u32)| message (String)  |
|                   | (only if success)      | (error if failed) |
+-------------------+------------------------+-------------------+
```

**Fields:**
- `success`: Whether registration was successful
- `heartbeat_interval`: Seconds between heartbeats (e.g., 300 = 5 minutes)
- `message`: Error description if failed, or welcome message if success

**Heartbeat Interval:**
- Directory calculates based on current load (number of registered servers)
- Typical values: 300s (5 min), 600s (10 min), 1800s (30 min), 3600s (1 hour)
- Server must send HEARTBEAT before interval expires or be removed

**Error Cases:**
- Rate limit exceeded: `success = false`, `message = "Rate limit exceeded"`
- Invalid hostname/port: `success = false`, `message = "Invalid hostname or port"`
- Verification failed: `success = false`, `message = "Could not verify server"`

### 0x9E - VERIFY_REGISTRATION (Directory → Server)

Challenge sent to verify server is reachable and authentic.

```
+-------------------+
| challenge (u64)   |
+-------------------+
```

**Fields:**
- `challenge`: Random 64-bit nonce

**Behavior:**
- Directory connects to server's hostname:port
- Sends VERIFY_REGISTRATION with random challenge
- Server must respond with VERIFY_RESPONSE containing same challenge
- If response matches, registration is confirmed (server added to directory)
- If connection fails or response incorrect, registration is rejected

**Two Use Cases:**

1. **Initial Registration** (server-initiated):
   - Server sends REGISTER_SERVER to directory
   - Directory immediately verifies by sending VERIFY_REGISTRATION
   - If verification succeeds, server is added and receives REGISTER_ACK
   - If verification fails, server receives REGISTER_ACK with `success = false`

2. **Gossip Discovery** (directory-initiated):
   - Directory A discovers Server C through gossip from Directory B
   - Directory A connects to Server C and sends VERIFY_REGISTRATION
   - If verification succeeds, Server C is added to Directory A's list (no REGISTER_SERVER needed)
   - If verification fails, Directory A ignores Server C
   - Server C is NOT notified of successful gossip-based addition (silent verification)

**Notes:**
- Prevents registering fake/unreachable servers
- Prevents malicious directories from injecting fake servers through gossip
- Directory times out after 10 seconds if no response
- Servers must respond to VERIFY_REGISTRATION from any directory (not just ones they registered to)

### 0x58 - VERIFY_RESPONSE (Server → Directory)

Response to verification challenge.

```
+-------------------+
| challenge (u64)   |
+-------------------+
```

**Fields:**
- `challenge`: Echo back the challenge from VERIFY_REGISTRATION

**Behavior:**
- Server receives VERIFY_REGISTRATION on its main listener
- Immediately responds with VERIFY_RESPONSE containing same challenge
- Directory verifies challenge matches and completes registration

**Notes:**
- Must be sent within 10 seconds of receiving VERIFY_REGISTRATION
- Failure to respond correctly removes server from directory

### 0x57 - HEARTBEAT (Server → Directory)

Periodic heartbeat to maintain directory listing.

```
+-------------------+-------------------+
| hostname (String) | port (u16)        |
+-------------------+-------------------+
| user_count (u32)  | uptime_seconds(u64)|
+-------------------+-------------------+
| channel_count (u32)                   |
+---------------------------------------+
```

**Fields:**
- `hostname`: Server's hostname (must match registration)
- `port`: Server's port (must match registration)
- `user_count`: Current number of connected users (updated)
- `uptime_seconds`: Server uptime in seconds (updated)
- `channel_count`: Number of channels available on the server (updated)

**Behavior:**
- Sent at interval specified in REGISTER_ACK or HEARTBEAT_ACK
- Updates server's metadata in directory (user count, uptime)
- Resets "last seen" timestamp to prevent removal

**Notes:**
- Must be sent before heartbeat_interval expires
- Missing 3 consecutive heartbeats removes server from directory
- Includes updated stats so directory has current info

### 0x9D - HEARTBEAT_ACK (Directory → Server)

Acknowledgment of heartbeat with updated interval.

```
+-------------------+
| heartbeat_interval(u32)|
+-------------------+
```

**Fields:**
- `heartbeat_interval`: Seconds until next heartbeat (may be adjusted)

**Behavior:**
- Directory may adjust interval based on current load
- If interval changes, server should use new value for next heartbeat
- Allows directory to scale heartbeat frequency dynamically

**Load-Based Intervals:**
```
< 100 servers:   300s (5 minutes)
< 1000 servers:  600s (10 minutes)
< 5000 servers:  1800s (30 minutes)
>= 5000 servers: 3600s (1 hour)
```

**Notes:**
- Server should log interval changes for debugging
- If server ignores interval adjustments, directory may remove it
- Prevents directory overload with thousands of servers

## Connection Flow

### Anonymous TCP Connection (Read-Only)

```
Client                                  Server
  |                                       |
  |--- TCP Connect ------------------->  |
  |                                       |
  |--- LIST_CHANNELS ------------------>  |
  |<-- CHANNEL_LIST --------------------  |
  |                                       |
  |--- JOIN_CHANNEL ------------------>  |
  |<-- JOIN_RESPONSE -------------------  |
  |<-- MESSAGE_LIST (initial) ----------  |
  |                                       |
  |<-- NEW_MESSAGE (real-time) ---------  |
  |<-- NEW_MESSAGE ---------------------  |
  |                                       |
```

**Note:** Anonymous users can browse and read without setting a nickname. Nickname is only required when posting a message.

### Anonymous TCP Connection (Posting)

```
Client                                  Server
  |                                       |
  |--- (already connected, browsing) --  |
  |                                       |
  |--- POST_MESSAGE ------------------>  |
  |<-- ERROR (nickname required) -------  |
  |                                       |
  |--- SET_NICKNAME ------------------>  |
  |<-- NICKNAME_RESPONSE (success) ----  |
  |                                       |
  |--- POST_MESSAGE ------------------>  |
  |<-- MESSAGE_POSTED -----------------  |
  |                                       |
```

**Note:** Server rejects POST_MESSAGE if session has no nickname set. Client must set nickname before posting.

### Registered User via Password

```
Client                                  Server
  |                                       |
  |--- TCP Connect ------------------->  |
  |                                       |
  |--- SET_NICKNAME ("alice") --------->  |
  |<-- NICKNAME_RESPONSE (fail) --------  |
  |    "Nickname registered"              |
  |                                       |
  |--- AUTH_REQUEST ------------------->  |
  |<-- AUTH_RESPONSE (success) ---------  |
  |                                       |
  |--- LIST_CHANNELS ------------------>  |
  |<-- CHANNEL_LIST (with unread) ------  |
  |                                       |
```

### SSH Connection

```
Client                                  Server
  |                                       |
  |--- SSH Connect (key auth) --------->  |
  |<-- SSH authenticated ---------------  |
  |    (Server checks key fingerprint)    |
  |                                       |
  |<-- AUTH_RESPONSE (success) ---------  |
  |    (nickname auto-set)                |
  |                                       |
  |--- LIST_CHANNELS ------------------>  |
  |<-- CHANNEL_LIST -------------------  |
  |                                       |
```

**SSH Key Authentication Flow:**

1. **Client connects**: `ssh username@superchat.example.com`
2. **SSH protocol authenticates**: Server verifies client has the private key matching their public key
3. **Server receives public key**: Full public key is available after SSH authentication
4. **Server computes fingerprint**: SHA256 hash of the public key
5. **Server looks up fingerprint** in `SSHKey` table:

   **Case A - Key is registered:**
   - Authenticate as the registered user (ignore SSH username)
   - Set session nickname to registered user's nickname
   - Example: Key registered to 'elegant', user connects as `bloopie@host` → signed in as 'elegant'

   **Case B - Key is not registered (first connection):**
   - Check if SSH username is already registered to a different user
     - **If username is taken**: Reject SSH connection with error message
     - **If username is available**: Proceed with auto-registration
   - Auto-register new user with SSH username as nickname
   - Store public key and fingerprint in `SSHKey` table
   - Create `User` record with nickname from SSH username
   - Set session nickname to the new username
   - Example: New key, connects as `bloopie@host`, 'bloopie' available → auto-register 'bloopie' to this key
   - Example: New key, connects as `bloopie@host`, 'bloopie' already registered → reject connection
   - **Race condition handling**: The unique index on `User.nickname` prevents duplicate registrations.
     If two users attempt to register the same nickname simultaneously, the database constraint
     will cause the second INSERT to fail, and that SSH connection will be rejected with an error.

6. **Send AUTH_RESPONSE**: Notify client of successful authentication with user_id

**Key Points:**
- SSH key is the source of truth for identity, not the SSH username
- Public key is stored on first connection for future authentication
- SSH username is only used for auto-registration on first connection
- If SSH username is already taken, connection is rejected (prevents confusion)
- Subsequent connections with the same key always authenticate as the registered user
- Users should connect with an available username on their first SSH connection

## Direct Message (DM) Flow

Direct messages are private, encrypted (optional) channels between users. The flow handles key setup, encryption negotiation, and supports both registered and anonymous users.

### DM Encryption Architecture

SuperChat uses **X25519 Diffie-Hellman** for key agreement and **AES-256-GCM** for message encryption. This provides end-to-end encryption where the server never sees plaintext messages or shared secrets.

#### Key Management by User Type

**SSH Ed25519 Users:**
- Ed25519 SSH key is converted to X25519 (mathematically equivalent curve)
- Conversion happens client-side automatically
- No additional setup needed - seamless experience

**SSH RSA/ECDSA Users:**
- SSH key cannot be converted to X25519
- Client generates separate X25519 keypair on first DM
- Public key uploaded via PROVIDE_PUBLIC_KEY
- Private key stored in `~/.superchat/keys/`

**Password-Only Users:**
- Generate X25519 keypair on first DM
- Public key uploaded to server
- Private key stored in `~/.superchat/keys/`

**Anonymous Users:**
- Generate ephemeral X25519 keypair for session
- Keys stored in memory only (destroyed on disconnect)
- Full encryption supported for session duration

#### Encryption Process

1. **Key Agreement (Diffie-Hellman):**
   - Each user has an X25519 keypair (public + private)
   - Both parties compute shared secret independently:
     - Alice: `shared = X25519(alice_private, bob_public)`
     - Bob: `shared = X25519(bob_private, alice_public)`
   - Math guarantees both compute the same 32-byte shared secret
   - Server never sees the shared secret

2. **Key Derivation:**
   - Channel key derived from shared secret using HKDF-SHA256:
     - `channel_key = HKDF(shared_secret, salt="superchat-dm-v1", info=channel_id)`
   - Produces 32-byte AES-256 key unique to this DM channel

3. **Message Encryption:**
   - Messages encrypted with AES-256-GCM using derived channel key
   - Nonce (12 bytes) generated randomly per message
   - Provides both confidentiality and authenticity

4. **Wire Format:**
   ```
   Encrypted Message Payload:
   +----------------+------------------+
   | Nonce (12 B)   | Ciphertext (N B) |
   +----------------+------------------+
   ```
   - Ciphertext includes GCM authentication tag (16 bytes)
   - Frame flags byte has bit 1 set (0x02) for encrypted messages

#### Security Properties

- **Algorithms:** X25519 + HKDF-SHA256 + AES-256-GCM (modern standard)
- **Forward Secrecy:** Not currently implemented (future enhancement)
- **End-to-End:** Server cannot decrypt messages (no access to shared secret)
- **Authentication:** Users authenticated via SSH keys or passwords
- **Key Storage:** Private keys never leave client
- **Anonymous Users:** Full encryption with ephemeral keys (session-only)

### Flow 1: Both Users Have Keys (Simple Case)

```
User A (has key)                        Server                          User B (has key)
  |                                       |                                       |
  |--- START_DM(target: "bob") -------->  |                                       |
  |                                       |--- DM_REQUEST from "alice" -------->  |
  |                                       |                                       |
  |<-- DM_PENDING (waiting for bob) ----  |                                       |
  |                                       |                                  (B accepts)
  |                                       |<-- (implicit accept via          |
  |                                       |     standard flow)                |
  |                                       |                                       |
  |<-- DM_READY (channel_id, bob_pub) --  |--- DM_READY (channel_id, alice_pub) ->|
  |                                       |                                       |
```

**Notes:**
- Server creates private channel with `is_dm = true`
- Each party receives the other's X25519 public key
- Both clients compute shared secret via DH and derive channel key
- Server never sees the shared secret or channel key
- Both users can now use standard messaging on this channel

### Flow 2: Initiator Needs Key

```
User A (no key)                         Server
  |                                       |
  |--- START_DM(target: "bob") -------->  |
  |                                       |
  |<-- KEY_REQUIRED -------------------  |
  |                                       |
  |(Client prompts A to set up key)       |
  |                                       |
  |--- PROVIDE_PUBLIC_KEY ------------->  |
  |    or ALLOW_UNENCRYPTED               |
  |                                       |
  |<-- DM_PENDING (waiting for bob) ----  |
  |                                       |
  |(continues as Flow 1)                  |
```

### Flow 3: Recipient Needs Key

```
User A (has key)                        Server                          User B (no key)
  |                                       |                                       |
  |--- START_DM(target: "bob") -------->  |                                       |
  |                                       |--- DM_REQUEST from "alice" -------->  |
  |                                       |--- KEY_REQUIRED ------------------>  |
  |                                       |                                       |
  |<-- DM_PENDING (waiting for bob) ----  |                                  (B sets up key)
  |                                       |                                       |
  |                                       |<-- PROVIDE_PUBLIC_KEY ------------  |
  |                                       |    or ALLOW_UNENCRYPTED              |
  |                                       |                                       |
  |<-- DM_READY (channel_id, bob_pub) --  |--- DM_READY (channel_id, alice_pub) ->|
  |                                       |                                       |
```

**Notes:**
- User A sees DM_PENDING immediately
- User B sees DM_REQUEST + KEY_REQUIRED simultaneously
- While B is setting up their key, they don't see "waiting for A" (they're busy with key setup)
- Once B completes key setup, both get DM_READY with each other's public keys

### Flow 4: Both Users Need Keys

```
User A (no key)                         Server                          User B (no key)
  |                                       |                                       |
  |--- START_DM(target: "bob") -------->  |                                       |
  |                                       |                                       |
  |<-- KEY_REQUIRED -------------------  |                                       |
  |                                       |                                       |
  |(A sets up key)                        |                                       |
  |                                       |                                       |
  |--- PROVIDE_PUBLIC_KEY ------------->  |                                       |
  |                                       |--- DM_REQUEST from "alice" -------->  |
  |                                       |--- KEY_REQUIRED ------------------>  |
  |                                       |                                       |
  |<-- DM_PENDING (waiting for bob) ----  |                                  (B sets up key)
  |                                       |                                       |
  |(A sees waiting indicator)             |                                  (B busy with UI)
  |                                       |                                       |
  |                                       |<-- PROVIDE_PUBLIC_KEY ------------  |
  |                                       |                                       |
  |<-- DM_READY (channel_id, bob_pub) --  |--- DM_READY (channel_id, alice_pub) ->|
  |                                       |                                       |
```

**Notes:**
- A sets up key first, then waits for B
- B receives DM_REQUEST while setting up key
- Both complete key setup before DM_READY is sent
- Each receives the other's public key to compute shared secret

### Flow 5: Unencrypted DM (Both Allow)

```
User A (no key, allows unencrypted)    Server                          User B (no key, allows unencrypted)
  |                                       |                                       |
  |--- START_DM(target: "bob",        -->  |                                       |
  |     allow_unencrypted: true)          |                                       |
  |                                       |--- DM_REQUEST from "alice" -------->  |
  |                                       |    (initiator_allows_unencrypted:     |
  |                                       |     true, requires_key: false)        |
  |                                       |                                       |
  |                                       |                                  (B accepts)
  |                                       |<-- (implicit accept)               |
  |                                       |                                       |
  |<-- DM_READY (is_encrypted: false) --  |--- DM_READY (is_encrypted: false) -> |
  |                                       |                                       |
```

**Notes:**
- No KEY_REQUIRED sent to either party
- Server creates unencrypted channel
- Messages are sent in plaintext
- Still private (not in public channel list), just not encrypted

### Flow 6: Anonymous User DM

```
User A (registered, has key)            Server                          User B (anonymous, no key)
  |                                       |                                       |
  |--- START_DM(target: session_123) ->  |                                       |
  |                                       |--- DM_REQUEST from "alice" -------->  |
  |                                       |--- KEY_REQUIRED ------------------>  |
  |                                       |                                       |
  |<-- DM_PENDING (waiting for bob) ----  |                                  (B generates local key)
  |                                       |                                       |
  |                                       |<-- PROVIDE_PUBLIC_KEY ------------  |
  |                                       |    (Note: B is still anonymous,      |
  |                                       |     key is session-only)             |
  |                                       |                                       |
  |<-- DM_READY (channel_id) ------------  |--- DM_READY (channel_id) ---------->  |
  |                                       |     (both derive key via DH)          |
  |                                       |                                       |
```

**Notes:**
- Anonymous user B can receive DMs by session_id
- B generates ephemeral X25519 keypair for this session
- B's key is not permanently stored (lost on disconnect)
- Full encryption via DH: both parties compute shared secret normally
- Alternatively, B can choose ALLOW_UNENCRYPTED (skips key generation)

### Encryption Details

**Key Agreement:**
- Each DM uses X25519 Diffie-Hellman between the two participants
- Shared secret computed client-side: `X25519(my_private, their_public)`
- Server stores only public keys, never sees shared secrets

**Key Derivation:**
- Channel key = `HKDF-SHA256(shared_secret, salt="superchat-dm-v1", info=channel_id_bytes)`
- Each DM channel has a unique derived key (same shared secret, different channel IDs)
- 32-byte output used as AES-256 key

**Message Encryption:**
- Messages in encrypted DMs have Flags bit 1 set (0x02 or 0x03)
- Payload format: `[nonce (12 bytes)][ciphertext + GCM tag]`
- Nonce generated randomly for each message (never reused)

**Key Updates:**
- If user generates a new X25519 keypair, they must re-derive all DM channel keys
- Client fetches other party's public key and recomputes shared secret
- No server-side re-encryption needed (DH is symmetric)

**Anonymous User Keys:**
- Generated client-side as ephemeral X25519 keypair
- Public key uploaded to server (session-only, deleted on disconnect)
- Private key stored in memory only (lost on disconnect)
- Full encryption supported - no plaintext fallback needed

## Server Discovery Flow

### Client Browsing Servers

```
Client                                  Directory (superchat.win)
  |                                       |
  |--- TCP Connect ------------------->  |
  |                                       |
  |--- LIST_SERVERS (limit: 100) ----->  |
  |<-- SERVER_LIST (50 servers) --------  |
  |                                       |
  |(User picks server from list)          |
  |                                       |
  |--- DISCONNECT --------------------->  |
  |                                       |
  (Client connects to chosen server at chat.example.com:6465)
```

**Notes:**
- Client connects to directory server(s) temporarily
- Receives server list, displays to user
- Disconnects from directory
- Connects to chosen server for actual chat

### Server Registration

```
Server (chat.example.com)              Directory (superchat.win)
  |                                       |
  |(Server startup with                   |
  | --announce-to superchat.win:6465)     |
  |                                       |
  |--- TCP Connect ------------------->  |
  |                                       |
  |--- REGISTER_SERVER ---------------->  |
  |    (hostname, port, name, desc)       |
  |                                       |
  |                                       |(Directory validates)
  |                                       |
  |                                       |--- TCP Connect to --------->  |
  |                                       |    chat.example.com:6465      |
  |                                       |                               |
  |<-- VERIFY_REGISTRATION (challenge) --|                               |
  |                                       |                               |
  |--- VERIFY_RESPONSE (challenge) ----->|                               |
  |                                       |(Verification OK)              |
  |                                       |
  |<-- REGISTER_ACK (success,             |
  |    heartbeat_interval: 300s) -------  |
  |                                       |
  |(Wait 5 minutes)                       |
  |                                       |
  |--- HEARTBEAT (hostname, port,         |
  |    user_count, uptime) ------------>  |
  |                                       |
  |<-- HEARTBEAT_ACK (300s) ------------  |
  |                                       |
  |(Repeat heartbeat every 5 min)         |
```

**Notes:**
- Server maintains persistent connection to directory
- Sends heartbeat at interval specified by directory
- Directory can adjust interval via HEARTBEAT_ACK
- Missing 3 heartbeats removes server from directory

### Server Registration Failure (Verification)

```
Server (fake.example.com)               Directory (superchat.win)
  |                                       |
  |--- REGISTER_SERVER ---------------->  |
  |    (hostname: fake.example.com:6465)  |
  |                                       |
  |                                       |(Attempts to connect to
  |                                       | fake.example.com:6465)
  |                                       |
  |                                       |(Connection fails - timeout)
  |                                       |
  |<-- REGISTER_ACK (success: false,      |
  |    message: "Could not verify...") -  |
  |                                       |
```

**Notes:**
- Directory rejects registration if it can't connect back
- Prevents spam registrations of fake servers
- Rate limiting prevents brute-force attempts

### Directory Load Adaptation

```
Server                                  Directory (has 1500 servers)
  |                                       |
  |--- REGISTER_SERVER ---------------->  |
  |                                       |
  |(Verification succeeds)                |
  |                                       |
  |<-- REGISTER_ACK (success,             |
  |    heartbeat_interval: 600s) -------  |
  |    "High load, heartbeat every 10m"   |
  |                                       |
  |(Wait 10 minutes - adjusted interval)  |
  |                                       |
  |--- HEARTBEAT ---------------------->  |
  |                                       |
  |<-- HEARTBEAT_ACK (600s) ------------  |
  |                                       |
```

**Notes:**
- Directory dynamically adjusts heartbeat interval based on number of servers
- Servers respect the interval to avoid overloading directory
- Prevents performance degradation with thousands of servers

### Directory Gossip (Network Expansion)

```
Directory A (superchat.win)             Server B (chat.example.com, also a directory)
  |                                       |
  |(B already registered to A)            |
  |                                       |
  |(Gossip timer fires - every 3 hours)   |
  |                                       |
  |--- LIST_SERVERS (limit: 500) ----->  |
  |                                       |
  |<-- SERVER_LIST (B knows 20 servers) -|
  |                                       |
  |(A discovers new Server C)             |
  |(A doesn't know C yet - needs verify)  |
  |                                       |
  |(A connects to Server C)               |
  |                                       |
  |                                       Server C (game.example.org)
  |                                       |
  |--- VERIFY_REGISTRATION (challenge) ----------------->  |
  |                                       |               |
  |<-- VERIFY_RESPONSE (challenge) -------------------  |
  |                                       |
  |(Verification OK - A adds C to list)   |
  |(C doesn't know A registered it)       |
  |                                       |
  |(Later, A may announce itself to C)    |
  |(A connects to C again)                |
  |                                       |
  |--- REGISTER_SERVER (A's info) ---------------------->  |
  |                                       |               |
  |                                       |          (C verifies A)
  |                                       |               |
  |<-- REGISTER_ACK (success) ------------------------  |
  |                                       |
  |(Now A and C know about each other)    |
  |(B acted as bridge for discovery)      |
```

**Notes:**
- Directories periodically query all registered servers for their server lists (gossip)
- New servers discovered through gossip are **verified** before being added
- Directory connects to discovered server and sends VERIFY_REGISTRATION
- Only servers that pass verification are added (prevents fake server injection)
- Server is silently added to directory (no notification sent to server)
- Directory may optionally register itself to newly discovered servers (bidirectional)
- This creates a mesh network where directories discover each other
- If superchat.win goes down, other directories continue discovering servers
- Gossip interval is randomized (1-6 hours) to prevent synchronized queries
- Only servers in directory mode respond with SERVER_LIST, regular chat servers return empty list

**Network Resilience:**
- No single point of failure - multiple directories can exist
- Directories learn from each other through gossip
- Clients can configure multiple directories as fallbacks
- If primary directory fails, clients use secondary directories

## Protocol Extensions

### Future Considerations

- **Typing Indicators**: Real-time typing notifications (optional)
- **Read Receipts**: Show when other party has read your messages (optional)
- **Multi-party DMs**: Group DMs with 3+ participants
- **Channel Moderation**: Tools for channel operators (kick, ban, mute)

### Explicitly Not Supported

To maintain the old-school, text-focused nature of SuperChat:

- **No file attachments / binary blobs**: Text only, keeps it simple and prevents abuse
- **No emoji reactions**: Want to react? Reply with "+1" or "agreed" like the old days
- **No rich text / markdown**: Plain text only, no formatting wars

## Implementation Notes

### Client Implementation
- Maintain persistent TCP connection
- Implement automatic reconnection with exponential backoff
- Buffer outgoing messages during disconnection
- Store local state for anonymous users in `~/.config/superchat-client/state.db`
- **SSH connections**: Notify user if connected username differs from authenticated nickname
  - Example: "Connected as 'bloopie@host' but authenticated as 'elegant' (SSH key registered to 'elegant')"
  - Prevents confusion when SSH username doesn't match registered identity
  - Display authenticated nickname prominently in UI

### Server Implementation
- Use event loop for handling multiple connections (goroutines + channels in Go)
- Implement per-user rate limiting (messages per minute)
- Broadcast NEW_MESSAGE to all sessions in the same channel
- Periodically send SERVER_STATS to all connected clients
- Implement graceful shutdown (notify clients before closing)

### Security Considerations
- Validate all string inputs for length and content
- Sanitize message content (strip control characters)
- Rate limit message posting (e.g., 10 messages/minute per user)
- Limit max connections per IP address
- Implement flood protection for channel creation
- Use TLS for TCP connections (or SSH tunneling)
