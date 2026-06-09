package modes

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// mode3 --- sqlcommenter: prepend a "/*key='val',...*/" comment carrying
// W3C traceparent / tracestate to the SQL text. Issued as a normal
// parameterized query.
//
// Wire shape per iteration: Parse + Bind + Execute + Sync, 1 RTT.
//
// The catch --- and the reason this mode exists in the bench as the
// "anti-pattern" baseline --- is that pgx's automatic statement cache
// keys on the full SQL text. Sqlcommenter mutates the SQL text with the
// per-query trace_id, so the cache misses on every iteration: every
// query pays a fresh Parse, and the server's pg_stat_statements blows
// up linearly. See `otelbench demo sqlcommenter-pool-break` for the
// pathology demo.
//
// TODO: implement using either observIQ/sqlcommenter via the
// database/sql shim, or hand-rolled via pgx QueryRewriter for a
// minimal-dependency single-allocation prepend.
type mode3 struct{}

func (mode3) ID() string { return "3" }
func (mode3) Description() string {
	return "sqlcommenter: /*traceparent='...'*/ <SQL>  (1 RTT, breaks pgx statement cache)"
}
func (mode3) Setup(ctx context.Context, c *pgx.Conn, w workload.Workload) error { return nil }
func (mode3) Teardown(ctx context.Context, c *pgx.Conn) error                    { return nil }
func (mode3) RunOne(ctx context.Context, conn *pgx.Conn, w workload.Workload, tc TraceContext) (int, error) {
	return 0, errors.New("mode 3 not yet implemented")
}
