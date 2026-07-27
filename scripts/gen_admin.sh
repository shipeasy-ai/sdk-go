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

# OpenAPI version-compat shim. The pinned openapi-generator (openapitools.json)
# bundles a swagger-parser that cannot parse OpenAPI >= 3.2 (it NPEs before
# codegen). The canonical admin spec is emitted as 3.2.x, but its *content* is
# 3.1-compatible — only the version label is ahead of the parser. Pin the label
# down to 3.1.0 (byte-preserving: only the version token changes) so the vendored
# spec is consumable. Harmless no-op when the spec is already <= 3.1.
perl -0pi -e 's/("openapi"\s*:\s*")3\.[2-9]\.\d+(")/${1}3.1.0${2}/' "$SPEC"

echo "Generating the admin Go client from $SPEC ..."
# --skip-validate-spec: the leniently-parsed 3.2-labelled spec trips the strict
# validator (spurious "unexpected"/"missing" errors); the codegen model builder
# handles the 3.1-expressible surface correctly, so skip validation.
#
# enumClassPrefix=true is REQUIRED, not cosmetic. The Go generator otherwise
# emits enum constants named only after the VALUE, so two enums that share a
# value collide at package scope and the module does not compile. `ready_for_qa`
# and `pending_approval` appear in both OpsItemStatus and OpsInvestigationState,
# which is exactly that case. The prefix makes them OpsItemStatusREADY_FOR_QA /
# OpsInvestigationStateREADY_FOR_QA.
npx --yes @openapitools/openapi-generator-cli generate \
  -i "$SPEC" \
  -g go \
  --skip-validate-spec \
  --additional-properties=packageName=admin,isGoSubmodule=false,withGoMod=true,gitHost=github.com,gitUserId=shipeasy-ai,gitRepoId=sdk-go/admin,enumClassPrefix=true \
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
