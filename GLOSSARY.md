# Glossary

**Panel** — The web application and API through which an administrator manages the reverse proxy. Equivalent in role to Nginx Proxy Manager.

**Proxy Engine** — Traefik. The component that actually routes traffic. The Panel configures it but is a separate thing.

**Proxy Host** — A public domain name (or several) forwarded to an upstream target, optionally with TLS, an Access List and other options.

**Redirection Host** — A domain that redirects visitors to another URL instead of forwarding to an upstream.

**Certificate** — TLS credentials for one or more domains, either issued automatically (Let's Encrypt) or uploaded by an administrator.

**Access List** — A named set of rules (allowed users or IP ranges) that can be attached to a Proxy Host to restrict who may reach it.

**User** — A person who can sign in to the Panel. The first User is the administrator created at first run.
