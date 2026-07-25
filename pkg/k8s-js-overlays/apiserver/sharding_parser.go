// js/wasm overlay: the real parser embeds a CEL parser (github.com/google/
// cel-go, multi-MB in wasm) to support ShardSelector expressions. k8flare
// never sets options.ShardSelector, so Parse only needs to handle the
// empty case; a non-empty selector is an explicit unsupported error.
package sharding

import (
	"fmt"

	apisharding "k8s.io/apimachinery/pkg/sharding"
)

func Parse(expr string) (apisharding.Selector, error) {
	if expr == "" {
		return nil, nil
	}
	return nil, fmt.Errorf("sharding: ShardSelector expressions are not supported on js/wasm")
}
