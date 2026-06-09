#!/bin/sh
# deploy/smoke.sh --- end-to-end sanity check for the docker-compose stack.
#
# Assumes `docker compose up -d --build` has succeeded; verifies that
# the full pipeline (postgres -> demo extension -> otel-collector ->
# jaeger) is working by:
#
#   1. waiting for postgres to be reachable through toxiproxy
#   2. building the bench from the parent dir
#   3. running a 50-iter Mode 0 + Mode 4 bench at the "intradc" preset
#   4. polling Jaeger's HTTP API for traces tagged service=postgres
#      (server-side spans from the demo extension) and service=otelbench
#      (client-side from this harness), ensuring at least one of each
#      lands.
#
# Exit codes:
#   0  --- everything wired up correctly
#   1  --- postgres unreachable
#   2  --- bench build failed
#   3  --- bench run failed
#   4  --- no spans in Jaeger after the timeout
#
# Usage:
#   ./deploy/smoke.sh                  # patched build (requires Mode 4)
#   PATCHED=0 ./deploy/smoke.sh        # unpatched build (modes 0,1a,1b,2a,2b,3)

set -e

# Sanity: must be run from anywhere in the repo, with the deploy stack
# already running. Resolves repo root via git so the script works from
# either the repo root or deploy/.
REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO_ROOT"

PATCHED="${PATCHED:-1}"
PG_DSN="postgres://postgres@127.0.0.1:5645/postgres?sslmode=disable"
OTLP_ENDPOINT="localhost:4317"
JAEGER_URL="http://localhost:16686"

echo "smoke: 1/4 wait for postgres-via-toxiproxy ${PG_DSN}"
deadline=$(( $(date +%s) + 120 ))
while ! psql "$PG_DSN" -c 'SELECT 1' >/dev/null 2>&1; do
    if [ $(date +%s) -ge $deadline ]; then
        echo "smoke: postgres unreachable after 120s --- check 'docker compose logs postgres toxiproxy'" >&2
        exit 1
    fi
    sleep 2
done
echo "  postgres reachable"

echo "smoke: 2/4 build otelbench (PATCHED=${PATCHED})"
if [ "$PATCHED" = "1" ]; then
    go build -tags=patched_pgx -o otelbench ./cmd/otelbench || exit 2
    MODES="0,1a,1b,2a,2b,3,4"
else
    go build -o otelbench ./cmd/otelbench || exit 2
    MODES="0,1a,1b,2a,2b,3"
fi

echo "smoke: 3/4 run bench (50 iterations per mode at intradc)"
./otelbench bench \
    --dsn "$PG_DSN" \
    --otlp-endpoint "$OTLP_ENDPOINT" \
    --modes "$MODES" \
    --latency intradc \
    --iterations 50 \
    --warmup 10 \
    --report /tmp/smoke-report.json \
    || exit 3
echo "  bench complete; report at /tmp/smoke-report.json"

echo "smoke: 4/4 poll Jaeger for arrived traces"
# Jaeger HTTP API: /api/services lists known service names.
# We expect both 'postgres' (from contrib/otel via demo extension) and
# 'otelbench' (from this harness's client spans) within 30s of bench
# finish; otel-collector batch + Jaeger ingest add some lag.
deadline=$(( $(date +%s) + 30 ))
found_postgres=0
found_otelbench=0
while [ $(date +%s) -lt $deadline ]; do
    services=$(curl -sf "${JAEGER_URL}/api/services" 2>/dev/null || true)
    if echo "$services" | grep -q '"postgres"'; then
        found_postgres=1
    fi
    if echo "$services" | grep -q '"otelbench"'; then
        found_otelbench=1
    fi
    if [ "$found_postgres" = "1" ] && [ "$found_otelbench" = "1" ]; then
        break
    fi
    sleep 2
done

if [ "$found_postgres" != "1" ] || [ "$found_otelbench" != "1" ]; then
    echo "smoke: missing services in Jaeger:" >&2
    echo "  postgres   = ${found_postgres}" >&2
    echo "  otelbench  = ${found_otelbench}" >&2
    echo "  check 'docker compose logs otel-collector jaeger'" >&2
    echo "  services Jaeger sees: ${services}" >&2
    exit 4
fi

echo "  postgres + otelbench services visible in Jaeger"
echo
echo "smoke: PASS"
echo "  open ${JAEGER_URL} and search for traces; you should see"
echo "  bench.iteration spans (client) with postgres-side children"
echo "  sharing the same trace_id."
