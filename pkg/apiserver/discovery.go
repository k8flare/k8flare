package apiserver

import (
	"encoding/json"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"

	"github.com/k8flare/k8flare/pkg/apiserver/apidef"
)

// APIResourcesForGroupVersion builds the metav1.APIResource list for a
// single GroupVersion from apidef.Table, including subresources (e.g.
// "pods/status") -- computed, not hand-listed, so a resource or subresource
// added to the table is automatically advertised without a second place to
// remember to update. Before this existed, apps/v1 and batch/v1's /status
// subresources were implemented in subresource.go but never advertised
// here -- exactly the drift this generalizes away.
func APIResourcesForGroupVersion(gv schema.GroupVersion) []metav1.APIResource {
	var out []metav1.APIResource
	for _, def := range apidef.ForGroupVersion(gv) {
		out = append(out, metav1.APIResource{
			Name:         def.Resource,
			SingularName: def.Singular,
			Namespaced:   def.Namespaced,
			Kind:         def.Kind,
			Verbs:        metav1.Verbs(def.EffectiveVerbs()),
			ShortNames:   def.ShortNames,
		})
		for _, sub := range def.Subresources {
			kind := sub.Kind
			if kind == "" {
				kind = def.Kind
			}
			out = append(out, metav1.APIResource{
				Name:       def.Resource + "/" + sub.Name,
				Namespaced: def.Namespaced,
				Kind:       kind,
				Verbs:      metav1.Verbs(sub.Verbs),
			})
		}
	}
	return out
}

// RegisterDiscovery registers the legacy core/v1 ("/api/...") discovery
// endpoints, plus /version and the health checks, on the given mux.
func RegisterDiscovery(mux *http.ServeMux) {
	mux.HandleFunc("GET /api", writeJSONHandler(metav1.APIVersions{
		TypeMeta: metav1.TypeMeta{Kind: "APIVersions"},
		Versions: []string{"v1"},
		ServerAddressByClientCIDRs: []metav1.ServerAddressByClientCIDR{
			{ClientCIDR: "0.0.0.0/0", ServerAddress: ""},
		},
	}))

	mux.HandleFunc("GET /api/v1", writeJSONHandler(metav1.APIResourceList{
		TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList"},
		GroupVersion: "v1",
		APIResources: APIResourcesForGroupVersion(corev1.SchemeGroupVersion),
	}))

	// Major/Minor/GitVersion are generated from go.mod's k8s.io/kubernetes
	// pin (zz_generated_version.go, cmd/k8flare-gen/version.go) so they
	// can't silently drift out of sync with the vendored API types the way
	// a hand-maintained literal did. The remaining fields aren't tied to
	// that pin and stay as static, honest placeholders (there is no real
	// build system stamping a git commit/date into this binary).
	mux.HandleFunc("GET /version", writeJSONHandler(version.Info{
		Major:        kubernetesMajor,
		Minor:        kubernetesMinor,
		GitVersion:   kubernetesGitVersion,
		GitCommit:    "",
		GitTreeState: "clean",
		BuildDate:    "2026-03-25T00:00:00Z",
		GoVersion:    "go1.26.1 js/wasm",
		Compiler:     "gc",
		Platform:     "js/wasm",
	}))

	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /livez", handleHealth)
	mux.HandleFunc("GET /readyz", handleHealth)
}

