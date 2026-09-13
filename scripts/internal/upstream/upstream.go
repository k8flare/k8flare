package upstream

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	Module  = "github.com/k3s-io/kubernetes"
	Version = "v1.36.4-k3s1"
)

type ServedGroupVersion struct {
	GV        string
	Resources []string
}

var Served = []ServedGroupVersion{
	{"v1", []string{"configmaps", "events", "namespaces", "nodes", "pods", "pods/binding", "pods/log", "replicationcontrollers", "secrets", "serviceaccounts", "services"}},
	{"apps/v1", []string{"replicasets", "statefulsets"}},
	{"policy/v1", []string{"poddisruptionbudgets"}},
	{"resource.k8s.io/v1", []string{"deviceclasses", "resourceclaims", "resourceclaimtemplates", "resourceslices"}},
	{"coordination.k8s.io/v1", []string{"leases"}},
	{"discovery.k8s.io/v1", []string{"endpointslices"}},
	{"node.k8s.io/v1", []string{"runtimeclasses"}},
	{"storage.k8s.io/v1", []string{"csidrivers", "csinodes"}},
	{"authentication.k8s.io/v1", []string{"tokenreviews"}},
	{"authorization.k8s.io/v1", []string{"selfsubjectaccessreviews", "subjectaccessreviews"}},
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
	group = strings.TrimSuffix(group, ".k8s.io")
	return group + version, "k8s.io/api/" + group + "/" + version
}
