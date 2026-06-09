package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/modes"
	"github.com/ringerc/postgres_otel_tracing_bench/internal/pgstat"
	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// DemoFlags is the parent flag set for `otelbench demo ...` subcommands.
type DemoFlags struct {
	Common CommonFlags
}

func NewDemoCommand() *cobra.Command {
	f := &DemoFlags{}
	cmd := &cobra.Command{
		Use:   "demo",
		Short: "Run non-timing pathology demonstrations",
		Long: `Demos illustrate problems that don't reduce to a single latency number.

The first such demo is sqlcommenter-pool-break, which shows how
sqlcommenter's per-query SQL-text mutation defeats pgx's automatic
statement cache and stuffs the per-backend prepared-statement LRU
with one-shot entries. This is a pathology demonstration, not a
benchmark --- there is no "good vs bad" comparison; the point is to
show the symptoms.
`,
	}
	bindCommonFlags(cmd, &f.Common)
	cmd.AddCommand(newPoolBreakCommand(&f.Common))
	return cmd
}

func newPoolBreakCommand(common *CommonFlags) *cobra.Command {
	var iterations int
	cmd := &cobra.Command{
		Use:   "sqlcommenter-pool-break",
		Short: "Demonstrate that sqlcommenter prevents pgx's statement cache from working",
		Long: `Issues `+ "`iterations`" + ` parameterized SELECTs through Mode 3
(sqlcommenter SQL-comment prepend). Snapshots pg_stat_statements
and pg_prepared_statements before and after, and prints the deltas.

What this should show:

  * pg_prepared_statements grows up to pgx's LRU cap (~512 entries),
    because every query has a unique SQL text from the per-query
    trace_id in the comment, so pgx's automatic statement cache
    misses every single iteration.

  * pg_stat_statements may NOT grow as dramatically, because postgres
    normalizes comments out of the query identifier --- the parser
    sees the same parse tree, so stats are aggregated to one row.
    The cost shows in the prepared-statement churn, not in
    pg_stat_statements rows.

  * The Parse work and the per-query payload size still bloat the
    wire and the server's plan cache; that cost just isn't visible in
    pg_stat_statements row count.
`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPoolBreak(cmd.Context(), common, iterations,
				cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().IntVar(&iterations, "iterations", 10000,
		"how many sqlcommenter-tagged queries to issue")
	return cmd
}

func runPoolBreak(ctx context.Context, common *CommonFlags, iterations int, stdout, stderr io.Writer) error {
	if iterations <= 0 {
		return fmt.Errorf("--iterations must be > 0")
	}

	connectCtx := ctx
	if common.ConnectTimeout > 0 {
		var cancel context.CancelFunc
		connectCtx, cancel = context.WithTimeout(ctx, common.ConnectTimeout)
		defer cancel()
	}
	conn, err := pgx.Connect(connectCtx, common.DSN)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	w := workload.NewSingle()
	if err := w.Setup(ctx, conn); err != nil {
		return err
	}

	mode := getModeByID("3")
	if mode == nil {
		return fmt.Errorf("mode 3 (sqlcommenter) not registered --- registry corruption?")
	}

	fmt.Fprintf(stderr, "running %d sqlcommenter-tagged iterations...\n", iterations)

	before, _ := pgstat.Take(ctx, conn)
	start := time.Now()
	for i := 0; i < iterations; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		tc, err := newTraceContext()
		if err != nil {
			return fmt.Errorf("traceparent: %w", err)
		}
		if _, err := mode.RunOne(ctx, conn, w, tc); err != nil {
			return fmt.Errorf("iter %d: %w", i, err)
		}
	}
	elapsed := time.Since(start)
	after, _ := pgstat.Take(ctx, conn)
	d := pgstat.Delta(before, after)

	fmt.Fprintf(stdout, "\nSqlcommenter pool-break demo\n")
	fmt.Fprintf(stdout, "============================\n\n")
	fmt.Fprintf(stdout, "  iterations:                  %d\n", iterations)
	fmt.Fprintf(stdout, "  elapsed:                     %s\n", elapsed)
	fmt.Fprintf(stdout, "  per-iteration mean:          %s\n", elapsed/time.Duration(iterations))
	fmt.Fprintf(stdout, "\n")
	fmt.Fprintf(stdout, "  pg_prepared_statements delta:   %d  (pgx default LRU cap is 512)\n",
		d.PreparedStatementsRows)
	if d.HasStatStatements {
		fmt.Fprintf(stdout, "  pg_stat_statements rows delta:  %d  (postgres normalizes comments;\n",
			d.StatStatementsRows)
		fmt.Fprintf(stdout, "                                       a low number here is expected)\n")
		fmt.Fprintf(stdout, "  pg_stat_statements calls delta: %d\n", d.StatStatementsTotalCalls)
	} else {
		fmt.Fprintf(stdout, "  pg_stat_statements: not installed (skip CREATE EXTENSION pg_stat_statements\n")
		fmt.Fprintf(stdout, "                       to see the comment-normalization effect)\n")
	}
	fmt.Fprintf(stdout, "\nInterpretation:\n")
	fmt.Fprintf(stdout, "  Every iteration produced a unique SQL text (trace_id in the\n")
	fmt.Fprintf(stdout, "  sqlcommenter comment varies), so pgx's automatic statement cache\n")
	fmt.Fprintf(stdout, "  missed every time. Each miss allocated a fresh prepared statement\n")
	fmt.Fprintf(stdout, "  on this backend; the LRU bounds growth at its cap. In a long-lived\n")
	fmt.Fprintf(stdout, "  process this means continuous churn: backend pays Parse + plan on\n")
	fmt.Fprintf(stdout, "  every query, plus the eviction cost of evicted statements.\n\n")
	return nil
}

// getModeByID is a tiny helper to avoid importing the bench-mode
// registry construction from inside the demo subcommand --- the demo
// only needs mode 3, by id.
func getModeByID(id string) modes.Mode {
	return modes.NewRegistry().Get(id)
}

// newTraceContext mints a fresh W3C trace context (matches runner's).
// Duplicated here so the demo command has no dependency on internal/runner.
func newTraceContext() (modes.TraceContext, error) {
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
