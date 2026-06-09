// Package runner drives one bench cell: a (mode × workload × latency
// preset) measurement. The runner connects to postgres, runs the
// workload's Setup, runs the mode's Setup, executes Warmup
// non-measured iterations, then Iterations measured ones, then tears
// down and returns the populated metrics.Cell.
//
// The runner is concurrency=1 today. Multi-goroutine support
// (--concurrency > 1) belongs in a higher layer that fan-outs across
// independent runners, each with its own connection and its own
// metrics.Cell, then merges the histograms.
package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/metrics"
	"github.com/ringerc/postgres_otel_tracing_bench/internal/modes"
	"github.com/ringerc/postgres_otel_tracing_bench/internal/pgstat"
	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// Config is the input to Run. All fields are required.
type Config struct {
	DSN           string
	Mode          modes.Mode
	Workload      workload.Workload
	Warmup        int
	Iterations    int
	LatencyPreset string // pass-through into the resulting Cell for the report

	// ConnectTimeout caps the initial Connect attempt.
	ConnectTimeout time.Duration
}

// Run executes one cell against the database. The returned Cell is
// always non-nil; on partial completion (errored mid-run) it carries
// the iterations completed successfully so far.
func Run(ctx context.Context, cfg Config) (*metrics.Cell, error) {
	if cfg.Mode == nil {
		return nil, fmt.Errorf("Config.Mode is nil")
	}
	if cfg.Workload == nil {
		return nil, fmt.Errorf("Config.Workload is nil")
	}

	connectCtx := ctx
	if cfg.ConnectTimeout > 0 {
		var cancel context.CancelFunc
		connectCtx, cancel = context.WithTimeout(ctx, cfg.ConnectTimeout)
		defer cancel()
	}
	conn, err := pgx.Connect(connectCtx, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("connect %q: %w", cfg.DSN, err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if err := cfg.Workload.Setup(ctx, conn); err != nil {
		return nil, fmt.Errorf("workload setup: %w", err)
	}
	if err := cfg.Mode.Setup(ctx, conn, cfg.Workload); err != nil {
		return nil, fmt.Errorf("mode setup: %w", err)
	}
	defer func() {
		// Teardown errors are non-fatal: the run's measurements are
		// already in hand and the connection is about to close anyway.
		_ = cfg.Mode.Teardown(ctx, conn)
	}()

	cell := &metrics.Cell{
		Mode:          cfg.Mode.ID(),
		Workload:      cfg.Workload.ID(),
		LatencyPreset: cfg.LatencyPreset,
		Iterations:    cfg.Iterations,
		Concurrency:   1,
	}
	rec := metrics.NewRecorder(cell)

	// Capture pre-run postgres-side counters. Snapshot errors are
	// non-fatal: the bench still produces useful timings even if
	// pg_stat_statements isn't installed.
	before, _ := pgstat.Take(ctx, conn)

	// Warmup --- not measured, but errors here are still surfaced so
	// a broken mode doesn't silently produce a Cell with 0 latency.
	for i := 0; i < cfg.Warmup; i++ {
		if err := ctx.Err(); err != nil {
			return cell, err
		}
		tc, err := newTraceContext()
		if err != nil {
			return cell, fmt.Errorf("warmup %d: traceparent: %w", i, err)
		}
		if _, err := cfg.Mode.RunOne(ctx, conn, cfg.Workload, tc); err != nil {
			return cell, fmt.Errorf("warmup %d: %w", i, err)
		}
	}

	// Measure.
	cell.StartedAt = time.Now()
	for i := 0; i < cfg.Iterations; i++ {
		if err := ctx.Err(); err != nil {
			cell.EndedAt = time.Now()
			rec.Finalize()
			return cell, err
		}
		tc, err := newTraceContext()
		if err != nil {
			rec.ObserveError()
			continue
		}
		start := time.Now()
		_, err = cfg.Mode.RunOne(ctx, conn, cfg.Workload, tc)
		elapsed := time.Since(start)
		if err != nil {
			rec.ObserveError()
			continue
		}
		rec.Observe(elapsed)
	}
	cell.EndedAt = time.Now()
	rec.Finalize()

	// Post-run snapshot; ignore errors here for the same reason as the
	// pre-snapshot. Empty Delta = no pgstat dimension in the report.
	after, _ := pgstat.Take(ctx, conn)
	d := pgstat.Delta(before, after)
	cell.PgStatStatementsRows = d.StatStatementsRows
	cell.PgStatStatementsCalls = d.StatStatementsTotalCalls
	cell.PgPreparedStatementsDelta = d.PreparedStatementsRows

	return cell, nil
}

// newTraceContext mints a fresh per-iteration W3C trace context.
// The trace_id varies per iteration so modes 3 / 4 (which mutate the
// SQL or the wire frame with the trace_id) actually exercise the
// per-query path, not a constant; pgx's statement cache only behaves
// pathologically under mode 3 because of this.
func newTraceContext() (modes.TraceContext, error) {
	var buf [24]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return modes.TraceContext{}, err
	}
	return modes.TraceContext{
		TraceID:    hex.EncodeToString(buf[:16]),
		SpanID:     hex.EncodeToString(buf[16:24]),
		TraceFlags: "01", // sampled
	}, nil
}
