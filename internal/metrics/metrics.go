// Package metrics owns the histogram + counter machinery for one bench
// run. Each (mode × latency_preset × workload) cell gets its own Cell;
// the runner emits the union of all Cells as a JSON report.
package metrics

import (
	"time"
)

// Cell is a single (mode × latency × workload) measurement bucket.
type Cell struct {
	Mode           string
	LatencyPreset  string
	Workload       string
	Iterations     int
	Concurrency    int
	StartedAt      time.Time
	EndedAt        time.Time
	Errors         int
	BytesSentToPG  int64  // pulled from toxiproxy after the run
	BytesRecvFromPG int64 // pulled from toxiproxy after the run

	// LatencyP50/P95/P99/Max are the per-iteration timings collected
	// via the hdr histogram below.
	LatencyP50 time.Duration
	LatencyP95 time.Duration
	LatencyP99 time.Duration
	LatencyMax time.Duration

	// PgStatStatementsDelta is a snapshot of pg_stat_statements before
	// and after the run; the runner subtracts and stores the unique-
	// query delta + total calls delta. Mode 3 (sqlcommenter) is where
	// this row blows up.
	PgStatStatementsRows  int
	PgStatStatementsCalls int64

	// PgPreparedStatementsDelta is pg_prepared_statements row count
	// snapshot at end of run, minus start. Modes 1a/1b/2a/2b/4 stay
	// near 1; mode 3 climbs to pgx's LRU cap (~512).
	PgPreparedStatementsDelta int
}

// Recorder ingests per-iteration timings into the histogram and
// counters of one Cell.
//
// TODO: implement on top of github.com/HdrHistogram/hdrhistogram-go.
type Recorder struct {
	cell *Cell
}

func NewRecorder(cell *Cell) *Recorder { return &Recorder{cell: cell} }

func (r *Recorder) Observe(d time.Duration) {
	_ = d
	// TODO
}

func (r *Recorder) ObserveError() {
	if r.cell != nil {
		r.cell.Errors++
	}
}

// Finalize computes percentiles and seals the Cell.
func (r *Recorder) Finalize() {
	// TODO: pull percentiles from the hdr histogram into r.cell.
}
