package pgcontainer

import (
	"errors"
	"testing"
)

func TestCheckHostAllowed(t *testing.T) {
	cases := []struct {
		url   string
		extra []string
		ok    bool
	}{
		{"postgres://u:p@localhost:5432/db", nil, true},
		{"postgres://u:p@127.0.0.1:5432/db", nil, true},
		{"postgresql://u:p@[::1]:5432/db", nil, true},
		{"postgres://u:p@127.0.0.2:5432/db", nil, true}, // loopback range
		{"postgres://u:p@docker-host:1234/db", []string{"docker-host"}, true},
		{"postgres://u:p@10.0.0.5:5432/db", nil, false},
		{"postgres://u:p@db.staging.example.com:5432/db", nil, false},
		{"postgres://u:p@pgbouncer.database.svc:6432/finance", []string{"docker-host"}, false},
		{"mysql://u:p@localhost/db", nil, false},
		{"host=localhost user=u", nil, false},
		{"postgres:///db", nil, false},
	}
	for _, tc := range cases {
		err := CheckHostAllowed(tc.url, tc.extra...)
		if tc.ok && err != nil {
			t.Errorf("%s: unexpected error %v", tc.url, err)
		}
		if !tc.ok && !errors.Is(err, ErrHostNotAllowed) {
			t.Errorf("%s: want ErrHostNotAllowed, got %v", tc.url, err)
		}
	}
}

func TestLoadMigrationsRepoChain(t *testing.T) {
	migs, err := LoadMigrations("../../../migrations/postgres")
	if err != nil {
		t.Fatalf("LoadMigrations: %v", err)
	}
	seen := map[uint64]bool{}
	var prev uint64
	for _, m := range migs {
		if m.Version <= prev {
			t.Fatalf("not strictly ascending at %d", m.Version)
		}
		prev = m.Version
		seen[m.Version] = true
	}
	for _, v := range []uint64{547, 548, 549, 550, 551, 552, 553, 554, 555, 557, 558, 559} {
		if !seen[v] {
			t.Errorf("migration %06d missing", v)
		}
	}
	if seen[556] {
		t.Errorf("000556 is gated and must not exist in this lane")
	}
}
