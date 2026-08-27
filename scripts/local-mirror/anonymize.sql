-- ============================================================================
-- Anonymization pass -- runs against the `goapps` database AFTER restore.
-- ============================================================================
-- Scrubs personal data and credentials from a production copy while keeping
-- business data (products, costs, parameters, org structure) intact, so the
-- local database stays realistic for debugging.
--
-- Every column referenced here was verified against the IAM migrations in
-- goapps-backend/services/iam/migrations/postgres/. Statements are wrapped in
-- per-table existence guards so this stays safe if a table has not been
-- migrated yet, and it is idempotent -- running it twice is harmless.
--
-- Dev password for EVERY user after this runs: Dev12345!
-- ============================================================================

\set ON_ERROR_STOP on

BEGIN;

DO $anon$
DECLARE
  -- bcrypt cost 10 hash of 'Dev12345!'
  dev_hash CONSTANT text := '$2a$10$/9L0DrQ3g.q4/rlTldT/3u.BwqTfGfzWlTU/JGz36KIpbenMR8jVe';
  n bigint;
BEGIN

-- ---------------------------------------------------------------------------
-- mst_user: credentials + contact details
-- ---------------------------------------------------------------------------
-- username is NOT touched: chk_username_format constrains its shape and the
-- seeded/admin accounts are how you actually log in. email must stay valid
-- against chk_email_format, hence the @example.local domain.
IF to_regclass('public.mst_user') IS NOT NULL THEN
  UPDATE mst_user
  SET password_hash       = dev_hash,
      email               = 'user' || left(replace(user_id::text, '-', ''), 12) || '@example.local',
      two_factor_secret   = NULL,
      two_factor_enabled  = false,
      last_login_ip       = NULL,
      failed_login_attempts = 0,
      is_locked           = false,
      locked_until        = NULL;
  GET DIAGNOSTICS n = ROW_COUNT;
  RAISE NOTICE 'mst_user scrubbed: % rows', n;
END IF;

-- ---------------------------------------------------------------------------
-- mst_user_detail: names, phone, address, DOB, free-form extra_data
-- ---------------------------------------------------------------------------
-- employee_code is preserved -- it is a business key that reports join on.
IF to_regclass('public.mst_user_detail') IS NOT NULL THEN
  UPDATE mst_user_detail
  SET full_name           = 'User ' || left(replace(user_id::text, '-', ''), 8),
      first_name          = 'User',
      last_name           = left(replace(user_id::text, '-', ''), 8),
      phone               = NULL,
      address             = NULL,
      date_of_birth       = NULL,
      profile_picture_url = NULL,
      extra_data          = NULL;
  GET DIAGNOSTICS n = ROW_COUNT;
  RAISE NOTICE 'mst_user_detail scrubbed: % rows', n;
END IF;

-- ---------------------------------------------------------------------------
-- Auth material: delete outright rather than mask.
-- ---------------------------------------------------------------------------
-- These are all short-lived hashed tokens. Keeping masked rows would let a
-- local run accept a token minted against production, so they are emptied.
IF to_regclass('public.user_sessions') IS NOT NULL THEN
  DELETE FROM user_sessions;
  RAISE NOTICE 'user_sessions emptied';
END IF;

IF to_regclass('public.password_reset_tokens') IS NOT NULL THEN
  DELETE FROM password_reset_tokens;
  RAISE NOTICE 'password_reset_tokens emptied';
END IF;

IF to_regclass('public.user_recovery_codes') IS NOT NULL THEN
  DELETE FROM user_recovery_codes;
  RAISE NOTICE 'user_recovery_codes emptied';
END IF;

-- API keys: real key_hash values would let a leaked prod key authenticate
-- locally. Rows are deleted so key management can be re-tested from scratch.
IF to_regclass('public.api_keys') IS NOT NULL THEN
  DELETE FROM api_keys;
  RAISE NOTICE 'api_keys emptied';
END IF;

-- ---------------------------------------------------------------------------
-- Chat: end-to-end encrypted message bodies + attachments
-- ---------------------------------------------------------------------------
-- ciphertext/nonce are useless locally anyway (the key version won't match),
-- and they are private employee conversations. Child tables go first to
-- respect the FKs; thread/conversation rows are kept so the UI still renders.
IF to_regclass('public.chat_message_edit_history') IS NOT NULL THEN
  DELETE FROM chat_message_edit_history;
END IF;
IF to_regclass('public.chat_read_receipt') IS NOT NULL THEN
  DELETE FROM chat_read_receipt;
END IF;
IF to_regclass('public.chat_attachment') IS NOT NULL THEN
  DELETE FROM chat_attachment;
END IF;
IF to_regclass('public.chat_message') IS NOT NULL THEN
  DELETE FROM chat_message;
  RAISE NOTICE 'chat_* message data emptied';
END IF;

-- ---------------------------------------------------------------------------
-- Notifications: titles/bodies quote real business events and names.
-- ---------------------------------------------------------------------------
IF to_regclass('public.mst_notification') IS NOT NULL THEN
  DELETE FROM mst_notification;
  RAISE NOTICE 'mst_notification emptied';
END IF;

-- ---------------------------------------------------------------------------
-- Audit logs: IPs, user agents, and full old_data/new_data row snapshots
-- (which contain pre-scrub copies of everything above).
-- ---------------------------------------------------------------------------
-- NOTE ON THE COLLISION: finance and iam BOTH define `audit_logs` in the
-- shared `public` schema with different columns (finance: id/action/request_id;
-- iam: log_id/event_type/user_id). Both use CREATE TABLE IF NOT EXISTS, so
-- production has exactly one of the two shapes -- whichever service migrated
-- first. A plain DELETE works regardless of which shape landed, which is why
-- this does not name any column.
IF to_regclass('public.audit_logs') IS NOT NULL THEN
  DELETE FROM audit_logs;
  RAISE NOTICE 'audit_logs emptied';
END IF;

IF to_regclass('public.chatbot_audit_log') IS NOT NULL THEN
  DELETE FROM chatbot_audit_log;
END IF;

END
$anon$;

COMMIT;

-- Report what remains, so the sync script's output shows the scrub took effect.
\echo ''
\echo '--- post-anonymize check (expect masked emails, zero sessions) ---'
SELECT count(*) AS users,
       count(*) FILTER (WHERE email LIKE '%@example.local') AS masked_emails
FROM mst_user;
