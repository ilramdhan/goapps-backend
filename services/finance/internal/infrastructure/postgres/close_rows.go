package postgres

import (
	"database/sql"

	"github.com/rs/zerolog/log"
)

// closeRows is a deferred helper that closes sql.Rows and logs any error
// instead of silently discarding it (satisfies errcheck linter).
func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		log.Warn().Err(err).Msg("failed to close rows")
	}
}
