package modes

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// mode1b --- "SET otel.traceparent='...'; <SQL>; RESET otel.traceparent;"
// issued as three sequential round-trips, no pipelining, no wrapping
// transaction. SET (without LOCAL) sets the session GUC; we RESET after
// the workload to undo the leak.
//
// Modeled on the "naive session-GUC instrumentation" pattern common in
// libraries that don't know whether the caller is using explicit txns.
// If RESET is skipped (e.g. exception thrown between SET and RESET),
// the next query on the same connection picks up the stale trace
// context --- this mode shows the cost of doing the RESET correctly.
type mode1b struct{}

func (mode1b) ID() string { return "1b" }
func (mode1b) Description() string {
	return "SET otel.traceparent=...; <SQL>; RESET otel.traceparent;  (sequential, 2+N RTT)"
}

func (mode1b) Setup(ctx context.Context, c *pgx.Conn, w workload.Workload) error { return nil }
func (mode1b) Teardown(ctx context.Context, c *pgx.Conn) error                    { return nil }

func (mode1b) RunOne(ctx context.Context, conn *pgx.Conn, w workload.Workload, tc TraceContext) (int, error) {
	if _, err := conn.Exec(ctx,
		"SET otel.traceparent = '"+tc.Traceparent()+"'"); err != nil {
		return 0, fmt.Errorf("SET: %w", err)
	}
	rows, err := workload.ExecuteSequential(ctx, conn, w.NextStatements())
	if err != nil {
		// Even on failure we issue the RESET to avoid leaking the GUC
		// to the next iteration on the same connection. Best-effort.
		_, _ = conn.Exec(ctx, "RESET otel.traceparent")
		return rows, fmt.Errorf("workload: %w", err)
	}
	if _, err := conn.Exec(ctx, "RESET otel.traceparent"); err != nil {
		return rows, fmt.Errorf("RESET: %w", err)
	}
	return rows, nil
}
