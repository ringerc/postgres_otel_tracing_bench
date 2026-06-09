// Package modes defines the trace-context-propagation methods we benchmark.
//
// Every mode implements a single tiny interface: given a connection and a
// trace context, run one iteration of the workload's query and return.
// The benchmark runner handles timing, repetition, and result aggregation
// around it.
//
// Each mode's file documents the wire-protocol shape it produces in its
// doc comment so the benchmark numbers can be read against the protocol
// behavior rather than against a black-box driver call.
package modes

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/ringerc/postgres_otel_tracing_bench/internal/workload"
)

// TraceContext is the W3C trace context we propagate via the
// mode-under-test. The benchmark runner generates one per iteration.
//
// We carry the values as already-formatted strings (lowercase hex) because
// every propagation channel ends up writing them as text on the wire
// anyway --- GUC value, SQL comment payload, or 'M' message field.
type TraceContext struct {
	TraceID    string // 32 lowercase hex chars
	SpanID     string // 16 lowercase hex chars
	TraceFlags string // 2 lowercase hex chars (e.g. "01" for sampled)
}

// Traceparent returns the W3C traceparent value for tc.
func (tc TraceContext) Traceparent() string {
	return "00-" + tc.TraceID + "-" + tc.SpanID + "-" + tc.TraceFlags
}

// Mode is the protocol-level propagation strategy under test.
type Mode interface {
	// ID is the short identifier used on the command line ("1a", "2b",
	// "3", "4", ...).
	ID() string

	// Description is the human-readable wire-protocol summary.
	Description() string

	// Setup is invoked once per connection before any iterations. Use it
	// to pre-prepare statements, set session-level state, etc. The
	// runner has already opened the connection and set up the workload's
	// tables.
	Setup(ctx context.Context, conn *pgx.Conn, w workload.Workload) error

	// RunOne issues the workload's query(s) for one iteration, with the
	// given trace context propagated via this mode's chosen channel.
	// Return value is the row count consumed (validated by the runner)
	// or an error.
	RunOne(ctx context.Context, conn *pgx.Conn, w workload.Workload, tc TraceContext) (rows int, err error)

	// Teardown is invoked once per connection after the last iteration.
	// Use it to RESET / DEALLOCATE anything Setup did. May be a no-op.
	Teardown(ctx context.Context, conn *pgx.Conn) error
}

// Registry holds the set of modes available to the current binary. Mode 4
// is registered only when built with -tags=patched_pgx; see m4.go and
// m4_stub.go.
type Registry struct {
	modes map[string]Mode
}

// NewRegistry returns a Registry populated with every Mode the current
// binary knows about (build-tag dependent).
func NewRegistry() *Registry {
	r := &Registry{modes: map[string]Mode{}}
	for _, m := range builtinModes() {
		r.modes[m.ID()] = m
	}
	return r
}

// Get returns the mode with the given ID, or nil.
func (r *Registry) Get(id string) Mode { return r.modes[id] }

// IDs returns all registered mode IDs, in registration order.
func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.modes))
	for id := range r.modes {
		ids = append(ids, id)
	}
	return ids
}
