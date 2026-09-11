package apiserver

import (
	"errors"
	"fmt"
)

// errRetriesExhausted marks casRetry's own exhaustion error so callers can
// tell it apart from a terminal error attempt itself returned (e.g.
// "range is full") -- only the former should be re-wrapped as "too many
// concurrent conflicts"; the latter already has its own complete message
// and must pass through unchanged. See casRetry's doc comment.
var errRetriesExhausted = errors.New("too many concurrent conflicts")

// casRetry runs attempt up to maxRetries times, retrying only when it
// returns ErrConflict or ErrKeyExists (a losing race against a concurrent
// writer) -- the retry shape behind ClusterIPAllocator's
// AllocateNext/Release (clusterip.go).
// Each attempt does its own load-mutate-save cycle against Storage's CAS
// primitives; casRetry doesn't know or care which step inside attempt
// produced an error -- anything other than ErrConflict/ErrKeyExists is
// returned to the caller immediately, unwrapped, exactly as if attempt
// had been called directly without a retry loop.
//
// On exhausting maxRetries, casRetry returns the zero value and an error
// wrapping both errRetriesExhausted and the last conflict seen; use
// errors.Is(err, errRetriesExhausted) to add operation-specific context
// (e.g. "allocate clusterip: %w") only in that case, leaving any other
// error's own message untouched.
func casRetry[R any](maxRetries int, attempt func() (R, error)) (R, error) {
	var zero R
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		result, err := attempt()
		if err == nil {
			return result, nil
		}
		if !errors.Is(err, ErrConflict) && !errors.Is(err, ErrKeyExists) {
			return zero, err
		}
		lastErr = err
	}
	return zero, fmt.Errorf("%w: %w", errRetriesExhausted, lastErr)
}
