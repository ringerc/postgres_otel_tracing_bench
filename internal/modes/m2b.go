package modes

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// mode2b --- pgx.Batch pipelining BEGIN, SET LOCAL, <workload stmts>,
// COMMIT under one trailing Sync.
//
// pgx v5's SendBatch issues all queued queries followed by exactly one
// Sync, so the whole sequence is a single round trip. COMMIT is required:
// in extended protocol, the Sync boundary is a transaction-protocol
// block, NOT a database-transaction block --- without explicit
// BEGIN/COMMIT each batch entry is its own implicit transaction and
// SET LOCAL would not carry to the workload statements.
//
// Wire shape per iteration (cache warm, N workload statements):
//
//	Bind+Execute (BEGIN)
//	Bind+Execute (SET LOCAL otel.traceparent='...')   <-- traceparent
//	                                                       inlined, never
//	                                                       cache-hits
//	Bind+Execute (<workload stmt 1>)                  <-- caches & hits
//	...
//	Bind+Execute (<workload stmt N>)                  <-- caches & hits
//	Bind+Execute (COMMIT)
//	Sync
//
// 2 + 2(N+2) + 1 client messages, 1 RTT.
type mode2b struct{}

func (mode2b) ID() string { return "2b" }
func (mode2b) Description() string {
	return "pgx.Batch [BEGIN; SET LOCAL ...; <SQL>; COMMIT] under one Sync  (1 RTT, statement cache hits on workload SQL)"
}
func (mode2b) Setup(ctx context.Context, c *pgx.Conn, w workload.Workload) error { return nil }
func (mode2b) Teardown(ctx context.Context, c *pgx.Conn) error                    { return nil }

func (mode2b) RunOne(ctx context.Context, conn *pgx.Conn, w workload.Workload, tc TraceContext) (int, error) {
	stmts := w.NextStatements()

	b := &pgx.Batch{}
	b.Queue("BEGIN")
	b.Queue("SET LOCAL otel.traceparent = '" + tc.Traceparent() + "'")
	for _, s := range stmts {
		b.Queue(s.SQL, s.Args...)
	}
	b.Queue("COMMIT")

	br := conn.SendBatch(ctx, b)
	defer br.Close()

	// Drain results in queue order. BEGIN / SET LOCAL / COMMIT use Exec
	// (no rows); the workload statements use Query so we consume rows.
	if _, err := br.Exec(); err != nil {
		return 0, fmt.Errorf("BEGIN: %w", err)
	}
	if _, err := br.Exec(); err != nil {
		return 0, fmt.Errorf("SET LOCAL: %w", err)
	}
	total := 0
	for i := range stmts {
		rows, err := br.Query()
		if err != nil {
			return total, fmt.Errorf("workload stmt %d: %w", i, err)
		}
		for rows.Next() {
			if _, err := rows.Values(); err != nil {
				rows.Close()
				return total, err
			}
			total++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return total, err
		}
		rows.Close()
	}
	if _, err := br.Exec(); err != nil {
		return total, fmt.Errorf("COMMIT: %w", err)
	}
	return total, nil
}
