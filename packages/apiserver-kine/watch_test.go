package kine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestWatchReturnsWhenTheDialerKeepsFailing(t *testing.T) {
	previous := WatchDialer
	defer func() { WatchDialer = previous }()
	attempts := 0
	WatchDialer = func(context.Context, string) (<-chan []byte, func(), error) {
		attempts++
		return nil, nil, fmt.Errorf("dial refused")
	}
	s := NewStorage(nil, nil, func() runtime.Object { return &v1.Pod{} })
	done := make(chan error, 1)
	go func() {
		_, err := s.Watch(context.Background(), "/registry/pods/", storage.ListOptions{Recursive: true, ResourceVersion: "1"})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Watch blocked instead of returning an error")
	}
	if attempts < 2 {
		t.Fatalf("attempts = %d, want the redial retries", attempts)
	}
}

func TestRequestWatchProgressAsksTheClusterToNotifyWatchers(t *testing.T) {
	var method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	s := NewStorage(&Client{HTTP: rewriteListClient(srv)}, nil, func() runtime.Object { return &v1.Pod{} })
	if err := s.RequestWatchProgress(context.Background()); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || path != "/progress" {
		t.Fatalf("request = %s %s, want POST /progress", method, path)
	}
}

func TestWatchDeliversProgressAsBookmarkOnlyWhenAsked(t *testing.T) {
	codec := scheme.Codecs.LegacyCodec(v1.SchemeGroupVersion)
	cases := []struct {
		name string
		opts storage.ListOptions
		want bool
	}{
		{"progress notify", storage.ListOptions{Recursive: true, ResourceVersion: "5", ProgressNotify: true}, true},
		{"allow bookmarks", storage.ListOptions{Recursive: true, ResourceVersion: "5", Predicate: storage.SelectionPredicate{AllowWatchBookmarks: true}}, true},
		{"neither", storage.ListOptions{Recursive: true, ResourceVersion: "5"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			previous := WatchDialer
			defer func() { WatchDialer = previous }()
			msgs := make(chan []byte, 2)
			msgs <- []byte(`{"rev":9,"type":"progress"}`)
			WatchDialer = func(context.Context, string) (<-chan []byte, func(), error) { return msgs, func() {}, nil }
			s := NewStorage(nil, codec, func() runtime.Object { return &v1.Pod{} })
			w, err := s.Watch(context.Background(), "/pods/", tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			defer w.Stop()
			select {
			case ev := <-w.ResultChan():
				if !tc.want {
					t.Fatalf("unexpected event %v", ev.Type)
				}
				if ev.Type != watch.Bookmark {
					t.Fatalf("event = %v, want Bookmark", ev.Type)
				}
				if rv := ev.Object.(*v1.Pod).ResourceVersion; rv != "9" {
					t.Fatalf("bookmark rv = %s, want 9", rv)
				}
			case <-time.After(300 * time.Millisecond):
				if tc.want {
					t.Fatal("no bookmark")
				}
			}
		})
	}
}

func TestSelectorWatchKeepsDeliveringUpdatesAfterTheObjectEntersTheSelector(t *testing.T) {
	previous := WatchDialer
	defer func() { WatchDialer = previous }()
	codec := scheme.Codecs.LegacyCodec(appsv1.SchemeGroupVersion)
	replicaSet := func(labelled bool, ready int32) string {
		rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "test-rs", Namespace: "ns"}}
		if labelled {
			rs.Labels = map[string]string{"test-rs": "patched"}
		}
		rs.Status.ReadyReplicas = ready
		data, err := runtime.Encode(codec, rs)
		if err != nil {
			t.Fatal(err)
		}
		return base64.StdEncoding.EncodeToString(data)
	}
	const key = "/registry/replicasets/ns/test-rs"
	modified := func(rev int64, prev, value string) []byte {
		msg, err := json.Marshal(kineEvent{Rev: rev, Type: "modified", Key: key, Prev: prev, Value: value})
		if err != nil {
			t.Fatal(err)
		}
		return msg
	}
	replicaSetAttrs := func(obj runtime.Object) (labels.Set, fields.Set, error) {
		rs := obj.(*appsv1.ReplicaSet)
		return labels.Set(rs.Labels), fields.Set{}, nil
	}
	msgs := make(chan []byte, 8)
	msgs <- modified(10, replicaSet(false, 0), replicaSet(true, 0))
	msgs <- modified(11, replicaSet(true, 0), replicaSet(true, 1))
	msgs <- modified(12, replicaSet(true, 1), replicaSet(true, 2))
	msgs <- modified(13, replicaSet(true, 2), replicaSet(true, 3))
	msgs <- []byte(`{"rev":14,"type":"progress"}`)
	WatchDialer = func(context.Context, string) (<-chan []byte, func(), error) { return msgs, func() {}, nil }
	s := NewStorage(&Client{Secrets: &SecretCipher{}}, codec, func() runtime.Object { return &appsv1.ReplicaSet{} })
	pred := storage.SelectionPredicate{Label: labels.SelectorFromSet(labels.Set{"test-rs": "patched"}), Field: fields.Everything(), GetAttrs: replicaSetAttrs, AllowWatchBookmarks: true}
	w, err := s.Watch(context.Background(), "/replicasets/ns/", storage.ListOptions{Recursive: true, ResourceVersion: "9", Predicate: pred})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	want := []watch.EventType{watch.Added, watch.Modified, watch.Modified, watch.Modified, watch.Bookmark}
	for i, wantType := range want {
		select {
		case ev := <-w.ResultChan():
			if ev.Type != wantType {
				t.Fatalf("event %d = %v, want %v", i, ev.Type, wantType)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("event %d (%v) never arrived", i, wantType)
		}
	}
}
