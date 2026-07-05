//go:build js && wasm

// exp-typedpkg-only checks whether merely referencing
// k8s.io/client-go/kubernetes/typed/core/v1's PodInterface TYPE (an
// interface -- no concrete implementation, no NewForConfig call) costs
// anything beyond baseline, isolating whether that package has the same
// "sibling files in the same package get linked despite no reference"
// issue found in applyconfigurations/core/v1 (exp-applystub:
// 44.25MiB for an unrelated single type reference, fixed by deleting
// unrelated sibling files down to 2.04MiB).
package main

import (
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

func stub(p corev1.PodInterface) {
	_ = p
}

func main() {
	stub(nil)
	println("built")
}
