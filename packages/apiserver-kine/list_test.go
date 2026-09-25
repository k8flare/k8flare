package kine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestGetListContinueKeepsResourceVersionAndRemaining(t *testing.T) {
	codec := scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion)
	srv := newListServer(t, 25, 0, 100)
	s := NewStorage(&Client{HTTP: rewriteListClient(srv)}, codec, func() runtime.Object { return &corev1.ConfigMap{} })
	pred := storage.SelectionPredicate{Label: labels.Everything(), Field: fields.Everything(), Limit: 7, GetAttrs: storage.DefaultClusterScopedAttr}
	var first corev1.ConfigMapList
	if err := s.GetList(context.Background(), "/configmaps/ns/", storage.ListOptions{Recursive: true, Predicate: pred}, &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 7 || first.Continue == "" || first.RemainingItemCount == nil || *first.RemainingItemCount != 18 {
		t.Fatalf("first page: items=%d continue=%q remaining=%v rv=%s", len(first.Items), first.Continue, first.RemainingItemCount, first.ResourceVersion)
	}
	rv := first.ResourceVersion
	found := len(first.Items)
	pred.Continue = first.Continue
	for pred.Continue != "" {
		var page corev1.ConfigMapList
		if err := s.GetList(context.Background(), "/configmaps/ns/", storage.ListOptions{Recursive: true, Predicate: pred}, &page); err != nil {
			t.Fatal(err)
		}
		if page.ResourceVersion != rv {
			t.Fatalf("rv changed %s -> %s", rv, page.ResourceVersion)
		}
		if page.Continue != "" {
			if page.RemainingItemCount == nil || int(*page.RemainingItemCount)+len(page.Items)+found != 25 {
				t.Fatalf("remaining=%v items=%d found=%d", page.RemainingItemCount, len(page.Items), found)
			}
		}
		found += len(page.Items)
		pred.Continue = page.Continue
	}
	if found != 25 {
		t.Fatalf("found %d", found)
	}
}

func TestGetListExactResourceVersion(t *testing.T) {
	codec := scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion)
	srv := newListServer(t, 25, 0, 100)
	s := NewStorage(&Client{HTTP: rewriteListClient(srv)}, codec, func() runtime.Object { return &corev1.ConfigMap{} })
	pred := storage.SelectionPredicate{Label: labels.Everything(), Field: fields.Everything(), GetAttrs: storage.DefaultClusterScopedAttr}
	var listed corev1.ConfigMapList
	err := s.GetList(context.Background(), "/configmaps/ns/", storage.ListOptions{
		Recursive:            true,
		Predicate:            pred,
		ResourceVersion:      "10",
		ResourceVersionMatch: metav1.ResourceVersionMatchExact,
	}, &listed)
	if err != nil {
		t.Fatal(err)
	}
	if listed.ResourceVersion != "10" || len(listed.Items) != 10 {
		t.Fatalf("items=%d rv=%s", len(listed.Items), listed.ResourceVersion)
	}
	if listed.Items[0].Name != "cm-0000" || listed.Items[9].Name != "cm-0009" {
		t.Fatalf("range %s..%s", listed.Items[0].Name, listed.Items[len(listed.Items)-1].Name)
	}
}

func TestGetListContinueStaysAtSnapshot(t *testing.T) {
	codec := scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion)
	srv := newListServer(t, 25, 0, 100)
	s := NewStorage(&Client{HTTP: rewriteListClient(srv)}, codec, func() runtime.Object { return &corev1.ConfigMap{} })
	pred := storage.SelectionPredicate{Label: labels.Everything(), Field: fields.Everything(), Limit: 4, GetAttrs: storage.DefaultClusterScopedAttr}
	var first corev1.ConfigMapList
	err := s.GetList(context.Background(), "/configmaps/ns/", storage.ListOptions{
		Recursive:            true,
		Predicate:            pred,
		ResourceVersion:      "10",
		ResourceVersionMatch: metav1.ResourceVersionMatchExact,
	}, &first)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 4 || first.ResourceVersion != "10" || first.Continue == "" {
		t.Fatalf("first page: items=%d rv=%s continue=%q", len(first.Items), first.ResourceVersion, first.Continue)
	}
	found := len(first.Items)
	pred.Continue = first.Continue
	for pred.Continue != "" {
		var page corev1.ConfigMapList
		if err := s.GetList(context.Background(), "/configmaps/ns/", storage.ListOptions{Recursive: true, Predicate: pred}, &page); err != nil {
			t.Fatal(err)
		}
		if page.ResourceVersion != "10" {
			t.Fatalf("rv changed %s", page.ResourceVersion)
		}
		for _, cm := range page.Items {
			if cm.Name > "cm-0009" {
				t.Fatalf("later object %s leaked into snapshot", cm.Name)
			}
		}
		found += len(page.Items)
		pred.Continue = page.Continue
	}
	if found != 10 {
		t.Fatalf("found %d", found)
	}
}

