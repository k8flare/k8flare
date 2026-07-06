//go:build js && wasm

// Package controllers hosts the WASM-resident kube-scheduler and
// kube-controller-manager glue for workers/controllers. See
// docs/platform-verification.md's S8 section for the execution shape this
// implements (DO-hosted + WaitUntil-resident + event-armed alarm() safety
// net) and spikes/s8-wasm-resident/FINDINGS.md for the underlying spike.
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
package controllers

import (
	restclient "k8s.io/client-go/rest"

	"github.com/syumai/workers/cloudflare"
	cffetch "github.com/syumai/workers/cloudflare/fetch"
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
	binding := cloudflare.GetBinding(bindingName)
	client := cffetch.NewClient(cffetch.WithBinding(binding))
	return &restclient.Config{
		Host:        "https://" + bindingName + ".k8flare.internal" + basePath + "/",
		BearerToken: token,
		Transport:   client.HTTPClient(cffetch.RedirectModeFollow).Transport,
	}
}
