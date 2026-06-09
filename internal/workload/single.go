package workload

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
)

// SingleSeedRows is the row count seeded by Single.Setup. Iterations
// rotate through them via id = (nextID++) mod SingleSeedRows so the
// access pattern is deterministic and the planner's LRU on the
// resulting plan stays warm.
const SingleSeedRows = 1024

// SingleSQL is the parameterized query Single issues. Exposed so modes
// that need the SQL text directly (mode 2a's multi-stmt Q, mode 3's
// sqlcommenter prepend) can quote against the same string and have
// pgx's statement cache hit deterministically.
const SingleSQL = `SELECT id, payload FROM bench_single WHERE id = $1`

// Single is the headline workload: one parameterized SELECT against a
// small seeded table. The SQL is intentionally trivial so the protocol-
// shape delta isn't drowned by query execution cost.
type Single struct {
	nextID atomic.Int64
}

func NewSingle() *Single { return &Single{} }

func (s *Single) ID() string { return "single" }

func (s *Single) Setup(ctx context.Context, conn *pgx.Conn) error {
	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS bench_single (
			id      int PRIMARY KEY,
			payload text NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create bench_single: %w", err)
	}
	var n int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM bench_single`).Scan(&n); err != nil {
		return fmt.Errorf("count bench_single: %w", err)
	}
	if n >= SingleSeedRows {
		return nil
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO bench_single (id, payload)
		SELECT g, rpad(g::text, 100, '.')
		FROM generate_series(0, $1 - 1) AS g
		ON CONFLICT (id) DO NOTHING
	`, SingleSeedRows); err != nil {
		return fmt.Errorf("seed bench_single: %w", err)
	}
	return nil
}

func (s *Single) Teardown(ctx context.Context, conn *pgx.Conn) error {
	_ = ctx
	_ = conn
	return nil
}

func (s *Single) NextStatements() []Statement {
	id := s.nextID.Add(1) % SingleSeedRows
	return []Statement{{SQL: SingleSQL, Args: []any{id}}}
}
