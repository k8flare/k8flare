// Command mirror copies pinned upstream Go modules into .build/ and applies
// the overlays that make them build for GOOS=js. Root go.mod's replace
// directives point at the mirrors, so this must run before any Go build.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/k8flare/k8flare/scripts/internal/upstream"
)

type op struct {
	kind     string
	path     string
	astOps   []edit
	decls    fileEdit
	required []string
}

// hostOnly keeps the upstream file for every target but js.
func hostOnly(path string) op { return op{kind: "hostOnly", path: path} }

func narrowClientset(path string) op { return op{kind: "narrowClientset", path: path} }

func narrowInformerFactory(path string) op { return op{kind: "narrowInformerFactory", path: path} }

func narrowInformerGroups(dir string) op { return op{kind: "narrowInformerGroups", path: dir} }

type mirror struct {
	name    string
	module  string
	version string
	ops     []op
}

var mirrors = []mirror{
	{
		name:   "k3s",
		module: "github.com/k3s-io/k3s",
		ops: []op{
			patchAST("pkg/daemons/control/deps/deps.go",
				insertAtStart("KubeConfig", "if KubeConfigOverride != nil {\n\tif handled, err := KubeConfigOverride(dest, url, caCert, clientCert, clientKey); handled || err != nil {\n\t\treturn err\n\t}\n}"),
				appendDecls("k3s/append/kubeconfig_override.go"),
			),
			patchAST("pkg/agent/tunnel/tunnel.go",
				replaceCallArg("agentTunnel.connect", "remotedialer.ConnectToProxyWithDialer", 2, "tunnelHeaders()"),
				replaceNode("agentTunnel.watchEndpointSlices", "syncProxyAddresses(addresses)", "if !TunnelIgnoreEndpointSlices {\n\tsyncProxyAddresses(addresses)\n}"),
				appendDecls("k3s/append/tunnel_overrides.go"),
			),
		},
	},
	{
		name:   "remotedialer",
		module: "github.com/rancher/remotedialer",
		ops: []op{
			patchAST("session.go",
				addField("Session", "unconfirmedStale map[int64]bool"),
			),
			patchAST("session_sync.go",
				replaceNode("Session.compareAndCloseStaleConnections", "toClose := diffSortedSetsGetRemoved(serverIDs, clientIDs)", "missing := diffSortedSetsGetRemoved(serverIDs, clientIDs)"),
				replaceNode("Session.compareAndCloseStaleConnections", "if len(toClose) == 0 {\n\treturn\n}", ""),
				insertAfter("Session.compareAndCloseStaleConnections", "defer s.Unlock()", "var toClose []int64\nunconfirmed := map[int64]bool{}\nfor _, id := range missing {\n\tif s.unconfirmedStale[id] {\n\t\ttoClose = append(toClose, id)\n\t} else {\n\t\tunconfirmed[id] = true\n\t}\n}\ns.unconfirmedStale = unconfirmed"),
			),
			patchAST("server.go",
				appendDecls("remotedialer/append/serve_conn.go"),
			),
		},
	},
	{
		name:   "component-base",
		module: "github.com/k3s-io/kubernetes/staging/src/k8s.io/component-base",
		ops: []op{
			patchJSAST("tracing/utils.go",
				replaceBody("WrapperFor", "return func(rt http.RoundTripper) http.RoundTripper { return rt }"),
				keepOnly("TracerProvider", "noopTracerProvider", "noopTracerProvider.Shutdown", "NewNoopTracerProvider", "Propagators", "WrapperFor"),
			),
		},
	},
	{
		name:   "kubernetes",
		module: "github.com/k3s-io/kubernetes",
		ops: []op{
			keepDeclsJS("pkg/securitycontext/util_darwin.go", "possibleCPUs"),
			patchJSAST("pkg/util/filesystem/util_unix.go",
				replaceBody("IsUnixDomainSocket", "return false, fmt.Errorf(\"unix domain sockets are not available: %s\", filePath)"),
			),
			patchJSAST("pkg/scheduler/backend/cache/debugger/signal.go",
				addImport("", "os"),
				replaceSelector("syscall", "SIGUSR2", "os.Interrupt"),
			),
			hostOnly("pkg/scheduler/backend/queue/testing.go"),
			hostOnly("pkg/controller/certificates/cleaner/pcrcleaner.go"),
			hostOnly("plugin/pkg/auth/authorizer/node/graph_populator.go"),
			patchJSAST("plugin/pkg/auth/authorizer/node/node_authorizer.go",
				addImport("corev1", "k8s.io/api/core/v1"),
				addImport("resourcev1", "k8s.io/api/resource/v1"),
				replaceVarValue("configMapResource", `schema.GroupResource{Resource: "configmaps"}`),
				replaceVarValue("secretResource", `schema.GroupResource{Resource: "secrets"}`),
				replaceVarValue("podResource", `schema.GroupResource{Resource: "pods"}`),
				replaceVarValue("nodeResource", `schema.GroupResource{Resource: "nodes"}`),
				replaceVarValue("resourceSlice", `schema.GroupResource{Group: "resource.k8s.io", Resource: "resourceslices"}`),
				replaceVarValue("pvcResource", `schema.GroupResource{Resource: "persistentvolumeclaims"}`),
				replaceVarValue("pvResource", `schema.GroupResource{Resource: "persistentvolumes"}`),
				replaceVarValue("resourceClaimResource", `schema.GroupResource{Group: "resource.k8s.io", Resource: "resourceclaims"}`),
				replaceVarValue("vaResource", `schema.GroupResource{Group: "storage.k8s.io", Resource: "volumeattachments"}`),
				replaceVarValue("svcAcctResource", `schema.GroupResource{Resource: "serviceaccounts"}`),
				replaceVarValue("leaseResource", `schema.GroupResource{Group: "coordination.k8s.io", Resource: "leases"}`),
				replaceVarValue("csiNodeResource", `schema.GroupResource{Group: "storage.k8s.io", Resource: "csinodes"}`),
				replaceVarValue("pcrResource", `schema.GroupResource{Group: "certificates.k8s.io", Resource: "podcertificaterequests"}`),
				replaceSelector("api", "NamespaceNodeLease", "corev1.NamespaceNodeLease"),
				replaceSelector("resourceapi", "ResourceSliceSelectorNodeName", "resourcev1.ResourceSliceSelectorNodeName"),
			),
			patchJSAST("pkg/controller/nodeipam/node_ipam_controller.go",
				replaceSelector("cloudprovider", "Interface", "interface{}"),
			),
			patchJSAST("pkg/controller/nodeipam/nolegacyprovider.go",
				replaceSelector("cloudprovider", "Interface", "interface{}"),
			),
			patchJSAST("pkg/controller/nodeipam/ipam/cidr_allocator.go",
				replaceSelector("cloudprovider", "Interface", "interface{}"),
			),
			patchJSAST("plugin/pkg/auth/authorizer/rbac/bootstrappolicy/controller_policy.go",
				replaceSelector("legacytokentracking", "ConfigMapName", `"kube-apiserver-legacy-service-account-token-tracking"`),
			),
			patchJSAST("pkg/kubelet/types/types.go",
				replaceSelector("logs", "RFC3339NanoLenient", "\"2006-01-02T15:04:05.999999999Z07:00\""),
				replaceSelector("logs", "RFC3339NanoFixed", "\"2006-01-02T15:04:05.000000000Z07:00\""),
			),
			patchJSAST("pkg/controller/podgc/gc_controller.go",
				replaceNode("byEvictionAndCreationTimestamp.Less", "eviction.PodIsEvicted(o[i].Status)", `o[i].Status.Phase == v1.PodFailed && o[i].Status.Reason == "Evicted"`),
				replaceNode("byEvictionAndCreationTimestamp.Less", "eviction.PodIsEvicted(o[j].Status)", `o[j].Status.Phase == v1.PodFailed && o[j].Status.Reason == "Evicted"`),
			),
			patchAST("pkg/controller/tainteviction/timed_workers.go",
				addImport("", "k8s.io/client-go/util/workqueue"),
				insertAfter("createWorker", "worker.Timer = clock.AfterFunc(delay, wrapper)", "workqueue.ObserveDelay(delay)"),
			),
		},
	},
	{
		name:   "client-go",
		module: "github.com/k3s-io/kubernetes/staging/src/k8s.io/client-go",
		ops: []op{
			patchAST("util/workqueue/delaying_queue.go",
				insertAfter("delayingType.AddAfter", "q.metrics.retry()", "ObserveDelay(duration)"),
				appendDecls("client-go/append/delay_observer.go"),
			),
			patchJSAST("kubernetes/scheme/register.go",
				replaceVarValue("localSchemeBuilder", "runtime.SchemeBuilder{}"),
			),
			narrowClientset("kubernetes/clientset.go"),
			narrowInformerFactory("informers/factory.go"),
			requireDecls("informers/generic.go",
				"type GenericInformer interface { Informer() cache.SharedIndexInformer; Lister() cache.GenericLister }",
				"func (f *sharedInformerFactory) ForResource(resource schema.GroupVersionResource) (GenericInformer, error)",
			),
			hostOnly("informers/generic.go"),
			narrowInformerGroups("informers"),
			keepDeclsJS("util/certificate/csr/csr.go", "ExpirationSecondsToDuration"),
		},
	},
	{
		name:   "apiextensions",
		module: "github.com/k3s-io/kubernetes/staging/src/k8s.io/apiextensions-apiserver",
		ops: []op{
			keepDeclsJS("pkg/apiserver/apiserver.go", "Scheme", "Codecs", "unversionedVersion", "unversionedTypes", "init"),
			patchAST("pkg/apiserver/customresource_discovery.go",
				appendDecls("apiextensions/append/discovery_handlers.go"),
			),
		},
	},
	{
		name:   "apiserver",
		module: "github.com/k3s-io/kubernetes/staging/src/k8s.io/apiserver",
		ops: []op{
			hostOnly("pkg/storage/storagebackend/factory/etcd3.go"),
			stubFuncsJS("pkg/storage/storagebackend/factory/factory.go", []string{"DestroyFunc"},
				"errNoEtcd", "storagebackend/factory: etcd storage is not available in this build",
				"Create", "CreateHealthCheck", "CreateReadyCheck"),
			patchJSAST("pkg/storage/feature/feature_support_checker.go",
				replaceVarValue("DefaultFeatureSupportChecker", "noEtcd{}"),
				removeField("FeatureSupportChecker", "CheckClient"),
				keepOnly("DefaultFeatureSupportChecker", "FeatureSupportChecker"),
				appendDecls("apiserver/append/no_etcd_feature_support.go"),
			),
			patchJSAST("pkg/sharding/parser.go",
				replaceBody("Parse", "if expr == \"\" {\n\treturn nil, nil\n}\nreturn nil, fmt.Errorf(\"sharding: ShardSelector expressions are not supported in this build\")"),
				keepOnly("Parse"),
			),
			patchJSAST("pkg/storage/cacher/cache_watcher.go",
				replaceNode("cacheWatcher.process", "utilflowcontrol.WatchInitialized(ctx)", ""),
			),
			keepDeclsJS("pkg/server/filters/priority-and-fairness.go", "tooManyRequests"),
			patchJSAST("pkg/storageversion/manager.go",
				replaceSelector("kubernetes", "NewForConfig", "apiserverinternalv1alpha1.NewForConfig"),
				addImport("apiserverinternalv1alpha1", "k8s.io/client-go/kubernetes/typed/apiserverinternal/v1alpha1"),
				replaceNode("defaultManager.UpdateStorageVersions", "clientset.InternalV1alpha1().StorageVersions()", "clientset.StorageVersions()"),
			),
			patchAST("pkg/util/webhook/client.go",
				insertBefore("ClientManager.hookClientConfig", "delegateDialer := cfg.Dial", "if cfg.Transport != nil {\n\tif WebhookTransportSetup != nil {\n\t\tWebhookTransportSetup(cfg, serverName, cc.CABundle)\n\t}\n\tcfg.QPS = -1\n\tcfg.ContentConfig.NegotiatedSerializer = cm.negotiatedSerializer\n\tcfg.ContentConfig.ContentType = runtime.ContentTypeJSON\n\tcfg.TLSClientConfig = rest.TLSClientConfig{}\n\treturn cfg, nil\n}\n\n"),
				insertBeforeTopLevel("ClientManager.hookClientConfig", "return complete(cfg)", "if cfg.Transport != nil {\n\tcfg.QPS = -1\n\tcfg.ContentConfig.NegotiatedSerializer = cm.negotiatedSerializer\n\tcfg.ContentConfig.ContentType = runtime.ContentTypeJSON\n\tcfg.TLSClientConfig = rest.TLSClientConfig{}\n\treturn cfg, nil\n}\n\n"),
				appendDecls("apiserver/append/webhook_transport_setup.go"),
			),
			patchJSAST("pkg/util/webhook/authentication.go",
				replaceParamType("NewDefaultAuthenticationInfoResolverWrapper", "egressSelector", "any"),
				dropIfBranch("NewDefaultAuthenticationInfoResolverWrapper", "egressSelector != nil", "egressselector.ControlPlane"),
				dropIfBranch("NewDefaultAuthenticationInfoResolverWrapper", "egressSelector != nil", "egressselector.Cluster"),
			),
			patchJSAST("pkg/storage/storagebackend/config.go",
				removeField("TransportConfig", "EgressLookup"),
				replaceFieldType("Config", "LeaseManagerConfig", "LeaseManagerConfig"),
				replaceSelector("etcd3", "NewDefaultLeaseManagerConfig", "NewDefaultLeaseManagerConfig"),
				appendDecls("apiserver/append/lease_manager_config.go"),
			),
			patchAST("pkg/endpoints/installer.go",
				replaceNode("APIInstaller.registerResourceHandlers", "schema.GroupVersion{Group: fqKindToRegister.Group, Version: runtime.APIVersionInternal}", "hubGroupVersionFor(a.group.Typer, a.group.GroupVersion, fqKindToRegister)"),
				appendDecls("apiserver/append/hub_group_version.go"),
			),
		},
	},
	{
		name:   "mount-utils",
		module: "github.com/k3s-io/kubernetes/staging/src/k8s.io/mount-utils",
		ops: []op{
			patchJSAST("mount_helper_unix.go",
				replaceBody("IsCorruptedMnt", "return false"),
				replaceBody("PathExists", "_, err := os.Stat(path)\nif err == nil {\n\treturn true, nil\n}\nif errors.Is(err, fs.ErrNotExist) {\n\treturn false, nil\n}\nreturn false, err"),
				keepOnly("IsCorruptedMnt", "PathExists"),
			),
		},
	},
}

