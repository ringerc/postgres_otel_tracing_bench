package modes

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// mode1b --- "SET otel.traceparent='...'; <SQL>; RESET otel.traceparent;"
// issued as three sequential round-trips, no pipelining.
//
// SET (without LOCAL) sets the session GUC and persists past the
// statement. We RESET after the workload statement to undo the leak.
// This is the "naive session-GUC instrumentation" pattern --- common in
// libraries that don't know whether the caller is using explicit txns.
//
// Wire shape per iteration (3 RTT): SET, <SQL>, RESET.
//
// TODO: implement.
type mode1b struct{}

func (mode1b) ID() string { return "1b" }
func (mode1b) Description() string {
	return "SET otel.traceparent=...; <SQL>; RESET otel.traceparent;  (sequential, 3 RTT, GUC-leak guard)"
}
func (mode1b) Setup(ctx context.Context, c *pgx.Conn, w workload.Workload) error    { return nil }
func (mode1b) Teardown(ctx context.Context, c *pgx.Conn) error                       { return nil }
func (mode1b) RunOne(ctx context.Context, conn *pgx.Conn, w workload.Workload, tc TraceContext) (int, error) {
	return 0, errors.New("mode 1b not yet implemented")
}
