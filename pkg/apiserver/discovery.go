package apiserver

import (
	"encoding/json"
	"net/http"
)

// APIResource describes a single API resource for discovery.
type APIResource struct {
	Name         string   `json:"name"`
	SingularName string   `json:"singularName"`
	Namespaced   bool     `json:"namespaced"`
	Kind         string   `json:"kind"`
	Verbs        []string `json:"verbs"`
	ShortNames   []string `json:"shortNames,omitempty"`
}

// apiVersions is the response for GET /api.
type apiVersions struct {
	Kind                         string                        `json:"kind"`
	Versions                     []string                      `json:"versions"`
	ServerAddressByClientCIDRs   []serverAddressByClientCIDR   `json:"serverAddressByClientCIDRs"`
}

type serverAddressByClientCIDR struct {
	ClientCIDR    string `json:"clientCIDR"`
	ServerAddress string `json:"serverAddress"`
}

// apiResourceList is the response for GET /api/v1.
type apiResourceList struct {
	Kind         string        `json:"kind"`
	GroupVersion string        `json:"groupVersion"`
	Resources    []APIResource `json:"resources"`
}

// apiGroupList is the response for GET /apis.
type apiGroupList struct {
	Kind       string        `json:"kind"`
	APIVersion string        `json:"apiVersion"`
	Groups     []interface{} `json:"groups"`
}

// versionInfo is the response for GET /version.
type versionInfo struct {
	Major        string `json:"major"`
	Minor        string `json:"minor"`
	GitVersion   string `json:"gitVersion"`
	GitCommit    string `json:"gitCommit"`
	GitTreeState string `json:"gitTreeState"`
	BuildDate    string `json:"buildDate"`
	GoVersion    string `json:"goVersion"`
	Compiler     string `json:"compiler"`
	Platform     string `json:"platform"`
}

// DefaultResources returns the default set of API resources that this server supports.
func DefaultResources() []APIResource {
	return []APIResource{
		{
			Name:         "namespaces",
			SingularName: "namespace",
			Namespaced:   false,
			Kind:         "Namespace",
			ShortNames:   []string{"ns"},
			Verbs:        []string{"create", "delete", "get", "list", "update", "watch"},
		},
		{
			Name:         "configmaps",
			SingularName: "configmap",
			Namespaced:   true,
			Kind:         "ConfigMap",
			ShortNames:   []string{"cm"},
			Verbs:        []string{"create", "delete", "get", "list", "update", "watch"},
		},
		{
			Name:         "secrets",
			SingularName: "secret",
			Namespaced:   true,
			Kind:         "Secret",
			Verbs:        []string{"create", "delete", "get", "list", "update", "watch"},
		},
		{
			Name:         "pods",
			SingularName: "pod",
			Namespaced:   true,
			Kind:         "Pod",
			ShortNames:   []string{"po"},
			Verbs:        []string{"create", "delete", "get", "list", "patch", "update", "watch"},
		},
		{
			Name:         "pods/status",
			SingularName: "",
			Namespaced:   true,
			Kind:         "Pod",
			Verbs:        []string{"get", "patch", "update"},
		},
		{
			Name:         "pods/binding",
			SingularName: "",
			Namespaced:   true,
			Kind:         "Binding",
			Verbs:        []string{"create"},
		},
		{
			Name:         "pods/log",
			SingularName: "",
			Namespaced:   true,
			Kind:         "Pod",
			Verbs:        []string{"get"},
		},
		{
			Name:         "pods/exec",
			SingularName: "",
			Namespaced:   true,
			Kind:         "Pod",
			Verbs:        []string{"create", "get"},
		},
		{
			Name:         "pods/attach",
			SingularName: "",
			Namespaced:   true,
			Kind:         "Pod",
			Verbs:        []string{"create", "get"},
		},
		{
			Name:         "nodes",
			SingularName: "node",
			Namespaced:   false,
			Kind:         "Node",
			ShortNames:   []string{"no"},
			Verbs:        []string{"create", "delete", "get", "list", "patch", "update", "watch"},
		},
		{
			Name:         "nodes/status",
			SingularName: "",
			Namespaced:   false,
			Kind:         "Node",
			Verbs:        []string{"get", "patch", "update"},
		},
		{
			Name:         "serviceaccounts",
			SingularName: "serviceaccount",
			Namespaced:   true,
			Kind:         "ServiceAccount",
			ShortNames:   []string{"sa"},
			Verbs:        []string{"create", "delete", "get", "list", "update", "watch"},
		},
		{
			Name:         "endpoints",
			SingularName: "endpoint",
			Namespaced:   true,
			Kind:         "Endpoints",
			ShortNames:   []string{"ep"},
			Verbs:        []string{"create", "delete", "get", "list", "update", "watch"},
		},
		{
			Name:         "services",
			SingularName: "service",
			Namespaced:   true,
			Kind:         "Service",
			ShortNames:   []string{"svc"},
			Verbs:        []string{"create", "delete", "get", "list", "update", "watch"},
		},
		{
			Name:         "events",
			SingularName: "event",
			Namespaced:   true,
			Kind:         "Event",
			ShortNames:   []string{"ev"},
			Verbs:        []string{"create", "delete", "get", "list", "patch", "update", "watch"},
		},
		{
			Name:         "limitranges",
			SingularName: "limitrange",
			Namespaced:   true,
			Kind:         "LimitRange",
			ShortNames:   []string{"limits"},
			Verbs:        []string{"create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
		},
		{
			Name:         "replicationcontrollers",
			SingularName: "replicationcontroller",
			Namespaced:   true,
			Kind:         "ReplicationController",
			ShortNames:   []string{"rc"},
			Verbs:        []string{"create", "delete", "get", "list", "patch", "update", "watch"},
		},
	}
}

// RegisterDiscovery registers Kubernetes API discovery endpoints on the given mux.
// The resources parameter defines which API resources are advertised in GET /api/v1.
func RegisterDiscovery(mux *http.ServeMux, resources []APIResource) {
	mux.HandleFunc("GET /api", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, apiVersions{
			Kind:     "APIVersions",
			Versions: []string{"v1"},
			ServerAddressByClientCIDRs: []serverAddressByClientCIDR{
				{ClientCIDR: "0.0.0.0/0", ServerAddress: ""},
			},
		})
	})

	mux.HandleFunc("GET /api/v1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, apiResourceList{
			Kind:         "APIResourceList",
			GroupVersion: "v1",
			Resources:    resources,
		})
	})

	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, versionInfo{
			Major:        "1",
			Minor:        "34",
			GitVersion:   "v0.1.0+k8flare",
			GitCommit:    "",
			GitTreeState: "clean",
			BuildDate:    "2026-03-25T00:00:00Z",
			GoVersion:    "go1.26.1 js/wasm",
			Compiler:     "gc",
			Platform:     "js/wasm",
		})
	})

	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /livez", handleHealth)
	mux.HandleFunc("GET /readyz", handleHealth)
}

