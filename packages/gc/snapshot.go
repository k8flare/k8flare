package gc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"

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
}

type graph struct {
	byUID      map[types.UID]*item
	dependents map[types.UID][]*item
	items      []*item
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
