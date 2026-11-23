package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/mitto98/homelab-proxy-manager/internal/model"
)

func host(domains ...string) model.Host {
	return model.Host{
		Kind: model.KindProxy, Enabled: true, Domains: domains,
		UpstreamScheme: "http", UpstreamHost: "192.168.1.20", UpstreamPort: 8080,
	}
}

func TestHostRoundTrip(t *testing.T) {
	s, ctx := open(t), context.Background()
	in := host("b.example.com", "a.example.com")
	in.UpstreamScheme, in.UpstreamHost, in.UpstreamPort, in.UpstreamSkipTLSVerify = "https", "nas.lan", 8443, true
	in.Enabled = false

	got, err := s.CreateHost(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == 0 || got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("id and timestamps must be set: %+v", got)
	}
	if !reflect.DeepEqual(got.Domains, []string{"b.example.com", "a.example.com"}) {
		t.Errorf("domains must keep the order given, got %v", got.Domains)
	}
	want := in
	want.ID, want.CreatedAt, want.UpdatedAt = got.ID, got.CreatedAt, got.UpdatedAt
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stored host differs:\n got  %+v\n want %+v", got, want)
	}

	again, err := s.GetHost(ctx, got.ID)
	if err != nil || !reflect.DeepEqual(again, got) {
		t.Fatalf("get: %+v, %v", again, err)
	}
}

func TestListHostsOldestFirst(t *testing.T) {
	s, ctx := open(t), context.Background()
	if hosts, err := s.ListHosts(ctx); err != nil || len(hosts) != 0 {
		t.Fatalf("empty store: %v, %v", hosts, err)
	}
	for i := 1; i <= 3; i++ {
		if _, err := s.CreateHost(ctx, host(fmt.Sprintf("h%d.example.com", i), fmt.Sprintf("h%d.example.org", i))); err != nil {
			t.Fatal(err)
		}
	}
	hosts, err := s.ListHosts(ctx)
	if err != nil || len(hosts) != 3 {
		t.Fatalf("list: %v, %v", hosts, err)
	}
	for i, h := range hosts {
		want := []string{fmt.Sprintf("h%d.example.com", i+1), fmt.Sprintf("h%d.example.org", i+1)}
		if !reflect.DeepEqual(h.Domains, want) {
			t.Errorf("host %d domains %v, want %v (each host keeps only its own domains)", i, h.Domains, want)
		}
	}
}

