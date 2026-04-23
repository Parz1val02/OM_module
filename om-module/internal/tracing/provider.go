package tracing

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// ServiceName is the name reported to Grafana Tempo for all spans.
const ServiceName = "om-module"

// Init sets up the global OpenTelemetry TracerProvider and returns a shutdown
// function that must be deferred by the caller.
//
// tempoEndpoint should be the OTLP/HTTP collector endpoint, e.g.
//
//	"tempo:4318"   (inside Docker network)
//	"localhost:4318" (local dev)
func Init(ctx context.Context, tempoEndpoint string) (func(context.Context) error, error) {
	// Build the OTLP HTTP exporter pointing at Tempo's OTLP receiver.
	// otlptracehttp appends "/v1/traces" automatically.
	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(tempoEndpoint),
		otlptracehttp.WithInsecure(), // no TLS needed inside Docker
	)
	if err != nil {
		return nil, fmt.Errorf("tracing: create OTLP exporter: %w", err)
	}

	// Describe this service to Jaeger / any backend.
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(ServiceName),
			semconv.ServiceVersion("1.0.0"),
			semconv.DeploymentEnvironment("lab"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("tracing: create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		// Always sample in the lab — every operation produces a trace.
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exp,
			sdktrace.WithBatchTimeout(2*time.Second),
		),
		sdktrace.WithResource(res),
	)

	// Register as the global provider so instrumented libraries pick it up.
	otel.SetTracerProvider(tp)

	// W3C TraceContext + Baggage propagation (works with Jaeger and OTel collectors).
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	log.Printf("✅ Distributed tracing initialised → %s", tempoEndpoint)

	return tp.Shutdown, nil
}

// Tracer returns a named tracer scoped to om-module.
// Use this in packages that need to create spans directly.
func Tracer() trace.Tracer {
	return otel.Tracer(ServiceName)
}
