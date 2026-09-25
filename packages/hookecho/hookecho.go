package hookecho

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

func Handler(api *http.Client, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/hook/")
		name, _, _ = strings.Cut(name, "/")
		switch name {
		case "admission-echo":
			admissionEcho(w, r)
		case "controller-echo":
			controllerEcho(w, r, api, token)
		case "convert-echo":
			convertEcho(w, r)
		default:
			http.Error(w, "unknown worker "+name, http.StatusNotFound)
		}
	})
}

func admissionEcho(w http.ResponseWriter, r *http.Request) {
	var review struct {
		APIVersion string `json:"apiVersion"`
		Request    struct {
			UID  string `json:"uid"`
			Name string `json:"name"`
			Kind struct {
				Kind string `json:"kind"`
			} `json:"kind"`
			Object struct {
				Metadata struct {
					Annotations map[string]string `json:"annotations"`
				} `json:"metadata"`
			} `json:"object"`
		} `json:"request"`
	}
	if json.NewDecoder(r.Body).Decode(&review) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	apiVersion := review.APIVersion
	if apiVersion == "" {
		apiVersion = "admission.k8s.io/v1"
	}
	uid := review.Request.UID
	if review.Request.Kind.Kind == "PodAttachOptions" {
		writeJSON(w, map[string]any{
			"apiVersion": apiVersion, "kind": "AdmissionReview",
			"response": map[string]any{"uid": uid, "allowed": false, "status": map[string]string{"message": "attaching to pod '" + review.Request.Name + "' is not allowed"}},
		})
		return
	}
	admit := review.Request.Object.Metadata.Annotations["k8flare.io/admit"]
	if admit == "deny" {
		writeJSON(w, map[string]any{
			"apiVersion": apiVersion, "kind": "AdmissionReview",
			"response": map[string]any{"uid": uid, "allowed": false, "status": map[string]string{"message": "denied by admission-echo"}},
		})
		return
	}
	response := map[string]any{"uid": uid, "allowed": true}
	if admit == "mutate" {
		patch, _ := json.Marshal([]any{map[string]string{"op": "add", "path": "/metadata/annotations/k8flare.io~1mutated", "value": "true"}})
		response["patchType"] = "JSONPatch"
		response["patch"] = base64.StdEncoding.EncodeToString(patch)
	}
	writeJSON(w, map[string]any{"apiVersion": apiVersion, "kind": "AdmissionReview", "response": response})
}

func controllerEcho(w http.ResponseWriter, r *http.Request, api *http.Client, token string) {
	var ev struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	}
	if json.NewDecoder(r.Body).Decode(&ev) != nil || ev.Type == "deleted" || ev.Key == "" || api == nil {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}
	parts := strings.Split(ev.Key, "/")
	if len(parts) < 6 {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}
	path := "/apis/" + parts[2] + "/v1/namespaces/" + parts[4] + "/" + parts[3] + "/" + parts[5]
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://apiserver.internal"+path, nil)
	if err != nil {
		http.Error(w, "get", http.StatusBadGateway)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := api.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
			http.Error(w, "get", http.StatusBadGateway)
			return
		}
		http.Error(w, "get", http.StatusBadGateway)
		return
	}
	var obj map[string]any
	if json.NewDecoder(resp.Body).Decode(&obj) != nil {
		resp.Body.Close()
		http.Error(w, "get", http.StatusBadGateway)
		return
	}
	resp.Body.Close()
	meta, _ := obj["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		obj["metadata"] = meta
	}
	ann, _ := meta["annotations"].(map[string]any)
	if ann == nil {
		ann = map[string]any{}
		meta["annotations"] = ann
	}
	if ann["k8flare.io/reconciled"] == "true" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}
	ann["k8flare.io/reconciled"] = "true"
	body, _ := json.Marshal(obj)
	put, err := http.NewRequestWithContext(r.Context(), http.MethodPut, "https://apiserver.internal"+path, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "put", http.StatusBadGateway)
		return
	}
	put.Header.Set("Authorization", "Bearer "+token)
	put.Header.Set("Content-Type", "application/json")
	putResp, err := api.Do(put)
	if err != nil || putResp.StatusCode != http.StatusOK {
		if putResp != nil {
			putResp.Body.Close()
		}
		http.Error(w, "put", http.StatusBadGateway)
		return
	}
	putResp.Body.Close()
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func convertEcho(w http.ResponseWriter, r *http.Request) {
	var review struct {
		APIVersion string `json:"apiVersion"`
		Request    struct {
			UID               string           `json:"uid"`
			DesiredAPIVersion string           `json:"desiredAPIVersion"`
			Objects           []map[string]any `json:"objects"`
		} `json:"request"`
	}
	if json.NewDecoder(r.Body).Decode(&review) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	objects := make([]any, 0, len(review.Request.Objects))
	for _, obj := range review.Request.Objects {
		objects = append(objects, convertHostPort(obj, review.Request.DesiredAPIVersion))
	}
	apiVersion := review.APIVersion
	if apiVersion == "" {
		apiVersion = "apiextensions.k8s.io/v1"
	}
	writeJSON(w, map[string]any{
		"apiVersion": apiVersion, "kind": "ConversionReview",
		"response": map[string]any{
			"uid": review.Request.UID, "convertedObjects": objects,
			"result": map[string]string{"status": "Success"},
		},
	})
}

func convertHostPort(obj map[string]any, desired string) map[string]any {
	next := map[string]any{}
	for k, v := range obj {
		next[k] = v
	}
	next["apiVersion"] = desired
	spec, _ := obj["spec"].(map[string]any)
	copied := map[string]any{}
	for k, v := range spec {
		copied[k] = v
	}
	version := desired
	if i := strings.LastIndex(desired, "/"); i >= 0 {
		version = desired[i+1:]
	}
	if version == "v2" {
		if hostPort, ok := copied["hostPort"].(string); ok {
			if cut := strings.LastIndex(hostPort, ":"); cut > 0 {
				copied["host"] = hostPort[:cut]
				copied["port"] = hostPort[cut+1:]
			}
			delete(copied, "hostPort")
		}
	}
	if version == "v1" && copied["host"] != nil && copied["port"] != nil {
		copied["hostPort"] = stringify(copied["host"]) + ":" + stringify(copied["port"])
		delete(copied, "host")
		delete(copied, "port")
	}
	next["spec"] = copied
	return next
}

func stringify(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		b, _ := json.Marshal(t)
		return strings.Trim(string(b), `"`)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
