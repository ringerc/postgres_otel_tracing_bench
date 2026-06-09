//go:build patched_pgx

package modes

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// mode4 --- 'M' (RequestHeaders) frontend message.
//
// Available only when the binary is built with `-tags=patched_pgx`,
// which swaps in github.com/ringerc/pgx_patches via go.mod's replace
// directive. The patched pgx exposes a method that emits one 'M' frame
// with the W3C traceparent / tracestate fields under the "otel." prefix
// just before the next P/B/E group; on the server, contrib/otel's
// registered handler reads the headers at Q/P/B/E entry via the
// deferred-apply dispatcher.
//
// Wire shape per iteration, cache warm:
//
//	M (otel.traceparent=..., otel.tracestate=...)
//	Bind+Execute (<SQL>)
//	Sync
//
// 4 client->server messages, 1 RTT, statement cache hits.
//
// TODO: implement against the patched pgx API once that fork exists.
type mode4 struct{}

func (mode4) ID() string { return "4" }
func (mode4) Description() string {
	return "'M' RequestHeaders message under 'otel.' prefix  (1 RTT, statement cache hits)"
}
func (mode4) Setup(ctx context.Context, c *pgx.Conn, w workload.Workload) error { return nil }
func (mode4) Teardown(ctx context.Context, c *pgx.Conn) error                    { return nil }
func (mode4) RunOne(ctx context.Context, conn *pgx.Conn, w workload.Workload, tc TraceContext) (int, error) {
	return 0, errors.New("mode 4 not yet implemented")
}
