//go:build !js

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WriteSubnetEnv watches for the node's PodCIDR allocation and writes
// /run/flannel/subnet.env once available.
//
// In our architecture, the k3s agent's flannel informer often fails to sync
// during the startup burst (the Go WASM API handler is temporarily overloaded).
// This goroutine bypasses the informer by querying the API directly and writing
// the subnet.env file that flannel's CNI plugin needs.
//
// The node name is discovered by reading the k3s node ID file (which k3s
// creates at startup when WithNodeID=true) and combining it with the base
// hostname. The function lists all nodes and matches by name prefix.
func WriteSubnetEnv(ctx context.Context, serverURL, token, baseName string) {
	subnetDir := "/run/flannel"
	subnetFile := filepath.Join(subnetDir, "subnet.env")

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Skip if already written
			if _, err := os.Stat(subnetFile); err == nil {
				log.Printf("flannel: %s already exists", subnetFile)
				return
			}

			podCIDR, err := findMyPodCIDR(ctx, serverURL, token, baseName)
			if err != nil {
				log.Printf("flannel: failed to get PodCIDR: %v", err)
				continue
			}
			if podCIDR == "" {
				continue // PodCIDR not yet allocated
			}

			if err := os.MkdirAll(subnetDir, 0755); err != nil {
				log.Printf("flannel: failed to create %s: %v", subnetDir, err)
				continue
			}

			// Write subnet.env in the format flannel expects
			content := fmt.Sprintf("FLANNEL_NETWORK=10.42.0.0/16\nFLANNEL_SUBNET=%s\nFLANNEL_MTU=1500\nFLANNEL_IPMASQ=true\n", podCIDR)
			if err := os.WriteFile(subnetFile, []byte(content), 0644); err != nil {
				log.Printf("flannel: failed to write %s: %v", subnetFile, err)
				continue
			}

			log.Printf("flannel: wrote %s with subnet %s", subnetFile, podCIDR)
			return
		}
	}
}

// findMyPodCIDR discovers this node's PodCIDR by constructing the node name
// from the hostname and k3s node ID, then querying the API.
func findMyPodCIDR(ctx context.Context, serverURL, token, baseName string) (string, error) {
	// k3s with WithNodeID=true stores a random 8-char hex ID at /etc/rancher/node/id.
	// The full node name is "<baseName>-<id>".
	nodeIDFile := "/etc/rancher/node/id"
	idBytes, err := os.ReadFile(nodeIDFile)
	if err != nil {
		return "", fmt.Errorf("read node ID: %w", err)
	}
	nodeID := strings.TrimSpace(string(idBytes))
	if nodeID == "" {
		return "", fmt.Errorf("node ID file is empty")
	}

	nodeName := baseName + "-" + nodeID
	return getNodePodCIDR(ctx, serverURL, token, nodeName)
}

func getNodePodCIDR(ctx context.Context, serverURL, token, nodeName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", serverURL+"/api/v1/nodes/"+nodeName, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	var node struct {
		Spec struct {
			PodCIDR string `json:"podCIDR"`
		} `json:"spec"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&node); err != nil {
		return "", err
	}
	return node.Spec.PodCIDR, nil
}
