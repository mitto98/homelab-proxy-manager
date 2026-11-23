-- Hosts and their domains (docs/architecture.md section 5).
--
-- One table for every kind of host. Slice [2.4] only creates Proxy Hosts; the
-- upstream columns are nullable so Redirection Hosts (stage 5) can share the
-- table, and the CHECK below makes them mandatory for kind = 'proxy'. Later
-- stages add their columns (TLS, certificates, Access Lists, redirects) with
-- their own migrations.
CREATE TABLE hosts (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    kind                     TEXT    NOT NULL CHECK (kind IN ('proxy', 'redirect')),
    enabled                  INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    upstream_scheme          TEXT    CHECK (upstream_scheme IN ('http', 'https')),
    upstream_host            TEXT,
    upstream_port            INTEGER CHECK (upstream_port BETWEEN 1 AND 65535),
    upstream_skip_tls_verify INTEGER NOT NULL DEFAULT 0 CHECK (upstream_skip_tls_verify IN (0, 1)),
    created_at               TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at               TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    CHECK (kind <> 'proxy' OR (upstream_scheme IS NOT NULL
                               AND upstream_host IS NOT NULL
                               AND upstream_port IS NOT NULL))
);

-- A domain belongs to at most one host, of any kind: it can never be both
-- proxied and redirected, or sit on two hosts. Case-insensitive.
CREATE TABLE host_domains (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    host_id INTEGER NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    domain  TEXT    NOT NULL UNIQUE COLLATE NOCASE
);

CREATE INDEX host_domains_host_id ON host_domains (host_id);
