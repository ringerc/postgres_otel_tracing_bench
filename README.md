# postgres_otel_tracing_bench

Benchmark and demo harness for comparing trace-context propagation methods
against PostgreSQL with [`contrib/otel`](https://github.com/ringerc/postgres/tree/postgres-otel-tracing/contrib/otel).

Status: scaffolding. CLI surface defined, mode skeletons in place, no
mode currently runs end-to-end.

## What it measures

Per-iteration latency, wire-byte counts, and postgres-side
`pg_stat_statements` / `pg_prepared_statements` deltas for six methods
of attaching a W3C trace context to a SQL workload:

| Mode | Wire shape | RTTs | Statement cache |
|------|------------|------|-----------------|
| 1a | `BEGIN; SET LOCAL otel.traceparent=...; <SQL>; COMMIT;` (sequential) | 4 | hits |
| 1b | `SET ...; <SQL>; RESET ...;` (sequential) | 3 | hits |
| 2a | `SET LOCAL ...; <SQL>;` as a multi-statement simple `Q` | 1 | **misses** (simple protocol) |
| 2b | `pgx.Batch` with `BEGIN/SET LOCAL/<SQL>/COMMIT` under one Sync | 1 | hits |
| 3  | sqlcommenter SQL-comment prepend | 1 | **misses every iteration** (SQL text changes) |
| 4  | `M` (RequestHeaders) frontend message | 1 | hits |

Mode 4 requires a patched pgx (see [pgx_patches](#pgx_patches)) and a
patched postgres ([PR #3](https://github.com/ringerc/postgres/pull/3)).

A separate batch suite (B1–B4) runs each propagation method against a
multi-statement workload where one trace context applies to N user
queries.

## What it's not

This isn't a `pgbench` replacement. The workload is intentionally trivial
(one parameterized SELECT or N parameterized SELECTs in a batch) so the
protocol-shape delta isn't drowned by query execution cost. It's also
not a correctness or compatibility test for trace propagation — it
assumes contrib/otel works.

## CLI

```
otelbench bench   --modes 1a,1b,2a,2b,3 --latency crossaz --iterations 10000
otelbench bench   --modes 2b,3,4 --latency crossregion --workload batch --batch-size 10
otelbench demo    sqlcommenter-pool-break --iterations 10000
otelbench check
```

## Requirements

- postgres with `contrib/otel` loaded and `pg_stat_statements` in
  `shared_preload_libraries`; for Mode 4, also with
  [PR #3](https://github.com/ringerc/postgres/pull/3) applied.
- [toxiproxy](https://github.com/Shopify/toxiproxy) in front of postgres
  for latency injection.
- An OTLP collector for client-side spans (Jaeger / Tempo / Grafana
  Agent).

The published demo target ships a `docker-compose.yml` that wires all
three up; see the [Running](#running) section.

## Modes — protocol-level rationale

See [`internal/modes/*.go`](internal/modes/) for the per-mode wire-shape
notes. Highlights:

**Mode 1a** is the "explicit-transaction SET LOCAL" pattern: four
sequential round-trips, statement cache works for the workload SQL. The
fairest baseline for "what you get if you wrap each instrumented query
in an explicit transaction."

**Mode 1b** uses session-level `SET` plus `RESET`, three sequential
round-trips. Models the naive instrumentation pattern that doesn't care
whether the caller is in a transaction. Leaks the session GUC if the
RESET is skipped.

**Mode 2a** packs `SET LOCAL ...; <SQL>;` into one simple `Q` —
postgres treats it as a single implicit transaction. One RTT, but no
parameter binding and no statement cache; every iteration pays a fresh
Parse + plan.

**Mode 2b** uses `pgx.Batch` (`SendBatch`) to coalesce `BEGIN`,
`SET LOCAL`, `<SQL>`, `COMMIT` into one round trip with a single
trailing Sync. Statement cache hits on the workload SQL. Pays the
wrapping-transaction overhead (XACT_COMMIT WAL record, transaction
setup/teardown) on every iteration.

**Mode 3** prepends a sqlcommenter `/*key='val',...*/` comment. One RTT,
but pgx's statement cache keys on SQL text → cache miss every iteration
→ `pg_stat_statements` blows up linearly. `otelbench demo
sqlcommenter-pool-break` is the standalone pathology demo.

**Mode 4** sends an `M` (RequestHeaders) frame just before the next
`P`/`B`/`E`. One RTT, statement cache hits, no wrapping transaction. The
fairest single-RTT comparison against modes 2b and 3.

## Latency injection

[toxiproxy](https://github.com/Shopify/toxiproxy) is the only mechanism
the harness uses. Presets (`--latency`):

| Preset | One-way | Jitter | RTT |
|--------|---------|--------|-----|
| `none` | 0 | 0 | passthrough (measure toxiproxy overhead) |
| `intradc` | 0.5 ms | ±10% | ~1 ms |
| `crossaz` | 2.5 ms | ±10% | ~5 ms |
| `crossregion` | 15 ms | ±10% | ~30 ms |
| `intercontinental` | 50 ms | ±10% | ~100 ms |

`--sweep-latency` runs every preset in one invocation. Toxiproxy's own
overhead is measured once at startup with the `none` preset and reported
alongside results.

## pgx_patches

Mode 4 requires the patched pgx fork at
[`ringerc/pgx_patches`](https://github.com/ringerc/pgx_patches) (branch
`m-protocol-headers`), which adds the `M` frontend message and the
`_pq_.headers=1` startup negotiation. Build with:

```
go build -tags=patched_pgx ./cmd/otelbench
```

The `replace` directive in `go.mod` swaps in the local checkout when
the tag is set; without the tag the build uses stock `jackc/pgx/v5`
and mode 4 is unregistered.

## Running

Local devcontainer (Go 1.23+ required; the workspace devcontainer ships
it via the `devcontainers/features/go` feature):

```
cd postgres_otel_tracing_bench
go build ./cmd/otelbench
./otelbench check
./otelbench bench --modes 1a --iterations 1000 --latency intradc
```

Docker compose (see `deploy/docker-compose.yml`):

```
docker compose -f deploy/docker-compose.yml up -d
docker compose -f deploy/docker-compose.yml run otelbench bench \
    --modes all --sweep-latency --iterations 5000
```

## License

PostgreSQL License — see [LICENSE](LICENSE).
