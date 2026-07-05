//go:build js && wasm

// Command baseline is throwaway forensics scaffolding for the S12 size
// investigation (see ../README.md). Empty program + the same Cloudflare
// Workers Go runtime shim every real Worker in this repo links against, so
// every other exp-* spike's size can be reported as a delta over this,
// isolating each candidate's own marginal contribution instead of its
// cumulative total.
package main

import (
	"github.com/syumai/workers"
)

func main() {
	workers.Ready()
}
