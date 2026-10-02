package gc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	registryPrefix = "/registry/"
	listPage       = 500
)

type item struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	key               string
	ownersMu          sync.Mutex
}

func (it *item) owners() []metav1.OwnerReference {
	it.ownersMu.Lock()
	defer it.ownersMu.Unlock()
	return it.OwnerReferences
}

func (it *item) setOwners(refs []metav1.OwnerReference) {
	it.ownersMu.Lock()
	defer it.ownersMu.Unlock()
	it.OwnerReferences = refs
}

type graph struct {
	byUID      map[types.UID]*item
	dependents map[types.UID][]*item
	items      []*item
	eventKeys  []string
	eventAt    map[string]time.Time
	leaseKeys  []string
}

func loadGraph(ctx context.Context, client *kine.Client) (*graph, error) {
	g := &graph{byUID: map[types.UID]*item{}, dependents: map[types.UID][]*item{}}
	from := ""
	for {
		kvs, _, more, err := client.List(ctx, registryPrefix, from, listPage)
		if err != nil {
			return nil, err
		}
		if len(kvs) == 0 {
			break
		}
		for _, kv := range kvs {
			from = kv.Key
			if _, ok := eventNamespace(kv.Key); ok {
				g.eventKeys = append(g.eventKeys, kv.Key)
				if at := eventTime(kv.Value); !at.IsZero() {
					if g.eventAt == nil {
						g.eventAt = map[string]time.Time{}
					}
					g.eventAt[kv.Key] = at
				}
				continue
			}
			if _, ok := leaseNamespace(kv.Key); ok {
				g.leaseKeys = append(g.leaseKeys, kv.Key)
				continue
			}
			if it := decodeItem(kv); it != nil {
				g.add(it)
			}
		}
		if !more {
			break
		}
	}
	return g, nil
}

func eventNamespace(key string) (string, bool) {
	if ns, ok := namespacedKey(registryPrefix+"events/", key); ok {
		return ns, true
	}
	return namespacedKey(registryPrefix+"events.k8s.io/events/", key)
}

func leaseNamespace(key string) (string, bool) {
	return namespacedKey(registryPrefix+"leases/", key)
}

func namespacedKey(prefix, key string) (string, bool) {
	rest, ok := strings.CutPrefix(key, prefix)
	if !ok {
		return "", false
	}
	ns, name, ok := strings.Cut(rest, "/")
	if !ok || ns == "" || name == "" || strings.Contains(name, "/") {
		return "", false
	}
	return ns, true
}

func eventTime(value string) time.Time {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}
	}
	var ev struct {
		Metadata struct {
			CreationTimestamp time.Time `json:"creationTimestamp"`
		} `json:"metadata"`
		EventTime     metav1.MicroTime `json:"eventTime"`
		LastTimestamp metav1.Time      `json:"lastTimestamp"`
		Series        *struct {
			LastObservedTime metav1.MicroTime `json:"lastObservedTime"`
		} `json:"series"`
	}
	if json.Unmarshal(data, &ev) != nil {
		return time.Time{}
	}
	at := ev.Metadata.CreationTimestamp
	if t := ev.LastTimestamp.Time; t.After(at) {
		at = t
	}
	if t := ev.EventTime.Time; t.After(at) {
		at = t
	}
	if ev.Series != nil && ev.Series.LastObservedTime.After(at) {
		at = ev.Series.LastObservedTime.Time
	}
	return at
}

func decodeItem(kv kine.KV) *item {
	if strings.HasPrefix(kv.Key, registryPrefix+"events/") || strings.HasPrefix(kv.Key, registryPrefix+"leases/") {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		return nil
	}
	it := &item{key: kv.Key}
	if json.Unmarshal(data, it) != nil || it.UID == "" || it.Kind == "" {
		return nil
	}
	it.ResourceVersion = strconv.FormatInt(kv.ModRevision, 10)
	return it
}

func (g *graph) add(it *item) {
	g.items = append(g.items, it)
	g.byUID[it.UID] = it
	for _, owner := range it.OwnerReferences {
		g.dependents[owner.UID] = append(g.dependents[owner.UID], it)
	}
}

func (it *item) hasFinalizer(name string) bool {
	for _, f := range it.Finalizers {
		if f == name {
			return true
		}
	}
	return false
}
