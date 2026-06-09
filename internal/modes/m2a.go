package modes

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// mode2a --- multi-statement simple Query:
//
//	"SET LOCAL otel.traceparent='...'; <SQL>;"
//
// sent as a single 'Q' message. Postgres treats a multi-statement simple Q
// as one implicit transaction, so SET LOCAL works.
//
// Cost: forces simple protocol. No parameter binding (we client-side
// interpolate every literal, including the traceparent), no statement
// cache. Server pays Parse + plan on every iteration.
//
// Wire shape: 1 client message, 1 RTT.
//
// TODO: implement (use conn.PgConn().Exec / sendQuery).
type mode2a struct{}

func (mode2a) ID() string { return "2a" }
func (mode2a) Description() string {
	return "Multi-statement simple Q: SET LOCAL ...; <SQL>;  (1 RTT, no statement cache)"
}
func (mode2a) Setup(ctx context.Context, c *pgx.Conn, w workload.Workload) error { return nil }
func (mode2a) Teardown(ctx context.Context, c *pgx.Conn) error                    { return nil }
func (mode2a) RunOne(ctx context.Context, conn *pgx.Conn, w workload.Workload, tc TraceContext) (int, error) {
	return 0, errors.New("mode 2a not yet implemented")
}
