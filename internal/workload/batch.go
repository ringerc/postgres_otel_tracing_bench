package workload

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Batch is the multi-statement workload: N parameterized SELECTs sharing
// one trace context. Models an ORM that fetches a parent row and then N-1
// related-child rows in one logical operation.
//
// Each mode in the batch suite (B1/B2/B3/B4) wraps Batch's queries with
// its own propagation strategy --- e.g. B2 uses a single pgx.Batch with
// BEGIN/SET LOCAL/<N queries>/COMMIT, B4 emits an M frame before each.
type Batch struct {
	N int // statements per iteration
}

func NewBatch(n int) *Batch { return &Batch{N: n} }

func (b *Batch) ID() string { return "batch" }

func (b *Batch) Setup(ctx context.Context, conn *pgx.Conn) error {
	// TODO: same seeded table as Single but with N×seed rows so a
	// batch can issue N distinct id lookups.
	_ = ctx
	_ = conn
	return errors.New("workload batch Setup not yet implemented")
}

func (b *Batch) Teardown(ctx context.Context, conn *pgx.Conn) error {
	_ = ctx
	_ = conn
	return nil
}

func (b *Batch) RunSingle(ctx context.Context, q Querier) (int, error) {
	// TODO: issue N SELECTs and return the row count. The mode owns
	// pipelining choices --- this function should not assume a Batch
	// is available; modes that want pipelining will provide a Querier
	// shim that batches under the hood.
	_ = ctx
	_ = q
	_ = b.N
	return 0, errors.New("workload batch RunSingle not yet implemented")
}
