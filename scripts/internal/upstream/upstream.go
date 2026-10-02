package upstream

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const Module = "github.com/k3s-io/kubernetes"

var Version = mustVersionOf(Module)

type ServedGroupVersion struct {
	GV        string
	Resources []string
}

var Served = []ServedGroupVersion{
	{"v1", []string{"bindings", "componentstatuses", "configmaps", "endpoints", "events", "limitranges", "namespaces", "namespaces/finalize", "namespaces/status", "nodes", "nodes/proxy", "persistentvolumeclaims", "persistentvolumes", "pods", "pods/attach", "pods/binding", "pods/ephemeralcontainers", "pods/eviction", "pods/exec", "pods/log", "pods/portforward", "pods/proxy", "pods/resize", "podtemplates", "replicationcontrollers", "replicationcontrollers/scale", "resourcequotas", "secrets", "serviceaccounts", "serviceaccounts/token", "services", "services/proxy"}},
	{"events.k8s.io/v1", []string{"events"}},
	{"apps/v1", []string{"controllerrevisions", "daemonsets", "deployments", "deployments/scale", "replicasets", "replicasets/scale", "statefulsets", "statefulsets/scale"}},
	{"batch/v1", []string{"cronjobs", "jobs"}},
	{"autoscaling/v2", []string{"horizontalpodautoscalers"}},
	{"autoscaling/v1", []string{"horizontalpodautoscalers"}},
	{"policy/v1", []string{"poddisruptionbudgets"}},
	{"resource.k8s.io/v1", []string{"deviceclasses", "resourceclaims", "resourceclaimtemplates", "resourceslices"}},
	{"coordination.k8s.io/v1", []string{"leases"}},
	{"discovery.k8s.io/v1", []string{"endpointslices"}},
	{"node.k8s.io/v1", []string{"runtimeclasses"}},
	{"storage.k8s.io/v1", []string{"csidrivers", "csinodes", "csistoragecapacities", "storageclasses", "volumeattachments", "volumeattributesclasses"}},
	{"authentication.k8s.io/v1", []string{"tokenreviews", "selfsubjectreviews"}},
	{"authorization.k8s.io/v1", []string{"localsubjectaccessreviews", "selfsubjectaccessreviews", "selfsubjectrulesreviews", "subjectaccessreviews"}},
	{"rbac.authorization.k8s.io/v1", []string{"clusterrolebindings", "clusterroles", "rolebindings", "roles"}},
	{"admissionregistration.k8s.io/v1", []string{"mutatingadmissionpolicies", "mutatingadmissionpolicybindings", "mutatingwebhookconfigurations", "validatingwebhookconfigurations", "validatingadmissionpolicies", "validatingadmissionpolicybindings"}},
	{"scheduling.k8s.io/v1", []string{"priorityclasses"}},
	{"networking.k8s.io/v1", []string{"ingressclasses", "ingresses", "ipaddresses", "networkpolicies", "servicecidrs"}},
	{"certificates.k8s.io/v1", []string{"certificatesigningrequests", "certificatesigningrequests/approval"}},
	{"flowcontrol.apiserver.k8s.io/v1", []string{"flowschemas", "prioritylevelconfigurations"}},
	{"apiregistration.k8s.io/v1", []string{"apiservices"}},
}

type APIResource struct {
	Name         string   `json:"name"`
	SingularName string   `json:"singularName"`
	Namespaced   bool     `json:"namespaced"`
	Kind         string   `json:"kind"`
	Verbs        []string `json:"verbs"`
	ShortNames   []string `json:"shortNames"`
	Categories   []string `json:"categories"`
}

type APIResourceList struct {
	Resources []APIResource `json:"resources"`
}

func RepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func ModuleDir() (string, error) {
	cmd := exec.Command("go", "mod", "download", "-json", Module+"@"+Version)
	cmd.Dir = os.TempDir()
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GO111MODULE=on")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	var info struct{ Dir string }
	if err := json.Unmarshal(out, &info); err != nil {
		return "", err
	}
	return info.Dir, nil
}

func LoadDiscovery(dir, gv string) (*APIResourceList, error) {
	name := "apis__" + strings.ReplaceAll(gv, "/", "__") + ".json"
	if !strings.Contains(gv, "/") {
		name = "api__" + gv + ".json"
	}
	data, err := os.ReadFile(filepath.Join(dir, "api/discovery", name))
	if err != nil {
		return nil, err
	}
	var list APIResourceList
	return &list, json.Unmarshal(data, &list)
}

func APIPackage(gv string) (alias, path string) {
	group, version, ok := strings.Cut(gv, "/")
	if !ok {
		group, version = "core", gv
	}
	if group == "apiregistration.k8s.io" {
		return "apiregistration" + version, "k8s.io/kube-aggregator/pkg/apis/apiregistration/" + version
	}
	group = strings.TrimSuffix(group, ".k8s.io")
	return group + version, "k8s.io/api/" + group + "/" + version
}

func SchemeExternal(gv string) string {
	if strings.HasPrefix(gv, "apiregistration.k8s.io/") {
		_, version, _ := strings.Cut(gv, "/")
		return "k8s.io/kube-aggregator/pkg/apis/apiregistration/" + version
	}
	name := GroupName(gv)
	if name == "core" {
		return "k8s.io/api/core/v1"
	}
	_, version, ok := strings.Cut(gv, "/")
	if !ok {
		version = "v1"
	}
	return "k8s.io/api/" + name + "/" + version
}

func SchemeInternal(gv string) string {
	if strings.HasPrefix(gv, "apiregistration.k8s.io/") {
		return "k8s.io/kube-aggregator/pkg/apis/apiregistration"
	}
	name := GroupName(gv)
	if name == "core" {
		return "k8s.io/kubernetes/pkg/apis/core/v1"
	}
	_, version, ok := strings.Cut(gv, "/")
	if !ok {
		version = "v1"
	}
	return "k8s.io/kubernetes/pkg/apis/" + name + "/" + version
}

func GroupName(gv string) string {
	group, _, ok := strings.Cut(gv, "/")
	if !ok {
		return "core"
	}
	name, _, _ := strings.Cut(group, ".")
	return name
}
