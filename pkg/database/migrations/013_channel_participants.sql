-- Migration 013: Add ChannelParticipant table for DM channel membership
-- This replaces ChannelAccess for DMs and supports both registered and anonymous users

-- ChannelParticipant tracks who is in a DM channel
-- For registered users: user_id is set, session_id tracks current connection (null if offline)
-- For anonymous users: only session_id is set, participation ends when session ends
CREATE TABLE IF NOT EXISTS ChannelParticipant (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id INTEGER NOT NULL REFERENCES Channel(id) ON DELETE CASCADE,
    user_id INTEGER REFERENCES User(id) ON DELETE CASCADE,  -- NULL for anonymous
    session_id INTEGER,  -- Current session, NULL if registered user is offline
    nickname TEXT NOT NULL,  -- Nickname at time of joining (for display)
    joined_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    -- At least one identifier must be set
    CHECK (user_id IS NOT NULL OR session_id IS NOT NULL)
);

-- Indexes for efficient lookups
CREATE INDEX IF NOT EXISTS idx_channel_participant_channel ON ChannelParticipant(channel_id);
CREATE INDEX IF NOT EXISTS idx_channel_participant_user ON ChannelParticipant(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_channel_participant_session ON ChannelParticipant(session_id) WHERE session_id IS NOT NULL;

-- Unique constraint for registered users (one entry per user per channel)
CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_participant_unique_user
    ON ChannelParticipant(channel_id, user_id) WHERE user_id IS NOT NULL;

-- Unique constraint for anonymous users (one entry per session per channel)
CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_participant_unique_session
    ON ChannelParticipant(channel_id, session_id) WHERE user_id IS NULL AND session_id IS NOT NULL;
