package edgehost

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
)

func TestProvisionGatewayClassAccepted(t *testing.T) {
	key := classPrefix + "k8flare"
	obj := map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "GatewayClass",
		"metadata":   map[string]any{"name": "k8flare", "generation": 1},
		"spec":       map[string]any{"controllerName": gatewayController},
	}
	raw, _ := json.Marshal(obj)
	stored := map[string][]byte{key: raw}
	store := &kine.Client{HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/list":
			prefix := r.URL.Query().Get("prefix")
			var kvs []any
			for k, v := range stored {
				if strings.HasPrefix(k, prefix) {
					kvs = append(kvs, map[string]any{"key": k, "value": base64.StdEncoding.EncodeToString(v), "modRevision": 2})
				}
			}
			body, _ := json.Marshal(map[string]any{"kvs": kvs})
			return jsonResp(200, body), nil
		case "/kv":
			var req struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			decoded, _ := base64.StdEncoding.DecodeString(req.Value)
			stored[req.Key] = decoded
			return jsonResp(200, []byte(`{"revision":3}`)), nil
		}
		return jsonResp(404, []byte(`{"error":"missing"}`)), nil
	})}}
	req, _ := http.NewRequest(http.MethodPost, "/internal/gateway/provision", strings.NewReader(`{"keys":["`+key+`"]}`))
	w := &codeWriter{header: http.Header{}}
	ProvisionGateways(w, req, store)
	if w.code != http.StatusNoContent {
		t.Fatal(w.code)
	}
	var got struct {
		Status struct {
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
		} `json:"status"`
	}
	if json.Unmarshal(stored[key], &got) != nil || got.Status.Conditions[0].Type != "Accepted" || got.Status.Conditions[0].Status != "True" {
		t.Fatalf("%s", stored[key])
	}
}

func jsonResp(code int, body []byte) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{"Content-Type": {"application/json"}}}
}
