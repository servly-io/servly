#!/usr/bin/env bash
# Run every Go test on SQLite and PostgreSQL.
#
# With SERVLY_TEST_POSTGRES_URL set, that server is used. Otherwise a
# throwaway PostgreSQL container is started (podman, else docker) and removed
# afterwards. Each test creates and drops its own schema.
#
# usage: scripts/test-integration.sh [go test args...]

set -euo pipefail

PG_IMAGE="${PG_IMAGE:-docker.io/library/postgres:17-alpine}"
GO="${GO:-go}"

if [[ -z "${SERVLY_TEST_POSTGRES_URL:-}" ]]; then
  engine="$(command -v podman || command -v docker || true)"
  if [[ -z "$engine" ]]; then
    echo "need podman or docker, or set SERVLY_TEST_POSTGRES_URL" >&2
    exit 1
  fi
  name="servly-it-$$"
  port="${PG_PORT:-55433}"
  "$engine" run -d --rm --name "$name" \
    -e POSTGRES_USER=servly -e POSTGRES_PASSWORD=servly -e POSTGRES_DB=servly \
    -p "127.0.0.1:$port:5432" "$PG_IMAGE" >/dev/null
  trap '"$engine" rm -f "$name" >/dev/null 2>&1 || true' EXIT
  for _ in $(seq 1 60); do
    if "$engine" exec "$name" pg_isready -U servly -d servly >/dev/null 2>&1; then
      break
    fi
    sleep 0.5
  done
  # pg_isready can pass during the image's init restart; wait for a query.
  for _ in $(seq 1 60); do
    if "$engine" exec "$name" psql -U servly -d servly -c 'select 1' >/dev/null 2>&1; then
      break
    fi
    sleep 0.5
  done
  export SERVLY_TEST_POSTGRES_URL="postgres://servly:servly@127.0.0.1:$port/servly?sslmode=disable"
fi

"$GO" test "${@:--count=1}" ./...
