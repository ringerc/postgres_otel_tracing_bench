// Package workload defines what SQL the benchmark actually runs.
//
// Two workloads:
//
//	single --- one parameterized SELECT, one row out. The headline shape
//	  for measuring per-query trace-propagation overhead.
//
//	batch  --- N parameterized SELECTs sharing a single trace context,
//	  modeling an ORM hydrating a parent + child set. Exercises the
//	  B1/B2/B3/B4 batch suite.
//
// Workloads own their schema setup/teardown. They expose the per-
// iteration SQL+args via NextStatements; modes are responsible for
// applying their propagation strategy around the returned statements
// (e.g. mode 2b wraps them in a pgx.Batch with BEGIN/SET LOCAL/COMMIT).
package workload

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Statement is one SQL string with its parameter args. Workloads return
// []Statement to describe one iteration's worth of queries; modes
// translate that into wire-protocol-specific sequences.
type Statement struct {
	SQL  string
	Args []any
}

// Workload is the SUT (system-under-test) the modes wrap their
// propagation around.
type Workload interface {
	// ID is the short identifier ("single" or "batch") used on the CLI.
	ID() string

	// Setup runs schema creation + seed data load. Idempotent.
	Setup(ctx context.Context, conn *pgx.Conn) error

	// Teardown drops or leaves whatever Setup created.
	Teardown(ctx context.Context, conn *pgx.Conn) error

	// NextStatements returns the statements to issue for one iteration.
	// The implementation advances any internal state (id counter, etc.)
	// so successive calls produce different parameter values.
	// Length is 1 for the "single" workload, N for "batch".
	NextStatements() []Statement
}

// ExecuteSequential is the helper modes 1a/1b/3 share: it issues each
// Statement in stmts via the given Querier (a *pgx.Conn or pgx.Tx),
// drains any rows, and returns the total rows seen. Errors short-
// circuit; callers wrap with their own context.
//
// Modes that pipeline (2a / 2b / 4) do NOT use this helper --- they
// queue statements onto a pgx.Batch / pgconn.Exec themselves so the
// wire shape they're measuring is actually what goes out.
func ExecuteSequential(ctx context.Context, q Querier, stmts []Statement) (int, error) {
	total := 0
	for _, s := range stmts {
		rows, err := q.Query(ctx, s.SQL, s.Args...)
		if err != nil {
			return total, err
		}
		for rows.Next() {
			// Scan into discarded vars so pgx actually consumes the
			// row payload off the wire. Using a single byte slice as
			// the scan target works for any column count via
			// (*pgx.Conn).Query's row.Values()? Simpler: just call
			// rows.Values() which doesn't require knowing the schema.
			if _, err := rows.Values(); err != nil {
				rows.Close()
				return total, err
			}
			total++
		}
		if err := rows.Err(); err != nil {
			return total, err
		}
		rows.Close()
	}
	return total, nil
}

// Querier is the subset of pgx.Conn / pgx.Tx we need.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}
