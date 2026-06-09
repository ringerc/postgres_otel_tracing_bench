package workload

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Single is the headline workload: one parameterized SELECT against a
// small seeded table. The SQL is intentionally trivial so the protocol-
// shape delta isn't drowned by query execution cost.
type Single struct {
	// nextID rotates through the seed range. Plain int with no lock
	// because each goroutine in the runner owns its own Single (and
	// its own connection).
	nextID int64
}

func NewSingle() *Single { return &Single{} }

func (s *Single) ID() string { return "single" }

func (s *Single) Setup(ctx context.Context, conn *pgx.Conn) error {
	// TODO: create + seed bench_single table (1024 rows, id int PK,
	// payload text). Use IF NOT EXISTS so reruns are cheap.
	_ = ctx
	_ = conn
	return errors.New("workload single Setup not yet implemented")
}

func (s *Single) Teardown(ctx context.Context, conn *pgx.Conn) error {
	_ = ctx
	_ = conn
	return nil
}

func (s *Single) RunSingle(ctx context.Context, q Querier) (int, error) {
	// TODO: SELECT id, payload FROM bench_single WHERE id = $1 using
	// s.nextID++ mod seed-size; drain Rows; return 1.
	_ = ctx
	_ = q
	return 0, errors.New("workload single RunSingle not yet implemented")
}