func TestGetListCompactedExactResourceVersion(t *testing.T) {
	codec := scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion)
	srv := newListServer(t, 10, 50, 40)
	s := NewStorage(&Client{HTTP: rewriteListClient(srv)}, codec, func() runtime.Object { return &corev1.ConfigMap{} })
	err := s.GetList(context.Background(), "/configmaps/ns/", storage.ListOptions{
		Recursive:            true,
		Predicate:            storage.SelectionPredicate{Label: labels.Everything(), Field: fields.Everything(), GetAttrs: storage.DefaultClusterScopedAttr},
		ResourceVersion:      "10",
		ResourceVersionMatch: metav1.ResourceVersionMatchExact,
	}, &corev1.ConfigMapList{})
	if !apierrors.IsResourceExpired(err) {
		t.Fatalf("got %v", err)
	}
}

func TestGetListCompactedContinue(t *testing.T) {
	codec := scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion)
	srv := newListServer(t, 10, 50, 100)
	s := NewStorage(&Client{HTTP: rewriteListClient(srv)}, codec, func() runtime.Object { return &corev1.ConfigMap{} })
	token, err := storage.EncodeContinue("/registry/configmaps/ns/cm-0003", "/registry/configmaps/ns/", 10)
	if err != nil {
		t.Fatal(err)
	}
	pred := storage.SelectionPredicate{Label: labels.Everything(), Field: fields.Everything(), Limit: 3, Continue: token, GetAttrs: storage.DefaultClusterScopedAttr}
	err = s.GetList(context.Background(), "/configmaps/ns/", storage.ListOptions{Recursive: true, Predicate: pred}, &corev1.ConfigMapList{})
	if !apierrors.IsResourceExpired(err) {
		t.Fatalf("got %v", err)
	}
	status, _ := err.(apierrors.APIStatus)
	if status.Status().ListMeta.Continue == "" {
		t.Fatal("expected inconsistent continue")
	}
	pred.Continue = status.Status().ListMeta.Continue
	var page corev1.ConfigMapList
	if err := s.GetList(context.Background(), "/configmaps/ns/", storage.ListOptions{Recursive: true, Predicate: pred}, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 3 || page.Items[0].Name != "cm-0003" {
		t.Fatalf("inconsistent page items=%d first=%v", len(page.Items), page.Items)
	}
	if page.RemainingItemCount == nil || int(*page.RemainingItemCount)+len(page.Items)+3 != 10 {
		t.Fatalf("remaining=%v items=%d", page.RemainingItemCount, len(page.Items))
	}
}

type listServer struct {
	keys    []KV
	rev     int64
	compact int64
}

func newListServer(t *testing.T, n int, compact, rev int64) *httptest.Server {
	t.Helper()
	codec := scheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion)
	keys := make([]KV, n)
	for i := 0; i < n; i++ {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm-" + itoa4(i), Namespace: "ns"}}
		data, err := runtime.Encode(codec, cm)
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = KV{Key: "/registry/configmaps/ns/" + cm.Name, Value: base64.StdEncoding.EncodeToString(data), ModRevision: int64(i + 1)}
	}
	ls := &listServer{keys: keys, rev: rev, compact: compact}
	return httptest.NewServer(ls)
}

func (l *listServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/stats":
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": l.rev, "compactRevision": l.compact})
	case "/list":
		prefix := r.URL.Query().Get("prefix")
		from := r.URL.Query().Get("from")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		at, _ := strconv.ParseInt(r.URL.Query().Get("revision"), 10, 64)
		if at > 0 && l.compact > 0 && at < l.compact {
			w.WriteHeader(http.StatusGone)
			_ = json.NewEncoder(w).Encode(map[string]any{"revision": l.rev, "compactRevision": l.compact, "error": "compacted"})
			return
		}
		var out []KV
		for _, kv := range l.keys {
			if !strings.HasPrefix(kv.Key, prefix) {
				continue
			}
			if from != "" && kv.Key < from {
				continue
			}
			if at > 0 && kv.ModRevision > at {
				continue
			}
			out = append(out, kv)
		}
		more := false
		if limit > 0 && len(out) > limit {
			more = true
			out = out[:limit]
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": l.rev, "kvs": out, "more": more})
	default:
		http.NotFound(w, r)
	}
}

func rewriteListClient(srv *httptest.Server) *http.Client {
	return &http.Client{Transport: hostRewrite{base: srv.URL, next: srv.Client().Transport}}
}

type hostRewrite struct {
	base string
	next http.RoundTripper
}

func (h hostRewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	next := req.Clone(req.Context())
	u, err := http.NewRequest(req.Method, h.base+req.URL.RequestURI(), req.Body)
	if err != nil {
		return nil, err
	}
	next.URL = u.URL
	next.Host = u.Host
	if h.next == nil {
		return http.DefaultTransport.RoundTrip(next)
	}
	return h.next.RoundTrip(next)
}

func itoa4(i int) string {
	s := strconv.Itoa(i)
	for len(s) < 4 {
		s = "0" + s
	}
	return s
}
