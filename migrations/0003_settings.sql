-- Instance settings (docs/architecture.md section 5): a single typed row.
--
-- The walking skeleton only needs the three Traefik ports. Later stages add
-- their columns (ACME defaults, Panel port, DNS provider, ...) with their own
-- migrations, so this table stays typed instead of becoming key/value pairs.
--
-- Defaults: 80 and 443 are the standard entrypoints. The Traefik API listens on
-- 127.0.0.1:8899, not Traefik's own 8080, because 8080 is the port most often
-- taken by the upstreams a homelab proxies to.
CREATE TABLE settings (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    http_port  INTEGER NOT NULL DEFAULT 80   CHECK (http_port  BETWEEN 1 AND 65535),
    https_port INTEGER NOT NULL DEFAULT 443  CHECK (https_port BETWEEN 1 AND 65535),
    api_port   INTEGER NOT NULL DEFAULT 8899 CHECK (api_port   BETWEEN 1 AND 65535),
    updated_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

INSERT INTO settings (id) VALUES (1);
