package edgehost

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
)

const servicePrefix = "/registry/services/"

func ProvisionServices(w http.ResponseWriter, r *http.Request, store *kine.Client) {
	if store == nil {
		http.Error(w, "storage unavailable", http.StatusBadGateway)
		return
	}
	var body struct {
		Keys []string `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	for _, key := range body.Keys {
		if !strings.HasPrefix(key, servicePrefix) {
			continue
		}
		if !provisionService(r.Context(), store, key) {
			http.Error(w, "status update failed", http.StatusBadGateway)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func provisionService(ctx context.Context, store *kine.Client, key string) bool {
	kv, _, err := store.Get(ctx, key)
	if err != nil || kv == nil {
		return true
	}
	raw, ok := decodeRaw(kv.Value)
	if !ok {
		return true
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return true
	}
	var meta struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if json.Unmarshal(obj["metadata"], &meta) != nil || meta.Name == "" || meta.Namespace == "" {
		return true
	}
	var spec struct {
		Type              string  `json:"type"`
		LoadBalancerClass *string `json:"loadBalancerClass"`
	}
	_ = json.Unmarshal(obj["spec"], &spec)
	var status struct {
		LoadBalancer struct {
			Ingress []struct {
				Hostname string `json:"hostname"`
			} `json:"ingress"`
		} `json:"loadBalancer"`
	}
	_ = json.Unmarshal(obj["status"], &status)
	hostname := IngressHostname(meta.Namespace, meta.Name)
	want := spec.Type == "LoadBalancer" && OwnsClass(deref(spec.LoadBalancerClass))
	ingress := status.LoadBalancer.Ingress
	have := len(ingress) == 1 && ingress[0].Hostname == hostname
	if want == have {
		return true
	}
	var next any
	if want {
		next = map[string]any{"loadBalancer": map[string]any{"ingress": []any{map[string]string{"hostname": hostname}}}}
	} else {
		next = map[string]any{"loadBalancer": map[string]any{"ingress": []any{}}}
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return false
	}
	obj["status"] = encoded
	out, err := json.Marshal(obj)
	if err != nil {
		return false
	}
	_, err = store.Put(ctx, key, out, kv.ModRevision)
	return err == nil
}

func decodeRaw(value string) ([]byte, bool) {
	var dest json.RawMessage
	if !decodeKV(value, &dest) {
		return nil, false
	}
	return dest, true
}
