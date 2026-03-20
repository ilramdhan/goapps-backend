-- Remove idle timeout tracking from user_sessions.
DROP INDEX IF EXISTS idx_session_last_activity;
ALTER TABLE user_sessions DROP COLUMN IF EXISTS last_activity_at;
