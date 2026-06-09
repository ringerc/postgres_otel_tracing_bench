package modes

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// mode2b --- pgx.Batch pipelining BEGIN, SET LOCAL, <SQL>, COMMIT.
//
// pgx v5's SendBatch issues all queued queries followed by ONE trailing
// Sync, so the whole sequence is a single round trip. The COMMIT is
// required: in extended protocol, the Sync boundary is a transaction-
// protocol block, NOT a database-transaction block --- without explicit
// BEGIN/COMMIT each batch entry is its own implicit transaction and
// SET LOCAL would not carry to <SQL>.
//
// Wire shape per iteration, cache warm:
//
//	Bind+Execute (BEGIN)
//	Bind+Execute (SET LOCAL otel.traceparent=$1)
//	Bind+Execute (<SQL>)
//	Bind+Execute (COMMIT)
//	Sync
//
// 9 client->server messages, 1 RTT.
//
// TODO: implement.
type mode2b struct{}

func (mode2b) ID() string { return "2b" }
func (mode2b) Description() string {
	return "pgx.Batch [BEGIN; SET LOCAL ...; <SQL>; COMMIT] under one Sync  (1 RTT, statement cache hits on SQL)"
}
func (mode2b) Setup(ctx context.Context, c *pgx.Conn, w workload.Workload) error { return nil }
func (mode2b) Teardown(ctx context.Context, c *pgx.Conn) error                    { return nil }
func (mode2b) RunOne(ctx context.Context, conn *pgx.Conn, w workload.Workload, tc TraceContext) (int, error) {
	return 0, errors.New("mode 2b not yet implemented")
}
