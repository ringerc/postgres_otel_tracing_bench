// Package pgstat captures the postgres-side metrics that distinguish
// statement-cache-friendly modes from cache-hostile ones.
//
// We snapshot before and after each cell:
//
//	pg_stat_statements             --- unique-query count and total calls
//	pg_prepared_statements         --- per-backend prepared statement
//	                                   count (1 connection per goroutine)
//
// The interesting deltas:
//
//	mode 3 (sqlcommenter)  --- pg_stat_statements grows linearly with
//	                           iterations because every query has a
//	                           unique trace_id in the comment; pgx's
//	                           statement cache also churns.
//	mode 1a/1b/2b/4         --- pg_stat_statements stays at the
//	                           workload's distinct-query count.
//	mode 2a (simple Q)     --- pg_stat_statements grows because the SQL
//	                           text changes every iter (interpolated
//	                           traceparent in the SQL).
package pgstat

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Snapshot captures the postgres-side counters we care about.
type Snapshot struct {
	StatStatementsRows       int   // distinct rows in pg_stat_statements
	StatStatementsTotalCalls int64 // sum of calls column across all rows
	PreparedStatementsRows   int   // rows in pg_prepared_statements (this backend)

	// HasStatStatements is false when the pg_stat_statements extension
	// is not installed --- in that case the StatStatements* fields are
	// zero and callers should not interpret deltas. The benchmark
	// still runs; we just can't report this dimension.
	HasStatStatements bool
}

// Take captures the current snapshot from the given connection.
//
// pg_prepared_statements is per-session (only this backend's prepared
// statements), which is what we want: it tracks how badly pgx's
// statement cache is churning for this connection.
//
// pg_stat_statements is cluster-wide and reflects every connection.
// For a benchmark where the harness is the only client, this is fine;
// in a noisy environment the delta still captures only the queries
// the harness ran.
func Take(ctx context.Context, conn *pgx.Conn) (Snapshot, error) {
	var s Snapshot

	// pg_stat_statements may not be installed; probe gracefully.
	var hasExt bool
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')`,
	).Scan(&hasExt); err != nil {
		return s, fmt.Errorf("probe pg_stat_statements extension: %w", err)
	}
	s.HasStatStatements = hasExt
	if hasExt {
		// COALESCE protects against an empty pg_stat_statements (no
		// queries recorded yet) returning NULL from sum().
		if err := conn.QueryRow(ctx,
			`SELECT count(*), COALESCE(sum(calls), 0) FROM pg_stat_statements`,
		).Scan(&s.StatStatementsRows, &s.StatStatementsTotalCalls); err != nil {
			return s, fmt.Errorf("read pg_stat_statements: %w", err)
		}
	}

	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM pg_prepared_statements`,
	).Scan(&s.PreparedStatementsRows); err != nil {
		return s, fmt.Errorf("read pg_prepared_statements: %w", err)
	}
	return s, nil
}

// Reset asks postgres to truncate pg_stat_statements so a subsequent
// Take starts from a clean slate. No-op (with a soft warning return) if
// the extension isn't installed.
func Reset(ctx context.Context, conn *pgx.Conn) error {
	var hasExt bool
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')`,
	).Scan(&hasExt); err != nil {
		return fmt.Errorf("probe pg_stat_statements extension: %w", err)
	}
	if !hasExt {
		return errors.New("pg_stat_statements not installed")
	}
	if _, err := conn.Exec(ctx, `SELECT pg_stat_statements_reset()`); err != nil {
		return fmt.Errorf("pg_stat_statements_reset(): %w", err)
	}
	return nil
}

// Delta returns after - before.
func Delta(before, after Snapshot) Snapshot {
	return Snapshot{
		StatStatementsRows:       after.StatStatementsRows - before.StatStatementsRows,
		StatStatementsTotalCalls: after.StatStatementsTotalCalls - before.StatStatementsTotalCalls,
		PreparedStatementsRows:   after.PreparedStatementsRows - before.PreparedStatementsRows,
		HasStatStatements:        before.HasStatStatements && after.HasStatStatements,
	}
}
