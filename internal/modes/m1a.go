package modes

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// mode1a --- "BEGIN; SET LOCAL otel.traceparent='...'; <SQL>; COMMIT;"
// issued as four sequential round-trips, no pipelining.
//
// Wire shape per iteration (cache warm):
//
//	C: BEGIN          ->  RFQ   (1 RTT)
//	C: SET LOCAL ...  ->  RFQ   (1 RTT, NOT cache-friendly: traceparent
//	                             literal is part of the SQL text)
//	C: <SQL>          ->  rows  (1 RTT, cache hits after first iter)
//	C: COMMIT         ->  RFQ   (1 RTT)
//
// SET LOCAL semantics: scoped to the explicit transaction; visible to
// the SQL statements that follow in the same transaction. Each batch
// statement in a "batch" workload pays one round trip in this mode.
//
// SET is a utility command --- postgres rejects parameter binding for
// its value --- so the traceparent is interpolated into the SQL text.
// Traceparent format ('NN-<hex>-<hex>-<hex>') contains no quote chars,
// so literal-single-quoting is safe.
type mode1a struct{}

func (mode1a) ID() string { return "1a" }
func (mode1a) Description() string {
	return "BEGIN; SET LOCAL otel.traceparent=...; <SQL>; COMMIT;  (sequential, 3+N RTT for N-statement iteration)"
}

func (mode1a) Setup(ctx context.Context, conn *pgx.Conn, w workload.Workload) error {
	return nil
}

func (mode1a) RunOne(ctx context.Context, conn *pgx.Conn, w workload.Workload, tc TraceContext) (int, error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("BEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op if Commit succeeded

	if _, err := tx.Exec(ctx,
		"SET LOCAL otel.traceparent = '"+tc.Traceparent()+"'"); err != nil {
		return 0, fmt.Errorf("SET LOCAL: %w", err)
	}

	rows, err := workload.ExecuteSequential(ctx, tx, w.NextStatements())
	if err != nil {
		return rows, fmt.Errorf("workload: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return rows, fmt.Errorf("COMMIT: %w", err)
	}
	return rows, nil
}

func (mode1a) Teardown(ctx context.Context, conn *pgx.Conn) error { return nil }
