#!/usr/bin/env bash
# Regenerates everything derived from api/openapi.yaml:
#   internal/api/api.gen.go      Go types, routing and the strict server interface (oapi-codegen)
#   web/src/api/schema.d.ts      TypeScript types for the client (openapi-typescript)
# CI runs this script and fails if it leaves a diff.
set -euo pipefail
cd "$(dirname "$0")/.."

go generate ./...
(cd web && npm ci --no-audit --no-fund && npm run generate:api)
