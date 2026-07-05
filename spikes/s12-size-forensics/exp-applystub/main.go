//go:build js && wasm

// exp-applystub checks whether merely referencing
// *applyconfigurationscorev1.PodApplyConfiguration in a stub method
// signature (never calling anything on it, never importing
// applyconfigurations/internal directly) drags in the ~40MiB
// applyconfigurations machinery found in exp-applyconfig, or whether the
// bare struct type alone is cheap. Needed before committing to a lean
// client design that must satisfy corev1.PodInterface's Apply/ApplyStatus
// methods (part of the interface even though our controllers never call
// them -- see docs/platform-verification.md's forensics notes).
package main

import (
	"context"

	applyconfigurationscorev1 "k8s.io/client-go/applyconfigurations/core/v1"
)

func applyStub(ctx context.Context, pod *applyconfigurationscorev1.PodApplyConfiguration) error {
	panic("not implemented")
}

func main() {
	applyStub(context.Background(), nil)
	println("built")
}
