// Package pgcontainer is the local-only PostgreSQL test harness (plan-01
// P0-T6, design §16).
//
// It starts a throwaway PostgreSQL 16 container via testcontainers-go, or —
// when TEST_DATABASE_URL is set — uses that server instead. Either way the
// target host must be local: CheckHostAllowed refuses anything other than
// localhost / 127.0.0.1 / ::1 (plus the host testcontainers reports for a
// container this package started itself). Shared, staging and production
// databases can therefore never be targeted from tests.
//
// It also carries a small ordered migration applier that mirrors the
// golang-migrate CLI used by the Makefile: each *.up.sql / *.down.sql file is
// sent as ONE simple-protocol Exec, in numeric version order, gaps allowed.
package pgcontainer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	// Registers the "pgx" database/sql driver.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// EnvDatabaseURL names the optional override pointing at an existing LOCAL
// PostgreSQL server (URL form, e.g. postgres://u:p@localhost:5432/db).
const EnvDatabaseURL = "TEST_DATABASE_URL"

// Image is the PostgreSQL image the harness starts (plan-01 P0-T6: PG 16).
const Image = "postgres:16-alpine"

const (
	containerDB   = "goapps_test"
	containerUser = "goapps_test"
	// containerPassword is a throwaway credential for an ephemeral local
	// container; it is not a secret of any real environment.
	containerPassword = "goapps_test"
)

// localHosts are the only hosts a TEST_DATABASE_URL may point at.
var localHosts = map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true}

// ErrHostNotAllowed is returned when a connection target is not local.
var ErrHostNotAllowed = errors.New("pgcontainer: database host not allowed (local/container only)")

// CheckHostAllowed returns nil only when rawURL is a postgres:// URL whose
// host is localhost, 127.0.0.1, ::1 or one of extraHosts (used for the host
// testcontainers reports for a container this package started).
func CheckHostAllowed(rawURL string, extraHosts ...string) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return fmt.Errorf("%w: only postgres:// URLs are accepted", ErrHostNotAllowed)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: empty host", ErrHostNotAllowed)
	}
	if localHosts[strings.ToLower(host)] {
		return nil
	}
	for _, h := range extraHosts {
		if h != "" && strings.EqualFold(h, host) {
			return nil
		}
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("%w: %q", ErrHostNotAllowed, host)
}

// Server is a running local PostgreSQL server usable by tests.
type Server struct {
	base *url.URL
}

// Start returns a local PostgreSQL server for the test, registering cleanup
// with t. It fails the test if the server cannot be started or the target is
// not local.
func Start(ctx context.Context, t testing.TB) *Server {
	t.Helper()

	if raw := os.Getenv(EnvDatabaseURL); raw != "" {
		if err := CheckHostAllowed(raw); err != nil {
			t.Fatalf("%s rejected: %v", EnvDatabaseURL, err)
		}
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %s: %v", EnvDatabaseURL, err)
		}
		return &Server{base: u}
	}

	ctr, err := tcpostgres.Run(ctx, Image,
		tcpostgres.WithDatabase(containerDB),
		tcpostgres.WithUsername(containerUser),
		tcpostgres.WithPassword(containerPassword),
		tcpostgres.BasicWaitStrategies(),
	)
	if ctr != nil {
		t.Cleanup(func() {
			if termErr := testcontainers.TerminateContainer(ctr); termErr != nil {
				t.Logf("terminate postgres container: %v", termErr)
			}
		})
	}
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	raw, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("container connection string: %v", err)
	}
	if err := CheckHostAllowed(raw, host); err != nil {
		t.Fatalf("container URL rejected: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse container URL: %v", err)
	}
	return &Server{base: u}
}

// URL returns the connection URL for database dbName on this server.
func (s *Server) URL(dbName string) string {
	u := *s.base
	u.Path = "/" + dbName
	return u.String()
}

// DefaultURL returns the URL of the server's own database.
func (s *Server) DefaultURL() string { return s.base.String() }

// Open opens database dbName and registers Close with t.
func (s *Server) Open(t testing.TB, dbName string) *sql.DB {
	t.Helper()
	raw := s.URL(dbName)
	if err := CheckHostAllowed(raw, s.base.Hostname()); err != nil {
		t.Fatalf("open: %v", err)
	}
	db, err := sql.Open("pgx", raw)
	if err != nil {
		t.Fatalf("open %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		if cerr := db.Close(); cerr != nil {
			t.Logf("close %s: %v", dbName, cerr)
		}
	})
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("ping %s: %v", dbName, err)
	}
	return db
}

var identRe = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// CreateDatabase creates database name (optionally cloned from template),
// drops it on cleanup, and returns an open handle to it. The statement runs
// from the "postgres" maintenance database so that a template database can
// be cloned (PostgreSQL requires no open connections on the template; the
// caller must close its own handles to it first).
func (s *Server) CreateDatabase(t testing.TB, name, template string) *sql.DB {
	t.Helper()
	if !identRe.MatchString(name) || (template != "" && !identRe.MatchString(template)) {
		t.Fatalf("invalid database identifier %q / %q", name, template)
	}
	admin := s.Open(t, "postgres")
	ctx := context.Background()
	stmt := "CREATE DATABASE " + name
	if template != "" {
		stmt += " TEMPLATE " + template
	}
	if _, err := admin.ExecContext(ctx, stmt); err != nil {
		t.Fatalf("%s: %v", stmt, err)
	}
	db := s.Open(t, name)
	t.Cleanup(func() {
		if cerr := db.Close(); cerr != nil {
			t.Logf("close %s: %v", name, cerr)
		}
		if _, err := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Logf("drop database %s: %v", name, err)
		}
	})
	return db
}

