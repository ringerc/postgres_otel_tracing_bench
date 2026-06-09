package modes

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// mode3 --- sqlcommenter: prepend a "/*key='val',...*/" comment carrying
// the W3C traceparent / tracestate to each workload SQL string.
//
// Wire shape per iteration: P + B + E + Sync per workload statement,
// 1 RTT per statement (no wrapping txn, no SET LOCAL).
//
// The catch --- and the reason this mode exists as the "anti-pattern"
// baseline --- is that pgx's automatic statement cache keys on the full
// SQL text. Sqlcommenter mutates the SQL text with the per-query
// trace_id, so the cache misses every iteration: every query pays a
// fresh Parse, and the server's pg_stat_statements blows up linearly.
// See `otelbench demo sqlcommenter-pool-break` for the pathology demo.
//
// Format we emit (per the sqlcommenter spec): leading "/* key='val',
// key='val' */" with single-quoted, URL-encoded values, sorted by key.
// Since our values are trace_id / span_id / trace_flags --- all
// fixed-shape hex strings --- URL encoding is a no-op and we skip it.
type mode3 struct{}

func (mode3) ID() string { return "3" }
func (mode3) Description() string {
	return "sqlcommenter: /*traceparent='...'*/ <SQL>  (1 RTT, breaks pgx statement cache)"
}
func (mode3) Setup(ctx context.Context, c *pgx.Conn, w workload.Workload) error { return nil }
func (mode3) Teardown(ctx context.Context, c *pgx.Conn) error                    { return nil }

func (mode3) RunOne(ctx context.Context, conn *pgx.Conn, w workload.Workload, tc TraceContext) (int, error) {
	stmts := w.NextStatements()
	tagged := make([]workload.Statement, len(stmts))
	for i, s := range stmts {
		tagged[i] = workload.Statement{
			SQL:  sqlcommenterPrefix(tc) + s.SQL,
			Args: s.Args,
		}
	}
	return workload.ExecuteSequential(ctx, conn, tagged)
}

// sqlcommenterPrefix returns the "/*key='val',...*/ " comment for tc.
// One allocation per call; the resulting string changes every iteration
// because trace_id and span_id are fresh per iter --- that's the whole
// point of the pathology this mode demonstrates.
func sqlcommenterPrefix(tc TraceContext) string {
	var sb strings.Builder
	sb.Grow(80)
	sb.WriteString("/*traceparent='")
	sb.WriteString(tc.Traceparent())
	sb.WriteString("'*/ ")
	return sb.String()
}
