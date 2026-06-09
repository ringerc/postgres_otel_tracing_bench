// Package tracing sets up the OTel SDK for client-side spans.
//
// The benchmark emits one span per iteration (`bench.iteration`) with
// `bench.mode`, `bench.workload`, `bench.latency_preset`, and
// `db.statement` attributes. The traceparent the runner generates for
// each iteration is the same one the mode propagates to the server, so
// contrib/otel's server-side span attaches as a child. This continuity
// is what makes the demo traces look "real" in Jaeger/Tempo.
//
// We deliberately do NOT use a BatchSpanProcessor's sampler --- the
// benchmark wants every iteration recorded for trace-correlation
// verification. If the OTLP collector chokes on volume, lower
// --iterations rather than sampling.
package tracing

import (
	"context"
	"errors"
)

// Tracer is the handle returned to bench code; it knows how to start a
// span with a pre-chosen trace context (so the mode and the SDK agree on
// what traceparent went on the wire).
type Tracer struct {
	endpoint string
	enabled  bool
}

func New(endpoint string) *Tracer {
	return &Tracer{endpoint: endpoint, enabled: endpoint != ""}
}

// Start initializes the OTLP gRPC exporter and the SDK provider.
//
// TODO: implement using go.opentelemetry.io/otel/sdk/trace +
// otlptracegrpc. Use service.name=otelbench, service.version=git SHA,
// schema URL = open-telemetry/semantic-conventions latest stable.
func (t *Tracer) Start(ctx context.Context) error {
	if !t.enabled {
		return nil
	}
	_ = ctx
	return errors.New("tracing.Tracer.Start not yet implemented")
}

// Shutdown flushes pending spans and tears down the provider. Must be
// called before process exit or last-batch spans are lost.
func (t *Tracer) Shutdown(ctx context.Context) error {
	if !t.enabled {
		return nil
	}
	_ = ctx
	return errors.New("tracing.Tracer.Shutdown not yet implemented")
}
