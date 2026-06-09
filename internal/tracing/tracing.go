// Package tracing sets up the OTel SDK for client-side spans.
//
// The benchmark emits one span per iteration (`bench.iteration`); per-
// query spans inside the iteration come from `otelpgx`, registered as
// pgx's `ConnConfig.Tracer`. The traceparent the runner produces for
// each iteration is derived from the iteration span's SpanContext, so
// the same trace_id flows through:
//
//	bench.iteration  (client-side, this process)
//	  -> pgx.Query   (client-side, otelpgx)
//	     -> postgres-side span(s)  (contrib/otel)
//
// All three layers share the trace_id, which is what makes the demo
// traces look "real" in Jaeger / Tempo. The mode under test is what
// carries the trace_id over the wire (M frame, SET LOCAL, sqlcommenter,
// etc.); the SDK + otelpgx provide the parent context the mode plumbs
// the value into.
package tracing

import (
	"context"
	"fmt"

	"github.com/exaring/otelpgx"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

const (
	serviceName    = "otelbench"
	tracerName     = "github.com/ringerc/postgres_otel_tracing_bench"
	pgxTracerName  = "github.com/ringerc/postgres_otel_tracing_bench/pgx"
)

// Provider is the runtime state owned by Setup. Callers must call
// Shutdown before process exit or the last in-flight batch is lost.
type Provider struct {
	tp      *sdktrace.TracerProvider
	enabled bool
}

// Tracer returns a no-op tracer if disabled, or a real one bound to
// the SDK provider Setup constructed. Always safe to call.
func (p *Provider) Tracer() trace.Tracer {
	if p == nil || !p.enabled {
		return tracenoop.NewTracerProvider().Tracer(tracerName)
	}
	return p.tp.Tracer(tracerName)
}

// PgxTracer returns a pgx-compatible QueryTracer/BatchTracer/etc.
// implementation (otelpgx) wired to the SDK provider. nil when
// tracing is disabled --- callers should leave ConnConfig.Tracer nil
// in that case.
func (p *Provider) PgxTracer() any {
	if p == nil || !p.enabled {
		return nil
	}
	return otelpgx.NewTracer(
		otelpgx.WithTracerProvider(p.tp),
		// Span names like "db.query: SELECT id, payload FROM ..." make
		// the trace tree readable at a glance. otelpgx's default uses
		// just "query" / "exec" which loses information.
		otelpgx.WithSpanNameFunc(spanNameFromSQL),
		// We instrument copy/batch too; not used by Mode 1a-4 today but
		// will be exercised by the batch suite.
	)
}

// Shutdown flushes pending spans and tears down the provider.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil || !p.enabled {
		return nil
	}
	return p.tp.Shutdown(ctx)
}

// Setup initializes the OTel SDK. An empty endpoint returns an
// always-disabled Provider --- the benchmark still runs, just without
// emitting spans, which is what we want in `--otlp-endpoint=""`
// quick-iteration mode.
func Setup(ctx context.Context, endpoint string) (*Provider, error) {
	if endpoint == "" {
		return &Provider{enabled: false}, nil
	}

	// Resource: identifies this process in the collector. We build the
	// custom resource schemaless (no schema URL) rather than tagging it
	// with our semconv version's SchemaURL: resource.Merge hard-errors
	// when both inputs carry different non-empty schema URLs, and
	// resource.Default()'s URL tracks the SDK's own semconv version,
	// which drifts ahead of whatever semconv package we import. A
	// schemaless resource merges cleanly and inherits Default()'s URL.
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	exp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(), // local collector; production would gate on TLS config
	)
	if err != nil {
		return nil, fmt.Errorf("OTLP gRPC exporter at %s: %w", endpoint, err)
	}

	// BatchSpanProcessor with default sizing. The benchmark wants
	// every iteration span recorded; AlwaysSample at the SDK side
	// makes that explicit. (otelpgx defers sampling to the SDK; it
	// doesn't apply its own decision.)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	// Register globally so libraries that pull from otel.Tracer() see
	// our provider too. Not strictly needed since we pass tp around
	// explicitly, but keeps surprise libraries (e.g. otelgrpc in
	// otlptracegrpc itself) from spinning up their own provider.
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Provider{tp: tp, enabled: true}, nil
}

// spanNameFromSQL trims SQL to a span name. otelpgx's default is just
// "query"; appending the first ~60 chars makes the trace tree readable
// without ballooning attribute payloads.
func spanNameFromSQL(stmt string) string {
	const limit = 60
	// Single-line: collapse runs of whitespace.
	out := make([]byte, 0, limit+8)
	prevSpace := false
	for i := 0; i < len(stmt) && len(out) < limit; i++ {
		c := stmt[i]
		if c == '\n' || c == '\r' || c == '\t' || c == ' ' {
			if prevSpace || len(out) == 0 {
				continue
			}
			out = append(out, ' ')
			prevSpace = true
			continue
		}
		out = append(out, c)
		prevSpace = false
	}
	if len(out) >= limit {
		out = append(out, []byte("...")...)
	}
	return string(out)
}

// IterationAttributes returns the OTel attribute set we tag every
// bench.iteration span with. Exposed so the runner can set them on the
// span it owns; otelpgx's child spans inherit the trace context but
// don't carry these bench-specific labels.
func IterationAttributes(mode, workload, latencyPreset string, iter int) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("bench.mode", mode),
		attribute.String("bench.workload", workload),
		attribute.String("bench.latency_preset", latencyPreset),
		attribute.Int("bench.iteration", iter),
	}
}

// IterationSpanName is the constant span name. Kept as a function for
// future per-mode customization.
func IterationSpanName() string { return "bench.iteration" }

