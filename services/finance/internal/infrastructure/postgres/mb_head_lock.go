package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// lockClauses renders the mst_mb_head lock-column assignments for one transition and
// returns them as a SQL fragment plus the arguments it consumes.
//
// 🔴 Placeholder numbers are computed from the CALLER'S current argument count, which
// is why args comes in and goes out. ⛔ Never hardcode $N here: mb_head_transition.go
// already appends a conditional mbh_check_status_calc clause, so the next free index
// is not a constant.
//
// An empty fragment means "this transition touches no lock column" — the caller then
// leaves them all alone rather than writing defaults over them.
func lockClauses(effect mbhead.LockEffect, actorUserID, reason string, args []any) (string, []any) {
	frag := ""
	if effect.SetLocked != nil {
		args = append(args, *effect.SetLocked)
		frag += fmt.Sprintf(`, mbh_is_locked = $%d`, len(args))
		if *effect.SetLocked {
			// Locked: stamp who and when. ⛔ mbh_locked_at/by are NOT cleared on
			// unlock — they stay as the trail of the lock that was lifted.
			args = append(args, actorUserID)
			frag += fmt.Sprintf(`, mbh_locked_at = NOW(), mbh_locked_by = NULLIF($%d, '')`, len(args))
		}
	}
	if effect.SetUnlockRequest {
		args = append(args, actorUserID, reason)
		frag += fmt.Sprintf(
			`, mbh_unlock_requested_at = NOW(), mbh_unlock_requested_by = NULLIF($%d, ''),`+
				` mbh_unlock_reason = NULLIF($%d, '')`,
			len(args)-1, len(args))
	}
	if effect.ClearUnlockRequest {
		// 🔴 mbh_unlock_reason is deliberately ABSENT from this list. Clearing the
		// pending markers is what "no request is waiting" means; erasing the REASON
		// would delete the record of why the unlock was ever asked for (principle U-2).
		frag += `, mbh_unlock_requested_at = NULL, mbh_unlock_requested_by = NULL`
	}
	return frag, args
}

// insertLockLogTx writes exactly ONE mst_mb_head_lock_log row for a lock-related
// transition.
//
// 🔴 It MUST run inside the same *sql.Tx as the mst_mb_head update — a lock that
// commits without its audit row, or an audit row without the lock, is worse than
// either failing. The callers in mb_head_transition.go guarantee this.
//
// 🔴 effect.Event is ALWAYS one of the five values mbhl_event's CHECK constraint
// accepts (migration 000485); DeriveLockEffect is the only producer and it emits
// nothing else. An empty Event means the transition is not lock-related and this
// function is not called at all.
//
// mbhl_auto_relock_at records the auto-relock DEADLINE on grant. ⚠ Recording it is
// all this phase does — no job acts on it yet (see mbhead.AutoRelockAfter).
func insertLockLogTx(
	ctx context.Context, tx *sql.Tx, id uuid.UUID,
	effect mbhead.LockEffect, actorUserID, reason string,
) error {
	// 🔴 ONE static query, fully parameterized. ⛔ The interval is NOT interpolated
	// into the SQL text (gosec G201, and a genuine injection shape even with a
	// constant today): make_interval takes the seconds as a bind parameter, and the
	// CASE turns the flag into NULL when no deadline applies. The deadline is still
	// measured against the DATABASE clock, matching mbhl_actor_at's NOW() default.
	//
	// The seconds value comes from the single named constant mbhead.AutoRelockAfter,
	// so the duration lives in exactly one place.
	const q = `
		INSERT INTO mst_mb_head_lock_log
			(mbhl_mbh_id, mbhl_event, mbhl_actor_user_id, mbhl_reason, mbhl_auto_relock_at)
		VALUES ($1, $2, $3, NULLIF($4, ''),
		        CASE WHEN $5::boolean THEN NOW() + make_interval(secs => $6::double precision)
		             ELSE NULL END)`
	_, err := tx.ExecContext(ctx, q,
		id, effect.Event, actorUserID, reason,
		effect.SetAutoRelock, mbhead.AutoRelockAfter.Seconds(),
	)
	if err != nil {
		return fmt.Errorf("mb_head_lock: insert lock log (%s): %w", effect.Event, err)
	}
	return nil
}

// unlockRequesterMetaKey is the mbhl_meta JSON key holding the requester's IAM user
// UUID. 🔴 ONE name, used by both the write and the read below — ⛔ never spelled
// inline anywhere else: a typo on one side silently loses every E4/E5 notification.
const unlockRequesterMetaKey = "requested_by_uuid"

// RecordUnlockRequestActor stamps the requester's IAM user UUID onto the mbhl_meta of
// the head's most recent UNLOCK_REQUEST lock-log row.
//
// 🔴 It writes mbhl_meta and NOTHING else. That column has existed since migration
// 000485 and no Go code has ever read or written it, so this needs ⛔ no migration and
// cannot disturb existing rows — pre-existing rows simply keep their NULL.
//
// jsonb_set on COALESCE(mbhl_meta, '{}'::jsonb) MERGES rather than replaces, so a future
// writer of some other meta key is not clobbered by this one.
//
// ⛔ The UUID is bound as a parameter and cast to jsonb via to_jsonb($2::text) — never
// concatenated into the JSON text.
func (r *MBHeadRepository) RecordUnlockRequestActor(ctx context.Context, mbhID uuid.UUID, actorUUID string) error {
	const q = `
		UPDATE mst_mb_head_lock_log
		SET mbhl_meta = jsonb_set(COALESCE(mbhl_meta, '{}'::jsonb), ARRAY[$3::text], to_jsonb($2::text), true)
		WHERE mbhl_id = (
			SELECT mbhl_id FROM mst_mb_head_lock_log
			WHERE mbhl_mbh_id = $1 AND mbhl_event = $4
			ORDER BY mbhl_actor_at DESC
			LIMIT 1
		)`
	if _, err := r.db.ExecContext(ctx, q,
		mbhID, actorUUID, unlockRequesterMetaKey, mbhead.LockEventUnlockRequest,
	); err != nil {
		return fmt.Errorf("mb_head_lock: record unlock requester: %w", err)
	}
	return nil
}

// LatestUnlockRequestActor returns the UUID stamped on the head's most recent
// UNLOCK_REQUEST row, or "" when there is none.
//
// 🔴 "" is a NORMAL answer, ⛔ not an error: every UNLOCK_REQUEST row written before
// this feature landed has mbhl_meta = NULL, and a request raised by a system/service
// caller has no UUID to stamp. The caller must treat "" as "notify nobody" — sending a
// blank BY_USER_ID rule to IAM would fail its uuid.Parse and abandon the whole fan-out.
//
// ->> yields SQL NULL for a missing key or a NULL column alike, so one COALESCE covers
// both shapes without a second query. ⛔ The $2::text cast is REQUIRED, not decorative:
// jsonb ->> has both a text and an integer overload, and an uncast parameter makes the
// operator ambiguous at prepare time.
func (r *MBHeadRepository) LatestUnlockRequestActor(ctx context.Context, mbhID uuid.UUID) (string, error) {
	const q = `
		SELECT COALESCE(mbhl_meta ->> $2::text, '')
		FROM mst_mb_head_lock_log
		WHERE mbhl_mbh_id = $1 AND mbhl_event = $3
		ORDER BY mbhl_actor_at DESC
		LIMIT 1`
	var out string
	err := r.db.QueryRowContext(ctx, q, mbhID, unlockRequesterMetaKey, mbhead.LockEventUnlockRequest).Scan(&out)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("mb_head_lock: read unlock requester: %w", err)
	}
	return out, nil
}
