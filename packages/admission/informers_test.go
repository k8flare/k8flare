package admission

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	admit "github.com/k8flare/k8flare/packages/apiserver-admit"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/tools/cache"
)

type failingPlugin struct{ err error }

func (failingPlugin) Handles(admission.Operation) bool { return true }
func (p failingPlugin) Validate(context.Context, admission.Attributes, admission.ObjectInterfaces) error {
	return p.err
}

func TestUpstreamStatusReachesTheClientUnchanged(t *testing.T) {
	statuses := map[string]error{
		"not found": apierrors.NewNotFound(schema.GroupResource{Group: "scheduling.k8s.io", Resource: "priorityclasses"}, "gone"),
		"invalid": apierrors.NewInvalid(schema.GroupKind{Kind: "Pod"}, "p", field.ErrorList{
			field.Invalid(field.NewPath("spec", "priority"), 5, "must match the class"),
			field.Required(field.NewPath("spec", "containers"), ""),
		}),
		"bad request": apierrors.NewBadRequest("could not convert"),
		"conflict":    apierrors.NewConflict(schema.GroupResource{Resource: "pods"}, "p", http.ErrAbortHandler),
	}
	for name, plugErr := range statuses {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req admit.Request
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				resp := denyResponse(runUpstreamPlugin(r.Context(), failingPlugin{err: plugErr}, &req))
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer srv.Close()

			attrs := admission.NewAttributesRecord(nil, nil, schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, "ns", "p",
				schema.GroupVersionResource{Version: "v1", Resource: "pods"}, "", admission.Create, nil, false, &user.DefaultInfo{Name: "alice"})
			err := admit.New(rewriteClient(srv)).(admission.ValidationInterface).Validate(context.Background(), attrs, nil)

			want := plugErr.(apierrors.APIStatus)
			got, ok := err.(apierrors.APIStatus)
			if !ok {
				t.Fatalf("client error %T %v is not an API status", err, err)
			}
			if !apiequality.Semantic.DeepEqual(want.Status(), got.Status()) {
				t.Fatalf("status changed on the way:\nwant %+v\ngot  %+v", want.Status(), got.Status())
			}
			if err.Error() != plugErr.Error() {
				t.Fatalf("message changed on the way: want %q got %q", plugErr.Error(), err.Error())
			}
		})
	}
}

func failingKine(t *testing.T) *store {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "store down", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return &store{client: &kine.Client{HTTP: rewriteClient(srv)}}
}

func TestStoreFailureIsNotAnEmptyList(t *testing.T) {
	req := priorityPodReq("", nil)
	err := applyPriority(context.Background(), failingKine(t), &req)
	if !apierrors.IsInternalError(err) {
		t.Fatalf("a failed store read during a list must surface as an internal error, got %v", err)
	}
}

func TestUnimplementedIndexFailsLoudly(t *testing.T) {
	f := newStoreInformerFactory(context.Background(), &store{})
	if _, err := f.priorityClass.ByIndex("byOwner", "x"); err == nil {
		t.Fatal("ByIndex on an unimplemented index must fail")
	}
	if err := f.priorityClass.AddIndexers(cache.Indexers{"byOwner": nil}); err == nil {
		t.Fatal("AddIndexers for an unimplemented index must fail")
	}
	f.priorityClass.ListIndexFuncValues("byOwner")
	if f.failure == nil {
		t.Fatal("ListIndexFuncValues on an unimplemented index must record a failure")
	}
}

func TestInterleavedRequestsKeepTheirOwnContext(t *testing.T) {
	backing := httptest.NewServer(&memStore{data: map[string][]byte{
		"/registry/priorityclasses/high": mustJSON(t, schedulingv1.PriorityClass{
			ObjectMeta: metav1.ObjectMeta{Name: "high"},
			Value:      1000,
		}),
	}})
	defer backing.Close()
	s := &store{client: &kine.Client{HTTP: rewriteClient(backing)}}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	var wg sync.WaitGroup
	results := make([]error, 20)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			if i%2 == 0 {
				ctx = cancelled
			}
			req := priorityPodReq("high", nil)
			results[i] = applyPriority(ctx, s, &req)
		}()
	}
	wg.Wait()
	for i, err := range results {
		if i%2 == 0 && err == nil {
			t.Fatalf("request %d with a cancelled context must fail", i)
		}
		if i%2 == 1 && err != nil {
			t.Fatalf("request %d with a live context must succeed, got %v", i, err)
		}
	}
}
