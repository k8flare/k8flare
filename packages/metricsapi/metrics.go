package metricsapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	Group       = "metrics.k8s.io"
	Version     = "v1beta1"
	GV          = Group + "/" + Version
	snapshotKey = "/metrics/snapshot"
	scrapeEvery = 15 * time.Second
	staleAfter  = 30 * time.Second
)

var promLine = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{([^}]*)\})?\s+(\S+)`)
var promLabel = regexp.MustCompile(`^\s*([a-zA-Z_][a-zA-Z0-9_]*)="(.*)"\s*$`)

type Usage struct {
	CPUSeconds  float64 `json:"cpuSeconds"`
	MemoryBytes float64 `json:"memoryBytes"`
}

type Snapshot struct {
	At        int64            `json:"at"`
	PrevAt    int64            `json:"prevAt,omitempty"`
	Nodes     map[string]Usage `json:"nodes"`
	PrevNodes map[string]Usage `json:"prevNodes,omitempty"`
	Pods      map[string]Usage `json:"pods"`
	PrevPods  map[string]Usage `json:"prevPods,omitempty"`
}

type Sample struct {
	Name   string
	Labels map[string]string
	Value  float64
}

type Handler struct {
	Store  *kine.Client
	Tunnel *http.Client
}

func Resources() []metav1.APIResource {
	return []metav1.APIResource{
		{Name: "nodes", SingularName: "node", Namespaced: false, Kind: "NodeMetrics", Verbs: metav1.Verbs{"get", "list"}},
		{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "PodMetrics", Verbs: metav1.Verbs{"get", "list"}},
	}
}

func APIGroup() metav1.APIGroup {
	v := metav1.GroupVersionForDiscovery{GroupVersion: GV, Version: Version}
	return metav1.APIGroup{Name: Group, Versions: []metav1.GroupVersionForDiscovery{v}, PreferredVersion: v}
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch path {
	case "/apis/" + Group:
		writeJSON(w, APIGroup())
		return
	case "/apis/" + GV:
		writeJSON(w, map[string]any{
			"kind": "APIResourceList", "apiVersion": "v1", "groupVersion": GV,
			"resources": []map[string]any{
				{"name": "nodes", "singularName": "node", "namespaced": false, "kind": "NodeMetrics", "verbs": []string{"get", "list"}},
				{"name": "pods", "singularName": "pod", "namespaced": true, "kind": "PodMetrics", "verbs": []string{"get", "list"}},
			},
		})
		return
	}
	snap := h.snapshot(r)
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch {
	case len(parts) == 4 && parts[3] == "nodes":
		writeJSON(w, map[string]any{"kind": "NodeMetricsList", "apiVersion": GV, "items": listNodes(snap)})
	case len(parts) == 5 && parts[3] == "nodes":
		item := nodeMetrics(snap, parts[4])
		if item == nil {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, item)
	case len(parts) == 4 && parts[3] == "pods":
		writeJSON(w, map[string]any{"kind": "PodMetricsList", "apiVersion": GV, "items": listPods(snap, "")})
	case len(parts) == 6 && parts[3] == "namespaces" && parts[5] == "pods":
		writeJSON(w, map[string]any{"kind": "PodMetricsList", "apiVersion": GV, "items": listPods(snap, parts[4])})
	case len(parts) == 7 && parts[3] == "namespaces" && parts[5] == "pods":
		item := podMetrics(snap, parts[4], parts[6])
		if item == nil {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, item)
	default:
		http.NotFound(w, r)
	}
}

func (h Handler) Scrape(w http.ResponseWriter, r *http.Request) {
	n := h.scrape(r.Context())
	writeJSON(w, map[string]int{"nodes": n})
}

func (h Handler) snapshot(r *http.Request) Snapshot {
	snap, ok := loadSnapshot(r.Context(), h.Store)
	if ok && time.Since(time.UnixMilli(snap.At)) < staleAfter {
		return snap
	}
	h.scrape(r.Context())
	if fresh, ok := loadSnapshot(r.Context(), h.Store); ok {
		return fresh
	}
	if ok {
		return snap
	}
	return emptySnap()
}

func (h Handler) scrape(ctx context.Context) int {
	prev, ok := loadSnapshot(ctx, h.Store)
	if ok && time.Since(time.UnixMilli(prev.At)) < scrapeEvery {
		return 0
	}
	next := emptySnap()
	next.At = time.Now().UnixMilli()
	if prev.At != 0 {
		next.PrevAt = prev.At
		next.PrevNodes = prev.Nodes
		next.PrevPods = prev.Pods
	}
	n := 0
	if h.Store != nil {
		kvs, _, _, err := h.Store.List(ctx, "/registry/nodes/", "", 500)
		if err == nil {
			for _, kv := range kvs {
				name := nodeName(kv)
				if name == "" || h.Tunnel == nil {
					continue
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://nodetunnel.internal/node/"+name+"/metrics/resource", nil)
				if err != nil {
					continue
				}
				req.Header.Set("Accept", "text/plain")
				resp, err := h.Tunnel.Do(req)
				if err != nil {
					continue
				}
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					continue
				}
				Apply(next, name, string(body))
				n++
			}
		}
	}
	storeSnapshot(ctx, h.Store, next)
	return n
}

func nodeName(kv kine.KV) string {
	raw, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		raw = []byte(kv.Value)
	}
	var obj struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	}
	if json.Unmarshal(raw, &obj) != nil || obj.Metadata.Name == "" {
		name := strings.TrimPrefix(kv.Key, "/registry/nodes/")
		if strings.Contains(name, "/") {
			return ""
		}
		return name
	}
	return obj.Metadata.Name
}

func emptySnap() Snapshot {
	return Snapshot{Nodes: map[string]Usage{}, Pods: map[string]Usage{}}
}

func loadSnapshot(ctx context.Context, store *kine.Client) (Snapshot, bool) {
	if store == nil {
		return emptySnap(), false
	}
	kv, _, err := store.Get(ctx, snapshotKey)
	if err != nil || kv == nil {
		return emptySnap(), false
	}
	raw, err := base64.StdEncoding.DecodeString(kv.Value)
	if err != nil {
		raw = []byte(kv.Value)
	}
	var snap Snapshot
	if json.Unmarshal(raw, &snap) != nil {
		return emptySnap(), false
	}
	if snap.Nodes == nil {
		snap.Nodes = map[string]Usage{}
	}
	if snap.Pods == nil {
		snap.Pods = map[string]Usage{}
	}
	return snap, true
}

func storeSnapshot(ctx context.Context, store *kine.Client, snap Snapshot) {
	if store == nil {
		return
	}
	body, err := json.Marshal(snap)
	if err != nil {
		return
	}
	rev := int64(0)
	if kv, _, err := store.Get(ctx, snapshotKey); err == nil && kv != nil {
		rev = kv.ModRevision
		if prev, ok := decodeSnapshot(kv.Value); ok && sameUsage(prev, snap) && time.Since(time.UnixMilli(prev.At)) < scrapeEvery {
			return
		}
	}
	_, _ = store.Put(ctx, snapshotKey, body, rev)
}

func decodeSnapshot(value string) (Snapshot, bool) {
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		raw = []byte(value)
	}
	var snap Snapshot
	if json.Unmarshal(raw, &snap) != nil {
		return Snapshot{}, false
	}
	return snap, true
}

func sameUsage(a, b Snapshot) bool {
	return sameMap(a.Nodes, b.Nodes) && sameMap(a.Pods, b.Pods)
}

func sameMap(a, b map[string]Usage) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || av != bv {
			return false
		}
	}
	return true
}

func Apply(snap Snapshot, nodeName, text string) {
	for _, sample := range ParseProm(text) {
		switch sample.Name {
		case "node_cpu_usage_seconds_total", "node_memory_working_set_bytes":
			name := sample.Labels["node"]
			if name == "" {
				name = nodeName
			}
			if name == "" {
				continue
			}
			u := snap.Nodes[name]
			if strings.HasPrefix(sample.Name, "node_cpu") {
				u.CPUSeconds = sample.Value
			} else {
				u.MemoryBytes = sample.Value
			}
			snap.Nodes[name] = u
		case "container_cpu_usage_seconds_total", "container_memory_working_set_bytes":
			ns, pod := sample.Labels["namespace"], sample.Labels["pod"]
			if ns == "" || pod == "" {
				continue
			}
			key := ns + "/" + pod
			u := snap.Pods[key]
			if strings.HasPrefix(sample.Name, "container_cpu") {
				u.CPUSeconds += sample.Value
			} else {
				u.MemoryBytes += sample.Value
			}
			snap.Pods[key] = u
		}
	}
}

func ParseProm(text string) []Sample {
	var out []Sample
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := promLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		labels := map[string]string{}
		if m[3] != "" {
			for _, part := range strings.Split(m[3], ",") {
				kv := promLabel.FindStringSubmatch(part)
				if kv != nil {
					labels[kv[1]] = strings.ReplaceAll(kv[2], `\"`, `"`)
				}
			}
		}
		value, err := strconv.ParseFloat(m[4], 64)
		if err != nil {
			continue
		}
		out = append(out, Sample{Name: m[1], Labels: labels, Value: value})
	}
	return out
}

func listNodes(snap Snapshot) []any {
	names := make([]string, 0, len(snap.Nodes))
	for name := range snap.Nodes {
		names = append(names, name)
	}
	sort.Strings(names)
	items := []any{}
	for _, name := range names {
		if item := nodeMetrics(snap, name); item != nil {
			items = append(items, item)
		}
	}
	return items
}

func listPods(snap Snapshot, namespace string) []any {
	keys := make([]string, 0, len(snap.Pods))
	for key := range snap.Pods {
		if namespace == "" || strings.HasPrefix(key, namespace+"/") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	items := []any{}
	for _, key := range keys {
		ns, name, _ := strings.Cut(key, "/")
		if item := podMetrics(snap, ns, name); item != nil {
			items = append(items, item)
		}
	}
	return items
}

func nodeMetrics(snap Snapshot, name string) map[string]any {
	cur, ok := snap.Nodes[name]
	if !ok {
		return nil
	}
	var prev *Usage
	if snap.PrevNodes != nil {
		if p, ok := snap.PrevNodes[name]; ok {
			prev = &p
		}
	}
	return metricsObject("NodeMetrics", name, "", snap, cur, prev)
}

func podMetrics(snap Snapshot, namespace, name string) map[string]any {
	cur, ok := snap.Pods[namespace+"/"+name]
	if !ok {
		return nil
	}
	var prev *Usage
	if snap.PrevPods != nil {
		if p, ok := snap.PrevPods[namespace+"/"+name]; ok {
			prev = &p
		}
	}
	obj := metricsObject("PodMetrics", name, namespace, snap, cur, prev)
	obj["containers"] = []any{map[string]any{"name": name, "usage": usage(cur, prev, snap)}}
	delete(obj, "usage")
	return obj
}

func metricsObject(kind, name, namespace string, snap Snapshot, cur Usage, prev *Usage) map[string]any {
	ts := time.UnixMilli(snap.At).UTC().Format(time.RFC3339Nano)
	meta := map[string]any{"name": name, "creationTimestamp": ts}
	if namespace != "" {
		meta["namespace"] = namespace
	}
	return map[string]any{
		"kind": kind, "apiVersion": GV, "metadata": meta,
		"timestamp": ts, "window": windowOf(snap), "usage": usage(cur, prev, snap),
	}
}

func usage(cur Usage, prev *Usage, snap Snapshot) map[string]string {
	dt := scrapeEvery.Seconds()
	if snap.PrevAt != 0 {
		dt = float64(snap.At-snap.PrevAt) / 1000
		if dt < 1 {
			dt = 1
		}
	}
	cpu := 0.0
	if prev != nil {
		cpu = (cur.CPUSeconds - prev.CPUSeconds) / dt
		if cpu < 0 {
			cpu = 0
		}
	}
	mem := cur.MemoryBytes
	if mem < 0 {
		mem = 0
	}
	return map[string]string{"cpu": strconv.FormatInt(int64(cpu*1e9+0.5), 10) + "n", "memory": strconv.FormatInt(int64(mem+0.5), 10)}
}

func windowOf(snap Snapshot) string {
	s := int64(scrapeEvery.Seconds())
	if snap.PrevAt != 0 {
		s = (snap.At - snap.PrevAt) / 1000
		if s < 1 {
			s = 1
		}
	}
	return strconv.FormatInt(s, 10) + "s"
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
