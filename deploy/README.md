# Docker-compose published demo stack

Brings up the four services the bench needs in one network:

```
   ┌────────────────────────┐
   │ otelbench (on the host)│
   └─────────────┬──────────┘
                 │ pgx → toxiproxy:5645 (mapped to host:5645)
                 │ OTLP → otel-collector:4317 (mapped to host:4317)
                 ↓
   ┌────────────────────────┐
   │ toxiproxy              │ pre-seeded with proxy "postgres" → postgres:5432
   └─────────────┬──────────┘
                 │
                 ↓
   ┌────────────────────────┐
   │ postgres               │ patched + contrib/otel + otel_postgres_tracing
   │                        │ + postgres_otel_tracing_demo (Rust .so)
   │                        │ + pg_stat_statements
   └─────────────┬──────────┘
                 │ OTLP gRPC (via demo extension)
                 ↓
   ┌────────────────────────┐
   │ otel-collector         │
   └─────────────┬──────────┘
                 │ OTLP gRPC
                 ↓
   ┌────────────────────────┐
   │ jaeger (all-in-one)    │ UI on host:16686
   └────────────────────────┘
```

## Status

**Not yet verified end-to-end.** The config files in this directory
are written but the postgres image hasn't been built or run by the
author. First-time `docker compose up --build` is expected to take
15–20 minutes (postgres-from-source ≈ 5 min, Rust extension build ≈
5 min, base apt setup the rest). Report bugs.

## Bring it up

```bash
cd deploy
docker compose up --build       # first time (15-20 min cold; mostly postgres-from-source + rust)
docker compose up -d            # subsequent (already-built images)
docker compose logs -f postgres # tail postgres logs (includes init.sh + contrib/otel)
```

## Verify it works

```bash
./deploy/smoke.sh                          # patched build (default --- includes Mode 4)
PATCHED=0 ./deploy/smoke.sh                # unpatched build (modes 0,1a,1b,2a,2b,3 only)
```

The smoke script waits for postgres-via-toxiproxy, builds the bench
from this repo, runs a 50-iter cell across every mode at the `intradc`
preset, then polls Jaeger's HTTP API to confirm both `postgres` (server-
side spans from the demo extension) and `otelbench` (client-side
spans from this harness) show up as known services. Exit code is
non-zero on any failure with a clear message about which step broke.

If it passes, open <http://localhost:16686> and search for traces —
you should see `bench.iteration` client spans with postgres-side
children sharing the same `trace_id`.

## Use it

From the host, build the bench and point at the toxiproxy DSN:

```bash
cd ..                           # back to repo root
go build -tags=patched_pgx -o otelbench ./cmd/otelbench
./otelbench bench \
    --dsn 'postgres://postgres@127.0.0.1:5645/postgres?sslmode=disable' \
    --otlp-endpoint localhost:4317 \
    --modes 0,1a,1b,2a,2b,3,4 \
    --latency crossregion \
    --iterations 300 \
    --report results.json
```

Then open <http://localhost:16686> to view traces in Jaeger. Both
client-side (`bench.iteration` + pgx per-query) and server-side
(contrib/otel statement spans, emitted via the Rust demo extension)
share the same `trace_id` per iteration, so they should appear under a
single trace in the UI.

## Tear it down

```bash
docker compose down            # stop, keep volumes
docker compose down -v         # stop and wipe the postgres data volume
```

## Per-service notes

### postgres

Built from `ringerc/postgres` branch `postgres-otel-tracing`. The
`shared_preload_libraries` list loads (in order): `otel`,
`otel_postgres_tracing`, `postgres_otel_tracing_demo`,
`pg_stat_statements`. `protocol_headers = on` enables the `'M'`
wire message used by Mode 4.

The Rust demo extension is built in a separate stage (`demo-builder`)
against the installed `pg_config` and dropped into
`/opt/pgsql/lib/postgresql/postgres_otel_tracing_demo.so`. It reads
its OTLP endpoint from the standard `OTEL_EXPORTER_OTLP_ENDPOINT`
environment variable, which the compose sets to
`http://otel-collector:4317`.

`init.sh` runs once on first boot: it `initdb`s the cluster, installs
the bench's `postgresql.conf` and `pg_hba.conf`, starts postgres
briefly on a Unix socket, and runs `CREATE EXTENSION` for the three
SQL-visible extensions (`otel`, `otel_postgres_tracing`,
`pg_stat_statements`).

### toxiproxy

Pre-seeded with one proxy named `postgres` (matching the bench's
default `--toxiproxy-proxy=postgres`) that fronts `postgres:5432`. The
bench creates / updates / removes latency toxics via the control-plane
HTTP API on `:8474`.

### otel-collector

`otel/opentelemetry-collector-contrib:0.114.0`. Receives OTLP/gRPC on
`:4317` from both the postgres-side demo extension (server-side spans)
and the host-side bench (client-side spans), batches them, and
forwards to Jaeger over OTLP/gRPC. The `debug` exporter is also wired
so the collector's stdout includes a sampled trace summary; comment it
out in `otel-collector/config.yaml` if it's noisy.

### jaeger

`jaegertracing/all-in-one:1.62.0` with `COLLECTOR_OTLP_ENABLED=true`.
UI on the host at <http://localhost:16686>.

## Building the postgres image from a local source tree

The default Dockerfile clones the upstream postgres branch at build
time. For local-tree iteration, override the build args:

```yaml
# docker-compose.override.yml --- not committed
services:
  postgres:
    build:
      args:
        POSTGRES_REPO: file:///src/postgres
        DEMO_REPO: file:///src/postgres_otel_tracing_demo
      additional_contexts:
        - src=../..   # parent of postgres_otel_tracing_bench
```

Then `docker compose build postgres` reads from your local checkouts
instead of github. (You'll need to bind-mount the source dirs in too;
left as an exercise.)
