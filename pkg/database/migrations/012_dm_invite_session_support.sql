-- Migration 012: Add session support to DMInvite for anonymous users
-- This allows DM invites between anonymous users (who have sessions but no user IDs)

-- SQLite doesn't support ALTER COLUMN, so we recreate the table
-- First, create the new table structure
CREATE TABLE IF NOT EXISTS DMInvite_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    -- For registered users (nullable for anonymous)
    initiator_user_id INTEGER REFERENCES User(id) ON DELETE CASCADE,
    target_user_id INTEGER REFERENCES User(id) ON DELETE CASCADE,
    -- For anonymous users (nullable for registered)
    -- Note: No FK constraint on session IDs because sessions are stored in-memory
    -- and only persisted during snapshots. Cleanup happens when sessions end.
    initiator_session_id INTEGER,
    target_session_id INTEGER,
    is_encrypted INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    -- At least one of user_id or session_id must be set for each party
    CHECK (initiator_user_id IS NOT NULL OR initiator_session_id IS NOT NULL),
    CHECK (target_user_id IS NOT NULL OR target_session_id IS NOT NULL),
    -- Maintain uniqueness for registered users (SQLite allows multiple NULLs in UNIQUE)
    UNIQUE(initiator_user_id, target_user_id)
);

-- Copy existing data (all existing invites have user IDs)
INSERT INTO DMInvite_new (id, initiator_user_id, target_user_id, is_encrypted, created_at)
SELECT id, initiator_user_id, target_user_id, is_encrypted, created_at FROM DMInvite;

-- Drop old table and rename new one
DROP TABLE DMInvite;
ALTER TABLE DMInvite_new RENAME TO DMInvite;

-- Recreate indexes (keep original names for backwards compatibility with tests)
CREATE INDEX IF NOT EXISTS idx_dm_invite_target ON DMInvite(target_user_id);
CREATE INDEX IF NOT EXISTS idx_dm_invite_initiator ON DMInvite(initiator_user_id);
CREATE INDEX IF NOT EXISTS idx_dm_invite_target_session ON DMInvite(target_session_id);
CREATE INDEX IF NOT EXISTS idx_dm_invite_initiator_session ON DMInvite(initiator_session_id);
