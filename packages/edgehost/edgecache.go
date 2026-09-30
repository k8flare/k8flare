package edgehost

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	corev1 "k8s.io/api/core/v1"
)

const edgeCacheTTL = 2 * time.Second

type portSel struct {
	Number int32
	Name   string
}

type dialTarget struct {
	Node string
	Host string
	Port string
}

type stamped[T any] struct {
	value T
	at    time.Time
}

type edgeCache struct {
	mu       sync.Mutex
	now      func() time.Time
	ttl      time.Duration
	table    *stamped[*Table]
	services map[string]stamped[corev1.Service]
	dials    map[string]stamped[dialTarget]
}

var edge = newEdgeCache(time.Now, edgeCacheTTL)

func newEdgeCache(now func() time.Time, ttl time.Duration) *edgeCache {
	return &edgeCache{now: now, ttl: ttl, services: map[string]stamped[corev1.Service]{}, dials: map[string]stamped[dialTarget]{}}
}

func (c *edgeCache) fresh(at time.Time) bool {
	return c.now().Sub(at) < c.ttl
}

func (c *edgeCache) loadTable(ctx context.Context, store *kine.Client) *Table {
	c.mu.Lock()
	cached := c.table
	c.mu.Unlock()
	if cached != nil && c.fresh(cached.at) {
		return cached.value
	}
	table := &Table{}
	kv, _, err := store.Get(ctx, routeTableKey)
	switch {
	case err == kine.ErrNotFound:
	case err != nil || kv == nil:
		if cached != nil {
			return cached.value
		}
		return nil
	default:
		if !decodeKV(kv.Value, table) {
			return nil
		}
		table.Prepare()
	}
	c.mu.Lock()
	c.table = &stamped[*Table]{table, c.now()}
	c.mu.Unlock()
	return table
}

func (c *edgeCache) service(r *http.Request, store *kine.Client, ref Ref) (corev1.Service, bool) {
	key := ref.Namespace + "/" + ref.Name
	c.mu.Lock()
	cached, ok := c.services[key]
	c.mu.Unlock()
	if ok && c.fresh(cached.at) {
		return cached.value, true
	}
	svc, found := getService(r, store, ref)
	if !found {
		return corev1.Service{}, false
	}
	c.mu.Lock()
	c.services[key] = stamped[corev1.Service]{svc, c.now()}
	c.mu.Unlock()
	return svc, true
}

func dialKey(ref Ref, sel portSel) string {
	return ref.Namespace + "/" + ref.Name + "/" + strconv.Itoa(int(sel.Number)) + "/" + sel.Name
}

func (c *edgeCache) dial(r *http.Request, store *kine.Client, ref Ref, ports []corev1.ServicePort, sel portSel) (dialTarget, string) {
	key := dialKey(ref, sel)
	c.mu.Lock()
	cached, ok := c.dials[key]
	c.mu.Unlock()
	if ok && c.fresh(cached.at) {
		return cached.value, key
	}
	node, host, port := resolveDial(r, store, ref, ports, sel)
	target := dialTarget{node, host, port}
	if node == "" || host == "" || port == "" {
		return dialTarget{}, key
	}
	c.mu.Lock()
	c.dials[key] = stamped[dialTarget]{target, c.now()}
	c.mu.Unlock()
	return target, key
}

func (c *edgeCache) evictDial(key string) {
	c.mu.Lock()
	delete(c.dials, key)
	c.mu.Unlock()
}
