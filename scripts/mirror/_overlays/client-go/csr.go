//go:build js

package csr

import "time"

func ExpirationSecondsToDuration(expirationSeconds int32) time.Duration {
	return time.Duration(expirationSeconds) * time.Second
}
