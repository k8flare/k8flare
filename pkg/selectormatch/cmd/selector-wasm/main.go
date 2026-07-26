//go:build js && wasm

// The gateway's watch fan-out (packages/k8flare-worker/src/k8s/watch.ts) used
// to re-implement Kubernetes label/field selector parsing and matching
// in TypeScript -- a hand-rolled subset that silently accepted syntax
// the real apimachinery parsers reject and missed operators they
// support. This binary replaces that subset with the real thing: it
// exports two synchronous js.FuncOf functions backed by
// k8s.io/apimachinery/pkg/labels and pkg/fields, and watch.ts calls
// them per watch event.
//
// Execution model (verified live, docs/platform-verification.md S22):
// the compiled binary ships as a normal bundled wasm module import in
// the gateway Worker (NOT a Loader dynamic worker -- production
// Workers forbid runtime WebAssembly compilation, and a per-event
// Loader hop would put a cold start on the watch hot path, violating
// cost invariant #7's spirit). watch.ts instantiates it once per
// isolate at first use (~26ms) and every subsequent independent
// fetch() event calls the exports synchronously (~0ms) -- S8's
// "goroutines only progress inside the hosting IoContext" constraint
// does not apply to synchronous js.FuncOf callbacks, which run on the
// host JS thread without needing the Go scheduler to advance.
//
// ABI (all strings in, plain JS values out; no promises):
//
//	k8flareValidateSelectors(labelSelector, fieldSelector) -> "" | error string
//	k8flareMatchSelectors(objJSON, labelSelector, fieldSelector) -> { match: bool, err: string }
//
// Selector strings are passed verbatim from the watch request's query
// parameters; empty string means "no selector of that kind". The
// object is the decoded watch-event object re-serialized to JSON --
// field extraction (dotted paths like status.phase) happens here, so
// TypeScript carries no selector semantics at all.
package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
)

// parseSelectors parses both selector strings, returning nil for an
// empty string (no selector).
func parseSelectors(labelSelector, fieldSelector string) (labels.Selector, fields.Selector, error) {
	var lsel labels.Selector
	var fsel fields.Selector
	if labelSelector != "" {
		var err error
		lsel, err = labels.Parse(labelSelector)
		if err != nil {
			return nil, nil, fmt.Errorf("labelSelector: %v", err)
		}
	}
	if fieldSelector != "" {
		var err error
		fsel, err = fields.ParseSelector(fieldSelector)
		if err != nil {
			return nil, nil, fmt.Errorf("fieldSelector: %v", err)
		}
	}
	return lsel, fsel, nil
}

// fieldValue reads a dotted field path from a decoded object, returning
// "" if any segment is missing or non-scalar -- the same "absent field
// compares unequal" semantics both upstream's ToSelectableFields
// fallbacks and the previous TypeScript implementation had.
func fieldValue(obj map[string]any, path string) string {
	var current any = obj
	start := 0
	for i := 0; i <= len(path); i++ {
		if i != len(path) && path[i] != '.' {
			continue
		}
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current, ok = m[path[start:i]]
		if !ok {
			return ""
		}
		start = i + 1
	}
	switch v := current.(type) {
	case string:
		return v
	case bool:
		return fmt.Sprintf("%t", v)
	case float64:
		// JSON numbers; format integers without a mantissa, matching
		// how kubectl-style field selectors express them.
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%v", v)
	default:
		return ""
	}
}

// validateSelectors(labelSelector, fieldSelector) -> "" | error string
func validateSelectors(this js.Value, args []js.Value) any {
	if len(args) != 2 {
		return "k8flareValidateSelectors: want 2 arguments"
	}
	if _, _, err := parseSelectors(args[0].String(), args[1].String()); err != nil {
		return err.Error()
	}
	return ""
}

// matchSelectors(objJSON, labelSelector, fieldSelector) -> {match, err}
func matchSelectors(this js.Value, args []js.Value) any {
	if len(args) != 3 {
		return map[string]any{"match": false, "err": "k8flareMatchSelectors: want 3 arguments"}
	}
	lsel, fsel, err := parseSelectors(args[1].String(), args[2].String())
	if err != nil {
		return map[string]any{"match": false, "err": err.Error()}
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(args[0].String()), &obj); err != nil {
		return map[string]any{"match": false, "err": "object: " + err.Error()}
	}
	if lsel != nil {
		labelSet := labels.Set{}
		if metadata, ok := obj["metadata"].(map[string]any); ok {
			if raw, ok := metadata["labels"].(map[string]any); ok {
				for k, v := range raw {
					if s, ok := v.(string); ok {
						labelSet[k] = s
					}
				}
			}
		}
		if !lsel.Matches(labelSet) {
			return map[string]any{"match": false, "err": ""}
		}
	}
	if fsel != nil {
		fieldSet := fields.Set{}
		for _, req := range fsel.Requirements() {
			fieldSet[req.Field] = fieldValue(obj, req.Field)
		}
		if !fsel.Matches(fieldSet) {
			return map[string]any{"match": false, "err": ""}
		}
	}
	return map[string]any{"match": true, "err": ""}
}

func main() {
	js.Global().Set("k8flareValidateSelectors", js.FuncOf(validateSelectors))
	js.Global().Set("k8flareMatchSelectors", js.FuncOf(matchSelectors))
	// Block forever: the exported js.FuncOf callbacks stay callable from
	// the host without the Go scheduler advancing (S22); main just must
	// not return, or the runtime tears the exports down.
	select {}
}
