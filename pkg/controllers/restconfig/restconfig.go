//go:build js && wasm

// Package restconfig builds an in-memory *rest.Config for this repo's
// WASM-resident control-plane binaries (pkg/controllers.
// RunControllerManager, pkg/controllers/gc.RunGarbageCollector) -- see
// docs/platform-verification.md's S8 section for the execution shape
// this supports (DO-hosted + WaitUntil-resident + event-armed alarm()
// safety net) and spikes/s8-wasm-resident/FINDINGS.md for the
// underlying spike.
//
// cmd/scheduler and cmd/controller-manager are unchanged: their normal
// startup path (NewSchedulerCommand/NewControllerManagerCommand ->
// SetArgs -> ExecuteContext) loads a kubeconfig from a real file on disk
// via client-go's clientcmd, which this environment cannot support (no
// filesystem -- wasm_exec.js's `fs` shim stubs every syscall to ENOSYS)
// and, even when a master-URL-only override avoids the filesystem, never
// attaches a Bearer token (verified by reading
// k8s.io/client-go/tools/clientcmd/client_config.go's BuildConfigFromFlags
// and ClientConfigLoadingRules.Load -- ConfigOverrides.ClusterInfo only
// carries Server, never AuthInfo). So this package bypasses that path
// entirely: it builds a *rest.Config in memory (Host placeholder + Bearer
// token + a Transport that routes every call, including long-lived
// watches, through a Cloudflare service binding instead of a real socket)
// and feeds it directly into the same exported Setup/Config/Run/ApplyTo
// functions cmd/scheduler and cmd/controller-manager already call --
// those two binaries themselves stay untouched; this is new, WASM-only
// glue that duplicates only the handful of lines that would otherwise
// touch a kubeconfig file.
//
// Its own package, separate from pkg/controllers/pkg/controllers/gc:
// every *-wasm entrypoint needs RestConfig, but pkg/controllers and
// pkg/controllers/gc each carry a different, mutually irrelevant real
// upstream controller tree (six workload controllers vs. the garbage
// collector) -- sharing a package with either would link the other's
// reachable code into every binary regardless of whether it's ever
// called (Go's dead-code elimination works at function granularity, not
// well enough to prune an entire unrelated controller's worth of code
// out of a shared package -- confirmed live: pkg/controllers/gc.go
// briefly lived directly in pkg/controllers and measurably added ~4MB to
// the KCM binary despite RunControllerManager never calling
// RunGarbageCollector).
package restconfig

import (
	restclient "k8s.io/client-go/rest"

	cffetch "github.com/k8flare/k8flare/pkg/cfruntime/cloudflare/fetch"
)

// RestConfig builds an in-memory *rest.Config that authenticates with
// token and routes every request through the named Cloudflare service
// binding (see wrangler.jsonc's "services" list) instead of a real
// network socket -- the same binding-backed transport pattern verified in
// spikes/s8-wasm-resident (S8 FINDINGS.md's "Verified fix" for the
// outbound net/http crash, and the (d) service-binding-hop soak). Host is
// a placeholder: the binding's Fetcher determines the actual destination
// Worker, not DNS/TLS, but client-go still requires a well-formed,
// non-empty URL to build request paths against.
//
// Deliberately does not set any TLS-related rest.Config field (Insecure,
// CAData, etc.) -- client-go's transport.New refuses to combine a custom
// Transport with TLS options ("using a custom transport with TLS
// certificate options or the insecure flag is not allowed").
// basePath is the cluster's public URL path prefix ("" for the default
// cluster, "/c/<id>" for provisioned ones): every API path this config
// produces must carry it so the single consolidated Worker's cluster
// resolver routes the traffic to the right Cluster DO tree.
func RestConfig(bindingName, token, basePath string) *restclient.Config {
	client := cffetch.NewClient(cffetch.WithLiveBinding(bindingName))
	return &restclient.Config{
		Host:        "https://" + bindingName + ".k8flare.internal" + basePath + "/",
		BearerToken: token,
		Transport:   client.HTTPClient(cffetch.RedirectModeFollow).Transport,
		// Unset, client-go defaults to QPS=5/Burst=10 -- which
		// rate-limited every resident controller in this repo to five
		// API calls per second: the KCM took ~9s to create a 50-replica
		// RC's pods and the GC ~10s to strip 50 ownerReferences during
		// an orphan delete, which flaked the GC conformance canary.
		//
		// 20/30 (upstream kube-controller-manager's defaults), NOT the
		// 50/100 this briefly shipped with: at 50 the controllers' boot
		// informer storm (kcm 15 + gc ~30 LIST/WATCHes at once) LIVELOCKED
		// wrangler dev -- each API call instantiates the ~63MB apiserver
		// wasm per request (the per-request contract), the synchronous
		// WebAssembly.Instance data-segment copy monopolizes workerd's
		// single dev event loop (native-profiled: InstanceBuilder::
		// LoadDataSegments/memory_copy_wrapper hot), informers time out
		// and re-list, and the pile never drains -- reproduced
		// deterministically (node+deployment wedged, 100% CPU forever;
		// 20/30 completes the same flow in 4s; measured 2026-07-12).
		// Production's Loader doesn't share one event loop, but the same
		// instantiation stampede would still burn real CPU-time there.
		// At 20/30 the canary flow stays fast: 50-replica RC creation
		// 11s, full orphan handoff 11s (measured).
		QPS:   20,
		Burst: 30,
	}
}
