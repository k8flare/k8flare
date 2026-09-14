// Command deploycheck measures the wake-up paths that only a real
// deployment can exercise: on Cloudflare the pump window runs inside
// waitUntil and the isolate has a memory cap, neither of which wrangler
// dev enforces. It times the three paths that depend on a resident
// worker staying alive long enough to finish its work, and leaves the
// cluster as it found it.
//
//	go run ./deploycheck -server https://<worker> -token <admin token>
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	pollInterval = time.Second
	pollTimeout  = 3 * time.Minute
)

type cluster struct {
	server string
	token  string
	http   *http.Client
}

func main() {
	server := flag.String("server", "", "base URL of the deployed worker")
	token := flag.String("token", os.Getenv("ADMIN_TOKEN"), "admin bearer token")
	node := flag.String("node", "", "name of a Ready node to schedule onto; without it the bind timing is skipped")
	flag.Parse()
	if *server == "" || *token == "" {
		log.Fatal("deploycheck: -server and -token are required")
	}
	c := &cluster{server: *server, token: *token, http: &http.Client{Timeout: 30 * time.Second}}
	if err := c.run(*node); err != nil {
		log.Fatalf("deploycheck: %v", err)
	}
}

func (c *cluster) run(node string) error {
	stamp := time.Now().UnixNano()
	fmt.Printf("%-28s %s\n", "server", c.server)
	if err := c.report("namespace default ServiceAccount", func() error {
		return c.namespaceBootstrap(fmt.Sprintf("deploycheck-%d", stamp))
	}); err != nil {
		return err
	}
	if err := c.report("CRD Established", func() error {
		return c.crdEstablished(fmt.Sprintf("probe%ds.deploycheck.k8flare.dev", stamp%1000))
	}); err != nil {
		return err
	}
	if node == "" {
		fmt.Printf("%-28s skipped (pass -node)\n", "pod bound to a node")
		return nil
	}
	return c.report("pod bound to a node", func() error {
		return c.podBound(fmt.Sprintf("deploycheck-%d", stamp), node)
	})
}

func (c *cluster) report(what string, run func() error) error {
	start := time.Now()
	err := run()
	if err != nil {
		fmt.Printf("%-28s FAILED after %s: %v\n", what, time.Since(start).Round(time.Millisecond), err)
		return err
	}
	fmt.Printf("%-28s %s\n", what, time.Since(start).Round(time.Millisecond))
	return nil
}

func (c *cluster) namespaceBootstrap(name string) error {
	body := map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": name}}
	if err := c.post("/api/v1/namespaces", body); err != nil {
		return err
	}
	defer c.delete("/api/v1/namespaces/" + name)
	if err := c.poll("/api/v1/namespaces/"+name+"/serviceaccounts/default", nil); err != nil {
		return fmt.Errorf("default ServiceAccount: %w", err)
	}
	return c.poll("/api/v1/namespaces/"+name+"/configmaps/kube-root-ca.crt", nil)
}

func (c *cluster) crdEstablished(name string) error {
	plural, group := splitCRDName(name)
	body := map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]any{"name": name},
		"spec": map[string]any{
			"group": group,
			"names": map[string]any{"plural": plural, "singular": plural[:len(plural)-1], "kind": "Probe"},
			"scope": "Namespaced",
			"versions": []any{map[string]any{
				"name": "v1", "served": true, "storage": true,
				"schema": map[string]any{"openAPIV3Schema": map[string]any{"type": "object"}},
			}},
		},
	}
	if err := c.post("/apis/apiextensions.k8s.io/v1/customresourcedefinitions", body); err != nil {
		return err
	}
	defer c.delete("/apis/apiextensions.k8s.io/v1/customresourcedefinitions/" + name)
	return c.poll("/apis/apiextensions.k8s.io/v1/customresourcedefinitions/"+name, func(obj map[string]any) bool {
		status, _ := obj["status"].(map[string]any)
		conditions, _ := status["conditions"].([]any)
		for _, raw := range conditions {
			cond, _ := raw.(map[string]any)
			if cond["type"] == "Established" && cond["status"] == "True" {
				return true
			}
		}
		return false
	})
}

func (c *cluster) podBound(name, node string) error {
	body := map[string]any{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]any{"name": name},
		"spec": map[string]any{
			"containers": []any{map[string]any{"name": "c", "image": "registry.k8s.io/pause:3.10"}},
		},
	}
	if err := c.post("/api/v1/namespaces/default/pods", body); err != nil {
		return err
	}
	defer c.delete("/api/v1/namespaces/default/pods/" + name)
	return c.poll("/api/v1/namespaces/default/pods/"+name, func(obj map[string]any) bool {
		spec, _ := obj["spec"].(map[string]any)
		bound, _ := spec["nodeName"].(string)
		return bound != ""
	})
}

func splitCRDName(name string) (plural, group string) {
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			return name[:i], name[i+1:]
		}
	}
	return name, ""
}

func (c *cluster) post(path string, body any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.server+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, nil)
}

func (c *cluster) delete(path string) {
	req, err := http.NewRequest(http.MethodDelete, c.server+path, nil)
	if err != nil {
		return
	}
	_ = c.do(req, nil)
}

func (c *cluster) poll(path string, ready func(map[string]any) bool) error {
	deadline := time.Now().Add(pollTimeout)
	var last error
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, c.server+path, nil)
		if err != nil {
			return err
		}
		obj := map[string]any{}
		if last = c.do(req, &obj); last == nil {
			if ready == nil || ready(obj) {
				return nil
			}
		}
		time.Sleep(pollInterval)
	}
	if last != nil {
		return fmt.Errorf("timed out after %s: %w", pollTimeout, last)
	}
	return fmt.Errorf("timed out after %s", pollTimeout)
}

func (c *cluster) do(req *http.Request, out any) error {
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: HTTP %d: %s", req.Method, req.URL.Path, resp.StatusCode, trim(payload))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(payload, out)
}

func trim(payload []byte) string {
	const limit = 200
	if len(payload) > limit {
		return string(payload[:limit])
	}
	return string(payload)
}
