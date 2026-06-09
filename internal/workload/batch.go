package workload

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
)

// Batch is the multi-statement workload: N parameterized SELECTs sharing
// one trace context per iteration. Models an ORM that fetches a parent
// row and then N-1 related-child rows in one logical operation.
//
// Setup reuses the bench_single table so we don't pay a second copy of
// the seed data; "batch" just issues N SELECTs per iteration against
// different ids.
type Batch struct {
	N      int
	nextID atomic.Int64
}

func NewBatch(n int) *Batch { return &Batch{N: n} }

func (b *Batch) ID() string { return "batch" }

func (b *Batch) Setup(ctx context.Context, conn *pgx.Conn) error {
	// Reuse Single's seeded table.
	return (&Single{}).Setup(ctx, conn)
}

func (b *Batch) Teardown(ctx context.Context, conn *pgx.Conn) error {
	_ = ctx
	_ = conn
	return nil
}

func (b *Batch) NextStatements() []Statement {
	if b.N <= 0 {
		// Defensive: a zero/negative N would silently produce no work
		// and lie about iteration count.
		return nil
	}
	out := make([]Statement, b.N)
	base := b.nextID.Add(int64(b.N)) - int64(b.N)
	for i := 0; i < b.N; i++ {
		out[i] = Statement{
			SQL:  SingleSQL,
			Args: []any{(base + int64(i)) % SingleSeedRows},
		}
	}
	return out
}

// Sanity for tests / callers: surface N in errors.
func (b *Batch) String() string { return fmt.Sprintf("batch[N=%d]", b.N) }
