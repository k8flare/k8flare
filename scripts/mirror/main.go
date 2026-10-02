// Command mirror copies pinned upstream Go modules into .build/ and applies
// the overlays that make them build for GOOS=js. Root go.mod's replace
// directives point at the mirrors, so this must run before any Go build.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type op struct {
	kind    string
	path    string
	overlay string
	from    string
	to      string
	text    string
	edits   []op
}

func patch(path, from, to string) op { return op{kind: "patch", path: path, from: from, to: to} }

// hostOnly keeps the upstream file for every target but js.
func hostOnly(path string) op { return op{kind: "hostOnly", path: path} }

// replaceJS keeps the upstream file for host builds and adds the overlay
// (which carries its own //go:build js constraint) beside it.
func replaceJS(path, overlay string) op { return op{kind: "replaceJS", path: path, overlay: overlay} }

func addJS(path, overlay string) op { return op{kind: "addJS", path: path, overlay: overlay} }

// patchJS keeps the upstream file for host builds and adds a js-only copy
// with the given replacements applied and text appended.
func patchJS(path string, edits []op) op { return op{kind: "patchJS", path: path, edits: edits} }

func appendText(path, text string) op { return op{kind: "append", path: path, text: text} }

type mirror struct {
	name    string
	module  string
	version string
	pins    []string
	ops     []op
}

