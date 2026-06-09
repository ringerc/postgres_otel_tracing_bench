//go:build patched_pgx

package modes

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgproto3"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// mode4 --- 'M' (RequestHeaders) frontend message.
//
// Available only when the binary is built with `-tags=patched_pgx`,
// which exposes pgproto3.RequestHeaders via the go.mod replace pointing
// at github.com/ringerc/pgx_patches.
//
// Per iteration we:
//
//  1. Queue an M frame onto pgx's Frontend buffer carrying the per-
//     iteration W3C trace context as ("otel.traceparent", "otel.tracestate")
//     entries.
//  2. Issue the workload statements via the normal pgx Query path.
//     The Frontend's wbuf already holds the M, so when pgx flushes its
//     P/B/E/Sync for the workload statement they go out in the same
//     write call --- one round trip.
//
// On the server, the M is parsed and stashed in PendingHeadersContext;
// at the next Q/P/B/E boundary (the workload Bind/Execute we just
// queued), ApplyPendingRequestHeaders dispatches the entries to
// contrib/otel's "otel." prefix handler. The workload SQL itself is
// parameterized and constant text → hits pgx's statement cache → no
// fresh Parse on the wire. Result: 4 client→server messages steady-
// state (M, Bind, Execute, Sync), 1 RTT, no wrapping transaction.
//
// Negotiation: the connection must have sent _pq_.headers=1 in its
// StartupMessage parameters. The Mode's Setup verifies the server
// echoed back a matching protocol_features ParameterStatus so we don't
// silently send M frames against an unpatched server (which would
// kill the connection with a protocol-violation FATAL).
type mode4 struct{}

func (mode4) ID() string { return "4" }
func (mode4) Description() string {
	return "'M' RequestHeaders message under 'otel.' prefix  (1 RTT, statement cache hits)"
}

func (mode4) Setup(ctx context.Context, conn *pgx.Conn, w workload.Workload) error {
	// Probe the protocol_features ParameterStatus the patched server
	// emits during startup. pgx exposes startup-time parameter status
	// via conn.PgConn().ParameterStatus(name).
	pfs := conn.PgConn().ParameterStatus("protocol_features")
	if pfs == "" {
		return fmt.Errorf("mode 4 requires a server that negotiated _pq_.headers=1 " +
			"and reports protocol_features; connect with " +
			"RuntimeParams[\"_pq_.headers\"] = \"1\" against a patched postgres")
	}
	return nil
}

func (mode4) Teardown(ctx context.Context, conn *pgx.Conn) error { return nil }

func (mode4) RunOne(ctx context.Context, conn *pgx.Conn, w workload.Workload, tc TraceContext) (int, error) {
	// Queue the M frame onto the same Frontend buffer pgx's Query
	// path will flush. Order matters: M must come before the next
	// P/B/E group, which is what happens because the next call
	// (conn.Query) appends its own messages after the M.
	conn.PgConn().Frontend().Send(&pgproto3.RequestHeaders{
		Headers: []pgproto3.Header{
			{Key: "otel.traceparent", Value: tc.Traceparent()},
		},
	})
	// No flush here --- conn.Query will flush.
	return workload.ExecuteSequential(ctx, conn, w.NextStatements())
}
