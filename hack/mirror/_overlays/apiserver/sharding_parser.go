//go:build js

// GOOS=js overlay: upstream embeds a CEL parser for ShardSelector
// expressions. Nothing here sets a ShardSelector.
package sharding

import (
	"fmt"

	apisharding "k8s.io/apimachinery/pkg/sharding"
)

func Parse(expr string) (apisharding.Selector, error) {
	if expr == "" {
		return nil, nil
	}
	return nil, fmt.Errorf("sharding: ShardSelector expressions are not supported in this build")
}
