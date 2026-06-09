package modes

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// mode0 --- the untraced baseline. No trace-context propagation at all:
// just the workload's statements issued via pgx's normal extended
// protocol path with the automatic statement cache active.
//
// Wire shape per iteration (cache warm):
//
//	Bind+Execute+Sync (cached SQL, $1=id)
//	-> BindComplete, rows, CommandComplete, ReadyForQuery
//
// 1 RTT. No SET LOCAL, no transaction, no sqlcommenter, no 'M' frame.
// This is what the other modes' overhead is measured against.
//
// Per-iteration spans still fire on the client (the runner wraps every
// iteration in a `bench.iteration` span and otelpgx fires per-query
// spans) so client-side instrumentation cost is comparable with the
// other modes; what mode 0 omits is anything that gets the trace_id
// onto the server.
type mode0 struct{}

func (mode0) ID() string { return "0" }
func (mode0) Description() string {
	return "Untraced baseline --- workload SQL only, no trace-context propagation (1 RTT, cache hits)"
}

func (mode0) Setup(ctx context.Context, c *pgx.Conn, w workload.Workload) error { return nil }
func (mode0) Teardown(ctx context.Context, c *pgx.Conn) error                    { return nil }

func (mode0) RunOne(ctx context.Context, conn *pgx.Conn, w workload.Workload, tc TraceContext) (int, error) {
	_ = tc // by definition, mode 0 doesn't propagate trace context
	return workload.ExecuteSequential(ctx, conn, w.NextStatements())
}
