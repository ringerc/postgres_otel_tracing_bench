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
// Wire shape per iteration (extended protocol, cache warm):
//
//	C: Query "BEGIN"             ->  S: CommandComplete, ReadyForQuery   (1 RTT)
//	C: Bind (SET LOCAL), Execute, Sync  ->  S: ..., ReadyForQuery        (1 RTT)
//	C: Bind (<SQL>),     Execute, Sync  ->  S: rows, ReadyForQuery       (1 RTT)
//	C: Query "COMMIT"            ->  S: CommandComplete, ReadyForQuery   (1 RTT)
//
// Cache: the SQL and SET LOCAL prepared statements hit on the second
// iteration onward; BEGIN/COMMIT go via simple Query (pgx's default for
// internal transaction-control statements).
//
// SET LOCAL semantics: scoped to the explicit transaction, so the
// traceparent set by `SET LOCAL` is visible to the SQL statement that
// follows in the same transaction.
type mode1a struct{}

func (mode1a) ID() string { return "1a" }
func (mode1a) Description() string {
	return "BEGIN; SET LOCAL otel.traceparent=...; <SQL>; COMMIT;  (sequential, 4 RTT)"
}

func (mode1a) Setup(ctx context.Context, conn *pgx.Conn, w workload.Workload) error {
	// TODO: pre-prepare workload statements so first iteration also hits
	// cache; for now rely on pgx's automatic cache to populate after
	// the warmup pass.
	_ = ctx
	_ = conn
	_ = w
	return nil
}

func (mode1a) RunOne(ctx context.Context, conn *pgx.Conn, w workload.Workload, tc TraceContext) (int, error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("BEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op if commit succeeded

	if _, err := tx.Exec(ctx,
		"SET LOCAL otel.traceparent = $1", tc.Traceparent()); err != nil {
		return 0, fmt.Errorf("SET LOCAL: %w", err)
	}

	rows, err := w.RunSingle(ctx, tx)
	if err != nil {
		return rows, fmt.Errorf("workload: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return rows, fmt.Errorf("COMMIT: %w", err)
	}
	return rows, nil
}

func (mode1a) Teardown(ctx context.Context, conn *pgx.Conn) error {
	_ = ctx
	_ = conn
	return nil
}
