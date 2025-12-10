-- Migration 011: Add Direct Messages support
-- Adds encryption keys, DM channels, and invite tracking

-- Add encryption public key to User table
-- X25519 public key (32 bytes) for DM encryption
ALTER TABLE User ADD COLUMN encryption_public_key BLOB;

-- Add is_dm flag to Channel table
-- DM channels are private and excluded from channel list
ALTER TABLE Channel ADD COLUMN is_dm INTEGER NOT NULL DEFAULT 0;

-- ChannelAccess: tracks who can access private channels (DMs)
-- For DMs, exactly 2 users will have access to the channel
CREATE TABLE IF NOT EXISTS ChannelAccess (
    channel_id INTEGER NOT NULL REFERENCES Channel(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES User(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    PRIMARY KEY (channel_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_channel_access_user ON ChannelAccess(user_id);
CREATE INDEX IF NOT EXISTS idx_channel_access_channel ON ChannelAccess(channel_id);

-- DMInvite: pending DM requests awaiting acceptance
-- Used when one or both users need to consent to unencrypted DM
CREATE TABLE IF NOT EXISTS DMInvite (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    initiator_user_id INTEGER NOT NULL REFERENCES User(id) ON DELETE CASCADE,
    target_user_id INTEGER NOT NULL REFERENCES User(id) ON DELETE CASCADE,
    is_encrypted INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    UNIQUE(initiator_user_id, target_user_id)
);

CREATE INDEX IF NOT EXISTS idx_dm_invite_target ON DMInvite(target_user_id);
CREATE INDEX IF NOT EXISTS idx_dm_invite_initiator ON DMInvite(initiator_user_id);
