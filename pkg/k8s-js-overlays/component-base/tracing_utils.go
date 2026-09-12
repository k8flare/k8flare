/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// GOOS=js replacement for k8s.io/component-base/tracing/utils.go.
//
// Upstream's utils.go imports go.opentelemetry.io/otel/exporters/otlp/
// otlptrace/otlptracegrpc at package scope, so merely importing
// component-base/tracing links the whole OTLP-over-gRPC exporter and, with
// it, google.golang.org/grpc and the protobuf runtime. k8s.io/apiserver/pkg/
// storage/cacher imports tracing for Start and SpanFromContext, which live in
// tracing.go and need none of that.
//
// Nothing in this build calls the functions utils.go provides: their callers
// (pkg/server/options, pkg/server, pkg/util/webhook, pkg/endpoints/filters)
// are not in the link graph, verified with `go list -deps`. The two that are
// referenced from linked code by name only -- NewNoopTracerProvider and
// Propagators -- are kept with equivalent no-op behaviour so a future caller
// compiles and gets tracing that does nothing, rather than tracing that dials
// a collector this runtime cannot reach.
//
// NewProvider and WrapperFor are deliberately absent: a caller that wants a
// real exporter should fail to compile here, not silently get a no-op.

package tracing

import (
	"context"

	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
	noopoteltrace "go.opentelemetry.io/otel/trace/noop"
)

// TracerProvider is upstream's interface, kept verbatim: the cacher and
// server config refer to it by name.
type TracerProvider interface {
	oteltrace.TracerProvider
	Shutdown(context.Context) error
}

type noopTracerProvider struct {
	oteltrace.TracerProvider
}

func (n *noopTracerProvider) Shutdown(context.Context) error { return nil }

// NewNoopTracerProvider returns a TracerProvider that does nothing.
func NewNoopTracerProvider() TracerProvider {
	return &noopTracerProvider{TracerProvider: noopoteltrace.NewTracerProvider()}
}

// Propagators returns the W3C propagators. No exporter is involved: the
// propagator only reads and writes request headers.
func Propagators() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
}