func main() {
	root, err := repoRoot()
	check(err)
	if len(os.Args) > 1 && os.Args[1] == "-check-keep" {
		check(checkKeep(root))
		return
	}
	for _, m := range mirrors {
		m.version, err = upstream.VersionOf(m.module)
		check(err)
		src, err := moduleDir(m.module, m.version)
		check(err)
		dst := filepath.Join(root, ".build", m.name+"-mirror")
		overlays := filepath.Join(root, "scripts/mirror/_overlays")
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
	switch o.kind {
	case "hostOnly":
		_, err := keepHostOnly(dst, o.path)
		return err
	case "patchAST", "patchJSAST":
		return applyAST(dst, overlays, o)
	case "astJS":
		return applyDeclsAST(dst, o)
	case "requireDecls":
		return applyRequireDecls(dst, o)
	case "narrowClientset", "narrowInformerFactory":
		data, err := keepHostOnly(dst, o.path)
		if err != nil {
			return err
		}
		var lean []byte
		if o.kind == "narrowClientset" {
			lean, err = leanClientset(data, keptGroupVersions(keptAPIs))
		} else {
			lean, err = leanInformerFactory(data, keptFactoryGroups(keptAPIs))
		}
		if err != nil {
			return fmt.Errorf("%s: %w", o.path, err)
		}
		return os.WriteFile(filepath.Join(dst, jsName(o.path)), lean, 0o644)
	case "narrowInformerGroups":
		for _, g := range keptAPIs {
			if !g.NarrowInformer {
				continue
			}
			rel := filepath.Join(o.path, g.Name, "interface.go")
			data, err := keepHostOnly(dst, rel)
			if err != nil {
				return err
			}
			lean, err := leanInformerGroup(data, g.Name, g.Versions)
			if err != nil {
				return fmt.Errorf("%s: %w", rel, err)
			}
			if err := os.WriteFile(filepath.Join(dst, jsName(rel)), lean, 0o644); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("unknown op %q", o.kind)
}
