// GOOS=js overlay of k8s.io/component-base/tracing/utils.go.
//
// Upstream imports the OTLP gRPC exporter at package scope, so any importer
// of this package (k8s.io/apiserver/pkg/storage/cacher, for one) links the
// otel SDK, grpc, and the protobuf reflection runtime. Nothing here
// configures tracing. NewProvider and WithTracing are deliberately absent:
// a caller that wants a real exporter should fail to compile rather than
// silently get a no-op. WrapperFor stays because
// k8s.io/apiserver/pkg/util/webhook links it; it returns the transport
// unchanged.
package tracing

import (
	"context"

	"net/http"

	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
	noopoteltrace "go.opentelemetry.io/otel/trace/noop"
	"k8s.io/client-go/transport"
)

type TracerProvider interface {
	oteltrace.TracerProvider
	Shutdown(context.Context) error
}

type noopTracerProvider struct {
	oteltrace.TracerProvider
}

func (n *noopTracerProvider) Shutdown(context.Context) error { return nil }

func NewNoopTracerProvider() TracerProvider {
	return &noopTracerProvider{TracerProvider: noopoteltrace.NewTracerProvider()}
}

func Propagators() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
}

func WrapperFor(oteltrace.TracerProvider) transport.WrapperFunc {
	return func(rt http.RoundTripper) http.RoundTripper { return rt }
}
