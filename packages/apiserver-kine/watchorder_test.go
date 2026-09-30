package kine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/client-go/kubernetes/scheme"
)

type fakeLog struct {
	mu        sync.Mutex
	events    []kineEvent
	subs      []*fakeSub
	dials     int
	dropEvery int
}

type fakeSub struct {
	prefix string
	msgs   chan []byte
	left   int
	closed bool
}

func (l *fakeLog) dial(_ context.Context, rawURL string) (<-chan []byte, func(), error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, err
	}
	since, _ := strconv.ParseInt(u.Query().Get("since"), 10, 64)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dials++
	sub := &fakeSub{prefix: u.Query().Get("prefix"), msgs: make(chan []byte, 1024), left: -1}
	if l.dropEvery > 0 {
		sub.left = 1 + l.dials%l.dropEvery
	}
	for _, ev := range l.events {
		if ev.Rev > since && strings.HasPrefix(ev.Key, sub.prefix) {
			l.push(sub, ev)
		}
	}
	l.subs = append(l.subs, sub)
	return sub.msgs, func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.end(sub)
	}, nil
}

func (l *fakeLog) end(sub *fakeSub) {
	if !sub.closed {
		sub.closed = true
		close(sub.msgs)
	}
}

func (l *fakeLog) push(sub *fakeSub, ev kineEvent) {
	if sub.closed {
		return
	}
	if sub.left == 0 {
		l.end(sub)
		return
	}
	sub.left--
	b, _ := json.Marshal(ev)
	sub.msgs <- b
}

func (l *fakeLog) write(key, kind string, obj runtime.Object, codec runtime.Codec) int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	rev := int64(len(l.events) + 2)
	value := ""
	if obj != nil {
		raw, err := runtime.Encode(codec, obj)
		if err != nil {
			panic(err)
		}
		value = base64.StdEncoding.EncodeToString(raw)
	}
	ev := kineEvent{Rev: rev, Type: kind, Key: key, Value: value}
	if kind == "progress" {
		ev.Key = ""
	}
	l.events = append(l.events, ev)
	for _, sub := range l.subs {
		if kind == "progress" || strings.HasPrefix(key, sub.prefix) {
			l.push(sub, ev)
		}
	}
	return rev
}

func TestConcurrentWatchesFromEachRevisionSeeTheSameOrderAcrossRedials(t *testing.T) {
	previous := WatchDialer
	defer func() { WatchDialer = previous }()
	log := &fakeLog{dropEvery: 3}
	WatchDialer = log.dial
	codec := scheme.Codecs.LegacyCodec(v1.SchemeGroupVersion)
	s := NewStorage(&Client{Secrets: &SecretCipher{}}, codec, func() runtime.Object { return &v1.ConfigMap{} })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := func(rv int64) storage.ListOptions {
		pred := storage.Everything
		pred.AllowWatchBookmarks = true
		return storage.ListOptions{Recursive: true, ResourceVersion: strconv.FormatInt(rv, 10), Predicate: pred}
	}
	next := func(t *testing.T, w watch.Interface) string {
		t.Helper()
		for {
			select {
			case ev, ok := <-w.ResultChan():
				if !ok {
					t.Fatal("watch closed")
				}
				if ev.Type == watch.Bookmark {
					continue
				}
				if ev.Type == watch.Error {
					t.Fatalf("watch error: %v", ev.Object)
				}
				return ev.Object.(*v1.ConfigMap).ResourceVersion
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for watch event")
			}
		}
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		var existing []string
		i := 0
		for {
			select {
			case <-stop:
				return
			case <-time.After(time.Millisecond):
			}
			switch {
			case len(existing) == 0 || i%3 == 0:
				name := fmt.Sprintf("cm-%d", i)
				log.write("/registry/configmaps/watch/"+name, "created", &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "watch"}}, codec)
				existing = append(existing, name)
			case i%3 == 1:
				name := existing[i%len(existing)]
				log.write("/registry/configmaps/watch/"+name, "modified", &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "watch", Labels: map[string]string{"mutated": strconv.Itoa(i)}}}, codec)
			default:
				name := existing[i%len(existing)]
				log.write("/registry/configmaps/watch/"+name, "deleted", &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "watch"}}, codec)
				existing = append(existing[:i%len(existing)], existing[i%len(existing)+1:]...)
			}
			if i%5 == 0 {
				log.write("/registry/leases/kube-node-lease/node", "progress", nil, codec)
			}
			i++
		}
	}()
	defer func() { close(stop); <-done }()
	first, err := s.Watch(ctx, "/configmaps/watch/", opts(1))
	if err != nil {
		t.Fatal(err)
	}
	watches := []watch.Interface{first}
	for i := 0; i < 60; i++ {
		rv := next(t, first)
		for n, w := range watches[1:] {
			if got := next(t, w); got != rv {
				t.Fatalf("iteration %d: watch %d got resource version %s, want %s", i, n+1, got, rv)
			}
		}
		parsed, _ := strconv.ParseInt(rv, 10, 64)
		w, err := s.Watch(ctx, "/configmaps/watch/", opts(parsed))
		if err != nil {
			t.Fatal(err)
		}
		watches = append(watches, w)
	}
	for _, w := range watches {
		w.Stop()
	}
	if log.dials <= len(watches) {
		t.Fatalf("no socket was closed and redialed: %d dials for %d watches", log.dials, len(watches))
	}
}
