//go:build js

package filters

import "net/http"

func tooManyRequests(req *http.Request, w http.ResponseWriter, retryAfter string) {
	w.Header().Set("Retry-After", retryAfter)
	http.Error(w, "Too many requests, please try again later.", http.StatusTooManyRequests)
}
