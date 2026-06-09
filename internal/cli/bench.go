package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// BenchFlags holds bench-specific flags layered on top of CommonFlags.
type BenchFlags struct {
	Common CommonFlags

	// Modes is the comma-separated list of modes to run, e.g. "1a,1b,2b,3"
	// or "all". Mode 4 is only registered when the binary is built with
	// -tags=patched_pgx; if requested without the tag we error out.
	Modes string

	// Workload selects which workload the modes are exercised against.
	// "single" = one parameterized SELECT per iteration.
	// "batch"  = a multi-statement SELECT batch (N rows hydrated) per
	//            iteration; exercises the B1/B2/B3/B4 batch suite.
	Workload string

	// BatchSize is N for the "batch" workload (statements per iteration).
	BatchSize int

	// Iterations is the count of measured iterations per mode (after
	// warmup). Higher = tighter percentiles, longer runs.
	Iterations int

	// Warmup is the count of pre-measurement iterations per mode. The
	// goal is to fill the statement cache and warm the connection.
	Warmup int

	// Concurrency is the number of parallel client goroutines.
	// Each gets its own connection (no shared pool internally; pgxpool
	// could be added later if we want to compare pool dynamics).
	Concurrency int

	// Latency selects the toxiproxy preset to apply for the run.
	Latency string

	// SweepLatency, when set, ignores --latency and runs every preset
	// in sequence, producing a row per preset in the output.
	SweepLatency bool

	// ReportPath is the JSON output file path. "-" = stdout.
	ReportPath string

	// MaxDuration caps the run wall-clock; the run ends after this even
	// if Iterations hasn't been hit. Zero = no cap.
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
			return fmt.Errorf("TODO: bench runner not yet implemented (modes=%s, workload=%s)", f.Modes, f.Workload)
		},
	}
	bindCommonFlags(cmd, &f.Common)
	cmd.Flags().StringVar(&f.Modes, "modes", "1a,1b,2a,2b,3",
		"comma-separated modes to run, or 'all' (mode 4 requires patched_pgx build)")
	cmd.Flags().StringVar(&f.Workload, "workload", "single",
		"workload: 'single' (one parameterized SELECT) or 'batch'")
	cmd.Flags().IntVar(&f.BatchSize, "batch-size", 10,
		"N for the 'batch' workload")
	cmd.Flags().IntVar(&f.Iterations, "iterations", 10000,
		"measured iterations per mode")
	cmd.Flags().IntVar(&f.Warmup, "warmup", 500,
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
