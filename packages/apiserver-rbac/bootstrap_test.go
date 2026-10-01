package rbac

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/k8flare/k8flare/packages/apiserver-registry/registrytest"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac/bootstrappolicy"
)

func TestUpstreamRBACBootstrapContainsDefaults(t *testing.T) {
	roles := append(bootstrappolicy.ClusterRoles(), bootstrappolicy.ControllerRoles()...)
	want := []string{"cluster-admin", "admin", "edit", "view", "system:discovery", "system:basic-user", "system:public-info-viewer", "system:node"}
	byName := map[string]bool{}
	for _, cr := range roles {
		if cr.Name == "" || byName[cr.Name] {
			t.Fatalf("clusterrole name %q", cr.Name)
		}
		if cr.Labels["kubernetes.io/bootstrapping"] != "rbac-defaults" {
			t.Fatalf("clusterrole %s labels = %v", cr.Name, cr.Labels)
		}
		byName[cr.Name] = true
	}
	for _, name := range want {
		if !byName[name] {
			t.Fatalf("missing clusterrole %s", name)
		}
	}
	bindings := append(bootstrappolicy.ClusterRoleBindings(), bootstrappolicy.ControllerRoleBindings()...)
	bound := map[string]bool{}
	for _, crb := range bindings {
		if crb.Name == "" || bound[crb.Name] {
			t.Fatalf("clusterrolebinding name %q", crb.Name)
		}
		bound[crb.Name] = true
	}
	if !bound["cluster-admin"] || !bound["system:discovery"] || !bound["system:basic-user"] {
		t.Fatalf("bindings = %v", bound)
	}
}


func TestBootstrapRBAC_Lifecycle(t *testing.T) {
	cs := registrytest.NewCountingStore(t)
	gv := rbacv1.SchemeGroupVersion
	cr := cs.NewStore(t, gv, metav1.APIResource{Name: "clusterroles", Kind: "ClusterRole", SingularName: "clusterrole"})
	crb := cs.NewStore(t, gv, metav1.APIResource{Name: "clusterrolebindings", Kind: "ClusterRoleBinding", SingularName: "clusterrolebinding"})
	r := cs.NewStore(t, gv, metav1.APIResource{Name: "roles", Kind: "Role", SingularName: "role", Namespaced: true})
	rb := cs.NewStore(t, gv, metav1.APIResource{Name: "rolebindings", Kind: "RoleBinding", SingularName: "rolebinding", Namespaced: true})

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	// (1) First load creates the bootstrap objects and writes the marker
	handler1 := bootstrapRBAC(cr, crb, r, rb, next)
	req1 := httptest.NewRequest("GET", "/apis/rbac.authorization.k8s.io/v1/clusterroles", nil)
	rec1 := httptest.NewRecorder()
	handler1.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first load status = %d", rec1.Code)
	}

	firstPuts := cs.RegistryPuts.Load()
	if firstPuts == 0 {
		t.Fatal("first load performed no object creates")
	}
	marker, ok := cs.GetMarker("rbac")
	if !ok || marker == "" {
		t.Fatal("first load did not write rbac marker")
	}

	// (2) Second load against the same store performs NO create calls and exactly one marker read
	cs.RegistryPuts.Store(0)
	cs.MarkerGets.Store(0)
	handler2 := bootstrapRBAC(cr, crb, r, rb, next)
	req2 := httptest.NewRequest("GET", "/apis/rbac.authorization.k8s.io/v1/clusterroles", nil)
	rec2 := httptest.NewRecorder()
	handler2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second load status = %d", rec2.Code)
	}

	if puts := cs.RegistryPuts.Load(); puts != 0 {
		t.Fatalf("second load performed %d create calls, want 0", puts)
	}
	if gets := cs.MarkerGets.Load(); gets != 1 {
		t.Fatalf("second load performed %d marker reads, want 1", gets)
	}

	// (3) A marker with a different hash triggers ensure again
	cs.SetMarker("rbac", "outdated-hash")
	cs.RegistryPuts.Store(0)
	handler3 := bootstrapRBAC(cr, crb, r, rb, next)
	req3 := httptest.NewRequest("GET", "/apis/rbac.authorization.k8s.io/v1/clusterroles", nil)
	rec3 := httptest.NewRecorder()
	handler3.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("hash change status = %d", rec3.Code)
	}

	if puts := cs.RegistryPuts.Load(); puts == 0 {
		t.Fatal("hash change did not trigger ensure")
	}
	newMarker, ok := cs.GetMarker("rbac")
	if !ok || newMarker == "outdated-hash" {
		t.Fatalf("hash change did not update marker, got %q", newMarker)
	}
}

func TestBootstrapRBAC_FailedEnsureDoesNotWriteMarker(t *testing.T) {
	cs := registrytest.NewCountingStore(t)
	gv := rbacv1.SchemeGroupVersion
	cr := cs.NewStore(t, gv, metav1.APIResource{Name: "clusterroles", Kind: "ClusterRole", SingularName: "clusterrole"})
	crb := cs.NewStore(t, gv, metav1.APIResource{Name: "clusterrolebindings", Kind: "ClusterRoleBinding", SingularName: "clusterrolebinding"})
	r := cs.NewStore(t, gv, metav1.APIResource{Name: "roles", Kind: "Role", SingularName: "role", Namespaced: true})
	rb := cs.NewStore(t, gv, metav1.APIResource{Name: "rolebindings", Kind: "RoleBinding", SingularName: "rolebinding", Namespaced: true})

	cs.FailPuts.Store(true)

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := bootstrapRBAC(cr, crb, r, rb, next)
	req := httptest.NewRequest("GET", "/apis/rbac.authorization.k8s.io/v1/clusterroles", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if _, ok := cs.GetMarker("rbac"); ok {
		t.Fatal("failed ensure wrote marker")
	}
}
