package apiserver

import (
	"net/http"
)

// HandleWatch handles Kubernetes watch requests.
// In production, the JS layer (worker.mjs) intercepts watch requests and handles
// them via WebSocket streaming. This Go handler serves as a fallback that returns
// an empty watch response, causing the client to reconnect.
func HandleWatch(w http.ResponseWriter, r *http.Request, resource string, namespace string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.WriteHeader(http.StatusOK)
	// Empty response body — client will interpret as watch timeout and reconnect
}