var mirrors = []mirror{
	{
		name:    "k3s",
		module:  "github.com/k3s-io/k3s",
		version: "v1.36.5-0.20260821152713-4dedb15be780",
		pins:    []string{"pkg/daemons/control/deps/deps.go", "pkg/agent/tunnel/tunnel.go"},
		ops: []op{
			patch("pkg/daemons/control/deps/deps.go",
				"func KubeConfig(dest, url, caCert, clientCert, clientKey string) error {\n",
				"func KubeConfig(dest, url, caCert, clientCert, clientKey string) error {\n\tif KubeConfigOverride != nil {\n\t\tif handled, err := KubeConfigOverride(dest, url, caCert, clientCert, clientKey); handled || err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n"),
			appendText("pkg/daemons/control/deps/deps.go", `
// KubeConfigOverride lets an embedding program write the agent's
// kubeconfigs itself. k8flare's control plane sits behind a TLS terminator
// that never sees client certificates, so packages/agent writes bearer-token
// kubeconfigs instead of the certificate ones above. Added by scripts/mirror.
var KubeConfigOverride func(dest, url, caCert, clientCert, clientKey string) (handled bool, err error)
`),
			patch("pkg/agent/tunnel/tunnel.go",
				"import (\n\t\"context\"\n\t\"crypto/tls\"\n\t\"fmt\"\n\t\"net\"\n\t\"os\"\n\t\"strconv\"\n\t\"time\"\n",
				"import (\n\t\"context\"\n\t\"crypto/tls\"\n\t\"fmt\"\n\t\"net\"\n\t\"net/http\"\n\t\"os\"\n\t\"strconv\"\n\t\"time\"\n"),
			patch("pkg/agent/tunnel/tunnel.go",
				"err := remotedialer.ConnectToProxyWithDialer(ctx, wsURL, nil, auth, ws, a.dialContext, onConnect)",
				"err := remotedialer.ConnectToProxyWithDialer(ctx, wsURL, tunnelHeaders(), auth, ws, a.dialContext, onConnect)"),
			patch("pkg/agent/tunnel/tunnel.go",
				"\t\t\tsyncProxyAddresses(addresses)\n",
				"\t\t\tif !TunnelIgnoreEndpointSlices {\n\t\t\t\tsyncProxyAddresses(addresses)\n\t\t\t}\n"),
			appendText("pkg/agent/tunnel/tunnel.go", `
// TunnelHeaderOverride lets an embedding program authenticate the
// remotedialer connect request. k8flare's control plane never sees the
// agent's TLS client certificate, so packages/agent sends the node's
// bearer token here instead. Added by scripts/mirror.
var TunnelHeaderOverride func() http.Header
var TunnelIgnoreEndpointSlices bool

func tunnelHeaders() http.Header {
	if TunnelHeaderOverride != nil {
		return TunnelHeaderOverride()
	}
	return nil
}
`),
		},
	},
	{
		name:    "remotedialer",
		module:  "github.com/rancher/remotedialer",
		version: "v0.6.0-rc.1.0.20250916111157-f160aa32568d",
		pins:    []string{"server.go", "session.go", "session_sync.go"},
		ops: []op{
			patch("session.go",
				"\tclient           bool\n}",
				"\tclient           bool\n\tunconfirmedStale map[int64]bool\n}"),
			patch("session_sync.go",
				"\ttoClose := diffSortedSetsGetRemoved(serverIDs, clientIDs)\n\tif len(toClose) == 0 {\n\t\treturn\n\t}\n\n\ts.Lock()\n\tdefer s.Unlock()\n",
				"\tmissing := diffSortedSetsGetRemoved(serverIDs, clientIDs)\n\n\ts.Lock()\n\tdefer s.Unlock()\n\tvar toClose []int64\n\tunconfirmed := map[int64]bool{}\n\tfor _, id := range missing {\n\t\tif s.unconfirmedStale[id] {\n\t\t\ttoClose = append(toClose, id)\n\t\t} else {\n\t\t\tunconfirmed[id] = true\n\t\t}\n\t}\n\ts.unconfirmedStale = unconfirmed\n"),
			patch("server.go",
				"import (\n\t\"net/http\"\n\t\"sync\"\n\t\"time\"\n",
				"import (\n\t\"context\"\n\t\"math/rand\"\n\t\"net/http\"\n\t\"sync\"\n\t\"time\"\n"),
			appendText("server.go", `
// ServeConn registers clientKey's session from an already-open conn instead
// of upgrading an *http.Request, which is what a Durable Object needs: the
// socket comes from the Workers runtime's hibernatable WebSocket API, not
// from an http.Hijacker. It runs the session (as ServeHTTP does after its
// own upgrade) and removes it on exit. Added by scripts/mirror.
func (s *Server) ServeConn(clientKey string, conn wsConn) error {
	sessionKey := rand.Int63()
	session := newSession(sessionKey, clientKey, conn)
	session.auth = s.ClientConnectAuthorizer

	s.sessions.Lock()
	s.sessions.clients[clientKey] = append(s.sessions.clients[clientKey], session)
	for l := range s.sessions.listeners {
		l.sessionAdded(clientKey, session.sessionKey)
	}
	s.sessions.Unlock()

	defer s.sessions.remove(session)

	_, err := session.Serve(context.Background())
	return err
}
`),
		},
	},
	{
		name:    "component-base",
		module:  "github.com/k3s-io/kubernetes/staging/src/k8s.io/component-base",
		version: "v1.36.4-k3s1",
		pins:    []string{"tracing/utils.go"},
		ops: []op{
			replaceJS("tracing/utils.go", "component-base/tracing_utils.go"),
		},
	},
	{
		name:    "kubernetes",
		module:  "github.com/k3s-io/kubernetes",
		version: "v1.36.4-k3s1",
		pins: []string{
			"pkg/scheduler/backend/cache/debugger/signal.go",
			"pkg/scheduler/backend/queue/testing.go",
			"pkg/kubelet/types/types.go",
			"pkg/scheduler/backend/queue/scheduling_queue.go",
			"plugin/pkg/auth/authorizer/rbac/bootstrappolicy/controller_policy.go",
			"pkg/controller/nodeipam/node_ipam_controller.go",
			"pkg/controller/nodeipam/nolegacyprovider.go",
			"pkg/controller/nodeipam/ipam/cidr_allocator.go",
			"pkg/controller/podgc/gc_controller.go",
			"pkg/controller/certificates/cleaner/pcrcleaner.go",
		},
		ops: []op{
			addJS("pkg/securitycontext/util_js.go", "kubernetes/securitycontext_cpus.go"),
			addJS("pkg/util/filesystem/util_js.go", "kubernetes/filesystem_js.go"),
			replaceJS("pkg/scheduler/backend/cache/debugger/signal.go", "kubernetes/signal.go"),
			hostOnly("pkg/scheduler/backend/queue/testing.go"),
			hostOnly("pkg/controller/certificates/cleaner/pcrcleaner.go"),
			patchJS("pkg/controller/nodeipam/node_ipam_controller.go", []op{
				patch("", "\tcloudprovider \"k8s.io/cloud-provider\"\n", ""),
				patch("", "cloud                cloudprovider.Interface", "cloud                interface{}"),
				patch("", "cloud cloudprovider.Interface,", "cloud interface{},"),
			}),
			patchJS("pkg/controller/nodeipam/nolegacyprovider.go", []op{
				patch("", "\tcloudprovider \"k8s.io/cloud-provider\"\n", ""),
				patch("", "cloudprovider.Interface", "interface{}"),
			}),
			patchJS("pkg/controller/nodeipam/ipam/cidr_allocator.go", []op{
				patch("", "\tcloudprovider \"k8s.io/cloud-provider\"\n", ""),
				patch("", "cloudprovider.Interface", "interface{}"),
			}),
			patchJS("plugin/pkg/auth/authorizer/rbac/bootstrappolicy/controller_policy.go", []op{
				patch("", "\t\"k8s.io/kubernetes/pkg/controlplane/controller/legacytokentracking\"\n", ""),
				patch("", "legacytokentracking.ConfigMapName", `"kube-apiserver-legacy-service-account-token-tracking"`),
			}),
			patchJS("pkg/kubelet/types/types.go", []op{
				patch("", "\t\"k8s.io/cri-client/pkg/logs\"\n", ""),
				patch("", "logs.RFC3339NanoLenient", "\"2006-01-02T15:04:05.999999999Z07:00\""),
				patch("", "logs.RFC3339NanoFixed", "\"2006-01-02T15:04:05.000000000Z07:00\""),
			}),
			patchJS("pkg/controller/podgc/gc_controller.go", []op{
				patch("", "\t\"k8s.io/kubernetes/pkg/kubelet/eviction\"\n", ""),
				patch("", "iEvicted, jEvicted := eviction.PodIsEvicted(o[i].Status), eviction.PodIsEvicted(o[j].Status)",
					`iEvicted, jEvicted := o[i].Status.Phase == v1.PodFailed && o[i].Status.Reason == "Evicted", o[j].Status.Phase == v1.PodFailed && o[j].Status.Reason == "Evicted"`),
			}),
			patch("pkg/controller/tainteviction/timed_workers.go",
				"\t\"k8s.io/klog/v2\"\n",
				"\t\"k8s.io/client-go/util/workqueue\"\n\t\"k8s.io/klog/v2\"\n"),
			patch("pkg/controller/tainteviction/timed_workers.go",
				"\tworker.Timer = clock.AfterFunc(delay, wrapper)\n",
				"\tworker.Timer = clock.AfterFunc(delay, wrapper)\n\tworkqueue.ObserveDelay(delay)\n"),
		},
	},
	{
		name:    "client-go",
		module:  "github.com/k3s-io/kubernetes/staging/src/k8s.io/client-go",
		version: "v1.36.4-k3s1",
		pins: []string{
			"kubernetes/scheme/register.go",
			"kubernetes/clientset.go",
			"informers/factory.go",
			"informers/generic.go",
			"informers/admissionregistration/interface.go",
			"informers/apps/interface.go",
			"informers/batch/interface.go",
			"informers/coordination/interface.go",
			"informers/discovery/interface.go",
			"informers/policy/interface.go",
			"informers/resource/interface.go",
			"informers/scheduling/interface.go",
			"informers/storage/interface.go",
		},
		ops: []op{
			patch("util/workqueue/delaying_queue.go",
				"\tq.metrics.retry()\n",
				"\tq.metrics.retry()\n\tObserveDelay(duration)\n"),
			appendText("util/workqueue/delaying_queue.go", `
var DelayObserver func(delay time.Duration)

func ObserveDelay(delay time.Duration) {
	if delay > 0 && DelayObserver != nil {
		DelayObserver(delay)
	}
}
`),
			replaceJS("kubernetes/scheme/register.go", "client-go/register.go"),
			replaceJS("kubernetes/clientset.go", "client-go/kubernetes/clientset.go"),
			replaceJS("informers/factory.go", "client-go/informers/factory.go"),
			hostOnly("informers/generic.go"),
			replaceJS("informers/admissionregistration/interface.go", "client-go/informers/admissionregistration/interface.go"),
			replaceJS("informers/apps/interface.go", "client-go/informers/apps/interface.go"),
			replaceJS("informers/batch/interface.go", "client-go/informers/batch/interface.go"),
			replaceJS("informers/coordination/interface.go", "client-go/informers/coordination/interface.go"),
			replaceJS("informers/discovery/interface.go", "client-go/informers/discovery/interface.go"),
			replaceJS("informers/policy/interface.go", "client-go/informers/policy/interface.go"),
			replaceJS("informers/resource/interface.go", "client-go/informers/resource/interface.go"),
			replaceJS("informers/scheduling/interface.go", "client-go/informers/scheduling/interface.go"),
			replaceJS("informers/storage/interface.go", "client-go/informers/storage/interface.go"),
			keepDeclsJS("util/certificate/csr/csr.go", "ExpirationSecondsToDuration"),
		},
	},
	{
		name:    "apiextensions",
		module:  "github.com/k3s-io/kubernetes/staging/src/k8s.io/apiextensions-apiserver",
		version: "v1.36.4-k3s1",
		pins: []string{
			"pkg/apiserver/customresource_discovery.go",
		},
		ops: []op{
			keepDeclsJS("pkg/apiserver/apiserver.go", "Scheme", "Codecs", "unversionedVersion", "unversionedTypes", "init"),
			appendText("pkg/apiserver/customresource_discovery.go", `
func NewDiscoveryHandlers(delegate http.Handler) (*versionDiscoveryHandler, *groupDiscoveryHandler) {
	return &versionDiscoveryHandler{discovery: map[schema.GroupVersion]*discovery.APIVersionHandler{}, delegate: delegate},
		&groupDiscoveryHandler{discovery: map[string]*discovery.APIGroupHandler{}, delegate: delegate}
}
`),
		},
	},
	{
		name:    "apiserver",
		module:  "github.com/k3s-io/kubernetes/staging/src/k8s.io/apiserver",
		version: "v1.36.4-k3s1",
		pins: []string{
			"pkg/util/webhook/authentication.go",
			"pkg/util/webhook/client.go",
			"pkg/storage/cacher/cache_watcher.go",
			"pkg/storageversion/manager.go",
			"pkg/storage/storagebackend/config.go",
			"pkg/storage/feature/feature_support_checker.go",
			"pkg/sharding/parser.go",
			"pkg/endpoints/installer.go",
		},
		ops: []op{
			hostOnly("pkg/storage/storagebackend/factory/etcd3.go"),
			stubFuncsJS("pkg/storage/storagebackend/factory/factory.go", []string{"DestroyFunc"},
				"errNoEtcd", "storagebackend/factory: etcd storage is not available in this build",
				"Create", "CreateHealthCheck", "CreateReadyCheck"),
			replaceJS("pkg/storage/feature/feature_support_checker.go", "apiserver/feature_support_checker.go"),
			replaceJS("pkg/sharding/parser.go", "apiserver/sharding_parser.go"),
			patchJS("pkg/storage/cacher/cache_watcher.go", []op{
				patch("", "\tutilflowcontrol \"k8s.io/apiserver/pkg/util/flowcontrol\"\n", ""),
				patch("", "\tutilflowcontrol.WatchInitialized(ctx)\n", ""),
			}),
			keepDeclsJS("pkg/server/filters/priority-and-fairness.go", "tooManyRequests"),
			patchJS("pkg/storageversion/manager.go", []op{
				patch("", "\t\"k8s.io/client-go/kubernetes\"\n", "\tapiserverinternalv1alpha1 \"k8s.io/client-go/kubernetes/typed/apiserverinternal/v1alpha1\"\n"),
				patch("", "clientset, err := kubernetes.NewForConfig(kubeAPIServerClientConfig)", "clientset, err := apiserverinternalv1alpha1.NewForConfig(kubeAPIServerClientConfig)"),
				patch("", "sc := clientset.InternalV1alpha1().StorageVersions()", "sc := clientset.StorageVersions()"),
			}),
			patch("pkg/util/webhook/client.go",
				"		if len(cfg.TLSClientConfig.ServerName) == 0 {\n			cfg.TLSClientConfig.ServerName = serverName\n		}\n\n		delegateDialer := cfg.Dial\n",
				"		if len(cfg.TLSClientConfig.ServerName) == 0 {\n			cfg.TLSClientConfig.ServerName = serverName\n		}\n\n		if cfg.Transport != nil {\n			if WebhookTransportSetup != nil {\n				WebhookTransportSetup(cfg, serverName, cc.CABundle)\n			}\n			cfg.QPS = -1\n			cfg.ContentConfig.NegotiatedSerializer = cm.negotiatedSerializer\n			cfg.ContentConfig.ContentType = runtime.ContentTypeJSON\n			cfg.TLSClientConfig = rest.TLSClientConfig{}\n			return cfg, nil\n		}\n\n		delegateDialer := cfg.Dial\n"),
			patch("pkg/util/webhook/client.go",
				"	if !isLocalHost(u) {\n		cfg.NextProtos = []string{\"http/1.1\"}\n	}\n\n	return complete(cfg)\n}\n",
				"	if !isLocalHost(u) {\n		cfg.NextProtos = []string{\"http/1.1\"}\n	}\n\n	if cfg.Transport != nil {\n		cfg.QPS = -1\n		cfg.ContentConfig.NegotiatedSerializer = cm.negotiatedSerializer\n		cfg.ContentConfig.ContentType = runtime.ContentTypeJSON\n		cfg.TLSClientConfig = rest.TLSClientConfig{}\n		return cfg, nil\n	}\n\n	return complete(cfg)\n}\n"),
			appendText("pkg/util/webhook/client.go", `
// WebhookTransportSetup, when set, lets an embedding program replace
// net.Dial for webhook Service backends (GOOS=js cannot dial ClusterIPs).
var WebhookTransportSetup func(cfg *rest.Config, serverName string, ca []byte)
`),
			patchJS("pkg/util/webhook/authentication.go", []op{
				patch("", "\tegressselector \"k8s.io/apiserver/pkg/server/egressselector\"\n", ""),
				patch("", "\tutilnet \"k8s.io/apimachinery/pkg/util/net\"\n", ""),
				patch("", "\tegressSelector *egressselector.EgressSelector,\n", "\tegressSelector any,\n"),
				patch("", `
				if egressSelector != nil {
					networkContext := egressselector.ControlPlane.AsNetworkContext()
					var egressDialer utilnet.DialFunc
					egressDialer, err = egressSelector.Lookup(networkContext)

					if err != nil {
						return nil, err
					}

					ret.Dial = egressDialer
				}
`, ""),
				patch("", `
				if egressSelector != nil {
					networkContext := egressselector.Cluster.AsNetworkContext()
					var egressDialer utilnet.DialFunc
					egressDialer, err = egressSelector.Lookup(networkContext)
					if err != nil {
						return nil, err
					}

					ret.Dial = egressDialer
				} else if proxyTransport != nil && proxyTransport.DialContext != nil {
`, `
				if proxyTransport != nil && proxyTransport.DialContext != nil {
`),
			}),
			patchJS("pkg/storage/storagebackend/config.go", []op{
				patch("", "\t\"k8s.io/apiserver/pkg/server/egressselector\"\n", ""),
				patch("", "\t\"k8s.io/apiserver/pkg/storage/etcd3\"\n", ""),
				patch("", "\tEgressLookup egressselector.Lookup\n", ""),
				patch("", "\tLeaseManagerConfig etcd3.LeaseManagerConfig\n", "\tLeaseManagerConfig LeaseManagerConfig\n"),
				patch("", "etcd3.NewDefaultLeaseManagerConfig()", "NewDefaultLeaseManagerConfig()"),
				appendText("", `
// Local copy of etcd3.LeaseManagerConfig so that this package does not link
// the etcd3 storage implementation and the etcd client, which do not build
// for GOOS=js. Added by scripts/mirror.
type LeaseManagerConfig struct {
	ReuseDurationSeconds int64
	MaxObjectCount       int64
}

func NewDefaultLeaseManagerConfig() LeaseManagerConfig {
	return LeaseManagerConfig{ReuseDurationSeconds: 60, MaxObjectCount: 1000}
}
`),
			}),
			patch("pkg/endpoints/installer.go",
				"\t\tHubGroupVersion: schema.GroupVersion{Group: fqKindToRegister.Group, Version: runtime.APIVersionInternal},",
				"\t\tHubGroupVersion: hubGroupVersionFor(a.group.Typer, a.group.GroupVersion, fqKindToRegister),"),
			appendText("pkg/endpoints/installer.go", `
// hubGroupVersionFor is the version a PATCH body is decoded to before the
// merge is applied: the internal version when the scheme has one, the served
// version otherwise. This apiserver registers external types only, and
// upstream hardcodes the internal hub. Added by scripts/mirror.
func hubGroupVersionFor(typer runtime.ObjectTyper, served schema.GroupVersion, kind schema.GroupVersionKind) schema.GroupVersion {
	internal := schema.GroupVersion{Group: kind.Group, Version: runtime.APIVersionInternal}
	if typer != nil && typer.Recognizes(internal.WithKind(kind.Kind)) {
		return internal
	}
	return served
}
`),
		},
	},
	{
		name:    "mount-utils",
		module:  "github.com/k3s-io/kubernetes/staging/src/k8s.io/mount-utils",
		version: "v1.36.4-k3s1",
		pins:    []string{"mount_helper_unix.go"},
		ops: []op{
			replaceJS("mount_helper_unix.go", "mount-utils/mount_helper_unix.go"),
		},
	},
}