// apiGroup describes a single API group for discovery.
type apiGroup struct {
	Name             string           `json:"name"`
	Versions         []groupVersion   `json:"versions"`
	PreferredVersion groupVersion     `json:"preferredVersion"`
}

// groupVersion describes a single version within an API group.
type groupVersion struct {
	GroupVersion string `json:"groupVersion"`
	Version      string `json:"version"`
}

// RegisterGroupDiscovery registers API group discovery endpoints on the given mux.
// This handles /apis (the group list) and individual group/version resource listings.
func RegisterGroupDiscovery(mux *http.ServeMux) {
	coordinationGroup := apiGroup{
		Name: "coordination.k8s.io",
		Versions: []groupVersion{
			{GroupVersion: "coordination.k8s.io/v1", Version: "v1"},
		},
		PreferredVersion: groupVersion{GroupVersion: "coordination.k8s.io/v1", Version: "v1"},
	}

	storageGroup := apiGroup{
		Name: "storage.k8s.io",
		Versions: []groupVersion{
			{GroupVersion: "storage.k8s.io/v1", Version: "v1"},
		},
		PreferredVersion: groupVersion{GroupVersion: "storage.k8s.io/v1", Version: "v1"},
	}

	nodeGroup := apiGroup{
		Name: "node.k8s.io",
		Versions: []groupVersion{
			{GroupVersion: "node.k8s.io/v1", Version: "v1"},
		},
		PreferredVersion: groupVersion{GroupVersion: "node.k8s.io/v1", Version: "v1"},
	}

	resourceGroup := apiGroup{
		Name: "resource.k8s.io",
		Versions: []groupVersion{
			{GroupVersion: "resource.k8s.io/v1", Version: "v1"},
		},
		PreferredVersion: groupVersion{GroupVersion: "resource.k8s.io/v1", Version: "v1"},
	}

	appsGroup := apiGroup{
		Name: "apps",
		Versions: []groupVersion{
			{GroupVersion: "apps/v1", Version: "v1"},
		},
		PreferredVersion: groupVersion{GroupVersion: "apps/v1", Version: "v1"},
	}

	policyGroup := apiGroup{
		Name: "policy",
		Versions: []groupVersion{
			{GroupVersion: "policy/v1", Version: "v1"},
		},
		PreferredVersion: groupVersion{GroupVersion: "policy/v1", Version: "v1"},
	}

	discoveryGroup := apiGroup{
		Name: "discovery.k8s.io",
		Versions: []groupVersion{
			{GroupVersion: "discovery.k8s.io/v1", Version: "v1"},
		},
		PreferredVersion: groupVersion{GroupVersion: "discovery.k8s.io/v1", Version: "v1"},
	}

	mux.HandleFunc("GET /apis", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, apiGroupList{
			Kind:       "APIGroupList",
			APIVersion: "v1",
			Groups:     []interface{}{coordinationGroup, storageGroup, nodeGroup, resourceGroup, appsGroup, policyGroup, discoveryGroup},
		})
	})

	mux.HandleFunc("GET /apis/coordination.k8s.io", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, coordinationGroup)
	})

	mux.HandleFunc("GET /apis/coordination.k8s.io/v1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, apiResourceList{
			Kind:         "APIResourceList",
			GroupVersion: "coordination.k8s.io/v1",
			Resources: []APIResource{
				{
					Name:         "leases",
					SingularName: "lease",
					Namespaced:   true,
					Kind:         "Lease",
					Verbs:        []string{"create", "delete", "get", "list", "patch", "update", "watch"},
				},
			},
		})
	})

	mux.HandleFunc("GET /apis/storage.k8s.io", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, storageGroup)
	})

	mux.HandleFunc("GET /apis/storage.k8s.io/v1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, apiResourceList{
			Kind:         "APIResourceList",
			GroupVersion: "storage.k8s.io/v1",
			Resources: []APIResource{
				{
					Name:         "csidrivers",
					SingularName: "csidriver",
					Namespaced:   false,
					Kind:         "CSIDriver",
					Verbs:        []string{"create", "delete", "get", "list", "patch", "update", "watch"},
				},
				{
					Name:         "csinodes",
					SingularName: "csinode",
					Namespaced:   false,
					Kind:         "CSINode",
					Verbs:        []string{"create", "delete", "get", "list", "patch", "update", "watch"},
				},
			},
		})
	})

	mux.HandleFunc("GET /apis/node.k8s.io", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, nodeGroup)
	})

	mux.HandleFunc("GET /apis/node.k8s.io/v1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, apiResourceList{
			Kind:         "APIResourceList",
			GroupVersion: "node.k8s.io/v1",
			Resources: []APIResource{
				{
					Name:         "runtimeclasses",
					SingularName: "runtimeclass",
					Namespaced:   false,
					Kind:         "RuntimeClass",
					Verbs:        []string{"create", "delete", "get", "list", "patch", "update", "watch"},
				},
			},
		})
	})

	mux.HandleFunc("GET /apis/resource.k8s.io", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, resourceGroup)
	})

	mux.HandleFunc("GET /apis/resource.k8s.io/v1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, apiResourceList{
			Kind:         "APIResourceList",
			GroupVersion: "resource.k8s.io/v1",
			Resources: []APIResource{
				{
					Name:         "resourceclaims",
					SingularName: "resourceclaim",
					Namespaced:   true,
					Kind:         "ResourceClaim",
					Verbs:        []string{"create", "delete", "get", "list", "patch", "update", "watch"},
				},
				{
					Name:         "resourceslices",
					SingularName: "resourceslice",
					Namespaced:   false,
					Kind:         "ResourceSlice",
					Verbs:        []string{"create", "delete", "get", "list", "patch", "update", "watch"},
				},
				{
					Name:         "deviceclasses",
					SingularName: "deviceclass",
					Namespaced:   false,
					Kind:         "DeviceClass",
					Verbs:        []string{"create", "delete", "get", "list", "patch", "update", "watch"},
				},
			},
		})
	})

	mux.HandleFunc("GET /apis/apps", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, appsGroup)
	})

	mux.HandleFunc("GET /apis/apps/v1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, apiResourceList{
			Kind:         "APIResourceList",
			GroupVersion: "apps/v1",
			Resources: []APIResource{
				{
					Name:         "replicasets",
					SingularName: "replicaset",
					Namespaced:   true,
					Kind:         "ReplicaSet",
					ShortNames:   []string{"rs"},
					Verbs:        []string{"create", "delete", "get", "list", "patch", "update", "watch"},
				},
				{
					Name:         "statefulsets",
					SingularName: "statefulset",
					Namespaced:   true,
					Kind:         "StatefulSet",
					ShortNames:   []string{"sts"},
					Verbs:        []string{"create", "delete", "get", "list", "patch", "update", "watch"},
				},
			},
		})
	})

	mux.HandleFunc("GET /apis/policy", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, policyGroup)
	})

	mux.HandleFunc("GET /apis/policy/v1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, apiResourceList{
			Kind:         "APIResourceList",
			GroupVersion: "policy/v1",
			Resources: []APIResource{
				{
					Name:         "poddisruptionbudgets",
					SingularName: "poddisruptionbudget",
					Namespaced:   true,
					Kind:         "PodDisruptionBudget",
					ShortNames:   []string{"pdb"},
					Verbs:        []string{"create", "delete", "get", "list", "patch", "update", "watch"},
				},
			},
		})
	})

	mux.HandleFunc("GET /apis/discovery.k8s.io", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, discoveryGroup)
	})

	mux.HandleFunc("GET /apis/discovery.k8s.io/v1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, apiResourceList{
			Kind:         "APIResourceList",
			GroupVersion: "discovery.k8s.io/v1",
			Resources: []APIResource{
				{
					Name:         "endpointslices",
					SingularName: "endpointslice",
					Namespaced:   true,
					Kind:         "EndpointSlice",
					Verbs:        []string{"create", "delete", "get", "list", "patch", "update", "watch"},
				},
			},
		})
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func writeJSON(w http.ResponseWriter, statusCode int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(v)
}
