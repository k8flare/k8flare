package edgehost

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	"k8s.io/apiserver/pkg/endpoints/request"
)

func LocateStream(w http.ResponseWriter, r *http.Request, store *kine.Client, admission *http.Client) {
	if r.Header.Get("X-K8flare-Stream-Locate") != "1" {
		http.NotFound(w, r)
		return
	}
	ref, ok := parseStreamPath(r.URL.Path)
	if !ok || store == nil {
		writeForbidden(w, "pod is not scheduled")
		return
	}
	var pod struct {
		Spec struct {
			NodeName   string `json:"nodeName"`
			Containers []struct {
				Name string `json:"name"`
			} `json:"containers"`
		} `json:"spec"`
	}
	if !loadJSON(r, store, "/registry/pods/"+ref.namespace+"/"+ref.name, &pod) || pod.Spec.NodeName == "" {
		writeForbidden(w, "pod is not scheduled")
		return
	}
	q := streamQuery(r)
	container := q.Get("container")
	if container == "" && len(pod.Spec.Containers) > 0 {
		container = pod.Spec.Containers[0].Name
	}
	kind := ref.kind
	switch kind {
	case "portforward":
		kind = "portForward"
	case "log":
		// The kubelet serves logs at containerLogs, the same path podlog.go
		// builds for the non-websocket read.
		kind = "containerLogs"
	}
	path := "/node/" + pod.Spec.NodeName + "/" + kind + "/" + ref.namespace + "/" + ref.name
	if kind != "portForward" {
		path += "/" + container
	}
	incoming := r.URL.RawQuery
	if incoming == "" {
		incoming = r.Header.Get("X-Stream-Query")
	}
	// The kubelet serves logs as a plain HTTP read rather than an upgrade, so
	// the caller has to fetch rather than dial; the _q path segment exists for
	// the upgrade path, which cannot carry a query.
	transport, protocols := "websocket", channelProtocols
	if kind == "containerLogs" {
		transport, protocols = "http", readerProtocols
	} else if incoming != "" {
		path += "/_q/" + url.PathEscape(incoming)
	}
	target := "https://nodetunnel.internal" + path
	if incoming != "" {
		target += "?" + incoming
	}
	if admission != nil {
		if msg, ok := admitStream(r, admission, ref, q); !ok {
			writeForbidden(w, msg)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"node": pod.Spec.NodeName, "url": target, "transport": transport,
		"protocol": streamProtocol(r.Header.Get("Sec-WebSocket-Protocol"), protocols),
	})
}

// Channel and reader subprotocols, named as apimachinery's
// httpstream/wsstream does. exec, attach and portforward multiplex several
// streams over one socket and negotiate a channel protocol; a log is a single
// one-way stream and negotiates a reader protocol, where the messages are the
// exact bytes written, or base64 of them.
var (
	channelProtocols = []string{"v5.channel.k8s.io", "v4.channel.k8s.io"}
	readerProtocols  = []string{"binary.k8s.io", "base64.binary.k8s.io"}
)

// StreamProtocol picks the first subprotocol the client offered that this kind
// of stream can speak. An empty result means none matched, and the upgrade
// carries no Sec-WebSocket-Protocol -- which wsstream reads as binary.
func StreamProtocol(header string) string { return streamProtocol(header, channelProtocols) }