func main() {
	writePins := len(os.Args) > 1 && os.Args[1] == "-write-pins"
	root, err := repoRoot()
	check(err)
	for _, m := range mirrors {
		src, err := moduleDir(m.module, m.version)
		check(err)
		dst := filepath.Join(root, ".build", m.name+"-mirror")
		overlays := filepath.Join(root, "scripts/mirror/_overlays")
		for _, rel := range m.pins {
			check(checkPin(src, rel, filepath.Join(overlays, m.name, pinName(rel)), writePins))
		}
		check(os.RemoveAll(dst))
		check(copyTree(src, dst))
		for _, o := range m.ops {
			check(apply(dst, overlays, o))
		}
		fmt.Printf("mirror: %s %s@%s -> %s\n", m.name, m.module, m.version, dst)
	}
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "mirror:", err)
		os.Exit(1)
	}
}

func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func moduleDir(module, version string) (string, error) {
	cmd := exec.Command("go", "mod", "download", "-json", module+"@"+version)
	cmd.Dir = os.TempDir()
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GO111MODULE=on")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go mod download %s@%s: %w", module, version, err)
	}
	var info struct{ Dir string }
	if err := json.Unmarshal(out, &info); err != nil {
		return "", err
	}
	if info.Dir == "" {
		return "", fmt.Errorf("go mod download %s@%s: no Dir", module, version)
	}
	return info.Dir, nil
}

