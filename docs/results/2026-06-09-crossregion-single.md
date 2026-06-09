# Benchmark result — trace-context propagation, single SELECT, crossregion

> [!NOTE]
> **Captured before the `otel` → `otel_api` extension rename.**
> SQL snippets, GUC names, and the postgres branch references in
> this doc are from before that work (`postgres_otel_api@4ca911d` +
> consumers, 2026-06-10). The propagation logic, wire shapes, RTT
> counts, and timing measurements are unchanged by the rename —
> `SET LOCAL otel.traceparent` simply became
> `SET LOCAL otel_api.traceparent`; the underlying server behaviour,
> pgx statement-cache interaction, and `'M'`-frame wire format are
> all identical. So the numbers stand; only the cut-and-paste SQL
> in the [Reproducing](#reproducing) section needs updating if you
> re-run today.

**Date:** 2026-06-09
**Run by:** `otelbench bench` (commit `c05202b`)
**pgx:** `ringerc/pgx_patches` branch `m-protocol-headers` (atop `v5.10.0`)
**postgres:** 19beta1 from `ringerc/postgres` branch `postgres-otel-tracing` (commit `0269dea8230`), built with `contrib/otel`, `contrib/otel_postgres_tracing`, `pg_stat_statements` loaded
**Latency injection:** [toxiproxy](https://github.com/Shopify/toxiproxy) v2.12.0,
preset `crossregion` (15 ms one-way, ±1 ms jitter, each leg → ~30 ms RTT)
**Workload:** `single` — one parameterized `SELECT id, payload FROM bench_single WHERE id = $1` per iteration, 1024-row seed table
**Iterations:** 200 measured + 30 warmup per mode, single goroutine

## Headline

| Propagation method | p50 | RTTs | pgx prepared stmts created |
|---|---|---|---|
| Sequential `BEGIN`/`SET LOCAL`/`SQL`/`COMMIT` | 120.7 ms | 4 | +1 |
| Sequential `SET`/`SQL`/`RESET` (session GUC) | 91.1 ms | 3 | +1 |
| Multi-statement simple `Q` (`SET LOCAL`; SQL) | 30.5 ms | 1 | +0 *(simple protocol — no cache)* |
| `pgx.Batch` pipelining BEGIN/SET LOCAL/SQL/COMMIT | 60.7 ms | 2 | +233 *(SET LOCAL text varies → cache thrash)* |
| `sqlcommenter` SQL-comment prepend | 60.7 ms | 2 | +230 *(workload SQL text varies → cache thrash)* |
| **`M` (RequestHeaders) frontend message** | **30.1 ms** | **1** | **+1** |

The `M`-message row is the only one that achieves **all three** properties simultaneously: single round trip, statement-cache friendly, and no wrapping transaction. Multi-statement simple `Q` matches the RTT only by giving up the statement cache entirely. Both `pgx.Batch`-with-`SET LOCAL` and `sqlcommenter` claim one RTT in theory but pay a hidden second round trip because the varying value forces a fresh `Parse` every iteration, and both stuff pgx's per-connection prepared-statement LRU with one-shot entries.

## Detail

Numbers as reported by `otelbench bench --modes 1a,1b,2a,2b,3,4 --latency crossregion --iterations 200 --warmup 30 --workload single`:

| Mode ID | Propagation | p50 | p95 | p99 | max | pg_stat_stmts rows | pg_stat_stmts calls | pg_prepared_+ |
|--|--|--:|--:|--:|--:|--:|--:|--:|
| 1a | BEGIN/SET LOCAL/SQL/COMMIT sequential | 120.7 ms | 124.1 ms | 124.8 ms | 126.5 ms | 6 | 923 | +1 |
| 1b | SET/SQL/RESET sequential | 91.2 ms | 94.1 ms | 94.9 ms | 96.0 ms | 2 | 693 | +1 |
| 2a | Multi-stmt simple `Q` | 30.5 ms | 31.9 ms | 32.2 ms | 32.5 ms | 1 | 463 | 0 |
| 2b | `pgx.Batch` (BEGIN/SET LOCAL/SQL/COMMIT) | 60.5 ms | 62.5 ms | 63.2 ms | 64.2 ms | 0 | 465 | +233 |
| 3  | sqlcommenter `/*traceparent='...'*/ SQL` | 60.9 ms | 62.7 ms | 63.3 ms | 63.5 ms | 0 | 233 | +230 |
| 4  | `M` (RequestHeaders) frame | 30.4 ms | 31.6 ms | 31.8 ms | 32.0 ms | 0 | 233 | +1 |

Latencies are reported in nanoseconds in the JSON report (`bench-all6.json`); converted to milliseconds here. `pg_stat_statements rows` is the delta in distinct rows over the run; `pg_prepared_statements +` is the delta in prepared statements on the single benchmark connection.

## Reading the table

- **Why mode 2a's `pg_stat_statements_rows` delta is exactly 1**: postgres normalizes literal values in the parser before computing the queryid, so `SET LOCAL otel.traceparent = 'a...'; SELECT … id = 1;` and `SET LOCAL otel.traceparent = 'b...'; SELECT … id = 2;` collapse to one queryid. The cost is still real — the server pays `Parse` + planner every iteration — but it doesn't show up as distinct rows in `pg_stat_statements`.
- **Why modes 2b and 3 hit +233 / +230 prepared statements** (not the LRU cap of 512): the run was 200 iterations + 30 warmup = 230 unique trace_ids. Each becomes a unique `Parse` against a unique cache key in pgx's LRU, so growth is bounded by iteration count, not LRU size. A longer run would top out at the cap.
- **Why mode 4 hits +1 prepared statement**: the M frame's payload is wire-only — it never appears in any SQL text, so pgx's automatic prepared-statement cache sees the same workload SQL every iteration and the LRU stays at one entry.

## Reproducing

```bash
# patched postgres, contrib/otel + pg_stat_statements loaded, listening on 127.0.0.1:5544
# toxiproxy running on 8474, with proxy "postgres" → 127.0.0.1:5544 listening at 127.0.0.1:5645

cd postgres_otel_tracing_bench
go build -tags=patched_pgx -o otelbench ./cmd/otelbench

./otelbench bench \
    --dsn="postgres://postgres@127.0.0.1:5645/postgres?sslmode=disable" \
    --modes 1a,1b,2a,2b,3,4 \
    --latency crossregion \
    --iterations 200 \
    --warmup 30 \
    --workload single \
    --report results.json
```

The same matrix at other latencies is one flag away (`--latency intradc|crossaz|crossregion|intercontinental` or `--sweep-latency` to run every preset in one invocation).
