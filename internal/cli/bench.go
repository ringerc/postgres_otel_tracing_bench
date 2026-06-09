package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/latency"
	"github.com/ringerc/postgres_otel_tracing_bench/internal/metrics"
	"github.com/ringerc/postgres_otel_tracing_bench/internal/modes"
	"github.com/ringerc/postgres_otel_tracing_bench/internal/runner"
	"github.com/ringerc/postgres_otel_tracing_bench/internal/tracing"
	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// BenchFlags holds bench-specific flags layered on top of CommonFlags.
type BenchFlags struct {
	Common      CommonFlags
	Modes       string
	Workload    string
	BatchSize   int
	Iterations  int
	Warmup      int
	Concurrency int
	Latency     string
	SweepLatency bool
	ReportPath  string
	MaxDuration time.Duration
}

func NewBenchCommand() *cobra.Command {
	f := &BenchFlags{}
	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Run the timing benchmark across one or more propagation modes",
		Long: `Run the latency benchmark for trace-context propagation modes.

Each mode runs the chosen workload for --iterations iterations (after
--warmup warmup iterations), under the chosen --latency preset. Latency
shaping is applied via toxiproxy (see --toxiproxy-url). Per-iteration
timings, wire-byte counts, and pg_stat_statements deltas are collected
and emitted as JSON.

Mode IDs:
  1a   BEGIN; SET LOCAL ...; <SQL>; COMMIT;  (sequential, 4 RTT)
  1b   SET ...; <SQL>; RESET ...;            (sequential, 3 RTT)
  2a   "SET LOCAL ...; <SQL>;" as simple Q   (1 RTT, no statement cache)
  2b   pgx.Batch (BEGIN/SETLOCAL/SQL/COMMIT) (1 RTT, cache hits on SQL)
  3    sqlcommenter SQL comment prepend      (1 RTT, NO cache hits)
  4    'M' RequestHeaders message            (1 RTT, requires patched pgx)
`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBench(cmd, f)
		},
	}
	bindCommonFlags(cmd, &f.Common)
	cmd.Flags().StringVar(&f.Modes, "modes", "1a",
		"comma-separated modes to run, or 'all' (mode 4 requires patched_pgx build)")
	cmd.Flags().StringVar(&f.Workload, "workload", "single",
		"workload: 'single' (one parameterized SELECT) or 'batch'")
	cmd.Flags().IntVar(&f.BatchSize, "batch-size", 10,
		"N for the 'batch' workload")
	cmd.Flags().IntVar(&f.Iterations, "iterations", 1000,
		"measured iterations per mode")
	cmd.Flags().IntVar(&f.Warmup, "warmup", 100,
		"warmup iterations per mode (not measured)")
	cmd.Flags().IntVar(&f.Concurrency, "concurrency", 1,
		"parallel client goroutines (each with its own connection)")
	cmd.Flags().StringVar(&f.Latency, "latency", "none",
		"toxiproxy preset: none|intradc|crossaz|crossregion|intercontinental")
	cmd.Flags().BoolVar(&f.SweepLatency, "sweep-latency", false,
		"run every latency preset in sequence; --latency is ignored")
	cmd.Flags().StringVar(&f.ReportPath, "report", "-",
		"JSON report output path ('-' = stdout)")
	cmd.Flags().DurationVar(&f.MaxDuration, "max-duration", 0,
		"wall-clock cap; 0 = no cap")
	return cmd
}