func pinName(rel string) string {
	return "upstream-" + strings.ReplaceAll(rel, "/", "-") + ".sha256"
}

func checkPin(src, rel, pinFile string, write bool) error {
	data, err := os.ReadFile(filepath.Join(src, rel))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if write {
		return os.WriteFile(pinFile, []byte(got+"\n"), 0o644)
	}
	want, err := os.ReadFile(pinFile)
	if err != nil {
		return fmt.Errorf("%s: no pin (run with -write-pins after reviewing the overlay): %w", rel, err)
	}
	if strings.TrimSpace(string(want)) != got {
		return fmt.Errorf("%s changed upstream (pin %s, got %s): review the overlay, then refresh the pin", rel, strings.TrimSpace(string(want)), got)
	}
	return nil
}

func copyTree(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return os.CopyFS(dst, os.DirFS(src))
}

const hostTag = "//go:build !js\n\n"
const jsTag = "//go:build js\n\n"

func jsName(path string) string {
	return strings.TrimSuffix(path, ".go") + "_js.go"
}

// keepHostOnly constrains the upstream file to every target but js and
// returns its original content.
func keepHostOnly(dst, path string) ([]byte, error) {
	target := filepath.Join(dst, path)
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}
	tagged := append([]byte(hostTag), data...)
	if i := bytes.Index(data, []byte("//go:build ")); i >= 0 {
		end := i + bytes.IndexByte(data[i:], '\n')
		expr := string(data[i+len("//go:build ") : end])
		tagged = append(append(append([]byte{}, data[:i]...), []byte("//go:build ("+expr+") && !js")...), data[end:]...)
	}
	return data, os.WriteFile(target, tagged, 0o644)
}

