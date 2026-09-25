package edgehost

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
)

const crdPrefix = "/registry/apiextensions.k8s.io/customresourcedefinitions/"
const controllerAnnot = "k8flare.io/controller"

func DispatchExtensions(w http.ResponseWriter, r *http.Request, store *kine.Client, hooks *http.Client) {
	if store == nil || hooks == nil {
		http.Error(w, "extensions unavailable", http.StatusBadGateway)
		return
	}
	var body struct {
		Messages []struct {
			Kind string `json:"kind"`
			Key  string `json:"key"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	controllers := crdControllers(r, store)
	for _, msg := range body.Messages {
		if msg.Kind != "change" {
			continue
		}
		name := controllerFor(msg.Key, controllers)
		if name == "" {
			continue
		}
		payload, _ := json.Marshal(msg)
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://hooks.internal/hook/"+name, bytes.NewReader(payload))
		if err != nil {
			http.Error(w, "extensions unavailable", http.StatusBadGateway)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := hooks.Do(req)
		if err != nil {
			http.Error(w, "extensions unavailable", http.StatusBadGateway)
			return
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			http.Error(w, "hook failed", http.StatusBadGateway)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func crdControllers(r *http.Request, store *kine.Client) map[string]string {
	out := map[string]string{}
	kvs, _, _, err := store.List(r.Context(), crdPrefix, "", 500)
	if err != nil {
		return out
	}
	for _, kv := range kvs {
		var crd struct {
			Spec struct {
				Group string `json:"group"`
				Names struct {
					Plural string `json:"plural"`
				} `json:"names"`
			} `json:"spec"`
			Metadata struct {
				Annotations map[string]string `json:"annotations"`
			} `json:"metadata"`
		}
		if !loadJSONValue(kv.Value, &crd) {
			continue
		}
		controller := crd.Metadata.Annotations[controllerAnnot]
		if controller == "" || crd.Spec.Group == "" || crd.Spec.Names.Plural == "" {
			continue
		}
		out[crd.Spec.Group+"/"+crd.Spec.Names.Plural] = controller
	}
	return out
}

func controllerFor(key string, controllers map[string]string) string {
	parts := strings.Split(key, "/")
	if len(parts) < 4 {
		return ""
	}
	return controllers[parts[2]+"/"+parts[3]]
}

func loadJSONValue(value string, dest any) bool {
	return decodeKV(value, dest)
}
