package mblegacyfill

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Usulan values that are executed.
const (
	UsulanFillEmpty       = "FILL_EMPTY"
	UsulanOverwrite       = "OVERWRITE"
	UsulanOverwriteStatus = "OVERWRITE_STATUS"
)

var proposalHeader = []string{
	"entity", "id", "sysid", "field", "goapps_value", "legacy_value",
	"match_method", "ambiguous", "usulan", "note",
}

// Row is one executable proposal row.
type Row struct {
	Line        int
	Entity      Entity
	ID          string
	SysID       string
	Field       string
	GoappsValue string
	LegacyValue string
	MatchMethod string
	Ambiguous   string
	Usulan      string
	Note        string
	Spec        FieldSpec
}

// RejectedRow is an executable-usulan row that must not be written.
type RejectedRow struct {
	Row    Row
	Reason string
}

// Proposal is the parsed proposal file.
type Proposal struct {
	Rows     []Row
	Rejected []RejectedRow
	Skipped  map[string]int // by usulan (REVIEW*, SKIP_*, anything not executable)
	Total    int
}

func isExecutable(u string) bool {
	return u == UsulanFillEmpty || u == UsulanOverwrite || u == UsulanOverwriteStatus
}

// ParseProposalFile opens and parses a proposal CSV.
func ParseProposalFile(path string) (p *Proposal, err error) {
	f, err := os.Open(path) //nolint:gosec // operator-supplied path
	if err != nil {
		return nil, fmt.Errorf("open proposal: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			p, err = nil, fmt.Errorf("close proposal: %w", cerr)
		}
	}()
	return ParseProposal(f)
}

// ParseProposal parses the proposal CSV, validating the header.
func ParseProposal(r io.Reader) (*Proposal, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	if err := checkHeader(head); err != nil {
		return nil, err
	}
	p := &Proposal{Skipped: map[string]int{}}
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return p, nil
		}
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if err := p.add(line, rec); err != nil {
			return nil, err
		}
	}
}

func checkHeader(head []string) error {
	if len(head) < len(proposalHeader) {
		return fmt.Errorf("proposal header has %d columns, want %d", len(head), len(proposalHeader))
	}
	for i, want := range proposalHeader {
		got := strings.TrimPrefix(strings.TrimSpace(head[i]), "\ufeff")
		if got != want {
			return fmt.Errorf("proposal header column %d = %q, want %q", i+1, got, want)
		}
	}
	return nil
}

func (p *Proposal) add(line int, rec []string) error {
	if len(rec) < len(proposalHeader)-1 {
		return fmt.Errorf("line %d: %d columns, want %d", line, len(rec), len(proposalHeader))
	}
	p.Total++
	get := func(i int) string {
		if i < len(rec) {
			return rec[i]
		}
		return ""
	}
	row := Row{
		Line: line, Entity: Entity(strings.ToLower(strings.TrimSpace(get(0)))), ID: strings.TrimSpace(get(1)),
		SysID: strings.TrimSpace(get(2)), Field: strings.ToLower(strings.TrimSpace(get(3))),
		GoappsValue: get(4), LegacyValue: get(5), MatchMethod: get(6), Ambiguous: get(7),
		Usulan: strings.TrimSpace(get(8)), Note: get(9),
	}
	if !isExecutable(row.Usulan) {
		p.Skipped[row.Usulan]++
		return nil
	}
	if !ValidEntity(row.Entity) {
		p.Rejected = append(p.Rejected, RejectedRow{row, "unknown entity " + string(row.Entity)})
		return nil
	}
	if row.ID == "" {
		p.Rejected = append(p.Rejected, RejectedRow{row, "empty id"})
		return nil
	}
	spec, ok := Lookup(row.Entity, row.Field)
	if !ok {
		p.Rejected = append(p.Rejected, RejectedRow{row, "field not writable (class X or unknown): " + row.Field})
		return nil
	}
	row.Spec = spec
	p.Rows = append(p.Rows, row)
	return nil
}

// SkippedTotal sums all skipped rows.
func (p *Proposal) SkippedTotal() int {
	n := 0
	for _, c := range p.Skipped {
		n += c
	}
	return n
}