func apply(dst, overlays string, o op) error {
	target := filepath.Join(dst, o.path)
	switch o.kind {
	case "hostOnly":
		_, err := keepHostOnly(dst, o.path)
		return err
	case "addJS":
		data, err := os.ReadFile(filepath.Join(overlays, o.overlay))
		if err != nil {
			return err
		}
		if !bytes.HasPrefix(data, []byte("//go:build js")) {
			return fmt.Errorf("%s: overlay must start with a //go:build js constraint", o.overlay)
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dst, o.path)), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, o.path), data, 0o644)
	case "replaceJS":
		if _, err := keepHostOnly(dst, o.path); err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(overlays, o.overlay))
		if err != nil {
			return err
		}
		if !bytes.HasPrefix(data, []byte("//go:build js")) {
			return fmt.Errorf("%s: overlay must start with a //go:build js constraint", o.overlay)
		}
		return os.WriteFile(filepath.Join(dst, jsName(o.path)), data, 0o644)
	case "astJS":
		return applyAST(dst, o)
	case "patchJS":
		data, err := keepHostOnly(dst, o.path)
		if err != nil {
			return err
		}
		for _, e := range o.edits {
			switch e.kind {
			case "patch":
				if !bytes.Contains(data, []byte(e.from)) {
					return fmt.Errorf("%s no longer contains the text this patch replaces:\n%s", o.path, e.from)
				}
				data = bytes.Replace(data, []byte(e.from), []byte(e.to), 1)
			case "append":
				data = append(data, e.text...)
			}
		}
		return os.WriteFile(filepath.Join(dst, jsName(o.path)), append([]byte(jsTag), data...), 0o644)
	case "patch":
		data, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		if !bytes.Contains(data, []byte(o.from)) {
			return fmt.Errorf("%s no longer contains the text this patch replaces:\n%s", o.path, o.from)
		}
		return os.WriteFile(target, bytes.Replace(data, []byte(o.from), []byte(o.to), 1), 0o644)
	case "append":
		f, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteString(o.text)
		return err
	}
	return fmt.Errorf("unknown op %q", o.kind)
}
