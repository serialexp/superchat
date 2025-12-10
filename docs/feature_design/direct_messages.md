# Direct Messages Implementation Plan

**Status:** In Progress (Phases 1-4 Complete)
**Created:** 2024-12-09
**Last Updated:** 2024-12-09

This document captures the complete implementation plan for DMs with E2E encryption, based on the protocol spec in `docs/PROTOCOL.md`.

---

## Overview

Direct Messages (DMs) are private channels between two users with optional end-to-end encryption using X25519 Diffie-Hellman key agreement and AES-256-GCM message encryption.

### Key Design Decisions

1. **X25519 DH for key agreement** - Both parties compute the same shared secret independently; server never sees it
2. **Ed25519 SSH keys convert to X25519** - Seamless encryption for SSH users
3. **One key per user per server** - Simple model; multi-device is user's responsibility to sync keys
4. **Unencrypted DMs require mutual consent** - Both parties must explicitly agree
5. **No forward secrecy (V3)** - Can be added later with ratcheting

---

## Progress Checklist

### Phase 1: Database Schema
- [x] Create migration `011_add_direct_messages.sql`
  - [x] Add `encryption_public_key BLOB` to User table
  - [x] Add `is_dm INTEGER DEFAULT 0` to Channel table
  - [x] Create `ChannelAccess` table (channel_id, user_id)
  - [x] Create `DMInvite` table for pending invites
- [x] Add migration tests in `migration_path_test.go` (v10→v11 test added)
- [x] Implement database methods in `database.go`
  - [x] `SetUserEncryptionKey(userID, publicKey)`
  - [x] `GetUserEncryptionKey(userID) -> []byte`
  - [x] `CreateDMChannel(user1ID, user2ID, isEncrypted) -> channelID`
  - [x] `GetDMChannels(userID) -> []Channel`
  - [x] `CreateDMInvite(initiatorID, targetID, isEncrypted) -> inviteID`
  - [x] `GetPendingDMInvitesForUser(userID) -> []DMInvite`
  - [x] `DeleteDMInvite(inviteID)` (replaces AcceptDMInvite/DeclineDMInvite)
  - [x] Additional helpers: `GetDMChannelBetweenUsers`, `GetDMOtherUser`, `UserHasAccessToChannel`
- [x] Mirror methods in `memdb.go`

### Phase 2: Protocol Messages
- [x] Implement in `pkg/protocol/messages.go`:
  - [x] `StartDM` (0x19) - Client → Server
  - [x] `ProvidePublicKey` (0x1A) - Client → Server (code updated from 0x13)
  - [x] `AllowUnencrypted` (0x1B) - Client → Server (code updated from 0x14)
  - [x] `KeyRequired` (0xA1) - Server → Client (code updated from 0x93)
  - [x] `DMReady` (0xA2) - Server → Client (code updated from 0x94)
  - [x] `DMPending` (0xA3) - Server → Client (code updated from 0x95)
  - [x] `DMRequest` (0xA4) - Server → Client (code updated from 0x96)
- [x] Add encode/decode tests for each message type (7 tests added)
- [x] Add round-trip tests (covered in encode/decode tests)
- [ ] Ensure 85%+ coverage for new code

### Phase 3: Server Handlers
- [x] Add handlers in `pkg/server/handlers.go`:
  - [x] `handleStartDM` - Initiate DM flow
  - [x] `handleProvidePublicKey` - Store user's encryption key
  - [x] `handleAllowUnencrypted` - Accept unencrypted DM
- [x] Add message dispatch in `server.go`
- [x] Add `EncryptionPublicKey` field to Session struct
- [x] Implement DM creation logic:
  - [x] Check if both users have encryption keys
  - [x] If encrypted: create channel, send DM_READY to both
  - [x] If needs consent: create invite, send DM_PENDING/DM_REQUEST
- [x] Filter DM channels from `CHANNEL_LIST` broadcast (done in database query)
- [ ] Only send DM messages to participants (needs client testing)

### Phase 4: Client Crypto
- [x] Create `pkg/client/crypto/` package:
  - [x] `GenerateX25519Keypair() -> (pub, priv)`
  - [x] `Ed25519ToX25519(ed25519Priv) -> x25519Priv`
  - [x] `ComputeSharedSecret(myPriv, theirPub) -> shared`
  - [x] `DeriveChannelKey(shared, channelID) -> aesKey`
  - [x] `EncryptMessage(key, plaintext) -> ciphertext`
  - [x] `DecryptMessage(key, ciphertext) -> plaintext`
