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
	"github.com/jackc/pgx/v5/tracelog"

	"go.opentelemetry.io/otel/attribute"
	otrace "go.opentelemetry.io/otel/trace"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/metrics"
	"github.com/ringerc/postgres_otel_tracing_bench/internal/modes"
	"github.com/ringerc/postgres_otel_tracing_bench/internal/pgstat"
	"github.com/ringerc/postgres_otel_tracing_bench/internal/tracing"
	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// Config is the input to Run. All fields are required.
type Config struct {
	DSN           string
	Mode          modes.Mode
	Workload      workload.Workload
	Warmup        int
	Iterations    int
	LatencyPreset string

	// ConnectTimeout caps the initial Connect attempt.
	ConnectTimeout time.Duration

	// RuntimeParams are extra StartupMessage parameters to set on the
	// pgconn config before dialing. Modes that need protocol-level
	// negotiation (e.g. Mode 4 with _pq_.headers=1) thread their
	// requirements through here.
	RuntimeParams map[string]string

	// Tracing is the OTel provider; may be a disabled Provider, in
	// which case the runner skips span emission and falls back to
	// fresh-random trace IDs per iteration.
	Tracing *tracing.Provider
}

// Run executes one cell against the database.
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
	pgxConfig, err := pgx.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse DSN %q: %w", cfg.DSN, err)
	}
	if pgxConfig.RuntimeParams == nil {
		pgxConfig.RuntimeParams = map[string]string{}
	}
	for k, v := range cfg.RuntimeParams {
		pgxConfig.RuntimeParams[k] = v
	}
	// Install otelpgx as pgx's query tracer if tracing is enabled.
	// otelpgx implements QueryTracer/BatchTracer/PrepareTracer; pgx
	// calls into it around every query/batch/prepare so per-statement
	// spans land automatically.
	if t := cfg.Tracing.PgxTracer(); t != nil {
		// pgx's Tracer slot is a generic interface; otelpgx satisfies
		// all the per-operation tracer interfaces it cares about.
		// (The runtime cast is safe: otelpgx.NewTracer returns a
		// *Tracer which implements QueryTracer + BatchTracer + ...)
		if tr, ok := t.(interface {
			pgx.QueryTracer
		}); ok {
			pgxConfig.Tracer = tr
		}
	}
	// Silence pgx's own debug tracer (if anyone wired one before).
	_ = tracelog.LogLevelNone

	conn, err := pgx.ConnectConfig(connectCtx, pgxConfig)
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
	defer func() { _ = cfg.Mode.Teardown(ctx, conn) }()

	cell := &metrics.Cell{
		Mode:          cfg.Mode.ID(),
		Workload:      cfg.Workload.ID(),
		LatencyPreset: cfg.LatencyPreset,
		Iterations:    cfg.Iterations,
		Concurrency:   1,
	}
	rec := metrics.NewRecorder(cell)

	before, _ := pgstat.Take(ctx, conn)

	tracer := cfg.Tracing.Tracer()

	// Warmup --- still wrapped in spans so the trace tree shows warmup
	// vs measure clearly; ObserveError counts apply but timings don't
	// land in the histogram.
	for i := 0; i < cfg.Warmup; i++ {
		if err := ctx.Err(); err != nil {
			return cell, err
		}
		if err := oneIteration(ctx, conn, cfg, tracer, i, true); err != nil {
			return cell, fmt.Errorf("warmup %d: %w", i, err)
		}
	}

	cell.StartedAt = time.Now()
	for i := 0; i < cfg.Iterations; i++ {
		if err := ctx.Err(); err != nil {
			cell.EndedAt = time.Now()
			rec.Finalize()
			return cell, err
		}
		start := time.Now()
		err := oneIteration(ctx, conn, cfg, tracer, i, false)
		elapsed := time.Since(start)
		if err != nil {
			rec.ObserveError()
			continue
		}
		rec.Observe(elapsed)
	}
	cell.EndedAt = time.Now()
	rec.Finalize()

	after, _ := pgstat.Take(ctx, conn)
	d := pgstat.Delta(before, after)
	cell.PgStatStatementsRows = d.StatStatementsRows
	cell.PgStatStatementsCalls = d.StatStatementsTotalCalls
	cell.PgPreparedStatementsDelta = d.PreparedStatementsRows

	return cell, nil
}

// oneIteration wraps one mode.RunOne in a bench.iteration span,
// deriving the W3C trace context the mode propagates from the span's
// SpanContext (when tracing is enabled). When tracing is disabled the
// SpanContext is invalid; we fall back to crypto/rand for trace_id so
// every iteration still gets a fresh, distinct value (Mode 3's
// sqlcommenter-cache-thrash story depends on this).
func oneIteration(ctx context.Context, conn *pgx.Conn, cfg Config, tracer otrace.Tracer, iter int, warmup bool) error {
	spanName := tracing.IterationSpanName()
	if warmup {
		spanName += ".warmup"
	}
	attrs := append(
		tracing.IterationAttributes(cfg.Mode.ID(), cfg.Workload.ID(), cfg.LatencyPreset, iter),
		attribute.Bool("bench.warmup", warmup),
	)
	ctx, span := tracer.Start(ctx, spanName, otrace.WithAttributes(attrs...))
	defer span.End()

	tc, err := traceContextFromSpan(span)
	if err != nil {
		return fmt.Errorf("traceparent: %w", err)
	}
	_, err = cfg.Mode.RunOne(ctx, conn, cfg.Workload, tc)
	return err
}

// traceContextFromSpan builds the W3C trace context the mode under
// test propagates over the wire. When the span has a valid trace_id
// (tracing enabled), use it so client-side spans and server-side
// contrib/otel spans share the trace. When invalid, mint a fresh
// random one so the mode still produces realistic input.
func traceContextFromSpan(span otrace.Span) (modes.TraceContext, error) {
	sc := span.SpanContext()
	if sc.HasTraceID() && sc.HasSpanID() {
		flags := "00"
		if sc.IsSampled() {
			flags = "01"
		}
		return modes.TraceContext{
			TraceID:    sc.TraceID().String(),
			SpanID:     sc.SpanID().String(),
			TraceFlags: flags,
		}, nil
	}
	var buf [24]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return modes.TraceContext{}, err
	}
	return modes.TraceContext{
		TraceID:    hex.EncodeToString(buf[:16]),
		SpanID:     hex.EncodeToString(buf[16:24]),
		TraceFlags: "01",
	}, nil
}