// Migration is one numbered up/down pair on disk.
type Migration struct {
	Version  uint64
	Name     string
	UpPath   string
	DownPath string
}

var migFileRe = regexp.MustCompile(`^(\d+)_(.+)\.(up|down)\.sql$`)

// LoadMigrations lists the migrations in dir, sorted by version. Every
// version must have both an up and a down file.
func LoadMigrations(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	byVer := map[uint64]*Migration{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := migFileRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		v, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse version %q: %w", e.Name(), err)
		}
		mig := byVer[v]
		if mig == nil {
			mig = &Migration{Version: v, Name: m[2]}
			byVer[v] = mig
		} else if mig.Name != m[2] {
			return nil, fmt.Errorf("version %d has two names: %q and %q", v, mig.Name, m[2])
		}
		p := filepath.Join(dir, e.Name())
		if m[3] == "up" {
			mig.UpPath = p
		} else {
			mig.DownPath = p
		}
	}
	out := make([]Migration, 0, len(byVer))
	for _, m := range byVer {
		if m.UpPath == "" || m.DownPath == "" {
			return nil, fmt.Errorf("migration %06d_%s is missing its up or down file", m.Version, m.Name)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// ExecSQL runs a whole SQL script as one simple-protocol Exec on a dedicated
// connection (as golang-migrate does). On failure it issues ROLLBACK so a
// script that opened its own BEGIN does not leave the connection aborted.
func ExecSQL(ctx context.Context, db *sql.DB, script string) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("release conn: %w", cerr)
		}
	}()
	if _, execErr := conn.ExecContext(ctx, script); execErr != nil {
		// Outside a transaction ROLLBACK only warns; its error is secondary
		// to the script failure, so it is joined rather than returned alone.
		if _, rbErr := conn.ExecContext(ctx, "ROLLBACK"); rbErr != nil {
			return errors.Join(execErr, fmt.Errorf("rollback: %w", rbErr))
		}
		return execErr
	}
	return nil
}

// ExecFile runs the SQL file at path via ExecSQL.
func ExecFile(ctx context.Context, db *sql.DB, path string) error {
	b, err := os.ReadFile(path) //nolint:gosec // test helper reading repo migration files
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := ExecSQL(ctx, db, string(b)); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return nil
}

// versionTable tracks the applied version inside the test database (so it
// survives CREATE DATABASE … TEMPLATE clones). It is harness-only and never
// named like the real schema_migrations_finance table.
const versionTable = "pgcontainer_schema_version"

// Migrator applies the on-disk migrations to one database.
type Migrator struct {
	db   *sql.DB
	migs []Migration
}

// NewMigrator loads dir and prepares the version table in db.
func NewMigrator(ctx context.Context, db *sql.DB, dir string) (*Migrator, error) {
	migs, err := LoadMigrations(dir)
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+versionTable+" (version BIGINT NOT NULL)"); err != nil {
		return nil, fmt.Errorf("create version table: %w", err)
	}
	return &Migrator{db: db, migs: migs}, nil
}

// Migrations returns the loaded migrations in version order.
func (m *Migrator) Migrations() []Migration { return m.migs }

// Latest returns the highest migration version on disk.
func (m *Migrator) Latest() uint64 {
	if len(m.migs) == 0 {
		return 0
	}
	return m.migs[len(m.migs)-1].Version
}

// Version returns the currently applied version (0 when none).
func (m *Migrator) Version(ctx context.Context) (uint64, error) {
	var v sql.NullInt64
	if err := m.db.QueryRowContext(ctx, "SELECT max(version) FROM "+versionTable).Scan(&v); err != nil {
		return 0, fmt.Errorf("read version: %w", err)
	}
	if !v.Valid || v.Int64 < 0 {
		return 0, nil
	}
	return uint64(v.Int64), nil
}

func (m *Migrator) setVersion(ctx context.Context, v uint64) error {
	if _, err := m.db.ExecContext(ctx, "DELETE FROM "+versionTable); err != nil {
		return fmt.Errorf("clear version: %w", err)
	}
	if v == 0 {
		return nil
	}
	if _, err := m.db.ExecContext(ctx, "INSERT INTO "+versionTable+" (version) VALUES ($1)", int64(v)); err != nil { //nolint:gosec // versions are small
		return fmt.Errorf("write version: %w", err)
	}
	return nil
}

// UpTo applies every migration with current < version <= target, in order.
func (m *Migrator) UpTo(ctx context.Context, target uint64) error {
	cur, err := m.Version(ctx)
	if err != nil {
		return err
	}
	for _, mig := range m.migs {
		if mig.Version <= cur || mig.Version > target {
			continue
		}
		if err := ExecFile(ctx, m.db, mig.UpPath); err != nil {
			return fmt.Errorf("up %06d: %w", mig.Version, err)
		}
		if err := m.setVersion(ctx, mig.Version); err != nil {
			return err
		}
	}
	return nil
}

// Up applies all pending migrations.
func (m *Migrator) Up(ctx context.Context) error { return m.UpTo(ctx, m.Latest()) }

// DownTo reverts every applied migration with version > target, newest first.
func (m *Migrator) DownTo(ctx context.Context, target uint64) error {
	cur, err := m.Version(ctx)
	if err != nil {
		return err
	}
	for i := len(m.migs) - 1; i >= 0; i-- {
		mig := m.migs[i]
		if mig.Version > cur || mig.Version <= target {
			continue
		}
		if err := ExecFile(ctx, m.db, mig.DownPath); err != nil {
			return fmt.Errorf("down %06d: %w", mig.Version, err)
		}
		var prev uint64
		if i > 0 {
			prev = m.migs[i-1].Version
		}
		if err := m.setVersion(ctx, prev); err != nil {
			return err
		}
	}
	return nil
}