// RegisterGroupDiscovery registers /apis (the group list) and, for every
// non-core GroupVersion in apidef.Table, its /apis/{group} and
// /apis/{group}/{version} discovery documents.
func RegisterGroupDiscovery(mux *http.ServeMux) {
	var groups []metav1.APIGroup
	for _, gv := range apidef.GroupVersions() {
		if gv.Group == "" {
			continue // core/v1 is /api/v1, handled by RegisterDiscovery above
		}

		groupVersion := metav1.GroupVersionForDiscovery{GroupVersion: gv.String(), Version: gv.Version}
		groups = append(groups, metav1.APIGroup{
			TypeMeta:         metav1.TypeMeta{Kind: "APIGroup"},
			Name:             gv.Group,
			Versions:         []metav1.GroupVersionForDiscovery{groupVersion},
			PreferredVersion: groupVersion,
		})

		resourceList := metav1.APIResourceList{
			TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList"},
			GroupVersion: gv.String(),
			APIResources: APIResourcesForGroupVersion(gv),
		}
		mux.HandleFunc("GET /apis/"+gv.Group, writeJSONHandler(metav1.APIGroup{
			TypeMeta:         metav1.TypeMeta{Kind: "APIGroup"},
			Name:             gv.Group,
			Versions:         []metav1.GroupVersionForDiscovery{groupVersion},
			PreferredVersion: groupVersion,
		}))
		mux.HandleFunc("GET /apis/"+gv.Group+"/"+gv.Version, writeJSONHandler(resourceList))
	}

	// authorization.k8s.io/v1 isn't in apidef.Table -- selfsubjectaccessreviews
	// is a compute-on-request resource with no backing ResourceStore (see
	// selfsubjectaccessreview.go), so it doesn't fit apidef.ResourceDef's
	// New/NewList shape the way every other resource above does. Its
	// discovery documents are hand-written here instead, the one exception
	// to "the table is the only place to register a resource" this project
	// currently has.
	authGV := metav1.GroupVersionForDiscovery{GroupVersion: "authorization.k8s.io/v1", Version: "v1"}
	groups = append(groups, metav1.APIGroup{
		TypeMeta:         metav1.TypeMeta{Kind: "APIGroup"},
		Name:             "authorization.k8s.io",
		Versions:         []metav1.GroupVersionForDiscovery{authGV},
		PreferredVersion: authGV,
	})
	mux.HandleFunc("GET /apis/authorization.k8s.io", writeJSONHandler(metav1.APIGroup{
		TypeMeta:         metav1.TypeMeta{Kind: "APIGroup"},
		Name:             "authorization.k8s.io",
		Versions:         []metav1.GroupVersionForDiscovery{authGV},
		PreferredVersion: authGV,
	}))
	mux.HandleFunc("GET /apis/authorization.k8s.io/v1", writeJSONHandler(metav1.APIResourceList{
		TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList"},
		GroupVersion: "authorization.k8s.io/v1",
		APIResources: []metav1.APIResource{
			{Name: "selfsubjectaccessreviews", SingularName: "selfsubjectaccessreview", Namespaced: false, Kind: "SelfSubjectAccessReview", Verbs: metav1.Verbs{"create"}},
			{Name: "subjectaccessreviews", SingularName: "subjectaccessreview", Namespaced: false, Kind: "SubjectAccessReview", Verbs: metav1.Verbs{"create"}},
		},
	}))

	// authentication.k8s.io/v1: tokenreviews, the kubelet webhook token
	// authenticator's endpoint (tokenreview.go). Hand-written for the same
	// reason as authorization.k8s.io above.
	authnGV := metav1.GroupVersionForDiscovery{GroupVersion: "authentication.k8s.io/v1", Version: "v1"}
	groups = append(groups, metav1.APIGroup{
		TypeMeta:         metav1.TypeMeta{Kind: "APIGroup"},
		Name:             "authentication.k8s.io",
		Versions:         []metav1.GroupVersionForDiscovery{authnGV},
		PreferredVersion: authnGV,
	})
	mux.HandleFunc("GET /apis/authentication.k8s.io", writeJSONHandler(metav1.APIGroup{
		TypeMeta:         metav1.TypeMeta{Kind: "APIGroup"},
		Name:             "authentication.k8s.io",
		Versions:         []metav1.GroupVersionForDiscovery{authnGV},
		PreferredVersion: authnGV,
	}))
	mux.HandleFunc("GET /apis/authentication.k8s.io/v1", writeJSONHandler(metav1.APIResourceList{
		TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList"},
		GroupVersion: "authentication.k8s.io/v1",
		APIResources: []metav1.APIResource{
			{Name: "tokenreviews", SingularName: "tokenreview", Namespaced: false, Kind: "TokenReview", Verbs: metav1.Verbs{"create"}},
		},
	}))

	mux.HandleFunc("GET /apis", writeJSONHandler(metav1.APIGroupList{
		TypeMeta: metav1.TypeMeta{Kind: "APIGroupList", APIVersion: "v1"},
		Groups:   groups,
	}))
}

// RegisterOpenAPIDiscovery registers GET /openapi/v3, the one OpenAPI route
// this Go binary answers directly. Every other OpenAPI path --
// /openapi/v2, and each /openapi/v3/... per-group-version document -- is
// served straight out of workers/k8flare/assets/openapi/ as a Static
// Asset without ever reaching this binary (see wrangler.jsonc's "assets"
// config); the bare "/openapi/v3" discovery index can't join them there
// because it would need to be both a file and (for its children) a
// directory at the same path. See zz_generated_openapi.go and
// cmd/k8flare-gen/openapi.go for how OpenAPIV3Discovery is generated.
func RegisterOpenAPIDiscovery(mux *http.ServeMux) {
	mux.HandleFunc("GET /openapi/v3", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(OpenAPIV3Discovery)
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

// writeJSONHandler returns a handler that always serves v (computed once,
// at registration time -- every value passed to it here is a static
// discovery document, not per-request state).
func writeJSONHandler(v interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, v)
	}
}