func TestGetUnknownHost(t *testing.T) {
	s := open(t)
	if _, err := s.GetHost(context.Background(), 42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDomainsAreUniqueAcrossHosts(t *testing.T) {
	s, ctx := open(t), context.Background()
	if _, err := s.CreateHost(ctx, host("app.example.com")); err != nil {
		t.Fatal(err)
	}

	_, err := s.CreateHost(ctx, host("other.example.com", "APP.example.com"))
	var taken *DomainTakenError
	if !errors.As(err, &taken) || taken.Domain != "app.example.com" {
		t.Fatalf("want DomainTakenError for app.example.com, got %v", err)
	}
	if !errors.Is(err, ErrConflict) {
		t.Error("DomainTakenError must match ErrConflict")
	}
	// The failed create must leave nothing behind, not even its free domain.
	hosts, _ := s.ListHosts(ctx)
	if len(hosts) != 1 {
		t.Fatalf("failed create left %d hosts", len(hosts))
	}
	if _, err := s.CreateHost(ctx, host("other.example.com")); err != nil {
		t.Fatalf("the free domain must still be usable: %v", err)
	}

	// Moving a domain onto another host is refused too.
	second, _ := s.CreateHost(ctx, host("second.example.com"))
	second.Domains = []string{"second.example.com", "app.example.com"}
	if _, err := s.UpdateHost(ctx, second); !errors.As(err, &taken) {
		t.Fatalf("update onto a taken domain: %v", err)
	}
}

func TestUpdateReplacesFieldsAndDomains(t *testing.T) {
	s, ctx := open(t), context.Background()
	h, _ := s.CreateHost(ctx, host("a.example.com", "b.example.com"))

	h.Domains = []string{"b.example.com", "c.example.com"} // keeps b, drops a, adds c
	h.UpstreamHost, h.UpstreamPort, h.Enabled = "10.0.0.5", 9000, false
	got, err := s.UpdateHost(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Domains, []string{"b.example.com", "c.example.com"}) ||
		got.UpstreamHost != "10.0.0.5" || got.UpstreamPort != 9000 || got.Enabled {
		t.Fatalf("update not applied: %+v", got)
	}
	if got.CreatedAt != h.CreatedAt {
		t.Error("created_at must not change on update")
	}
	// The dropped domain is free again.
	if _, err := s.CreateHost(ctx, host("a.example.com")); err != nil {
		t.Fatalf("a dropped domain must be reusable: %v", err)
	}
	// A host may keep its own domains while being updated.
	if _, err := s.UpdateHost(ctx, got); err != nil {
		t.Fatalf("re-saving a host with its own domains: %v", err)
	}
}

func TestUpdateUnknownHost(t *testing.T) {
	s := open(t)
	h := host("a.example.com")
	h.ID = 99
	if _, err := s.UpdateHost(context.Background(), h); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDeleteHostCascadesDomains(t *testing.T) {
	s, ctx := open(t), context.Background()
	h, _ := s.CreateHost(ctx, host("a.example.com", "b.example.com"))
	if err := s.DeleteHost(ctx, h.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteHost(ctx, h.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM host_domains`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("domains must be deleted with their host: %d, %v", n, err)
	}
	if _, err := s.CreateHost(ctx, host("a.example.com")); err != nil {
		t.Fatalf("a deleted host's domains must be reusable: %v", err)
	}
}

func TestWithTxRollsBackEverything(t *testing.T) {
	s, ctx := open(t), context.Background()
	boom := errors.New("validation failed after the change")
	err := s.WithTx(ctx, func(tx *Tx) error {
		if _, err := tx.CreateHost(ctx, host("a.example.com")); err != nil {
			return err
		}
		if _, err := tx.CreateHost(ctx, host("b.example.com")); err != nil {
			return err
		}
		// The transaction sees its own changes, as the renderer will need to.
		if hosts, err := tx.ListHosts(ctx); err != nil || len(hosts) != 2 {
			t.Errorf("inside the tx: %v, %v", hosts, err)
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("want the callback's error, got %v", err)
	}
	if hosts, _ := s.ListHosts(ctx); len(hosts) != 0 {
		t.Fatalf("rollback left %d hosts", len(hosts))
	}
	if _, err := s.CreateHost(ctx, host("a.example.com")); err != nil {
		t.Fatalf("domains from a rolled-back tx must be free: %v", err)
	}
}

func TestWithTxCommits(t *testing.T) {
	s, ctx := open(t), context.Background()
	err := s.WithTx(ctx, func(tx *Tx) error {
		h, err := tx.CreateHost(ctx, host("a.example.com"))
		if err != nil {
			return err
		}
		h.Enabled = false
		_, err = tx.UpdateHost(ctx, h)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	hosts, _ := s.ListHosts(ctx)
	if len(hosts) != 1 || hosts[0].Enabled {
		t.Fatalf("committed state wrong: %+v", hosts)
	}
}

// The schema itself refuses a proxy host without an upstream, so bad data
// cannot get in even if a code path forgets to validate.
func TestSchemaRejectsIncompleteProxyHost(t *testing.T) {
	s := open(t)
	for name, q := range map[string]string{
		"no upstream":     `INSERT INTO hosts (kind) VALUES ('proxy')`,
		"no port":         `INSERT INTO hosts (kind, upstream_scheme, upstream_host) VALUES ('proxy', 'http', 'nas')`,
		"unknown kind":    `INSERT INTO hosts (kind, upstream_scheme, upstream_host, upstream_port) VALUES ('stream', 'http', 'nas', 80)`,
		"bad scheme":      `INSERT INTO hosts (kind, upstream_scheme, upstream_host, upstream_port) VALUES ('proxy', 'ftp', 'nas', 80)`,
		"port 0":          `INSERT INTO hosts (kind, upstream_scheme, upstream_host, upstream_port) VALUES ('proxy', 'http', 'nas', 0)`,
		"port 65536":      `INSERT INTO hosts (kind, upstream_scheme, upstream_host, upstream_port) VALUES ('proxy', 'http', 'nas', 65536)`,
		"orphan domain":   `INSERT INTO host_domains (host_id, domain) VALUES (999, 'x.example.com')`,
		"enabled neither": `INSERT INTO hosts (kind, enabled, upstream_scheme, upstream_host, upstream_port) VALUES ('proxy', 2, 'http', 'nas', 80)`,
	} {
		if _, err := s.db.Exec(q); err == nil {
			t.Errorf("%s: the schema accepted %s", name, q)
		}
	}
	// A redirect host needs no upstream; the table is shared with stage 5.
	if _, err := s.db.Exec(`INSERT INTO hosts (kind) VALUES ('redirect')`); err != nil {
		t.Errorf("redirect hosts must not need an upstream: %v", err)
	}
}

func TestDomainUniquenessIsCaseInsensitiveInSchema(t *testing.T) {
	s := open(t)
	res, err := s.db.Exec(`INSERT INTO hosts (kind, upstream_scheme, upstream_host, upstream_port) VALUES ('proxy', 'http', 'nas', 80)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.db.Exec(`INSERT INTO host_domains (host_id, domain) VALUES (?, 'A.example.com')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO host_domains (host_id, domain) VALUES (?, 'a.EXAMPLE.com')`, id); err == nil {
		t.Error("the unique index must ignore case")
	}
}
