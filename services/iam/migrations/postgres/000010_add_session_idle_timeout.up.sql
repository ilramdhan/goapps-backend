-- Add last_activity_at column to user_sessions for idle timeout tracking.
-- This column records the last time the user made an authenticated request.
-- If NULL, the session was created before this feature — treat created_at as last activity.

ALTER TABLE user_sessions
    ADD COLUMN IF NOT EXISTS last_activity_at TIMESTAMP WITH TIME ZONE;

-- Backfill existing rows: use created_at as initial last_activity_at
UPDATE user_sessions SET last_activity_at = created_at WHERE last_activity_at IS NULL;

-- Make NOT NULL after backfill
ALTER TABLE user_sessions ALTER COLUMN last_activity_at SET NOT NULL;

-- Set default for future rows
ALTER TABLE user_sessions ALTER COLUMN last_activity_at SET DEFAULT NOW();

-- Index for idle timeout queries (active sessions ordered by activity)
CREATE INDEX IF NOT EXISTS idx_session_last_activity
    ON user_sessions (last_activity_at)
    WHERE revoked_at IS NULL;