func runBench(cmd *cobra.Command, f *BenchFlags) error {
	if f.Concurrency != 1 {
		return fmt.Errorf("--concurrency=%d not yet supported; only 1 works today", f.Concurrency)
	}

	ids, err := parseModeList(f.Modes)
	if err != nil {
		return err
	}
	registry := modes.NewRegistry()
	w, err := newWorkload(f.Workload, f.BatchSize)
	if err != nil {
		return err
	}

	latencyPresets, err := resolveLatencyPresets(f)
	if err != nil {
		return err
	}

	// Toxiproxy client is optional --- if we can't reach it, run only
	// the "none" preset (a no-op latency). This lets the harness work
	// in plain dev runs without toxiproxy present.
	var toxic *latency.Client
	if anyNonPassthrough(latencyPresets) {
		toxic, err = latency.New(f.Common.ToxiproxyURL, f.Common.ToxiproxyProxyName)
		if err != nil {
			return fmt.Errorf("toxiproxy: %w (run with --latency=none or start toxiproxy)", err)
		}
		defer func() { _ = toxic.Clear() }()
	}

	ctx := cmd.Context()
	if f.MaxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, f.MaxDuration)
		defer cancel()
	}

	tp, err := tracing.Setup(ctx, f.Common.OTLPEndpoint)
	if err != nil {
		return fmt.Errorf("tracing setup: %w", err)
	}
	defer func() {
		// Allow up to 5s for the last batch of spans to flush.
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(shutCtx)
	}()

	var cells []*metrics.Cell
	for _, preset := range latencyPresets {
		if toxic != nil {
			if err := toxic.Apply(preset); err != nil {
				return fmt.Errorf("apply latency %s: %w", preset, err)
			}
		}
		for _, id := range ids {
			m := registry.Get(id)
			if m == nil {
				return fmt.Errorf("unknown mode %q (available: %s)", id,
					strings.Join(registry.IDs(), ","))
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "running mode %s @ latency=%s: %s\n",
				m.ID(), preset, m.Description())
			cell, err := runner.Run(ctx, runner.Config{
				DSN:            f.Common.DSN,
				Mode:           m,
				Workload:       w,
				Warmup:         f.Warmup,
				Iterations:     f.Iterations,
				LatencyPreset:  string(preset),
				ConnectTimeout: f.Common.ConnectTimeout,
				RuntimeParams:  runtimeParamsFor(m),
				Tracing:        tp,
			})
			if cell != nil {
				cells = append(cells, cell)
			}
			if err != nil {
				return fmt.Errorf("mode %s @ latency=%s: %w", m.ID(), preset, err)
			}
		}
	}

	return emitReport(f.ReportPath, cells, cmd.OutOrStdout())
}

func resolveLatencyPresets(f *BenchFlags) ([]latency.Preset, error) {
	if f.SweepLatency {
		return latency.AllPresets, nil
	}
	p := latency.Preset(f.Latency)
	if !p.IsValid() {
		return nil, fmt.Errorf("unknown --latency preset %q (valid: %v)", f.Latency, latency.AllPresets)
	}
	return []latency.Preset{p}, nil
}

func anyNonPassthrough(ps []latency.Preset) bool {
	for _, p := range ps {
		if p != latency.PresetNone {
			return true
		}
	}
	return false
}

// runtimeParamsFor returns the StartupMessage extras needed for a given
// mode. Mode 4 ('M' RequestHeaders) requires _pq_.headers=1; everyone
// else gets an empty map.
func runtimeParamsFor(m modes.Mode) map[string]string {
	if m.ID() == "4" {
		return map[string]string{"_pq_.headers": "1"}
	}
	return nil
}

func parseModeList(spec string) ([]string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("--modes is empty")
	}
	if spec == "all" {
		// Resolve at runner time so the patched_pgx build picks up mode 4.
		return modes.NewRegistry().IDs(), nil
	}
	parts := strings.Split(spec, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--modes parsed to empty list")
	}
	return out, nil
}

func newWorkload(id string, batchSize int) (workload.Workload, error) {
	switch id {
	case "single":
		return workload.NewSingle(), nil
	case "batch":
		if batchSize <= 0 {
			return nil, fmt.Errorf("--batch-size must be > 0")
		}
		return workload.NewBatch(batchSize), nil
	default:
		return nil, fmt.Errorf("unknown workload %q (want 'single' or 'batch')", id)
	}
}

func emitReport(path string, cells []*metrics.Cell, stdout io.Writer) error {
	var w io.Writer
	switch path {
	case "", "-":
		w = stdout
	default:
		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("open report: %w", err)
		}
		defer f.Close()
		w = f
	}

	// Human-readable table on stderr; JSON for machine consumption on
	// the chosen writer. The JSON encoder writes one Cell per array
	// element so streaming consumers can pretty-print incrementally.
	printTable(os.Stderr, cells)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(cells)
}

func printTable(w io.Writer, cells []*metrics.Cell) {
	fmt.Fprintf(w, "\n%-4s %-7s %-17s %6s %6s %10s %10s %10s %10s %9s %10s %12s\n",
		"mode", "wl", "latency", "iters", "errs", "p50", "p95", "p99", "max",
		"pgss_rows", "pgss_calls", "prep_stmts_+")
	for _, c := range cells {
		fmt.Fprintf(w, "%-4s %-7s %-17s %6d %6d %10s %10s %10s %10s %9d %10d %12d\n",
			c.Mode, c.Workload, c.LatencyPreset,
			c.Iterations, c.Errors,
			fmtDur(c.LatencyP50Ns), fmtDur(c.LatencyP95Ns),
			fmtDur(c.LatencyP99Ns), fmtDur(c.LatencyMaxNs),
			c.PgStatStatementsRows, c.PgStatStatementsCalls,
			c.PgPreparedStatementsDelta)
	}
	fmt.Fprintln(w)
}

func fmtDur(ns int64) string {
	return time.Duration(ns).String()
}
