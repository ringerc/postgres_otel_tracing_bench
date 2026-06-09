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

	"github.com/jackc/pgx/v5"
)

// Snapshot captures the postgres-side counters we care about.
type Snapshot struct {
	StatStatementsRows       int
	StatStatementsTotalCalls int64
	PreparedStatementsRows   int
}

// Take captures the current snapshot. Requires the pg_stat_statements
// extension to be installed (CREATE EXTENSION pg_stat_statements);
// returns a clear error if it isn't, so misconfiguration is loud.
//
// TODO: implement.
func Take(ctx context.Context, conn *pgx.Conn) (Snapshot, error) {
	_ = ctx
	_ = conn
	return Snapshot{}, errors.New("pgstat.Take not yet implemented")
}

// Reset asks postgres to truncate pg_stat_statements so the next Take
// after a Reset returns counters scoped only to the upcoming run.
//
// TODO: SELECT pg_stat_statements_reset();
func Reset(ctx context.Context, conn *pgx.Conn) error {
	_ = ctx
	_ = conn
	return errors.New("pgstat.Reset not yet implemented")
}