- [x] Key storage for non-SSH users:
  - [x] Store in `~/.config/superchat-client/keys/`
  - [x] Format: `server_host_userid.x25519` (private key)
- [x] Comprehensive tests for crypto operations (82.3% coverage)

### Phase 5: Client UI - Terminal
- [ ] Add DM-related state to `model.go`:
  - [ ] `dmChannels []Channel`
  - [ ] `pendingDMInvites []DMInvite`
  - [ ] `encryptionKeyPair *X25519KeyPair`
  - [ ] `dmOtherUserKeys map[uint64][]byte` (channelID -> other's pubkey)
- [ ] New views:
  - [ ] `ViewDMList` - List of active DMs and pending invites
  - [ ] `ViewDMInvite` - "User X wants to chat (unencrypted). Accept?"
- [ ] Entry points for starting DMs:
  - [ ] From user list: select user, press `d`
  - [ ] From thread view: on message, press `d` to DM author
- [ ] Encryption setup flow (for password users):
  - [ ] Prompt to generate keypair on first DM
  - [ ] Show backup warning
  - [ ] Store key locally
- [ ] Message encryption/decryption in send/receive handlers
- [ ] Visual indicator for encrypted vs unencrypted DMs (lock icon)

### Phase 6: Client UI - Desktop (Wails)
- [ ] Mirror terminal UI changes in desktop client
- [ ] DM list in sidebar
- [ ] DM invite modal
- [ ] Encryption setup modal

### Phase 7: Testing & Polish
- [ ] Integration tests for full DM flow
- [ ] Test encrypted DM between two clients
- [ ] Test unencrypted DM consent flow
- [ ] Test key mismatch scenarios
- [ ] Load testing with DM traffic
- [ ] Update V3.md with completion status

---

## Database Schema

### Migration: `011_add_direct_messages.sql`

```sql
-- Add encryption public key to User table
-- X25519 public key (32 bytes) for DM encryption
ALTER TABLE User ADD COLUMN encryption_public_key BLOB;

-- Add is_dm flag to Channel table
-- DM channels are private and excluded from channel list
ALTER TABLE Channel ADD COLUMN is_dm INTEGER NOT NULL DEFAULT 0;

-- ChannelAccess: tracks who can access private channels (DMs)
CREATE TABLE IF NOT EXISTS ChannelAccess (
    channel_id INTEGER NOT NULL REFERENCES Channel(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES User(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    PRIMARY KEY (channel_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_channel_access_user ON ChannelAccess(user_id);

-- DMInvite: pending DM requests awaiting acceptance
CREATE TABLE IF NOT EXISTS DMInvite (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    initiator_user_id INTEGER NOT NULL REFERENCES User(id) ON DELETE CASCADE,
    target_user_id INTEGER NOT NULL REFERENCES User(id) ON DELETE CASCADE,
    is_encrypted INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    UNIQUE(initiator_user_id, target_user_id)
);

CREATE INDEX IF NOT EXISTS idx_dm_invite_target ON DMInvite(target_user_id);
```

---

## Protocol Messages (Wire Format)

### 0x19 - START_DM (Client → Server)

```
+-------------------+---------------------------+-------------------------+
| target_type (u8)  | target_id (varies)        | allow_unencrypted(bool) |
+-------------------+---------------------------+-------------------------+
```

**Target Types:**
- 0x00 = by user_id (u64)
- 0x01 = by nickname (String)
- 0x02 = by session_id (u64, for anonymous users)

### 0x13 - PROVIDE_PUBLIC_KEY (Client → Server)

```
+-------------------+------------------------+-------------------------+
| key_type (u8)     | public_key (32 bytes)  | label (String)          |
+-------------------+------------------------+-------------------------+
```

**Key Types:**
- 0x00 = Derived from SSH key (Ed25519 → X25519)
- 0x01 = Generated X25519 key
- 0x02 = Ephemeral X25519 key (anonymous, session-only)

### 0x14 - ALLOW_UNENCRYPTED (Client → Server)

```
+------------------------+-------------------+
| dm_channel_id (u64)    | permanent (bool)  |
+------------------------+-------------------+
```

### 0x93 - KEY_REQUIRED (Server → Client)

```
+-------------------+---------------------------+
| reason (String)   | dm_channel_id (Opt u64)   |
+-------------------+---------------------------+
```

### 0x94 - DM_READY (Server → Client)

```
+-------------------+-------------------+------------------------+
| channel_id (u64)  | other_user_id     | other_nickname(String) |
|                   | (Optional u64)    |                        |
+-------------------+-------------------+------------------------+
| is_encrypted(bool)| other_public_key (Optional 32 bytes)      |
+-------------------+-------------------------------------------+
```

### 0x95 - DM_PENDING (Server → Client)

```
+-------------------+---------------------------+------------------------+
| dm_channel_id(u64)| waiting_for_user_id       | waiting_for_nickname   |
|                   | (Optional u64)            | (String)               |
+-------------------+---------------------------+------------------------+
| reason (String)   |
+-------------------+
```

### 0x96 - DM_REQUEST (Server → Client)

```
+-------------------+-------------------------+------------------------+
| dm_channel_id(u64)| from_user_id            | from_nickname (String) |
|                   | (Optional u64)          |                        |
+-------------------+-------------------------+------------------------+
| requires_key(bool)| initiator_allows_unencrypted (bool)            |
+-------------------+------------------------------------------------+
```

---

## Encryption Architecture

### Key Agreement Flow

```
Alice                          Server                          Bob
  |                              |                               |
  |-- PROVIDE_PUBLIC_KEY ------> |                               |
  |   (alice_x25519_pub)         | (stores alice's pubkey)       |
  |                              |                               |
  |                              | <-- PROVIDE_PUBLIC_KEY -------|
  |                              |     (bob_x25519_pub)          |
  |                              |                               |
  |-- START_DM(bob) -----------> |                               |
  |                              |--- DM_REQUEST --------------> |
  |<-- DM_PENDING -------------- |                               |
  |                              | <-- (implicit accept) --------|
  |                              |                               |
  |<-- DM_READY(bob_pub) ------- |--- DM_READY(alice_pub) -----> |
  |                              |                               |

Alice computes: shared = X25519(alice_priv, bob_pub)
Bob computes:   shared = X25519(bob_priv, alice_pub)
Both get same 32-byte shared secret.

Channel key = HKDF-SHA256(shared, salt="superchat-dm-v1", info=channel_id_bytes)
```

### Message Encryption

```
Plaintext message
       |
       v
+------+-------+
| AES-256-GCM  | <-- channel_key (derived from DH)
| Encrypt      | <-- nonce (12 bytes, random)
+------+-------+
       |
       v
+--------+--------------------+
| Nonce  | Ciphertext + Tag   |
| 12B    | N bytes + 16B      |
+--------+--------------------+
       |
       v
Frame with flags byte bit 1 set (0x02)
```

### Ed25519 to X25519 Conversion

Ed25519 and X25519 use the same underlying curve (Curve25519), just different representations:
- Ed25519: twisted Edwards form (for signatures)
- X25519: Montgomery form (for key exchange)

```go
import "filippo.io/edwards25519"

func Ed25519PrivateToX25519(ed25519Priv []byte) []byte {
    // Hash the seed to get the scalar
    h := sha512.Sum512(ed25519Priv[:32])

    // Clamp the scalar (standard X25519 clamping)
    h[0] &= 248
    h[31] &= 127
    h[31] |= 64

    return h[:32]
}
```

Or use `golang.org/x/crypto/curve25519` with `filippo.io/edwards25519` for the conversion.

---

## Client Key Storage

### SSH Ed25519 Users
- Private key: derived from SSH key on-demand (no storage needed)
- Public key: uploaded to server once on first DM

### Password/Anonymous Users
- Key location: `~/.config/superchat-client/keys/`
- Filename: `{server_host}_{user_id}.x25519`
- Format: 32 bytes raw private key
- Permissions: 0600 (owner read/write only)

### Key Backup Warning (UX)

On first DM setup for password users:
```
┌─────────────────────────────────────────────────────┐
│  Encryption Key Generated                           │
├─────────────────────────────────────────────────────┤
│                                                     │
│  Your encryption key has been saved to:             │
│  ~/.config/superchat-client/keys/                   │
│                                                     │
│  ⚠ IMPORTANT: Back up this key!                     │
│                                                     │
│  If you lose this key, you will NOT be able to      │
│  read your encrypted messages on a new device.      │
│  There is no recovery option.                       │
│                                                     │
│  [OK]                                               │
└─────────────────────────────────────────────────────┘
```

---

## DM Flows

### Flow 1: Both Have Keys (Happy Path)

1. Alice sends `START_DM(target: bob_id, allow_unencrypted: false)`
2. Server checks: Alice has key? Yes. Bob has key? Yes.
3. Server creates DM channel with `is_dm = true`
4. Server adds both to `ChannelAccess`
5. Server sends `DM_READY` to both with each other's public keys
6. Both compute shared secret and derive channel key
7. Ready to exchange encrypted messages

### Flow 2: Unencrypted (Both Consent)

1. Alice sends `START_DM(target: bob_id, allow_unencrypted: true)`
2. Server checks: Alice has key? No (or Bob has no key)
3. Server creates `DMInvite` with `is_encrypted = false`
4. Server sends `DM_PENDING` to Alice
5. Server sends `DM_REQUEST` to Bob with `initiator_allows_unencrypted: true`
6. Bob sends `ALLOW_UNENCRYPTED(dm_channel_id, permanent: false)`
7. Server creates unencrypted DM channel
8. Server sends `DM_READY(is_encrypted: false)` to both
9. Messages sent in plaintext

### Flow 3: Key Setup Required

1. Alice sends `START_DM(target: bob_id, allow_unencrypted: false)`
2. Server checks: Alice has key? No.
3. Server sends `KEY_REQUIRED` to Alice
4. Alice generates keypair, sends `PROVIDE_PUBLIC_KEY`
5. Server stores key, continues with Flow 1

---

## UI Entry Points

### Terminal Client

**From User List:**
```
Online Users (5)
─────────────────
> alice         [d] DM
  bob
  charlie

[d] Start DM  [Esc] Back
```

**From Thread View:**
```
#general > Thread: "Hello world"
─────────────────────────────────
alice (2 min ago)
  Hello everyone!

> bob (1 min ago)                  <- cursor here
    Welcome alice!

[r] Reply  [d] DM author  [Esc] Back
```

**DM List View:**
```
Direct Messages
─────────────────
  alice           (3 new) 🔒
> bob             (unread) 🔒
  charlie         🔓 unencrypted

Pending Invites
─────────────────
  dave wants to chat (encrypted)

[Enter] Open  [a] Accept invite  [x] Decline  [n] New DM
```

### Icons
- 🔒 = Encrypted DM
- 🔓 = Unencrypted DM
- Or use `[E]` / `[U]` for terminal compatibility

---

## Go Dependencies

```go
import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "crypto/sha512"

    "golang.org/x/crypto/curve25519"
    "golang.org/x/crypto/hkdf"
    "filippo.io/edwards25519" // for Ed25519 -> X25519 conversion
)
```

---

## Testing Strategy

### Unit Tests
- [ ] All message encode/decode
- [ ] X25519 key generation
- [ ] Ed25519 → X25519 conversion
- [ ] Shared secret computation (verify both parties get same result)
- [ ] HKDF key derivation
- [ ] AES-GCM encrypt/decrypt round-trip
- [ ] Nonce generation (uniqueness)

### Integration Tests
- [ ] Full DM flow: key setup → DM creation → message exchange
- [ ] Unencrypted consent flow
- [ ] DM invite accept/decline
- [ ] DM persistence across reconnect
- [ ] DM filtering from channel list

### Security Tests
- [ ] Server cannot decrypt messages (verify ciphertext is opaque)
- [ ] Different channel IDs produce different keys (same users)
- [ ] Nonce reuse detection/prevention
- [ ] Invalid ciphertext handling (authentication failure)

---

## Open Questions

1. **DM List Message** - Do we need a new `GET_DM_LIST` / `DM_LIST` message pair, or can we reuse channel list with filtering?
   - Recommendation: Add new messages for cleaner separation

2. **Anonymous → Registered DM** - If anonymous user registers mid-session, do we migrate their DMs?
   - Recommendation: No, keep them separate. Old DMs stay with session.

3. **Blocking** - Should we add user blocking in V3 or defer?
   - Recommendation: Defer to V4, just decline unwanted DMs for now.

---

## References

- Protocol spec: `docs/PROTOCOL.md` (DM Flow section)
- V3 overview: `docs/versions/V3.md`
- Go X25519: https://pkg.go.dev/golang.org/x/crypto/curve25519
- HKDF: https://pkg.go.dev/golang.org/x/crypto/hkdf
- Edwards25519: https://pkg.go.dev/filippo.io/edwards25519
