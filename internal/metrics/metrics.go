// Package metrics owns the histogram + counter machinery for one bench
// run. Each (mode × latency_preset × workload) cell gets its own Cell;
// the runner emits the union of all Cells as a JSON report.
package metrics

import (
	"time"

	hdr "github.com/HdrHistogram/hdrhistogram-go"
)

// Cell is a single (mode × latency × workload) measurement bucket.
//
// Fields are JSON-tagged for the runner's report emitter.
type Cell struct {
	Mode            string        `json:"mode"`
	LatencyPreset   string        `json:"latency_preset"`
	Workload        string        `json:"workload"`
	Iterations      int           `json:"iterations"`
	Concurrency     int           `json:"concurrency"`
	StartedAt       time.Time     `json:"started_at"`
	EndedAt         time.Time     `json:"ended_at"`
	WallDuration    time.Duration `json:"wall_duration_ns"`
	Errors          int           `json:"errors"`
	BytesSentToPG   int64         `json:"bytes_sent_to_pg"`
	BytesRecvFromPG int64         `json:"bytes_recv_from_pg"`

	// LatencyP50/P95/P99/Max are the per-iteration timings collected
	// via the hdr histogram below, expressed as nanoseconds for
	// machine-readable consumers and in the human-readable table
	// printer.
	LatencyP50Ns int64 `json:"latency_p50_ns"`
	LatencyP95Ns int64 `json:"latency_p95_ns"`
	LatencyP99Ns int64 `json:"latency_p99_ns"`
	LatencyMaxNs int64 `json:"latency_max_ns"`
	LatencyMinNs int64 `json:"latency_min_ns"`

	// PgStatStatementsDelta is the unique-query delta + total calls
	// delta across the run. Mode 3 (sqlcommenter) is where this row
	// blows up.
	PgStatStatementsRows  int   `json:"pg_stat_statements_rows_delta"`
	PgStatStatementsCalls int64 `json:"pg_stat_statements_calls_delta"`

	// PgPreparedStatementsDelta is pg_prepared_statements row count
	// snapshot at end of run, minus start. Modes 1a/1b/2a/2b/4 stay
	// near 1; mode 3 climbs to pgx's LRU cap (~512).
	PgPreparedStatementsDelta int `json:"pg_prepared_statements_delta"`
}

// Recorder ingests per-iteration timings into the histogram and
// counters of one Cell.
//
// The hdr histogram tracks latencies in nanoseconds from 1ns to ~10
// minutes with 3 significant figures --- plenty of headroom for the
// 100ms intercontinental preset, tight resolution at the intradc end.
type Recorder struct {
	cell *Cell
	hist *hdr.Histogram
}

// histMin/histMax are the bounds of the hdr histogram. 1ns lower,
// 10 minutes upper. Sized so even pathological hangs don't trigger
// the recording-out-of-range error path; the actual measurement
// range we care about is ~10µs to ~1s.
const (
	histMin = int64(1)
	histMax = int64(10 * time.Minute / time.Nanosecond)
	sigFigs = 3
)

func NewRecorder(cell *Cell) *Recorder {
	return &Recorder{
		cell: cell,
		hist: hdr.New(histMin, histMax, sigFigs),
	}
}

func (r *Recorder) Observe(d time.Duration) {
	// hdr's RecordValue can fail if d is outside [histMin, histMax].
	// We've sized the range to cover anything sane, but if it does
	// land outside (e.g. a system clock jump returning negative
	// elapsed), bin it as min or max rather than swallowing data.
	v := d.Nanoseconds()
	if v < histMin {
		v = histMin
	}
	if v > histMax {
		v = histMax
	}
	_ = r.hist.RecordValue(v)
}

func (r *Recorder) ObserveError() {
	if r.cell != nil {
		r.cell.Errors++
	}
}

// Finalize computes percentiles and seals the Cell.
func (r *Recorder) Finalize() {
	r.cell.LatencyP50Ns = r.hist.ValueAtQuantile(50)
	r.cell.LatencyP95Ns = r.hist.ValueAtQuantile(95)
	r.cell.LatencyP99Ns = r.hist.ValueAtQuantile(99)
	r.cell.LatencyMaxNs = r.hist.Max()
	r.cell.LatencyMinNs = r.hist.Min()
	r.cell.WallDuration = r.cell.EndedAt.Sub(r.cell.StartedAt)
}
