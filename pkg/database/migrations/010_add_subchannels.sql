-- Add parent_id to Channel table for V3 two-level channel hierarchy
-- Top-level channels have parent_id = NULL, subchannels reference their parent

ALTER TABLE Channel ADD COLUMN parent_id INTEGER REFERENCES Channel(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_channel_parent ON Channel(parent_id) WHERE parent_id IS NOT NULL;
