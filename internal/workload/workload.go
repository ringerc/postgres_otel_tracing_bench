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
// Workloads own their schema setup/teardown; modes are workload-agnostic.
package workload

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Workload is the SUT (system-under-test) the modes wrap their
// propagation around.
type Workload interface {
	// ID is the short identifier ("single" or "batch") used on the CLI.
	ID() string

	// Setup runs schema creation + seed data load. Idempotent.
	Setup(ctx context.Context, conn *pgx.Conn) error

	// Teardown drops any objects Setup created. May be a no-op if the
	// caller wants to inspect state post-run.
	Teardown(ctx context.Context, conn *pgx.Conn) error

	// RunSingle issues one user-visible query (or one batch's worth of
	// queries) against the given transaction or connection. The mode's
	// RunOne wraps this with the trace-propagation prologue/epilogue.
	//
	// The querier abstracts over *pgx.Conn and pgx.Tx --- both implement
	// the same Query method set we need. Returns the rows processed.
	RunSingle(ctx context.Context, q Querier) (int, error)
}

// Querier is the subset of pgx.Conn / pgx.Tx we need from a workload.
// Keeping it minimal lets mode1a hand us a pgx.Tx while mode3 hands us a
// pgx.Conn directly.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}
