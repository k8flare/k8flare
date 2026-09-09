package apiserver

import (
	"net/http"
)

// RegisterInternalHandlers registers routes meant only for other Workers in
// this project to call via a Cloudflare service binding (not for kubectl or
// any other public client) -- mirrors RegisterSupervisorHandlers'
// registration style (supervisor.go) and its lack of AuthMiddleware: a
// service binding is only reachable by a Worker this account explicitly
// wired a "services" binding to, which is the actual trust boundary here,
// the same way it already is for the Cluster DO's existing CONTROLLERS
// binding (index.ts's pingControllers).
func RegisterInternalHandlers(mux *http.ServeMux, storage *Storage) {
	// GET /internal/vkubeproxy-resolve?ip=&port=: the ClusterIP->(Pod
	// UID, container port) resolution half of nodes/podproxy.ts's
	// handleVKubeProxy (vkubeproxy.go).
	mux.HandleFunc("GET /internal/vkubeproxy-resolve", handleVKubeProxyResolve(storage))

	// GET /internal/next-cron-schedule: when the Controllers DO must wake
	// next for a CronJob, instead of parking (cronschedule.go).
	mux.HandleFunc("GET /internal/next-cron-schedule", handleNextCronSchedule(storage))
}
