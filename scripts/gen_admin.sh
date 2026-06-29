#!/usr/bin/env bash
#
# Regenerate the OPTIONAL Admin API client (the `admin` Go module) from the
# vendored OpenAPI spec. The generated client is a raw, 1:1 projection of
# `admin/openapi.json` (id-based, basis-points, snake_case) — no name->id or
# percent->bp ergonomics. The hand-written `admin/client.go` shim (the
# `admin.NewClient` entry point) sits on top and is NEVER touched by this script:
# only the generator-emitted Go files are replaced.
#
# The `admin/` directory is a SEPARATE Go module (`module
# github.com/shipeasy-ai/sdk-go/admin`) so the root SDK never depends on it —
# users opt in with `import "github.com/shipeasy-ai/sdk-go/admin"`. This mirrors
# the nested `openfeature/` module.
#
# Usage:
#   1. Refresh the vendored spec when the contract changes:
#        cp <monorepo>/marketplace/openapi/openapi.json admin/openapi.json
#   2. Regenerate:
#        bash scripts/gen_admin.sh
#   3. Commit `admin/openapi.json` + the regenerated `admin/*.go` files.
#
# Requires Java (for openapi-generator) and npx. The generator version is pinned
# in `openapitools.json` (7.23.0).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

SPEC="admin/openapi.json"
DEST="admin"
BUILD="$(mktemp -d)"
trap 'rm -rf "$BUILD"' EXIT

if [[ ! -f "$SPEC" ]]; then
  echo "error: missing vendored spec at $SPEC — copy it from the monorepo's marketplace/openapi/openapi.json" >&2
  exit 1
fi

echo "Generating the admin Go client from $SPEC ..."
npx --yes @openapitools/openapi-generator-cli generate \
  -i "$SPEC" \
  -g go \
  --additional-properties=packageName=admin,isGoSubmodule=false,withGoMod=true,gitHost=github.com,gitUserId=shipeasy-ai,gitRepoId=sdk-go/admin \
  -o "$BUILD" >/dev/null

if ! ls "$BUILD"/*.go >/dev/null 2>&1; then
  echo "error: generator did not produce any Go files in $BUILD" >&2
  exit 1
fi

# Remove ONLY previously-generated files, keeping the hand-written shim + spec.
# The generator emits api_*.go, model_*.go, client.go (named api_client.go here),
# configuration.go, response.go, utils.go, etc. Our shim lives in admin/client.go
# — to avoid a name clash the generator's client is renamed (see below); guard by
# never deleting our shim/spec/go.mod-by-hand files.
KEEP=("$DEST/openapi.json")
# Delete all generator-owned Go files (everything except our hand-written shim).
find "$DEST" -maxdepth 1 -name '*.go' \
  ! -name 'client.go' ! -name 'client_test.go' \
  -delete

# Copy generated Go sources + supporting files in. The generator's own
# `client.go` (the ApiClient) would collide with our shim — rename it.
for f in "$BUILD"/*.go "$BUILD"/*.gomod "$BUILD"/go.mod "$BUILD"/go.sum; do
  [[ -e "$f" ]] || continue
  base="$(basename "$f")"
  if [[ "$base" == "client.go" ]]; then
    cp "$f" "$DEST/api_client.go"
  else
    cp "$f" "$DEST/$base"
  fi
done

# The generated go.mod declares the module path; make sure it matches the
# nested-module convention even if the additional-properties drift.
if [[ -f "$DEST/go.mod" ]]; then
  sed -i.bak '1s|^module .*|module github.com/shipeasy-ai/sdk-go/admin|' "$DEST/go.mod" && rm -f "$DEST/go.mod.bak"
fi

echo "Wrote $(find "$DEST" -maxdepth 1 -name '*.go' | wc -l | tr -d ' ') Go files to $DEST"
echo "Done. Review the diff and commit admin/openapi.json + admin/*.go."