func streamProtocol(header string, want []string) string {
	var parts []string
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	for _, want := range want {
		for _, part := range parts {
			if part == want {
				return part
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

type streamRef struct {
	kind, namespace, name string
}

func parseStreamPath(path string) (streamRef, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 7 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "namespaces" || parts[4] != "pods" {
		return streamRef{}, false
	}
	kind := parts[6]
	if kind != "exec" && kind != "attach" && kind != "portforward" && kind != "log" {
		return streamRef{}, false
	}
	if parts[3] == "" || parts[5] == "" {
		return streamRef{}, false
	}
	return streamRef{kind: kind, namespace: parts[3], name: parts[5]}, true
}

func streamQuery(r *http.Request) url.Values {
	if r.URL.RawQuery != "" {
		return r.URL.Query()
	}
	if header := r.Header.Get("X-Stream-Query"); header != "" {
		q, _ := url.ParseQuery(header)
		return q
	}
	parts := strings.Split(r.URL.Path, "/")
	for i := len(parts) - 2; i >= 0; i-- {
		if parts[i] == "_q" && parts[i+1] != "" {
			raw, err := url.PathUnescape(parts[i+1])
			if err != nil {
				raw = parts[i+1]
			}
			q, _ := url.ParseQuery(raw)
			return q
		}
	}
	return url.Values{}
}

func admitStream(r *http.Request, admission *http.Client, ref streamRef, q url.Values) (string, bool) {
	userInfo, _ := request.UserFrom(r.Context())
	username, uid := "admin", ""
	groups := []string{"system:masters", "system:authenticated"}
	if userInfo != nil {
		username = userInfo.GetName()
		uid = userInfo.GetUID()
		if g := userInfo.GetGroups(); len(g) > 0 {
			groups = g
		}
	}
	kind := "PodExecOptions"
	if ref.kind == "attach" {
		kind = "PodAttachOptions"
	}
	if ref.kind == "portforward" {
		kind = "PodPortForwardOptions"
	}
	var object any
	if ref.kind == "portforward" {
		var ports []int
		for _, p := range append(q["ports"], q["port"]...) {
			n, err := strconv.Atoi(p)
			if err == nil && n > 0 {
				ports = append(ports, n)
			}
		}
		object = map[string]any{"apiVersion": "v1", "kind": kind, "ports": ports}
	} else {
		object = map[string]any{
			"apiVersion": "v1", "kind": kind,
			"stdin": queryFlag(q, "stdin", "input"), "stdout": queryFlag(q, "stdout", "output"),
			"stderr": queryFlag(q, "stderr", "error"), "tty": queryFlag(q, "tty"),
			"container": q.Get("container"), "command": q["command"],
		}
	}
	base := map[string]any{
		"name": ref.name, "namespace": ref.namespace,
		"resource":    map[string]string{"group": "", "version": "v1", "resource": "pods"},
		"subresource": ref.kind, "operation": "CONNECT",
		"kind":   map[string]string{"group": "", "version": "v1", "kind": kind},
		"object": object,
		"user":   map[string]any{"username": username, "uid": uid, "groups": groups, "extra": map[string][]string{}},
	}
	for _, phase := range []string{"admit", "validate"} {
		base["phase"] = phase
		body, _ := json.Marshal(base)
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://admission.internal/admit", bytes.NewReader(body))
		if err != nil {
			return "attaching to pod is not allowed", false
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := admission.Do(req)
		if err != nil {
			return "attaching to pod is not allowed", false
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			msg := strings.TrimSpace(string(raw))
			if msg == "" {
				msg = "attaching to pod is not allowed"
			}
			return msg, false
		}
		var out struct {
			Allowed bool            `json:"allowed"`
			Message string          `json:"message"`
			Object  json.RawMessage `json:"object"`
		}
		if json.Unmarshal(raw, &out) != nil || !out.Allowed {
			if out.Message == "" {
				out.Message = "attaching to pod is not allowed"
			}
			return out.Message, false
		}
		if len(out.Object) > 0 {
			base["object"] = out.Object
		}
	}
	return "", true
}

func writeForbidden(w http.ResponseWriter, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "attaching to pod is not allowed"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"kind":       "Status",
		"apiVersion": "v1",
		"metadata":   map[string]any{},
		"status":     "Failure",
		"message":    message,
		"reason":     "Forbidden",
		"code":       http.StatusForbidden,
	})
}

func queryFlag(q url.Values, keys ...string) bool {
	for _, key := range keys {
		v := q.Get(key)
		if v == "1" || v == "true" {
			return true
		}
	}
	return false
}
