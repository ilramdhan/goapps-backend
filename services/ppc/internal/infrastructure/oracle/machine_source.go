package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

// TxtMachine is a raw row from Oracle TXTMACH used by the machine sync.
type TxtMachine struct {
	// MachNo is TXTMACH.MACH_NO, the natural machine key.
	MachNo string
	// Dept is TXTMACH.MACH_DEPT (TXT/TWT/SPG), mapped to machine_area.
	Dept string
	// OrionCode is TXTMACH.MACH_ORION, the display code used by Orion
	// ("TXT 01", "ACY 01"). Populated for every row in the live master.
	OrionCode string
	// Line is TXTMACH.MACH_LINE ("TXLINE1", "TFOLINE", ...). May be empty:
	// 24 of 179 live rows carry no line.
	Line string
	// Group is TXTMACH.MACH_GROUP, the machine-group name ("Texturising
	// Machine", "TFO Machine", ...). TXTMACH is the source of truth for the
	// machine→group assignment; the sync resolves it to a machine_group row.
	Group string
}

// ListTxtMachines reads the ASPTXT.TXTMACH master.
// Read-only. Schema-qualified because the connecting user (MGTDAT) does not own
// TXTMACH — it lives in the ASPTXT schema (see PPC_ORACLE_DDL.sql), and an
// unqualified name would resolve to the caller's own schema and fail.
//
// Column set verified against ALL_TAB_COLUMNS on the live master (2026-07-28):
// TXTMACH has no doff-weight column, and MACH_TYPE is NULL on 171 of 179 rows,
// so neither is read here — doff weight stays PPC-local.
func (c *Client) ListTxtMachines(ctx context.Context) ([]TxtMachine, error) {
	if c == nil || c.db == nil {
		return nil, nil
	}
	const query = `SELECT MACH_NO, MACH_DEPT, MACH_ORION, MACH_LINE, MACH_GROUP FROM ASPTXT.TXTMACH`
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query TXTMACH: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("oracle: failed to close TXTMACH rows")
		}
	}()

	var result []TxtMachine
	for rows.Next() {
		var machNo, dept, orion, line, group sql.NullString
		if scanErr := rows.Scan(&machNo, &dept, &orion, &line, &group); scanErr != nil {
			return nil, fmt.Errorf("scan TXTMACH row: %w", scanErr)
		}
		result = append(result, TxtMachine{
			MachNo:    strings.TrimSpace(machNo.String),
			Dept:      strings.TrimSpace(dept.String),
			OrionCode: strings.TrimSpace(orion.String),
			Line:      strings.TrimSpace(line.String),
			Group:     strings.TrimSpace(group.String),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate TXTMACH rows: %w", err)
	}
	return result, nil
}
